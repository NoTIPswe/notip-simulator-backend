package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/sqlite"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

//Setup.

func newTestStore(t *testing.T) *sqlite.SQLiteGatewayStore {
	t.Helper()
	store, err := sqlite.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func makeGateway() domain.SimGateway {
	// Create a dummy 32-byte encryption key for testing purposes.
	key, _ := domain.NewEncryptionKey(make([]byte, 32))
	return domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		FactoryID:           "factory-1",
		FactoryKey:          "key-1",
		SerialNumber:        "SN001",
		Model:               "TestModel",
		FirmwareVersion:     "1.0.0",
		SendFrequencyMs:     1000,
		Status:              domain.Provisioning,
		TenantID:            "tenant-1",
		EncryptionKey:       key,
		CreatedAt:           time.Now().UTC().Truncate(time.Second),
	}
}

func makeSensor(gatewayID int64) domain.SimSensor {
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

// Gateway CRUD.
func TestStore_CreateGateway_ReturnsID(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateGateway(context.Background(), makeGateway())
	if err != nil {
		t.Fatalf("CreateGateway: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestStore_GetGateway_AfterCreate(t *testing.T) {
	s := newTestStore(t)
	gw := makeGateway()
	id, _ := s.CreateGateway(context.Background(), gw)

	got, err := s.GetGateway(context.Background(), id)
	if err != nil {
		t.Fatalf("GetGateway: %v", err)
	}
	if got.ManagementGatewayID != gw.ManagementGatewayID {
		t.Errorf("ManagementGatewayID mismatch")
	}
	if got.TenantID != gw.TenantID {
		t.Errorf("TenantID mismatch: got %s", got.TenantID)
	}
	if got.FirmwareVersion != gw.FirmwareVersion {
		t.Errorf("FirmwareVersion mismatch: got %s", got.FirmwareVersion)
	}
}

func TestStore_GetGateway_NotFound_ReturnsError(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetGateway(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for non-existent gateway")
	}
}

func TestStore_GetGatewayByManagementID_Success(t *testing.T) {
	s := newTestStore(t)
	gw := makeGateway()
	_, _ = s.CreateGateway(context.Background(), gw)

	got, err := s.GetGatewayByManagementID(context.Background(), gw.ManagementGatewayID)
	if err != nil {
		t.Fatalf("GetGatewayByManagementID: %v", err)
	}
	if got.ManagementGatewayID != gw.ManagementGatewayID {
		t.Error("ManagementGatewayID mismatch")
	}
}

func TestStore_GetGatewayByManagementID_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetGatewayByManagementID(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown management ID")
	}
}

func TestStore_ListGateways_Empty(t *testing.T) {
	s := newTestStore(t)
	gws, err := s.ListGateways(context.Background())
	if err != nil {
		t.Fatalf("ListGateways: %v", err)
	}
	if len(gws) != 0 {
		t.Errorf("expected 0 gateways, got %d", len(gws))
	}
}

func TestStore_ListGateways_Multiple(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		_, _ = s.CreateGateway(context.Background(), makeGateway())
	}
	gws, err := s.ListGateways(context.Background())
	if err != nil {
		t.Fatalf("ListGateways: %v", err)
	}
	if len(gws) != 3 {
		t.Errorf("expected 3 gateways, got %d", len(gws))
	}
}

func TestStore_UpdateProvisioned_SetsFields(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateGateway(context.Background(), makeGateway())

	key, _ := domain.NewEncryptionKey(make([]byte, 32))
	result := domain.ProvisionResult{
		CertPEM:       []byte("cert-pem"),
		PrivateKeyPEM: []byte("key-pem"),
		AESKey:        key,
	}
	if err := s.UpdateProvisioned(context.Background(), id, result); err != nil {
		t.Fatalf("UpdateProvisioned: %v", err)
	}

	got, _ := s.GetGateway(context.Background(), id)
	if !got.Provisioned {
		t.Error("expected Provisioned to be true")
	}
	if string(got.CertPEM) != "cert-pem" {
		t.Errorf("CertPEM mismatch: %s", got.CertPEM)
	}
}

func TestStore_UpdateProvisioned_NotFound_ReturnsError(t *testing.T) {
	s := newTestStore(t)
	key, _ := domain.NewEncryptionKey(make([]byte, 32))
	err := s.UpdateProvisioned(context.Background(), 99999, domain.ProvisionResult{AESKey: key})
	if err == nil {
		t.Fatal("expected error for non-existent gateway")
	}
}

func TestStore_UpdateStatus_Success(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateGateway(context.Background(), makeGateway())
	if err != nil {
		t.Fatalf("failed to create gateway for status update test: %v.", err)
	}

	if err := s.UpdateStatus(context.Background(), id, domain.Running); err != nil {
		t.Fatalf("UpdateStatus failed unexpectedly: %v.", err)
	}

	got, err := s.GetGateway(context.Background(), id)
	if err != nil {
		t.Fatalf("GetGateway failed after status update: %v.", err)
	}
	if got.Status != domain.Running {
		t.Errorf("expected Running, got %v", got.Status)
	}
}

func TestStore_UpdateStatus_NotFound_ReturnsError(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateStatus(context.Background(), 99999, domain.Running)
	if err == nil {
		t.Fatal("expected error for non-existent gateway")
	}
}

func TestStore_UpdateFirmwareVersion_Success(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateGateway(context.Background(), makeGateway())

	if err := s.UpdateFirmwareVersion(context.Background(), id, "2.0.0"); err != nil {
		t.Fatalf("UpdateFirmwareVersion: %v", err)
	}
	got, _ := s.GetGateway(context.Background(), id)
	if got.FirmwareVersion != "2.0.0" {
		t.Errorf("expected 2.0.0, got %s", got.FirmwareVersion)
	}
}

func TestStore_UpdateFirmwareVersion_NotFound_ReturnsError(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateFirmwareVersion(context.Background(), 99999, "2.0.0")
	if err == nil {
		t.Fatal("expected error for non-existent gateway")
	}
}

func TestStore_DeleteGateway_Success(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateGateway(context.Background(), makeGateway())

	if err := s.DeleteGateway(context.Background(), id); err != nil {
		t.Fatalf("DeleteGateway: %v", err)
	}
	_, err := s.GetGateway(context.Background(), id)
	if err == nil {
		t.Error("expected error after deletion")
	}
}

func TestStore_DeleteGateway_NonExistent_NoError(t *testing.T) {
	// DELETE di un ID non esistente è idempotente in SQL
	s := newTestStore(t)
	err := s.DeleteGateway(context.Background(), 99999)
	if err != nil {
		t.Errorf("unexpected error deleting non-existent gateway: %v", err)
	}
}

// Sensor CRUD.
func TestStore_CreateSensor_ReturnsID(t *testing.T) {
	s := newTestStore(t)
	gwID, _ := s.CreateGateway(context.Background(), makeGateway())

	id, err := s.CreateSensor(context.Background(), makeSensor(gwID))
	if err != nil {
		t.Fatalf("CreateSensor: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive sensor ID, got %d", id)
	}
}

func TestStore_GetSensor_AfterCreate(t *testing.T) {
	s := newTestStore(t)
	gwID, _ := s.CreateGateway(context.Background(), makeGateway())
	sensor := makeSensor(gwID)
	sID, _ := s.CreateSensor(context.Background(), sensor)

	got, err := s.GetSensor(context.Background(), sID)
	if err != nil {
		t.Fatalf("GetSensor: %v", err)
	}
	if got.GatewayID != gwID {
		t.Errorf("GatewayID mismatch: got %d", got.GatewayID)
	}
	if got.SensorID != sensor.SensorID {
		t.Errorf("SensorID mismatch")
	}
	if got.Type != domain.Temperature {
		t.Errorf("Type mismatch: got %v", got.Type)
	}
}

func TestStore_GetSensor_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetSensor(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error for non-existent sensor")
	}
}

