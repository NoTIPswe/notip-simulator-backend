package fakes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

// ErrSimulated is the standard error used to test the fake failure paths.
var ErrSimulated = errors.New("simulated fake error")

const GatewaynotFound = "gateway %d not found"

type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{now: t}
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Encryptor.
type FakeEncryptor struct {
	Err error
}

func (e *FakeEncryptor) Encrypt(_ domain.EncryptionKey, _ []byte) (domain.EncryptedPayload, error) {
	if e.Err != nil {
		return domain.EncryptedPayload{}, e.Err
	}
	return domain.EncryptedPayload{
		EncryptedData: "fake-encrypted",
		IV:            "fake-iv",
		AuthTag:       "fake-tag",
	}, nil
}

// Publisher & Subscriber.
type PublishedMessage struct {
	Subject string
	Payload []byte
}

type FakePublisher struct {
	mu             sync.Mutex
	Messages       []PublishedMessage
	Err            error
	ReconnectErr   error
	ReconnectCalls int
	Closed         bool
}

func (p *FakePublisher) Publish(_ context.Context, subject string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Err != nil {
		return p.Err
	}
	p.Messages = append(p.Messages, PublishedMessage{Subject: subject, Payload: payload})
	return nil
}

func (p *FakePublisher) Reconnect(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ReconnectCalls++
	return p.ReconnectErr
}

func (p *FakePublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Closed = true
	return nil
}

func (p *FakePublisher) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.Messages)
}

func (p *FakePublisher) ReconnectCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ReconnectCalls
}

func (p *FakePublisher) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.Closed
}

type FakeCommandSubscription struct {
	mu     sync.Mutex
	Ch     chan domain.IncomingCommand
	closed bool
}

func NewFakeCommandSubscription() *FakeCommandSubscription {
	return &FakeCommandSubscription{Ch: make(chan domain.IncomingCommand, 10)}
}

func (s *FakeCommandSubscription) Messages() <-chan domain.IncomingCommand {
	return s.Ch
}

func (s *FakeCommandSubscription) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *FakeCommandSubscription) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Connector.
type FakeConnector struct {
	mu           sync.Mutex
	Publisher    *FakePublisher
	Subscription *FakeCommandSubscription
	Err          error
}

func (c *FakeConnector) Connect(_ context.Context, _ []byte, _ []byte, _ string, _ uuid.UUID) (ports.GatewayPublisher, ports.CommandSubscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Err != nil {
		return nil, nil, c.Err
	}
	if c.Publisher == nil {
		c.Publisher = &FakePublisher{}
	}
	if c.Subscription == nil {
		c.Subscription = NewFakeCommandSubscription()
	}
	return c.Publisher, c.Subscription, nil
}

//Provisioning Client.

type FakeProvisioningClient struct {
	Result domain.ProvisionResult
	Err    error
}

func (p *FakeProvisioningClient) Onboard(_ context.Context, _, _, _ string, _ uuid.UUID) (domain.ProvisionResult, error) {
	return p.Result, p.Err
}

// GatewayStore.
type FakeGatewayStore struct {
	mu       sync.RWMutex
	gateways map[int64]*domain.SimGateway
	sensors  map[int64]*domain.SimSensor
	nextGwID int64
	nextSnID int64

	// Granular errors for simulating specific failures.
	ErrCreateGateway            error
	ErrGetGateway               error
	ErrGetGatewayByManagementID error
	ErrListGateways             error
	ErrUpdateProvisioned        error
	ErrUpdateStatus             error
	ErrUpdateFirmwareVersion    error
	ErrDeleteGateway            error
	ErrCreateSensor             error
	ErrGetSensor                error
	ErrListSensors              error
	ErrDeleteSensor             error
}

func NewFakeGatewayStore() *FakeGatewayStore {
	return &FakeGatewayStore{
		gateways: make(map[int64]*domain.SimGateway),
		sensors:  make(map[int64]*domain.SimSensor),
		nextGwID: 1,
		nextSnID: 1,
	}
}

