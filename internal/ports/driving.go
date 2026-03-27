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
	Decommission(ctx context.Context, managementID uuid.UUID) error
	ListGateways(ctx context.Context) ([]*domain.SimGateway, error)
	GetGateway(ctx context.Context, managementID uuid.UUID) (*domain.SimGateway, error)
}

// SensorManagementService handles sensor CRUD on a running gateway worker.
// Called by SensorHandler.
type SensorManagementService interface {
	AddSensor(ctx context.Context, gatewayID int64, sensor domain.SimSensor) (*domain.SimSensor, error)
	ListSensors(ctx context.Context, gatewayID int64) ([]*domain.SimSensor, error)
	DeleteSensor(ctx context.Context, sensorID int64) error
}

// SimulatorControlService handles live config updates and anomaly injection.
// Called by AnomalyHandler and GatewayHandler (PATCH /config).
type SimulatorControlService interface {
	UpdateConfig(ctx context.Context, managementID uuid.UUID, update domain.GatewayConfigUpdate) error
	InjectGatewayAnomaly(ctx context.Context, managementID uuid.UUID, cmd domain.GatewayAnomalyCommand) error
	// InjectSensorOutlier resolves sensor→gateway internally; the adapter only provides the store ID.
	InjectSensorOutlier(ctx context.Context, sensorID int64, value *float64) error
}

type DecommissionEventReceiver interface {
	HandleDecommission(tenantID string, managementGatewayID string)
}
