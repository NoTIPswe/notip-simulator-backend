package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	httpadapter "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http/dto"
	"github.com/NoTIPswe/notip-simulator-backend/internal/app"
	"github.com/NoTIPswe/notip-simulator-backend/internal/config"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"
	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
)

const (
	tenantOneID                      = "tenant-1"
	headerContentType                = "Content-Type"
	contentTypeJSON                  = "application/json"
	pathSimGateways                  = "/sim/gateways"
	decodeErrMsg                     = "decode: %v"
	gatewayByIDURLFmt                = "%s/sim/gateways/%s"
	expected404Msg                   = "expected 404, got %d"
	expected400Msg                   = "expected 400, got %d"
	expected204Msg                   = "expected 204, got %d"
	postStartErrMsg                  = "POST start: %v"
	gatewayNetworkDegradationPathFmt = "/sim/gateways/%s/anomaly/network-degradation"
)

// Test infrastructure.

// testEnv holds all wired-up fakes and the running httptest.Server for one integration test. Every test gets its own isolated instance via newIntegrationEnv.
type testEnv struct {
	store       *fakes.FakeGatewayStore
	provisioner *fakes.FakeProvisioningClient
	connector   *fakes.FakeConnector
	clock       *fakes.FakeClock
	registry    *app.GatewayRegistry
	server      *httptest.Server
}

// newIntegrationEnv wires the real GatewayRegistry with in-memory fakes and starts an httptest. Server with the same route table as NewHTTPServer.
// No Docker, no NATS, no SQLite on disk — all I/O goes through fakes.
func newIntegrationEnv(t *testing.T) *testEnv {
	t.Helper()

	store := fakes.NewFakeGatewayStore()
	provisioner := &fakes.FakeProvisioningClient{}
	connector := &fakes.FakeConnector{}
	encryptor := &fakes.FakeEncryptor{}
	clock := fakes.NewFakeClock(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
	met := metrics.NewTestMetrics()
	cfg := &config.Config{
		DefaultSendFrequencyMs: 50, // fast ticks for timing-sensitive tests.
		GatewayBufferSize:      10,
	}

	// Provision responses need a valid 32-byte AES key.
	aesKey, err := domain.NewEncryptionKey(make([]byte, 32))
	if err != nil {
		t.Fatalf("setup: create AES key: %v", err)
	}
	provisioner.Result = domain.ProvisionResult{
		CertPEM:         []byte("fake-cert"),
		PrivateKeyPEM:   []byte("fake-key"),
		AESKey:          aesKey,
		GatewayID:       uuid.NewString(),
		TenantID:        tenantOneID,
		SendFrequencyMs: 50,
	}

	registry := app.NewGatewayRegistry(store, provisioner, connector, encryptor, clock, cfg, met)

	// Mirror the route table from NewHTTPServer exactly so we exercise the real handler/registry integration without binding a TCP port.
	gwHandler := httpadapter.NewGatewayHandler(registry)
	sensorHandler := httpadapter.NewSensorHandler(registry)
	anomalyHandler := httpadapter.NewAnomalyHandler(registry)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /sim/gateways", gwHandler.Create)
	mux.HandleFunc("POST /sim/gateways/bulk", gwHandler.BulkCreate)
	mux.HandleFunc("GET /sim/gateways", gwHandler.List)
	mux.HandleFunc("GET /sim/gateways/{id}", gwHandler.Get)
	mux.HandleFunc("POST /sim/gateways/{id}/start", gwHandler.Start)
	mux.HandleFunc("POST /sim/gateways/{id}/stop", gwHandler.Stop)
	mux.HandleFunc("DELETE /sim/gateways/{id}", gwHandler.Delete)
	mux.HandleFunc("POST /sim/gateways/{id}/sensors", sensorHandler.Add)
	mux.HandleFunc("GET /sim/gateways/{id}/sensors", sensorHandler.List)
	mux.HandleFunc("DELETE /sim/sensors/{sensorId}", sensorHandler.Delete)
	mux.HandleFunc("POST /sim/gateways/{id}/anomaly/network-degradation", anomalyHandler.InjectNetworkDegradation)
	mux.HandleFunc("POST /sim/gateways/{id}/anomaly/disconnect", anomalyHandler.InjectDisconnect)
	mux.HandleFunc("POST /sim/sensors/{sensorId}/anomaly/outlier", anomalyHandler.InjectOutlier)

	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		registry.StopAll(2 * time.Second)
	})

	return &testEnv{
		store:       store,
		provisioner: provisioner,
		connector:   connector,
		clock:       clock,
		registry:    registry,
		server:      srv,
	}
}

