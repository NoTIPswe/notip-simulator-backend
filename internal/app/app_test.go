package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	nethttp "net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
	"github.com/NoTIPswe/notip-simulator-backend/internal/config"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

func writeTempCACert(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	caPath := filepath.Join(t.TempDir(), "ca.pem")
	f, err := os.Create(caPath)
	if err != nil {
		t.Fatalf("create ca file: %v", err)
	}
	defer func() { _ = f.Close() }()

	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encode certificate pem: %v", err)
	}

	return caPath
}

func newHTTPServerForAppTests(addr string) *httpadapter.HTTPServer {
	gw := httpadapter.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	sensor := httpadapter.NewSensorHandler(&fakes.FakeSensorManagementService{})
	anomaly := httpadapter.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	return httpadapter.NewHTTPServer(addr, "", gw, sensor, anomaly)
}

func TestSetupDatabaseSuccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := setupDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("unexpected setupDatabase error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	_ = store.Close()
}

func TestSetupDatabaseOpenFails(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "missing-dir", "test.db")
	_, err := setupDatabase(context.Background(), badPath)
	if err == nil {
		t.Fatal("expected setupDatabase to fail when parent directory does not exist")
	}
}

func TestSetupDecommissionListenerReadCACertFails(t *testing.T) {
	cfg := &config.Config{
		NATSCACertPath: "/definitely/missing/ca.pem",
		NATSUrl:        "nats://127.0.0.1:4222",
	}

	_, err := setupDecommissionListener(context.Background(), cfg, newTestRegistry(newTestDeps()))
	if err == nil {
		t.Fatal("expected error when CA cert path is invalid")
	}
}

func TestSetupDecommissionListenerParseCACertFails(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, []byte("not-a-cert"), 0o600); err != nil {
		t.Fatalf("write temp cert file: %v", err)
	}

	cfg := &config.Config{
		NATSCACertPath: caPath,
		NATSUrl:        "nats://127.0.0.1:4222",
	}

	_, err := setupDecommissionListener(context.Background(), cfg, newTestRegistry(newTestDeps()))
	if err == nil {
		t.Fatal("expected parse CA cert error")
	}
	if !strings.Contains(err.Error(), "failed to parse global NATS ca cert") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetupDecommissionListenerConnectFails(t *testing.T) {
	caPath := writeTempCACert(t)

	cfg := &config.Config{
		NATSCACertPath: caPath,
		NATSUrl:        "nats://127.0.0.1:1",
	}

	_, err := setupDecommissionListener(context.Background(), cfg, newTestRegistry(newTestDeps()))
	if err == nil {
		t.Fatal("expected connect failure with unreachable NATS")
	}
	if !strings.Contains(err.Error(), "global nats connect") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartMetricsServerInvalidAddrSendsError(t *testing.T) {
	errCh := make(chan error, 1)
	srv := startMetricsServer("bad-addr", errCh)
	if srv == nil {
		t.Fatal("expected non-nil metrics server")
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected non-nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected metrics server error for invalid address")
	}
}

func TestStartAPIServerInvalidAddrSendsError(t *testing.T) {
	errCh := make(chan error, 1)
	cfg := &config.Config{HTTPAddr: "bad-addr"}
	reg := newTestRegistry(newTestDeps())

	srv := startAPIServer(cfg, reg, errCh)
	if srv == nil {
		t.Fatal("expected non-nil api server")
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected non-nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected api server error for invalid address")
	}
}

func TestHandleShutdownReturnsServerError(t *testing.T) {
	errCh := make(chan error, 1)
	want := errors.New("server failed")
	errCh <- want

	err := handleShutdown(context.Background(), newHTTPServerForAppTests("127.0.0.1:0"), &nethttp.Server{}, newTestRegistry(newTestDeps()), errCh)
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestHandleShutdownContextCanceledGraceful(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := handleShutdown(ctx, newHTTPServerForAppTests("127.0.0.1:0"), &nethttp.Server{}, newTestRegistry(newTestDeps()), make(chan error))
	if err != nil {
		t.Fatalf("expected nil error on graceful shutdown, got %v", err)
	}
}

func TestRunConfigLoadFails(t *testing.T) {
	t.Setenv("PROVISIONING_URL", "")
	t.Setenv("NATS_URL", "")
	t.Setenv("NATS_CA_CERT_PATH", "")

	err := Run(context.Background())
	if err == nil {
		t.Fatal("expected run to fail when required config is missing")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSetupDatabaseFails(t *testing.T) {
	t.Setenv("PROVISIONING_URL", "http://provisioning.local")
	t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
	t.Setenv("NATS_CA_CERT_PATH", "/tmp/ca.pem")
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "missing-dir", "sim.db"))

	err := Run(context.Background())
	if err == nil {
		t.Fatal("expected run to fail when sqlite path parent does not exist")
	}
	if !strings.Contains(err.Error(), "run sqlite migrations") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCreateNATSConnectorFails(t *testing.T) {
	t.Setenv("PROVISIONING_URL", "http://provisioning.local")
	t.Setenv("NATS_URL", "nats://127.0.0.1:4222")
	t.Setenv("NATS_CA_CERT_PATH", "/definitely/missing/ca.pem")
	t.Setenv("SQLITE_PATH", filepath.Join(t.TempDir(), "sim.db"))

	err := Run(context.Background())
	if err == nil {
		t.Fatal("expected run to fail when NATS connector cannot load CA")
	}
	if !strings.Contains(err.Error(), "create NATS connector") {
		t.Fatalf("unexpected error: %v", err)
	}
}
