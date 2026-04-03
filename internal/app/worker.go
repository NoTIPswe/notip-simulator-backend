package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/generator"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

type activeAnomalyState struct {
	anomalyType   domain.AnomalyType
	expiresAt     time.Time
	packetLossPct float64
}

type innerSensorData struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

func getUnitForSensor(t domain.SensorType) string {
	switch t {
	case domain.Temperature:
		return "°C"
	case domain.Humidity:
		return "%"
	case domain.Pressure:
		return "hPa"
	case domain.Movement:
		return "m/s"
	case domain.Biometric:
		return "bpm"
	default:
		return ""
	}
}

type GatewayWorker struct {
	gateway    domain.SimGateway
	sensorMu   sync.RWMutex
	sensors    []*domain.SimSensor
	generators []generator.Generator
	buffer     *MessageBuffer
	publisher  ports.GatewayPublisher
	closeNC    func() error
	encryptor  ports.Encryptor
	clock      ports.Nower

	cancel          context.CancelFunc
	done            chan struct{}
	isRunning       atomic.Bool
	publisherClosed atomic.Bool

	store         ports.GatewayStore
	commandCh     chan domain.IncomingCommand
	anomalyCh     chan domain.GatewayAnomalyCommand
	outlierCh     chan domain.SensorOutlierCommand
	configCh      chan domain.GatewayConfigUpdate
	activeAnomaly *activeAnomalyState

	commandPumpCancel context.CancelFunc
}

type WorkerDeps struct {
	Gateway    domain.SimGateway
	Sensors    []*domain.SimSensor
	Generators []generator.Generator
	Publisher  ports.GatewayPublisher
	CloseNC    func() error
	Encryptor  ports.Encryptor
	Clock      ports.Nower
	Buffer     *MessageBuffer
	Store      ports.GatewayStore
}

func NewGatewayWorker(deps WorkerDeps) *GatewayWorker {
	return &GatewayWorker{
		gateway:    deps.Gateway,
		sensors:    deps.Sensors,
		generators: deps.Generators,
		publisher:  deps.Publisher,
		closeNC:    deps.CloseNC,
		encryptor:  deps.Encryptor,
		clock:      deps.Clock,
		buffer:     deps.Buffer,
		store:      deps.Store,
		done:       make(chan struct{}),
		anomalyCh:  make(chan domain.GatewayAnomalyCommand, 10),
		outlierCh:  make(chan domain.SensorOutlierCommand, 10),
		configCh:   make(chan domain.GatewayConfigUpdate, 10),
		commandCh:  make(chan domain.IncomingCommand, 10),
	}
}

func (w *GatewayWorker) AttachSensor(sensor *domain.SimSensor, gen generator.Generator) error {
	if sensor.MinRange >= sensor.MaxRange {
		return fmt.Errorf("%w: minRange %.2f must be less than maxRange %.2f", domain.ErrInvalidSensorRange, sensor.MinRange, sensor.MaxRange)
	}

	w.sensorMu.Lock()
	defer w.sensorMu.Unlock()
	w.sensors = append(w.sensors, sensor)
	w.generators = append(w.generators, gen)

	return nil
}

func (w *GatewayWorker) Start(ctx context.Context) {
	if w.isRunning.Swap(true) {
		return
	}
	w.done = make(chan struct{})
	ctx, w.cancel = context.WithCancel(ctx)

	go w.buffer.Flush(ctx)
	go w.sensorLoop(ctx)
}

func (w *GatewayWorker) Stop(timeout time.Duration) {
	if !w.isRunning.Swap(false) {
		return
	}
	w.cancel()

	select {
	case <-w.done:
	case <-time.After(timeout):
		slog.Warn("worker stop timeout", "gatewayID", w.gateway.ManagementGatewayID)
	}
	if w.publisherClosed.CompareAndSwap(false, true) {
		_ = w.publisher.Close()
		if w.closeNC != nil {
			_ = w.closeNC()
		}
	}
}

func (w *GatewayWorker) IsRunning() bool {
	return w.isRunning.Load()
}

