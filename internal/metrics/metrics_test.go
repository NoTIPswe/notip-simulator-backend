package metrics_test

import (
	"testing"

	"github.com/NoTIPswe/notip-simulator-backend/internal/metrics"
)

func TestNewTestMetricsReturnsNonNil(t *testing.T) {
	m := metrics.NewTestMetrics()
	if m == nil {
		t.Fatal("expected non-nil Metrics")
	}
}

func TestMetricsGaugeOperationsNoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.GatewaysRunning.Set(3)
	m.GatewaysRunning.Inc()
	m.GatewaysRunning.Dec()
}

const testGatewayID = "gw-test"

func TestMetricsCounterVecNoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.EnvelopesPublished.WithLabelValues(testGatewayID).Inc()
	m.PublishErrors.WithLabelValues(testGatewayID).Inc()
	m.BufferDropped.WithLabelValues(testGatewayID).Inc()
	m.NATSReconnects.WithLabelValues(testGatewayID).Inc()
	m.AnomaliesInjected.WithLabelValues("network_degradation").Inc()
}

func TestMetricsGaugeVecNoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.BufferFill.WithLabelValues(testGatewayID).Set(5)
}

func TestMetricsCountersNoPanic(t *testing.T) {
	m := metrics.NewTestMetrics()
	m.ProvisioningSuccess.Inc()
	m.ProvisioningErrors.Inc()
}

func TestNewTestMetricsMultipleInstancesNoPanic(t *testing.T) {
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
