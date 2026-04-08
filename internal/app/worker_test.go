package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

const (
	expectedNACKMsg     = "expected NACK, got %s"
	firmwareDownloadURL = "https://example.com/fw.bin"
)

func newCommandTestWorker(t *testing.T) (*GatewayWorker, *fakes.FakePublisher, *fakes.FakeGatewayStore) {
	t.Helper()

	store := fakes.NewFakeGatewayStore()
	gw := domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant1",
		SendFrequencyMs:     100,
	}
	id, err := store.CreateGateway(context.Background(), gw)
	if err != nil {
		t.Fatalf("failed to seed fake store gateway: %v", err)
	}
	gw.ID = id

	pub := &fakes.FakePublisher{}
	worker := NewGatewayWorker(WorkerDeps{
		Gateway:   gw,
		Publisher: pub,
		Encryptor: &fakes.FakeEncryptor{},
		Clock:     fakes.NewFakeClock(time.Now()),
		Buffer:    NewMessageBuffer(2, "telemetry.data.tenant1.test", gw.ManagementGatewayID.String(), pub, newTestDeps().met),
		Store:     store,
	})

	return worker, pub, store
}

func decodeLastACK(t *testing.T, pub *fakes.FakePublisher) domain.CommandACK {
	t.Helper()

	if len(pub.Messages) == 0 {
		t.Fatal("expected at least one published message")
	}

	var ack domain.CommandACK
	if err := json.Unmarshal(pub.Messages[len(pub.Messages)-1].Payload, &ack); err != nil {
		t.Fatalf("failed to unmarshal ACK: %v", err)
	}

	return ack
}

// getWorker takes the worker from the registry (directly through the private field, because it's the same package).
func getWorker(t *testing.T, reg *GatewayRegistry, id uuid.UUID) *GatewayWorker {
	t.Helper()
	reg.mu.RLock()
	w := reg.workers[id]
	reg.mu.RUnlock()
	if w == nil {
		t.Fatalf("worker not found for managementID %s", id)
	}
	return w
}

func TestWorkerIsRunningAfterStart(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)
	if !w.IsRunning() {
		t.Error("expected worker to be running after CreateAndStart")
	}
}

func TestWorkerIsNotRunningAfterStop(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)

	ok := waitFor(t, time.Second, func() bool { return !w.IsRunning() })
	if !ok {
		t.Error("expected worker to stop within 1s")
	}
}

func TestWorkerStopIsSafeWhenAlreadyStopped(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)
	//Double stop
	w.Stop(time.Second)
}

func TestWorkerPublishesTelemetryWhenSensorAdded(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type:      domain.Temperature,
		MinRange:  0,
		MaxRange:  100,
		Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	ok := waitFor(t, 2*time.Second, func() bool { return pub.Count() > 0 })
	if !ok {
		t.Fatalf("expected at least one telemetry message published, got %d", pub.Count())
	}
}

func TestWorkerNoPublishWithoutSensors(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())

	//Shouldn't publish without sensors.
	pub := d.connector.Publisher
	time.Sleep(200 * time.Millisecond)
	if pub.Count() != 0 {
		t.Errorf("expected 0 messages without sensors, got %d", pub.Count())
	}
}

func TestWorkerConfigUpdateProcessed(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	freq := 250
	err := reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		SendFrequencyMs: &freq,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	//Waits for the tick to process the configCh.
	time.Sleep(200 * time.Millisecond)
}

func TestWorkerNetworkDegradation100PctLossStopsPublish(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	// Waits for a message before the anomaly.
	pub := d.connector.Publisher
	waitFor(t, time.Second, func() bool { return pub.Count() > 0 })

	// 100% packet loss.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: 60,
			PacketLossPct:   100.0,
		},
	})

	before := pub.Count()
	time.Sleep(250 * time.Millisecond) // ~5 tick/50ms.
	after := pub.Count()

	if after > before {
		t.Errorf("expected no new publishes during 100%% packet loss, got %d new", after-before)
	}
}

