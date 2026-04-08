package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

const (
	errCreateGateway     = "CreateGateway: %v"
	errGetGateway        = "GetGateway: %v"
	errCreateSensor      = "CreateSensor: %v"
	errDirectInsert      = "direct insert: %v"
	errExpectedMissingGW = "expected error for missing gateway"
)

// newTestStore creates a temp-file SQLite store with migrations applied.
func newTestStore(t *testing.T) *SQLiteGatewayStore {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return store
}

func newTestGateway() domain.SimGateway {
	key, _ := domain.NewEncryptionKey(make([]byte, 32))
	return domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		FactoryID:           "factory-1",
		FactoryKey:          "key-1",
		Model:               "model-X",
		FirmwareVersion:     "1.0.0",
		SendFrequencyMs:     5000,
		Status:              domain.Provisioning,
		TenantID:            "tenant-1",
		CreatedAt:           time.Now().UTC().Truncate(time.Second),
		EncryptionKey:       key,
	}
}

func newTestSensor(gatewayID int64) domain.SimSensor {
	return domain.SimSensor{
		GatewayID: gatewayID,
		SensorID:  uuid.New(),
		Type:      domain.Temperature,
		MinRange:  0,
		MaxRange:  100,
		Algorithm: domain.UniformRandom,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

// --- Gateway CRUD ---

func TestGetGatewayFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	gw, err := store.GetGateway(ctx, id)
	if err != nil {
		t.Fatalf(errGetGateway, err)
	}
	if gw.ID != id {
		t.Errorf("want ID %d, got %d", id, gw.ID)
	}
	if gw.FactoryID != "factory-1" {
		t.Errorf("want factory-1, got %s", gw.FactoryID)
	}
}

func TestGetGatewayNotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetGateway(context.Background(), 99999)
	if err == nil {
		t.Fatal(errExpectedMissingGW)
	}
}

func TestListGatewaysEmpty(t *testing.T) {
	store := newTestStore(t)
	gws, err := store.ListGateways(context.Background())
	if err != nil {
		t.Fatalf("ListGateways: %v", err)
	}
	if len(gws) != 0 {
		t.Errorf("want 0 gateways, got %d", len(gws))
	}
}

func TestListGatewaysWithData(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for range 3 {
		if _, err := store.CreateGateway(ctx, newTestGateway()); err != nil {
			t.Fatalf(errCreateGateway, err)
		}
	}

	gws, err := store.ListGateways(ctx)
	if err != nil {
		t.Fatalf("ListGateways: %v", err)
	}
	if len(gws) != 3 {
		t.Errorf("want 3 gateways, got %d", len(gws))
	}
}

func TestUpdateProvisionedSuccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	aesKey, _ := domain.NewEncryptionKey(make([]byte, 32))
	result := domain.ProvisionResult{
		CertPEM:       []byte("cert"),
		PrivateKeyPEM: []byte("key"),
		AESKey:        aesKey,
	}
	if err := store.UpdateProvisioned(ctx, id, result); err != nil {
		t.Fatalf("UpdateProvisioned: %v", err)
	}

	gw, err := store.GetGateway(ctx, id)
	if err != nil {
		t.Fatalf("GetGateway after update: %v", err)
	}
	if !gw.Provisioned {
		t.Error("expected gateway to be provisioned")
	}
}

func TestUpdateProvisionedNotFound(t *testing.T) {
	store := newTestStore(t)
	aesKey, _ := domain.NewEncryptionKey(make([]byte, 32))
	err := store.UpdateProvisioned(context.Background(), 99999, domain.ProvisionResult{AESKey: aesKey})
	if err == nil {
		t.Fatal(errExpectedMissingGW)
	}
}

func TestUpdateStatusSuccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	if err := store.UpdateStatus(ctx, id, domain.Online); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	gw, err := store.GetGateway(ctx, id)
	if err != nil {
		t.Fatalf(errGetGateway, err)
	}
	if gw.Status != domain.Online {
		t.Errorf("want Online, got %s", gw.Status)
	}
}

func TestUpdateStatusNotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.UpdateStatus(context.Background(), 99999, domain.Online)
	if err == nil {
		t.Fatal(errExpectedMissingGW)
	}
}

func TestUpdateFrequencySuccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	if err := store.UpdateFrequency(ctx, id, 2000); err != nil {
		t.Fatalf("UpdateFrequency: %v", err)
	}

	gw, err := store.GetGateway(ctx, id)
	if err != nil {
		t.Fatalf(errGetGateway, err)
	}
	if gw.SendFrequencyMs != 2000 {
		t.Errorf("want 2000, got %d", gw.SendFrequencyMs)
	}
}

func TestUpdateFrequencyNotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.UpdateFrequency(context.Background(), 99999, 2000)
	if err == nil {
		t.Fatal(errExpectedMissingGW)
	}
}

func TestUpdateFirmwareVersionSuccess(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	if err := store.UpdateFirmwareVersion(ctx, id, "2.0.0"); err != nil {
		t.Fatalf("UpdateFirmwareVersion: %v", err)
	}

	gw, err := store.GetGateway(ctx, id)
	if err != nil {
		t.Fatalf(errGetGateway, err)
	}
	if gw.FirmwareVersion != "2.0.0" {
		t.Errorf("want 2.0.0, got %s", gw.FirmwareVersion)
	}
}

