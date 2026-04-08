package nats

import (
	"testing"

	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
	"github.com/google/uuid"
)

func TestDecommissionListenerExtractIDsValidSubject(t *testing.T) {
	tenantID := uuid.New().String()
	gatewayID := uuid.New().String()
	subject := "gateway.decommissioned." + tenantID + "." + gatewayID

	receiver := &fakes.FakeDecommissionEventReceiver{}
	l := &NATSDecommissionListener{receiver: receiver}

	l.dispatchFromSubject(subject)

	if len(receiver.Calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(receiver.Calls))
	}
	if receiver.Calls[0].TenantID != tenantID {
		t.Errorf("want tenantID %s, got %s", tenantID, receiver.Calls[0].TenantID)
	}
	if receiver.Calls[0].ManagementGatewayID != gatewayID {
		t.Errorf("want gatewayID %s, got %s", gatewayID, receiver.Calls[0].ManagementGatewayID)
	}
}

func TestDecommissionListenerExtractIDsInvalidFormat(t *testing.T) {
	receiver := &fakes.FakeDecommissionEventReceiver{}
	l := &NATSDecommissionListener{receiver: receiver}

	l.dispatchFromSubject("gateway.decommissioned.only-two-parts")

	if len(receiver.Calls) != 0 {
		t.Error("invalid subject should not trigger HandleDecommission")
	}
}

func TestDecommissionListenerExtractIDsInvalidGatewayUUID(t *testing.T) {
	tenantID := uuid.New().String()
	subject := "gateway.decommissioned." + tenantID + ".not-a-uuid"

	receiver := &fakes.FakeDecommissionEventReceiver{}
	l := &NATSDecommissionListener{receiver: receiver}

	l.dispatchFromSubject(subject)

	if len(receiver.Calls) != 0 {
		t.Error("invalid gateway UUID should not trigger HandleDecommission")
	}
}

func TestNewNATSDecommissionListenerConstruction(t *testing.T) {
	receiver := &fakes.FakeDecommissionEventReceiver{}
	l := NewNATSDecommissionListener(nil, receiver)
	if l == nil {
		t.Fatal("expected non-nil listener")
	}
	if l.receiver != receiver {
		t.Error("receiver not stored correctly")
	}
}

func TestDecommissionListenerExtractIDsInvalidTenantUUID(t *testing.T) {
	gatewayID := uuid.New().String()
	subject := "gateway.decommissioned.not-a-uuid." + gatewayID

	receiver := &fakes.FakeDecommissionEventReceiver{}
	l := &NATSDecommissionListener{receiver: receiver}

	l.dispatchFromSubject(subject)

	if len(receiver.Calls) != 0 {
		t.Error("invalid tenant UUID should not trigger HandleDecommission")
	}
}
