//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/sqlite"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

// ─────────────────────────────────────────────────────────────────────────────
// NATS container helpers
// ─────────────────────────────────────────────────────────────────────────────

type natsEnv struct {
	URI string
}

// startNATS spins up a real NATS 2.10 container and returns its URI.
// The container is terminated automatically via t.Cleanup.
func startNATS(t *testing.T) *natsEnv {
	t.Helper()
	return &natsEnv{URI: sharedPlainNATSURI}
}

// connectNATS opens a plain (non-mTLS) NATS connection to the test container.
// The connection is closed automatically via t.Cleanup.
func connectNATS(t *testing.T, uri string) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(uri,
		nats.Timeout(5*time.Second),
		nats.MaxReconnects(5),
		nats.ReconnectWait(200*time.Millisecond),
	)
	require.NoError(t, err, "connect to NATS")
	t.Cleanup(func() { nc.Drain() }) //nolint:errcheck
	return nc
}

// ─────────────────────────────────────────────────────────────────────────────
// SQLite store helper
// ─────────────────────────────────────────────────────────────────────────────

// newSQLiteStore creates a real on-disk SQLite store in a temp directory.
// Migrations are applied automatically. Store is closed via t.Cleanup.
func newSQLiteStore(t *testing.T) *sqlite.SQLiteGatewayStore {
	t.Helper()
	dir := t.TempDir()
	store, err := sqlite.NewStore(filepath.Join(dir, "test.db"))
	require.NoError(t, err, "open SQLite store")
	require.NoError(t, store.RunMigrations(context.Background()), "run migrations")
	t.Cleanup(func() { store.Close() })
	return store
}

// ─────────────────────────────────────────────────────────────────────────────
// Domain fixture helpers
// ─────────────────────────────────────────────────────────────────────────────

// validAESKey returns a domain.EncryptionKey backed by 32 zero bytes.
func validAESKey(t *testing.T) domain.EncryptionKey {
	t.Helper()
	key, err := domain.NewEncryptionKey(make([]byte, 32))
	require.NoError(t, err)
	return key
}

var sharedPlainNATSURI string

// TestMain executed one time before all the package's tests.
func TestMain(m *testing.M) {
	ctx := context.Background()

	// Single NATS container for all the tests.
	natsC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nats:latest",
			ExposedPorts: []string{"4222/tcp"},
			Cmd:          []string{"-js"}, // Abilita JetStream
			WaitingFor:   wait.ForLog("Server is ready"),
		},
		Started: true,
	})
	if err != nil {
		fmt.Printf("Impossible to boot the shared NATS container: %v\n", err)
		os.Exit(1)
	}

	// Ip and Port.
	host, _ := natsC.Host(ctx)
	port, _ := natsC.MappedPort(ctx, "4222/tcp")
	sharedPlainNATSURI = fmt.Sprintf("nats://%s:%s", host, port.Port())

	// Execute the tests.
	exitCode := m.Run()

	//Clean the container.
	_ = natsC.Terminate(ctx)

	os.Exit(exitCode)
}

// Tls Helper.
type testCerts struct {
	CACert     string
	ServerCert string
	ServerKey  string
	ClientCert string // .pem file's path
	ClientKey  string // .pem key's path.
	ClientDER  []byte // Raw certificate.
	ClientPriv []byte // Raw Key.
}

func generateCerts(dir string, extraIPs []net.IP) (*testCerts, error) {
	// CA.
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "notip-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}

	// Server cert.
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "nats-server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  append([]net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback}, extraIPs...),
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	// Client cert (the CN should match the nats certificaion).
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "gateway-simulator"}, // Nome adattato per il tuo scope
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	// Prepare structs.
	c := &testCerts{
		CACert:     filepath.Join(dir, "ca.pem"),
		ServerCert: filepath.Join(dir, "server-cert.pem"),
		ServerKey:  filepath.Join(dir, "server-key.pem"),
		ClientCert: filepath.Join(dir, "client-cert.pem"),
		ClientKey:  filepath.Join(dir, "client-key.pem"),
	}

	// Save on file for Nats.
	_ = writeCertPEM(c.CACert, caDER)
	_ = writeCertPEM(c.ServerCert, serverDER)
	_ = writeECKeyPEM(c.ServerKey, serverKey)
	_ = writeCertPEM(c.ClientCert, clientDER)
	_ = writeECKeyPEM(c.ClientKey, clientKey)

	// Save the raw PEMs
	c.ClientDER = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	clientKeyDER, _ := x509.MarshalECPrivateKey(clientKey)
	c.ClientPriv = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	return c, nil
}

func writeCertPEM(path string, der []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writeECKeyPEM(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

// ─────────────────────────────────────────────────────────────────────────────
// Setup Testcontainers NATS Secure.
// ─────────────────────────────────────────────────────────────────────────────

const natsSecureConf = `
listen: "0.0.0.0:4222"

jetstream {
  store_dir: "/data"
}

tls {
  ca_file:        "/certs/ca.pem"
  cert_file:      "/certs/server-cert.pem"
  key_file:       "/certs/server-key.pem"
  verify:         true
  verify_and_map: true
}
authorization {
  default_permissions: {
    publish:   { deny: [">"] }
    subscribe: { deny: [">"] }
  }
  users = [
    { user: "CN=gateway-simulator", permissions: { publish: { allow: [">"] }, subscribe: { allow: [">"] } } }
  ]
}
`

type secureNATSEnv struct {
	URI   string
	Certs *testCerts
}

func setupSecureNATS(t *testing.T) *secureNATSEnv {
	t.Helper()
	ctx := context.Background()

	certDir := t.TempDir()

	// Ip for the certificates.
	var extraIPs []net.IP
	provider, _ := testcontainers.NewDockerProvider()
	if dockerHost, err := provider.DaemonHost(ctx); err == nil {
		if ip := net.ParseIP(dockerHost); ip != nil {
			extraIPs = append(extraIPs, ip)
		}
	}
	for i := 16; i <= 31; i++ {
		extraIPs = append(extraIPs, net.ParseIP(fmt.Sprintf("172.%d.0.1", i)))
	}

	certs, err := generateCerts(certDir, extraIPs)
	require.NoError(t, err)

	confPath := filepath.Join(certDir, "nats.conf")
	err = os.WriteFile(confPath, []byte(natsSecureConf), 0o600)
	require.NoError(t, err)

	natsC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "nats:latest",
			ExposedPorts: []string{"4222/tcp"},
			Cmd:          []string{"-c", "/etc/nats/nats.conf"},
			Files: []testcontainers.ContainerFile{
				{HostFilePath: confPath, ContainerFilePath: "/etc/nats/nats.conf", FileMode: 0o644},
				{HostFilePath: certs.CACert, ContainerFilePath: "/certs/ca.pem", FileMode: 0o644},
				{HostFilePath: certs.ServerCert, ContainerFilePath: "/certs/server-cert.pem", FileMode: 0o644},
				{HostFilePath: certs.ServerKey, ContainerFilePath: "/certs/server-key.pem", FileMode: 0o600},
			},
			WaitingFor: wait.ForLog("Server is ready"),
		},
		Started: true,
	})
	require.NoError(t, err)

	t.Cleanup(func() { _ = natsC.Terminate(ctx) })

	natsHost, err := natsC.Host(ctx)
	require.NoError(t, err)
	natsPort, err := natsC.MappedPort(ctx, "4222/tcp")
	require.NoError(t, err)

	return &secureNATSEnv{
		URI:   fmt.Sprintf("tls://%s:%s", natsHost, natsPort.Port()),
		Certs: certs,
	}
}
