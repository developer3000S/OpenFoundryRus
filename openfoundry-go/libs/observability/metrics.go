package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns a fresh Prometheus registry pre-populated with Go
// runtime + process collectors so every service exposes a baseline
// matching the Rust workspace defaults.
//
// Service-specific metrics should be registered via Metrics.Register.
type Metrics struct {
	Registry *prometheus.Registry
}

// NewMetrics builds a Metrics with the standard runtime + process collectors registered.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return &Metrics{Registry: reg}
}

// Register registers a collector and panics if it has already been
// registered — same semantics as `prometheus::register!` macro.
func (m *Metrics) Register(c prometheus.Collector) {
	m.Registry.MustRegister(c)
}

// Handler returns the http.Handler to mount at /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{Registry: m.Registry})
}