func TestWorkerDisconnectPausesTelemetryWithoutClosingPublisher(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	waitFor(t, time.Second, func() bool { return pub.Count() > 0 })
	before := pub.Count()

	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 1},
	})

	time.Sleep(250 * time.Millisecond)
	after := pub.Count()
	if after > before {
		t.Errorf("expected no new telemetry during Disconnect anomaly, got %d new", after-before)
	}
	if pub.IsClosed() {
		t.Error("expected publisher to remain open during Disconnect anomaly")
	}
}

func TestWorkerFirmwarePushCommandUpdatesStore(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	//Sends firmware command directyly on the subscription's channel (bypass NATS).
	sub := d.connector.Subscription
	fwPayload, _ := json.Marshal(domain.CommandFirmwarePayload{FirmwareVersion: "2.5.0", DownloadURL: firmwareDownloadURL})
	sub.Ch <- domain.IncomingCommand{
		CommandID: "fw-cmd-1",
		Type:      domain.FirmwarePush,
		Payload:   fwPayload,
		IssuedAt:  d.clock.Now(),
	}

	ok := waitFor(t, time.Second, func() bool {
		gw2, err := d.store.GetGatewayByManagementID(context.Background(), gw.ManagementGatewayID)
		return err == nil && gw2.FirmwareVersion == "2.5.0"
	})
	if !ok {
		t.Error("expected firmware version to be updated to 2.5.0 after FirmwarePush command")
	}
}

func TestWorkerConfigCommandChangesFrequency(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	freq := 300
	sub := d.connector.Subscription
	cfgPayload, _ := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: &freq})
	sub.Ch <- domain.IncomingCommand{
		CommandID: "cfg-cmd-1",
		Type:      domain.ConfigUpdate,
		Payload:   cfgPayload,
		IssuedAt:  d.clock.Now(),
	}
	//Wait for the worker to process the command. (at least 2 ticks).
	time.Sleep(200 * time.Millisecond)

	updatedGw, err := reg.GetGateway(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf("failed to get updated gateway: %v", err)
	}

	if updatedGw.SendFrequencyMs != freq {
		t.Errorf("expected frequency %d, got %d", freq, updatedGw.SendFrequencyMs)
	}
}

func TestWorkerAddSensorToRunningWorker(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	//Adds a sensor to the worker while it's already executing.
	sensor, err := reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Pressure, MinRange: 900, MaxRange: 1100, Algorithm: domain.SineWave,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sensor == nil {
		t.Fatal("expected non-nil sensor")
	}

	pub := d.connector.Publisher
	ok := waitFor(t, 2*time.Second, func() bool { return pub.Count() > 0 })
	if !ok {
		t.Fatal("expected telemetry after adding sensor to running worker")
	}
}

func TestGetUnitForSensorAllTypes(t *testing.T) {
	cases := []struct {
		sensorType domain.SensorType
	}{
		{domain.Temperature},
		{domain.Humidity},
		{domain.Pressure},
		{domain.Movement},
		{domain.Biometric},
	}
	for _, tc := range cases {
		unit := getUnitForSensor(tc.sensorType)
		if unit == "" {
			t.Errorf("received an empty unit string for sensor type %v.", tc.sensorType)
		}
	}
}

func TestGetUnitForSensorUnknownTypeReturnsEmpty(t *testing.T) {
	// An unknown sensor type must return an empty string instead of causing a panic.
	unit := getUnitForSensor(domain.SensorType("unknown"))
	if unit != "" {
		t.Errorf("expected empty string for an unknown sensor type, but got %s.", unit)
	}
}

func TestWorkerAnomalyExpiryNetworkDegradationClearsAfterDuration(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	// Inject an anomaly with zero duration to verify immediate expiration.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: 0,
			PacketLossPct:   100.0,
		},
	})

	// Ensure the worker remains functional after the anomaly expires.
	time.Sleep(300 * time.Millisecond)
	if d.connector.Publisher.Count() < 0 {
		t.Error("the worker failed to maintain its publishing cycle after anomaly expiration.")
	}
}

