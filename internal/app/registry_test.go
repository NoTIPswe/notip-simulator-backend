package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

const unexpected_error = "unexpected error: %v"

// testWriter redirects standard output to the Go testing framework.
type testWriter struct {
	t *testing.T
}

func (tw testWriter) Write(p []byte) (n int, err error) {
	// Use t.Log to format application logs as standard test output.
	tw.t.Log(string(bytes.TrimSpace(p)))
	return len(p), nil
}

// setupTestLogger configures slog to write to the test runner.
func setupTestLogger(t *testing.T) {
	t.Helper()
	originalLogger := slog.Default()

	// Create a new logger that writes to our testWriter.
	testLogger := slog.New(slog.NewTextHandler(testWriter{t: t}, nil))
	slog.SetDefault(testLogger)

	// Restore the original logger when the test completes.
	t.Cleanup(func() {
		slog.SetDefault(originalLogger)
	})
}

// waitFor polls cond every 10ms until timeout.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func provisionResult() domain.ProvisionResult {
	aesKey, _ := domain.NewEncryptionKey(make([]byte, 32))
	return domain.ProvisionResult{
		CertPEM:         []byte("fake-cert-pem"),
		PrivateKeyPEM:   []byte("fake-key-pem"),
		AESKey:          aesKey,
		GatewayID:       uuid.NewString(),
		TenantID:        "tenant1",
		SendFrequencyMs: 50,
	}
}

func makeCreateReq() domain.CreateGatewayRequest {
	return domain.CreateGatewayRequest{
		FactoryID:       "factory1",
		FactoryKey:      "fkey1",
		SerialNumber:    "SN001",
		Model:           "TestModel",
		FirmwareVersion: "1.0.0",
		SendFrequencyMs: 50,
	}
}

// CreateAndStart.
func TestCreateAndStart_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	if gw == nil {
		t.Fatal("expected non-nil gateway")
	}
	if gw.Status != domain.Online {
		t.Errorf("want status Online, got %v", gw.Status)
	}
	if gw.ManagementGatewayID == uuid.Nil {
		t.Error("expected non-nil ManagementGatewayID")
	}
	if !gw.Provisioned {
		t.Error("expected gateway to be marked as provisioned")
	}
}

func TestCreateAndStart_StoreCreateFails(t *testing.T) {
	d := newTestDeps()
	d.store.ErrCreateGateway = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when store.CreateGateway fails")
	}
}

func TestCreateAndStart_ProvisionerFails_RollsBack(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Err = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when provisioner fails")
	}
	//Compensate: Gateway has to be eliminated from the store.
	gws, _ := d.store.ListGateways(context.Background())
	if len(gws) != 0 {
		t.Errorf("expected 0 gateways after rollback, got %d", len(gws))
	}
}

func TestCreateAndStart_UpdateProvisionedStoreErrorIgnored(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrUpdateProvisioned = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	if gw == nil || gw.Status != domain.Online {
		t.Fatal("expected online gateway")
	}
}

func TestCreateAndStart_ConnectorFails_RollsBack(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.connector.Err = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when connector.Connect fails")
	}
	// The gateway should be deleted from the store.
	gws, _ := d.store.ListGateways(context.Background())
	if len(gws) != 0 {
		t.Errorf("expected 0 gateways after connector rollback, got %d", len(gws))
	}
}

//BulkCreateGateways.

func TestBulkCreate_AllSucceed(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gws, errs := reg.BulkCreateGateways(context.Background(), domain.BulkCreateRequest{
		Count:           3,
		FactoryID:       "fid",
		FactoryKey:      "fkey",
		Model:           "M1",
		FirmwareVersion: "1.0",
		SendFrequencyMs: 50,
	})

	for i, err := range errs {
		if err != nil {
			t.Errorf("unexpected error for gateway %d: %v", i, err)
		}
	}

	if len(gws) != 3 {
		t.Errorf("want 3 gateways, got %d", len(gws))
	}
}

func TestBulkCreate_AllFail(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Err = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, errs := reg.BulkCreateGateways(context.Background(), domain.BulkCreateRequest{
		Count: 2, FactoryID: "fid", FactoryKey: "fkey",
	})
	if len(errs) == 0 {
		t.Fatal("expected errors for all failed creations")
	}
}

