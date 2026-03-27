package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"

	simhttp "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
)

const simHelper = "/sim/gateways"
const simHelper2 = "/sim/gateways/"
const Helper204 = "want 204, got %d"

// Helpers.
func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func newReq(method, path string, body *bytes.Reader) *http.Request {
	if body == nil {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, body)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// registerAndServe creates a mux with the supplied pattern, registers the handler and serves the request.
func serveWithMux(pattern string, handlerFn http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, handlerFn)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

//GatewayHandler.

func TestGatewayHandler_Create_201(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		CreateAndStartFn: func(_ context.Context, req domain.CreateGatewayRequest) (*domain.SimGateway, error) {
			return &domain.SimGateway{ID: 1, ManagementGatewayID: id, Status: domain.Running}, nil
		},
	}
	ctrl := &fakes.FakeSimulatorControlService{}
	h := simhttp.NewGatewayHandler(lc, ctrl)

	req := newReq(http.MethodPost, simHelper, jsonBody(t, domain.CreateGatewayRequest{
		TenantID: "t1", FactoryID: "fid", FactoryKey: "fkey",
	}))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", w.Code)
	}
}

func TestGatewayHandler_Create_ServiceError_500(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		CreateAndStartFn: func(_ context.Context, _ domain.CreateGatewayRequest) (*domain.SimGateway, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})

	req := newReq(http.MethodPost, simHelper, jsonBody(t, domain.CreateGatewayRequest{TenantID: "t1"}))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code < 400 {
		t.Errorf("want >=400 on service error, got %d", w.Code)
	}
}

func TestGatewayHandler_Create_BadBody_400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, simHelper, bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGatewayHandler_List_200(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		ListGatewaysFn: func(_ context.Context) ([]*domain.SimGateway, error) {
			return []*domain.SimGateway{{ID: 1}, {ID: 2}}, nil
		},
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})

	w := serveWithMux("GET /sim/gateways", h.List, newReq(http.MethodGet, simHelper, nil))
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestGatewayHandler_Get_200(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		GetGatewayFn: func(_ context.Context, mID uuid.UUID) (*domain.SimGateway, error) {
			return &domain.SimGateway{ManagementGatewayID: mID}, nil
		},
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})

	w := serveWithMux("GET /sim/gateways/{id}", h.Get,
		newReq(http.MethodGet, simHelper2+id.String(), nil))
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestGatewayHandler_Get_InvalidUUID_400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("GET /sim/gateways/{id}", h.Get,
		newReq(http.MethodGet, "/sim/gateways/not-a-uuid", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid UUID, got %d", w.Code)
	}
}

func TestGatewayHandler_Start_204(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StartFn: func(_ context.Context, mID uuid.UUID) error { return nil },
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})

	w := serveWithMux("POST /sim/gateways/{id}/start", h.Start,
		newReq(http.MethodPost, simHelper2+id.String()+"/start", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestGatewayHandler_Stop_204(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StopFn: func(_ context.Context, mID uuid.UUID) error { return nil },
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})

	w := serveWithMux("POST /sim/gateways/{id}/stop", h.Stop,
		newReq(http.MethodPost, simHelper2+id.String()+"/stop", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestGatewayHandler_Decommission_204(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		DecommissionFn: func(_ context.Context, mID uuid.UUID) error { return nil },
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})

	w := serveWithMux("DELETE /sim/gateways/{id}", h.Decommission,
		newReq(http.MethodDelete, simHelper2+id.String(), nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestGatewayHandler_UpdateConfig_204(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		UpdateConfigFn: func(_ context.Context, mID uuid.UUID, _ domain.GatewayConfigUpdate) error { return nil },
	}
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, ctrl)
	freq := 200
	w := serveWithMux("PATCH /sim/gateways/{id}/config", h.UpdateConfig,
		newReq(http.MethodPatch, simHelper2+id.String()+"/config",
			jsonBody(t, domain.GatewayConfigUpdate{SendFrequencyMs: &freq})))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestGatewayHandler_BulkCreate_201(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		BulkCreateGatewaysFn: func(_ context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error) {
			gws := make([]*domain.SimGateway, req.Count)
			for i := range gws {
				gws[i] = &domain.SimGateway{ID: int64(i + 1)}
			}
			return gws, nil
		},
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})

	w := httptest.NewRecorder()
	h.BulkCreate(w, newReq(http.MethodPost, "/sim/gateways/bulk", jsonBody(t, domain.BulkCreateRequest{
		Count: 2, TenantID: "t1", FactoryID: "fid", FactoryKey: "fkey",
	})))
	if w.Code != http.StatusCreated && w.Code != http.StatusMultiStatus {
		t.Errorf("want 201 or 207, got %d", w.Code)
	}
}

// SensorHandler.
func TestSensorHandler_Add_201(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		AddSensorFn: func(_ context.Context, gwID int64, s domain.SimSensor) (*domain.SimSensor, error) {
			s.ID = 10
			s.SensorID = uuid.New()
			return &s, nil
		},
	}
	h := simhttp.NewSensorHandler(svc)
	gwID := int64(1)

	w := serveWithMux("POST /sim/gateways/{id}/sensors", h.Add,
		newReq(http.MethodPost, simHelper2+strconv.FormatInt(gwID, 10)+"/sensors",
			jsonBody(t, domain.SimSensor{Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom})))
	if w.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", w.Code)
	}
}