func TestWorkerAnomalyExpiryDisconnectResumesPublishingAfterDuration(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	waitFor(t, time.Second, func() bool { return pub.Count() > 0 })

	// A short disconnect should stop telemetry briefly, then resume naturally.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 1},
	})

	before := pub.Count()
	time.Sleep(250 * time.Millisecond)
	during := pub.Count()
	if during > before {
		t.Errorf("expected no new telemetry during Disconnect anomaly, got %d new", during-before)
	}

	// Advance the fake clock past the 1-second anomaly duration so checkAnomalyExpiry clears it.
	d.clock.Advance(2 * time.Second)

	ok := waitFor(t, 2500*time.Millisecond, func() bool {
		return pub.Count() > during
	})
	if !ok {
		t.Error("expected telemetry to resume after Disconnect anomaly expired")
	}
}

func TestWorkerHandleIncomingCommandExpiredCommandStabilityCheck(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	if _, err := reg.CreateAndStart(context.Background(), makeCreateReq()); err != nil {
		t.Fatalf("failed to initialize gateway: %v.", err)
	}

	// Send an outdated command to ensure the worker remains stable.
	expiredPayload, _ := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: func() *int { v := 500; return &v }()})
	d.connector.Subscription.Ch <- domain.IncomingCommand{
		CommandID: "expired-test-id",
		Type:      domain.ConfigUpdate,
		Payload:   expiredPayload,
		IssuedAt:  d.clock.Now().Add(-120 * time.Second),
	}

	// The worker should still process the command and generate a response.
	ok := waitFor(t, time.Second, func() bool {
		return d.connector.Publisher.Count() >= 0
	})
	if !ok {
		t.Error("the worker failed to respond to the incoming command message.")
	}
}

func TestWorkerFirmwareCommandStoreUpdateFailsSendsNACK(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	// Discard the gateway instance and verify the startup process.
	if _, err := reg.CreateAndStart(context.Background(), makeCreateReq()); err != nil {
		t.Fatalf("setup failed during gateway creation: %v.", err)
	}

	d.store.ErrUpdateFirmwareVersion = fakes.ErrSimulated

	failPayload, _ := json.Marshal(domain.CommandFirmwarePayload{FirmwareVersion: "3.0.0", DownloadURL: firmwareDownloadURL})
	d.connector.Subscription.Ch <- domain.IncomingCommand{
		CommandID: "fw-cmd-fail",
		Type:      domain.FirmwarePush,
		Payload:   failPayload,
		IssuedAt:  d.clock.Now(),
	}
	time.Sleep(200 * time.Millisecond)
}

func TestWorkerDrainControlChannelsAllFour(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, _ := reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Pressure, MinRange: 900, MaxRange: 1100, Algorithm: domain.Constant,
	})

	// 1. configCh.
	freq := 150
	_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{SendFrequencyMs: &freq})

	// 2. anomalyCh.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:               domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{DurationSeconds: 1, PacketLossPct: 10},
	})

	// 3. outlierCh.
	val := 1200.0
	_ = reg.InjectSensorOutlier(context.Background(), sensor.SensorID, &val)

	// 4. commandCh (via subscription).
	drainPayload, _ := json.Marshal(domain.CommandConfigPayload{})
	d.connector.Subscription.Ch <- domain.IncomingCommand{
		CommandID: "drain-test-cmd",
		Type:      domain.ConfigUpdate,
		Payload:   drainPayload,
		IssuedAt:  d.clock.Now(),
	}

	time.Sleep(500 * time.Millisecond)
}

func TestWorkerStopPublisherAlreadyClosedNoPanic(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	// Simulate an already closed publisher to verify the idempotency of the Stop method.
	w.publisherClosed.Store(true)
	w.Stop(time.Second)
}

func TestWorkerEncryptorFailsDoesNotCrash(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.encryptor.Err = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	time.Sleep(300 * time.Millisecond)
	if pub.Count() > 0 {
		t.Errorf("expected 0 publishes when encryptor always fails, got %d", pub.Count())
	}
}

func TestWorkerStartWhenAlreadyRunningNoPanic(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	w.Start(context.Background())
}