func TestUpdateFirmwareVersionNotFound(t *testing.T) {
	store := newTestStore(t)
	err := store.UpdateFirmwareVersion(context.Background(), 99999, "2.0.0")
	if err == nil {
		t.Fatal(errExpectedMissingGW)
	}
}

func TestDeleteGateway(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	if err := store.DeleteGateway(ctx, id); err != nil {
		t.Fatalf("DeleteGateway: %v", err)
	}

	_, err = store.GetGateway(ctx, id)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// --- Sensor CRUD ---

func TestCreateAndGetSensor(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	sensorID, err := store.CreateSensor(ctx, newTestSensor(gwID))
	if err != nil {
		t.Fatalf(errCreateSensor, err)
	}

	sensor, err := store.GetSensor(ctx, sensorID)
	if err != nil {
		t.Fatalf("GetSensor: %v", err)
	}
	if sensor.ID != sensorID {
		t.Errorf("want sensor ID %d, got %d", sensorID, sensor.ID)
	}
	if sensor.Type != domain.Temperature {
		t.Errorf("want Temperature, got %s", sensor.Type)
	}
}

func TestGetSensorNotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetSensor(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for missing sensor")
	}
}

func TestGetSensorBySensorID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	s := newTestSensor(gwID)
	_, err = store.CreateSensor(ctx, s)
	if err != nil {
		t.Fatalf(errCreateSensor, err)
	}

	found, err := store.GetSensorBySensorID(ctx, s.SensorID)
	if err != nil {
		t.Fatalf("GetSensorBySensorID: %v", err)
	}
	if found.SensorID != s.SensorID {
		t.Errorf("want SensorID %s, got %s", s.SensorID, found.SensorID)
	}
}

func TestGetSensorBySensorIDNotFound(t *testing.T) {
	store := newTestStore(t)
	_, err := store.GetSensorBySensorID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for missing sensor")
	}
}

func TestListSensorsEmpty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	sensors, err := store.ListSensors(ctx, gwID)
	if err != nil {
		t.Fatalf("ListSensors: %v", err)
	}
	if len(sensors) != 0 {
		t.Errorf("want 0 sensors, got %d", len(sensors))
	}
}

func TestListSensorsWithData(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	for range 2 {
		if _, err := store.CreateSensor(ctx, newTestSensor(gwID)); err != nil {
			t.Fatalf(errCreateSensor, err)
		}
	}

	sensors, err := store.ListSensors(ctx, gwID)
	if err != nil {
		t.Fatalf("ListSensors: %v", err)
	}
	if len(sensors) != 2 {
		t.Errorf("want 2 sensors, got %d", len(sensors))
	}
}

func TestDeleteSensor(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	sensorID, err := store.CreateSensor(ctx, newTestSensor(gwID))
	if err != nil {
		t.Fatalf(errCreateSensor, err)
	}

	if err := store.DeleteSensor(ctx, sensorID); err != nil {
		t.Fatalf("DeleteSensor: %v", err)
	}

	_, err = store.GetSensor(ctx, sensorID)
	if err == nil {
		t.Fatal("expected error after sensor delete")
	}
}

// --- scanGateway / scanSensor error paths via direct SQL ---

func TestScanGatewayInvalidManagementUUID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.db.ExecContext(ctx, `
		INSERT INTO gateways (
			management_gateway_id, factory_id, factory_key, model,
			firmware_version, send_frequency_ms, status, tenant_id, created_at
		) VALUES ('not-a-valid-uuid', 'f', 'k', 'm', '1.0', 1000, 'provisioning', 't', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf(errDirectInsert, err)
	}

	_, err = store.ListGateways(ctx)
	if err == nil {
		t.Fatal("expected parse error for invalid management_gateway_id")
	}
}

func TestScanGatewayInvalidEncryptionKey(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	_, err := store.db.ExecContext(ctx, `
		INSERT INTO gateways (
			management_gateway_id, factory_id, factory_key, model,
			firmware_version, send_frequency_ms, status, tenant_id,
			created_at, encryption_key
		) VALUES (?, 'f', 'k', 'm', '1.0', 1000, 'provisioning', 't', CURRENT_TIMESTAMP, ?)
	`, uuid.New().String(), []byte("too-short"))
	if err != nil {
		t.Fatalf(errDirectInsert, err)
	}

	_, err = store.ListGateways(ctx)
	if err == nil {
		t.Fatal("expected parse error for invalid encryption_key length")
	}
}

func TestScanSensorInvalidSensorUUID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, newTestGateway())
	if err != nil {
		t.Fatalf(errCreateGateway, err)
	}

	_, err = store.db.ExecContext(ctx, `
		INSERT INTO sensors (gateway_id, sensor_id, type, min_range, max_range, algorithm, created_at)
		VALUES (?, 'not-a-valid-uuid', 'temperature', 0, 100, 'uniform_random', CURRENT_TIMESTAMP)
	`, gwID)
	if err != nil {
		t.Fatalf(errDirectInsert, err)
	}

	_, err = store.ListSensors(ctx, gwID)
	if err == nil {
		t.Fatal("expected parse error for invalid sensor_id")
	}
}

// --- Helpers ---

func TestBoolToIntTrue(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should return 1")
	}
}

func TestBoolToIntFalse(t *testing.T) {
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should return 0")
	}
}
