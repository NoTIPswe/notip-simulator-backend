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
}

func NewAnomalyHandler(control ports.SimulatorControlService) *AnomalyHandler {
	return &AnomalyHandler{control: control}
}

func (h *AnomalyHandler) InjectNetworkDegradation(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
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
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AnomalyHandler) InjectDisconnect(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
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
		writeError(w, err)
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

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req struct {
		Value *float64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.control.InjectSensorOutlier(r.Context(), sensorID, req.Value); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
