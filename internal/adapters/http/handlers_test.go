package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/fakes"

	simhttp "github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http"
)

const simHelper = "/sim/gateways"
const simHelper2 = "/sim/gateways/"
const want204Msg = "want 204, got %d"
const want200Msg = "want 200, got %d"
const want400Msg = "want 400, got %d"
const want500Msg = "want 500, got %d"
const wantServiceErrMin400Msg = "want >=400 on service error, got %d"
const invalidJSONBody = "not-json"

const routeGetGatewayByID = "GET /sim/gateways/{id}"
const routeStartGateway = "POST /sim/gateways/{id}/start"
const routeStopGateway = "POST /sim/gateways/{id}/stop"
const routeDeleteGatewayByID = "DELETE /sim/gateways/{id}"
const pathBulkCreateGateway = "/sim/gateways/bulk"
const suffixStartGateway = "/start"

const routeGatewaySensorsAdd = "POST /sim/gateways/{id}/sensors"
const routeGatewaySensorsList = "GET /sim/gateways/{id}/sensors"
const testGatewayUUIDStr = "11111111-1111-1111-1111-111111111111"
const testSensorUUIDStr = "22222222-2222-2222-2222-222222222222"
const pathGateway1Sensors = "/sim/gateways/" + testGatewayUUIDStr + "/sensors"
const routeDeleteSensorByID = "DELETE /sim/sensors/{sensorId}"

const routeGatewayAnomalyNetworkDegradation = "POST /sim/gateways/{id}/anomaly/network-degradation"
const suffixAnomalyNetworkDegradation = "/anomaly/network-degradation"
const routeGatewayAnomalyDisconnect = "POST /sim/gateways/{id}/anomaly/disconnect"
const suffixAnomalyDisconnect = "/anomaly/disconnect"
const pathSimSensors = "/sim/sensors/"
const routeSensorAnomalyOutlier = "POST /sim/sensors/{sensorId}/anomaly/outlier"
const pathSensor5AnomalyOutlier = pathSimSensors + testSensorUUIDStr + "/anomaly/outlier"

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

func TestGatewayHandlerCreate201(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		CreateAndStartFn: func(_ context.Context, req domain.CreateGatewayRequest) (*domain.SimGateway, error) {
			return &domain.SimGateway{ID: 1, ManagementGatewayID: id, Status: domain.Online}, nil
		},
	}

	h := simhttp.NewGatewayHandler(lc)

	req := newReq(http.MethodPost, simHelper, jsonBody(t, domain.CreateGatewayRequest{
		FactoryID: "fid", FactoryKey: "fkey",
	}))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", w.Code)
	}
}

func TestGatewayHandlerCreateServiceError500(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		CreateAndStartFn: func(_ context.Context, _ domain.CreateGatewayRequest) (*domain.SimGateway, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewGatewayHandler(lc)

	req := newReq(http.MethodPost, simHelper, jsonBody(t, domain.CreateGatewayRequest{}))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code < 400 {
		t.Errorf(wantServiceErrMin400Msg, w.Code)
	}
}

func TestGatewayHandlerCreateAlreadyProvisioned409(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		CreateAndStartFn: func(_ context.Context, _ domain.CreateGatewayRequest) (*domain.SimGateway, error) {
			return nil, domain.ErrGatewayAlreadyProvisioned
		},
	}
	h := simhttp.NewGatewayHandler(lc)

	req := newReq(http.MethodPost, simHelper, jsonBody(t, domain.CreateGatewayRequest{}))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
}

func TestGatewayHandlerCreateInvalidFactoryCredentials401(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		CreateAndStartFn: func(_ context.Context, _ domain.CreateGatewayRequest) (*domain.SimGateway, error) {
			return nil, domain.ErrInvalidFactoryCredentials
		},
	}
	h := simhttp.NewGatewayHandler(lc)

	req := newReq(http.MethodPost, simHelper, jsonBody(t, domain.CreateGatewayRequest{}))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestGatewayHandlerCreateBadBody400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, simHelper, bytes.NewReader([]byte(invalidJSONBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestGatewayHandlerList200(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		ListGatewaysFn: func(_ context.Context) ([]*domain.SimGateway, error) {
			return []*domain.SimGateway{{ID: 1}, {ID: 2}}, nil
		},
	}
	h := simhttp.NewGatewayHandler(lc)

	w := serveWithMux("GET /sim/gateways", h.List, newReq(http.MethodGet, simHelper, nil))
	if w.Code != http.StatusOK {
		t.Errorf(want200Msg, w.Code)
	}
}

func TestGatewayHandlerGet200(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		GetGatewayFn: func(_ context.Context, mID uuid.UUID) (*domain.SimGateway, error) {
			return &domain.SimGateway{ManagementGatewayID: mID}, nil
		},
	}
	h := simhttp.NewGatewayHandler(lc)

	w := serveWithMux(routeGetGatewayByID, h.Get,
		newReq(http.MethodGet, simHelper2+id.String(), nil))
	if w.Code != http.StatusOK {
		t.Errorf(want200Msg, w.Code)
	}
}

