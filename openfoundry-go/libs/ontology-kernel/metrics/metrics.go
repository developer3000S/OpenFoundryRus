// Package metrics is the Go port of `libs/ontology-kernel/src/metrics.rs`.
//
// TASK F — Action metrics surface for the ontology actions service.
// Every binary calls [RegisterActionMetrics] once at startup, then
// handlers grab the singleton via [ActionMetricsSingleton] and call
// the record helpers.
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// FailureType mirrors `enum FailureType` — the `failure_type` label of
// `action_failures_total`.
type FailureType int

const (
	FailureTypeInvalidParameter FailureType = iota
	FailureTypeScaleLimit
	FailureTypeAuthentication
	FailureTypeSideEffect
	FailureTypeFunction
	FailureTypeUserFacingFunction
	FailureTypeConflict
	FailureTypeUnclassified
)

// AsStr mirrors `impl FailureType::as_str(self)`.
func (f FailureType) AsStr() string {
	switch f {
	case FailureTypeInvalidParameter:
		return "invalid_parameter"
	case FailureTypeScaleLimit:
		return "scale_limit"
	case FailureTypeAuthentication:
		return "authentication"
	case FailureTypeSideEffect:
		return "side_effect"
	case FailureTypeFunction:
		return "function"
	case FailureTypeUserFacingFunction:
		return "user_facing_function"
	case FailureTypeConflict:
		return "conflict"
	case FailureTypeUnclassified:
		return "unclassified"
	}
	return "unclassified"
}

// FailureTypeFromHTTPStatus mirrors `FailureType::from_http_status`.
func FailureTypeFromHTTPStatus(status int) FailureType {
	switch {
	case status == 400 || status == 422:
		return FailureTypeInvalidParameter
	case status == 401 || status == 403:
		return FailureTypeAuthentication
	case status == 409:
		return FailureTypeConflict
	case status == 429:
		return FailureTypeScaleLimit
	case status >= 500 && status <= 599:
		return FailureTypeSideEffect
	default:
		return FailureTypeUnclassified
	}
}

// ActionMetrics mirrors `struct ActionMetrics`.
type ActionMetrics struct {
	ExecutionsTotal          *prometheus.CounterVec
	ExecutionDurationSeconds *prometheus.HistogramVec
	FailuresTotal            *prometheus.CounterVec
}

// RecordSuccess mirrors `ActionMetrics::record_success`.
func (m *ActionMetrics) RecordSuccess(actionID string, durationSeconds float64) {
	m.ExecutionsTotal.WithLabelValues(actionID, "success").Inc()
	m.ExecutionDurationSeconds.WithLabelValues(actionID).Observe(durationSeconds)
}

// RecordFailure mirrors `ActionMetrics::record_failure`.
func (m *ActionMetrics) RecordFailure(actionID string, failureType FailureType, durationSeconds float64) {
	m.ExecutionsTotal.WithLabelValues(actionID, "failure").Inc()
	m.ExecutionDurationSeconds.WithLabelValues(actionID).Observe(durationSeconds)
	m.FailuresTotal.WithLabelValues(actionID, failureType.AsStr()).Inc()
}

// exponentialBuckets mirrors `prometheus::exponential_buckets(0.005, 2.0, 10)` —
// 10 buckets starting at 5ms, doubling each step, ending at ~2.56s.
func exponentialBuckets(start, factor float64, count int) []float64 {
	out := make([]float64, count)
	v := start
	for i := 0; i < count; i++ {
		out[i] = v
		v *= factor
	}
	return out
}

// newActionMetrics mirrors `impl ActionMetrics::new()`.
func newActionMetrics() *ActionMetrics {
	buckets := exponentialBuckets(0.005, 2.0, 10)
	return &ActionMetrics{
		ExecutionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "action_executions_total",
				Help: "Total ontology action executions, partitioned by action and outcome",
			},
			[]string{"action_id", "status"},
		),
		ExecutionDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "action_execution_duration_seconds",
				Help:    "Wall-clock duration of ontology action executions",
				Buckets: buckets,
			},
			[]string{"action_id"},
		),
		FailuresTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "action_failures_total",
				Help: "Failed ontology action executions, partitioned by failure_type",
			},
			[]string{"action_id", "failure_type"},
		),
	}
}

// Singleton mirrors the `OnceLock<ActionMetrics>` in metrics.rs.
var (
	metricsOnce      sync.Once
	metricsSingleton *ActionMetrics
)

// RegisterActionMetrics mirrors `register_action_metrics`. Initialises
// the singleton on first call and registers each collector against
// the supplied registry; subsequent calls re-register against the new
// registry but do not reset the singleton (multi-binary tests share
// counter state, mirroring Rust `OnceLock` behaviour).
//
// Re-registering the same collector twice in client_golang yields
// `prometheus.AlreadyRegisteredError`, which we silently ignore so
// the contract matches Rust's `let _ = registry.register(...)`.
func RegisterActionMetrics(registry *prometheus.Registry) *ActionMetrics {
	metricsOnce.Do(func() { metricsSingleton = newActionMetrics() })
	// Match Rust `let _ = registry.register(...)` — re-registration of
	// the same collector returns AlreadyRegisteredError, which we
	// silently ignore so multi-binary tests stay safe.
	_ = registry.Register(metricsSingleton.ExecutionsTotal)
	_ = registry.Register(metricsSingleton.ExecutionDurationSeconds)
	_ = registry.Register(metricsSingleton.FailuresTotal)
	return metricsSingleton
}

// ActionMetricsSingleton mirrors `action_metrics()` — returns nil if
// no binary has called [RegisterActionMetrics] yet.
func ActionMetricsSingleton() *ActionMetrics {
	return metricsSingleton
}
