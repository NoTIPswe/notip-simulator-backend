package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/migrations"
)

type SQLiteGatewayStore struct {
	db      *sql.DB
	writeMu sync.Mutex // protects db during migrations
}

func NewStore(path string) (*SQLiteGatewayStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes, so we serialize all access with a mutex

	return &SQLiteGatewayStore{db: db}, nil
}

func (s *SQLiteGatewayStore) RunMigrations(ctx context.Context) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", entry.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, string(data)); err != nil {
			return fmt.Errorf("execute migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *SQLiteGatewayStore) Close() error {
	return s.db.Close()
}

// --- GatewayStore methods implemented below ---

func (s *SQLiteGatewayStore) CreateGateway(ctx context.Context, gw domain.SimGateway) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	res, err := s.db.ExecContext(ctx, `
		INSERT INTO gateways (
			management_gateway_id, factory_id, factory_key,
			serial_number, model, firmware_version,
			provisioned, cert_pem, private_key_pem, encryption_key,
			send_frequency_ms, status, tenant_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		gw.ManagementGatewayID.String(),
		gw.FactoryID,
		gw.FactoryKey,
		gw.SerialNumber,
		gw.Model,
		gw.FirmwareVersion,
		boolToInt(gw.Provisioned),
		gw.CertPEM,
		gw.PrivateKeyPEM,
		gw.EncryptionKey.Bytes(),
		gw.SendFrequencyMs,
		string(gw.Status),
		gw.TenantID,
		gw.CreatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("create gateway: %w", err)
	}
	return res.LastInsertId()
}

func (s *SQLiteGatewayStore) GetGateway(ctx context.Context, id int64) (*domain.SimGateway, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT * FROM gateways WHERE id = ?`, id)
	return scanGateway(row)
}

func (s *SQLiteGatewayStore) GetGatewayByManagementID(ctx context.Context, managementID uuid.UUID) (*domain.SimGateway, error) {
	row := s.db.QueryRowContext(ctx, `SELECT * FROM gateways WHERE management_gateway_id = ?`, managementID.String())
	return scanGateway(row)
}

func (s *SQLiteGatewayStore) ListGateways(ctx context.Context) ([]*domain.SimGateway, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM gateways`)
	if err != nil {
		return nil, fmt.Errorf("list gateways: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Warn("failed to close database rows", "error", closeErr)
		}
	}()

	var gateways []*domain.SimGateway
	for rows.Next() {
		gw, err := scanGateway(rows)
		if err != nil {
			return nil, err
		}
		gateways = append(gateways, gw)
	}
	return gateways, rows.Err()
}

func (s *SQLiteGatewayStore) UpdateProvisioned(ctx context.Context, id int64, result domain.ProvisionResult) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	res, err := s.db.ExecContext(ctx, `
		UPDATE gateways 
		SET provisioned = 1, cert_pem = ?, private_key_pem = ?, encryption_key = ?
		WHERE id = ?`,
		result.CertPEM,
		result.PrivateKeyPEM,
		result.AESKey.Bytes(),
		id,
	)
	if err != nil {
		return err
	}
	// Confirm that the target gateway exists by checking the number of affected rows.
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("gateway with ID %d not found", id)
	}

	return nil
}

func (s *SQLiteGatewayStore) UpdateStatus(ctx context.Context, id int64, status domain.GatewayStatus) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Execute the update statement and capture the result metadata.
	res, err := s.db.ExecContext(ctx, `UPDATE gateways SET status = ? WHERE id = ?`, string(status), id)
	if err != nil {
		return err
	}

	// Verify that at least one record was modified by the operation.
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("gateway with ID %d not found", id)
	}

	return nil
}

func (s *SQLiteGatewayStore) UpdateFrequency(ctx context.Context, id int64, frequency int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Update the reporting frequency for the specified gateway instance.
	res, err := s.db.ExecContext(ctx, `UPDATE gateways SET send_frequency_ms = ? WHERE id = ?`, frequency, id)
	if err != nil {
		return err
	}

	// An error is returned if no records match the provided gateway ID.
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("gateway with ID %d not found", id)
	}

	return nil
}

func (s *SQLiteGatewayStore) UpdateFirmwareVersion(ctx context.Context, id int64, version string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Persist the new firmware version string in the database.
	res, err := s.db.ExecContext(ctx, `UPDATE gateways SET firmware_version = ? WHERE id = ?`, version, id)
	if err != nil {
		return err
	}

	// Ensure the update operation targeted an existing record.
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("gateway with ID %d not found", id)
	}

	return nil
}

func (s *SQLiteGatewayStore) DeleteGateway(ctx context.Context, id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM gateways WHERE id = ?`, id)
	return err
}

