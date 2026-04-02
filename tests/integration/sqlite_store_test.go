//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/sqlite"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

// ─────────────────────────────────────────────────────────────────────────────
// Gateway CRUD
// ─────────────────────────────────────────────────────────────────────────────

func TestSQLiteStore_CreateAndGetGateway(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	gw := domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		FactoryID:           "factory-1",
		FactoryKey:          "key-1",
		SerialNumber:        "SN-001",
		Model:               "ModelX",
		FirmwareVersion:     "1.0.0",
		SendFrequencyMs:     1000,
		Status:              domain.Provisioning,
		TenantID:            "tenant-1",
		CreatedAt:           time.Now().UTC().Truncate(time.Second),
		EncryptionKey:       validAESKey(t),
	}

	id, err := store.CreateGateway(ctx, gw)
	require.NoError(t, err)
	assert.Positive(t, id)

	got, err := store.GetGateway(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, gw.ManagementGatewayID, got.ManagementGatewayID)
	assert.Equal(t, gw.TenantID, got.TenantID)
	assert.Equal(t, gw.Status, got.Status)
}

func TestSQLiteStore_GetGatewayByManagementID(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	mgmtID := uuid.New()
	id, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: mgmtID,
		TenantID:            "tenant-1",
		Status:              domain.Provisioning,
		EncryptionKey:       validAESKey(t),
	})
	require.NoError(t, err)

	got, err := store.GetGatewayByManagementID(ctx, mgmtID)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, mgmtID, got.ManagementGatewayID)
}

func TestSQLiteStore_GetGatewayByManagementID_NotFound(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	_, err := store.GetGatewayByManagementID(ctx, uuid.New())
	assert.Error(t, err)
}

func TestSQLiteStore_ListGateways_Empty(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	list, err := store.ListGateways(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestSQLiteStore_ListGateways_Multiple(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := store.CreateGateway(ctx, domain.SimGateway{
			ManagementGatewayID: uuid.New(),
			TenantID:            "tenant-1",
			Status:              domain.Provisioning,
			EncryptionKey:       validAESKey(t),
		})
		require.NoError(t, err)
	}

	list, err := store.ListGateways(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestSQLiteStore_UpdateProvisioned(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant-1",
		Status:              domain.Provisioning,
		EncryptionKey:       validAESKey(t),
	})
	require.NoError(t, err)

	result := domain.ProvisionResult{
		CertPEM:       []byte("cert-pem"),
		PrivateKeyPEM: []byte("key-pem"),
		AESKey:        validAESKey(t),
	}
	require.NoError(t, store.UpdateProvisioned(ctx, id, result))

	got, err := store.GetGateway(ctx, id)
	require.NoError(t, err)
	assert.True(t, got.Provisioned)
	assert.Equal(t, []byte("cert-pem"), got.CertPEM)
	assert.Equal(t, []byte("key-pem"), got.PrivateKeyPEM)
}

func TestSQLiteStore_UpdateStatus(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant-1",
		Status:              domain.Provisioning,
		EncryptionKey:       validAESKey(t),
	})
	require.NoError(t, err)

	require.NoError(t, store.UpdateStatus(ctx, id, domain.Online))

	got, err := store.GetGateway(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.Online, got.Status)
}

func TestSQLiteStore_UpdateFirmwareVersion(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant-1",
		FirmwareVersion:     "1.0.0",
		Status:              domain.Provisioning,
		EncryptionKey:       validAESKey(t),
	})
	require.NoError(t, err)

	require.NoError(t, store.UpdateFirmwareVersion(ctx, id, "2.0.0"))

	got, err := store.GetGateway(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", got.FirmwareVersion)
}

func TestSQLiteStore_DeleteGateway(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	id, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant-1",
		Status:              domain.Provisioning,
		EncryptionKey:       validAESKey(t),
	})
	require.NoError(t, err)

	require.NoError(t, store.DeleteGateway(ctx, id))

	_, err = store.GetGateway(ctx, id)
	assert.Error(t, err, "gateway should not exist after deletion")
}

// ─────────────────────────────────────────────────────────────────────────────
// Sensor CRUD
// ─────────────────────────────────────────────────────────────────────────────

func TestSQLiteStore_CreateAndGetSensor(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant-1",
		Status:              domain.Provisioning,
		EncryptionKey:       validAESKey(t),
	})
	require.NoError(t, err)

	sensor := domain.SimSensor{
		GatewayID: gwID,
		SensorID:  uuid.New(),
		Type:      domain.Temperature,
		MinRange:  0,
		MaxRange:  100,
		Algorithm: domain.UniformRandom,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	sensorID, err := store.CreateSensor(ctx, sensor)
	require.NoError(t, err)
	assert.Positive(t, sensorID)

	got, err := store.GetSensor(ctx, sensorID)
	require.NoError(t, err)
	assert.Equal(t, sensorID, got.ID)
	assert.Equal(t, gwID, got.GatewayID)
	assert.Equal(t, domain.Temperature, got.Type)
	assert.Equal(t, domain.UniformRandom, got.Algorithm)
}

