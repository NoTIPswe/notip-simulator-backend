package app

import (
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/config"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
)

type testDeps struct {
	store       *fakes.FakeGatewayStore
	provisioner *fakes.FakeProvisioningClient
	connector   *fakes.FakeConnector
	encryptor   *fakes.FakeEncryptor
	clock       *fakes.FakeClock
	met         *metrics.Metrics
	cfg         *config.Config
}

func newTestDeps() testDeps {
	return testDeps{
		store:       fakes.NewFakeGatewayStore(),
		provisioner: &fakes.FakeProvisioningClient{},
		connector:   &fakes.FakeConnector{},
		encryptor:   &fakes.FakeEncryptor{},
		clock:       fakes.NewFakeClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		met:         metrics.NewTestMetrics(),
		cfg: &config.Config{
			DefaultSendFrequencyMs: 100, // Molto basso per far girare i test velocemente
			GatewayBufferSize:      10,
		},
	}
}

func newTestRegistry(d testDeps) *GatewayRegistry {
	return NewGatewayRegistry(d.store, d.provisioner, d.connector, d.encryptor, d.clock, d.cfg, d.met)
}
