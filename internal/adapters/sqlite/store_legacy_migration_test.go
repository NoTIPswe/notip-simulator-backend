package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

func TestRunMigrationsRebuildsLegacyGatewaySchemaWithoutSerial(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })

	ctx := context.Background()

	_, err = rawDB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT     PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE gateways (
			id                    INTEGER PRIMARY KEY AUTOINCREMENT,
			management_gateway_id TEXT    NOT NULL UNIQUE,
			factory_id            TEXT    NOT NULL,
			factory_key           TEXT    NOT NULL,
			serial_number         TEXT    NOT NULL,
			model                 TEXT    NOT NULL,
			firmware_version      TEXT    NOT NULL DEFAULT '',
			provisioned           INTEGER NOT NULL DEFAULT 0,
			cert_pem              BLOB,
			private_key_pem       BLOB,
			encryption_key        BLOB,
			send_frequency_ms     INTEGER NOT NULL DEFAULT 5000,
			status                TEXT    NOT NULL DEFAULT 'provisioning',
			tenant_id             TEXT    NOT NULL,
			created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE sensors (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			gateway_id  INTEGER NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
			sensor_id   TEXT    NOT NULL UNIQUE,
			type        TEXT    NOT NULL,
			min_range   REAL    NOT NULL,
			max_range   REAL    NOT NULL,
			algorithm   TEXT    NOT NULL,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_sensors_gateway_id ON sensors(gateway_id);

		INSERT INTO schema_migrations(version) VALUES
			('001_create_gateways.sql'),
			('002_create_sensors.sql');
	`)
	require.NoError(t, err)

	legacyMgmtID := uuid.New()
	legacySensorID := uuid.New()
	res, err := rawDB.ExecContext(ctx, `
		INSERT INTO gateways (
			management_gateway_id, factory_id, factory_key, serial_number,
			model, firmware_version, provisioned, send_frequency_ms,
			status, tenant_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		legacyMgmtID.String(),
		"factory-legacy",
		"key-legacy",
		"serial-legacy",
		"legacy-model",
		"1.0.0",
		1,
		1500,
		"online",
		"tenant-legacy",
		time.Now().UTC(),
	)
	require.NoError(t, err)
	legacyGatewayID, err := res.LastInsertId()
	require.NoError(t, err)

	_, err = rawDB.ExecContext(ctx, `
		INSERT INTO sensors (
			gateway_id, sensor_id, type, min_range, max_range, algorithm, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		legacyGatewayID,
		legacySensorID.String(),
		"temperature",
		0,
		100,
		"constant",
		time.Now().UTC(),
	)
	require.NoError(t, err)

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.RunMigrations(ctx))

	rows, err := store.db.QueryContext(ctx, `PRAGMA table_info(gateways)`)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rows.Close()) })

	hasSerialColumn := false
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk))
		if name == "serial_number" {
			hasSerialColumn = true
		}
	}
	require.NoError(t, rows.Err())
	require.False(t, hasSerialColumn, "legacy serial_number column must be removed")

	gw, err := store.GetGatewayByManagementID(ctx, legacyMgmtID)
	require.NoError(t, err)
	require.Equal(t, "factory-legacy", gw.FactoryID)
	require.Equal(t, "tenant-legacy", gw.TenantID)

	sensors, err := store.ListSensors(ctx, legacyGatewayID)
	require.NoError(t, err)
	require.Len(t, sensors, 1)
	require.Equal(t, legacySensorID, sensors[0].SensorID)

	encryptionKey, err := domain.NewEncryptionKey(make([]byte, 32))
	require.NoError(t, err)
	_, err = store.CreateGateway(ctx, domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		FactoryID:           "factory-new",
		FactoryKey:          "key-new",
		Model:               "model-new",
		FirmwareVersion:     "1.0.0",
		SendFrequencyMs:     1000,
		Status:              domain.Provisioning,
		TenantID:            "tenant-new",
		CreatedAt:           time.Now().UTC(),
		EncryptionKey:       encryptionKey,
	})
	require.NoError(t, err)
}