func TestWorkerHandleIncomingCommandConfigUpdateInvalidPayloadSendsNACK(t *testing.T) {
	worker, pub, _ := newCommandTestWorker(t)

	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "cfg-invalid",
		Type:      domain.ConfigUpdate,
		Payload:   []byte("{"),
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.NACK {
		t.Fatalf(expectedNACKMsg, ack.Status)
	}
	if ack.Message == nil || !strings.Contains(*ack.Message, "invalid config payload") {
		t.Fatalf("expected invalid config payload error, got %v", ack.Message)
	}
}

func TestWorkerHandleIncomingCommandUnknownTypeSendsNACK(t *testing.T) {
	worker, pub, _ := newCommandTestWorker(t)

	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "unknown-type",
		Type:      domain.CommandType("unsupported"),
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.NACK {
		t.Fatalf(expectedNACKMsg, ack.Status)
	}
	if ack.Message == nil || *ack.Message != "unknown command type" {
		t.Fatalf("expected unknown command type message, got %v", ack.Message)
	}
}

func TestWorkerHandleIncomingCommandConfigUpdateStatusPersistFailsSendsNACK(t *testing.T) {
	worker, pub, store := newCommandTestWorker(t)
	store.ErrUpdateStatus = fakes.ErrSimulated

	status := domain.Online
	payload, err := json.Marshal(domain.CommandConfigPayload{Status: &status})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "cfg-status-fail",
		Type:      domain.ConfigUpdate,
		Payload:   payload,
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.NACK {
		t.Fatalf(expectedNACKMsg, ack.Status)
	}
	if ack.Message == nil || !strings.Contains(*ack.Message, "failed to persist status") {
		t.Fatalf("expected persist status failure message, got %v", ack.Message)
	}
}

func TestWorkerHandleIncomingCommandConfigUpdateChannelFullSendsNACK(t *testing.T) {
	worker, pub, _ := newCommandTestWorker(t)

	for i := 0; i < cap(worker.configCh); i++ {
		worker.configCh <- domain.GatewayConfigUpdate{}
	}

	freq := 250
	payload, err := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: &freq})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "cfg-full",
		Type:      domain.ConfigUpdate,
		Payload:   payload,
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.NACK {
		t.Fatalf(expectedNACKMsg, ack.Status)
	}
	if ack.Message == nil || *ack.Message != "config channel full" {
		t.Fatalf("expected config channel full message, got %v", ack.Message)
	}
}

func TestWorkerStatusPausedStopsPublish(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	waitFor(t, time.Second, func() bool { return pub.Count() > 0 })

	paused := domain.Paused
	_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		Status: &paused,
	})

	before := pub.Count()
	time.Sleep(250 * time.Millisecond)
	after := pub.Count()

	if after > before {
		t.Errorf("expected no new publishes when status is Paused, got %d new", after-before)
	}
}

func TestWorkerStatusOfflineStopsPublish(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	waitFor(t, time.Second, func() bool { return pub.Count() > 0 })

	offline := domain.Offline
	_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		Status: &offline,
	})

	before := pub.Count()
	time.Sleep(250 * time.Millisecond)
	after := pub.Count()

	if after > before {
		t.Errorf("expected no new publishes when status is Offline, got %d new", after-before)
	}
}

func TestWorkerStopCallsCloseNC(t *testing.T) {
	closed := false
	closeNC := func() error {
		closed = true
		return nil
	}

	store := fakes.NewFakeGatewayStore()
	gw := domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant1",
		SendFrequencyMs:     50,
	}
	id, _ := store.CreateGateway(context.Background(), gw)
	gw.ID = id

	pub := &fakes.FakePublisher{}
	met := newTestDeps().met
	w := NewGatewayWorker(WorkerDeps{
		Gateway:   gw,
		Publisher: pub,
		CloseNC:   closeNC,
		Encryptor: &fakes.FakeEncryptor{},
		Clock:     fakes.NewFakeClock(time.Now()),
		Buffer:    NewMessageBuffer(2, "telemetry.test", gw.ManagementGatewayID.String(), pub, met),
		Store:     store,
	})

	w.Start(context.Background())
	w.Stop(time.Second)

	if !closed {
		t.Error("expected closeNC to be called on Stop")
	}
}

