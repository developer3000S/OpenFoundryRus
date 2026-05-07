// Package metrics declares the Prometheus families specific to
// iceberg-catalog-service. Mirrors services/iceberg-catalog-service/src/metrics.rs
// — names, labels and help text are byte-for-byte identical so the
// dashboards and alerts wired against the Rust crate keep firing
// against the Go service unchanged.
//
// The HTTP instrumentation families (RESTRequestsTotal,
// RESTRequestLatencySeconds, RESTRequestsInFlight) are populated by the
// chi middleware in package server. The remaining families
// (OAuth tokens, tables-by-format, commit conflicts, etc.) are surfaced
// directly from the relevant handlers.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/openfoundry/openfoundry-go/libs/observability"
)

// Metrics owns the typed Prometheus handles. Instantiated once in
// cmd/iceberg-catalog-service/main.go and threaded through the server
// Deps so handlers and middleware share the same registered families.
type Metrics struct {
	// RESTRequestsTotal counts every request handled by the chi router,
	// labelled by method, route pattern and HTTP status. Mirrors
	// `iceberg_rest_catalog_requests_total` from the Rust crate.
	RESTRequestsTotal *prometheus.CounterVec
	// RESTRequestLatencySeconds is the per-route latency histogram.
	// Buckets follow the platform default (5ms…10s, geometric).
	RESTRequestLatencySeconds *prometheus.HistogramVec
	// RESTRequestsInFlight is a gauge of currently-handled requests
	// per route — useful to spot saturation independent of latency.
	RESTRequestsInFlight *prometheus.GaugeVec

	// OAuthTokensIssued counts OAuth2 tokens minted, by grant_type.
	OAuthTokensIssued *prometheus.CounterVec
	// TablesTotal tracks the live count of catalog tables, by
	// format-version. Set by the table CRUD handlers.
	TablesTotal *prometheus.GaugeVec
	// MetadataFilesTotal counts metadata.json files written, per table.
	MetadataFilesTotal *prometheus.CounterVec

	// CommitConflictsTotal counts multi-table commit conflicts surfaced
	// as HTTP 409 to the build executor.
	CommitConflictsTotal *prometheus.CounterVec
	// SchemaStrictRejectionsTotal counts commits rejected by the
	// strict-mode schema enforcer, per delta kind.
	SchemaStrictRejectionsTotal *prometheus.CounterVec
	// BranchAliasAppliedTotal counts master/main alias rewrites that
	// the branch resolver applies on read or write.
	BranchAliasAppliedTotal *prometheus.CounterVec
	// FoundryTransactionsTotal is the FoundryIcebergTxn lifecycle
	// counter (begin/commit/abort).
	FoundryTransactionsTotal *prometheus.CounterVec
}

// New registers the families on the supplied observability registry
// and returns the typed handle.
func New(o *observability.Metrics) *Metrics {
	m := &Metrics{
		RESTRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "iceberg_rest_catalog_requests_total",
				Help: "REST Catalog requests by method, endpoint and HTTP status",
			},
			[]string{"method", "endpoint", "status"},
		),
		RESTRequestLatencySeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "iceberg_rest_catalog_request_latency_seconds",
				Help:    "REST Catalog request latency in seconds, by method and endpoint",
				Buckets: prometheus.ExponentialBuckets(0.005, 2, 12),
			},
			[]string{"method", "endpoint"},
		),
		RESTRequestsInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "iceberg_rest_catalog_requests_in_flight",
				Help: "REST Catalog requests currently being handled, by method and endpoint",
			},
			[]string{"method", "endpoint"},
		),
		OAuthTokensIssued: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "iceberg_oauth_token_issued_total",
				Help: "OAuth2 tokens issued by grant_type",
			},
			[]string{"grant_type"},
		),
		TablesTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "iceberg_tables_total",
				Help: "Number of Iceberg tables tracked by the catalog by format_version",
			},
			[]string{"format_version"},
		),
		MetadataFilesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "iceberg_metadata_files_total",
				Help: "Cumulative count of v{N}.metadata.json files written",
			},
			[]string{"table_uuid"},
		),
		CommitConflictsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "iceberg_commit_conflicts_total",
				Help: "Multi-table commit conflicts surfaced as 409 Retryable to the build executor",
			},
			[]string{"conflicting_with"},
		),
		SchemaStrictRejectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "iceberg_schema_strict_rejections_total",
				Help: "Commits rejected by strict-mode schema enforcement (per delta kind)",
			},
			[]string{"delta_kind"},
		),
		BranchAliasAppliedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "iceberg_branch_alias_applied_total",
				Help: "Master/main alias rewrites applied per Foundry doc § Default branches",
			},
			[]string{"from", "to"},
		),
		FoundryTransactionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "iceberg_foundry_transactions_total",
				Help: "FoundryIcebergTxn lifecycle counter (begin/commit/abort)",
			},
			[]string{"lifecycle"},
		),
	}
	o.Register(m.RESTRequestsTotal)
	o.Register(m.RESTRequestLatencySeconds)
	o.Register(m.RESTRequestsInFlight)
	o.Register(m.OAuthTokensIssued)
	o.Register(m.TablesTotal)
	o.Register(m.MetadataFilesTotal)
	o.Register(m.CommitConflictsTotal)
	o.Register(m.SchemaStrictRejectionsTotal)
	o.Register(m.BranchAliasAppliedTotal)
	o.Register(m.FoundryTransactionsTotal)
	return m
}
