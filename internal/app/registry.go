package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/config"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/generator"
	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

type provisioningStage int

const (
	stageProvision provisioningStage = iota
	stageStore
	stageConnect
	stageStart
)

const errGatewayNotFound = "gateway %s not found"

type GatewayRegistry struct {
	workers     map[uuid.UUID]*GatewayWorker
	mu          sync.RWMutex
	store       ports.GatewayStore
	provisioner ports.ProvisioningClient
	connector   ports.GatewayConnector
	encryptor   ports.Encryptor
	clock       ports.Clock
	cfg         *config.Config
	metrics     *metrics.Metrics
}

func NewGatewayRegistry(
	store ports.GatewayStore,
	provisioner ports.ProvisioningClient,
	connector ports.GatewayConnector,
	encryptor ports.Encryptor,
	clock ports.Clock,
	cfg *config.Config,
	met *metrics.Metrics,
) *GatewayRegistry {
	return &GatewayRegistry{
		workers:     make(map[uuid.UUID]*GatewayWorker),
		store:       store,
		provisioner: provisioner,
		connector:   connector,
		encryptor:   encryptor,
		clock:       clock,
		cfg:         cfg,
		metrics:     met,
	}
}

// GatewaLifecycleService methods.

func (r *GatewayRegistry) CreateAndStart(ctx context.Context, req domain.CreateGatewayRequest) (*domain.SimGateway, error) {
	managementID := uuid.New()

	gw := domain.SimGateway{
		ManagementGatewayID: managementID,
		FactoryID:           req.FactoryID,
		FactoryKey:          req.FactoryKey,
		SerialNumber:        req.SerialNumber,
		Model:               req.Model,
		FirmwareVersion:     req.FirmwareVersion,
		SendFrequencyMs:     req.SendFrequencyMs,
		Status:              domain.Provisioning,
		TenantID:            req.TenantID,
		CreatedAt:           r.clock.Now(),
	}

	id, err := r.store.CreateGateway(ctx, gw)
	if err != nil {
		return nil, fmt.Errorf("create gateway in store: %w", err)
	}
	gw.ID = id

	if err := r.runProvisioningSaga(ctx, &gw); err != nil {
		return nil, err
	}

	return &gw, nil
}

func (r *GatewayRegistry) BulkCreateGateways(ctx context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error) {
	results := make([]*domain.SimGateway, req.Count)
	errs := make([]error, req.Count)

	var wg sync.WaitGroup
	for i := 0; i < req.Count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			gw, err := r.CreateAndStart(ctx, domain.CreateGatewayRequest{
				Name:            fmt.Sprintf("SIM-%d", idx),
				TenantID:        req.TenantID,
				FactoryID:       req.FactoryID,
				FactoryKey:      req.FactoryKey,
				SerialNumber:    fmt.Sprintf("SN-SIM-%d", idx),
				Model:           req.Model,
				FirmwareVersion: req.FirmwareVersion,
				SendFrequencyMs: r.cfg.DefaultSendFrequencyMs,
			})
			results[idx] = gw
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	return results, errs
}

func (r *GatewayRegistry) Start(ctx context.Context, managementID uuid.UUID) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errGatewayNotFound, managementID)
	}

	// If the worker is already running, we return nil to ensure idempotency.
	if w.IsRunning() {
		return nil
	}

	w.Start(ctx)
	return r.store.UpdateStatus(ctx, w.gateway.ID, domain.Running)
}

func (r *GatewayRegistry) Stop(ctx context.Context, managementID uuid.UUID) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errGatewayNotFound, managementID)
	}

	w.Stop(workerStopTimeout)
	return r.store.UpdateStatus(ctx, w.gateway.ID, domain.Stopped)
}

func (r *GatewayRegistry) Decommission(ctx context.Context, managementID uuid.UUID) error {
	r.mu.Lock()
	w, ok := r.workers[managementID]
	if ok {
		delete(r.workers, managementID)
	}
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf(errGatewayNotFound, managementID)
	}

	w.Stop(workerStopTimeout)

	if err := r.store.DeleteGateway(ctx, w.gateway.ID); err != nil {
		return fmt.Errorf("delete gateway from store: %w", err)
	}

	r.metrics.GatewaysRunning.Dec()
	return nil
}

func (r *GatewayRegistry) ListGateways(ctx context.Context) ([]*domain.SimGateway, error) {
	return r.store.ListGateways(ctx)
}

func (r *GatewayRegistry) GetGateway(ctx context.Context, managementID uuid.UUID) (*domain.SimGateway, error) {
	return r.store.GetGatewayByManagementID(ctx, managementID)
}

// --- SensorManagementService.

