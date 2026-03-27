package http

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/NoTIPswe/notip-simulator-backend/internal/adapters/http/dto"
	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
	"github.com/NoTIPswe/notip-simulator-backend/internal/ports"
	"github.com/google/uuid"
)

const invalidGatewayIDFormat = "invalid gateway ID format"
const contentType = "Content-Type"
const contentTypeJSON = "application/json"
const maxBodyBytes = 1 << 20 // 1 MiB

type GatewayHandler struct {
	lifecycle ports.GatewayLifecycleService
	control   ports.SimulatorControlService
}

func NewGatewayHandler(lifecycle ports.GatewayLifecycleService, control ports.SimulatorControlService) *GatewayHandler {
	return &GatewayHandler{
		lifecycle: lifecycle,
		control:   control,
	}
}

func (h *GatewayHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req domain.CreateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	gw, err := h.lifecycle.CreateAndStart(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(dto.GatewayFromDomain(gw)); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func (h *GatewayHandler) Start(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	if err := h.lifecycle.Start(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GatewayHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	if err := h.lifecycle.Stop(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GatewayHandler) Decommission(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	if err := h.lifecycle.Decommission(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GatewayHandler) List(w http.ResponseWriter, r *http.Request) {
	gateways, err := h.lifecycle.ListGateways(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(dto.GatewayListFromDomain(gateways)); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func (h *GatewayHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	gw, err := h.lifecycle.GetGateway(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set(contentType, contentTypeJSON)
	if err := json.NewEncoder(w).Encode(dto.GatewayFromDomain(gw)); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}

func (h *GatewayHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, invalidGatewayIDFormat, http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var update domain.GatewayConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.control.UpdateConfig(r.Context(), id, update); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *GatewayHandler) BulkCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req domain.BulkCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	gateways, errs := h.lifecycle.BulkCreateGateways(r.Context(), req)
	stringErrs := make([]string, len(errs))
	hasErrors := false
	for i, err := range errs {
		if err != nil {
			stringErrs[i] = err.Error()
			hasErrors = true
		}
	}

	response := struct {
		Gateways []dto.GatewayResponse `json:"gateways"`
		Errors   []string              `json:"errors"`
	}{
		Gateways: dto.GatewayListFromDomain(gateways),
		Errors:   stringErrs,
	}

	w.Header().Set(contentType, contentTypeJSON)
	if hasErrors {
		w.WriteHeader(http.StatusMultiStatus)
	} else {
		w.WriteHeader(http.StatusCreated)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode response", "err", err)
	}
}