func TestStore_ListSensors_FiltersByGatewayID(t *testing.T) {
	s := newTestStore(t)
	gw1, _ := s.CreateGateway(context.Background(), makeGateway())
	gw2, _ := s.CreateGateway(context.Background(), makeGateway())

	_, _ = s.CreateSensor(context.Background(), makeSensor(gw1))
	_, _ = s.CreateSensor(context.Background(), makeSensor(gw1))
	_, _ = s.CreateSensor(context.Background(), makeSensor(gw2))

	sensors, err := s.ListSensors(context.Background(), gw1)
	if err != nil {
		t.Fatalf("ListSensors: %v", err)
	}
	if len(sensors) != 2 {
		t.Errorf("expected 2 sensors for gw1, got %d", len(sensors))
	}

	sensors2, _ := s.ListSensors(context.Background(), gw2)
	if len(sensors2) != 1 {
		t.Errorf("expected 1 sensor for gw2, got %d", len(sensors2))
	}
}

func TestStore_ListSensors_Empty(t *testing.T) {
	s := newTestStore(t)
	gwID, _ := s.CreateGateway(context.Background(), makeGateway())

	sensors, err := s.ListSensors(context.Background(), gwID)
	if err != nil {
		t.Fatalf("ListSensors: %v", err)
	}
	if len(sensors) != 0 {
		t.Errorf("expected 0 sensors, got %d", len(sensors))
	}
}

