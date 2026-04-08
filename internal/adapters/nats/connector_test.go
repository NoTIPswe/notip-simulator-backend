package nats

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
)

const (
	tlsLoopbackAddr  = "tls://127.0.0.1:4222"
	errExpectedNil   = "expected error, got nil"
	caPEMFilename    = "ca.pem"
	unexpectedErrFmt = "unexpected error: %v"
)

func writeTempFile(t *testing.T, path string, data []byte) {
	t.Helper()
	err := os.WriteFile(path, data, 0o600)
	if err != nil {
		t.Fatalf("write temp file: %v", err)
	}
}

func generateCAAndClientPEM(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "connector-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "connector-test-client"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTmpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client certificate: %v", err)
	}

	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})

	return caPEM, clientCertPEM, clientKeyPEM
}

func TestNewNATSMTLSConnectorReadCAError(t *testing.T) {
	_, err := NewNATSMTLSConnector(tlsLoopbackAddr, "/tmp/notip-missing-ca.pem", "", "", adapters.SystemClock{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "read CA certificate") {
		t.Fatalf("expected read CA error, got: %v", err)
	}
}

func TestNewNATSMTLSConnectorInvalidCAPEM(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, caPEMFilename)
	writeTempFile(t, caPath, []byte("not-a-certificate"))

	_, err := NewNATSMTLSConnector(tlsLoopbackAddr, caPath, "", "", adapters.SystemClock{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "failed to parse CA certificate") {
		t.Fatalf("expected parse CA error, got: %v", err)
	}
}

func TestNewNATSMTLSConnectorStaticMTLSSuccess(t *testing.T) {
	dir := t.TempDir()
	caPEM, clientCertPEM, clientKeyPEM := generateCAAndClientPEM(t)

	caPath := filepath.Join(dir, caPEMFilename)
	certPath := filepath.Join(dir, "client-cert.pem")
	keyPath := filepath.Join(dir, "client-key.pem")

	writeTempFile(t, caPath, caPEM)
	writeTempFile(t, certPath, clientCertPEM)
	writeTempFile(t, keyPath, clientKeyPEM)

	c, err := NewNATSMTLSConnector(tlsLoopbackAddr, caPath, certPath, keyPath, adapters.SystemClock{})
	if err != nil {
		t.Fatalf(unexpectedErrFmt, err)
	}
	if !c.useStaticMTLS {
		t.Fatal("expected static mTLS to be enabled")
	}
	if c.staticCert == nil {
		t.Fatal("expected static certificate to be loaded")
	}
}

func TestNewNATSMTLSConnectorStaticMTLSReadError(t *testing.T) {
	dir := t.TempDir()
	caPEM, _, _ := generateCAAndClientPEM(t)

	caPath := filepath.Join(dir, caPEMFilename)
	writeTempFile(t, caPath, caPEM)

	_, err := NewNATSMTLSConnector(tlsLoopbackAddr, caPath, "/tmp/notip-missing-client-cert.pem", "/tmp/notip-missing-client-key.pem", adapters.SystemClock{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "read static client certificate") {
		t.Fatalf("expected static cert read error, got: %v", err)
	}
}

func TestNewNATSMTLSConnectorStaticMTLSParseError(t *testing.T) {
	dir := t.TempDir()
	caPEM, clientCertPEM, _ := generateCAAndClientPEM(t)

	caPath := filepath.Join(dir, caPEMFilename)
	certPath := filepath.Join(dir, "client-cert.pem")
	keyPath := filepath.Join(dir, "client-key.pem")

	writeTempFile(t, caPath, caPEM)
	writeTempFile(t, certPath, clientCertPEM)
	writeTempFile(t, keyPath, []byte("invalid-key"))

	_, err := NewNATSMTLSConnector(tlsLoopbackAddr, caPath, certPath, keyPath, adapters.SystemClock{})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "parse static client certificate/key") {
		t.Fatalf("expected static cert parse error, got: %v", err)
	}
}

func TestBuildTLSConfigDynamicError(t *testing.T) {
	c := &NATSMTLSConnector{caPool: x509.NewCertPool(), useStaticMTLS: false}

	_, err := c.buildTLSConfig(nil, nil)
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "parse client certificate/key") {
		t.Fatalf("expected dynamic parse error, got: %v", err)
	}
}

func TestBuildTLSConfigStaticNotLoadedError(t *testing.T) {
	c := &NATSMTLSConnector{caPool: x509.NewCertPool(), useStaticMTLS: true, staticCert: nil}

	_, err := c.buildTLSConfig(nil, nil)
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "static client certificate is enabled but not loaded") {
		t.Fatalf(unexpectedErrFmt, err)
	}
}

func TestBuildTLSConfigStaticSuccess(t *testing.T) {
	_, clientCertPEM, clientKeyPEM := generateCAAndClientPEM(t)
	cert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatalf("parse cert pair: %v", err)
	}

	c := &NATSMTLSConnector{caPool: x509.NewCertPool(), useStaticMTLS: true, staticCert: &cert}

	tlsCfg, err := c.buildTLSConfig(nil, nil)
	if err != nil {
		t.Fatalf(unexpectedErrFmt, err)
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS1.3 min version, got %d", tlsCfg.MinVersion)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Fatalf("expected one certificate, got %d", len(tlsCfg.Certificates))
	}
}

func TestConnectBuildTLSError(t *testing.T) {
	dir := t.TempDir()
	caPEM, _, _ := generateCAAndClientPEM(t)
	caPath := filepath.Join(dir, caPEMFilename)
	writeTempFile(t, caPath, caPEM)

	c, err := NewNATSMTLSConnector(tlsLoopbackAddr, caPath, "", "", adapters.SystemClock{})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	_, _, _, err = c.Connect(context.Background(), nil, nil, "tenant", uuid.New())
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "build TLS config") {
		t.Fatalf("expected TLS config error, got: %v", err)
	}
}

func TestConnectNATSError(t *testing.T) {
	dir := t.TempDir()
	caPEM, clientCertPEM, clientKeyPEM := generateCAAndClientPEM(t)
	caPath := filepath.Join(dir, caPEMFilename)
	writeTempFile(t, caPath, caPEM)

	c, err := NewNATSMTLSConnector("tls://127.0.0.1:1", caPath, "", "", adapters.SystemClock{})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, _, _, err = c.Connect(ctx, clientCertPEM, clientKeyPEM, "tenant", uuid.New())
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), "connect to NATS") {
		t.Fatalf("expected NATS connect error, got: %v", err)
	}
}
