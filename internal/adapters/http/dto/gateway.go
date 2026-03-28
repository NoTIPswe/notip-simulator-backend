package dto

import (
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/google/uuid"
)

// GatewayResponse is the HTTP representation of a SimGateway.
// Sensitive fields (CertPEM, PrivateKeyPEM, EncryptionKey, FactoryKey) are intentionally omitted.
type GatewayResponse struct {
	ID                  int64                `json:"id"`
	ManagementGatewayID uuid.UUID            `json:"managementGatewayId"`
	FactoryID           string               `json:"factoryId"`
	SerialNumber        string               `json:"serialNumber"`
	Model               string               `json:"model"`
	FirmwareVersion     string               `json:"firmwareVersion"`
	Provisioned         bool                 `json:"provisioned"`
	SendFrequencyMs     int                  `json:"sendFrequencyMs"`
	Status              domain.GatewayStatus `json:"status"`
	TenantID            string               `json:"tenantId"`
	CreatedAt           time.Time            `json:"createdAt"`
}

func GatewayFromDomain(gw *domain.SimGateway) GatewayResponse {
	return GatewayResponse{
		ID:                  gw.ID,
		ManagementGatewayID: gw.ManagementGatewayID,
		FactoryID:           gw.FactoryID,
		SerialNumber:        gw.SerialNumber,
		Model:               gw.Model,
		FirmwareVersion:     gw.FirmwareVersion,
		Provisioned:         gw.Provisioned,
		SendFrequencyMs:     gw.SendFrequencyMs,
		Status:              gw.Status,
		TenantID:            gw.TenantID,
		CreatedAt:           gw.CreatedAt,
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
type SensorResponse struct {
	ID        int64                          `json:"id"`
	GatewayID int64                          `json:"gatewayId"`
	SensorID  uuid.UUID                      `json:"sensorId"`
	Type      domain.SensorType              `json:"type"`
	MinRange  float64                        `json:"minRange"`
	MaxRange  float64                        `json:"maxRange"`
	Algorithm domain.GenerationAlgorithmType `json:"algorithm"`
	CreatedAt time.Time                      `json:"createdAt"`
}

func SensorFromDomain(s *domain.SimSensor) SensorResponse {
	return SensorResponse{
		ID:        s.ID,
		GatewayID: s.GatewayID,
		SensorID:  s.SensorID,
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
