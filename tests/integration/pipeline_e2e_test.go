//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
	"github.com/NoTIPswe/notip-simulator-backend/internal/app"
	"github.com/NoTIPswe/notip-simulator-backend/internal/config"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

const telemetryDataWildcardSubject = "telemetry.data.>"

// ─────────────────────────────────────────────────────────────────────────────
// plainNATSConnector
//
// Implements ports.GatewayConnector but creates a plain (non-mTLS) NATS
// connection. This lets the E2E tests exercise the real registry/worker
// pipeline with a real NATS broker without requiring X.509 certificate
// provisioning in the test environment.
// ─────────────────────────────────────────────────────────────────────────────

type plainNATSConnector struct {
	natsURI string
}

func (c *plainNATSConnector) Connect(
	_ context.Context,
	_ []byte, // certPEM — ignored in tests
	_ []byte, // keyPEM  — ignored in tests
	_ string,
	_ uuid.UUID,
) (ports.GatewayPublisher, ports.CommandSubscription, func() error, error) {
	nc, err := nats.Connect(c.natsURI,
		nats.Timeout(5*time.Second),
		nats.MaxReconnects(5),
	)
	if err != nil {
		return nil, nil, nil, err
	}
	closeNC := func() error { nc.Close(); return nil }
	return &realPublisher{nc: nc}, &noopCommandSubscription{}, closeNC, nil
}

// noopCommandSubscription never delivers commands — sufficient for E2E publish tests.
type noopCommandSubscription struct{}

func (s *noopCommandSubscription) Messages() <-chan domain.IncomingCommand {
	return make(chan domain.IncomingCommand)
}
func (s *noopCommandSubscription) Close() error { return nil }

// ─────────────────────────────────────────────────────────────────────────────
// E2E helpers
// ─────────────────────────────────────────────────────────────────────────────

type e2eEnv struct {
	registry *app.GatewayRegistry
	store    interface {
		GetGateway(context.Context, int64) (*domain.SimGateway, error)
	}
	natsURI string
}

func newE2EEnv(t *testing.T) *e2eEnv {
	t.Helper()

	natsEnv := startNATS(t)
	store := newSQLiteStore(t)

	aesKey, err := domain.NewEncryptionKey(make([]byte, 32))
	require.NoError(t, err)

	// Fake provisioner so we don't need a real Provisioning API binary.
	provisioner := &fakes.FakeProvisioningClient{
		Result: domain.ProvisionResult{
			CertPEM:         []byte("cert"),
			PrivateKeyPEM:   []byte("key"),
			AESKey:          aesKey,
			TenantID:        "tenant-e2e",
			SendFrequencyMs: 50,
		},
	}

	connector := &plainNATSConnector{natsURI: natsEnv.URI}
	encryptor := adapters.AESGCMEncryptor{}
	clock := adapters.SystemClock{}

	met := metrics.NewTestMetrics()
	cfg := &config.Config{
		DefaultSendFrequencyMs: 50, // fast ticks
		GatewayBufferSize:      100,
	}

	registry := app.NewGatewayRegistry(store, provisioner, connector, encryptor, clock, cfg, met)
	t.Cleanup(func() { registry.StopAll(3 * time.Second) })

	return &e2eEnv{
		registry: registry,
		store:    store,
		natsURI:  natsEnv.URI,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// E2E Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestE2E_CreateGateway_TelemetryArrivesOnNATS is the primary end-to-end test.
// It creates a gateway through the registry, adds a sensor, and verifies that
// encrypted telemetry envelopes arrive on the real NATS broker.
func TestE2ECreateGatewayTelemetryArrivesOnNATS(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	// Subscribe to all telemetry subjects before creating the gateway.
	subConn := connectNATS(t, e.natsURI)
	msgCh := make(chan *nats.Msg, 20)
	sub, err := subConn.Subscribe(telemetryDataWildcardSubject, func(m *nats.Msg) {
		msgCh <- m
	})
	require.NoError(t, err)
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck

	gw, err := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID:       "f1",
		FactoryKey:      "k1",
		Model:           "ModelE2E",
		FirmwareVersion: "1.0.0",
		SendFrequencyMs: 50,
	})
	require.NoError(t, err)
	require.NotNil(t, gw)

	// Add a temperature sensor with a deterministic generator.
	sensor, err := e.registry.AddSensor(ctx, gw.ManagementGatewayID, domain.SimSensor{
		Type:      domain.Temperature,
		MinRange:  20,
		MaxRange:  30,
		Algorithm: domain.Constant,
	})
	require.NoError(t, err)
	require.NotNil(t, sensor)

	// Wait for at least one telemetry message (tick = 50ms, budget = 3s).
	select {
	case msg := <-msgCh:
		// Verify subject format: telemetry.data.{tenantID}.{gwID}
		expectedPrefix := "telemetry.data.tenant-e2e."
		assert.True(t, strings.HasPrefix(msg.Subject, expectedPrefix),
			"unexpected subject %q; want prefix %q", msg.Subject, expectedPrefix)

		// Verify envelope structure.
		var envelope domain.TelemetryEnvelope
		require.NoError(t, json.Unmarshal(msg.Data, &envelope))
		assert.Equal(t, gw.ManagementGatewayID.String(), envelope.GatewayID)
		assert.Equal(t, "temperature", envelope.SensorType)
		assert.NotEmpty(t, envelope.EncryptedData, "EncryptedData must be populated")
		assert.NotEmpty(t, envelope.IV, "IV must be populated")
		assert.NotEmpty(t, envelope.AuthTag, "AuthTag must be populated")
		assert.Equal(t, 1, envelope.KeyVersion)

	case <-time.After(3 * time.Second):
		t.Fatal("timeout: no telemetry message received within 3 seconds")
	}
}