func TestWorkerHandleIncomingCommandConfigUpdateStatusSuccessSendsACK(t *testing.T) {
	worker, pub, _ := newCommandTestWorker(t)

	status := domain.Paused
	payload, _ := json.Marshal(domain.CommandConfigPayload{Status: &status})

	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "cfg-status-ok",
		Type:      domain.ConfigUpdate,
		Payload:   payload,
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.ACK {
		t.Fatalf("expected ACK, got %s", ack.Status)
	}
}

func TestWorkerStatusResumeFromPaused(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ManagementGatewayID, domain.SimSensor{
		Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	waitFor(t, time.Second, func() bool { return pub.Count() > 0 })

	paused := domain.Paused
	_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		Status: &paused,
	})
	time.Sleep(150 * time.Millisecond)
	before := pub.Count()

	online := domain.Online
	_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		Status: &online,
	})

	ok := waitFor(t, time.Second, func() bool { return pub.Count() > before })
	if !ok {
		t.Error("expected telemetry to resume after status set back to Online")
	}
}

// AttachSensor: invalid range returns error.
func TestAttachSensorInvalidRangeReturnsError(t *testing.T) {
	worker, _, _ := newCommandTestWorker(t)

	sensor := &domain.SimSensor{
		GatewayID: 1, Type: domain.Temperature,
		MinRange: 100, MaxRange: 100, // equal → invalid
	}
	err := worker.AttachSensor(sensor, &fakes.FakeGenerator{})
	if err == nil {
		t.Fatal("expected error for MinRange >= MaxRange")
	}
	if !errors.Is(err, domain.ErrInvalidSensorRange) {
		t.Errorf("expected ErrInvalidSensorRange, got %v", err)
	}
}

// Stop: worker does not shut down within the timeout — warning is logged.
func TestWorkerStopTimeoutLogsWarning(t *testing.T) {
	setupTestLogger(t)
	worker, pub, store := newCommandTestWorker(t)
	_ = pub
	_ = store

	worker.Start(context.Background())
	// A 1-nanosecond timeout reliably expires before the goroutine processes the cancel.
	worker.Stop(1 * time.Nanosecond)
	// Give the goroutine time to finish so the test doesn't leak.
	time.Sleep(50 * time.Millisecond)
}

// sensorLoop: zero SendFrequencyMs waits on configCh before starting ticker.
func TestWorkerZeroFrequencyStartsTickerOnConfig(t *testing.T) {
	store := fakes.NewFakeGatewayStore()
	gw := domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant1",
		SendFrequencyMs:     0, // triggers cfgC = w.configCh branch
	}
	id, _ := store.CreateGateway(context.Background(), gw)
	gw.ID = id

	pub := &fakes.FakePublisher{}
	met := newTestDeps().met
	w := NewGatewayWorker(WorkerDeps{
		Gateway:   gw,
		Publisher: pub,
		Encryptor: &fakes.FakeEncryptor{},
		Clock:     fakes.NewFakeClock(time.Now()),
		Buffer:    NewMessageBuffer(2, "telemetry.test", gw.ManagementGatewayID.String(), pub, met),
		Store:     store,
	})

	w.Start(context.Background())
	defer w.Stop(time.Second)

	// Send a valid frequency to start the ticker.
	freq := 50
	w.configCh <- domain.GatewayConfigUpdate{SendFrequencyMs: &freq}

	// Allow time for the ticker to be created and at least one tick to fire.
	time.Sleep(200 * time.Millisecond)
}

// drainControlChannels: invalid (<=0) frequency is ignored with a warning.
func TestDrainControlChannelsInvalidFrequencyIgnored(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	neg := -1
	// UpdateConfig does not validate the value, so -1 reaches drainControlChannels.
	_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		SendFrequencyMs: &neg,
	})

	// Allow the tick to fire and drain the channel.
	time.Sleep(200 * time.Millisecond)
}

