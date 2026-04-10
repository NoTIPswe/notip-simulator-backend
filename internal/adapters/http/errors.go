package http

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/NoTIPswe/notip-simulator-backend/internal/domain"
)

// writeError maps domain sentinel errors to HTTP status codes.
// Unknown errors are logged and hidden behind a generic 500.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrGatewayNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, domain.ErrSensorNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, domain.ErrGatewayAlreadyRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, domain.ErrGatewayAlreadyProvisioned):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, domain.ErrInvalidFactoryCredentials):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, domain.ErrInvalidSensorRange):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		slog.Error("internal error", "err", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
