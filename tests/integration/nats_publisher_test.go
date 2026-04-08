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

	natsadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/nats"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

func addJetStreamStream(t *testing.T, nc *nats.Conn, subjects ...string) {
	t.Helper()

	js, err := nc.JetStream()
	require.NoError(t, err)

	streamName := "TEST_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: subjects,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteStream(streamName) })
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests.
// ─────────────────────────────────────────────────────────────────────────────

func TestNATSPublisherPublishMessageArrivesOnSubscriber(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)
	addJetStreamStream(t, nc, "telemetry.data.tenant-1.gw-abc")

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, env.URI)
	require.NoError(t, err)

	// Subscribe on a separate connection so we receive our own publish.
	subConn := connectNATS(t, env.URI)
	msgCh := make(chan *nats.Msg, 1)
	sub, err := subConn.Subscribe("telemetry.data.tenant-1.gw-abc", func(m *nats.Msg) {
		msgCh <- m
	})
	require.NoError(t, err)
	subConn.Flush()
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck

	// Build a real TelemetryEnvelope just like the worker does.
	envelope := domain.TelemetryEnvelope{
		GatewayID:     "gw-abc",
		SensorID:      "sensor-1",
		SensorType:    "temperature",
		Timestamp:     time.Now().UTC(),
		KeyVersion:    1,
		EncryptedData: "enc",
		IV:            "iv",
		AuthTag:       "tag",
	}
	payload, err := json.Marshal(envelope)
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "telemetry.data.tenant-1.gw-abc", payload)
	require.NoError(t, err)

	select {
	case msg := <-msgCh:
		var got domain.TelemetryEnvelope
		require.NoError(t, json.Unmarshal(msg.Data, &got))
		assert.Equal(t, envelope.GatewayID, got.GatewayID)
		assert.Equal(t, envelope.SensorType, got.SensorType)
		assert.Equal(t, envelope.EncryptedData, got.EncryptedData)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: NATS message never arrived")
	}
}

func TestNATSPublisherPublishSubjectRouting(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)
	addJetStreamStream(t, nc, "telemetry.data.>")

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, env.URI)
	require.NoError(t, err)

	subConn := connectNATS(t, env.URI)
	hitCh := make(chan string, 5)

	// Wildcard subscription covering all telemetry subjects.
	sub, err := subConn.Subscribe("telemetry.data.>", func(m *nats.Msg) {
		hitCh <- m.Subject
	})
	require.NoError(t, err)
	subConn.Flush()
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck.

	subjects := []string{
		"telemetry.data.tenant-A.gw-1",
		"telemetry.data.tenant-A.gw-2",
		"telemetry.data.tenant-B.gw-1",
	}
	for _, s := range subjects {
		require.NoError(t, pub.Publish(context.Background(), s, []byte(`{}`)))
	}

	received := make(map[string]bool)
	deadline := time.After(3 * time.Second)
	for len(received) < len(subjects) {
		select {
		case subj := <-hitCh:
			received[subj] = true
		case <-deadline:
			t.Fatalf("timeout: only received subjects %v, expected %v", received, subjects)
		}
	}

	for _, s := range subjects {
		assert.True(t, received[s], "expected to receive on subject %s", s)
	}
}

func TestNATSPublisherCloseStopsPublishing(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)
	addJetStreamStream(t, nc, "telemetry.data.x.y")

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, env.URI)
	require.NoError(t, err)

	require.NoError(t, pub.Close())
	assert.NoError(t, nc.Publish("telemetry.data.x.y", []byte(`{}`)), "publisher close must not close the shared NATS connection")
}

func TestNATSPublisherCommandACKDeliveredCorrectly(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)
	addJetStreamStream(t, nc, "command.ack.>")

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, env.URI)
	require.NoError(t, err)

	subConn := connectNATS(t, env.URI)
	ackCh := make(chan *nats.Msg, 1)

	ackSubject := "command.ack.tenant-1.gw-abc"
	sub, err := subConn.Subscribe(ackSubject, func(m *nats.Msg) {
		ackCh <- m
	})
	require.NoError(t, err)
	subConn.Flush()
	t.Cleanup(func() { sub.Unsubscribe() }) //nolint:errcheck

	ack := domain.CommandACK{
		CommandID: "cmd-123",
		Status:    domain.ACK,
		Timestamp: time.Now().UTC(),
	}
	ackPayload, err := json.Marshal(ack)
	require.NoError(t, err)

	require.NoError(t, pub.Publish(context.Background(), ackSubject, ackPayload))

	select {
	case msg := <-ackCh:
		var got domain.CommandACK
		require.NoError(t, json.Unmarshal(msg.Data, &got))
		assert.Equal(t, "cmd-123", got.CommandID)
		assert.Equal(t, domain.ACK, got.Status)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: ACK message never arrived")
	}
}

func TestNATSPublisherPublishReturnsErrorWithoutMatchingStream(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, env.URI)
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "telemetry.data.no.stream", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish message")
}

func TestNATSPublisherReconnectContextCanceled(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, env.URI)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = pub.Reconnect(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNATSPublisherReconnectErrorWithInvalidServer(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, "nats://127.0.0.1:1")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = pub.Reconnect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reconnect to NATS")
}

func TestNATSPublisherReconnectSuccess(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)
	addJetStreamStream(t, nc, "telemetry.data.reconnect")

	pub, err := natsadapter.NewNATSGatewayPublisher(nc, env.URI)
	require.NoError(t, err)

	require.NoError(t, pub.Reconnect(context.Background()))
	require.NoError(t, pub.Publish(context.Background(), "telemetry.data.reconnect", []byte(`{}`)))
}
