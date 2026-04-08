package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
)

func newTestBuffer(pub *fakes.FakePublisher, capacity int) *MessageBuffer {
	met := metrics.NewTestMetrics()
	return NewMessageBuffer(capacity, "telemetry.test.gw1", "gw1", pub, met)
}

func TestBufferSendAndFlushDeliveryAll(t *testing.T) {
	pub := &fakes.FakePublisher{}
	buf := newTestBuffer(pub, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go buf.Flush(ctx)

	buf.Send([]byte("msg-1"))
	buf.Send([]byte("msg-2"))
	buf.Send([]byte("msg-3"))

	ok := waitFor(t, time.Second, func() bool { return pub.Count() >= 3 })
	if !ok {
		t.Errorf("expected 3 messages delivered, got %d", pub.Count())
	}
}

func TestBufferOverflowNoPanic(t *testing.T) {
	//overflow should drop old-1
	pub := &fakes.FakePublisher{Err: errors.New("nats unavailable")}
	buf := newTestBuffer(pub, 2)

	//sends more messages.
	buf.Send([]byte("old-1"))
	buf.Send([]byte("old-2"))
	buf.Send([]byte("new-3")) // overflow.
}

func TestBufferPublisherErrorDoesNotCrash(t *testing.T) {
	pub := &fakes.FakePublisher{Err: errors.New("transient error")}
	buf := newTestBuffer(pub, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go buf.Flush(ctx)
	buf.Send([]byte("data"))

	<-ctx.Done() // no panic = success.
}

func TestBufferContextCancelStopsFlush(t *testing.T) {
	pub := &fakes.FakePublisher{}
	buf := newTestBuffer(pub, 10)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		buf.Flush(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("Flush did not terminate after context cancellation")
	}
}

func TestBufferEmptyFlushNoPanic(t *testing.T) {
	pub := &fakes.FakePublisher{}
	buf := newTestBuffer(pub, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	go buf.Flush(ctx)
	<-ctx.Done()
}