func (w *GatewayWorker) sensorLoop(ctx context.Context) {
	defer close(w.done)

	var ticker *time.Ticker
	var tickC <-chan time.Time                 // nil = never fires in select
	var cfgC <-chan domain.GatewayConfigUpdate // non-nil only while freq=0

	if w.gateway.SendFrequencyMs > 0 {
		ticker = time.NewTicker(time.Duration(w.gateway.SendFrequencyMs) * time.Millisecond)
		tickC = ticker.C
	} else {
		cfgC = w.configCh // no ticker yet: consume config directly to start it
	}
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case cfg := <-cfgC:
			// Only reachable when freq=0. Start the ticker and hand config
			// drain back to drainControlChannels (preserves backpressure).
			if cfg.SendFrequencyMs != nil && *cfg.SendFrequencyMs > 0 {
				w.gateway.SendFrequencyMs = *cfg.SendFrequencyMs
				ticker = time.NewTicker(time.Duration(w.gateway.SendFrequencyMs) * time.Millisecond)
				tickC = ticker.C
				cfgC = nil
			}
		case cmd := <-w.commandCh:
			w.handleIncomingCommand(ctx, cmd)
		case <-tickC:
			w.processTick(ticker)
		}
	}
}

func (w *GatewayWorker) processTick(ticker *time.Ticker) {
	w.drainControlChannels(ticker)
	w.checkAnomalyExpiry()
	w.publishSensorData()
}

func (w *GatewayWorker) drainControlChannels(ticker *time.Ticker) {
	select {
	case cfg := <-w.configCh:
		if cfg.SendFrequencyMs != nil {
			if *cfg.SendFrequencyMs <= 0 {
				slog.Warn("ignoring invalid send frequency update", "gatewayID", w.gateway.ManagementGatewayID, "sendFrequencyMs", *cfg.SendFrequencyMs)
				break
			}
			w.gateway.SendFrequencyMs = *cfg.SendFrequencyMs
			ticker.Reset(time.Duration(w.gateway.SendFrequencyMs) * time.Millisecond)
		}
		if cfg.Status != nil {
			w.gateway.Status = *cfg.Status
		}
	default:
	}

	select {
	case cmd := <-w.anomalyCh:
		w.handleAnomalyCommand(cmd)
	default:
	}

drainOutliers:
	for {
		select {
		case cmd := <-w.outlierCh:
			w.sensorMu.RLock()
			for i, s := range w.sensors {
				if s.SensorID == cmd.SensorID {
					val := s.MaxRange * 2.0
					if cmd.Value != nil {
						val = *cmd.Value
					}
					w.generators[i].InjectOutlier(val)
					break
				}
			}
			w.sensorMu.RUnlock()

		default:
			break drainOutliers
		}
	}
}

func (w *GatewayWorker) checkAnomalyExpiry() {
	if w.activeAnomaly == nil {
		return
	}
	if w.activeAnomaly.expiresAt.IsZero() {
		return
	}

	if w.clock.Now().Before(w.activeAnomaly.expiresAt) {
		return
	}
	w.activeAnomaly = nil
}

func (w *GatewayWorker) publishSensorData() {
	if w.isCommunicationBlocked() {
		return
	}

	w.sensorMu.RLock()
	defer w.sensorMu.RUnlock()

	for i, sensor := range w.sensors {
		w.sendSensorTelemetry(sensor, w.generators[i])
	}
}

func (w *GatewayWorker) isCommunicationBlocked() bool {
	isDisconnected := w.activeAnomaly != nil && w.activeAnomaly.anomalyType == domain.Disconnect
	isNotOnline := w.gateway.Status == domain.Paused || w.gateway.Status == domain.Offline
	return isDisconnected || isNotOnline
}

func (w *GatewayWorker) sendSensorTelemetry(sensor *domain.SimSensor, gen generator.Generator) {
	value := gen.Next()
	innerData := innerSensorData{Value: value, Unit: getUnitForSensor(sensor.Type)}

	innerBytes, err := json.Marshal(innerData)
	if err != nil {
		slog.Error("failed to marshal sensor data", "sensorID", sensor.SensorID, "err", err)
		return
	}

	payload, err := w.encryptor.Encrypt(w.gateway.EncryptionKey, innerBytes)
	if err != nil {
		return
	}

	envelope := w.buildEnvelope(*sensor, payload)
	envBytes, err := json.Marshal(envelope)
	if err != nil {
		slog.Error("failed to marshal telemetry envelope", "sensorID", sensor.SensorID, "err", err)
		return
	}

	// Network Degradation.
	if w.activeAnomaly != nil && w.activeAnomaly.anomalyType == domain.NetworkDegradation {
		if rand.Float64() < w.activeAnomaly.packetLossPct {
			return
		}
	}

	w.buffer.Send(envBytes)
}

