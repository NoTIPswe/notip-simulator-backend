//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	natsmodule "github.com/testcontainers/testcontainers-go/modules/nats"

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
	ctx := context.Background()

	c, err := natsmodule.RunContainer(ctx,
		testcontainers.WithImage("nats:2.10-alpine"),
	)
	require.NoError(t, err, "start NATS container")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Terminate(ctx)
	})

	uri, err := c.ConnectionString(ctx)
	require.NoError(t, err, "get NATS connection string")

	return &natsEnv{URI: uri}
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