//Stop.

func TestStop_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	err := reg.Stop(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
}

func TestStop_NotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Stop(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown gateway")
	}
}

//Delete.

func TestDelete_Success(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	err := reg.Delete(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	_, storeErr := d.store.GetGatewayByManagementID(context.Background(), gw.ManagementGatewayID)
	if storeErr == nil {
		t.Error("expected gateway to be deleted from store after decommission")
	}
}

func TestDelete_NotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Delete(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown gateway")
	}
}

//GetGateway/ListGateways.

func TestGetGateway_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	got, err := reg.GetGateway(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	if got.ManagementGatewayID != gw.ManagementGatewayID {
		t.Error("returned wrong gateway")
	}
}

func TestGetGateway_NotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	_, err := reg.GetGateway(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown gateway")
	}
}

func TestListGateways_Empty(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	gws, err := reg.ListGateways(context.Background())
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	if len(gws) != 0 {
		t.Errorf("want 0 gateways, got %d", len(gws))
	}
}

func TestListGateways_Multiple(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())

	gws, err := reg.ListGateways(context.Background())
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	if len(gws) != 2 {
		t.Errorf("want 2 gateways, got %d", len(gws))
	}
}

func TestListGateways_StoreError(t *testing.T) {
	d := newTestDeps()
	d.store.ErrListGateways = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.ListGateways(context.Background())
	if err == nil {
		t.Fatal("expected error when store.ListGateways fails")
	}
}

//AddSensor/ListSensors/DeleteSensor.

func TestAddSensor_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID,
		Type:      domain.Temperature,
		MinRange:  0,
		MaxRange:  100,
		Algorithm: domain.UniformRandom,
	})
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	if sensor == nil {
		t.Fatal("expected non-nil sensor")
	}
	if sensor.SensorID == uuid.Nil {
		t.Error("expected SensorID to be generated (non-nil UUID)")
	}
}

func TestAddSensor_StoreError(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrCreateSensor = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Humidity, Algorithm: domain.Constant,
	})
	if err == nil {
		t.Fatal("expected error when store.CreateSensor fails")
	}
}

func TestListSensors_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, err1 := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, Algorithm: domain.UniformRandom,
		MinRange: 0, MaxRange: 50,
	})
	if err1 != nil {
		t.Fatalf("unexpected error adding sensor 1: %v", err1)
	}

	_, err2 := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Humidity, Algorithm: domain.Constant,
		MinRange: 10, MaxRange: 90,
	})
	if err2 != nil {
		t.Fatalf("unexpected error adding sensor 2: %v", err2)
	}

	sensors, err := reg.ListSensors(context.Background(), gw.ID)
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	if len(sensors) != 2 {
		t.Errorf("want 2 sensors, got %d", len(sensors))
	}
}

func TestDeleteSensor_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID,
		Type:      domain.Temperature,
		Algorithm: domain.UniformRandom,
		MinRange:  0,
		MaxRange:  100,
	})

	if err != nil {
		t.Fatalf("unexpected error adding sensor: %v", err)
	}

	err = reg.DeleteSensor(context.Background(), sensor.ID)
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
}

func TestDeleteSensor_StoreError(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrDeleteSensor = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID,
		Type:      domain.Temperature,
		Algorithm: domain.UniformRandom,
		MinRange:  0,
		MaxRange:  100,
	})

	if err != nil {
		t.Fatalf("unexpected error adding sensor: %v", err)
	}

	err = reg.DeleteSensor(context.Background(), sensor.ID)
	if err == nil {
		t.Fatal("expected error when store.DeleteSensor fails")
	}
}

//UpdateConfig.

func TestUpdateConfig_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	freq := 200
	err := reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{
		SendFrequencyMs: &freq,
	})
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
}

func TestUpdateConfig_WorkerNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	freq := 200
	err := reg.UpdateConfig(context.Background(), uuid.New(), domain.GatewayConfigUpdate{
		SendFrequencyMs: &freq,
	})
	if err == nil {
		t.Fatal("expected error for unknown worker")
	}
}

// InjectGatewayAnomaly.
func TestInjectAnomaly_NetworkDegradation(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	err := reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: 5,
			PacketLossPct:   50.0,
		},
	})
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
}

func TestInjectAnomaly_Disconnect(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	err := reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 1},
	})
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
}

