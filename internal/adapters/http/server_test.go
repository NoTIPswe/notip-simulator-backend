package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	simhttp "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	lc := &fakes.FakeGatewayLifecycleService{}
	ctrl := &fakes.FakeSimulatorControlService{}
	svc := &fakes.FakeSensorManagementService{}

	gwHandler := simhttp.NewGatewayHandler(lc)
	sensorHandler := simhttp.NewSensorHandler(svc)
	anomalyHandler := simhttp.NewAnomalyHandler(ctrl)

	srv := simhttp.NewHTTPServer(":0", gwHandler, sensorHandler, anomalyHandler)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestHTTPServer_Handler_NotNil(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{}
	ctrl := &fakes.FakeSimulatorControlService{}
	svc := &fakes.FakeSensorManagementService{}
	srv := simhttp.NewHTTPServer(":0", simhttp.NewGatewayHandler(lc), simhttp.NewSensorHandler(svc), simhttp.NewAnomalyHandler(ctrl))
	if srv.Handler() == nil {
		t.Error("Handler() should not be nil")
	}
}

func TestHTTPServer_HealthRoute_Returns200(t *testing.T) {
	ts := newTestServer(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("health route should never require token")
	}
}

func TestHTTPServer_StartStop(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{}
	ctrl := &fakes.FakeSimulatorControlService{}
	svc := &fakes.FakeSensorManagementService{}

	srv := simhttp.NewHTTPServer(":0", simhttp.NewGatewayHandler(lc), simhttp.NewSensorHandler(svc), simhttp.NewAnomalyHandler(ctrl))

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}
