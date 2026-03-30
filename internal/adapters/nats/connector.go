package nats

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type NATSMTLSConnector struct {
	natsURL string
	caPool  *x509.CertPool
	clock   ports.Nower
}

// NewNATSMTLSConnector reads and parses the CA certificate once at construction time.
// Subsequent Connect calls reuse the cached pool — no disk I/O per connection.
func NewNATSMTLSConnector(natsURL, caCertPath string, clock ports.Nower) (*NATSMTLSConnector, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &NATSMTLSConnector{
		natsURL: natsURL,
		caPool:  caPool,
		clock:   clock,
	}, nil
}

func (c *NATSMTLSConnector) Connect(ctx context.Context, certPEM []byte, keyPEM []byte, tenantID string, managementGatewayID uuid.UUID) (ports.GatewayPublisher, ports.CommandSubscription, error) {
	tlsCfg, err := c.buildTLSConfig(certPEM, keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("build TLS config: %w", err)
	}

	opts := []nats.Option{
		nats.Secure(tlsCfg),
		nats.MaxReconnects(60),
		nats.ReconnectWait(2 * time.Second),
		nats.ReconnectJitter(500*time.Millisecond, 2*time.Second),
	}

	nc, err := nats.Connect(c.natsURL, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("create JetStream context: %w", err)
	}

	pub, err := NewNATSGatewayPublisher(nc, c.natsURL, opts...)
	if err != nil {
		nc.Close()
		return nil, nil, err
	}

	sub, err := NewNATSGatewaySubscriber(js, tenantID, managementGatewayID.String(), pub, c.clock)
	if err != nil {
		_ = pub.Close()
		return nil, nil, fmt.Errorf("create subscriber: %w", err)
	}

	return pub, sub, nil
}

// buildTLSConfig constructs a mutual TLS config using the cached CA pool.
func (c *NATSMTLSConnector) buildTLSConfig(certPEM, keyPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse client certificate/key: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      c.caPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
