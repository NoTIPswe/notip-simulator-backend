package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	GatewaysRunning     prometheus.Gauge
	EnvelopesPublished  *prometheus.CounterVec
	PublishErrors       *prometheus.CounterVec
	BufferDropped       *prometheus.CounterVec
	BufferFill          *prometheus.GaugeVec
	ProvisioningSuccess prometheus.Counter
	ProvisioningErrors  prometheus.Counter
	NATSReconnects      *prometheus.CounterVec
	AnomaliesInjected   *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	return &Metrics{
		GatewaysRunning: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "notip_sim_gateways_running",
			Help: "Current running worker count",
		}),
		EnvelopesPublished: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "notip_sim_envelopes_published_total",
			Help: "Succesfull NATS publish attempts",
		}, []string{"gateway_id"}),
		PublishErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "notip_sim_publish_errors_total",
			Help: "Failed publishes",
		}, []string{"gateway_id"}),
		BufferDropped: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "notip_sim_buffer_dropped_total",
			Help: "Messages dropped on overflow",
		}, []string{"gateway_id"}),
		BufferFill: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "notip_sim_buffer_fill_level",
			Help: "Current buffer occupancy",
		}, []string{"gateway_id"}),
		ProvisioningSuccess: promauto.NewCounter(prometheus.CounterOpts{
			Name: "notip_sim_provisioning_success_total",
		}),
		ProvisioningErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "notip_sim_provisioning_errors_total",
		}),
		NATSReconnects: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "notip_sim_nats_reconnects_total",
		}, []string{"gateway_id"}),
		AnomaliesInjected: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "notip_sim_anomalies_injected_total",
		}, []string{"type"}),
	}
}

// NewTestMetrics creates an istance of the metrics for the tests.
// it's useful to avoid the duplicate registration from the global registry of Prometheus.
func NewTestMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)

	return &Metrics{
		GatewaysRunning:     factory.NewGauge(prometheus.GaugeOpts{Name: "notip_sim_gateways_running"}),
		EnvelopesPublished:  factory.NewCounterVec(prometheus.CounterOpts{Name: "notip_sim_envelopes_published_total"}, []string{"gateway_id"}),
		PublishErrors:       factory.NewCounterVec(prometheus.CounterOpts{Name: "notip_sim_publish_errors_total"}, []string{"gateway_id"}),
		BufferDropped:       factory.NewCounterVec(prometheus.CounterOpts{Name: "notip_sim_buffer_dropped_total"}, []string{"gateway_id"}),
		BufferFill:          factory.NewGaugeVec(prometheus.GaugeOpts{Name: "notip_sim_buffer_fill_level"}, []string{"gateway_id"}),
		ProvisioningSuccess: factory.NewCounter(prometheus.CounterOpts{Name: "notip_sim_provisioning_success_total"}),
		ProvisioningErrors:  factory.NewCounter(prometheus.CounterOpts{Name: "notip_sim_provisioning_errors_total"}),
		NATSReconnects:      factory.NewCounterVec(prometheus.CounterOpts{Name: "notip_sim_nats_reconnects_total"}, []string{"gateway_id"}),
		AnomaliesInjected:   factory.NewCounterVec(prometheus.CounterOpts{Name: "notip_sim_anomalies_injected_total"}, []string{"type"}),
	}
}
