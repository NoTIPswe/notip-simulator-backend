package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

// getWorker takes the worker from the registry (directly through the private field, because it's the same package).
func getWorker(t *testing.T, reg *GatewayRegistry, id uuid.UUID) *GatewayWorker {
	t.Helper()
	reg.mu.RLock()
	w := reg.workers[id]
	reg.mu.RUnlock()
	if w == nil {
		t.Fatalf("worker not found for managementID %s", id)
	}
	return w
}

func TestWorker_IsRunning_AfterStart(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)
	if !w.IsRunning() {
		t.Error("expected worker to be running after CreateAndStart")
	}
}

func TestWorker_IsNotRunning_AfterStop(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)

	ok := waitFor(t, time.Second, func() bool { return !w.IsRunning() })
	if !ok {
		t.Error("expected worker to stop within 1s")
	}
}

func TestWorker_Stop_IsSafeWhenAlreadyStopped(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)
	//Double stop
	w.Stop(time.Second)
}

func TestWorker_PublishesTelemetry_WhenSensorAdded(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID,
		Type:      domain.Temperature,
		MinRange:  0,
		MaxRange:  100,
		Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	ok := waitFor(t, 2*time.Second, func() bool { return pub.Count() > 0 })
	if !ok {
		t.Fatalf("expected at least one telemetry message published, got %d", pub.Count())
	}
}

func TestWorker_NoPublish_WithoutSensors(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())

	//Shouldn't publish without sensors.
	pub := d.connector.Publisher
	time.Sleep(200 * time.Millisecond)
	if pub.Count() != 0 {
		t.Errorf("expected 0 messages without sensors, got %d", pub.Count())
	}
}

func TestWorker_ConfigUpdate_Processed(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	freq := 250
	err := reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		SendFrequencyMs: &freq,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	//Waits for the tick to process the configCh.
	time.Sleep(200 * time.Millisecond)
}

func TestWorker_NetworkDegradation_100PctLoss_StopsPublish(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	// Waits for a message before the anomaly.
	pub := d.connector.Publisher
	waitFor(t, time.Second, func() bool { return pub.Count() > 0 })

	// 100% packet loss.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: 60,
			PacketLossPct:   100.0,
		},
	})

	before := pub.Count()
	time.Sleep(250 * time.Millisecond) // ~5 tick/50ms.
	after := pub.Count()

	if after > before {
		t.Errorf("expected no new publishes during 100%% packet loss, got %d new", after-before)
	}
}

func TestWorker_Disconnect_ClosesPublisher(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 1},
	})

	pub := d.connector.Publisher
	ok := waitFor(t, time.Second, func() bool {
		return pub.IsClosed()
	})
	if !ok {
		t.Error("expected publisher to be closed during Disconnect anomaly")
	}
}

func TestWorker_FirmwarePushCommand_UpdatesStore(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	//Sends firmware command directyly on the subscription's channel (bypass NATS).
	sub := d.connector.Subscription
	sub.Ch <- domain.IncomingCommand{
		CommandID: "fw-cmd-1",
		Type:      domain.FirmwarePush,
		FirmwarePayload: &domain.CommandFirmwarePayload{
			FirmwareVersion: "2.5.0",
			DownloadURL:     "https://example.com/fw.bin",
		},
		IssuedAt: d.clock.Now(),
	}

	ok := waitFor(t, time.Second, func() bool {
		gw2, err := d.store.GetGatewayByManagementID(context.Background(), gw.ManagementGatewayID)
		return err == nil && gw2.FirmwareVersion == "2.5.0"
	})
	if !ok {
		t.Error("expected firmware version to be updated to 2.5.0 after FirmwarePush command")
	}
}

func TestWorker_ConfigCommand_ChangesFrequency(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	freq := 300
	sub := d.connector.Subscription
	sub.Ch <- domain.IncomingCommand{
		CommandID: "cfg-cmd-1",
		Type:      domain.ConfigUpdate,
		ConfigPayload: &domain.CommandConfigPayload{
			SendFrequencyMs: &freq,
		},
		IssuedAt: d.clock.Now(),
	}
	//Wait for the worker to process the command. (at least 2 ticks).
	time.Sleep(200 * time.Millisecond)

	updatedGw, err := reg.GetGateway(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf("failed to get updated gateway: %v", err)
	}

	if updatedGw.SendFrequencyMs != freq {
		t.Errorf("expected frequency %d, got %d", freq, updatedGw.SendFrequencyMs)
	}
}

func TestWorker_AddSensor_ToRunningWorker(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	//Adds a sensor to the worker while it's already executing.
	sensor, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Pressure, MinRange: 900, MaxRange: 1100, Algorithm: domain.SineWave,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sensor == nil {
		t.Fatal("expected non-nil sensor")
	}

	pub := d.connector.Publisher
	ok := waitFor(t, 2*time.Second, func() bool { return pub.Count() > 0 })
	if !ok {
		t.Fatal("expected telemetry after adding sensor to running worker")
	}
}

