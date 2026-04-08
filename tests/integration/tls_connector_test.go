//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
	natsadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/nats"
)

// TestGatewayConnector_MTLSConnects_VerificaTLSSuccesso verifies the encrypted TLS transmission requirement.
// It uses on-the-fly certificates to connect to a secured NATS broker.
func TestGatewayConnectorMTLSConnectsVerificaTLSSuccesso(t *testing.T) {
	// Start the NATS container with forced mTLS.
	env := setupSecureNATS(t)
	ctx := context.Background()

	nc, err := nats.Connect(
		env.URI,
		nats.RootCAs(env.Certs.CACert),
		nats.ClientCert(env.Certs.ClientCert, env.Certs.ClientKey),
	)
	require.NoError(t, err)
	js, err := nc.JetStream()
	require.NoError(t, err)

	// Stream catch-all for the commands in this test.
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "TEST_COMMANDS",
		Subjects: []string{"command.gw.>"},
	})
	require.NoError(t, err)
	nc.Close()

	// Instantiate the production NATS connector.
	connector, err := natsadapter.NewNATSMTLSConnector(env.URI, env.Certs.CACert, adapters.SystemClock{})
	require.NoError(t, err)

	tenantID := "tenant-tls-1"
	gatewayID := uuid.New()

	// Test the connection using PEM certificate bytes.
	pub, sub, closeFn, err := connector.Connect(
		ctx,
		env.Certs.ClientDER,  // PEM certificate.
		env.Certs.ClientPriv, // PEM private key.
		tenantID,
		gatewayID,
	)

	// Ensure the connection is successful.
	require.NoError(t, err, "TLS connection must succeed with valid certificates.")
	require.NotNil(t, pub)
	require.NotNil(t, sub)

	t.Cleanup(func() { _ = closeFn() })
}

// TestGatewayConnector_PlaintextRejected_VerificaRifiutoSenzaTLS demonstrates that the broker rejects unencrypted communications.
func TestGatewayConnectorPlaintextRejectedVerificaRifiutoSenzaTLS(t *testing.T) {
	env := setupSecureNATS(t)
	ctx := context.Background()

	// Instantiate the production NATS connector.
	connector, err := natsadapter.NewNATSMTLSConnector(env.URI, env.Certs.CACert, adapters.SystemClock{})
	require.NoError(t, err)

	tenantID := "tenant-tls-2"
	gatewayID := uuid.New()

	// Attempt to connect without passing certificates.
	_, _, _, err = connector.Connect(
		ctx,
		nil, // No certificate.
		nil, // No private key.
		tenantID,
		gatewayID,
	)

	// The test must fail because the NATS server requires TLS.
	require.Error(t, err, "Connection must be rejected due to missing TLS certificates.")
}
