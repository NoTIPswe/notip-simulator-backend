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

const (
	unexpectedErrorMsg           = "unexpected error: %v"
	msgExpectedErrUnknownGateway = "expected error for unknown gateway"
	msgExpectedErrUnknownWorker  = "expected error for unknown worker"
)

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
func TestCreateAndStartSuccess(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
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

func TestCreateAndStartStoreCreateFails(t *testing.T) {
	d := newTestDeps()
	d.store.ErrCreateGateway = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when store.CreateGateway fails")
	}
}

func TestCreateAndStartProvisionerFailsRollsBack(t *testing.T) {
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

func TestCreateAndStartUpdateProvisionedStoreErrorIgnored(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrUpdateProvisioned = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	if gw == nil || gw.Status != domain.Online {
		t.Fatal("expected online gateway")
	}
}

func TestCreateAndStartConnectorFailsRollsBack(t *testing.T) {
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

func TestBulkCreateAllSucceed(t *testing.T) {
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

func TestBulkCreateAllFail(t *testing.T) {
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

func TestStopSuccess(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	err := reg.Stop(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
}

//Delete.

func TestDeleteSuccess(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	err := reg.Delete(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	_, storeErr := d.store.GetGatewayByManagementID(context.Background(), gw.ManagementGatewayID)
	if storeErr == nil {
		t.Error("expected gateway to be deleted from store after decommission")
	}
}

//GetGateway/ListGateways.

func TestGetGatewaySuccess(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	got, err := reg.GetGateway(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	if got.ManagementGatewayID != gw.ManagementGatewayID {
		t.Error("returned wrong gateway")
	}
}

func TestGetGatewayNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	_, err := reg.GetGateway(context.Background(), uuid.New())
	if err == nil {
		t.Fatal(msgExpectedErrUnknownGateway)
	}
}

func TestListGatewaysEmpty(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	gws, err := reg.ListGateways(context.Background())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	if len(gws) != 0 {
		t.Errorf("want 0 gateways, got %d", len(gws))
	}
}

func TestListGatewaysMultiple(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())
	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())

	gws, err := reg.ListGateways(context.Background())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	if len(gws) != 2 {
		t.Errorf("want 2 gateways, got %d", len(gws))
	}
}

func TestListGatewaysStoreError(t *testing.T) {
	d := newTestDeps()
	d.store.ErrListGateways = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.ListGateways(context.Background())
	if err == nil {
		t.Fatal("expected error when store.ListGateways fails")
	}
}

//AddSensor/ListSensors/DeleteSensor.

func TestAddSensorSuccess(t *testing.T) {
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
		t.Fatalf(unexpectedErrorMsg, err)
	}
	if sensor == nil {
		t.Fatal("expected non-nil sensor")
	}
	if sensor.SensorID == uuid.Nil {
		t.Error("expected SensorID to be generated (non-nil UUID)")
	}
}

func TestAddSensorStoreError(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrCreateSensor = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID,
		Type:      domain.Humidity,
		MinRange:  0,
		MaxRange:  100,
		Algorithm: domain.Constant,
	})
	if err == nil {
		t.Fatal("expected error when store.CreateSensor fails")
	}
}

func TestListSensorsSuccess(t *testing.T) {
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
		t.Fatalf(unexpectedErrorMsg, err)
	}
	if len(sensors) != 2 {
		t.Errorf("want 2 sensors, got %d", len(sensors))
	}
}

func TestDeleteSensorSuccess(t *testing.T) {
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
		t.Fatalf(unexpectedErrorMsg, err)
	}
}

func TestDeleteSensorStoreError(t *testing.T) {
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

func TestUpdateConfigSuccess(t *testing.T) {
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
		t.Fatalf(unexpectedErrorMsg, err)
	}
}

func TestUpdateConfigWorkerNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	freq := 200
	err := reg.UpdateConfig(context.Background(), uuid.New(), domain.GatewayConfigUpdate{
		SendFrequencyMs: &freq,
	})
	if err == nil {
		t.Fatal(msgExpectedErrUnknownWorker)
	}
}

// InjectGatewayAnomaly.
func TestInjectAnomalyNetworkDegradation(t *testing.T) {
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
		t.Fatalf(unexpectedErrorMsg, err)
	}
}

func TestInjectAnomalyDisconnect(t *testing.T) {
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
		t.Fatalf(unexpectedErrorMsg, err)
	}
}

func TestInjectAnomalyWorkerNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.InjectGatewayAnomaly(context.Background(), uuid.New(), domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
	})
	if err == nil {
		t.Fatal(msgExpectedErrUnknownWorker)
	}
}

// InjectSensorOutlier.
func TestInjectSensorOutlierSuccess(t *testing.T) {
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
		t.Fatalf(unexpectedErrorMsg, err)
	}
}

//HandleDecommission

func TestHandleDecommissionSuccess(t *testing.T) {
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

func TestHandleDecommissionInvalidUUIDNoPanic(t *testing.T) {
	setupTestLogger(t)
	reg := newTestRegistry(newTestDeps())
	reg.HandleDecommission("tenant1", "not-a-valid-uuid")
}

func TestHandleDecommissionUnknownGatewayNoPanic(t *testing.T) {
	setupTestLogger(t)
	reg := newTestRegistry(newTestDeps())
	reg.HandleDecommission("tenant1", uuid.New().String())
}

// StopAll.
func TestStopAllMultipleRunning(t *testing.T) {
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

func TestStopAllEmptyNoPanic(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	reg.StopAll(time.Second)
}

//RestoreAll.

func TestRestoreAllNoGateways(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.RestoreAll(context.Background())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
}

func TestRestoreAllStoreListError(t *testing.T) {
	d := newTestDeps()
	d.store.ErrListGateways = fakes.ErrSimulated
	reg := newTestRegistry(d)

	err := reg.RestoreAll(context.Background())
	if err == nil {
		t.Fatal("expected error when store.ListGateways fails")
	}
}

func TestRestoreAllWithProvisionedGateway(t *testing.T) {
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

func TestRegistryStartStoppedGatewayRestarts(t *testing.T) {
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

func TestRegistryStartNotFoundReturnsError(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Start(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown managementID")
	}
}

func TestRegistryStartAlreadyRunningIsIdempotent(t *testing.T) {
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

func TestCompensateConnectorFailsPubAndSubClosed(t *testing.T) {
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

func TestCompensateStoreUpdateProvisionedErrorIgnored(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrUpdateProvisioned = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	gws, _ := d.store.ListGateways(context.Background())
	if len(gws) != 1 {
		t.Errorf("expected 1 gateway, got %d", len(gws))
	}
	if gw.Status != domain.Online {
		t.Errorf("expected Online status, got %v", gw.Status)
	}
}

func TestDeleteStoreDeleteFailsReturnsError(t *testing.T) {
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

func TestUpdateConfigChannelFullDoesNotBlock(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)

	// Fill the internal channel buffer to force the non-blocking default branch.
	for range cap(w.configCh) {
		w.configCh <- domain.GatewayConfigUpdate{}
	}

	freq := 100
	err := reg.UpdateConfig(context.Background(), gw.ManagementGatewayID, domain.GatewayConfigUpdate{SendFrequencyMs: &freq})
	if err == nil {
		t.Errorf("expected a channel full error, but the operation was accepted.")
	}
}

func TestInjectAnomalyChannelFullDoesNotBlock(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, gw.ManagementGatewayID)
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)

	cmd := domain.GatewayAnomalyCommand{
		Type:       domain.Disconnect,
		Disconnect: &domain.DisconnectParams{DurationSeconds: 60},
	}

	// Occupy the anomaly channel to force the non-blocking default branch.
	for range cap(w.anomalyCh) {
		w.anomalyCh <- cmd
	}

	err := reg.InjectGatewayAnomaly(context.Background(), gw.ManagementGatewayID, cmd)
	if err == nil {
		t.Errorf("the operation should have returned a buffer full error.")
	}
}

func TestRestoreAllConnectorFailsContinuesOtherGateways(t *testing.T) {
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

func TestRestoreAllListSensorsFailsHandledGracefully(t *testing.T) {
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

func TestInjectSensorOutlierNilValueUsesDefault(t *testing.T) {
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

func TestAddSensorInvalidRangeReturnsError(t *testing.T) {
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

func TestHandleDecommissionTenantMismatchIgnored(t *testing.T) {
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

func TestDeleteCancelsCommandPump(t *testing.T) {
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

func TestStopNotFoundReturnsErrGatewayNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Stop(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrGatewayNotFound) {
		t.Errorf("expected ErrGatewayNotFound, got %v", err)
	}
}

func TestDeleteNotFoundReturnsErrGatewayNotFound(t *testing.T) {
	reg := newTestRegistry(newTestDeps())
	err := reg.Delete(context.Background(), uuid.New())
	if !errors.Is(err, domain.ErrGatewayNotFound) {
		t.Errorf("expected ErrGatewayNotFound, got %v", err)
	}
}

// GetGateway generic store error (not ErrGatewayNotFound).
func TestGetGatewayGenericStoreError(t *testing.T) {
	d := newTestDeps()
	d.store.ErrGetGatewayByManagementID = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.GetGateway(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error when store returns non-ErrGatewayNotFound error")
	}
	if errors.Is(err, domain.ErrGatewayNotFound) {
		t.Error("expected generic store error, not ErrGatewayNotFound")
	}
}

// InjectSensorOutlier: store returns domain.ErrSensorNotFound.
func TestInjectSensorOutlierStoreReturnsErrSensorNotFound(t *testing.T) {
	d := newTestDeps()
	d.store.ErrGetSensor = domain.ErrSensorNotFound
	reg := newTestRegistry(d)

	err := reg.InjectSensorOutlier(context.Background(), 1, nil)
	if !errors.Is(err, domain.ErrSensorNotFound) {
		t.Errorf("expected ErrSensorNotFound, got %v", err)
	}
}

// InjectSensorOutlier: outlier channel is full.
func TestInjectSensorOutlierChannelFull(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	sensor, _ := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	w := getWorker(t, reg, gw.ManagementGatewayID)
	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)

	// Fill the outlier channel buffer.
	for range cap(w.outlierCh) {
		w.outlierCh <- domain.SensorOutlierCommand{}
	}

	val := 9999.0
	err := reg.InjectSensorOutlier(context.Background(), sensor.ID, &val)
	if err == nil {
		t.Fatal("expected error when outlier channel is full")
	}
}

// RestoreAll skips non-provisioned gateways.
func TestRestoreAllSkipsUnprovisionedGateways(t *testing.T) {
	d := newTestDeps()
	reg := newTestRegistry(d)

	_, _ = d.store.CreateGateway(context.Background(), domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant1",
		Provisioned:         false,
		SendFrequencyMs:     50,
	})

	err := reg.RestoreAll(context.Background())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	// No workers should have been created.
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	if len(reg.workers) != 0 {
		t.Errorf("expected 0 workers for unprovisioned gateways, got %d", len(reg.workers))
	}
}

// runProvisioningSaga: compensate with stageConnect when ListSensors fails after connect.
func TestRunProvisioningSagaListSensorsFailCompensatesStageConnect(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrListSensors = fakes.ErrSimulated
	reg := newTestRegistry(d)

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when ListSensors fails after connect")
	}
	// Gateway must be rolled back.
	gws, _ := d.store.ListGateways(context.Background())
	if len(gws) != 0 {
		t.Errorf("expected 0 gateways after stageConnect rollback, got %d", len(gws))
	}
}

// runProvisioningSaga: compensate logs error when DeleteGateway also fails.
func TestRunProvisioningSagaCompensateDeleteGatewayFails(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrListSensors = fakes.ErrSimulated
	reg := newTestRegistry(d)

	// First call to CreateGateway must succeed; set DeleteGateway error after that.
	// We rely on the fact that CreateGateway runs before ListSensors in the saga.
	// Swap in the delete error right before RestoreAll triggers compensate.
	// The simplest approach: set ErrDeleteGateway upfront; the saga will log the compensate error.
	d.store.ErrDeleteGateway = fakes.ErrSimulated

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when ListSensors fails")
	}
}

// HandleDecommission: store.DeleteGateway fails, error is logged but no panic.
func TestHandleDecommissionDeleteGatewayFails(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	d.store.ErrDeleteGateway = fakes.ErrSimulated

	// Must not panic despite the store error.
	reg.HandleDecommission(gw.TenantID, gw.ManagementGatewayID.String())
}

// startWorker: store.UpdateStatus fails — worker still starts, warning is logged.
func TestStartWorkerUpdateStatusFailsWorkerStillRuns(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrUpdateStatus = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	// Even though UpdateStatus fails, CreateAndStart should succeed (warning only).
	gw, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err != nil {
		t.Fatalf(unexpectedErrorMsg, err)
	}
	if gw == nil {
		t.Fatal("expected non-nil gateway")
	}
}

// startWorker: covers the generator-loop body when sensors exist at restore time.
func TestRestoreAllWithSensorsCoversGeneratorLoop(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	// Add a sensor so it is stored and returned during RestoreAll.
	_, _ = reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom,
	})

	_ = reg.Stop(context.Background(), gw.ManagementGatewayID)
	reg.mu.Lock()
	delete(reg.workers, gw.ManagementGatewayID)
	reg.mu.Unlock()

	err := reg.RestoreAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error during RestoreAll with sensors: %v", err)
	}
	defer reg.StopAll(time.Second)
}

// AddSensor: store.CreateSensor fails with valid sensor ranges → error is returned.
func TestAddSensorStoreErrorWithValidRanges(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.store.ErrCreateSensor = fakes.ErrSimulated
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	gw, _ := reg.CreateAndStart(context.Background(), makeCreateReq())
	_, err := reg.AddSensor(context.Background(), gw.ID, domain.SimSensor{
		GatewayID: gw.ID, Type: domain.Temperature,
		MinRange: 0, MaxRange: 100, // valid ranges so CreateSensor is actually called
		Algorithm: domain.UniformRandom,
	})
	if err == nil {
		t.Fatal("expected error when store.CreateSensor fails with valid ranges")
	}
}

// InjectSensorOutlier: sensor exists in store but no worker handles its gateway → ErrGatewayNotFound.
func TestInjectSensorOutlierSensorExistsButNoWorker(t *testing.T) {
	d := newTestDeps()
	reg := newTestRegistry(d)

	// Insert a sensor directly into the store with a gateway ID that has no worker.
	sensorID, _ := d.store.CreateSensor(context.Background(), domain.SimSensor{
		GatewayID: 9999,
		SensorID:  uuid.New(),
		Type:      domain.Temperature,
		MinRange:  0,
		MaxRange:  100,
		Algorithm: domain.UniformRandom,
	})

	err := reg.InjectSensorOutlier(context.Background(), sensorID, nil)
	if !errors.Is(err, domain.ErrGatewayNotFound) {
		t.Errorf("expected ErrGatewayNotFound when sensor exists but no worker, got %v", err)
	}
}

// runProvisioningSaga: provisioner returns a non-UUID GatewayID → parse error.
func TestRunProvisioningSagaBadGatewayIDFromProvisioner(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	d.provisioner.Result.GatewayID = "not-a-valid-uuid"
	reg := newTestRegistry(d)

	_, err := reg.CreateAndStart(context.Background(), makeCreateReq())
	if err == nil {
		t.Fatal("expected error when provisioner returns invalid UUID")
	}
}

// startWorker goroutine: subscription channel closed — pump goroutine exits gracefully via !ok.
func TestStartWorkerSubscriptionChannelClosedExitsGracefully(t *testing.T) {
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())
	// Closing the channel causes the pump goroutine to receive (_, false) and return.
	close(d.connector.Subscription.Ch)
	time.Sleep(50 * time.Millisecond)
}

// startWorker goroutine: command channel is full — command is dropped with a warning.
func TestStartWorkerCommandChannelFullDropsCommand(t *testing.T) {
	setupTestLogger(t)
	d := newTestDeps()
	d.provisioner.Result = provisionResult()
	reg := newTestRegistry(d)
	defer reg.StopAll(2 * time.Second)

	_, _ = reg.CreateAndStart(context.Background(), makeCreateReq())
	w := getWorker(t, reg, func() uuid.UUID {
		reg.mu.RLock()
		defer reg.mu.RUnlock()
		for id := range reg.workers {
			return id
		}
		return uuid.Nil
	}())

	// Stop the sensorLoop (which would otherwise drain commandCh) without cancelling
	// the pump goroutine context — commandPumpCtx is a child of the outer context, not
	// of w.cancel, so the pump goroutine keeps running after w.cancel() is called.
	w.isRunning.Store(false)
	w.cancel()
	<-w.done // wait for sensorLoop to fully exit

	// Fill commandCh now that no goroutine is draining it.
	for range cap(w.commandCh) {
		w.commandCh <- domain.IncomingCommand{}
	}

	// Send a command via the subscription — pump goroutine will find commandCh full
	// and hit the default (drop) branch, logging the warning.
	d.connector.Subscription.Ch <- domain.IncomingCommand{CommandID: "drop-me", Type: domain.ConfigUpdate}
	time.Sleep(100 * time.Millisecond)
}
