package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// A named type wrapping a []byte of exactly 32 bytes (AES-256).
// Using a distinct named type prevents accidental logging, string comparison, or JSON unmarshalling of the raw key material.
type EncryptionKey struct {
	value []byte
}

// NewEncryptionKey with a check on the key length, since AES-256 requires a 32-byte key.
func NewEncryptionKey(key []byte) (EncryptionKey, error) {
	if len(key) != 32 {
		return EncryptionKey{}, errors.New("encryption key must be exactly 32 bytes long")
	}
	return EncryptionKey{value: key}, nil
}

// Bytes returns the raw byte slice of the key. This is the only way to access the key material. Used exculisively by AESGCMEncryptor.
func (k EncryptionKey) Bytes() []byte {
	b := make([]byte, len(k.value))
	copy(b, k.value[:])
	return b
}

// The three-part output of Encryptor.Encrypt. Assembled into TelemetryEnvelope.
type EncryptedPayload struct {
	EncryptedData string
	IV            string
	AuthTag       string
}

// TelemetryEnvelope represents the payload that will be transmitted via NATS.
type TelemetryEnvelope struct {
	GatewayID     string    `json:"gatewayId"`
	SensorID      string    `json:"sensorId"`
	SensorType    string    `json:"sensorType"`
	Timestamp     time.Time `json:"timestamp"`
	KeyVersion    int       `json:"keyVersion"`
	EncryptedData string    `json:"encryptedData"`
	IV            string    `json:"iv"`
	AuthTag       string    `json:"authTag"`
}

// Command object for creating a new virtual gateway.
type CreateGatewayRequest struct {
	Name            string
	TenantID        string
	FactoryID       string
	FactoryKey      string
	SerialNumber    string
	Model           string
	FirmwareVersion string
	SendFrequencyMs int
}

// Command object for load-test batch creation.
type BulkCreateRequest struct {
	Count           int
	TenantID        string
	FactoryID       string
	FactoryKey      string
	Model           string
	FirmwareVersion string
	SendFrequencyMs int
}

// The result of a successful provisioning bootstrap.
type ProvisionResult struct {
	CertPEM       []byte
	PrivateKeyPEM []byte
	AESKey        EncryptionKey
}

//Enums.

type GatewayStatus string

const (
	Provisioning   GatewayStatus = "provisioning"
	Running        GatewayStatus = "running"
	Stopped        GatewayStatus = "stopped"
	Decommissioned GatewayStatus = "decommissioned"
)

type SensorType string

const (
	Temperature SensorType = "temperature"
	Humidity    SensorType = "humidity"
	Pressure    SensorType = "pressure"
	Movement    SensorType = "movement"
	Biometric   SensorType = "biometric"
)

type GenerationAlgorithmType string

const (
	UniformRandom GenerationAlgorithmType = "uniform_random"
	SineWave      GenerationAlgorithmType = "sine_wave"
	Spike         GenerationAlgorithmType = "spike"
	Constant      GenerationAlgorithmType = "constant"
)

// The primary entity. Owns the complete lifecycle state of one virtual gateway.
type SimGateway struct {
	ID                  int64
	ManagementGatewayID uuid.UUID
	FactoryID           string
	FactoryKey          string
	SerialNumber        string
	Model               string
	FirmwareVersion     string
	Provisioned         bool
	CertPEM             []byte
	PrivateKeyPEM       []byte
	EncryptionKey       EncryptionKey
	SendFrequencyMs     int
	Status              GatewayStatus
	TenantID            string
	CreatedAt           time.Time
}

// One sensor profile attached to a virtual gateway.
// Controls what synthetic data the worker generates.
type SimSensor struct {
	ID        int64
	GatewayID int64
	SensorID  uuid.UUID
	Type      SensorType
	MinRange  float64
	MaxRange  float64
	Algorithm GenerationAlgorithmType
	CreatedAt time.Time
}

type GatewayConfigUpdate struct {
	SendFrequencyMs *int
	Status          *GatewayStatus
}

//Anomaly domain types.

type AnomalyType string

const (
	NetworkDegradation AnomalyType = "network_degradation"
	Disconnect         AnomalyType = "disconnect"
)

type NetworkDegradationParams struct {
	DurationSeconds int
	PacketLossPct   float64
}

type DisconnectParams struct {
	DurationSeconds int
}

//Commands

type CommandType string

const (
	ConfigUpdate CommandType = "config_update"
	FirmwarePush CommandType = "firmware_push"
)

type CommandACKStatus string

const (
	ACK     CommandACKStatus = "ack"
	NACK    CommandACKStatus = "nack"
	Expired CommandACKStatus = "expired"
)

type CommandConfigPayload struct {
	SendFrequencyMs *int           `json:"send_frequency_ms,omitempty"`
	Status          *GatewayStatus `json:"status,omitempty"`
}

type CommandFirmwarePayload struct {
	FirmwareVersion string `json:"firmware_version"`
	DownloadURL     string `json:"download_url"`
}

type IncomingCommand struct {
	CommandID       string
	Type            CommandType
	ConfigPayload   *CommandConfigPayload
	FirmwarePayload *CommandFirmwarePayload
	IssuedAt        time.Time
}

// CommandACK.
type CommandACK struct {
	CommandID string           `json:"commandId"`
	Status    CommandACKStatus `json:"status"`
	Message   *string          `json:"message,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

// Dispatched to a running GatewayWorker via its anomalyCh.
// Exactly one of the params fields is non-nil, determined by Type.
type GatewayAnomalyCommand struct {
	Type               AnomalyType
	NetworkDegradation *NetworkDegradationParams
	Disconnect         *DisconnectParams
}

// Primes a one-shot override on the generator for the specified sensor.
type SensorOutlierCommand struct {
	SensorID uuid.UUID
	Value    *float64
}