func TestStore_DeleteSensor_Success(t *testing.T) {
	s := newTestStore(t)
	gwID, _ := s.CreateGateway(context.Background(), makeGateway())
	sID, _ := s.CreateSensor(context.Background(), makeSensor(gwID))

	if err := s.DeleteSensor(context.Background(), sID); err != nil {
		t.Fatalf("DeleteSensor: %v", err)
	}
	_, err := s.GetSensor(context.Background(), sID)
	if err == nil {
		t.Error("expected error after sensor deletion")
	}
}

func TestStore_AllSensorTypes_CanBeStored(t *testing.T) {
	s := newTestStore(t)
	gwID, _ := s.CreateGateway(context.Background(), makeGateway())

	types := []domain.SensorType{
		domain.Temperature,
		domain.Humidity,
		domain.Pressure,
		domain.Movement,
		domain.Biometric,
	}
	for _, ty := range types {
		sensor := makeSensor(gwID)
		sensor.SensorID = uuid.New()
		sensor.Type = ty
		id, err := s.CreateSensor(context.Background(), sensor)
		if err != nil {
			t.Errorf("CreateSensor for type %v: %v", ty, err)
			continue
		}
		got, err := s.GetSensor(context.Background(), id)
		if err != nil {
			t.Errorf("GetSensor for type %v: %v", ty, err)
			continue
		}
		if got.Type != ty {
			t.Errorf("type mismatch: want %v, got %v", ty, got.Type)
		}
	}
}

func TestStore_AllAlgorithms_CanBeStored(t *testing.T) {
	s := newTestStore(t)
	gwID, _ := s.CreateGateway(context.Background(), makeGateway())

	algos := []domain.GenerationAlgorithmType{
		domain.UniformRandom,
		domain.SineWave,
		domain.Spike,
		domain.Constant,
	}
	for _, algo := range algos {
		sensor := makeSensor(gwID)
		sensor.SensorID = uuid.New()
		sensor.Algorithm = algo
		_, err := s.CreateSensor(context.Background(), sensor)
		if err != nil {
			t.Errorf("CreateSensor for algo %v: %v", algo, err)
		}
	}
}

// Migrations.
func TestStore_RunMigrations_Idempotent(t *testing.T) {
	s := newTestStore(t) //The first run is done by the builder.
	// Second run so must be flawless.
	if err := s.RunMigrations(context.Background()); err != nil {
		t.Fatalf("RunMigrations (second run): %v", err)
	}
}
