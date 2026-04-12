# notip-simulator-backend
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=NoTIPswe_notip-simulator-backend&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=NoTIPswe_notip-simulator-backend)
[![Coverage](https://sonarcloud.io/api/project_badges/measure?project=NoTIPswe_notip-simulator-backend&metric=coverage)](https://sonarcloud.io/summary/new_code?id=NoTIPswe_notip-simulator-backend)

A Go service that simulates IoT gateways for the NoTIP platform. It provisions virtual gateways against the real Provisioning Service, attaches configurable sensors to each one, and streams encrypted telemetry over NATS JetStream — all without physical hardware.

## Overview

The simulator is designed for creating simulated gateways for the NoTIP platform. Each simulated gateway:

1. Onboards itself through the Provisioning Service, receiving a TLS certificate and an AES-256 key.
2. Runs a per-gateway worker goroutine that ticks at a configurable frequency and publishes AES-256-GCM encrypted telemetry envelopes to NATS.
3. Consumes commands (`config_update`, `firmware_push`) from NATS JetStream and replies with ACK/NACK.
4. Supports on-demand anomaly injection (network degradation, disconnect, sensor outliers) to exercise downstream fault-handling logic.
5. Stops automatically when a decommission event is received from NATS.

Persistence is backed by SQLite. On startup the service can optionally restore all previously provisioned gateways from the database (`RECOVERY_MODE=true`).

## Architecture

```
HTTP API (:8090)
    └── GatewayRegistry
            ├── GatewayWorker (one per active gateway)
            │       ├── Sensor generators (uniform_random | sine_wave | spike | constant)
            │       ├── MessageBuffer → NATS publisher (per-gateway mTLS connection)
            │       └── Command pump (NATS JetStream consumer)
            ├── SQLiteGatewayStore (persistence)
            └── NATS Decommission Listener (global connection)

Metrics (:9090) — Prometheus /metrics endpoint
```

The codebase follows a ports-and-adapters (hexagonal) layout under `internal/`:

| Package | Role |
|---|---|
| `domain` | Core entities and value objects (`SimGateway`, `SimSensor`, `TelemetryEnvelope`, …) |
| `app` | Application logic: `GatewayRegistry`, `GatewayWorker`, `MessageBuffer` |
| `adapters/http` | REST handlers and DTO layer |
| `adapters/nats` | NATS connector, publisher, command pump, decommission listener |
| `adapters/sqlite` | SQLite store with migrations |
| `generator` | Sensor value generators |
| `ports` | Interface definitions (`GatewayStore`, `GatewayPublisher`, `Encryptor`, …) |
| `config` | Environment variable loading |
| `metrics` | Prometheus metrics |

## REST API

Base URL: `http://localhost:8090`

### Gateways

| Method | Path | Description |
|---|---|---|
| `POST` | `/sim/gateways` | Create and provision a single gateway |
| `POST` | `/sim/gateways/bulk` | Create and provision multiple gateways |
| `GET` | `/sim/gateways` | List all gateways |
| `GET` | `/sim/gateways/{id}` | Get a single gateway |
| `POST` | `/sim/gateways/{id}/start` | Start telemetry emission |
| `POST` | `/sim/gateways/{id}/stop` | Stop telemetry emission |
| `DELETE` | `/sim/gateways/{id}` | Decommission and remove a gateway |

### Sensors

| Method | Path | Description |
|---|---|---|
| `POST` | `/sim/gateways/{id}/sensors` | Attach a sensor to a gateway |
| `GET` | `/sim/gateways/{id}/sensors` | List sensors for a gateway |
| `DELETE` | `/sim/sensors/{sensorId}` | Remove a sensor |

Supported sensor types: `temperature`, `humidity`, `pressure`, `movement`, `biometric`.  
Supported generation algorithms: `uniform_random`, `sine_wave`, `spike`, `constant`.

### Anomaly injection

| Method | Path | Description |
|---|---|---|
| `POST` | `/sim/gateways/{id}/anomaly/network-degradation` | Simulate packet loss for a duration |
| `POST` | `/sim/gateways/{id}/anomaly/disconnect` | Simulate a full disconnect for a duration |
| `POST` | `/sim/sensors/{sensorId}/anomaly/outlier` | Inject a single out-of-range sensor value |

### Health

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness probe |
| `GET` | `/metrics` (`:9090`) | Prometheus metrics |

## NATS subjects

The service interacts with the following JetStream subjects (see [`api-contracts/asyncapi/nats-contracts.yaml`](api-contracts/asyncapi/nats-contracts.yaml) for the full schema):

| Direction | Subject pattern | Description |
|---|---|---|
| Publish | `telemetry.data.{tenantId}.{gatewayId}` | Encrypted sensor telemetry |
| Subscribe | `command.gw.{tenantId}.{gatewayId}` | Incoming gateway commands |
| Publish | `command.ack.{tenantId}.{gatewayId}` | Command acknowledgements |
| Subscribe | `gateway.decommissioned.{tenantId}.{gatewayId}` | Decommission events |

All NATS connections use mTLS (TLS 1.3).

## Configuration

All configuration is provided through environment variables.

| Variable | Required | Default | Description |
|---|---|---|---|
| `PROVISIONING_URL` | Yes | — | Base URL of the Provisioning Service |
| `NATS_URL` | Yes | — | NATS server URL (e.g. `tls://nats:4222`) |
| `NATS_CA_CERT_PATH` | Yes | — | Path to the CA certificate for NATS mTLS |
| `NATS_TLS_CERT` | No | — | Path to the client TLS certificate (must be paired with `NATS_TLS_KEY`) |
| `NATS_TLS_KEY` | No | — | Path to the client TLS private key |
| `SQLITE_PATH` | No | `/data/simulator.db` | SQLite database file path |
| `HTTP_ADDR` | No | `:8090` | Address for the REST API server |
| `METRICS_ADDR` | No | `:9090` | Address for the Prometheus metrics server |
| `DEFAULT_SEND_FREQUENCY_MS` | No | `5000` | Default telemetry send interval in milliseconds |
| `GATEWAY_BUFFER_SIZE` | No | `1000` | Per-gateway outbound message buffer size |
| `RECOVERY_MODE` | No | `false` | Restore provisioned gateways from SQLite on startup |

## Development

```bash
# Run unit tests with coverage
make test

# Run integration tests (requires Docker for Testcontainers)
make integration-test

# Build the binary
make build

# Run locally
make run

# Format and lint
make fmt lint

# Build Docker image
make docker-build
```

### Fetching API contracts

```bash
# Sync AsyncAPI NATS contracts from the infra repo
make fetch-contracts

# Sync OpenAPI spec from the provisioning service repo
make fetch-openapi
```

## Docker

```dockerfile
# Production image (CGO_ENABLED=0, scratch base)
docker build --target prod -t notip-simulator-backend .
```

The container expects the environment variables above and, if mTLS is enabled, certificate files to be mounted at the configured paths.
