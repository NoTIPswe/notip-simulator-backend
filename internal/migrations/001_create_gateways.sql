CREATE TABLE IF NOT EXISTS gateways (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    management_gateway_id TEXT    NOT NULL UNIQUE,
    factory_id           TEXT    NOT NULL,
    factory_key          TEXT    NOT NULL,
    serial_number        TEXT    NOT NULL,
    model                TEXT    NOT NULL,
    firmware_version     TEXT    NOT NULL DEFAULT '',
    provisioned          INTEGER NOT NULL DEFAULT 0,
    cert_pem             BLOB,
    private_key_pem      BLOB,
    encryption_key       BLOB,
    send_frequency_ms    INTEGER NOT NULL DEFAULT 5000,
    status               TEXT    NOT NULL DEFAULT 'provisioning',
    tenant_id            TEXT    NOT NULL,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);