CREATE TABLE IF NOT EXISTS sensors (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    gateway_id  INTEGER NOT NULL REFERENCES gateways(id) ON DELETE CASCADE,
    sensor_id   TEXT    NOT NULL UNIQUE,
    type        TEXT    NOT NULL,
    min_range   REAL    NOT NULL,
    max_range   REAL    NOT NULL,
    algorithm   TEXT    NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sensors_gateway_id ON sensors(gateway_id);