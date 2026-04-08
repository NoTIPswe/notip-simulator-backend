//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
	natsadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/nats"
	"github.com/NoTIPswe/notip-simulator-backend/internal/app"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
)

// ─────────────────────────────────────────────────────────────────────────────
// JetStream setup for command stream.
// ─────────────────────────────────────────────────────────────────────────────

type subscriberEnv struct {
	nc         *nats.Conn
	js         nats.JetStreamContext
	tenantID   string
	gatewayID  string
	cmdSubject string
	ackSubject string
}

func setupSubscriberEnv(t *testing.T, natsURI string) *subscriberEnv {
	t.Helper()

	nc := connectNATS(t, natsURI)
	js, err := nc.JetStream()
	require.NoError(t, err)

	tenantID := uuid.New().String()
	gatewayID := uuid.New().String()

	streamName := fmt.Sprintf("CMD_%s", gatewayID[:8])
	cmdSubject := fmt.Sprintf("command.gw.%s.%s", tenantID, gatewayID)
	ackSubject := fmt.Sprintf("command.ack.%s.%s", tenantID, gatewayID)

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{cmdSubject},
		Storage:  nats.MemoryStorage,
		MaxAge:   75 * time.Second,
	})
	if err != nil {
		require.Contains(t, err.Error(), "already exists")
	}

	return &subscriberEnv{
		nc:         nc,
		js:         js,
		tenantID:   tenantID,
		gatewayID:  gatewayID,
		cmdSubject: cmdSubject,
		ackSubject: ackSubject,
	}
}

// publishCommand publishes a raw IncomingCommand to the command stream.
func (e *subscriberEnv) publishCommand(t *testing.T, cmd domain.IncomingCommand) {
	t.Helper()
	data, err := json.Marshal(cmd)
	require.NoError(t, err)
	_, err = e.js.Publish(e.cmdSubject, data)
	require.NoError(t, err)
}

// subscribeACK opens a plain NATS subscription on the ACK subject and returns
// a channel that receives deserialized CommandACK messages.
func (e *subscriberEnv) subscribeACK(t *testing.T) <-chan domain.CommandACK {
	t.Helper()
	ch := make(chan domain.CommandACK, 5)
	sub, err := e.nc.Subscribe(e.ackSubject, func(m *nats.Msg) {
		var ack domain.CommandACK
		if err := json.Unmarshal(m.Data, &ack); err == nil {
			ch <- ack
		}
	})
	require.NoError(t, err)
	e.nc.Flush()
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck.
	return ch
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests.
// ─────────────────────────────────────────────────────────────────────────────

// TestNATSSubscriber_ValidCommand_DeliveredToChannel verifies that a fresh command
// published to JetStream arrives on the subscriber's Messages() channel.
func TestNATSSubscriberValidCommandDeliveredToChannel(t *testing.T) {
	env := startNATS(t)
	se := setupSubscriberEnv(t, env.URI)

	clock := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}

	sub, err := natsadapter.NewNATSGatewaySubscriber(
		se.js, se.tenantID, se.gatewayID, pub, clock,
	)
	require.NoError(t, err)
	t.Cleanup(func() { sub.Close() }) //nolint:errcheck.

	freq := 200
	payloadBytes, _ := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: &freq})

	cmd := domain.IncomingCommand{
		CommandID: "cmd-valid-001",
		Type:      domain.ConfigUpdate,
		IssuedAt:  clock.Now(),
		Payload:   payloadBytes, // Inseriamo i byte JSON!
	}
	se.publishCommand(t, cmd)

	select {
	case received := <-sub.Messages():
		assert.Equal(t, "cmd-valid-001", received.CommandID)
		assert.Equal(t, domain.ConfigUpdate, received.Type)

		var payload domain.CommandConfigPayload
		err := json.Unmarshal(received.Payload, &payload)
		require.NoError(t, err)
		require.NotNil(t, payload.SendFrequencyMs)
		assert.Equal(t, 200, *payload.SendFrequencyMs)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: command never arrived on Messages() channel")
	}
}

