package nats

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

type NATSMTLSConnector struct {
	natsURL    string
	caCertPath string
	clock      ports.Clock
}

func NewNATSMTLSConnector(natsURL, caCertPath string, clock ports.Clock) *NATSMTLSConnector {
	return &NATSMTLSConnector{
		natsURL:    natsURL,
		caCertPath: caCertPath,
		clock:      clock,
	}
}

func (c *NATSMTLSConnector) Connect(ctx context.Context, certPEM []byte, keyPEM []byte, tenantID string, managementGatewayID uuid.UUID) (ports.GatewayPublisher, ports.CommandSubscription, error) {
	tlsCfg, err := buildTLSConfig(c.caCertPath, certPEM, keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("build TLS config: %w", err)
	}

	opts := []nats.Option{
		nats.Secure(tlsCfg),
		nats.MaxReconnects(-1),
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

// First it reads and parses the CA certificate, then it builds the client certificate and key, and finally it constructs a tls.Config with the appropriate settings for mutual TLS authentication.
func buildTLSConfig(caCertPath string, certPEM, keyPEM []byte) (*tls.Config, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse ca certificate")
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse client certificate/key: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
	}, nil
}
