package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters"
	httpadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
	natsadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/nats"
	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/sqlite"
	"github.com/NoTIPswe/notip-simulator-backend/internal/config"
	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"

	natsio "github.com/nats-io/nats.go"
)

const (
	serverShutdownTimeout = 10 * time.Second
	workerStopTimeout     = 5 * time.Second
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	store, err := setupDatabase(ctx, cfg.SQLitePath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	met := metrics.NewMetrics()
	clock := adapters.SystemClock{}

	connector, err := natsadapter.NewNATSMTLSConnector(cfg.NATSUrl, cfg.NATSCACertPath, clock)
	if err != nil {
		return fmt.Errorf("create NATS connector: %w", err)
	}

	registry := NewGatewayRegistry(
		store,
		httpadapter.NewProvisioningServiceClient(cfg.ProvisioningURL),
		connector,
		adapters.AESGCMEncryptor{},
		clock,
		cfg,
		met,
	)

	globalNats, err := setupDecommissionListener(ctx, cfg, registry)
	if err != nil {
		return err
	}
	defer globalNats.Close()

	if cfg.RecoveryMode {
		slog.Info("RecoveryMode is enabled, restoring provisioned gateways...")
		if err := registry.RestoreAll(ctx); err != nil {
			slog.Error("Failed to restore some or all gateways", "err", err)
		}
	} else {
		slog.Info("RecoveryMode is disabled, skipping SQLite state hydration.")
	}

	serverErr := make(chan error, 2)
	metricsServer := startMetricsServer(cfg.MetricsAddr, serverErr)
	apiServer := startAPIServer(cfg, registry, serverErr)

	return handleShutdown(ctx, apiServer, metricsServer, registry, serverErr)
}

func setupDatabase(ctx context.Context, path string) (*sqlite.SQLiteGatewayStore, error) {
	store, err := sqlite.NewStore(path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite store: %w", err)
	}
	if err := store.RunMigrations(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("run sqlite migrations: %w", err)
	}
	return store, nil
}

func setupDecommissionListener(ctx context.Context, cfg *config.Config, registry *GatewayRegistry) (*natsio.Conn, error) {
	caCert, err := os.ReadFile(cfg.NATSCACertPath)
	if err != nil {
		return nil, fmt.Errorf("read nats ca cert: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse global NATS ca cert")
	}

	globalNats, err := natsio.Connect(cfg.NATSUrl,
		natsio.Secure(&tls.Config{
			RootCAs:    caPool,
			MinVersion: tls.VersionTLS13,
		}),
		natsio.MaxReconnects(-1),
		natsio.ReconnectWait(2*time.Second),
		natsio.ReconnectJitter(500*time.Millisecond, 2*time.Second),
		natsio.PingInterval(20*time.Second),
		natsio.DisconnectErrHandler(func(_ *natsio.Conn, err error) {
			slog.Warn("global NATS disconnected", "error", err)
		}),
		natsio.ReconnectHandler(func(_ *natsio.Conn) {
			slog.Info("global NATS reconnected")
		}),
		natsio.ClosedHandler(func(_ *natsio.Conn) {
			slog.Error("global NATS connection permanently closed")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("global nats connect: %w", err)
	}

	js, err := globalNats.JetStream()
	if err != nil {
		globalNats.Close()
		return nil, fmt.Errorf("global jetstream context: %w", err)
	}

	listener := natsadapter.NewNATSDecommissionListener(js, registry)
	go func() {
		slog.Info("Starting NATS Decommission Listener...")
		if err := listener.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Decommission listener stopped with error", "err", err)
		}
	}()

	return globalNats, nil
}

func startMetricsServer(addr string, errCh chan<- error) *nethttp.Server {
	mux := nethttp.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &nethttp.Server{Addr: addr, Handler: mux}

	go func() {
		slog.Info("Starting metrics server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			errCh <- fmt.Errorf("metrics server error: %w", err)
		}
	}()
	return srv
}

func startAPIServer(cfg *config.Config, registry *GatewayRegistry, errCh chan<- error) *httpadapter.HTTPServer {
	gwHandler := httpadapter.NewGatewayHandler(registry)
	sensorHandler := httpadapter.NewSensorHandler(registry)
	anomalyHandler := httpadapter.NewAnomalyHandler(registry)

	srv := httpadapter.NewHTTPServer(
		cfg.HTTPAddr,
		gwHandler,
		sensorHandler,
		anomalyHandler,
	)

	go func() {
		slog.Info("Starting HTTP API server", "addr", cfg.HTTPAddr)
		if err := srv.Start(); err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			errCh <- fmt.Errorf("http api server: %w", err)
		}
	}()
	return srv
}

func handleShutdown(ctx context.Context, apiSrv *httpadapter.HTTPServer, metSrv *nethttp.Server, registry *GatewayRegistry, errCh <-chan error) error {
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("Shutdown signal received, initiating graceful shutdown")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		defer cancel()

		if err := apiSrv.Stop(shutdownCtx); err != nil {
			slog.Error("API server shutdown error", "err", err)
		}
		if err := metSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("Metrics server shutdown error", "err", err)
		}

		slog.Info("Stopping all gateway workers...")
		registry.StopAll(workerStopTimeout)

		slog.Info("Graceful shutdown completed")
		return nil
	}
}
