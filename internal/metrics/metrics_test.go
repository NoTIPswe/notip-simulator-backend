package metrics_test

import (
	"testing"

	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
)

func TestNewTestMetrics_ReturnsNonNil(t *testing.T) {
	m := metrics.NewTestMetrics()
	if m == nil {
		t.Fatal("expected non-nil Metrics")
	}
}

func TestMetrics_GaugeOperations_NoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.GatewaysRunning.Set(3)
	m.GatewaysRunning.Inc()
	m.GatewaysRunning.Dec()
}

func TestMetrics_CounterVec_NoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.EnvelopesPublished.WithLabelValues("gw-test").Inc()
	m.PublishErrors.WithLabelValues("gw-test").Inc()
	m.BufferDropped.WithLabelValues("gw-test").Inc()
	m.NATSReconnects.WithLabelValues("gw-test").Inc()
	m.AnomaliesInjected.WithLabelValues("network_degradation").Inc()
}

func TestMetrics_GaugeVec_NoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.BufferFill.WithLabelValues("gw-test").Set(5)
}

func TestMetrics_Counters_NoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.ProvisioningSuccess.Inc()
	m.ProvisioningErrors.Inc()
}

func TestNewTestMetrics_MultipleInstances_NoPanic(t *testing.T) {
	// NewTestMetrics deve usare registries isolate (non il registry globale di Prometheus)
	m1 := metrics.NewTestMetrics()
	m2 := metrics.NewTestMetrics()
	m1.GatewaysRunning.Set(1)
	m2.GatewaysRunning.Set(2)
}

func TestNewMetricsReturnsUsableCollectors(t *testing.T) {
	m := metrics.NewMetrics()
	if m == nil {
		t.Fatal("expected non-nil Metrics")
	}

	m.GatewaysRunning.Inc()
	m.GatewaysRunning.Dec()
	m.EnvelopesPublished.WithLabelValues("gw1").Inc()
	m.PublishErrors.WithLabelValues("gw1").Inc()
	m.BufferDropped.WithLabelValues("gw1").Inc()
	m.BufferFill.WithLabelValues("gw1").Set(1)
	m.ProvisioningSuccess.Inc()
	m.ProvisioningErrors.Inc()
	m.NATSReconnects.WithLabelValues("gw1").Inc()
	m.AnomaliesInjected.WithLabelValues("disconnect").Inc()
}
