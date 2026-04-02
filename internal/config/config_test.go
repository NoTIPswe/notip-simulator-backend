package config_test

import (
	"os"
	"testing"

	"github.com/NoTIPswe/notip-simulator-backend/internal/config"
)

func setEnv(t *testing.T, pairs ...string) {
	t.Helper()
	for i := 0; i < len(pairs); i += 2 {
		t.Setenv(pairs[i], pairs[i+1])
	}
}

func requiredEnv(t *testing.T) {
	t.Helper()
	setEnv(t,
		"PROVISIONING_URL", "http://provisioning:3000",
		"NATS_URL", "nats://nats:4222",
		"NATS_CA_CERT_PATH", "/certs/ca.pem",
	)
}

func TestLoad_AllRequiredEnvSet_Success(t *testing.T) {
	requiredEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProvisioningURL != "http://provisioning:3000" {
		t.Errorf("ProvisioningURL mismatch: %s", cfg.ProvisioningURL)
	}
	if cfg.NATSUrl != "nats://nats:4222" {
		t.Errorf("NATSUrl mismatch: %s", cfg.NATSUrl)
	}
}

func TestLoad_Defaults_Applied(t *testing.T) {
	requiredEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SQLitePath == "" {
		t.Error("SQLitePath default should not be empty")
	}
	if cfg.HTTPAddr == "" {
		t.Error("HTTPAddr default should not be empty")
	}
	if cfg.DefaultSendFrequencyMs <= 0 {
		t.Errorf("DefaultSendFrequencyMs should be positive, got %d", cfg.DefaultSendFrequencyMs)
	}
	if cfg.GatewayBufferSize <= 0 {
		t.Errorf("GatewayBufferSize should be positive, got %d", cfg.GatewayBufferSize)
	}
}

func TestLoad_MissingProvisioningURL_ReturnsError(t *testing.T) {
	requiredEnv(t)
	_ = os.Unsetenv("PROVISIONING_URL")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when PROVISIONING_URL is missing")
	}
}

func TestLoad_MissingNATSUrl_ReturnsError(t *testing.T) {
	requiredEnv(t)
	_ = os.Unsetenv("NATS_URL")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when NATS_URL is missing")
	}
}

func TestLoad_MissingCACertPath_ReturnsError(t *testing.T) {
	requiredEnv(t)
	_ = os.Unsetenv("NATS_CA_CERT_PATH")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error when NATS_CA_CERT_PATH is missing")
	}
}

func TestLoad_RecoveryMode_DefaultFalse(t *testing.T) {
	requiredEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RecoveryMode != false {
		t.Error("RecoveryMode should default to false")
	}
}

func TestLoad_RecoveryMode_TrueWhenSet(t *testing.T) {
	requiredEnv(t)
	t.Setenv("RECOVERY_MODE", "true")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.RecoveryMode {
		t.Error("expected RecoveryMode to be true")
	}
}

func TestLoad_CustomHTTPAddr(t *testing.T) {
	requiredEnv(t)
	t.Setenv("HTTP_ADDR", ":9999")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Errorf("expected :9999, got %s", cfg.HTTPAddr)
	}
}

func TestLoad_CustomSendFrequency(t *testing.T) {
	requiredEnv(t)
	t.Setenv("DEFAULT_SEND_FREQUENCY_MS", "2500")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultSendFrequencyMs != 2500 {
		t.Errorf("expected 2500, got %d", cfg.DefaultSendFrequencyMs)
	}
}

func TestLoad_InvalidSendFrequency_UsesFallback(t *testing.T) {
	requiredEnv(t)
	t.Setenv("DEFAULT_SEND_FREQUENCY_MS", "0")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultSendFrequencyMs != 5000 {
		t.Errorf("expected fallback 5000, got %d", cfg.DefaultSendFrequencyMs)
	}
}

func TestLoad_NegativeSendFrequency_UsesFallback(t *testing.T) {
	requiredEnv(t)
	t.Setenv("DEFAULT_SEND_FREQUENCY_MS", "-1")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultSendFrequencyMs != 5000 {
		t.Errorf("expected fallback 5000, got %d", cfg.DefaultSendFrequencyMs)
	}
}

func TestLoad_InvalidBufferSize_UsesFallback(t *testing.T) {
	requiredEnv(t)
	t.Setenv("GATEWAY_BUFFER_SIZE", "0")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GatewayBufferSize != 1000 {
		t.Errorf("expected fallback 1000, got %d", cfg.GatewayBufferSize)
	}
}