func TestInjectAnomaly_WorkerNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.InjectGatewayAnomaly(context.Background(), uuid.New(), domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
	})
	if err == nil {
		t.Fatal("expected error for unknown worker")
	}
}

// InjectSensorOutlier.
func TestInjectSensorOutlier_Success(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, _ := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	val := 9999.9
	err := reg.InjectSensorOutlier(context.Background(), sensor.ID, &val)
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
}

func TestInjectSensorOutlier_SensorNotInWorker(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())
	err := reg.InjectSensorOutlier(context.Background(), 99999, nil) // non-existing sensor ID.
	if err == nil {
		t.Fatal("expected error for sensor not in worker")
	}
}

func TestInjectSensorOutlier_WorkerNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.InjectSensorOutlier(context.Background(), 99999, nil) // non-existing sensor ID.
	if err == nil {
		t.Fatal("expected error for unknown worker")
	}
}

//HandleDecommission

func TestHandleDecommission_Success(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	reg.HandleDecommission("tenant1", gw.ManagementGatewayID.String())

	reg.mu.RLock()
	_, exists := reg.workers[gw.ManagementGatewayID]
	reg.mu.RUnlock()
	if exists {
		t.Error("expected worker to be removed from map after HandleDecommission")
	}
}

func TestHandleDecommission_InvalidUUID_NoPanic(t *testing.T) {
	setupTestLogger(t)
	reg := newTestRegistry(newTestDeps())
	reg.HandleDecommission("tenant1", "not-a-valid-uuid")
}

func TestHandleDecommission_UnknownGateway_NoPanic(t *testing.T) {
	setupTestLogger(t)
	reg := newTestRegistry(newTestDeps())
	reg.HandleDecommission("tenant1", uuid.New().String())
}

// StopAll.
func TestStopAll_MultipleRunning(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())

	done := make(chan struct{})
	go func() {
		reg.StopAll(2 * time.Second)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("StopAll did not complete within timeout")
	}
}

func TestStopAll_Empty_NoPanic(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	reg.StopAll(time.Second)
}

//RestoreAll.

func TestRestoreAll_NoGateways(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.RestoreAll(context.Background())
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
}

func TestRestoreAll_StoreListError(t *testing.T) {
	d := newTestDeps()
	d.store.ErrListGateways = fakes.ErrSimulated
	reg := newTestRegistry(d)

	err := reg.RestoreAll(context.Background())
	if err == nil {
		t.Fatal("expected error when store.ListGateways fails")
	}
}

func TestRestoreAll_WithProvisionedGateway(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	// Create and stops a gateway to simulate a previous state.
	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)
	// Remove from the map of the worker to simulate the reboot of the service.
	reg.mu.Lock()
	delete(reg.workers, gw.ManagementGatewayID)
	reg.mu.Unlock()

	err := reg.RestoreAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error during RestoreAll: %v", err)
	}
	defer reg.StopAll(time.Second)
}

func TestRegistryStart_StoppedGateway_Restarts(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)

	// Restarts the stopped gateway.
	err := reg.Start(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf("unexpected error on Start: %v", err)
	}
	w := getWorker(t, reg, gw.ManagementGatewayID)
	ok := waitFor(t, time.Second, func() bool { return w.IsRunning() })
	if !ok {
		t.Error("expected worker to be running after Start")
	}
}

func TestRegistryStart_NotFound_ReturnsError(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Start(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown managementID")
	}
}

func TestRegistryStart_AlreadyRunning_IsIdempotent(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	//Start on a running worker shouuld not crash.
	err := reg.Start(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf("Start on already-running worker returned error: %v", err)
	}
}

func TestCompensate_ConnectorFails_PubAndSubClosed(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.connector.Err = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when connector fails")
	}
	//Gateway must be removed from the store.
	gws, _ := d.store.ListGateways(context.Background())
	if len(gws) != 0 {
		t.Errorf("expected 0 gateways after compensate(stageConnect), got %d", len(gws))
	}
}

func TestCompensate_StoreUpdateProvisionedErrorIgnored(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrUpdateProvisioned = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err != nil {
		t.Fatalf(unexpected_error, err)
	}
	gws, _ := d.store.ListGateways(context.Background())
	if len(gws) != 1 {
		t.Errorf("expected 1 gateway, got %d", len(gws))
	}
	if gw.Status != domain.Online {
		t.Errorf("expected Online status, got %v", gw.Status)
	}
}

