package nats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type NATSGatewayPublisher struct {
	mu      sync.RWMutex
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
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, err := p.js.Publish(subject, payload, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("publish message to %s: %w", subject, err)
	}
	return nil
}

// Close drops the publisher's references. It does NOT close the underlying
// nats.Conn — that is the responsibility of the component that created it
// (the GatewayConnector), which returns a dedicated closer.
func (p *NATSGatewayPublisher) Close() error {
	p.mu.Lock()
	p.nc = nil
	p.js = nil
	p.mu.Unlock()
	return nil
}

func (p *NATSGatewayPublisher) Reconnect(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	opts := append([]nats.Option{}, p.opts...)
	if deadline, ok := ctx.Deadline(); ok {
		timeout := time.Until(deadline)
		if timeout <= 0 {
			return context.DeadlineExceeded
		}
		opts = append(opts, nats.Timeout(timeout))
	}

	nc, err := nats.Connect(p.servers, opts...)
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
