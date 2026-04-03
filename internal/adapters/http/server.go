package http

import (
	"context"
	"net/http"
	"time"

	"github.com/NoTIPswe/notip-simulator-backend/internal/health"
)

type HTTPServer struct {
	addr   string
	mux    *http.ServeMux
	server *http.Server
}

func NewHTTPServer(
	addr string,
	gwHandler *GatewayHandler,
	sensorHandler *SensorHandler,
	anomalyHandler *AnomalyHandler,
) *HTTPServer {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", health.Handler)

	// Gateways
	mux.HandleFunc("POST /sim/gateways", gwHandler.Create)
	mux.HandleFunc("POST /sim/gateways/bulk", gwHandler.BulkCreate)
	mux.HandleFunc("GET /sim/gateways", gwHandler.List)
	mux.HandleFunc("GET /sim/gateways/{id}", gwHandler.Get)
	mux.HandleFunc("POST /sim/gateways/{id}/start", gwHandler.Start)
	mux.HandleFunc("POST /sim/gateways/{id}/stop", gwHandler.Stop)
	mux.HandleFunc("DELETE /sim/gateways/{id}", gwHandler.Delete)

	// Sensors.
	mux.HandleFunc("POST /sim/gateways/{id}/sensors", sensorHandler.Add)
	mux.HandleFunc("GET /sim/gateways/{id}/sensors", sensorHandler.List)
	mux.HandleFunc("DELETE /sim/sensors/{sensorId}", sensorHandler.Delete)

	// Anomalies.
	mux.HandleFunc("POST /sim/gateways/{id}/anomaly/network-degradation", anomalyHandler.InjectNetworkDegradation)
	mux.HandleFunc("POST /sim/gateways/{id}/anomaly/disconnect", anomalyHandler.InjectDisconnect)
	mux.HandleFunc("POST /sim/sensors/{sensorId}/anomaly/outlier", anomalyHandler.InjectOutlier)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return &HTTPServer{addr: addr, mux: mux, server: srv}
}

func (s *HTTPServer) Start() error {
	return s.server.ListenAndServe()
}

func (s *HTTPServer) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *HTTPServer) Handler() http.Handler {
	return s.server.Handler
}
