package nats

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
)

type NATSGatewayPublisher struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	servers string
	opts    []nats.Option
}

func NewNATSGatewayPublisher(nc *nats.Conn, servers string, opts ...nats.Option) (*NATSGatewayPublisher, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}
	return &NATSGatewayPublisher{nc: nc, js: js, servers: servers, opts: opts}, nil
}

func (p *NATSGatewayPublisher) Publish(ctx context.Context, subject string, payload []byte) error {
	_, err := p.js.Publish(subject, payload, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("publish message to %s: %w", subject, err)
	}
	return nil
}

func (p *NATSGatewayPublisher) Close() error {
	p.nc.Close()
	return nil
}

func (p *NATSGatewayPublisher) Reconnect(ctx context.Context) error {
	nc, err := nats.Connect(p.servers, p.opts...)
	if err != nil {
		return fmt.Errorf("reconnect to NATS: %w", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return fmt.Errorf("create JetStream context after reconnect: %w", err)
	}
	p.nc = nc
	p.js = js
	return nil
}
