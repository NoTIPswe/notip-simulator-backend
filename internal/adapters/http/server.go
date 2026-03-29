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
	simToken string,
	gwHandler *GatewayHandler,
	sensorHandler *SensorHandler,
	anomalyHandler *AnomalyHandler,
) *HTTPServer {
	mux := http.NewServeMux()

	// Health — no auth.
	mux.HandleFunc("GET /health", health.Handler)

	// Sensors e Anomalies — no auth, simulation dashboard only.
	mux.HandleFunc("POST /sim/gateways/{id}/sensors", sensorHandler.Add)
	mux.HandleFunc("GET /sim/gateways/{id}/sensors", sensorHandler.List)
	mux.HandleFunc("DELETE /sim/sensors/{sensorId}", sensorHandler.Delete)
	mux.HandleFunc("POST /sim/gateways/{id}/anomaly/network-degradation", anomalyHandler.InjectNetworkDegradation)
	mux.HandleFunc("POST /sim/gateways/{id}/anomaly/disconnect", anomalyHandler.InjectDisconnect)
	mux.HandleFunc("POST /sim/sensors/{sensorId}/anomaly/outlier", anomalyHandler.InjectOutlier)

	// Gateways — requires SimTokenSecret, called by mgmt API.
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("POST /sim/gateways", gwHandler.Create)
	protectedMux.HandleFunc("POST /sim/gateways/bulk", gwHandler.BulkCreate)
	protectedMux.HandleFunc("GET /sim/gateways", gwHandler.List)
	protectedMux.HandleFunc("GET /sim/gateways/{id}", gwHandler.Get)
	protectedMux.HandleFunc("POST /sim/gateways/{id}/start", gwHandler.Start)
	protectedMux.HandleFunc("POST /sim/gateways/{id}/stop", gwHandler.Stop)
	protectedMux.HandleFunc("DELETE /sim/gateways/{id}", gwHandler.Decommission)
	protectedMux.HandleFunc("PATCH /sim/gateways/{id}/config", gwHandler.UpdateConfig)

	mux.Handle("/sim/gateways", SimTokenMiddleware(simToken, protectedMux))
	mux.Handle("/sim/gateways/", SimTokenMiddleware(simToken, protectedMux))

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