func TestSQLiteStore_ListSensors(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	gwID, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant-1",
		Status:              domain.Provisioning,
		EncryptionKey:       validAESKey(t),
	})
	require.NoError(t, err)

	types := []domain.SensorType{domain.Temperature, domain.Humidity, domain.Pressure}
	for _, st := range types {
		_, err := store.CreateSensor(ctx, domain.SimSensor{
			GatewayID: gwID,
			SensorID:  uuid.New(),
			Type:      st,
			Algorithm: domain.Constant,
		})
		require.NoError(t, err)
	}

	list, err := store.ListSensors(ctx, gwID)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestSQLiteStore_ListSensors_IsolatedByGateway(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	gw1, _ := store.CreateGateway(ctx, domain.SimGateway{ManagementGatewayID: uuid.New(), TenantID: "t1", Status: domain.Provisioning, EncryptionKey: validAESKey(t)})
	gw2, _ := store.CreateGateway(ctx, domain.SimGateway{ManagementGatewayID: uuid.New(), TenantID: "t1", Status: domain.Provisioning, EncryptionKey: validAESKey(t)})

	store.CreateSensor(ctx, domain.SimSensor{GatewayID: gw1, SensorID: uuid.New(), Type: domain.Temperature, Algorithm: domain.Constant}) //nolint:errcheck
	store.CreateSensor(ctx, domain.SimSensor{GatewayID: gw1, SensorID: uuid.New(), Type: domain.Humidity, Algorithm: domain.Constant})    //nolint:errcheck
	store.CreateSensor(ctx, domain.SimSensor{GatewayID: gw2, SensorID: uuid.New(), Type: domain.Pressure, Algorithm: domain.Constant})    //nolint:errcheck

	list1, err := store.ListSensors(ctx, gw1)
	require.NoError(t, err)
	assert.Len(t, list1, 2)

	list2, err := store.ListSensors(ctx, gw2)
	require.NoError(t, err)
	assert.Len(t, list2, 1)
}

func TestSQLiteStore_DeleteSensor(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	gwID, _ := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(), TenantID: "t1", Status: domain.Provisioning, EncryptionKey: validAESKey(t),
	})

	sensorID, err := store.CreateSensor(ctx, domain.SimSensor{
		GatewayID: gwID, SensorID: uuid.New(), Type: domain.Temperature, Algorithm: domain.Constant,
	})
	require.NoError(t, err)

	require.NoError(t, store.DeleteSensor(ctx, sensorID))

	_, err = store.GetSensor(ctx, sensorID)
	assert.Error(t, err, "sensor should not exist after deletion")
}

func TestSQLiteStore_GetSensor_NotFound(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	_, err := store.GetSensor(ctx, 999)
	assert.Error(t, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Persistence across open/close
// ─────────────────────────────────────────────────────────────────────────────

// TestSQLiteStore_DataSurvivesReopen verifies that records written to SQLite
// survive a close/reopen cycle (i.e. actually written to disk, not just RAM).
func TestSQLiteStore_DataSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/test.db"
	ctx := context.Background()
	mgmtID := uuid.New()

	// Write a gateway.
	{
		store, err := sqlite.NewStore(dbPath)
		require.NoError(t, err)
		require.NoError(t, store.RunMigrations(ctx))
		defer store.Close()

		_, err = store.CreateGateway(ctx, domain.SimGateway{
			ManagementGatewayID: mgmtID,
			TenantID:            "tenant-persist",
			Status:              domain.Provisioning,
			EncryptionKey:       validAESKey(t),
		})
		require.NoError(t, err)
		store.Close()
	}

	// Reopen and verify.
	{
		store, err := sqlite.NewStore(dbPath)
		require.NoError(t, err)
		require.NoError(t, store.RunMigrations(ctx))
		defer store.Close()

		gw, err := store.GetGatewayByManagementID(ctx, mgmtID)
		require.NoError(t, err)
		assert.Equal(t, "tenant-persist", gw.TenantID)
	}
}

func TestSQLiteStore_GetGateway_NoEncryptionKey(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	//Gateway created without an encryption key (not provisioned).
	id, err := store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            "tenant-1",
		Status:              domain.Provisioning,
		// empty encKeyBytes.
	})
	require.NoError(t, err)

	got, err := store.GetGateway(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	// EncryptionKey must be zero value, no panic.
	assert.Equal(t, domain.EncryptionKey{}, got.EncryptionKey)
}

func TestSQLiteStore_TenantIsolation_GatewaysNotShared(t *testing.T) {
	store := newSQLiteStore(t)
	ctx := context.Background()

	//Gateways on different tenants.
	_, _ = store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(), TenantID: "tenant-A",
		Status: domain.Provisioning, EncryptionKey: validAESKey(t),
	})
	_, _ = store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(), TenantID: "tenant-B",
		Status: domain.Provisioning, EncryptionKey: validAESKey(t),
	})

	list, err := store.ListGateways(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)

	tenants := map[string]int{}
	for _, gw := range list {
		tenants[gw.TenantID]++
	}
	assert.Equal(t, 1, tenants["tenant-A"], "tenant-A should have exactly 1 gateway")
	assert.Equal(t, 1, tenants["tenant-B"], "tenant-B should have exactly 1 gateway")
}