func TestGatewayHandlerGetInvalidUUID400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{})
	w := serveWithMux(routeGetGatewayByID, h.Get,
		newReq(http.MethodGet, "/sim/gateways/not-a-uuid", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid UUID, got %d", w.Code)
	}
}

func TestGatewayHandlerStart204(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StartFn: func(_ context.Context, mID uuid.UUID) error { return nil },
	}
	h := simhttp.NewGatewayHandler(lc)

	w := serveWithMux(routeStartGateway, h.Start,
		newReq(http.MethodPost, simHelper2+id.String()+suffixStartGateway, nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
}

func TestGatewayHandlerStop204(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StopFn: func(_ context.Context, mID uuid.UUID) error { return nil },
	}
	h := simhttp.NewGatewayHandler(lc)

	w := serveWithMux(routeStopGateway, h.Stop,
		newReq(http.MethodPost, simHelper2+id.String()+"/stop", nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
}

func TestGatewayHandlerDelete204(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		DeleteFn: func(_ context.Context, mID uuid.UUID) error { return nil },
	}
	h := simhttp.NewGatewayHandler(lc)

	w := serveWithMux(routeDeleteGatewayByID, h.Delete,
		newReq(http.MethodDelete, simHelper2+id.String(), nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
}

func TestGatewayHandlerBulkCreate201(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		BulkCreateGatewaysFn: func(_ context.Context, req domain.BulkCreateRequest) ([]*domain.SimGateway, []error) {
			gws := make([]*domain.SimGateway, req.Count)
			for i := range gws {
				gws[i] = &domain.SimGateway{ID: int64(i + 1)}
			}
			return gws, nil
		},
	}
	h := simhttp.NewGatewayHandler(lc)

	w := httptest.NewRecorder()
	h.BulkCreate(w, newReq(http.MethodPost, pathBulkCreateGateway, jsonBody(t, domain.BulkCreateRequest{
		Count: 2, FactoryID: "fid", FactoryKey: "fkey",
	})))
	if w.Code != http.StatusCreated && w.Code != http.StatusMultiStatus {
		t.Errorf("want 201 or 207, got %d", w.Code)
	}
}

// SensorHandler.
func TestSensorHandlerAdd201(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		AddSensorFn: func(_ context.Context, gwID uuid.UUID, s domain.SimSensor) (*domain.SimSensor, error) {
			s.SensorID = uuid.New()
			s.ManagementGatewayID = gwID
			return &s, nil
		},
	}
	h := simhttp.NewSensorHandler(svc)

	w := serveWithMux(routeGatewaySensorsAdd, h.Add,
		newReq(http.MethodPost, simHelper2+testGatewayUUIDStr+"/sensors",
			jsonBody(t, domain.SimSensor{Type: domain.Temperature, MinRange: 0, MaxRange: 100, Algorithm: domain.UniformRandom})))
	if w.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", w.Code)
	}
}

func TestSensorHandlerAddInvalidGatewayID400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux(routeGatewaySensorsAdd, h.Add,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/sensors",
			jsonBody(t, domain.SimSensor{})))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid gateway ID, got %d", w.Code)
	}
}

func TestSensorHandlerList200(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		ListSensorsFn: func(_ context.Context, gwID uuid.UUID) ([]*domain.SimSensor, error) {
			return []*domain.SimSensor{{SensorID: uuid.New()}, {SensorID: uuid.New()}}, nil
		},
	}
	h := simhttp.NewSensorHandler(svc)

	w := serveWithMux(routeGatewaySensorsList, h.List,
		newReq(http.MethodGet, pathGateway1Sensors, nil))
	if w.Code != http.StatusOK {
		t.Errorf(want200Msg, w.Code)
	}
}