func TestSensorHandler_Add_InvalidGatewayID_400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux("POST /sim/gateways/{id}/sensors", h.Add,
		newReq(http.MethodPost, "/sim/gateways/not-a-number/sensors",
			jsonBody(t, domain.SimSensor{})))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid gateway ID, got %d", w.Code)
	}
}

func TestSensorHandler_List_200(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		ListSensorsFn: func(_ context.Context, gwID int64) ([]*domain.SimSensor, error) {
			return []*domain.SimSensor{{ID: 1}, {ID: 2}}, nil
		},
	}
	h := simhttp.NewSensorHandler(svc)

	w := serveWithMux("GET /sim/gateways/{id}/sensors", h.List,
		newReq(http.MethodGet, "/sim/gateways/1/sensors", nil))
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestSensorHandler_Delete_204(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		DeleteSensorFn: func(_ context.Context, sensorID int64) error { return nil },
	}
	h := simhttp.NewSensorHandler(svc)

	w := serveWithMux("DELETE /sim/sensors/{sensorId}", h.Delete,
		newReq(http.MethodDelete, "/sim/sensors/5", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestSensorHandler_Delete_ServiceError(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		DeleteSensorFn: func(_ context.Context, _ int64) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewSensorHandler(svc)

	w := serveWithMux("DELETE /sim/sensors/{sensorId}", h.Delete,
		newReq(http.MethodDelete, "/sim/sensors/5", nil))
	if w.Code < 400 {
		t.Errorf("want >=400 on service error, got %d", w.Code)
	}
}

//AnomalyHandler.

func TestAnomalyHandler_NetworkDegradation_204(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, _ domain.GatewayAnomalyCommand) error {
			return nil
		},
	}

	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux("POST /sim/gateways/{id}/anomaly/network-degradation", h.InjectNetworkDegradation,
		newReq(http.MethodPost, simHelper2+id.String()+"/anomaly/network-degradation",
			jsonBody(t, map[string]any{"duration_seconds": 5, "packet_loss_pct": 50.0})))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestAnomalyHandler_Disconnect_204(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, _ domain.GatewayAnomalyCommand) error {
			return nil
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux("POST /sim/gateways/{id}/anomaly/disconnect", h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+"/anomaly/disconnect",
			jsonBody(t, map[string]any{"duration_seconds": 3})))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestAnomalyHandler_Outlier_204(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ int64, _ *float64) error {
			return nil
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	val := 999.9
	w := serveWithMux("POST /sim/sensors/{sensorId}/anomaly/outlier", h.InjectOutlier,
		newReq(http.MethodPost, "/sim/sensors/3/anomaly/outlier",
			jsonBody(t, map[string]any{"value": val})))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
}

func TestAnomalyHandler_Outlier_InvalidSensorID_400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})

	w := serveWithMux("POST /sim/sensors/{sensorId}/anomaly/outlier", h.InjectOutlier,
		newReq(http.MethodPost, "/sim/sensors/not-a-number/anomaly/outlier", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid sensorId, got %d", w.Code)
	}
}

func TestAnomalyHandler_ServiceError_ReturnsError(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, _ domain.GatewayAnomalyCommand) error {
			return fakes.ErrSimulated
		},
	}

	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux("POST /sim/gateways/{id}/anomaly/disconnect", h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+"/anomaly/disconnect",
			jsonBody(t, map[string]any{"duration_seconds": 1})))
	if w.Code < 400 {
		t.Errorf("want >=400 on service error, got %d", w.Code)
	}
}

// SimTokenMiddleware.
func TestSimTokenMiddleware_ValidToken_Passes(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := simhttp.SimTokenMiddleware("secret-token", next)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, simHelper, nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 for valid token, got %d", w.Code)
	}
}

func TestSimTokenMiddleware_InvalidToken_401(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := simhttp.SimTokenMiddleware("correct-token", next)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, simHelper, nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for wrong token, got %d", w.Code)
	}
}

func TestSimTokenMiddleware_MissingHeader_401(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := simhttp.SimTokenMiddleware("secret", next)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, simHelper, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for missing token, got %d", w.Code)
	}
}

