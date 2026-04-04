package dto

import (
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/google/uuid"
)

// GatewayResponse is the HTTP representation of a SimGateway.
// Sensitive fields (CertPEM, PrivateKeyPEM, EncryptionKey, FactoryKey) are intentionally omitted.
// ID is the public UUID (ManagementGatewayID); the internal SQLite int64 key is not exposed.
type GatewayResponse struct {
	ID              uuid.UUID            `json:"id"`
	FactoryID       string               `json:"factoryId"`
	Model           string               `json:"model"`
	FirmwareVersion string               `json:"firmwareVersion"`
	Provisioned     bool                 `json:"provisioned"`
	SendFrequencyMs int                  `json:"sendFrequencyMs"`
	Status          domain.GatewayStatus `json:"status"`
	TenantID        string               `json:"tenantId"`
	CreatedAt       time.Time            `json:"createdAt"`
}

func GatewayFromDomain(gw *domain.SimGateway) GatewayResponse {
	return GatewayResponse{
		ID:              gw.ManagementGatewayID,
		FactoryID:       gw.FactoryID,
		Model:           gw.Model,
		FirmwareVersion: gw.FirmwareVersion,
		Provisioned:     gw.Provisioned,
		SendFrequencyMs: gw.SendFrequencyMs,
		Status:          gw.Status,
		TenantID:        gw.TenantID,
		CreatedAt:       gw.CreatedAt,
	}
}

// GatewayListFromDomain converts a slice of domain gateways, skipping nil entries.
func GatewayListFromDomain(gws []*domain.SimGateway) []GatewayResponse {
	out := make([]GatewayResponse, 0, len(gws))
	for _, gw := range gws {
		if gw != nil {
			out = append(out, GatewayFromDomain(gw))
		}
	}
	return out
}

// SensorResponse is the HTTP representation of a SimSensor.
// ID is the public UUID (SensorID); GatewayID is the gateway's public UUID (ManagementGatewayID).
// Internal SQLite int64 keys are not exposed.
type SensorResponse struct {
	ID        uuid.UUID                      `json:"id"`
	GatewayID uuid.UUID                      `json:"gatewayId"`
	Type      domain.SensorType              `json:"type"`
	MinRange  float64                        `json:"minRange"`
	MaxRange  float64                        `json:"maxRange"`
	Algorithm domain.GenerationAlgorithmType `json:"algorithm"`
	CreatedAt time.Time                      `json:"createdAt"`
}

func SensorFromDomain(s *domain.SimSensor) SensorResponse {
	return SensorResponse{
		ID:        s.SensorID,
		GatewayID: s.ManagementGatewayID,
		Type:      s.Type,
		MinRange:  s.MinRange,
		MaxRange:  s.MaxRange,
		Algorithm: s.Algorithm,
		CreatedAt: s.CreatedAt,
	}
}

func SensorListFromDomain(ss []*domain.SimSensor) []SensorResponse {
	out := make([]SensorResponse, 0, len(ss))
	for _, s := range ss {
		if s != nil {
			out = append(out, SensorFromDomain(s))
		}
	}
	return out
}

// CreateSensorRequest is the HTTP request body for adding a sensor.
type CreateSensorRequest struct {
	Type      domain.SensorType              `json:"type"`
	MinRange  float64                        `json:"minRange"`
	MaxRange  float64                        `json:"maxRange"`
	Algorithm domain.GenerationAlgorithmType `json:"algorithm"`
}

func (r CreateSensorRequest) ToDomain() domain.SimSensor {
	return domain.SimSensor{
		Type:      r.Type,
		MinRange:  r.MinRange,
		MaxRange:  r.MaxRange,
		Algorithm: r.Algorithm,
	}
}
