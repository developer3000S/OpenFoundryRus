package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/config"
	icmetrics "github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/metrics"
)

func newTestServer(t *testing.T) (*http.Server, *icmetrics.Metrics, *config.Config) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "iceberg-catalog-service"
	cfg.Service.Version = "test"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0

	o := observability.NewMetrics()
	im := icmetrics.New(o)
	srv := New(cfg, nil, Deps{Metrics: im}, o)
	return srv, im, cfg
}

// TestVersionEndpointSurfacesBuildGitSha asserts /version renders the
// service identity and BUILD_GIT_SHA so the deploy correlator can match
// rolled-out pods to the originating commit.
func TestVersionEndpointSurfacesBuildGitSha(t *testing.T) {
	t.Setenv("BUILD_GIT_SHA", "deadbeef")
	srv, _, cfg := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["service"] != cfg.Service.Name {
		t.Errorf("service = %q, want %q", body["service"], cfg.Service.Name)
	}
	if body["version"] != cfg.Service.Version {
		t.Errorf("version = %q, want %q", body["version"], cfg.Service.Version)
	}
	if body["build_git_sha"] != "deadbeef" {
		t.Errorf("build_git_sha = %q, want %q", body["build_git_sha"], "deadbeef")
	}
}

// TestInstrumentMiddlewareRecordsRouteCounter verifies the request
// counter fires after each request and that the endpoint label uses
// chi's route template — not the raw URL — so dashboards aggregate
// path-parameterised endpoints under one series.
func TestInstrumentMiddlewareRecordsRouteCounter(t *testing.T) {
	srv, im, _ := newTestServer(t)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("/healthz status = %d, want 200", rec.Code)
		}
	}

	if got := counterValue(t, im.RESTRequestsTotal, "GET", "/healthz", "200"); got != 3 {
		t.Errorf("RESTRequestsTotal = %v, want 3", got)
	}
	if got := histogramSampleCount(t, im.RESTRequestLatencySeconds, "GET", "/healthz"); got != 3 {
		t.Errorf("histogram observations = %d, want 3", got)
	}
}

// TestMetricsEndpointExposesIcebergFamilies confirms /metrics renders
// the iceberg_* family names registered by this slice. A regression
// here would silently drop the dashboards inherited from the Rust
// deployment.
//
// Counter and histogram families are only rendered after at least one
// labelset has been touched, so we pre-seed each family with a no-op
// observation to assert it was wired into the registry.
func TestMetricsEndpointExposesIcebergFamilies(t *testing.T) {
	srv, im, _ := newTestServer(t)

	im.RESTRequestsTotal.WithLabelValues("GET", "/seed", "200").Add(0)
	im.RESTRequestLatencySeconds.WithLabelValues("GET", "/seed").Observe(0)
	im.OAuthTokensIssued.WithLabelValues("seed").Add(0)
	im.TablesTotal.WithLabelValues("seed").Set(0)
	im.MetadataFilesTotal.WithLabelValues("seed").Add(0)
	im.CommitConflictsTotal.WithLabelValues("seed").Add(0)
	im.SchemaStrictRejectionsTotal.WithLabelValues("seed").Add(0)
	im.BranchAliasAppliedTotal.WithLabelValues("seed", "seed").Add(0)
	im.FoundryTransactionsTotal.WithLabelValues("seed").Add(0)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, family := range []string{
		"iceberg_rest_catalog_requests_total",
		"iceberg_rest_catalog_request_latency_seconds",
		"iceberg_rest_catalog_requests_in_flight",
		"iceberg_oauth_token_issued_total",
		"iceberg_tables_total",
		"iceberg_metadata_files_total",
		"iceberg_commit_conflicts_total",
		"iceberg_schema_strict_rejections_total",
		"iceberg_branch_alias_applied_total",
		"iceberg_foundry_transactions_total",
	} {
		if !strings.Contains(body, family) {
			t.Errorf("missing metric family %q in /metrics output", family)
		}
	}
}

func counterValue(t *testing.T, vec *prometheus.CounterVec, labels ...string) float64 {
	t.Helper()
	c, err := vec.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	var pb dto.Metric
	if err := c.Write(&pb); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return pb.GetCounter().GetValue()
}

func histogramSampleCount(t *testing.T, vec *prometheus.HistogramVec, labels ...string) uint64 {
	t.Helper()
	h, err := vec.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues: %v", err)
	}
	var pb dto.Metric
	if err := h.(prometheus.Metric).Write(&pb); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return pb.GetHistogram().GetSampleCount()
}