func TestSensorHandlerDelete204(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		DeleteSensorFn: func(_ context.Context, sensorID uuid.UUID) error { return nil },
	}
	h := simhttp.NewSensorHandler(svc)

	w := serveWithMux(routeDeleteSensorByID, h.Delete,
		newReq(http.MethodDelete, pathSimSensors+testSensorUUIDStr, nil))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
}

func TestSensorHandlerDeleteServiceError(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		DeleteSensorFn: func(_ context.Context, _ uuid.UUID) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewSensorHandler(svc)

	w := serveWithMux(routeDeleteSensorByID, h.Delete,
		newReq(http.MethodDelete, pathSimSensors+testSensorUUIDStr, nil))
	if w.Code < 400 {
		t.Errorf(wantServiceErrMin400Msg, w.Code)
	}
}

//AnomalyHandler.

func TestAnomalyHandlerNetworkDegradation204(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, _ domain.GatewayAnomalyCommand) error {
			return nil
		},
	}

	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux(routeGatewayAnomalyNetworkDegradation, h.InjectNetworkDegradation,
		newReq(http.MethodPost, simHelper2+id.String()+suffixAnomalyNetworkDegradation,
			jsonBody(t, map[string]any{"duration_seconds": 5, "packet_loss_pct": 50.0})))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
}

func TestAnomalyHandlerDisconnect204(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, _ domain.GatewayAnomalyCommand) error {
			return nil
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux(routeGatewayAnomalyDisconnect, h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+suffixAnomalyDisconnect,
			jsonBody(t, map[string]any{"duration_seconds": 3})))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
}

func TestAnomalyHandlerOutlier204(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ uuid.UUID, _ *float64) error {
			return nil
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	val := 999.9
	w := serveWithMux(routeSensorAnomalyOutlier, h.InjectOutlier,
		newReq(http.MethodPost, pathSimSensors+testSensorUUIDStr+"/anomaly/outlier",
			jsonBody(t, map[string]any{"value": val})))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
}

func TestAnomalyHandlerOutlierInvalidSensorID400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})

	w := serveWithMux(routeSensorAnomalyOutlier, h.InjectOutlier,
		newReq(http.MethodPost, "/sim/sensors/not-a-number/anomaly/outlier", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for invalid sensorId, got %d", w.Code)
	}
}

func TestAnomalyHandlerServiceErrorReturnsError(t *testing.T) {
	id := uuid.New()
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, _ domain.GatewayAnomalyCommand) error {
			return fakes.ErrSimulated
		},
	}

	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux(routeGatewayAnomalyDisconnect, h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+suffixAnomalyDisconnect,
			jsonBody(t, map[string]any{"duration_seconds": 1})))
	if w.Code < 400 {
		t.Errorf(wantServiceErrMin400Msg, w.Code)
	}
}

func TestGatewayHandlerStartInvalidUUID400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{})
	w := serveWithMux(routeStartGateway, h.Start,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/start", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestGatewayHandlerStartServiceError500(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StartFn: func(_ context.Context, _ uuid.UUID) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewGatewayHandler(lc)
	w := serveWithMux(routeStartGateway, h.Start,
		newReq(http.MethodPost, simHelper2+id.String()+suffixStartGateway, nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf(want500Msg, w.Code)
	}
}

func TestGatewayHandlerStartAlreadyRunning409(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StartFn: func(_ context.Context, _ uuid.UUID) error { return domain.ErrGatewayAlreadyRunning },
	}
	h := simhttp.NewGatewayHandler(lc)
	w := serveWithMux(routeStartGateway, h.Start,
		newReq(http.MethodPost, simHelper2+id.String()+suffixStartGateway, nil))
	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
}