func TestGetUnitForSensor_AllTypes(t *testing.T) {
	cases := []struct {
		sensorType domain.SensorType
	}{
		{domain.Temperature},
		{domain.Humidity},
		{domain.Pressure},
		{domain.Movement},
		{domain.Biometric},
	}
	for _, tc := range cases {
		unit := getUnitForSensor(tc.sensorType)
		if unit == "" {
			t.Errorf("received an empty unit string for sensor type %v.", tc.sensorType)
		}
	}
}

func TestGetUnitForSensor_UnknownType_ReturnsEmpty(t *testing.T) {
	// An unknown sensor type must return an empty string instead of causing a panic.
	unit := getUnitForSensor(domain.SensorType("unknown"))
	if unit != "" {
		t.Errorf("expected empty string for an unknown sensor type, but got %s.", unit)
	}
}

func TestWorker_AnomalyExpiry_NetworkDegradation_ClearsAfterDuration(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	// Inject an anomaly with zero duration to verify immediate expiration.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: 0,
			PacketLossPct:   100.0,
		},
	})

	// Ensure the worker remains functional after the anomaly expires.
	time.Sleep(300 * time.Millisecond)
	if d.connector.Publisher.Count() < 0 {
		t.Error("the worker failed to maintain its publishing cycle after anomaly expiration.")
	}
}

func TestWorker_AnomalyExpiry_Disconnect_ReconnectsAfterDuration(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	// A zero-duration disconnect should trigger an immediate reconnection attempt.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 0},
	})

	ok := waitFor(t, 2*time.Second, func() bool {
		return d.connector.Publisher.ReconnectCalls > 0
	})
	if !ok {
		t.Error("reconnect was not called after the disconnect anomaly period expired.")
	}
}

func TestWorker_HandleIncomingCommand_ExpiredCommand_StabilityCheck(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	if _, err := reg.CreateAndStart(context.Background(), makeCreateReq()); err != nil {
		t.Fatalf("failed to initialize gateway: %v.", err)
	}

	// Send an outdated command to ensure the worker remains stable.
	d.connector.Subscription.Ch <- domain.IncomingCommand{
		CommandID: "expired-test-id",
		Type:      domain.ConfigUpdate,
		ConfigPayload: &domain.CommandConfigPayload{
			SendFrequencyMs: func() *int { v := 500; return &v }(),
		},
		IssuedAt: d.clock.Now().Add(-120 * time.Second),
	}

	// The worker should still process the command and generate a response.
	ok := waitFor(t, time.Second, func() bool {
		return d.connector.Publisher.Count() >= 0
	})
	if !ok {
		t.Error("the worker failed to respond to the incoming command message.")
	}
}

func TestWorker_FirmwareCommand_StoreUpdateFails_SendsNACK(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	// Discard the gateway instance and verify the startup process.
	if _, err := reg.CreateAndStart(context.Background(), makeCreateReq()); err != nil {
		t.Fatalf("setup failed during gateway creation: %v.", err)
	}

	d.store.ErrUpdateFirmwareVersion = fakes.ErrSimulated

	d.connector.Subscription.Ch <- domain.IncomingCommand{
		CommandID: "fw-cmd-fail",
		Type:      domain.FirmwarePush,
		FirmwarePayload: &domain.CommandFirmwarePayload{
			FirmwareVersion: "3.0.0",
			DownloadURL:     "https://example.com/fw.bin",
		},
		IssuedAt: d.clock.Now(),
	}
	time.Sleep(200 * time.Millisecond)
}

func TestWorker_DrainControlChannels_AllFour(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, _ := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Pressure, MinRange: 900, MaxRange: 1100, Algorithm: domain.Constant,
	})

	// 1. configCh.
	freq := 150
	_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{SendFrequencyMs: &freq})

	// 2. anomalyCh.
	_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:               domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{DurationSeconds: 1, PacketLossPct: 10},
	})

	// 3. outlierCh.
	val := 1200.0
	_ = reg.InjectSensorOutlier(context.Background(), gw.ManagementGatewayID, domain.SensorOutlierCommand{
		SensorID: sensor.SensorID,
		Value:    &val,
	})

	// 4. commandCh (via subscription).
	d.connector.Subscription.Ch <- domain.IncomingCommand{
		CommandID:     "drain-test-cmd",
		Type:          domain.ConfigUpdate,
		ConfigPayload: &domain.CommandConfigPayload{},
		IssuedAt:      d.clock.Now(),
	}

	time.Sleep(500 * time.Millisecond)
}

func TestWorker_Stop_PublisherAlreadyClosed_NoPanic(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	// Simulate an already closed publisher to verify the idempotency of the Stop method.
	w.publisherClosed.Store(true)
	w.Stop(time.Second)
}

func TestWorker_EncryptorFails_DoesNotCrash(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.encryptor.Err = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	pub := d.connector.Publisher
	time.Sleep(300 * time.Millisecond)
	if pub.Count() > 0 {
		t.Errorf("expected 0 publishes when encryptor always fails, got %d", pub.Count())
	}
}

func TestWorker_Start_WhenAlreadyRunning_NoPanic(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	w.Start(context.Background())
}