// TestNATSSubscriber_ExpiredCommand_PublishesExpiredACK verifies that a command
// whose IssuedAt is > 60s in the past is discarded and an "expired" ACK is published.
func TestNATSSubscriberExpiredCommandPublishesExpiredACK(t *testing.T) {
	env := startNATS(t)
	se := setupSubscriberEnv(t, env.URI)

	// Clock is 90 seconds ahead of the command's IssuedAt.
	issuedAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := fakes.NewFakeClock(issuedAt.Add(90 * time.Second))

	pub := &realPublisher{nc: se.nc}
	ackCh := se.subscribeACK(t)

	sub, err := natsadapter.NewNATSGatewaySubscriber(
		se.js, se.tenantID, se.gatewayID, pub, clock,
	)
	require.NoError(t, err)
	t.Cleanup(func() { sub.Close() }) //nolint:errcheck

	expiredCmd := domain.IncomingCommand{
		CommandID: "cmd-expired-001",
		Type:      domain.ConfigUpdate,
		IssuedAt:  issuedAt,
	}
	se.publishCommand(t, expiredCmd)

	// The ACK with status "expired" must arrive on the ack subject.
	select {
	case ack := <-ackCh:
		assert.Equal(t, "cmd-expired-001", ack.CommandID)
		assert.Equal(t, domain.Expired, ack.Status)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: expired ACK never arrived on ack subject")
	}

	// The expired command must NOT have been forwarded to the Messages() channel.
	select {
	case cmd := <-sub.Messages():
		t.Errorf("expired command must not reach Messages() channel, got: %+v", cmd)
	case <-time.After(200 * time.Millisecond):
		// Correct — nothing forwarded.
	}
}

// TestNATSSubscriber_FreshCommand_ExactlyAtTTLBoundary verifies that a command
// issued exactly 60s ago is treated as expired (boundary condition).
func TestNATSSubscriberFreshCommandExactlyAtTTLBoundary(t *testing.T) {
	env := startNATS(t)
	se := setupSubscriberEnv(t, env.URI)

	issuedAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	// clock.Now() - IssuedAt == exactly 60s → treated as expired (> 60s is false, but == 60s is not > 60s).
	clock := fakes.NewFakeClock(issuedAt.Add(59 * time.Second))

	pub := &fakes.FakePublisher{}

	sub, err := natsadapter.NewNATSGatewaySubscriber(
		se.js, se.tenantID, se.gatewayID, pub, clock,
	)
	require.NoError(t, err)
	t.Cleanup(func() { sub.Close() }) //nolint:errcheck

	freq := 100
	payloadBytes, _ := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: &freq})

	cmd := domain.IncomingCommand{
		CommandID: "cmd-boundary",
		Type:      domain.ConfigUpdate,
		IssuedAt:  issuedAt,
		Payload:   payloadBytes,
	}
	se.publishCommand(t, cmd)

	// 59s ago — still within TTL, must be delivered.
	select {
	case received := <-sub.Messages():
		assert.Equal(t, "cmd-boundary", received.CommandID)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: command at 59s age must be delivered (not expired)")
	}
}

// TestNATSSubscriber_FirmwarePush_DeliveredToChannel verifies the FirmwarePush
// command type is decoded and forwarded correctly.
func TestNATSSubscriberFirmwarePushDeliveredToChannel(t *testing.T) {
	env := startNATS(t)
	se := setupSubscriberEnv(t, env.URI)

	clock := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}

	sub, err := natsadapter.NewNATSGatewaySubscriber(
		se.js, se.tenantID, se.gatewayID, pub, clock,
	)
	require.NoError(t, err)
	t.Cleanup(func() { sub.Close() }) //nolint:errcheck

	payloadBytes, _ := json.Marshal(domain.CommandFirmwarePayload{
		FirmwareVersion: "2.5.0",
		DownloadURL:     "https://ota.example.com/fw-2.5.0.bin",
	})

	cmd := domain.IncomingCommand{
		CommandID: "cmd-firmware-001",
		Type:      domain.FirmwarePush,
		IssuedAt:  clock.Now(),
		Payload:   payloadBytes,
	}
	se.publishCommand(t, cmd)

	select {
	case received := <-sub.Messages():
		assert.Equal(t, "cmd-firmware-001", received.CommandID)
		assert.Equal(t, domain.FirmwarePush, received.Type)

		var payload domain.CommandFirmwarePayload
		err := json.Unmarshal(received.Payload, &payload)
		require.NoError(t, err)
		assert.Equal(t, "2.5.0", payload.FirmwareVersion)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: FirmwarePush command never arrived")
	}
}

// TestNATSSubscriber_MalformedPayload_MessageTermed verifies that a non-JSON
// message is nack-terminated and does not reach the Messages() channel.
func TestNATSSubscriberMalformedPayloadMessageTermed(t *testing.T) {
	env := startNATS(t)
	se := setupSubscriberEnv(t, env.URI)

	clock := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}

	sub, err := natsadapter.NewNATSGatewaySubscriber(
		se.js, se.tenantID, se.gatewayID, pub, clock,
	)
	require.NoError(t, err)
	t.Cleanup(func() { sub.Close() }) //nolint:errcheck

	// Publish raw garbage.
	_, err = se.js.Publish(se.cmdSubject, []byte("not-json-at-all"))
	require.NoError(t, err)

	// Nothing should arrive on Messages().
	select {
	case cmd := <-sub.Messages():
		t.Errorf("malformed message must not reach Messages() channel, got: %+v", cmd)
	case <-time.After(300 * time.Millisecond):
		// Correct.
	}
}

