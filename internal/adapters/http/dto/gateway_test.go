package dto_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http/dto"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

func sampleGateway() *domain.SimGateway {
	return &domain.SimGateway{
		ID:                  42,
		ManagementGatewayID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		FactoryID:           "factory-1",
		SerialNumber:        "SN-001",
		Model:               "ModelX",
		FirmwareVersion:     "1.2.3",
		Provisioned:         true,
		SendFrequencyMs:     500,
		Status:              domain.Running,
		TenantID:            "tenant-abc",
		CreatedAt:           time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
	}
}

func sampleSensor() *domain.SimSensor {
	return &domain.SimSensor{
		ID:        7,
		GatewayID: 42,
		SensorID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Type:      domain.Temperature,
		MinRange:  -10.0,
		MaxRange:  60.0,
		Algorithm: domain.UniformRandom,
		CreatedAt: time.Date(2024, 2, 1, 8, 0, 0, 0, time.UTC),
	}
}

func TestGatewayFromDomain(t *testing.T) {
	gw := sampleGateway()
	resp := dto.GatewayFromDomain(gw)

	if resp.ID != gw.ID {
		t.Errorf("ID: got %d, want %d", resp.ID, gw.ID)
	}
	if resp.ManagementGatewayID != gw.ManagementGatewayID {
		t.Errorf("ManagementGatewayID: got %v, want %v", resp.ManagementGatewayID, gw.ManagementGatewayID)
	}
	if resp.FactoryID != gw.FactoryID {
		t.Errorf("FactoryID: got %q, want %q", resp.FactoryID, gw.FactoryID)
	}
	if resp.SerialNumber != gw.SerialNumber {
		t.Errorf("SerialNumber: got %q, want %q", resp.SerialNumber, gw.SerialNumber)
	}
	if resp.Model != gw.Model {
		t.Errorf("Model: got %q, want %q", resp.Model, gw.Model)
	}
	if resp.FirmwareVersion != gw.FirmwareVersion {
		t.Errorf("FirmwareVersion: got %q, want %q", resp.FirmwareVersion, gw.FirmwareVersion)
	}
	if resp.Provisioned != gw.Provisioned {
		t.Errorf("Provisioned: got %v, want %v", resp.Provisioned, gw.Provisioned)
	}
	if resp.SendFrequencyMs != gw.SendFrequencyMs {
		t.Errorf("SendFrequencyMs: got %d, want %d", resp.SendFrequencyMs, gw.SendFrequencyMs)
	}
	if resp.Status != gw.Status {
		t.Errorf("Status: got %q, want %q", resp.Status, gw.Status)
	}
	if resp.TenantID != gw.TenantID {
		t.Errorf("TenantID: got %q, want %q", resp.TenantID, gw.TenantID)
	}
	if !resp.CreatedAt.Equal(gw.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", resp.CreatedAt, gw.CreatedAt)
	}
}

func TestGatewayListFromDomain_AllValid(t *testing.T) {
	gws := []*domain.SimGateway{sampleGateway(), sampleGateway()}
	result := dto.GatewayListFromDomain(gws)
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestGatewayListFromDomain_SkipsNil(t *testing.T) {
	gws := []*domain.SimGateway{sampleGateway(), nil, sampleGateway()}
	result := dto.GatewayListFromDomain(gws)
	if len(result) != 2 {
		t.Errorf("expected 2 results (nil skipped), got %d", len(result))
	}
}

func TestGatewayListFromDomain_Empty(t *testing.T) {
	result := dto.GatewayListFromDomain(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestSensorFromDomain(t *testing.T) {
	s := sampleSensor()
	resp := dto.SensorFromDomain(s)

	if resp.ID != s.ID {
		t.Errorf("ID: got %d, want %d", resp.ID, s.ID)
	}
	if resp.GatewayID != s.GatewayID {
		t.Errorf("GatewayID: got %d, want %d", resp.GatewayID, s.GatewayID)
	}
	if resp.SensorID != s.SensorID {
		t.Errorf("SensorID: got %v, want %v", resp.SensorID, s.SensorID)
	}
	if resp.Type != s.Type {
		t.Errorf("Type: got %q, want %q", resp.Type, s.Type)
	}
	if resp.MinRange != s.MinRange {
		t.Errorf("MinRange: got %f, want %f", resp.MinRange, s.MinRange)
	}
	if resp.MaxRange != s.MaxRange {
		t.Errorf("MaxRange: got %f, want %f", resp.MaxRange, s.MaxRange)
	}
	if resp.Algorithm != s.Algorithm {
		t.Errorf("Algorithm: got %q, want %q", resp.Algorithm, s.Algorithm)
	}
	if !resp.CreatedAt.Equal(s.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", resp.CreatedAt, s.CreatedAt)
	}
}

func TestSensorListFromDomain_AllValid(t *testing.T) {
	sensors := []*domain.SimSensor{sampleSensor(), sampleSensor()}
	result := dto.SensorListFromDomain(sensors)
	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestSensorListFromDomain_SkipsNil(t *testing.T) {
	sensors := []*domain.SimSensor{nil, sampleSensor(), nil}
	result := dto.SensorListFromDomain(sensors)
	if len(result) != 1 {
		t.Errorf("expected 1 result (nils skipped), got %d", len(result))
	}
}

func TestSensorListFromDomain_Empty(t *testing.T) {
	result := dto.SensorListFromDomain(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestCreateSensorRequest_ToDomain(t *testing.T) {
	req := dto.CreateSensorRequest{
		Type:      domain.Humidity,
		MinRange:  20.0,
		MaxRange:  80.0,
		Algorithm: domain.SineWave,
	}
	s := req.ToDomain()

	if s.Type != req.Type {
		t.Errorf("Type: got %q, want %q", s.Type, req.Type)
	}
	if s.MinRange != req.MinRange {
		t.Errorf("MinRange: got %f, want %f", s.MinRange, req.MinRange)
	}
	if s.MaxRange != req.MaxRange {
		t.Errorf("MaxRange: got %f, want %f", s.MaxRange, req.MaxRange)
	}
	if s.Algorithm != req.Algorithm {
		t.Errorf("Algorithm: got %q, want %q", s.Algorithm, req.Algorithm)
	}
	// Fields not set by ToDomain must be zero values
	if s.ID != 0 {
		t.Errorf("ID should be zero, got %d", s.ID)
	}
	if s.GatewayID != 0 {
		t.Errorf("GatewayID should be zero, got %d", s.GatewayID)
	}
}
