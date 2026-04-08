package domain

import (
	"encoding/json"
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
	copy(b, k.value)
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

// CreateGatewayRequest is the application-layer command for creating a gateway.
// JSON tags reflect the camelCase contract used by HTTP clients.
type CreateGatewayRequest struct {
	FactoryID       string `json:"factoryId"`
	FactoryKey      string `json:"factoryKey"`
	Model           string `json:"model"`
	FirmwareVersion string `json:"firmwareVersion"`
	SendFrequencyMs int    `json:"sendFrequencyMs"`
}

// BulkCreateRequest is the application-layer command for batch gateway creation.
type BulkCreateRequest struct {
	Count           int    `json:"count"`
	FactoryID       string `json:"factoryId"`
	FactoryKey      string `json:"factoryKey"`
	Model           string `json:"model"`
	FirmwareVersion string `json:"firmwareVersion"`
	SendFrequencyMs int    `json:"sendFrequencyMs"`
}

type ProvisionResult struct {
	CertPEM         []byte
	PrivateKeyPEM   []byte
	AESKey          EncryptionKey
	GatewayID       string
	TenantID        string
	SendFrequencyMs int
}

// Enums.

type GatewayStatus string

const (
	Provisioning GatewayStatus = "provisioning"
	Online       GatewayStatus = "online"
	Offline      GatewayStatus = "offline"
	Paused       GatewayStatus = "paused"
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

// SimGateway is the core domain entity. No JSON tags — use the HTTP DTO layer for serialization.
type SimGateway struct {
	ID                  int64
	ManagementGatewayID uuid.UUID
	FactoryID           string
	FactoryKey          string
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

// SimSensor is the sensor domain entity. No JSON tags.
// ID and GatewayID are internal SQLite keys; ManagementGatewayID and SensorID are the public UUIDs.
type SimSensor struct {
	ID                  int64
	GatewayID           int64     // internal SQLite FK, not exposed via HTTP
	ManagementGatewayID uuid.UUID // populated at service layer, not persisted
	SensorID            uuid.UUID
	Type                SensorType
	MinRange            float64
	MaxRange            float64
	Algorithm           GenerationAlgorithmType
	CreatedAt           time.Time
}

// GatewayConfigUpdate carries live configuration changes from NATS commands or HTTP PATCH.
type GatewayConfigUpdate struct {
	SendFrequencyMs *int           `json:"sendFrequencyMs,omitempty"`
	Status          *GatewayStatus `json:"status,omitempty"`
}

// Anomaly domain types.

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

// Commands.

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

// IncomingCommand is deserialized from NATS JetStream messages.
// Payload is a raw JSON object; the worker decodes it based on Type.
type IncomingCommand struct {
	CommandID string          `json:"command_id"`
	Type      CommandType     `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	IssuedAt  time.Time       `json:"issued_at"`
}

type CommandACK struct {
	CommandID string           `json:"command_id"`
	Status    CommandACKStatus `json:"status"`
	Message   *string          `json:"message,omitempty"`
	Timestamp time.Time        `json:"timestamp"`
}

type GatewayAnomalyCommand struct {
	Type               AnomalyType
	NetworkDegradation *NetworkDegradationParams
	Disconnect         *DisconnectParams
}

type SensorOutlierCommand struct {
	SensorID uuid.UUID
	Value    *float64
}
