package nats

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
)

type NATSGatewayPublisher struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

func NewNATSGatewayPublisher(nc *nats.Conn) (*NATSGatewayPublisher, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}
	return &NATSGatewayPublisher{nc: nc, js: js}, nil
}

func (p *NATSGatewayPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	_, err := p.js.Publish(subject, payload)
	if err != nil {
		return fmt.Errorf("publish message to %s: %w", subject, err)
	}
	return nil
}

func (p *NATSGatewayPublisher) Close() error {
	p.nc.Close()
	return nil
}