func TestDelete_StoreDeleteFails_ReturnsError(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	d.store.ErrDeleteGateway = fakes.ErrSimulated

	err := reg.Delete(context.Background(), gw.ManagementGatewayID)
	if err == nil {
		t.Fatal("expected error when store.DeleteGateway fails")
	}
}

func TestUpdateConfig_ChannelFull_DoesNotBlock(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	freq := 100
	// Fill the internal channel buffer capacity of 10.
	for range 100 {
		_ = reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{SendFrequencyMs: &freq})
	}

	// buffer should be full, and the next call must return an error.
	err := reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{SendFrequencyMs: &freq})
	if err == nil {
		t.Errorf("expected a channel full error, but the operation was accepted.")
	}
}

func TestInjectAnomaly_ChannelFull_DoesNotBlock(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	cmd := domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 60},
	}

	// Occupy all 10 available slots in the command channel.
	for range 100 {
		_ = reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, cmd)
	}

	// Verify that the call completes successfully instead of blocking the execution.
	err := reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, cmd)
	if err == nil {
		t.Errorf("the operation should have returned a buffer full error.")
	}
}

func TestRestoreAll_ConnectorFails_ContinuesOtherGateways(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	//Create a provisioned gateway.
	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)
	reg.mu.Lock()
	delete(reg.workers, gw.ManagementGatewayID)
	reg.mu.Unlock()
	//Inject error on the connector, restoreGateway has to fail, but shouldn't crash
	d.connector.Err = fakes.ErrSimulated
	err := reg.RestoreAll(context.Background())

	if err != nil {
		t.Errorf("RestoreAll should handle individual failures without returning an error: %v.", err)
	}

}

func TestRestoreAll_ListSensorsFails_HandledGracefully(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)
	reg.mu.Lock()
	delete(reg.workers, gw.ManagementGatewayID)
	reg.mu.Unlock()

	d.store.ErrListSensors = fakes.ErrSimulated
	err := reg.RestoreAll(context.Background())

	if err != nil {
		t.Errorf("expected no global error during restoration failure: %v.", err)
	}
}

func TestInjectSensorOutlier_NilValue_UsesDefault(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, _ := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.Spike,
	})

	// When Value is nil, the generator logic should apply its default spike behavior.
	err := reg.InjectSensorOutlier(context.Background(), sensor.ID, nil)

	if err != nil {
		t.Fatalf("unexpected failure when injecting outlier with nil value: %v.", err)
	}
}

func TestAddSensor_InvalidRange_ReturnsError(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	_, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID,
		Type:      domain.Temperature,
		MinRange:  100,
		MaxRange:  100,
		Algorithm: domain.UniformRandom,
	})

	if err == nil {
		t.Fatal("expected error for invalid sensor range (MinRange >= MaxRange)")
	}
	if !errors.Is(err, domain.ErrInvalidSensorRange) {
		t.Errorf("expected ErrInvalidSensorRange, got %v", err)
	}
}

func TestHandleDecommission_TenantMismatch_Ignored(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())

	// Wrong tenantID — should be ignored, worker must still be running.
	reg.HandleDecommission("wrong-tenant", gw.ManagementGatewayID.String())

	w := getWorker(t, reg, gw.ManagementGatewayID)
	if !w.IsRunning() {
		t.Error("worker should still be running after tenant mismatch decommission event")
	}
}

func TestDelete_CancelsCommandPump(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)

	_ = reg.Delete(context.Background(), gw.ManagementGatewayID)

	// commandPumpCancel should have been called — worker must not be running.
	ok := waitFor(t, time.Second, func() bool { return !w.IsRunning() })
	if !ok {
		t.Error("expected worker to stop after Delete")
	}
}

func TestStop_NotFound_ReturnsErrGatewayNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Stop(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrGatewayNotFound) {
		t.Errorf("expected ErrGatewayNotFound, got %v", err)
	}
}

func TestDelete_NotFound_ReturnsErrGatewayNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Delete(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrGatewayNotFound) {
		t.Errorf("expected ErrGatewayNotFound, got %v", err)
	}
}
