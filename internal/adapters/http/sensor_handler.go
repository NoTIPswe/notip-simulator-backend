package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
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
	gwID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid gateway ID format", http.StatusBadRequest)
		return
	}

	var sensor domain.SimSensor
	if err := json.NewDecoder(r.Body).Decode(&sensor); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	createdSensor, err := h.sensors.AddSensor(r.Context(), gwID, sensor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(createdSensor); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func (h *SensorHandler) List(w http.ResponseWriter, r *http.Request) {
	gwID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid gateway ID format", http.StatusBadRequest)
		return
	}

	sensors, err := h.sensors.ListSensors(r.Context(), gwID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(sensors); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func (h *SensorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	sensorID, err := strconv.ParseInt(r.PathValue("sensorId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid sensor ID format", http.StatusBadRequest)
		return
	}

	if err := h.sensors.DeleteSensor(r.Context(), sensorID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