// postJSON marshals body as JSON and sends a POST to path.
func (e *testEnv) postJSON(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("postJSON: marshal: %v", err)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.server.URL+path, bytes.NewReader(b))
	req.Header.Set(headerContentType, contentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("postJSON: POST %s: %v", path, err)
	}
	return resp
}

// createGateway POSTs a gateway and returns the decoded GatewayResponse.
// Fails the test immediately if the response is not 201.
func (e *testEnv) createGateway(t *testing.T) *dto.GatewayResponse {
	t.Helper()
	resp := e.postJSON(t, pathSimGateways, domain.CreateGatewayRequest{
		FactoryID:       "factory-1",
		FactoryKey:      "key-1",
		Model:           "ModelX",
		FirmwareVersion: "1.0.0",
		SendFrequencyMs: 50,
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("createGateway: expected 201, got %d", resp.StatusCode)
	}
	var gw dto.GatewayResponse
	if err := json.NewDecoder(resp.Body).Decode(&gw); err != nil {
		t.Fatalf("createGateway: decode: %v", err)
	}
	return &gw
}

// Gateway lifecycle.

func TestIntegrationCreateGatewaySuccess(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	if gw.ID == (uuid.UUID{}) {
		t.Error("gateway ID should not be zero UUID")
	}
	if gw.TenantID != tenantOneID {
		t.Errorf("TenantID: want 'tenant-1', got %q", gw.TenantID)
	}
	if gw.Status != domain.Online {
		t.Errorf("Status: want Online, got %q", gw.Status)
	}
}

func TestIntegrationCreateGatewayProvisioningFailure(t *testing.T) {
	e := newIntegrationEnv(t)
	e.provisioner.Err = fakes.ErrSimulated

	resp := e.postJSON(t, pathSimGateways, domain.CreateGatewayRequest{
		FactoryID: "f1", FactoryKey: "k1",
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 on provisioning failure, got %d", resp.StatusCode)
	}
}

func TestIntegrationCreateGatewayConnectorFailure(t *testing.T) {
	e := newIntegrationEnv(t)
	e.connector.Err = fakes.ErrSimulated

	resp := e.postJSON(t, pathSimGateways, domain.CreateGatewayRequest{
		FactoryID: "f1", FactoryKey: "k1",
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 on connector failure, got %d", resp.StatusCode)
	}
}

func TestIntegrationCreateGatewayStoreCreateFailure(t *testing.T) {
	e := newIntegrationEnv(t)
	e.store.ErrCreateGateway = fakes.ErrSimulated

	resp := e.postJSON(t, pathSimGateways, domain.CreateGatewayRequest{
		FactoryID: "f1", FactoryKey: "k1",
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 on store create failure, got %d", resp.StatusCode)
	}
}

func TestIntegrationCreateGatewayUpdateProvisionedFailureNoEffect(t *testing.T) {
	e := newIntegrationEnv(t)
	e.store.ErrUpdateProvisioned = fakes.ErrSimulated

	resp := e.postJSON(t, pathSimGateways, domain.CreateGatewayRequest{
		FactoryID: "f1", FactoryKey: "k1",
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201 because UpdateProvisioned is no longer used, got %d", resp.StatusCode)
	}
}

func TestIntegrationListGatewaysEmpty(t *testing.T) {
	e := newIntegrationEnv(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.server.URL+pathSimGateways, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /sim/gateways: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegrationListGatewaysMultiple(t *testing.T) {
	e := newIntegrationEnv(t)
	e.createGateway(t)
	e.createGateway(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.server.URL+pathSimGateways, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /sim/gateways: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	var gateways []dto.GatewayResponse
	if err := json.NewDecoder(resp.Body).Decode(&gateways); err != nil {
		t.Fatalf(decodeErrMsg, err)
	}
	if len(gateways) != 2 {
		t.Errorf("expected 2 gateways, got %d", len(gateways))
	}
}

func TestIntegrationGetGatewaySuccess(t *testing.T) {
	e := newIntegrationEnv(t)
	created := e.createGateway(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf(gatewayByIDURLFmt, e.server.URL, created.ID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET gateway: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var gw dto.GatewayResponse
	if err := json.NewDecoder(resp.Body).Decode(&gw); err != nil {
		t.Fatalf(decodeErrMsg, err)
	}
	if gw.ID != created.ID {
		t.Errorf("ID mismatch: want %s, got %s", created.ID, gw.ID)
	}
}

func TestIntegrationGetGatewayNotFound(t *testing.T) {
	e := newIntegrationEnv(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf(gatewayByIDURLFmt, e.server.URL, uuid.New()), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf(expected404Msg, resp.StatusCode)
	}
}

func TestIntegrationGetGatewayInvalidID(t *testing.T) {
	e := newIntegrationEnv(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, e.server.URL+"/sim/gateways/not-a-uuid", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(expected400Msg, resp.StatusCode)
	}
}

func TestIntegrationStopGatewaySuccess(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("%s/sim/gateways/%s/stop", e.server.URL, gw.ID), nil)
	req.Header.Set(headerContentType, contentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf(expected204Msg, resp.StatusCode)
	}
}

func TestIntegrationStopGatewayNotFound(t *testing.T) {
	e := newIntegrationEnv(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("%s/sim/gateways/%s/stop", e.server.URL, uuid.New()), nil)
	req.Header.Set(headerContentType, contentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown gateway, got %d", resp.StatusCode)
	}
}

func TestIntegrationStartGatewayAlreadyRunning(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	// Gateway is Running right after CreateAndStart — starting again must fail.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("%s/sim/gateways/%s/start", e.server.URL, gw.ID), nil)
	req.Header.Set(headerContentType, contentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf(postStartErrMsg, err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 for already-running gateway, got %d", resp.StatusCode)
	}
}

func TestIntegrationStartGatewayNotFound(t *testing.T) {
	e := newIntegrationEnv(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("%s/sim/gateways/%s/start", e.server.URL, uuid.New()), nil)
	req.Header.Set(headerContentType, contentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf(postStartErrMsg, err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown gateway, got %d", resp.StatusCode)
	}
}

func TestIntegrationStartGatewayInvalidID(t *testing.T) {
	e := newIntegrationEnv(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, e.server.URL+"/sim/gateways/not-a-uuid/start", nil)
	req.Header.Set(headerContentType, contentTypeJSON)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf(postStartErrMsg, err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(expected400Msg, resp.StatusCode)
	}
}

func TestIntegrationDeleteGatewaySuccess(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf(gatewayByIDURLFmt, e.server.URL, gw.ID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE gateway: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf(expected204Msg, resp.StatusCode)
	}
	// Verify it was actually removed from the store.
	if _, err := e.store.GetGatewayByManagementID(context.Background(), gw.ID); err == nil {
		t.Error("gateway should have been deleted from the store after decommission")
	}
}

func TestIntegrationDeleteGatewayNotFound(t *testing.T) {
	e := newIntegrationEnv(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf(gatewayByIDURLFmt, e.server.URL, uuid.New()), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf(expected404Msg, resp.StatusCode)
	}
}

// Bulk create.

func TestIntegrationBulkCreateAllSuccess(t *testing.T) {
	e := newIntegrationEnv(t)

	resp := e.postJSON(t, "/sim/gateways/bulk", domain.BulkCreateRequest{
		Count: 3,
	})
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	var result struct {
		Gateways []dto.GatewayResponse `json:"gateways"`
		Errors   []string              `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf(decodeErrMsg, err)
	}
	if len(result.Gateways) != 3 {
		t.Errorf("expected 3 gateways, got %d", len(result.Gateways))
	}
}

func TestIntegrationBulkCreateAllFailure(t *testing.T) {
	e := newIntegrationEnv(t)
	e.provisioner.Err = fakes.ErrSimulated // make every provisioning call fail.

	resp := e.postJSON(t, "/sim/gateways/bulk", domain.BulkCreateRequest{
		Count: 2,
	})
	defer func() { _ = resp.Body.Close() }()

	// handler returns 207 when hasErrors == true.
	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("expected 207 on all-failure bulk create, got %d", resp.StatusCode)
	}
}

// Anomaly injection.

func TestIntegrationInjectNetworkDegradationSuccess(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	resp := e.postJSON(t,
		fmt.Sprintf(gatewayNetworkDegradationPathFmt, gw.ID),
		map[string]any{"duration_seconds": 10, "packet_loss_pct": 0.5},
	)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf(expected204Msg, resp.StatusCode)
	}
}

func TestIntegrationInjectNetworkDegradationDefaultPacketLoss(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	// Omitting packet_loss_pct — handler defaults it to 0.3.
	resp := e.postJSON(t,
		fmt.Sprintf(gatewayNetworkDegradationPathFmt, gw.ID),
		map[string]any{"duration_seconds": 5},
	)

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf(expected204Msg, resp.StatusCode)
	}
}

func TestIntegrationInjectNetworkDegradationNotFound(t *testing.T) {
	e := newIntegrationEnv(t)

	resp := e.postJSON(t,
		fmt.Sprintf(gatewayNetworkDegradationPathFmt, uuid.New()),
		map[string]any{"duration_seconds": 5},
	)

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf(expected404Msg, resp.StatusCode)
	}
}

func TestIntegrationInjectDisconnectSuccess(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	resp := e.postJSON(t,
		fmt.Sprintf("/sim/gateways/%s/anomaly/disconnect", gw.ID),
		map[string]any{"duration_seconds": 5},
	)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf(expected204Msg, resp.StatusCode)
	}
}

func TestIntegrationInjectDisconnectZeroDuration(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	resp := e.postJSON(t,
		fmt.Sprintf("/sim/gateways/%s/anomaly/disconnect", gw.ID),
		map[string]any{"duration_seconds": 0},
	)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for zero duration, got %d", resp.StatusCode)
	}
}

func TestIntegrationInjectDisconnectInvalidID(t *testing.T) {
	e := newIntegrationEnv(t)

	resp := e.postJSON(t,
		"/sim/gateways/not-a-uuid/anomaly/disconnect",
		map[string]any{"duration_seconds": 5},
	)
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf(expected400Msg, resp.StatusCode)
	}
}

// NATS decommission events (HandleDecommission driving port).

func TestIntegrationHandleDecommissionRemovesGateway(t *testing.T) {
	e := newIntegrationEnv(t)
	gw := e.createGateway(t)

	e.registry.HandleDecommission(gw.TenantID, gw.ID.String())

	if _, err := e.store.GetGatewayByManagementID(context.Background(), gw.ID); err == nil {
		t.Error("gateway should have been removed from the store after NATS decommission")
	}
}

func TestIntegrationHandleDecommissionInvalidUUIDNoopNoPanic(t *testing.T) {
	e := newIntegrationEnv(t)
	// Must not panic on a malformed UUID.
	e.registry.HandleDecommission(tenantOneID, "this-is-not-a-uuid")
}

func TestIntegrationHandleDecommissionUnknownGatewayNoop(t *testing.T) {
	e := newIntegrationEnv(t)
	// Unknown gateway — must be a no-op.
	e.registry.HandleDecommission(tenantOneID, uuid.New().String())
}

// RestoreAll (recovery mode).

func TestIntegrationRestoreAllEmptyStore(t *testing.T) {
	e := newIntegrationEnv(t)

	if err := e.registry.RestoreAll(context.Background()); err != nil {
		t.Errorf("RestoreAll on empty store should not error: %v", err)
	}
}

func TestIntegrationRestoreAllSkipsUnprovisioned(t *testing.T) {
	e := newIntegrationEnv(t)

	_, err := e.store.CreateGateway(context.Background(), domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            tenantOneID,
		Provisioned:         false,
		SendFrequencyMs:     50,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := e.registry.RestoreAll(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIntegrationRestoreAllRestartsProvisionedGateway(t *testing.T) {
	e := newIntegrationEnv(t)

	aesKey, _ := domain.NewEncryptionKey(make([]byte, 32))
	id, err := e.store.CreateGateway(context.Background(), domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            tenantOneID,
		Provisioned:         true,
		SendFrequencyMs:     50,
		Status:              domain.Paused,
	})
	if err != nil {
		t.Fatalf("setup: create gateway: %v", err)
	}
	if err := e.store.UpdateProvisioned(context.Background(), id, domain.ProvisionResult{
		CertPEM:       []byte("cert"),
		PrivateKeyPEM: []byte("key"),
		AESKey:        aesKey,
	}); err != nil {
		t.Fatalf("setup: update provisioned: %v", err)
	}

	if err := e.registry.RestoreAll(context.Background()); err != nil {
		t.Errorf("RestoreAll should not error: %v", err)
	}
}

func TestIntegrationRestoreAllConnectorFailureContinuesOthers(t *testing.T) {
	e := newIntegrationEnv(t)
	e.connector.Err = fakes.ErrSimulated

	aesKey, _ := domain.NewEncryptionKey(make([]byte, 32))
	id, err := e.store.CreateGateway(context.Background(), domain.SimGateway{
		ManagementGatewayID: uuid.New(),
		TenantID:            tenantOneID,
		Provisioned:         true,
		SendFrequencyMs:     50,
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := e.store.UpdateProvisioned(context.Background(), id, domain.ProvisionResult{
		CertPEM: []byte("cert"), PrivateKeyPEM: []byte("key"), AESKey: aesKey,
	}); err != nil {
		t.Fatalf("setup: update provisioned: %v", err)
	}

	// RestoreAll logs per-gateway errors but does NOT propagate them.
	if err := e.registry.RestoreAll(context.Background()); err != nil {
		t.Errorf("RestoreAll should absorb per-gateway errors: %v", err)
	}
}
