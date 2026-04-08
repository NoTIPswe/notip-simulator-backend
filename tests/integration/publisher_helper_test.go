//go:build integration

package integration

import (
	"context"

	"github.com/nats-io/nats.go"
)

// realPublisher is a lightweight ports.GatewayPublisher implementation backed by a plain NATS connection.
// Some integration tests use it where full JetStream publisher behavior is not required.
type realPublisher struct {
	nc *nats.Conn
}

func (p *realPublisher) Publish(_ context.Context, subject string, payload []byte) error {
	return p.nc.Publish(subject, payload)
}

func (p *realPublisher) Close() error {
	if !p.nc.IsClosed() {
		p.nc.Close()
	}
	return nil
}

func (p *realPublisher) Reconnect(_ context.Context) error {
	return nil
}
