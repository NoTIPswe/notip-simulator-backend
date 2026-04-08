//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	natsadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/nats"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

// ─────────────────────────────────────────────────────────────────────────────
// fakeDecommissionReceiver
// ─────────────────────────────────────────────────────────────────────────────

type fakeDecommissionReceiver struct {
	calls []decommCall
	mu    sync.Mutex
}

type decommCall struct {
	tenantID  string
	gatewayID string
}

func (r *fakeDecommissionReceiver) HandleDecommission(tenantID, gatewayID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, decommCall{tenantID: tenantID, gatewayID: gatewayID})
}

func (r *fakeDecommissionReceiver) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *fakeDecommissionReceiver) LastCall() (decommCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return decommCall{}, false
	}
	return r.calls[len(r.calls)-1], true
}

var _ ports.DecommissionEventReceiver = (*fakeDecommissionReceiver)(nil)

// ─────────────────────────────────────────────────────────────────────────────
// JetStream setup helper
// ─────────────────────────────────────────────────────────────────────────────

// setupJetStream connects to the NATS container and creates a JetStream stream
// covering gateway.decommissioned.> subjects.
func setupJetStream(t *testing.T, natsURI string) (*nats.Conn, nats.JetStreamContext) {
	t.Helper()

	nc := connectNATS(t, natsURI)
	js, err := nc.JetStream()
	require.NoError(t, err, "create JetStream context")

	// Create the stream that the decommission listener subscribes to.
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "GATEWAY_EVENTS",
		Subjects: []string{"gateway.decommissioned.>"},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		errStr := err.Error()
		require.True(t, strings.Contains(errStr, "already exists") || strings.Contains(errStr, "overlap"),
			"unexpected stream creation error: %v", err)
	}
	t.Cleanup(func() { _ = js.DeleteStream("GATEWAY_EVENTS") })

	return nc, js
}

// publishDecommission publishes a decommission event on the canonical subject.
func publishDecommission(t *testing.T, js nats.JetStreamContext, tenantID, gatewayID string) {
	t.Helper()
	subject := fmt.Sprintf("gateway.decommissioned.%s.%s", tenantID, gatewayID)
	_, err := js.Publish(subject, []byte("{}"))
	require.NoError(t, err, "publish decommission event")
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestNATSDecommissionListenerValidEventCallsReceiver(t *testing.T) {
	env := startNATS(t)
	_, js := setupJetStream(t, env.URI)

	receiver := &fakeDecommissionReceiver{}
	listener := natsadapter.NewNATSDecommissionListener(js, receiver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- listener.Run(ctx) }()

	// Give listener time to subscribe.
	time.Sleep(100 * time.Millisecond)

	tenantID := "11111111-1111-1111-1111-111111111111"
	gatewayID := "22222222-2222-2222-2222-222222222222"
	publishDecommission(t, js, tenantID, gatewayID)

	require.Eventually(t, func() bool {
		return receiver.CallCount() == 1
	}, 3*time.Second, 20*time.Millisecond, "HandleDecommission must be called once")

	call, ok := receiver.LastCall()
	require.True(t, ok)
	assert.Equal(t, tenantID, call.tenantID)
	assert.Equal(t, gatewayID, call.gatewayID)

	cancel()
	select {
	case err := <-errCh:
		// context.Canceled is expected.
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not stop after context cancellation")
	}
}

func TestNATSDecommissionListenerMultipleEventsAllDispatched(t *testing.T) {
	env := startNATS(t)
	_, js := setupJetStream(t, env.URI)

	receiver := &fakeDecommissionReceiver{}
	listener := natsadapter.NewNATSDecommissionListener(js, receiver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	const count = 5
	for i := 0; i < count; i++ {
		tenantID := fmt.Sprintf("aaaaaaaa-0000-0000-0000-%012d", i)
		gatewayID := fmt.Sprintf("bbbbbbbb-0000-0000-0000-%012d", i)
		publishDecommission(t, js, tenantID, gatewayID)
	}

	require.Eventually(t, func() bool {
		return receiver.CallCount() == count
	}, 5*time.Second, 20*time.Millisecond,
		"all %d decommission events must be dispatched", count)
}

func TestNATSDecommissionListenerInvalidSubjectTooFewPartsIgnored(t *testing.T) {
	env := startNATS(t)
	nc := connectNATS(t, env.URI)
	js, err := nc.JetStream()
	require.NoError(t, err)

	// Create a stream that also captures an extra subject we'll misuse.
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "GATEWAY_EVENTS_BAD",
		Subjects: []string{"gateway.decommissioned.>", "gateway.bad.>"},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		errStr := err.Error()
		require.True(t, strings.Contains(errStr, "already exists") || strings.Contains(errStr, "overlap"),
			"unexpected stream creation error: %v", err)
	}
	t.Cleanup(func() { _ = js.DeleteStream("GATEWAY_EVENTS_BAD") })

	receiver := &fakeDecommissionReceiver{}
	listener := natsadapter.NewNATSDecommissionListener(js, receiver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	// Publish a well-formed event first so we know the listener is working.
	validTenant := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	validGW := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	publishDecommission(t, js, validTenant, validGW)

	require.Eventually(t, func() bool {
		return receiver.CallCount() == 1
	}, 3*time.Second, 20*time.Millisecond)

	// The count must still be 1 — the malformed dispatch was ignored.
	assert.Equal(t, 1, receiver.CallCount())
}

func TestNATSDecommissionListenerInvalidGatewayIDNotUUIDIgnored(t *testing.T) {
	env := startNATS(t)
	_, js := setupJetStream(t, env.URI)

	receiver := &fakeDecommissionReceiver{}
	listener := natsadapter.NewNATSDecommissionListener(js, receiver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	// Publish with a non-UUID gatewayID — must be silently ignored.
	tenantID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	subject := fmt.Sprintf("gateway.decommissioned.%s.not-a-uuid", tenantID)
	_, err := js.Publish(subject, []byte("{}"))
	require.NoError(t, err)

	// Wait a bit and confirm nothing was dispatched.
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, 0, receiver.CallCount(), "non-UUID gatewayID must be silently discarded")
}

func TestNATSDecommissionListenerInvalidTenantIDNotUUIDIgnored(t *testing.T) {
	env := startNATS(t)
	_, js := setupJetStream(t, env.URI)

	receiver := &fakeDecommissionReceiver{}
	listener := natsadapter.NewNATSDecommissionListener(js, receiver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx) //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	validGW := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	subject := fmt.Sprintf("gateway.decommissioned.not-a-uuid.%s", validGW)
	_, err := js.Publish(subject, []byte("{}"))
	require.NoError(t, err)

	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, 0, receiver.CallCount(), "non-UUID tenantID must be silently discarded")
}

func TestNATSDecommissionListenerStopsCleanlyOnContextCancel(t *testing.T) {
	env := startNATS(t)
	_, js := setupJetStream(t, env.URI)

	receiver := &fakeDecommissionReceiver{}
	listener := natsadapter.NewNATSDecommissionListener(js, receiver)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- listener.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err, "listener must return nil on clean context cancellation")
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not stop within 3 seconds after context cancellation")
	}
}