func (w *GatewayWorker) handleAnomalyCommand(cmd domain.GatewayAnomalyCommand) {
	state := &activeAnomalyState{anomalyType: cmd.Type}

	switch cmd.Type {
	case domain.NetworkDegradation:
		if cmd.NetworkDegradation != nil {
			state.expiresAt = w.clock.Now().Add(time.Duration(cmd.NetworkDegradation.DurationSeconds) * time.Second)
			loss := cmd.NetworkDegradation.PacketLossPct
			if loss > 1.0 {
				loss = loss / 100.0
			}
			state.packetLossPct = loss
		}

	case domain.Disconnect:
		if cmd.Disconnect != nil {
			state.expiresAt = w.clock.Now().Add(time.Duration(cmd.Disconnect.DurationSeconds) * time.Second)
		}
	}
	w.activeAnomaly = state
}

func (w *GatewayWorker) buildEnvelope(sensor domain.SimSensor, payload domain.EncryptedPayload) domain.TelemetryEnvelope {
	return domain.TelemetryEnvelope{
		GatewayID:     w.gateway.ManagementGatewayID.String(),
		SensorID:      sensor.SensorID.String(),
		SensorType:    string(sensor.Type),
		Timestamp:     w.clock.Now(),
		KeyVersion:    1,
		EncryptedData: payload.EncryptedData,
		IV:            payload.IV,
		AuthTag:       payload.AuthTag,
	}
}

func (w *GatewayWorker) handleIncomingCommand(ctx context.Context, cmd domain.IncomingCommand) {
	ackStatus, ackMessage := w.processIncomingCommand(ctx, cmd)
	w.sendACK(ctx, cmd.CommandID, ackStatus, ackMessage)
}

func (w *GatewayWorker) processIncomingCommand(ctx context.Context, cmd domain.IncomingCommand) (domain.CommandACKStatus, *string) {
	switch cmd.Type {
	case domain.ConfigUpdate:
		return w.handleConfigUpdateCommand(ctx, cmd.Payload)
	case domain.FirmwarePush:
		return w.handleFirmwarePushCommand(ctx, cmd.Payload)
	default:
		return domain.NACK, messagePtr("unknown command type")
	}
}

func (w *GatewayWorker) handleConfigUpdateCommand(ctx context.Context, payload []byte) (domain.CommandACKStatus, *string) {
	var p domain.CommandConfigPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return domain.NACK, messagePtr(fmt.Sprintf("invalid config payload: %v", err))
	}

	if p.SendFrequencyMs != nil {
		if *p.SendFrequencyMs <= 0 {
			return domain.NACK, messagePtr("sendFrequencyMs must be > 0")
		}
		if err := w.store.UpdateFrequency(ctx, w.gateway.ID, *p.SendFrequencyMs); err != nil {
			return domain.NACK, messagePtr(fmt.Sprintf("failed to update frequency in store: %v", err))
		}
	}

	if p.Status != nil {
		if err := w.store.UpdateStatus(ctx, w.gateway.ID, *p.Status); err != nil {
			return domain.NACK, messagePtr(fmt.Sprintf("failed to persist status: %v", err))
		}
	}

	update := domain.GatewayConfigUpdate(p)
	select {
	case w.configCh <- update:
		return domain.ACK, nil
	default:
		return domain.NACK, messagePtr("config channel full")
	}
}

func (w *GatewayWorker) handleFirmwarePushCommand(ctx context.Context, payload []byte) (domain.CommandACKStatus, *string) {
	var p domain.CommandFirmwarePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return domain.NACK, messagePtr(fmt.Sprintf("invalid firmware payload: %v", err))
	}

	if p.FirmwareVersion == "" {
		return domain.ACK, nil
	}

	if err := w.store.UpdateFirmwareVersion(ctx, w.gateway.ID, p.FirmwareVersion); err != nil {
		return domain.NACK, messagePtr(err.Error())
	}

	return domain.ACK, nil
}

func messagePtr(msg string) *string {
	return &msg
}

func (w *GatewayWorker) sendACK(ctx context.Context, commandID string, status domain.CommandACKStatus, message *string) {
	ack := domain.CommandACK{
		CommandID: commandID,
		Status:    status,
		Message:   message,
		Timestamp: w.clock.Now(),
	}

	payload, err := json.Marshal(ack)
	if err != nil {
		slog.Error("failed to marshal command ACK", "commandID", commandID, "err", err)
		return
	}

	subject := fmt.Sprintf("command.ack.%s.%s", w.gateway.TenantID, w.gateway.ManagementGatewayID.String())

	if err := w.publisher.Publish(ctx, subject, payload); err != nil {
		slog.Error("Failed to publish command ACK", "commandID", commandID, "err", err)
	}
}