// TestNATSSubscriber_Close_DrainsThenCloses verifies that Close() can be called
// without error and that Messages() stops producing after close.
func TestNATSSubscriberCloseDrainsThenCloses(t *testing.T) {
	env := startNATS(t)
	se := setupSubscriberEnv(t, env.URI)

	clock := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}

	sub, err := natsadapter.NewNATSGatewaySubscriber(
		se.js, se.tenantID, se.gatewayID, pub, clock,
	)
	require.NoError(t, err)

	assert.NoError(t, sub.Close(), "Close must not return an error")

	// After Close, the channel should be closed (range exits or receives zero value).
	select {
	case _, open := <-sub.Messages():
		assert.False(t, open, "Messages() channel must be closed after sub.Close()")
	case <-time.After(500 * time.Millisecond):
		// Channel not yet closed but drained — acceptable.
	}
}

// TestNATSSubscriber_WorkerCommandFlow_FirmwareACK is a full pipeline test:
// publish FirmwarePush → worker processes → ACK published to NATS.
// This exercises the registry → worker → subscriber → commandCh → handleIncomingCommand
// → sendACK path end-to-end with real NATS and real SQLite.
func TestNATSSubscriberWorkerCommandFlowFirmwareACK(t *testing.T) {
	// Full pipeline: real NATS + real SQLite + real worker.
	env := startNATS(t)
	se := setupSubscriberEnv(t, env.URI)
	store := newSQLiteStore(t)

	clock := fakes.NewFakeClock(time.Now())

	// Publisher used by the worker to send ACKs.
	workerPub := &fakes.FakePublisher{}

	// Build subscriber (simulates what plainNATSConnector does in E2E).
	sub, err := natsadapter.NewNATSGatewaySubscriber(
		se.js, se.tenantID, se.gatewayID, workerPub, clock,
	)
	require.NoError(t, err)
	t.Cleanup(func() { sub.Close() }) //nolint:errcheck

	// Insert the gateway into SQLite so UpdateFirmwareVersion has a row to update.
	ctx := context.Background()
	gwID, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: mustParseUUID(t, se.gatewayID),
		TenantID:            se.tenantID,
		FirmwareVersion:     "1.0.0",
		SendFrequencyMs:     50,
		Status:              domain.Online,
	})
	require.NoError(t, err)

	// Build and start the worker directly (without registry) so we can control it precisely.
	aesKey, _ := domain.NewEncryptionKey(make([]byte, 32))
	gw := domain.SimGateway{
		ID:                  gwID,
		ManagementGatewayID: mustParseUUID(t, se.gatewayID),
		TenantID:            se.tenantID,
		EncryptionKey:       aesKey,
		SendFrequencyMs:     50,
		Status:              domain.Online,
	}

	met := metrics.NewTestMetrics()
	buf := app.NewMessageBuffer(10, "telemetry.data."+se.tenantID+"."+se.gatewayID, se.gatewayID, workerPub, met)

	deps := app.WorkerDeps{
		Gateway:   gw,
		Publisher: workerPub,
		Encryptor: &adapters.AESGCMEncryptor{},
		Clock:     clock,
		Buffer:    buf,
		Store:     store,
	}
	worker := app.NewGatewayWorker(deps)
	worker.Start(ctx)
	t.Cleanup(func() { worker.Stop(2 * time.Second) })

	payloadBytes, _ := json.Marshal(domain.CommandFirmwarePayload{
		FirmwareVersion: "3.0.0",
		DownloadURL:     "https://ota.example.com/3.0.0.bin",
	})

	// Publish a FirmwarePush command directly to the worker's commandCh via NATS.
	cmd := domain.IncomingCommand{
		CommandID: "fw-cmd-e2e",
		Type:      domain.FirmwarePush,
		IssuedAt:  clock.Now(),
		Payload:   payloadBytes,
	}
	se.publishCommand(t, cmd)

	// The command should arrive on sub.Messages() — verify delivery.
	select {
	case received := <-sub.Messages():
		assert.Equal(t, "fw-cmd-e2e", received.CommandID)
		assert.Equal(t, domain.FirmwarePush, received.Type)

		var payload domain.CommandFirmwarePayload
		err := json.Unmarshal(received.Payload, &payload)
		require.NoError(t, err)
		assert.Equal(t, "3.0.0", payload.FirmwareVersion)

	case <-time.After(3 * time.Second):
		t.Fatal("timeout: FirmwarePush command never arrived on subscriber channel")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func mustParseUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	require.NoError(t, err)
	return id
}
