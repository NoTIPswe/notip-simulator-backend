package fakes

import (
	"context"
	"testing"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/google/uuid"
)

func TestFakeClockNowAndAdvance(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewFakeClock(start)

	if got := c.Now(); !got.Equal(start) {
		t.Fatalf("expected %v, got %v", start, got)
	}

	c.Advance(2 * time.Minute)
	if got := c.Now(); !got.Equal(start.Add(2 * time.Minute)) {
		t.Fatalf("expected advanced time, got %v", got)
	}
}

func TestFakeEncryptorEncrypt(t *testing.T) {
	e := &FakeEncryptor{}
	payload, err := e.Encrypt(domain.EncryptionKey{}, []byte("x"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.EncryptedData == "" || payload.IV == "" || payload.AuthTag == "" {
		t.Fatal("expected fake encrypted payload fields to be populated")
	}

	e.Err = ErrSimulated
	if _, err := e.Encrypt(domain.EncryptionKey{}, []byte("x")); err == nil {
		t.Fatal("expected simulated error")
	}
}

func TestFakePublisherBehavior(t *testing.T) {
	p := &FakePublisher{}
	if err := p.Publish(context.Background(), "s", []byte("p")); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if p.Count() != 1 {
		t.Fatalf("expected 1 message, got %d", p.Count())
	}

	if err := p.Reconnect(context.Background()); err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}
	if p.ReconnectCount() != 1 {
		t.Fatalf("expected 1 reconnect, got %d", p.ReconnectCount())
	}

	if err := p.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !p.IsClosed() {
		t.Fatal("expected publisher to be closed")
	}

	p.Err = ErrSimulated
	if err := p.Publish(context.Background(), "s", []byte("p")); err == nil {
		t.Fatal("expected publish error")
	}

	p.ReconnectErr = ErrSimulated
	if err := p.Reconnect(context.Background()); err == nil {
		t.Fatal("expected reconnect error")
	}
}

func TestFakeCommandSubscriptionBehavior(t *testing.T) {
	s := NewFakeCommandSubscription()
	if s.Messages() == nil {
		t.Fatal("expected non-nil messages channel")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !s.IsClosed() {
		t.Fatal("expected subscription to be marked closed")
	}
}

func TestFakeConnectorConnect(t *testing.T) {
	c := &FakeConnector{}
	pub, sub, closeNC, err := c.Connect(context.Background(), nil, nil, "t", uuid.New())
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if pub == nil || sub == nil || closeNC == nil {
		t.Fatal("expected publisher, subscriber and closeNC")
	}

	c.Err = ErrSimulated
	if _, _, _, err := c.Connect(context.Background(), nil, nil, "t", uuid.New()); err == nil {
		t.Fatal("expected connect error")
	}
}

func TestFakeProvisioningClientOnboard(t *testing.T) {
	p := &FakeProvisioningClient{Err: ErrSimulated}
	if _, err := p.Onboard(context.Background(), "f", "k", 100, "fw-1.0"); err == nil {
		t.Fatal("expected onboard error")
	}

	p.Err = nil
	p.Result = domain.ProvisionResult{CertPEM: []byte("cert")}
	res, err := p.Onboard(context.Background(), "f", "k", 100, "fw-1.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.CertPEM) != "cert" {
		t.Fatalf("unexpected result: %q", string(res.CertPEM))
	}
}

func TestFakeGatewayStoreGatewayCRUDAndUpdates(t *testing.T) {
	s := NewFakeGatewayStore()
	mgmtID := uuid.New()
	gw := domain.SimGateway{ManagementGatewayID: mgmtID, FirmwareVersion: "1.0.0"}

	id, err := s.CreateGateway(context.Background(), gw)
	if err != nil {
		t.Fatalf("create gateway failed: %v", err)
	}

	if _, err := s.GetGateway(context.Background(), id); err != nil {
		t.Fatalf("get gateway failed: %v", err)
	}
	if _, err := s.GetGatewayByManagementID(context.Background(), mgmtID); err != nil {
		t.Fatalf("get gateway by mgmt id failed: %v", err)
	}
	if list, err := s.ListGateways(context.Background()); err != nil || len(list) != 1 {
		t.Fatalf("expected 1 gateway, got %d err=%v", len(list), err)
	}

	if err := s.UpdateStatus(context.Background(), id, domain.Online); err != nil {
		t.Fatalf("update status failed: %v", err)
	}
	if err := s.UpdateFrequency(context.Background(), id, 250); err != nil {
		t.Fatalf("update frequency failed: %v", err)
	}
	if err := s.UpdateFirmwareVersion(context.Background(), id, "2.0.0"); err != nil {
		t.Fatalf("update fw failed: %v", err)
	}

	_, _ = s.GetGateway(context.Background(), id)

	prov := domain.ProvisionResult{CertPEM: []byte("cert"), PrivateKeyPEM: []byte("key")}
	if err := s.UpdateProvisioned(context.Background(), id, prov); err != nil {
		t.Fatalf("update provisioned failed: %v", err)
	}

	if err := s.DeleteGateway(context.Background(), id); err != nil {
		t.Fatalf("delete gateway failed: %v", err)
	}
	if _, err := s.GetGateway(context.Background(), id); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestFakeGatewayStoreSensorCRUD(t *testing.T) {
	s := NewFakeGatewayStore()
	gwID, _ := s.CreateGateway(context.Background(), domain.SimGateway{ManagementGatewayID: uuid.New()})

	sensor := domain.SimSensor{GatewayID: gwID, SensorID: uuid.New(), Type: domain.Temperature}
	sid, err := s.CreateSensor(context.Background(), sensor)
	if err != nil {
		t.Fatalf("create sensor failed: %v", err)
	}

	if _, err := s.GetSensor(context.Background(), sid); err != nil {
		t.Fatalf("get sensor failed: %v", err)
	}
	if list, err := s.ListSensors(context.Background(), gwID); err != nil || len(list) != 1 {
		t.Fatalf("expected 1 sensor, got %d err=%v", len(list), err)
	}
	if err := s.DeleteSensor(context.Background(), sid); err != nil {
		t.Fatalf("delete sensor failed: %v", err)
	}
	if _, err := s.GetSensor(context.Background(), sid); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestFakeGatewayStoreErrorPaths(t *testing.T) {
	s := NewFakeGatewayStore()
	s.ErrCreateGateway = ErrSimulated
	if _, err := s.CreateGateway(context.Background(), domain.SimGateway{}); err == nil {
		t.Fatal("expected create gateway error")
	}

	s.ErrCreateGateway = nil
	id, _ := s.CreateGateway(context.Background(), domain.SimGateway{ManagementGatewayID: uuid.New()})

	s.ErrGetGateway = ErrSimulated
	if _, err := s.GetGateway(context.Background(), id); err == nil {
		t.Fatal("expected get gateway error")
	}
	s.ErrGetGateway = nil

	s.ErrListGateways = ErrSimulated
	if _, err := s.ListGateways(context.Background()); err == nil {
		t.Fatal("expected list gateways error")
	}
	s.ErrListGateways = nil

	s.ErrUpdateStatus = ErrSimulated
	if err := s.UpdateStatus(context.Background(), id, domain.Online); err == nil {
		t.Fatal("expected update status error")
	}
	s.ErrUpdateStatus = nil

	s.ErrDeleteGateway = ErrSimulated
	if err := s.DeleteGateway(context.Background(), id); err == nil {
		t.Fatal("expected delete gateway error")
	}

	s2 := NewFakeGatewayStore()
	if err := s2.UpdateFrequency(context.Background(), 999, 100); err == nil {
		t.Fatal("expected update frequency error for missing gateway")
	}
}

func TestFakeGeneratorBehavior(t *testing.T) {
	g := &FakeGenerator{NextValue: 7.5}
	if got := g.Next(); got != 7.5 {
		t.Fatalf("expected 7.5, got %v", got)
	}
	g.InjectOutlier(11.2)
	if g.InjectOutlierCalled != 1 {
		t.Fatalf("expected 1 inject call, got %d", g.InjectOutlierCalled)
	}
	if len(g.InjectedOutliers) != 1 || g.InjectedOutliers[0] != 11.2 {
		t.Fatalf("unexpected outliers: %+v", g.InjectedOutliers)
	}
}

func TestFakeGatewayLifecycleServiceDefaultsAndCallbacks(t *testing.T) {
	svc := &FakeGatewayLifecycleService{}
	if _, err := svc.CreateAndStart(context.Background(), domain.CreateGatewayRequest{}); err != nil {
		t.Fatalf("default CreateAndStart should not error: %v", err)
	}
	if err := svc.Start(context.Background(), uuid.New()); err != nil {
		t.Fatalf("default Start should not error: %v", err)
	}
	if err := svc.Stop(context.Background(), uuid.New()); err != nil {
		t.Fatalf("default Stop should not error: %v", err)
	}
	if err := svc.Delete(context.Background(), uuid.New()); err != nil {
		t.Fatalf("default Decommission should not error: %v", err)
	}
	if _, err := svc.ListGateways(context.Background()); err != nil {
		t.Fatalf("default ListGateways should not error: %v", err)
	}
	if _, err := svc.GetGateway(context.Background(), uuid.New()); err != nil {
		t.Fatalf("default GetGateway should not error: %v", err)
	}

	called := false
	svc.StartFn = func(context.Context, uuid.UUID) error {
		called = true
		return nil
	}
	_ = svc.Start(context.Background(), uuid.New())
	if !called {
		t.Fatal("expected callback StartFn to be called")
	}
}

func TestFakeSensorManagementServiceDefaultsAndCallbacks(t *testing.T) {
	svc := &FakeSensorManagementService{}
	if _, err := svc.AddSensor(context.Background(), uuid.New(), domain.SimSensor{}); err != nil {
		t.Fatalf("default AddSensor should not error: %v", err)
	}
	if _, err := svc.ListSensors(context.Background(), uuid.New()); err != nil {
		t.Fatalf("default ListSensors should not error: %v", err)
	}
	if err := svc.DeleteSensor(context.Background(), uuid.New()); err != nil {
		t.Fatalf("default DeleteSensor should not error: %v", err)
	}

	called := false
	svc.DeleteSensorFn = func(context.Context, uuid.UUID) error {
		called = true
		return nil
	}
	_ = svc.DeleteSensor(context.Background(), uuid.New())
	if !called {
		t.Fatal("expected callback DeleteSensorFn to be called")
	}
}

func TestFakeSimulatorControlServiceDefaultsAndCallbacks(t *testing.T) {
	svc := &FakeSimulatorControlService{}
	if err := svc.UpdateConfig(context.Background(), uuid.New(), domain.GatewayConfigUpdate{}); err != nil {
		t.Fatalf("default UpdateConfig should not error: %v", err)
	}
	if err := svc.InjectGatewayAnomaly(context.Background(), uuid.New(), domain.GatewayAnomalyCommand{}); err != nil {
		t.Fatalf("default InjectGatewayAnomaly should not error: %v", err)
	}
	if err := svc.InjectSensorOutlier(context.Background(), uuid.New(), nil); err != nil {
		t.Fatalf("default InjectSensorOutlier should not error: %v", err)
	}

	called := false
	svc.InjectSensorOutlierFn = func(context.Context, uuid.UUID, *float64) error {
		called = true
		return nil
	}
	_ = svc.InjectSensorOutlier(context.Background(), uuid.New(), nil)
	if !called {
		t.Fatal("expected callback InjectSensorOutlierFn to be called")
	}
}

func TestFakeDecommissionEventReceiverCollectsCalls(t *testing.T) {
	r := &FakeDecommissionEventReceiver{}
	r.HandleDecommission("tenant", "gateway")
	if len(r.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.Calls))
	}
	if r.Calls[0].TenantID != "tenant" || r.Calls[0].ManagementGatewayID != "gateway" {
		t.Fatalf("unexpected call: %+v", r.Calls[0])
	}
}
