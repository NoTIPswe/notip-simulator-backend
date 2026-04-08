package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
	ProvisioningURL        string
	NATSUrl                string
	NATSCACertPath         string
	NATSTLSCertPath        string
	NATSTLSKeyPath         string
	SQLitePath             string
	HTTPAddr               string
	DefaultSendFrequencyMs int
	GatewayBufferSize      int
	MetricsAddr            string
	RecoveryMode           bool
}

func Load() (*Config, error) {
	cfg := &Config{
		SQLitePath:      getEnv("SQLITE_PATH", "/data/simulator.db"),
		HTTPAddr:        getEnv("HTTP_ADDR", ":8090"),
		MetricsAddr:     getEnv("METRICS_ADDR", ":9090"),
		ProvisioningURL: getEnv("PROVISIONING_URL", ""),
		NATSUrl:         getEnv("NATS_URL", ""),
		NATSCACertPath:  getEnv("NATS_CA_CERT_PATH", ""),
		NATSTLSCertPath: getEnv("NATS_TLS_CERT", ""),
		NATSTLSKeyPath:  getEnv("NATS_TLS_KEY", ""),
	}

	var errs []error

	if cfg.ProvisioningURL == "" {
		errs = append(errs, errors.New("PROVISIONING_URL is required"))
	}
	if cfg.NATSUrl == "" {
		errs = append(errs, errors.New("NATS_URL is required"))
	}
	if cfg.NATSCACertPath == "" {
		errs = append(errs, errors.New("NATS_CA_CERT_PATH is required"))
	}
	if (cfg.NATSTLSCertPath == "") != (cfg.NATSTLSKeyPath == "") {
		errs = append(errs, errors.New("NATS_TLS_CERT and NATS_TLS_KEY must both be set or both be empty"))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	cfg.DefaultSendFrequencyMs = getEnvInt("DEFAULT_SEND_FREQUENCY_MS", 5000)
	cfg.GatewayBufferSize = getEnvInt("GATEWAY_BUFFER_SIZE", 1000)
	cfg.RecoveryMode = getEnvBool("RECOVERY_MODE", false)

	if cfg.DefaultSendFrequencyMs <= 0 {
		slog.Warn("DEFAULT_SEND_FREQUENCY_MS must be > 0, using fallback", "value", cfg.DefaultSendFrequencyMs, "fallback", 5000)
		cfg.DefaultSendFrequencyMs = 5000
	}

	if cfg.GatewayBufferSize <= 0 {
		slog.Warn("GATEWAY_BUFFER_SIZE must be > 0, using fallback", "value", cfg.GatewayBufferSize, "fallback", 1000)
		cfg.GatewayBufferSize = 1000
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("invalid value for env var, using fallback", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return i
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		slog.Warn("invalid value for env var, using fallback", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return b
}
