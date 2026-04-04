package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http/dto"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
)

type SensorHandler struct {
	sensors ports.SensorManagementService
}

func NewSensorHandler(sensors ports.SensorManagementService) *SensorHandler {
	return &SensorHandler{
		sensors: sensors,
	}
}

func (h *SensorHandler) Add(w http.ResponseWriter, r *http.Request) {
	gwID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req dto.CreateSensorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	createdSensor, err := h.sensors.AddSensor(r.Context(), gwID, req.ToDomain())
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(dto.SensorFromDomain(createdSensor)); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func (h *SensorHandler) List(w http.ResponseWriter, r *http.Request) {
	gwID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	sensors, err := h.sensors.ListSensors(r.Context(), gwID)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(dto.SensorListFromDomain(sensors)); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func (h *SensorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sensorID, err := uuid.Parse(r.PathValue("sensorId"))
	if err != nil {
		http.Error(w, "invalid sensor ID format", http.StatusBadRequest)
		return
	}

	if err := h.sensors.DeleteSensor(r.Context(), sensorID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