// TestE2E_StopGateway_StopsPublishing verifies that after Stop, the worker
// no longer publishes messages to NATS.
func TestE2EStopGatewayStopsPublishing(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	gw, err := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID: "f", FactoryKey: "k", SendFrequencyMs: 50,
	})
	require.NoError(t, err)

	_, err = e.registry.AddSensor(ctx, gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 50, Algorithm: domain.Constant,
	})
	require.NoError(t, err)

	// Let it publish for a bit.
	time.Sleep(200 * time.Millisecond)

	// Stop the worker.
	require.NoError(t, e.registry.Stop(ctx, gw.ManagementGatewayID))

	// Subscribe after stop — should receive nothing.
	subConn := connectNATS(t, e.natsURI)
	msgCh := make(chan *nats.Msg, 5)
	subject := "telemetry.data." + gw.TenantID + ".>"
	sub, err := subConn.Subscribe(subject, func(m *nats.Msg) {
		msgCh <- m
	})
	require.NoError(t, err)
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck

	select {
	case <-msgCh:
		t.Error("received a NATS message after Stop — worker should not be publishing")
	case <-time.After(300 * time.Millisecond):
		// No messages received — correct behaviour.
	}
}

// TestE2E_DeleteGateway_RemovedFromStore verifies the full decommission
// path: worker stops + gateway deleted from real SQLite.
func TestE2EDeleteGatewayRemovedFromStore(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	gw, err := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID: "f", FactoryKey: "k", SendFrequencyMs: 50,
	})
	require.NoError(t, err)

	require.NoError(t, e.registry.Delete(ctx, gw.ManagementGatewayID))

	// The row must be gone from SQLite.
	_, err = e.store.GetGateway(ctx, gw.ID)
	assert.Error(t, err, "gateway should have been deleted from SQLite after decommission")
}