// Gateway Methods.
func (s *FakeGatewayStore) CreateGateway(_ context.Context, gw domain.SimGateway) (int64, error) {
	if s.ErrCreateGateway != nil {
		return 0, s.ErrCreateGateway
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextGwID
	s.nextGwID++
	gw.ID = id
	s.gateways[id] = &gw
	return id, nil
}

func (s *FakeGatewayStore) GetGateway(_ context.Context, id int64) (*domain.SimGateway, error) {
	if s.ErrGetGateway != nil {
		return nil, s.ErrGetGateway
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	gw, ok := s.gateways[id]
	if !ok {
		return nil, fmt.Errorf(GatewaynotFound, id)
	}
	cp := *gw
	return &cp, nil
}

func (s *FakeGatewayStore) GetGatewayByManagementID(_ context.Context, managementID uuid.UUID) (*domain.SimGateway, error) {
	if s.ErrGetGatewayByManagementID != nil {
		return nil, s.ErrGetGatewayByManagementID
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, gw := range s.gateways {
		if gw.ManagementGatewayID == managementID {
			cp := *gw
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("gateway with mgmt ID %s not found", managementID)
}

func (s *FakeGatewayStore) ListGateways(_ context.Context) ([]*domain.SimGateway, error) {
	if s.ErrListGateways != nil {
		return nil, s.ErrListGateways
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*domain.SimGateway, 0, len(s.gateways))
	for _, gw := range s.gateways {
		list = append(list, gw)
	}
	return list, nil
}

func (s *FakeGatewayStore) UpdateProvisioned(_ context.Context, id int64, result domain.ProvisionResult) error {
	if s.ErrUpdateProvisioned != nil {
		return s.ErrUpdateProvisioned
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	gw, ok := s.gateways[id]
	if !ok {
		return fmt.Errorf(GatewaynotFound, id)
	}
	gw.Provisioned = true
	gw.CertPEM = result.CertPEM
	gw.PrivateKeyPEM = result.PrivateKeyPEM
	gw.EncryptionKey = result.AESKey
	return nil
}

func (s *FakeGatewayStore) UpdateStatus(_ context.Context, id int64, status domain.GatewayStatus) error {
	if s.ErrUpdateStatus != nil {
		return s.ErrUpdateStatus
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	gw, ok := s.gateways[id]
	if !ok {
		return fmt.Errorf(GatewaynotFound, id)
	}
	gw.Status = status
	return nil
}

func (s *FakeGatewayStore) UpdateFirmwareVersion(_ context.Context, id int64, version string) error {
	if s.ErrUpdateFirmwareVersion != nil {
		return s.ErrUpdateFirmwareVersion
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	gw, ok := s.gateways[id]
	if !ok {
		return fmt.Errorf(GatewaynotFound, id)
	}
	gw.FirmwareVersion = version
	return nil
}

func (s *FakeGatewayStore) DeleteGateway(_ context.Context, id int64) error {
	if s.ErrDeleteGateway != nil {
		return s.ErrDeleteGateway
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.gateways, id)
	return nil
}

// Sensor Methods.
func (s *FakeGatewayStore) CreateSensor(_ context.Context, sensor domain.SimSensor) (int64, error) {
	if s.ErrCreateSensor != nil {
		return 0, s.ErrCreateSensor
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextSnID
	s.nextSnID++
	sensor.ID = id
	s.sensors[id] = &sensor
	return id, nil
}

func (s *FakeGatewayStore) GetSensor(_ context.Context, id int64) (*domain.SimSensor, error) {
	if s.ErrGetSensor != nil {
		return nil, s.ErrGetSensor
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sn, ok := s.sensors[id]
	if !ok {
		return nil, fmt.Errorf("sensor %d not found", id)
	}
	return sn, nil
}

func (s *FakeGatewayStore) ListSensors(_ context.Context, gatewayID int64) ([]*domain.SimSensor, error) {
	if s.ErrListSensors != nil {
		return nil, s.ErrListSensors
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*domain.SimSensor
	for _, sn := range s.sensors {
		if sn.GatewayID == gatewayID {
			list = append(list, sn)
		}
	}
	return list, nil
}

func (s *FakeGatewayStore) DeleteSensor(_ context.Context, id int64) error {
	if s.ErrDeleteSensor != nil {
		return s.ErrDeleteSensor
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sensors, id)
	return nil
}

func (f *FakeGatewayStore) UpdateFrequency(ctx context.Context, id int64, frequency int) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if gw, ok := f.gateways[id]; ok {
		gw.SendFrequencyMs = frequency
		return nil
	}
	return fmt.Errorf("gateway not found")
}

// Generator.
type FakeGenerator struct {
	mu                  sync.Mutex
	NextValue           float64
	InjectedOutliers    []float64
	InjectOutlierCalled int
}

func (g *FakeGenerator) Next() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.NextValue
}

func (g *FakeGenerator) InjectOutlier(value float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.InjectOutlierCalled++
	g.InjectedOutliers = append(g.InjectedOutliers, value)
}

//Driving Ports.

type FakeGatewayLifecycleService struct {
	CreateAndStartFn     func(ctx context.Context, req domain.CreateGatewayRequest) (*domain.SimGateway, error)
	BulkCreateGatewaysFn func(ctx context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error)
	StartFn              func(ctx context.Context, managementID uuid.UUID) error
	StopFn               func(ctx context.Context, managementID uuid.UUID) error
	DecommissionFn       func(ctx context.Context, managementID uuid.UUID) error
	ListGatewaysFn       func(ctx context.Context) ([]*domain.SimGateway, error)
	GetGatewayFn         func(ctx context.Context, managementID uuid.UUID) (*domain.SimGateway, error)
}

func (f *FakeGatewayLifecycleService) CreateAndStart(ctx context.Context, req domain.CreateGatewayRequest) (*domain.SimGateway, error) {
	if f.CreateAndStartFn != nil {
		return f.CreateAndStartFn(ctx, req)
	}
	return &domain.SimGateway{}, nil
}

func (f *FakeGatewayLifecycleService) BulkCreateGateways(ctx context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error) {
	if f.BulkCreateGatewaysFn != nil {
		return f.BulkCreateGatewaysFn(ctx, req)
	}
	return nil, nil
}

func (f *FakeGatewayLifecycleService) Start(ctx context.Context, managementID uuid.UUID) error {
	if f.StartFn != nil {
		return f.StartFn(ctx, managementID)
	}
	return nil
}

func (f *FakeGatewayLifecycleService) Stop(ctx context.Context, managementID uuid.UUID) error {
	if f.StopFn != nil {
		return f.StopFn(ctx, managementID)
	}
	return nil
}

func (f *FakeGatewayLifecycleService) Decommission(ctx context.Context, managementID uuid.UUID) error {
	if f.DecommissionFn != nil {
		return f.DecommissionFn(ctx, managementID)
	}
	return nil
}

func (f *FakeGatewayLifecycleService) ListGateways(ctx context.Context) ([]*domain.SimGateway, error) {
	if f.ListGatewaysFn != nil {
		return f.ListGatewaysFn(ctx)
	}
	return nil, nil
}

func (f *FakeGatewayLifecycleService) GetGateway(ctx context.Context, managementID uuid.UUID) (*domain.SimGateway, error) {
	if f.GetGatewayFn != nil {
		return f.GetGatewayFn(ctx, managementID)
	}
	return &domain.SimGateway{ManagementGatewayID: managementID}, nil
}

// FakeSensorManagementService implements ports.SensorManagementService.
type FakeSensorManagementService struct {
	AddSensorFn    func(ctx context.Context, gatewayID int64, sensor domain.SimSensor) (*domain.SimSensor, error)
	ListSensorsFn  func(ctx context.Context, gatewayID int64) ([]*domain.SimSensor, error)
	DeleteSensorFn func(ctx context.Context, sensorID int64) error
}

func (f *FakeSensorManagementService) AddSensor(ctx context.Context, gatewayID int64, sensor domain.SimSensor) (*domain.SimSensor, error) {
	if f.AddSensorFn != nil {
		return f.AddSensorFn(ctx, gatewayID, sensor)
	}
	return &sensor, nil
}

func (f *FakeSensorManagementService) ListSensors(ctx context.Context, gatewayID int64) ([]*domain.SimSensor, error) {
	if f.ListSensorsFn != nil {
		return f.ListSensorsFn(ctx, gatewayID)
	}
	return make([]*domain.SimSensor, 0), nil
}

func (f *FakeSensorManagementService) DeleteSensor(ctx context.Context, sensorID int64) error {
	if f.DeleteSensorFn != nil {
		return f.DeleteSensorFn(ctx, sensorID)
	}
	return nil
}

// FakeSimulatorControlService implements ports.SimulatorControlService.
type FakeSimulatorControlService struct {
	UpdateConfigFn         func(ctx context.Context, managementID uuid.UUID, update domain.GatewayConfigUpdate) error
	InjectGatewayAnomalyFn func(ctx context.Context, managementID uuid.UUID, cmd domain.GatewayAnomalyCommand) error
	InjectSensorOutlierFn  func(ctx context.Context, sensorID int64, value *float64) error
}

func (f *FakeSimulatorControlService) UpdateConfig(ctx context.Context, managementID uuid.UUID, update domain.GatewayConfigUpdate) error {
	if f.UpdateConfigFn != nil {
		return f.UpdateConfigFn(ctx, managementID, update)
	}
	return nil
}

func (f *FakeSimulatorControlService) InjectGatewayAnomaly(ctx context.Context, managementID uuid.UUID, cmd domain.GatewayAnomalyCommand) error {
	if f.InjectGatewayAnomalyFn != nil {
		return f.InjectGatewayAnomalyFn(ctx, managementID, cmd)
	}
	return nil
}

func (f *FakeSimulatorControlService) InjectSensorOutlier(ctx context.Context, sensorID int64, value *float64) error {
	if f.InjectSensorOutlierFn != nil {
		return f.InjectSensorOutlierFn(ctx, sensorID, value)
	}
	return nil
}

// FakeDecommissionEventReceiver implements ports.DecommissionEventReceiver.
type FakeDecommissionEventReceiver struct {
	mu    sync.Mutex
	Calls []struct{ TenantID, ManagementGatewayID string }
}

func (f *FakeDecommissionEventReceiver) HandleDecommission(tenantID string, managementGatewayID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, struct{ TenantID, ManagementGatewayID string }{TenantID: tenantID, ManagementGatewayID: managementGatewayID})
}
