package app

import (
	"context"
	"errors"
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

// Compile-time interface assertions.
var (
	_ ports.GatewayLifecycleService   = (*GatewayRegistry)(nil)
	_ ports.SensorManagementService   = (*GatewayRegistry)(nil)
	_ ports.SimulatorControlService   = (*GatewayRegistry)(nil)
	_ ports.DecommissionEventReceiver = (*GatewayRegistry)(nil)
)

type provisioningStage int

const (
	stageProvision provisioningStage = iota
	stageStore
	stageConnect
	errNotFoundFormat     = "%w: %s"
	errGetGatewayFormat   = "get gateway: %w"
	bulkCreateConcurrency = 10
)

type GatewayRegistry struct {
	workers     map[uuid.UUID]*GatewayWorker
	mu          sync.RWMutex
	store       ports.GatewayStore
	provisioner ports.Onboarder
	connector   ports.GatewayConnector
	encryptor   ports.Encryptor
	clock       ports.Nower
	cfg         *config.Config
	metrics     *metrics.Metrics
}

func NewGatewayRegistry(
	store ports.GatewayStore,
	provisioner ports.Onboarder,
	connector ports.GatewayConnector,
	encryptor ports.Encryptor,
	clock ports.Nower,
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

// GatewayLifecycleService methods.

func (r *GatewayRegistry) CreateAndStart(ctx context.Context, req domain.CreateGatewayRequest) (*domain.SimGateway, error) {
	sendFrequency := req.SendFrequencyMs
	if sendFrequency <= 0 {
		sendFrequency = r.cfg.DefaultSendFrequencyMs
	}

	// ManagementGatewayID and TenantID are assigned by the provisioning service and set inside runProvisioningSaga.
	gw := domain.SimGateway{
		FactoryID:       req.FactoryID,
		FactoryKey:      req.FactoryKey,
		Model:           req.Model,
		FirmwareVersion: req.FirmwareVersion,
		SendFrequencyMs: sendFrequency,
		Status:          domain.Provisioning,
		CreatedAt:       r.clock.Now(),
	}

	if err := r.runProvisioningSaga(ctx, &gw); err != nil {
		return nil, err
	}

	return &gw, nil
}

func (r *GatewayRegistry) BulkCreateGateways(ctx context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error) {
	results := make([]*domain.SimGateway, req.Count)
	errs := make([]error, req.Count)

	sem := make(chan struct{}, bulkCreateConcurrency)
	var wg sync.WaitGroup
	for i := 0; i < req.Count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			gw, err := r.CreateAndStart(ctx, domain.CreateGatewayRequest{
				FactoryID:       req.FactoryID,
				FactoryKey:      req.FactoryKey,
				Model:           req.Model,
				FirmwareVersion: req.FirmwareVersion,
				SendFrequencyMs: req.SendFrequencyMs,
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
		return fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementID)
	}

	if w.IsRunning() {
		return nil
	}

	w.Start(context.WithoutCancel(ctx))
	return r.store.UpdateStatus(ctx, w.gateway.ID, domain.Online)
}

func (r *GatewayRegistry) Stop(ctx context.Context, managementID uuid.UUID) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementID)
	}

	if w.commandPumpCancel != nil {
		w.commandPumpCancel()
	}
	w.Stop(workerStopTimeout)
	return r.store.UpdateStatus(ctx, w.gateway.ID, domain.Offline)
}

func (r *GatewayRegistry) Delete(ctx context.Context, managementID uuid.UUID) error {
	r.mu.Lock()
	w, ok := r.workers[managementID]
	if ok {
		delete(r.workers, managementID)
	}
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementID)
	}

	if w.commandPumpCancel != nil {
		w.commandPumpCancel()
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
	gw, err := r.store.GetGatewayByManagementID(ctx, managementID)
	if err != nil {
		if errors.Is(err, domain.ErrGatewayNotFound) {
			return nil, fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementID)
		}
		return nil, fmt.Errorf(errGetGatewayFormat, err)
	}
	return gw, nil
}

// SensorManagementService methods.

func (r *GatewayRegistry) AddSensor(ctx context.Context, managementGatewayID uuid.UUID, sensor domain.SimSensor) (*domain.SimSensor, error) {
	if sensor.MinRange >= sensor.MaxRange {
		return nil, fmt.Errorf("%w: minRange %.2f must be less than maxRange %.2f", domain.ErrInvalidSensorRange, sensor.MinRange, sensor.MaxRange)
	}

	gw, err := r.store.GetGatewayByManagementID(ctx, managementGatewayID)
	if err != nil {
		if errors.Is(err, domain.ErrGatewayNotFound) {
			return nil, fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementGatewayID)
		}
		return nil, fmt.Errorf(errGetGatewayFormat, err)
	}

	sensor.SensorID = uuid.New()
	sensor.GatewayID = gw.ID
	sensor.ManagementGatewayID = managementGatewayID
	sensor.CreatedAt = r.clock.Now()

	id, err := r.store.CreateSensor(ctx, sensor)
	if err != nil {
		return nil, fmt.Errorf("create sensor in store: %w", err)
	}
	sensor.ID = id

	r.mu.RLock()
	defer r.mu.RUnlock()
	if w, ok := r.workers[managementGatewayID]; ok {
		gen := generator.NewGeneratorFactory().New(&sensor, r.clock)
		if err := w.AttachSensor(&sensor, gen); err != nil {
			return nil, fmt.Errorf("failed to attach sensor to worker: %w", err)
		}
	}

	return &sensor, nil
}

