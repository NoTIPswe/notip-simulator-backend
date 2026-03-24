package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
	"github.com/google/uuid"
)

type AnomalyHandler struct {
	control ports.SimulatorControlService
	store   ports.GatewayStore
}

func NewAnomalyHandler(control ports.SimulatorControlService, store ports.GatewayStore) *AnomalyHandler {
	return &AnomalyHandler{control: control, store: store}
}

func (h *AnomalyHandler) InjectNetworkDegradation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidformat, http.StatusBadRequest)
		return
	}

	var req struct {
		DurationSeconds int     `json:"duration_seconds"`
		PacketLossPct   float64 `json:"packet_loss_pct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.PacketLossPct == 0 {
		req.PacketLossPct = 0.3
	}

	cmd := domain.GatewayAnomalyCommand{
		Type: domain.NetworkDegradation,
		NetworkDegradation: &domain.NetworkDegradationParams{
			DurationSeconds: req.DurationSeconds,
			PacketLossPct:   req.PacketLossPct,
		},
	}

	if err := h.control.InjectGatewayAnomaly(r.Context(), id, cmd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AnomalyHandler) InjectDisconnect(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidformat, http.StatusBadRequest)
		return
	}

	var req struct {
		DurationSeconds int `json:"duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.DurationSeconds <= 0 {
		http.Error(w, "duration_seconds is required and must be > 0", http.StatusBadRequest)
		return
	}

	cmd := domain.GatewayAnomalyCommand{
		Type: domain.Disconnect,
		Disconnect: &domain.DisconnectParams{
			DurationSeconds: req.DurationSeconds,
		},
	}

	if err := h.control.InjectGatewayAnomaly(r.Context(), id, cmd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AnomalyHandler) InjectOutlier(w http.ResponseWriter, r *http.Request) {
	sensorID, err := strconv.ParseInt(r.PathValue("sensorId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid sensor ID format", http.StatusBadRequest)
		return
	}

	var req struct {
		Value *float64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	//Find the sensor via SQLite.
	sensor, err := h.store.GetSensor(r.Context(), sensorID)
	if err != nil {
		http.Error(w, "sensor not found", http.StatusNotFound)
		return
	}

	//Find the Gateway to extract the ID.
	gw, err := h.store.GetGateway(r.Context(), sensor.GatewayID)
	if err != nil {
		http.Error(w, "associated gateway not found", http.StatusInternalServerError)
		return
	}

	// Prepare the command.
	cmd := domain.SensorOutlierCommand{
		SensorID: sensor.SensorID,
		Value:    req.Value, //If nil, the Worker has the default (sensor.MaxRange * 2.0).
	}

	if err := h.control.InjectSensorOutlier(r.Context(), gw.ManagementGatewayID, cmd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