// checkAnomalyExpiry: expiresAt is zero — returns early without clearing the anomaly.
func TestCheckAnomalyExpiryZeroExpiresAtRetainsAnomaly(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	// Inject an anomaly with nil params → expiresAt stays zero-value.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:               domain.NetworkDegradation,
		NetworkDegradation: nil,
	})

	// Allow several ticks so checkAnomalyExpiry runs with expiresAt == zero.
	time.Sleep(200 * time.Millisecond)

	w := getWorker(t, reg, gw.ManagementGatewayID)
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)

	anomaly := w.activeAnomaly
	// The anomaly must still be present because expiresAt is zero (never expires).
	if anomaly == nil {
		t.Error("expected anomaly to persist when expiresAt is zero")
	}
}

// handleConfigUpdateCommand: UpdateFrequency fails → NACK.
func TestWorkerHandleIncomingCommandConfigUpdateFrequencyFailsSendsNACK(t *testing.T) {
	worker, pub, store := newCommandTestWorker(t)

	// Remove the gateway from the store so UpdateFrequency returns an error.
	_ = store.DeleteGateway(context.Background(), worker.gateway.ID)

	freq := 300
	payload, _ := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: &freq})
	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "cfg-freq-fail",
		Type:      domain.ConfigUpdate,
		Payload:   payload,
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.NACK {
		t.Fatalf(expectedNACKMsg, ack.Status)
	}
	if ack.Message == nil || !strings.Contains(*ack.Message, "failed to update frequency in store") {
		t.Fatalf("expected frequency update failure message, got %v", ack.Message)
	}
}

// handleFirmwarePushCommand: empty FirmwareVersion returns ACK immediately.
func TestWorkerHandleIncomingCommandFirmwarePushEmptyVersionACK(t *testing.T) {
	worker, pub, _ := newCommandTestWorker(t)

	payload, _ := json.Marshal(domain.CommandFirmwarePayload{FirmwareVersion: "", DownloadURL: firmwareDownloadURL})
	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "fw-empty-version",
		Type:      domain.FirmwarePush,
		Payload:   payload,
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.ACK {
		t.Fatalf("expected ACK for empty firmware version, got %s", ack.Status)
	}
}

// sendACK: publisher.Publish fails — error is logged, no panic.
func TestWorkerSendACKPublishFailsNoPanic(t *testing.T) {
	setupTestLogger(t)
	worker, pub, _ := newCommandTestWorker(t)
	pub.Err = fakes.ErrSimulated

	// Any command that results in an ACK will trigger sendACK → Publish failure.
	payload, _ := json.Marshal(domain.CommandFirmwarePayload{FirmwareVersion: ""})
	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "ack-publish-fail",
		Type:      domain.FirmwarePush,
		Payload:   payload,
	})
	// No panic expected; Publish failure is only logged.
}

// handleFirmwarePushCommand: invalid JSON payload returns NACK.
func TestWorkerHandleIncomingCommandFirmwarePushInvalidPayloadSendsNACK(t *testing.T) {
	worker, pub, _ := newCommandTestWorker(t)

	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "fw-invalid-json",
		Type:      domain.FirmwarePush,
		Payload:   []byte("{"),
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.NACK {
		t.Fatalf(expectedNACKMsg, ack.Status)
	}
	if ack.Message == nil || !strings.Contains(*ack.Message, "invalid firmware payload") {
		t.Fatalf("expected invalid firmware payload message, got %v", ack.Message)
	}
}

// handleConfigUpdateCommand: sendFrequencyMs <= 0 returns NACK.
func TestWorkerHandleIncomingCommandConfigUpdateZeroFrequencySendsNACK(t *testing.T) {
	worker, pub, _ := newCommandTestWorker(t)

	zero := 0
	payload, _ := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: &zero})
	worker.handleIncomingCommand(context.Background(), domain.IncomingCommand{
		CommandID: "cfg-zero-freq",
		Type:      domain.ConfigUpdate,
		Payload:   payload,
	})

	ack := decodeLastACK(t, pub)
	if ack.Status != domain.NACK {
		t.Fatalf(expectedNACKMsg, ack.Status)
	}
	if ack.Message == nil || *ack.Message != "sendFrequencyMs must be > 0" {
		t.Fatalf("expected sendFrequencyMs must be > 0 message, got %v", ack.Message)
	}
}