func (r *GatewayRegistry) ListSensors(ctx context.Context, managementGatewayID uuid.UUID) ([]*domain.SimSensor, error) {
	gw, err := r.store.GetGatewayByManagementID(ctx, managementGatewayID)
	if err != nil {
		if errors.Is(err, domain.ErrGatewayNotFound) {
			return nil, fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementGatewayID)
		}
		return nil, fmt.Errorf(errGetGatewayFormat, err)
	}

	sensors, err := r.store.ListSensors(ctx, gw.ID)
	if err != nil {
		return nil, err
	}

	for _, s := range sensors {
		s.ManagementGatewayID = managementGatewayID
	}

	return sensors, nil
}

func (r *GatewayRegistry) DeleteSensor(ctx context.Context, sensorID uuid.UUID) error {
	sensor, err := r.store.GetSensorBySensorID(ctx, sensorID)
	if err != nil {
		return err
	}
	return r.store.DeleteSensor(ctx, sensor.ID)
}

// SimulatorControlService methods.

func (r *GatewayRegistry) UpdateConfig(ctx context.Context, managementID uuid.UUID, update domain.GatewayConfigUpdate) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementID)
	}

	select {
	case w.configCh <- update:
	default:
		return fmt.Errorf("config channel full for gateway %s", managementID)
	}
	return nil
}

func (r *GatewayRegistry) InjectGatewayAnomaly(ctx context.Context, managementID uuid.UUID, cmd domain.GatewayAnomalyCommand) error {
	r.mu.RLock()
	w, ok := r.workers[managementID]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf(errNotFoundFormat, domain.ErrGatewayNotFound, managementID)
	}

	select {
	case w.anomalyCh <- cmd:
	default:
		return fmt.Errorf("anomaly channel full for gateway %s", managementID)
	}
	return nil
}

// InjectSensorOutlier resolves the sensor from the store, finds its gateway worker, and enqueues the command.
// Store lookup is done here so the HTTP adapter does not need a store dependency.
func (r *GatewayRegistry) InjectSensorOutlier(ctx context.Context, sensorID uuid.UUID, value *float64) error {
	sensor, err := r.store.GetSensorBySensorID(ctx, sensorID)
	if err != nil {
		if errors.Is(err, domain.ErrSensorNotFound) {
			return fmt.Errorf("%w: sensor %s", domain.ErrSensorNotFound, sensorID)
		}
		return fmt.Errorf("get sensor: %w", err)
	}

	r.mu.RLock()
	var w *GatewayWorker
	for _, worker := range r.workers {
		if worker.gateway.ID == sensor.GatewayID {
			w = worker
			break
		}
	}
	r.mu.RUnlock()

	if w == nil {
		return fmt.Errorf("%w: gateway for sensor %s", domain.ErrGatewayNotFound, sensorID)
	}

	cmd := domain.SensorOutlierCommand{
		SensorID: sensor.SensorID,
		Value:    value,
	}

	select {
	case w.outlierCh <- cmd:
	default:
		return fmt.Errorf("outlier channel full for sensor %s", sensorID)
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

	pub, sub, closeNC, err := r.connector.Connect(ctx, gw.CertPEM, gw.PrivateKeyPEM, gw.TenantID, gw.ManagementGatewayID)
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}

	if _, err := r.startWorker(ctx, gw, sensors, pub, sub, closeNC); err != nil {
		_ = sub.Close()
		_ = closeNC()
		return err
	}

	return nil
}