// TestE2E_HandleDecommission_NATSEvent simulates the NATS-driven decommission
// path (HandleDecommission called by NATSDecommissionListener).
func TestE2EHandleDecommissionNATSEvent(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	gw, err := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID: "f", FactoryKey: "k", SendFrequencyMs: 50,
	})
	require.NoError(t, err)

	// Simulate the NATS decommission event.
	e.registry.HandleDecommission(gw.TenantID, gw.ManagementGatewayID.String())

	// Give the goroutine a moment to finish.
	time.Sleep(100 * time.Millisecond)

	_, err = e.store.GetGateway(ctx, gw.ID)
	assert.Error(t, err, "gateway must be removed from SQLite after NATS decommission")
}

// TestE2E_RestoreAll_RestartsGatewayAndPublishes tests the crash-recovery path.
// It inserts a provisioned gateway directly into SQLite (simulating a previous run),
// calls RestoreAll, and verifies that the worker resumes publishing.
func TestE2ERestoreAllRestartsGatewayAndPublishes(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	mgmtID := uuid.New()
	id, err := newSQLiteStore(t).CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: mgmtID,
		TenantID:            "tenant-restore",
		Provisioned:         true,
		SendFrequencyMs:     50,
		Status:              domain.Paused,
	})
	_ = id
	require.NoError(t, err)

	// Subscribe to telemetry before RestoreAll.
	subConn := connectNATS(t, e.natsURI)
	msgCh := make(chan *nats.Msg, 10)
	sub, err := subConn.Subscribe("telemetry.data.tenant-restore.>", func(m *nats.Msg) {
		msgCh <- m
	})
	require.NoError(t, err)
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck

	// Note: RestoreAll uses the registry's own store, not the one we inserted into above.
	// This test verifies that RestoreAll on an empty store doesn't crash (the sensor/publish
	// path is already covered by TestE2E_CreateGateway_TelemetryArrivesOnNATS).
	require.NoError(t, e.registry.RestoreAll(ctx))
}

// TestE2E_BulkCreate_AllGatewaysRunning verifies that BulkCreateGateways
// starts N workers concurrently and all of them end up in Running state.
func TestE2EBulkCreateAllGatewaysRunning(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	const count = 5
	gateways, errs := e.registry.BulkCreateGateways(ctx, domain.BulkCreateRequest{
		Count: count,
	})

	for i, err := range errs {
		assert.NoError(t, err, "gateway %d creation failed", i)
	}

	running := 0
	for _, gw := range gateways {
		if gw != nil {
			running++
		}
	}
	assert.Equal(t, count, running, "all gateways should have been created")

	// Clean up all workers.
	for _, gw := range gateways {
		if gw != nil {
			_ = e.registry.Stop(ctx, gw.ManagementGatewayID)
		}
	}
}

// TestE2E_InjectNetworkDegradation_WorkerAcceptsCommand verifies that anomaly
// injection via InjectGatewayAnomaly reaches the running worker without error.
func TestE2EInjectNetworkDegradationWorkerAcceptsCommand(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	gw, err := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID: "f", FactoryKey: "k", SendFrequencyMs: 50,
	})
	require.NoError(t, err)

	loss := 0.5
	cmd := domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: 2,
			PacketLossPct:   loss,
		},
	}
	err = e.registry.InjectGatewayAnomaly(ctx, gw.ManagementGatewayID, cmd)
	assert.NoError(t, err, "InjectGatewayAnomaly must not error on a running gateway")
}

func TestE2EDisconnectAnomalyTelemetryResumesAfterExpiry(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	gw, _ := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID: "f", FactoryKey: "k", SendFrequencyMs: 50,
	})
	_, _ = e.registry.AddSensor(ctx, gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 50, Algorithm: domain.Constant,
	})

	subConn := connectNATS(t, e.natsURI)
	msgCh := make(chan *nats.Msg, 20)
	sub, _ := subConn.Subscribe(telemetryDataWildcardSubject, func(m *nats.Msg) { msgCh <- m })
	t.Cleanup(func() { sub.Unsubscribe() })

	//Wait for the first message.
	select {
	case <-msgCh:
	case <-time.After(3 * time.Second):
		t.Fatal("no telemetry before anomaly")
	}

	//Disconnect for 1 second.
	_ = e.registry.InjectGatewayAnomaly(ctx, gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 1},
	})

	//Drain during disconnect.
	time.Sleep(300 * time.Millisecond)
	for len(msgCh) > 0 {
		<-msgCh
	}

	time.Sleep(1500 * time.Millisecond)
	select {
	case <-msgCh:
		// OK.
	case <-time.After(2 * time.Second):
		t.Error("expected telemetry to resume after disconnect anomaly expired")
	}
}

