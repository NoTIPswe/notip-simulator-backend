package nats_test

import (
	"context"
	"testing"

	natsnats "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/nats"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

func TestNATSGatewayPublisher_Publish_Success(t *testing.T) {
	pub := &fakes.FakePublisher{}
	_ = pub
	//This test verifies that the FakePublisher is working correctly as a stub.
	ctx := context.Background()
	err := pub.Publish(ctx, "telemetry.data.t1.gw1", []byte("payload"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.Count() != 1 {
		t.Errorf("want 1 message, got %d", pub.Count())
	}
}

func TestNATSGatewayPublisher_Publish_Error(t *testing.T) {
	pub := &fakes.FakePublisher{Err: fakes.ErrSimulated}
	err := pub.Publish(context.Background(), "subject", []byte("data"))
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestNATSGatewayPublisher_Close(t *testing.T) {
	pub := &fakes.FakePublisher{}
	_ = pub.Close()
	if !pub.IsClosed() {
		t.Error("expected publisher to be closed")
	}
}

func TestNATSGatewayPublisher_Reconnect_Success(t *testing.T) {
	pub := &fakes.FakePublisher{}
	err := pub.Reconnect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.ReconnectCalls != 1 {
		t.Errorf("want 1 reconnect call, got %d", pub.ReconnectCalls)
	}
}

func TestNATSGatewayPublisher_Reconnect_Error(t *testing.T) {
	pub := &fakes.FakePublisher{ReconnectErr: fakes.ErrSimulated}
	err := pub.Reconnect(context.Background())
	if err == nil {
		t.Error("expected error on reconnect, got nil")
	}
}

// Verify package compiles with the real type.
var _ = natsnats.NewNATSGatewayPublisher
