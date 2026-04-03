package nats

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

func newFakeMsg(data []byte) *nats.Msg {
	return &nats.Msg{Data: data}
}

func newSubscriberForTest(clk *fakes.FakeClock, pub *fakes.FakePublisher) *NATSGatewaySubscriber {
	return &NATSGatewaySubscriber{
		ch:         make(chan domain.IncomingCommand, 8),
		publisher:  pub,
		clock:      clk,
		ackSubject: "command.ack.t1.gw1",
	}
}

func validCmd(issuedAt time.Time) []byte {
	payload, _ := json.Marshal(domain.CommandConfigPayload{SendFrequencyMs: intPtr(1000)})
	cmd := domain.IncomingCommand{
		CommandID: uuid.New().String(),
		Type:      domain.ConfigUpdate,
		IssuedAt:  issuedAt,
		Payload:   payload,
	}
	b, _ := json.Marshal(cmd)
	return b
}

func intPtr(v int) *int { return &v }

func TestSubscriberHandleMsgValidCommandEnqueuedAndAcked(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}
	s := newSubscriberForTest(clk, pub)

	msg := newFakeMsg(validCmd(clk.Now()))
	s.handleMsg(msg)

	select {
	case cmd := <-s.ch:
		if cmd.Type != domain.ConfigUpdate {
			t.Errorf("want ConfigUpdate, got %v", cmd.Type)
		}
	default:
		t.Error("expected command in channel, got none")
	}
}

func TestSubscriberHandleMsgExpiredCommandACKAndDiscard(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}
	s := newSubscriberForTest(clk, pub)

	// IssuedAt 120s ago → expired.
	expiredData := validCmd(clk.Now().Add(-120 * time.Second))
	msg := newFakeMsg(expiredData)
	s.handleMsg(msg)

	// Command must NOT be in channel.
	select {
	case <-s.ch:
		t.Error("expired command should not be enqueued")
	default:
	}

	// An Expired ACK must have been published.
	if pub.Count() != 1 {
		t.Errorf("want 1 expired ACK published, got %d", pub.Count())
	}

	var ack domain.CommandACK
	if err := json.Unmarshal(pub.Messages[0].Payload, &ack); err != nil {
		t.Fatalf("unmarshal ACK: %v", err)
	}
	if ack.Status != domain.Expired {
		t.Errorf("want Expired ACK status, got %v", ack.Status)
	}
}

func TestSubscriberHandleMsgInvalidJSONTermed(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}
	s := newSubscriberForTest(clk, pub)

	msg := newFakeMsg([]byte("not-json"))
	s.handleMsg(msg)

	select {
	case <-s.ch:
		t.Error("invalid JSON should not produce a command")
	default:
	}
	if pub.Count() != 0 {
		t.Error("no ACK should be published for malformed messages")
	}
}

func TestSubscriberHandleMsgChannelFullNacked(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}
	s := &NATSGatewaySubscriber{
		ch:         make(chan domain.IncomingCommand), // zero capacity
		publisher:  pub,
		clock:      clk,
		ackSubject: "command.ack.t1.gw1",
	}

	msg := newFakeMsg(validCmd(clk.Now()))
	s.handleMsg(msg)

	select {
	case <-s.ch:
		t.Error("should not enqueue when channel is full")
	default:
	}
}

func TestSubscriberMessagesReturnsChan(t *testing.T) {
	s := newSubscriberForTest(fakes.NewFakeClock(time.Now()), &fakes.FakePublisher{})
	ch := s.Messages()
	if ch == nil {
		t.Error("Messages() should return non-nil channel")
	}
}

func TestSubscriberCloseDrainsChan(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}
	s := newSubscriberForTest(clk, pub)

	// Enqueue one command
	s.ch <- domain.IncomingCommand{CommandID: "x", Type: domain.ConfigUpdate, IssuedAt: clk.Now()}

	// Close should not panic even with a pending message.
	_ = s.Close()
}

func TestSubscriberFirmwarePushEnqueuedCorrectly(t *testing.T) {
	clk := fakes.NewFakeClock(time.Now())
	pub := &fakes.FakePublisher{}
	s := newSubscriberForTest(clk, pub)

	payload, _ := json.Marshal(domain.CommandFirmwarePayload{
		FirmwareVersion: "v2.0.0",
		DownloadURL:     "http://example.com/fw.bin",
	})
	cmd := domain.IncomingCommand{
		CommandID: uuid.New().String(),
		Type:      domain.FirmwarePush,
		IssuedAt:  clk.Now(),
		Payload:   payload,
	}
	data, _ := json.Marshal(cmd)
	msg := newFakeMsg(data)
	s.handleMsg(msg)

	select {
	case received := <-s.ch:
		if received.Type != domain.FirmwarePush {
			t.Errorf("want FirmwarePush, got %v", received.Type)
		}
		var fp domain.CommandFirmwarePayload
		if err := json.Unmarshal(received.Payload, &fp); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		if fp.FirmwareVersion != "v2.0.0" {
			t.Errorf("want v2.0.0, got %s", fp.FirmwareVersion)
		}
	default:
		t.Error("expected command in channel")
	}
}
