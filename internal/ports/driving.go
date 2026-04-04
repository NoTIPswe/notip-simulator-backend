package ports

import (
	"context"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/google/uuid"
)

// GatewayLifecycleService owns gateway creation, bulk creation, and lifecycle state transitions.
// Called by GatewayHandler.
type GatewayLifecycleService interface {
	CreateAndStart(ctx context.Context, req domain.CreateGatewayRequest) (*domain.SimGateway, error)
	BulkCreateGateways(ctx context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error)
	Start(ctx context.Context, managementID uuid.UUID) error
	Stop(ctx context.Context, managementID uuid.UUID) error
	Delete(ctx context.Context, managementID uuid.UUID) error
	ListGateways(ctx context.Context) ([]*domain.SimGateway, error)
	GetGateway(ctx context.Context, managementID uuid.UUID) (*domain.SimGateway, error)
}

// SensorManagementService handles sensor CRUD on a running gateway worker.
// Called by SensorHandler.
type SensorManagementService interface {
	AddSensor(ctx context.Context, managementGatewayID uuid.UUID, sensor domain.SimSensor) (*domain.SimSensor, error)
	ListSensors(ctx context.Context, managementGatewayID uuid.UUID) ([]*domain.SimSensor, error)
	DeleteSensor(ctx context.Context, sensorID uuid.UUID) error
}

// SimulatorControlService handles live config updates and anomaly injection.
// Called by AnomalyHandler and GatewayHandler (PATCH /config).
type SimulatorControlService interface {
	UpdateConfig(ctx context.Context, managementID uuid.UUID, update domain.GatewayConfigUpdate) error
	InjectGatewayAnomaly(ctx context.Context, managementID uuid.UUID, cmd domain.GatewayAnomalyCommand) error
	InjectSensorOutlier(ctx context.Context, sensorID uuid.UUID, value *float64) error
}

type DecommissionEventReceiver interface {
	HandleDecommission(tenantID, managementGatewayID string)
}