func TestGatewayHandlerStopInvalidUUID400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{})
	w := serveWithMux(routeStopGateway, h.Stop,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/stop", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestGatewayHandlerStopServiceError500(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		StopFn: func(_ context.Context, _ uuid.UUID) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewGatewayHandler(lc)
	w := serveWithMux(routeStopGateway, h.Stop,
		newReq(http.MethodPost, simHelper2+id.String()+"/stop", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf(want500Msg, w.Code)
	}
}

func TestGatewayHandlerDeleteInvalidUUID400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{})
	w := serveWithMux(routeDeleteGatewayByID, h.Delete,
		newReq(http.MethodDelete, "/sim/gateways/not-a-uuid", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestGatewayHandlerDeleteServiceError500(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		DeleteFn: func(_ context.Context, _ uuid.UUID) error { return fakes.ErrSimulated },
	}
	h := simhttp.NewGatewayHandler(lc)
	w := serveWithMux(routeDeleteGatewayByID, h.Delete,
		newReq(http.MethodDelete, simHelper2+id.String(), nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf(want500Msg, w.Code)
	}
}

func TestGatewayHandlerListServiceError500(t *testing.T) {
	lc := &fakes.FakeGatewayLifecycleService{
		ListGatewaysFn: func(_ context.Context) ([]*domain.SimGateway, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewGatewayHandler(lc)
	w := serveWithMux("GET /sim/gateways", h.List, newReq(http.MethodGet, simHelper, nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf(want500Msg, w.Code)
	}
}

func TestGatewayHandlerGetServiceError404(t *testing.T) {
	id := uuid.New()
	lc := &fakes.FakeGatewayLifecycleService{
		GetGatewayFn: func(_ context.Context, _ uuid.UUID) (*domain.SimGateway, error) {
			return nil, domain.ErrGatewayNotFound
		},
	}
	h := simhttp.NewGatewayHandler(lc)
	w := serveWithMux(routeGetGatewayByID, h.Get,
		newReq(http.MethodGet, simHelper2+id.String(), nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestGatewayHandlerBulkCreateBadBody400(t *testing.T) {
	h := simhttp.NewGatewayHandler(&fakes.FakeGatewayLifecycleService{})
	w := httptest.NewRecorder()
	h.BulkCreate(w, newReq(http.MethodPost, pathBulkCreateGateway, bytes.NewReader([]byte(invalidJSONBody))))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestGatewayHandlerBulkCreatePartialErrors207(t *testing.T) {
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
	h := simhttp.NewGatewayHandler(lc)
	w := httptest.NewRecorder()
	h.BulkCreate(w, newReq(http.MethodPost, pathBulkCreateGateway, jsonBody(t, domain.BulkCreateRequest{
		Count: 2,
	})))
	if w.Code != http.StatusMultiStatus {
		t.Errorf("want 207, got %d", w.Code)
	}
}

// SensorHandler missing branches.
func TestSensorHandlerAddBadBody400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux(routeGatewaySensorsAdd, h.Add,
		newReq(http.MethodPost, pathGateway1Sensors, bytes.NewReader([]byte(invalidJSONBody))))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestSensorHandlerAddServiceError500(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		AddSensorFn: func(_ context.Context, _ uuid.UUID, _ domain.SimSensor) (*domain.SimSensor, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewSensorHandler(svc)
	w := serveWithMux(routeGatewaySensorsAdd, h.Add,
		newReq(http.MethodPost, pathGateway1Sensors,
			jsonBody(t, domain.SimSensor{Type: domain.Temperature})))
	if w.Code != http.StatusInternalServerError {
		t.Errorf(want500Msg, w.Code)
	}
}

func TestSensorHandlerListInvalidGatewayID400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux(routeGatewaySensorsList, h.List,
		newReq(http.MethodGet, "/sim/gateways/not-a-uuid/sensors", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestSensorHandlerListServiceError500(t *testing.T) {
	svc := &fakes.FakeSensorManagementService{
		ListSensorsFn: func(_ context.Context, _ uuid.UUID) ([]*domain.SimSensor, error) {
			return nil, fakes.ErrSimulated
		},
	}
	h := simhttp.NewSensorHandler(svc)
	w := serveWithMux(routeGatewaySensorsList, h.List,
		newReq(http.MethodGet, pathGateway1Sensors, nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf(want500Msg, w.Code)
	}
}

func TestSensorHandlerDeleteInvalidSensorID400(t *testing.T) {
	h := simhttp.NewSensorHandler(&fakes.FakeSensorManagementService{})
	w := serveWithMux(routeDeleteSensorByID, h.Delete,
		newReq(http.MethodDelete, pathSimSensors+"not-a-uuid", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

// AnomalyHandler missing branches.
func TestAnomalyHandlerNetworkDegradationInvalidUUID400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux(routeGatewayAnomalyNetworkDegradation, h.InjectNetworkDegradation,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/anomaly/network-degradation",
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestAnomalyHandlerNetworkDegradationBadBody400(t *testing.T) {
	id := uuid.New()
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux(routeGatewayAnomalyNetworkDegradation, h.InjectNetworkDegradation,
		newReq(http.MethodPost, simHelper2+id.String()+suffixAnomalyNetworkDegradation,
			bytes.NewReader([]byte(invalidJSONBody))))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestAnomalyHandlerNetworkDegradationDefaultPacketLoss(t *testing.T) {
	id := uuid.New()
	var capturedCmd domain.GatewayAnomalyCommand
	ctrl := &fakes.FakeSimulatorControlService{
		InjectGatewayAnomalyFn: func(_ context.Context, _ uuid.UUID, cmd domain.GatewayAnomalyCommand) error {
			capturedCmd = cmd
			return nil
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)
	w := serveWithMux(routeGatewayAnomalyNetworkDegradation, h.InjectNetworkDegradation,
		newReq(http.MethodPost, simHelper2+id.String()+suffixAnomalyNetworkDegradation,
			jsonBody(t, map[string]any{"duration_seconds": 5})))
	if w.Code != http.StatusNoContent {
		t.Errorf(want204Msg, w.Code)
	}
	if capturedCmd.NetworkDegradation.PacketLossPct != 0.3 {
		t.Errorf("want default packet loss 0.3, got %f", capturedCmd.NetworkDegradation.PacketLossPct)
	}
}

func TestAnomalyHandlerDisconnectInvalidUUID400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux(routeGatewayAnomalyDisconnect, h.InjectDisconnect,
		newReq(http.MethodPost, "/sim/gateways/not-a-uuid/anomaly/disconnect",
			jsonBody(t, map[string]any{"duration_seconds": 1})))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestAnomalyHandlerDisconnectBadBody400(t *testing.T) {
	id := uuid.New()
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux(routeGatewayAnomalyDisconnect, h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+suffixAnomalyDisconnect,
			bytes.NewReader([]byte(invalidJSONBody))))
	if w.Code != http.StatusBadRequest {
		t.Errorf(want400Msg, w.Code)
	}
}

func TestAnomalyHandlerDisconnectZeroDuration400(t *testing.T) {
	id := uuid.New()
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})
	w := serveWithMux(routeGatewayAnomalyDisconnect, h.InjectDisconnect,
		newReq(http.MethodPost, simHelper2+id.String()+suffixAnomalyDisconnect,
			jsonBody(t, map[string]any{"duration_seconds": 0})))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for zero duration, got %d", w.Code)
	}
}

func TestAnomalyHandlerOutlierSensorNotFound404(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ uuid.UUID, _ *float64) error {
			return domain.ErrSensorNotFound
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux(routeSensorAnomalyOutlier, h.InjectOutlier,
		newReq(http.MethodPost, pathSensor5AnomalyOutlier,
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing sensor, got %d", w.Code)
	}
}

func TestAnomalyHandlerOutlierGatewayNotFound404(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ uuid.UUID, _ *float64) error {
			return domain.ErrGatewayNotFound
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux(routeSensorAnomalyOutlier, h.InjectOutlier,
		newReq(http.MethodPost, pathSensor5AnomalyOutlier,
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing gateway, got %d", w.Code)
	}
}

func TestAnomalyHandlerOutlierServiceError500(t *testing.T) {
	ctrl := &fakes.FakeSimulatorControlService{
		InjectSensorOutlierFn: func(_ context.Context, _ uuid.UUID, _ *float64) error {
			return fakes.ErrSimulated
		},
	}
	h := simhttp.NewAnomalyHandler(ctrl)

	w := serveWithMux(routeSensorAnomalyOutlier, h.InjectOutlier,
		newReq(http.MethodPost, pathSensor5AnomalyOutlier,
			jsonBody(t, map[string]any{})))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 on service error, got %d", w.Code)
	}
}

func TestAnomalyHandlerOutlierBadBody400(t *testing.T) {
	h := simhttp.NewAnomalyHandler(&fakes.FakeSimulatorControlService{})

	w := serveWithMux(routeSensorAnomalyOutlier, h.InjectOutlier,
		newReq(http.MethodPost, pathSensor5AnomalyOutlier,
			bytes.NewReader([]byte(invalidJSONBody))))
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for bad body, got %d", w.Code)
	}
}