func (r *GatewayRegistry) runProvisioningSaga(ctx context.Context, gw *domain.SimGateway) error {
	var (
		pub          ports.GatewayPublisher
		sub          ports.CommandSubscription
		reachedStage provisioningStage
	)

	// Stage 1: Provision — identity (gatewayId, tenantId) is assigned by the provisioning service.
	// Nothing is stored yet, so no compensation needed if this fails.
	result, err := r.provisioner.Onboard(ctx, gw.FactoryID, gw.FactoryKey, gw.SendFrequencyMs, gw.FirmwareVersion)
	if err != nil {
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("onboard: %w", err)
	}

	mgmtID, err := uuid.Parse(result.GatewayID)
	if err != nil {
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("parse gateway ID from provisioning: %w", err)
	}
	gw.ManagementGatewayID = mgmtID
	gw.TenantID = result.TenantID
	gw.CertPEM = result.CertPEM
	gw.PrivateKeyPEM = result.PrivateKeyPEM
	gw.EncryptionKey = result.AESKey
	gw.Provisioned = true

	// Stage 2: Persist with the full provisioned state.
	id, err := r.store.CreateGateway(ctx, *gw)
	if err != nil {
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("create gateway in store: %w", err)
	}
	gw.ID = id
	reachedStage = stageStore

	// Stage 3: Connect.
	var closeNC func() error
	pub, sub, closeNC, err = r.connector.Connect(ctx, gw.CertPEM, gw.PrivateKeyPEM, gw.TenantID, gw.ManagementGatewayID)
	if err != nil {
		r.compensate(ctx, gw, reachedStage, sub, nil)
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("connect: %w", err)
	}
	reachedStage = stageConnect

	// Stage 4: Start worker.
	sensors, err := r.store.ListSensors(ctx, gw.ID)
	if err != nil {
		r.compensate(ctx, gw, reachedStage, sub, closeNC)
		r.metrics.ProvisioningErrors.Inc()
		return fmt.Errorf("list sensors: %w", err)
	}

	if _, err := r.startWorker(ctx, gw, sensors, pub, sub, closeNC); err != nil {
		r.compensate(ctx, gw, reachedStage, sub, closeNC)
		r.metrics.ProvisioningErrors.Inc()
		return err
	}

	gw.Status = domain.Online
	r.metrics.ProvisioningSuccess.Inc()
	return nil
}

func (r *GatewayRegistry) compensate(ctx context.Context, gw *domain.SimGateway, failedAt provisioningStage, sub ports.CommandSubscription, closeNC func() error) {
	switch failedAt {
	case stageConnect:
		if sub != nil {
			_ = sub.Close()
		}
		if closeNC != nil {
			_ = closeNC()
		}
		fallthrough
	case stageStore:
		if err := r.store.DeleteGateway(ctx, gw.ID); err != nil {
			slog.Error("compensate: delete gateway failed", "err", err)
		}
	case stageProvision:
		// nothing persisted yet — nothing to roll back
	}
}

func (r *GatewayRegistry) startWorker(ctx context.Context, gw *domain.SimGateway, sensors []*domain.SimSensor, pub ports.GatewayPublisher, sub ports.CommandSubscription, closeNC func() error) (*GatewayWorker, error) {
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
		CloseNC:    closeNC,
		Encryptor:  r.encryptor,
		Clock:      r.clock,
		Buffer:     buf,
		Store:      r.store,
	}

	w := NewGatewayWorker(deps)
	workerCtx := context.WithoutCancel(ctx)
	w.Start(workerCtx)
	commandPumpCtx, cancelCommandPump := context.WithCancel(workerCtx)
	w.commandPumpCancel = cancelCommandPump

	go func() {
		defer func() { _ = sub.Close() }()
		messages := sub.Messages()
		for {
			select {
			case <-commandPumpCtx.Done():
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

	r.mu.Lock()
	r.workers[gw.ManagementGatewayID] = w
	r.mu.Unlock()

	r.metrics.GatewaysRunning.Inc()

	if err := r.store.UpdateStatus(ctx, gw.ID, domain.Online); err != nil {
		slog.Warn("failed to update gateway status to running", "gatewayID", gw.ManagementGatewayID, "err", err)
	}

	return w, nil
}

func (r *GatewayRegistry) HandleDecommission(tenantID, managementGatewayID string) {
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
	if w.gateway.TenantID != tenantID {
		r.mu.Unlock()
		slog.Warn("decommission event tenant mismatch, ignoring",
			"eventTenantID", tenantID,
			"gatewayTenantID", w.gateway.TenantID,
			"mgmID", mgmID,
		)
		return
	}
	delete(r.workers, mgmID)
	r.mu.Unlock()

	if w.commandPumpCancel != nil {
		w.commandPumpCancel()
	}
	w.Stop(10 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.store.DeleteGateway(ctx, w.gateway.ID); err != nil {
		slog.Error("Failed to delete decommissioned gateway from store", "err", err)
	}

	r.metrics.GatewaysRunning.Dec()
	slog.Info("Gateway decommissioned successfully via NATS event", "mgmID", mgmID.String())
}

func (r *GatewayRegistry) StopAll(timeout time.Duration) {
	r.mu.RLock()
	workersToStop := make([]*GatewayWorker, 0, len(r.workers))
	for _, w := range r.workers {
		workersToStop = append(workersToStop, w)
	}
	r.mu.RUnlock()

	var wg sync.WaitGroup
	for _, w := range workersToStop {
		wg.Add(1)
		go func(worker *GatewayWorker) {
			defer wg.Done()
			if worker.commandPumpCancel != nil {
				worker.commandPumpCancel()
			}
			worker.Stop(timeout)
		}(w)
	}
	wg.Wait()
}