func (s *SQLiteGatewayStore) CreateSensor(ctx context.Context, sensor domain.SimSensor) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	res, err := s.db.ExecContext(ctx, `INSERT INTO sensors (gateway_id, sensor_id, type, min_range, max_range, algorithm, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sensor.GatewayID,
		sensor.SensorID.String(),
		string(sensor.Type),
		sensor.MinRange,
		sensor.MaxRange,
		string(sensor.Algorithm),
		sensor.CreatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("create sensor: %w", err)
	}
	return res.LastInsertId()
}

func (s *SQLiteGatewayStore) ListSensors(ctx context.Context, gatewayID int64) ([]*domain.SimSensor, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM sensors WHERE gateway_id = ?`, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("list sensors: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Warn("failed to close database rows", "error", closeErr)
		}
	}()

	var sensors []*domain.SimSensor
	for rows.Next() {
		s, err := scanSensor(rows)
		if err != nil {
			return nil, err
		}
		sensors = append(sensors, s)
	}
	return sensors, rows.Err()
}

func (s *SQLiteGatewayStore) DeleteSensor(ctx context.Context, id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM sensors WHERE id = ?`, id)
	return err
}

func (s *SQLiteGatewayStore) GetSensor(ctx context.Context, id int64) (*domain.SimSensor, error) {
	row := s.db.QueryRowContext(ctx, `SELECT * FROM sensors WHERE id = ?`, id)
	return scanSensor(row)
}

// --- Helper functions below ---

type scanner interface {
	Scan(dest ...any) error
}

func scanGateway(s scanner) (*domain.SimGateway, error) {
	var (
		gw           domain.SimGateway
		managementID string
		provisioned  int
		status       string
		encKeyBytes  []byte
		createdAt    time.Time
	)

	err := s.Scan(
		&gw.ID,
		&managementID,
		&gw.FactoryID,
		&gw.FactoryKey,
		&gw.SerialNumber,
		&gw.Model,
		&gw.FirmwareVersion,
		&provisioned,
		&gw.CertPEM,
		&gw.PrivateKeyPEM,
		&encKeyBytes,
		&gw.SendFrequencyMs,
		&status,
		&gw.TenantID,
		&createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan gateway: %w", err)
	}

	gw.ManagementGatewayID, err = uuid.Parse(managementID)
	if err != nil {
		return nil, fmt.Errorf("parse management_gateway_id: %w", err)
	}

	gw.EncryptionKey, err = domain.NewEncryptionKey(encKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse encryption_key: %w", err)
	}

	gw.Provisioned = provisioned == 1
	gw.Status = domain.GatewayStatus(status)
	gw.CreatedAt = createdAt

	return &gw, nil
}

func scanSensor(s scanner) (*domain.SimSensor, error) {
	var (
		sensor     domain.SimSensor
		sensorID   string
		sensorType string
		algorithm  string
		createdAt  time.Time
	)

	err := s.Scan(
		&sensor.ID,
		&sensor.GatewayID,
		&sensorID,
		&sensorType,
		&sensor.MinRange,
		&sensor.MaxRange,
		&algorithm,
		&createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan sensor: %w", err)
	}

	sensor.SensorID, err = uuid.Parse(sensorID)
	if err != nil {
		return nil, fmt.Errorf("parse sensor_id: %w", err)
	}

	sensor.Type = domain.SensorType(sensorType)
	sensor.Algorithm = domain.GenerationAlgorithmType(algorithm)
	sensor.CreatedAt = createdAt

	return &sensor, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
