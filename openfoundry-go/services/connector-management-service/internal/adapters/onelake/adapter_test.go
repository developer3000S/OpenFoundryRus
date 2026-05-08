package onelake

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

func TestValidateConfigRejectsMissingWorkspace(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatalf("expected error for empty config")
	}
	if err := ValidateConfig(json.RawMessage(`{"lakehouse":"l","oauth_token":"t"}`)); err == nil {
		t.Fatalf("expected error when 'workspace' is missing")
	}
}

func TestValidateConfigRejectsMissingLakehouse(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{"workspace":"w","oauth_token":"t"}`)); err == nil {
		t.Fatalf("expected error when 'lakehouse' is missing")
	}
}

func TestValidateConfigRejectsMissingCredential(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{"workspace":"w","lakehouse":"l"}`)); err == nil {
		t.Fatalf("expected error for missing credential")
	}
	cfg := json.RawMessage(`{"workspace":"w","lakehouse":"l","tenant_id":"t","client_id":"c"}`)
	if err := ValidateConfig(cfg); err == nil {
		t.Fatalf("expected error when service-principal triplet is incomplete")
	}
}

func TestValidateConfigAcceptsOAuthToken(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{"workspace":"w","lakehouse":"l","oauth_token":"t"}`)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfigAcceptsServicePrincipal(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"workspace":"w","lakehouse":"l",
		"tenant_id":"t","client_id":"c","client_secret":"s"
	}`)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoveryEmitsOneLakeIcebergSources(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"workspace":"w","lakehouse":"l","oauth_token":"t",
		"iceberg_tables":[
			{"selector":"db.t","metadata_location":"abfss://w@onelake.dfs.fabric.microsoft.com/l.Lakehouse/x.json"}
		]
	}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	got, err := New().DiscoverSources(context.Background(), conn, "")
	if err != nil {
		t.Fatalf("DiscoverSources: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].SourceKind != "onelake_iceberg_table" {
		t.Fatalf("source_kind = %q", got[0].SourceKind)
	}
	if !got[0].SupportsZeroCopy {
		t.Fatalf("expected SupportsZeroCopy=true")
	}
}

func TestDiscoveryFailsWhenNoTablesDeclared(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{"workspace":"w","lakehouse":"l","oauth_token":"t"}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	_, err := New().DiscoverSources(context.Background(), conn, "")
	if err == nil {
		t.Fatalf("expected error when no inline tables declared")
	}
	if !strings.Contains(err.Error(), "list_with_delimiter failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryVirtualTableServesInlineSampleRows(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"workspace":"w","lakehouse":"l","oauth_token":"t",
		"tables":[
			{"selector":"sales","sample_rows":[{"id":1},{"id":2},{"id":3}]}
		]
	}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	limit := 2
	q := &adapters.Query{Selector: "sales", Limit: &limit}
	res, err := New().QueryVirtualTable(context.Background(), conn, q, "")
	if err != nil {
		t.Fatalf("QueryVirtualTable: %v", err)
	}
	if res.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2 (limit-clamped)", res.RowCount)
	}
}

func TestQueryVirtualTableFetchesRemoteCatalogResource(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tables/sales" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":1},{"id":2}]}`))
	}))
	defer server.Close()

	cfgJSON, err := json.Marshal(map[string]any{
		"workspace":              "w",
		"lakehouse":              "l",
		"oauth_token":            "t",
		"base_url":               server.URL,
		"resource_path_template": "/tables/{selector}",
		"tables":                 []map[string]any{{"selector": "sales"}},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfgJSON}
	q := &adapters.Query{Selector: "sales"}

	a := New()
	a.SetHTTPClient(server.Client())
	res, err := a.QueryVirtualTable(context.Background(), conn, q, "")
	if err != nil {
		t.Fatalf("QueryVirtualTable: %v", err)
	}
	if res.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", res.RowCount)
	}
}

func TestQueryVirtualTableValidatesOneLakeConfig(t *testing.T) {
	t.Parallel()
	conn := &models.Connection{ConnectorType: ConnectorType, Config: json.RawMessage(`{}`)}
	_, err := New().QueryVirtualTable(context.Background(), conn, &adapters.Query{Selector: "x"}, "")
	if err == nil {
		t.Fatalf("expected validation error for missing workspace")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoveryListsObjectsWithFakeClient(t *testing.T) {
	t.Parallel()
	etag := "abc"
	a := New()
	a.SetObjectLister(fakeLister{listing: &ObjectListing{
		CommonPrefixes: []string{"Files/raw/"},
		Objects:        []ObjectInfo{{Location: "Files/raw/orders.jsonl", Size: 12, ETag: &etag, LastModified: time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)}},
	}})
	conn := &models.Connection{ConnectorType: ConnectorType, Config: json.RawMessage(`{"workspace":"w","lakehouse":"l","oauth_token":"t","prefix":"raw"}`)}
	got, err := a.DiscoverSources(context.Background(), conn, "")
	if err != nil {
		t.Fatalf("DiscoverSources: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].SourceKind != "onelake_prefix" || got[1].SourceKind != "onelake_object" {
		t.Fatalf("source kinds = %q, %q", got[0].SourceKind, got[1].SourceKind)
	}
	if !got[1].SupportsZeroCopy || got[1].SourceSignature == nil || *got[1].SourceSignature != etag {
		t.Fatalf("unexpected object source: %+v", got[1])
	}
}

func TestBuildIngestSpecEmitsOneLakeDescriptor(t *testing.T) {
	t.Parallel()
	conn := &models.Connection{Name: "onelake-sync", ConnectorType: ConnectorType, Config: json.RawMessage(`{"workspace":"w","lakehouse":"l","oauth_token":"t"}`)}
	spec, err := New().BuildIngestSpec(context.Background(), conn, &adapters.Source{Selector: "Files/raw/orders.jsonl", SourceKind: "onelake_object"})
	if err != nil {
		t.Fatalf("BuildIngestSpec: %v", err)
	}
	if spec.Source != ConnectorType {
		t.Fatalf("source = %q", spec.Source)
	}
	var cfg map[string]any
	if err := json.Unmarshal(spec.Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg["workspace"] != "w" || cfg["selector"] != "Files/raw/orders.jsonl" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestUnsupportedCapabilitiesReturnNotImplemented(t *testing.T) {
	t.Parallel()
	a := New()
	conn := &models.Connection{}
	if _, err := a.StreamArrow(context.Background(), conn, &adapters.Query{}, ""); !errors.Is(err, adapters.ErrNotImplemented) {
		t.Fatalf("StreamArrow err = %v", err)
	}
}

type fakeLister struct {
	listing *ObjectListing
	err     error
}

func (f fakeLister) ListObjects(context.Context, string) (*ObjectListing, error) {
	return f.listing, f.err
}

func TestFactoryReturnsConnectorAdapter(t *testing.T) {
	t.Parallel()
	var _ adapters.ConnectorAdapter = Factory().New()
}