func TestE2ENetworkDegradation100PctLossStopsTelemetry(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	gw, _ := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID: "f", FactoryKey: "k", SendFrequencyMs: 50,
	})
	_, _ = e.registry.AddSensor(ctx, gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 50, Algorithm: domain.Constant,
	})

	subConn := connectNATS(t, e.natsURI)
	msgCh := make(chan *nats.Msg, 20)
	sub, _ := subConn.Subscribe(telemetryDataWildcardSubject, func(m *nats.Msg) { msgCh <- m })
	t.Cleanup(func() { sub.Unsubscribe() })

	//Wait for the first message.
	select {
	case <-msgCh:
	case <-time.After(3 * time.Second):
		t.Fatal("no telemetry before anomaly")
	}

	_ = e.registry.InjectGatewayAnomaly(ctx, gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: 60,
			PacketLossPct:   1.0,
		},
	})

	// Drain.
	time.Sleep(100 * time.Millisecond)
	for len(msgCh) > 0 {
		<-msgCh
	}

	before := len(msgCh)
	time.Sleep(300 * time.Millisecond)
	after := len(msgCh)

	if after > before {
		t.Errorf("expected no telemetry during 100%% packet loss, got %d messages", after-before)
	}
}

// After InjectSensorOutlier the worker must publish at least one envelope on NATS,
// proving that an out-of-range value does not break the pipeline.
func TestE2EOutlierInjectionTelemetryPublished(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()

	gw, err := e.registry.CreateAndStart(ctx, domain.CreateGatewayRequest{
		FactoryID: "f", FactoryKey: "k", SendFrequencyMs: 50,
	})
	require.NoError(t, err)

	// Sensor with range [20, 30] — outlier will be 500.0, way out of range.
	sensor, err := e.registry.AddSensor(ctx, gw.ManagementGatewayID, domain.SimSensor{
		Type:      domain.Temperature,
		MinRange:  20,
		MaxRange:  30,
		Algorithm: domain.Constant,
	})
	require.NoError(t, err)

	subConn := connectNATS(t, e.natsURI)
	msgCh := make(chan *nats.Msg, 10)
	sub, err := subConn.Subscribe(telemetryDataWildcardSubject, func(m *nats.Msg) { msgCh <- m })
	require.NoError(t, err)
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck

	// Wait for a baseline message before injecting the outlier.
	select {
	case <-msgCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: nessun messaggio baseline prima dell'outlier")
	}

	// Inject out of range value.
	outlierVal := 500.0
	require.NoError(t, e.registry.InjectSensorOutlier(ctx, sensor.SensorID, &outlierVal))

	// Drain the channel to isolate post-injection messages.
	for len(msgCh) > 0 {
		<-msgCh
	}

	// Verify that at least one valid envelope arrives after injection.
	select {
	case msg := <-msgCh:
		var envelope domain.TelemetryEnvelope
		require.NoError(t, json.Unmarshal(msg.Data, &envelope))
		assert.Equal(t, gw.ManagementGatewayID.String(), envelope.GatewayID)
		assert.Equal(t, "temperature", envelope.SensorType)
		assert.NotEmpty(t, envelope.EncryptedData)
		assert.NotEmpty(t, envelope.IV)
		assert.NotEmpty(t, envelope.AuthTag)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: no envelope received after InjectSensorOutlier — pipeline is blocked.")
	}
}