func (r *GatewayRegistry) AddSensor(ctx context.Context, gatewayID int64, sensor domain.SimSensor) (*domain.SimSensor, error) {
	sensor.SensorID = uuid.New()
	sensor.GatewayID = gatewayID
	sensor.CreatedAt = r.clock.Now()

	id, err := r.store.CreateSensor(ctx, sensor)
	if err != nil {
		return nil, fmt.Errorf("create sensor in store: %w", err)
	}
	sensor.ID = id

	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, w := range r.workers {
		if w.gateway.ID == gatewayID {
			gen := generator.NewGeneratorFactory().New(&sensor, r.clock)
			w.AddSensor(&sensor, gen)
			break
		}
	}

	return &sensor, nil
}

func (r *GatewayRegistry) ListSensors(ctx context.Context, gatewayID int64) ([]*domain.SimSensor, error) {
	return r.store.ListSensors(ctx, gatewayID)
}

func (r *GatewayRegistry) DeleteSensor(ctx context.Context, sensorID int64) error {
	return r.store.DeleteSensor(ctx, sensorID)
}

// SimulatorControlService.

func (r *GatewayRegistry) UpdateConfig(ctx context.Context, managementID uuid.UUID, update domain.GatewayConfigUpdate) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errGatewayNotFound, managementID)
	}

	select {
	case w.configCh <- update:
	default:
		return fmt.Errorf("config channel full for gateway %s ", managementID)
	}
	return nil
}

func (r *GatewayRegistry) InjectGatewayAnomaly(ctx context.Context, managementID uuid.UUID, cmd domain.GatewayAnomalyCommand) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errGatewayNotFound, managementID)
	}

	select {
	case w.anomalyCh <- cmd:
	default:
		return fmt.Errorf("anomaly channel full for gateway %s ", managementID)
	}
	return nil
}

func (r *GatewayRegistry) InjectSensorOutlier(ctx context.Context, managementID uuid.UUID, cmd domain.SensorOutlierCommand) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errGatewayNotFound, managementID)
	}

	found := false
	for _, s := range w.sensors {
		if s.SensorID == cmd.SensorID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("sensor %s not found on gateway %s", cmd.SensorID, managementID)
	}

	select {
	case w.outlierCh <- cmd:
	default:
		return fmt.Errorf("outlier channel full for gateway %s ", managementID)
	}
	return nil
}

// Internal methods.

func (r *GatewayRegistry) RestoreAll(ctx context.Context) error {
	gateways, err := r.store.ListGateways(ctx)
	if err != nil {
		return fmt.Errorf("list gateways from store: %w", err)
	}

	for _, gw := range gateways {
		if !gw.Provisioned {
			continue
		}
		if err := r.restoreGateway(ctx, gw); err != nil {
			slog.Error("failed to restore gateway", "gatewayID", gw.ManagementGatewayID, "err", err)
		}
	}

	return nil
}

func (r *GatewayRegistry) restoreGateway(ctx context.Context, gw *domain.SimGateway) error {
	sensors, err := r.store.ListSensors(ctx, gw.ID)
	if err != nil {
		return fmt.Errorf("list sensors for gateway %d: %w", gw.ID, err)
	}

	pub, sub, err := r.connector.Connect(ctx, gw.CertPEM, gw.PrivateKeyPEM, gw.TenantID, gw.ManagementGatewayID)
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}

	if _, err := r.startWorker(ctx, gw, sensors, pub, sub); err != nil {
		_ = pub.Close()
		_ = sub.Close()
		return err
	}

	return nil
}

// Private methods.

func (r *GatewayRegistry) runProvisioningSaga(ctx context.Context, gw *domain.SimGateway) error {
	var (
		pub          ports.GatewayPublisher
		sub          ports.CommandSubscription
		reachedStage provisioningStage
	)

	//StageProvision.
	result, err := r.provisioner.Onboard(ctx, gw.FactoryID, gw.FactoryKey, gw.TenantID, gw.ManagementGatewayID)
	if err != nil {
		r.compensate(ctx, gw, stageProvision, pub, sub)
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("onboard: %w", err)
	}
	gw.CertPEM = result.CertPEM
	gw.PrivateKeyPEM = result.PrivateKeyPEM
	gw.EncryptionKey = result.AESKey
	reachedStage = stageProvision

	//StageStore.
	if err := r.store.UpdateProvisioned(ctx, gw.ID, result); err != nil {
		r.compensate(ctx, gw, reachedStage, pub, sub)
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("update provisioned gateway: %w", err)
	}
	gw.Provisioned = true
	reachedStage = stageStore

	//StageConnect.
	pub, sub, err = r.connector.Connect(ctx, gw.CertPEM, gw.PrivateKeyPEM, gw.TenantID, gw.ManagementGatewayID)
	if err != nil {
		r.compensate(ctx, gw, reachedStage, pub, sub)
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("connect: %w", err)
	}
	reachedStage = stageConnect

	//StageStart.
	sensors, err := r.store.ListSensors(ctx, gw.ID)
	if err != nil {
		r.compensate(ctx, gw, reachedStage, pub, sub)
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("list sensors: %w", err)
	}

	if _, err := r.startWorker(ctx, gw, sensors, pub, sub); err != nil {
		r.compensate(ctx, gw, reachedStage, pub, sub)
		r.metrics.ProvisioningErrors.Inc()
		return err
	}

	gw.Status = domain.Running

	r.metrics.ProvisioningSuccess.Inc()
	return nil
}

