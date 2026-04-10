-- Rebuild legacy schemas that still contain gateways.serial_number.
-- Keeps existing data and enforces the current gateway model (no serial number).

CREATE TABLE gateways_new (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    management_gateway_id TEXT    NOT NULL UNIQUE,
    factory_id            TEXT    NOT NULL,
    factory_key           TEXT    NOT NULL,
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

INSERT INTO gateways_new (
    id,
    management_gateway_id,
    factory_id,
    factory_key,
    model,
    firmware_version,
    provisioned,
    cert_pem,
    private_key_pem,
    encryption_key,
    send_frequency_ms,
    status,
    tenant_id,
    created_at
)
SELECT
    id,
    management_gateway_id,
    factory_id,
    factory_key,
    model,
    firmware_version,
    provisioned,
    cert_pem,
    private_key_pem,
    encryption_key,
    send_frequency_ms,
    status,
    tenant_id,
    created_at
FROM gateways;

CREATE TABLE sensors_backup AS
SELECT
    id,
    gateway_id,
    sensor_id,
    type,
    min_range,
    max_range,
    algorithm,
    created_at
FROM sensors;

DROP TABLE sensors;
DROP TABLE gateways;

ALTER TABLE gateways_new RENAME TO gateways;

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

INSERT INTO sensors (
    id,
    gateway_id,
    sensor_id,
    type,
    min_range,
    max_range,
    algorithm,
    created_at
)
SELECT
    id,
    gateway_id,
    sensor_id,
    type,
    min_range,
    max_range,
    algorithm,
    created_at
FROM sensors_backup;

DROP TABLE sensors_backup;

CREATE INDEX IF NOT EXISTS idx_sensors_gateway_id ON sensors(gateway_id);
