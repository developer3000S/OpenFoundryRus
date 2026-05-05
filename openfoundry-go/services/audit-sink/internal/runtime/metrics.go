// Package runtime is the audit-sink Kafka → Writer batching loop.
package runtime

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metric names — pinned constants matching the Rust crate. Renaming
// breaks the dashboards + alerts in `infra/observability/`.
const (
	MetricLagSeconds    = "audit_sink_lag_seconds"
	MetricRecordsTotal  = "audit_sink_records_total"
	MetricBatchSize     = "audit_sink_batch_size_records"
	MetricCommitsTotal  = "audit_sink_commits_total"
)

// Metrics bundles the gauges/histograms emitted by the runtime.
type Metrics struct {
	Registry      *prometheus.Registry
	LagSeconds    *prometheus.HistogramVec
	RecordsTotal  *prometheus.CounterVec
	BatchSize     *prometheus.HistogramVec
	CommitsTotal  *prometheus.CounterVec
}

// NewMetrics builds a fresh Registry + the four sink metrics. Bucket
// boundaries match the Rust crate so PromQL queries port unchanged.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	lag := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    MetricLagSeconds,
		Help:    "Seconds between audit event production time and successful Iceberg append.",
		Buckets: []float64{0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0},
	}, []string{"table"})

	records := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: MetricRecordsTotal,
		Help: "Total audit records appended.",
	}, []string{"table"})

	batchSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    MetricBatchSize,
		Help:    "Audit records per successful append.",
		Buckets: []float64{1, 10, 100, 1_000, 10_000, 50_000, 100_000},
	}, []string{"table"})

	commits := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: MetricCommitsTotal,
		Help: "Append attempts by outcome (success | failure | poison).",
	}, []string{"table", "outcome"})

	reg.MustRegister(lag, records, batchSize, commits)

	return &Metrics{
		Registry:     reg,
		LagSeconds:   lag,
		RecordsTotal: records,
		BatchSize:    batchSize,
		CommitsTotal: commits,
	}
}
