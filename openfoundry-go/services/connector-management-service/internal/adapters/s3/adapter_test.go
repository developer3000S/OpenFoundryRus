package s3

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

func TestValidateConfigRejectsMissingIdentity(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatalf("expected error for empty config")
	}
	if err := ValidateConfig(json.RawMessage(`{"endpoint":"e"}`)); err == nil {
		t.Fatalf("expected error when 'url' and 'bucket' are missing")
	}
}

func TestValidateConfigRequiresCatalog(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{"url":"s3://bucket/prefix/"}`)
	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatalf("expected error when iceberg_tables/delta_tables are missing")
	}
	if !strings.Contains(err.Error(), "inline catalog") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfigAcceptsURLWithIcebergTables(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"url":"s3://bucket/prefix/",
		"iceberg_tables":[{"selector":"db.t","metadata_location":"s3://bucket/x.json"}]
	}`)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfigAcceptsBucketWithDeltaTables(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"bucket":"my-bucket",
		"delta_tables":[{"selector":"db.d","metadata_location":"s3://bucket/_delta_log/"}]
	}`)
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoveryEmitsS3IcebergSources(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"url":"s3://bucket/prefix/",
		"iceberg_tables":[
			{"selector":"db.t","metadata_location":"s3://bucket/x.json","snapshot_id":"42"}
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
	if got[0].Selector != "db.t" {
		t.Fatalf("selector = %q", got[0].Selector)
	}
	if got[0].SourceKind != "s3_iceberg_table" {
		t.Fatalf("source_kind = %q", got[0].SourceKind)
	}
	if !got[0].SupportsZeroCopy {
		t.Fatalf("expected SupportsZeroCopy=true")
	}
	if got[0].SupportsSync {
		t.Fatalf("expected SupportsSync=false")
	}
	if got[0].SourceSignature == nil || *got[0].SourceSignature != "42" {
		t.Fatalf("expected snapshot id signature, got %v", got[0].SourceSignature)
	}
}

func TestDiscoveryDeduplicatesIcebergAndDeltaBySelector(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"url":"s3://bucket/",
		"iceberg_tables":[{"selector":"shared","metadata_location":"s3://bucket/iceberg.json"}],
		"delta_tables":[{"selector":"shared","metadata_location":"s3://bucket/_delta_log/"}]
	}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	got, err := New().DiscoverSources(context.Background(), conn, "")
	if err != nil {
		t.Fatalf("DiscoverSources: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry after de-dup, got %d", len(got))
	}
}

func TestDiscoveryFailsWhenNoTablesDeclared(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{"url":"s3://bucket/prefix/"}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	_, err := New().DiscoverSources(context.Background(), conn, "")
	if err == nil {
		t.Fatalf("expected error when no inline tables declared")
	}
}

func TestQueryVirtualTableServesInlineSampleRows(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"url":"s3://bucket/prefix/",
		"tables":[{"selector":"orders","sample_rows":[{"id":1},{"id":2}]}]
	}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	res, err := New().QueryVirtualTable(context.Background(), conn, &adapters.Query{Selector: "orders"}, "")
	if err != nil {
		t.Fatalf("QueryVirtualTable: %v", err)
	}
	if res.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", res.RowCount)
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

func TestBuildIngestSpecEmitsS3Descriptor(t *testing.T) {
	t.Parallel()
	conn := &models.Connection{Name: "s3-sync", ConnectorType: ConnectorType, Config: json.RawMessage(`{"url":"s3://bucket/prefix/","iceberg_tables":[{"selector":"db.t","metadata_location":"s3://bucket/x.json"}]}`)}
	spec, err := New().BuildIngestSpec(context.Background(), conn, &adapters.Source{Selector: "db.t", SourceKind: "s3_iceberg_table"})
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
	if cfg["url"] != "s3://bucket/prefix/" || cfg["selector"] != "db.t" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestFactoryReturnsConnectorAdapter(t *testing.T) {
	t.Parallel()
	var _ adapters.ConnectorAdapter = Factory().New()
}
