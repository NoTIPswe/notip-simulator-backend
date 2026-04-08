//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

// realPublisher is a minimal ports.GatewayPublisher backed by a plain NATS connection.
// Used in integration tests to verify that the publish path actually delivers messages
// to a real NATS broker, without needing mTLS certificates.
type realPublisher struct {
	nc *nats.Conn
}

func (p *realPublisher) Publish(_ context.Context, subject string, payload []byte) error {
	return p.nc.Publish(subject, payload)
}

func (p *realPublisher) Close() error {
	if !p.nc.IsClosed() {
		p.nc.Close()
	}
	return nil
}

func (p *realPublisher) Reconnect(_ context.Context) error {
	// Not meaningful for plain connections in tests.
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests.
// ─────────────────────────────────────────────────────────────────────────────

func TestNATSPublisherPublishMessageArrivesOnSubscriber(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)

	pub := &realPublisher{nc: nc}

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
	pub := &realPublisher{nc: nc}

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
	pub := &realPublisher{nc: nc}

	require.NoError(t, pub.Close())

	// Publishing after close must return an error (connection is closed).
	err := pub.Publish(context.Background(), "telemetry.data.x.y", []byte(`{}`))
	assert.Error(t, err, "publishing on a closed connection must fail")
}

func TestNATSPublisherCommandACKDeliveredCorrectly(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)
	pub := &realPublisher{nc: nc}

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
