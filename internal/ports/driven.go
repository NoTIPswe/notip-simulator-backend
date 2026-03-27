package ports

import (
	"context"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/google/uuid"
)

// All persistence operations for virtual gateways and sensors.
type GatewayStore interface {
	CreateGateway(ctx context.Context, gw domain.SimGateway) (int64, error)
	GetGateway(ctx context.Context, id int64) (*domain.SimGateway, error)
	GetGatewayByManagementID(ctx context.Context, managementID uuid.UUID) (*domain.SimGateway, error)
	ListGateways(ctx context.Context) ([]*domain.SimGateway, error)
	UpdateProvisioned(ctx context.Context, id int64, result domain.ProvisionResult) error
	UpdateStatus(ctx context.Context, id int64, status domain.GatewayStatus) error
	UpdateFirmwareVersion(ctx context.Context, id int64, version string) error
	DeleteGateway(ctx context.Context, id int64) error
	CreateSensor(ctx context.Context, sensor domain.SimSensor) (int64, error)
	ListSensors(ctx context.Context, gatewayID int64) ([]*domain.SimSensor, error)
	DeleteSensor(ctx context.Context, id int64) error
	GetSensor(ctx context.Context, id int64) (*domain.SimSensor, error)
	UpdateFrequency(ctx context.Context, id int64, frequency int) error
}

// Runs the full factory-key provisioning bootstrap. Internally generates an EC keypair and CSR, then calls POST /api/provision/onboard.
type Onboarder interface {
	Onboard(ctx context.Context, factoryID, factoryKey, tenantID string, managementGatewayID uuid.UUID) (domain.ProvisionResult, error)
}

// Publishes a raw byte payload to a NATS subject.
type GatewayPublisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
	Close() error
	Reconnect(ctx context.Context) error
}

// CommandSubscription uses NATS for incoming commands.
type CommandSubscription interface {
	Messages() <-chan domain.IncomingCommand
	Close() error
}

// Opens a per-gateway mTLS NATS connection using the gateway's certificate and private key.
type GatewayConnector interface {
	Connect(ctx context.Context, certPEM []byte, keyPEM []byte, tenantID string, managementGatewayID uuid.UUID) (GatewayPublisher, CommandSubscription, error)
}

// Encryptor encrypts a float64 sensor value with a gateway's EncryptionKey.
// Generates a fresh 12-byte IV per call.
type Encryptor interface {
	Encrypt(key domain.EncryptionKey, data []byte) (domain.EncryptedPayload, error)
}

// Clock abstracts time.Now() for deterministic tests.
type Nower interface {
	Now() time.Time
}