func TestSimTokenMiddleware_HealthRoute_SkipsAuth(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := simhttp.SimTokenMiddleware("secret", next)

	// /health doesn't need a token.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 for /health without token, got %d", w.Code)
	}
}

func TestSimTokenMiddleware_EmptySecret_AllowsAll(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// With an empty secret middleware should not block.
	handler := simhttp.SimTokenMiddleware("", next)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, simHelper, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200 when token secret is empty, got %d", w.Code)
	}
}

func TestGatewayHandler_Start_InvalidUUID_400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/start", h.Start,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/start", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGatewayHandler_Start_ServiceError_500(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StartFn: func(_ context.Context, _ uuid.UUID) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/start", h.Start,
		newReq(http.MethodPost, simHelper2+id.String()+"/start", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestGatewayHandler_Stop_InvalidUUID_400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/stop", h.Stop,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/stop", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGatewayHandler_Stop_ServiceError_500(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StopFn: func(_ context.Context, _ uuid.UUID) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/stop", h.Stop,
		newReq(http.MethodPost, simHelper2+id.String()+"/stop", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestGatewayHandler_Decommission_InvalidUUID_400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("DELETE /sim/gateways/{id}", h.Decommission,
		newReq(http.MethodDelete, "/sim/gateways/not-a-uuid", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGatewayHandler_Decommission_ServiceError_500(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		DecommissionFn: func(_ context.Context, _ uuid.UUID) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("DELETE /sim/gateways/{id}", h.Decommission,
		newReq(http.MethodDelete, simHelper2+id.String(), nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestGatewayHandler_List_ServiceError_500(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		ListGatewaysFn: func(_ context.Context) ([]*domain.SimGateway, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("GET /sim/gateways", h.List, newReq(http.MethodGet, simHelper, nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestGatewayHandler_Get_ServiceError_404(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		GetGatewayFn: func(_ context.Context, _ uuid.UUID) (*domain.SimGateway, error) {
			return nil, domain.ErrGatewayNotFound
		},
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("GET /sim/gateways/{id}", h.Get,
		newReq(http.MethodGet, simHelper2+id.String(), nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestGatewayHandler_UpdateConfig_InvalidUUID_400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("PATCH /sim/gateways/{id}/config", h.UpdateConfig,
		newReq(http.MethodPatch, "/sim/gateways/not-a-uuid/config", jsonBody(t, domain.GatewayConfigUpdate{})))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGatewayHandler_UpdateConfig_BadBody_400(t *testing.T) {
	id := uuid.New()
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	w := serveWithMux("PATCH /sim/gateways/{id}/config", h.UpdateConfig,
		newReq(http.MethodPatch, simHelper2+id.String()+"/config",
			bytes.NewReader([]byte("not-json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGatewayHandler_UpdateConfig_ServiceError_500(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		UpdateConfigFn: func(_ context.Context, _ uuid.UUID, _ domain.GatewayConfigUpdate) error {
			return fakes.ErrSimulated
		},
	}
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, ctrl)
	freq := 100
	w := serveWithMux("PATCH /sim/gateways/{id}/config", h.UpdateConfig,
		newReq(http.MethodPatch, simHelper2+id.String()+"/config",
			jsonBody(t, domain.GatewayConfigUpdate{SendFrequencyMs: &freq})))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestGatewayHandler_BulkCreate_BadBody_400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{}, &fakes.FakeSimulatorControlService{})
	w := httptest.NewRecorder()
	h.BulkCreate(w, newReq(http.MethodPost, "/sim/gateways/bulk", bytes.NewReader([]byte("not-json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestGatewayHandler_BulkCreate_PartialErrors_207(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		BulkCreateGatewaysFn: func(_ context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error) {
			return []*domain.SimGateway{
					{ID: 1},
					nil,
				}, []error{
					nil,
					fakes.ErrSimulated,
				}
		},
	}
	h := simhttp.NewGatewayHandler(lc, &fakes.FakeSimulatorControlService{})
	w := httptest.NewRecorder()
	h.BulkCreate(w, newReq(http.MethodPost, "/sim/gateways/bulk", jsonBody(t, domain.BulkCreateRequest{
		Count: 2, TenantID: "t1",
	})))
	if w.Code != http.StatusMultiStatus {
		t.Errorf("want 207, got %d", w.Code)
	}
}

// SensorHandler missing branches.
func TestSensorHandler_Add_BadBody_400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux("POST /sim/gateways/{id}/sensors", h.Add,
		newReq(http.MethodPost, "/sim/gateways/1/sensors", bytes.NewReader([]byte("not-json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestSensorHandler_Add_ServiceError_500(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		AddSensorFn: func(_ context.Context, _ int64, _ domain.SimSensor) (*domain.SimSensor, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewSensorHandler(svc)
	w := serveWithMux("POST /sim/gateways/{id}/sensors", h.Add,
		newReq(http.MethodPost, "/sim/gateways/1/sensors",
			jsonBody(t, domain.SimSensor{Type: domain.Temperature})))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestSensorHandler_List_InvalidGatewayID_400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux("GET /sim/gateways/{id}/sensors", h.List,
		newReq(http.MethodGet, "/sim/gateways/not-a-number/sensors", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestSensorHandler_List_ServiceError_500(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		ListSensorsFn: func(_ context.Context, _ int64) ([]*domain.SimSensor, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewSensorHandler(svc)
	w := serveWithMux("GET /sim/gateways/{id}/sensors", h.List,
		newReq(http.MethodGet, "/sim/gateways/1/sensors", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", w.Code)
	}
}

func TestSensorHandler_Delete_InvalidSensorID_400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux("DELETE /sim/sensors/{sensorId}", h.Delete,
		newReq(http.MethodDelete, "/sim/sensors/not-a-number", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

// AnomalyHandler missing branches.
func TestAnomalyHandler_NetworkDegradation_InvalidUUID_400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/anomaly/network-degradation", h.InjectNetworkDegradation,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/anomaly/network-degradation",
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAnomalyHandler_NetworkDegradation_BadBody_400(t *testing.T) {
	id := uuid.New()
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/anomaly/network-degradation", h.InjectNetworkDegradation,
		newReq(http.MethodPost, simHelper2+id.String()+"/anomaly/network-degradation",
			bytes.NewReader([]byte("not-json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAnomalyHandler_NetworkDegradation_DefaultPacketLoss(t *testing.T) {
	id := uuid.New()
	var capturedCmd domain.GatewayAnomalyCommand
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, cmd domain.GatewayAnomalyCommand) error {
			capturedCmd = cmd
			return nil
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)
	w := serveWithMux("POST /sim/gateways/{id}/anomaly/network-degradation", h.InjectNetworkDegradation,
		newReq(http.MethodPost, simHelper2+id.String()+"/anomaly/network-degradation",
			jsonBody(t, map[string]any{"duration_seconds": 5})))
	if w.Code != http.StatusNoContent {
		t.Errorf(Helper204, w.Code)
	}
	if capturedCmd.NetworkDegradation.PacketLossPct != 0.3 {
		t.Errorf("want default packet loss 0.3, got %f", capturedCmd.NetworkDegradation.PacketLossPct)
	}
}

func TestAnomalyHandler_Disconnect_InvalidUUID_400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/anomaly/disconnect", h.InjectDisconnect,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/anomaly/disconnect",
			jsonBody(t, map[string]any{"duration_seconds": 1})))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAnomalyHandler_Disconnect_BadBody_400(t *testing.T) {
	id := uuid.New()
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/anomaly/disconnect", h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+"/anomaly/disconnect",
			bytes.NewReader([]byte("not-json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestAnomalyHandler_Disconnect_ZeroDuration_400(t *testing.T) {
	id := uuid.New()
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux("POST /sim/gateways/{id}/anomaly/disconnect", h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+"/anomaly/disconnect",
			jsonBody(t, map[string]any{"duration_seconds": 0})))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for zero duration, got %d", w.Code)
	}
}

func TestAnomalyHandler_Outlier_SensorNotFound_404(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ int64, _ *float64) error {
			return domain.ErrSensorNotFound
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux("POST /sim/sensors/{sensorId}/anomaly/outlier", h.InjectOutlier,
		newReq(http.MethodPost, "/sim/sensors/99/anomaly/outlier",
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing sensor, got %d", w.Code)
	}
}

func TestAnomalyHandler_Outlier_GatewayNotFound_404(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ int64, _ *float64) error {
			return domain.ErrGatewayNotFound
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux("POST /sim/sensors/{sensorId}/anomaly/outlier", h.InjectOutlier,
		newReq(http.MethodPost, "/sim/sensors/5/anomaly/outlier",
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing gateway, got %d", w.Code)
	}
}

func TestAnomalyHandler_Outlier_ServiceError_500(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ int64, _ *float64) error {
			return fakes.ErrSimulated
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux("POST /sim/sensors/{sensorId}/anomaly/outlier", h.InjectOutlier,
		newReq(http.MethodPost, "/sim/sensors/5/anomaly/outlier",
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 on service error, got %d", w.Code)
	}
}

func TestAnomalyHandler_Outlier_BadBody_400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})

	w := serveWithMux("POST /sim/sensors/{sensorId}/anomaly/outlier", h.InjectOutlier,
		newReq(http.MethodPost, "/sim/sensors/5/anomaly/outlier",
			bytes.NewReader([]byte("not-json"))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for bad body, got %d", w.Code)
	}
}
