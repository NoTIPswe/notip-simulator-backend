package domain

import "errors"

var (
	ErrGatewayNotFound           = errors.New("gateway not found")
	ErrSensorNotFound            = errors.New("sensor not found")
	ErrGatewayAlreadyRunning     = errors.New("gateway already running")
	ErrGatewayAlreadyProvisioned = errors.New("gateway already provisioned")
	ErrInvalidFactoryCredentials = errors.New("invalid factory credentials")
	ErrInvalidSensorRange        = errors.New("invalid sensor range")
)
