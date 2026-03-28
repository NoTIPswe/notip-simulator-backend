package domain

import "errors"

var (
	ErrGatewayNotFound       = errors.New("gateway not found")
	ErrSensorNotFound        = errors.New("sensor not found")
	ErrGatewayAlreadyRunning = errors.New("gateway already running")
)
