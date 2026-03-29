package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	simhttp "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
)

func newTestServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	lc := &fakes.FakeGatewayLifecycleService{}
	ctrl := &fakes.FakeSimulatorControlService{}
	svc := &fakes.FakeSensorManagementService{}

	gwHandler := simhttp.NewGatewayHandler(lc, ctrl)
	sensorHandler := simhttp.NewSensorHandler(svc)
	anomalyHandler := simhttp.NewAnomalyHandler(ctrl)

	srv := simhttp.NewHTTPServer(":0", token, gwHandler, sensorHandler, anomalyHandler)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestHTTPServer_Handler_NotNil(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{}
	ctrl := &fakes.FakeSimulatorControlService{}
	svc := &fakes.FakeSensorManagementService{}
	srv := simhttp.NewHTTPServer(":0", "", simhttp.NewGatewayHandler(lc, ctrl), simhttp.NewSensorHandler(svc), simhttp.NewAnomalyHandler(ctrl))
	if srv.Handler() == nil {
		t.Error("Handler() should not be nil")
	}
}

func TestHTTPServer_GatewayRoutes_NoToken_AllowedWhenEmpty(t *testing.T) {
	ts := newTestServer(t, "")
	id := uuid.New()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/sim/gateways/"+id.String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("should not require token when SimTokenSecret is empty")
	}
}

func TestHTTPServer_GatewayRoutes_WithToken_BlocksWithoutToken(t *testing.T) {
	ts := newTestServer(t, "mysecret")

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/sim/gateways", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 on gateway route without token, got %d", resp.StatusCode)
	}
}

func TestHTTPServer_GatewayRoutes_WithToken_AllowsCorrectToken(t *testing.T) {
	ts := newTestServer(t, "mysecret")

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/sim/gateways", nil)
	req.Header.Set("X-Sim-Token", "mysecret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("correct token should pass, got %d", resp.StatusCode)
	}
}

func TestHTTPServer_SensorRoutes_NoTokenRequired(t *testing.T) {
	ts := newTestServer(t, "mysecret")

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/sim/gateways/1/sensors", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("sensor routes should not require token")
	}
}

func TestHTTPServer_AnomalyRoutes_NoTokenRequired(t *testing.T) {
	ts := newTestServer(t, "mysecret")
	id := uuid.New()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/sim/gateways/"+id.String()+"/anomaly/disconnect", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("anomaly routes should not require token")
	}
}

func TestHTTPServer_HealthRoute_NoTokenRequired(t *testing.T) {
	ts := newTestServer(t, "mysecret")

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

	srv := simhttp.NewHTTPServer(":0", "", simhttp.NewGatewayHandler(lc, ctrl), simhttp.NewSensorHandler(svc), simhttp.NewAnomalyHandler(ctrl))

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