func (r *GatewayRegistry) compensate(ctx context.Context, gw *domain.SimGateway, failedAt provisioningStage, pub ports.GatewayPublisher, sub ports.CommandSubscription) {
	switch failedAt {
	case stageStart, stageConnect:
		if pub != nil {
			_ = pub.Close()
		}
		if sub != nil {
			_ = sub.Close()
		}
		fallthrough
	case stageStore, stageProvision:
		if err := r.store.DeleteGateway(ctx, gw.ID); err != nil {
			slog.Error("compensate: delete gateway failed", "err", err)
		}
	}
}

func (r *GatewayRegistry) startWorker(ctx context.Context, gw *domain.SimGateway, sensors []*domain.SimSensor, pub ports.GatewayPublisher, sub ports.CommandSubscription) (*GatewayWorker, error) {
	gens := make([]generator.Generator, len(sensors))
	factory := generator.NewGeneratorFactory()
	for i, s := range sensors {
		gens[i] = factory.New(s, r.clock)
	}

	subject := fmt.Sprintf("telemetry.data.%s.%s", gw.TenantID, gw.ManagementGatewayID.String())
	buf := NewMessageBuffer(r.cfg.GatewayBufferSize, subject, gw.ManagementGatewayID.String(), pub, r.metrics)

	deps := WorkerDeps{
		Gateway:    *gw,
		Sensors:    sensors,
		Generators: gens,
		Publisher:  pub,
		Encryptor:  r.encryptor,
		Clock:      r.clock,
		Buffer:     buf,
		Store:      r.store,
	}

	w := NewGatewayWorker(deps)
	w.Start(ctx)

	go func() {
		defer func() { _ = sub.Close() }()
		messages := sub.Messages()
		for {
			select {
			case <-w.done:
				return
			case cmd, ok := <-messages:
				if !ok {
					return
				}
				select {
				case w.commandCh <- cmd:
				default:
					slog.Warn("Worker command channel full, dropping command", "gwID", gw.ManagementGatewayID.String())
				}
			}
		}
	}()

	//Register the worker.
	r.mu.Lock()
	r.workers[gw.ManagementGatewayID] = w
	r.mu.Unlock()

	r.metrics.GatewaysRunning.Inc()

	if err := r.store.UpdateStatus(ctx, gw.ID, domain.Running); err != nil {
		slog.Warn("failed to update gateway status to running", "gatewayID", gw.ManagementGatewayID, "err", err)
	}

	return w, nil

}

func (r *GatewayRegistry) HandleDecommission(tenantID string, managementGatewayID string) {
	mgmID, err := uuid.Parse(managementGatewayID)
	if err != nil {
		slog.Error("Invalid UUID in decommission event", "id", managementGatewayID)
		return
	}

	r.mu.Lock()
	w, ok := r.workers[mgmID]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.workers, mgmID)
	r.mu.Unlock()

	// Stop the worker.
	w.Stop(10 * time.Second)

	ctx := context.Background()
	if err := r.store.DeleteGateway(ctx, w.gateway.ID); err != nil {
		slog.Error("Failed to delete decommissioned gateway from store", "err", err)
	}

	r.metrics.GatewaysRunning.Dec()
	slog.Info("Gateway decommissioned successfully via NATS event", "mgmID", mgmID.String())

}

func (r *GatewayRegistry) StopAll(timeout time.Duration) {
	//Read the workers.
	r.mu.RLock()
	workersToStop := make([]*GatewayWorker, 0, len(r.workers))
	for _, w := range r.workers {
		workersToStop = append(workersToStop, w)
	}
	r.mu.RUnlock()

	//Launch the stop procedures.
	var wg sync.WaitGroup
	for _, w := range workersToStop {
		wg.Add(1)
		go func(worker *GatewayWorker) {
			defer wg.Done()
			worker.Stop(timeout)
		}(w)
	}
	wg.Wait()
}
