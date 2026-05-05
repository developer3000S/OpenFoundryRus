// Package runtime is the ai-sink Kafka → Writer batching loop.
package runtime

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metric names — pinned to match the Rust crate. Rename = dashboard break.
const (
	MetricLagSeconds   = "ai_sink_lag_seconds"
	MetricRecordsTotal = "ai_sink_records_total"
	MetricBatchSize    = "ai_sink_batch_size_records"
	MetricCommitsTotal = "ai_sink_commits_total"
)

// Metrics bundles the gauges/histograms emitted by the runtime.
//
// All metrics are labelled by the Iceberg target table
// (prompts|responses|evaluations|traces) so per-table SLOs land cleanly.
type Metrics struct {
	Registry     *prometheus.Registry
	LagSeconds   *prometheus.HistogramVec
	RecordsTotal *prometheus.CounterVec
	BatchSize    *prometheus.HistogramVec
	CommitsTotal *prometheus.CounterVec
}

// NewMetrics builds a fresh Registry + the four sink metrics.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	lag := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    MetricLagSeconds,
		Help:    "Seconds between AI event production time and successful Iceberg append.",
		Buckets: []float64{0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0},
	}, []string{"table"})

	records := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: MetricRecordsTotal,
		Help: "Total AI records appended to Iceberg.",
	}, []string{"table"})

	batchSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    MetricBatchSize,
		Help:    "AI records per successful Iceberg append.",
		Buckets: []float64{1, 10, 100, 1_000, 10_000, 50_000, 100_000},
	}, []string{"table"})

	commits := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: MetricCommitsTotal,
		Help: "Iceberg append attempts by outcome (success | failure | poison).",
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
