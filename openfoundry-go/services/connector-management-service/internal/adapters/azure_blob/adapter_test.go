package azure_blob

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

func TestValidateConfigRejectsMissingAccountName(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{"account_key":"k"}`)); err == nil {
		t.Fatalf("expected error for missing account_name")
	}
}

func TestValidateConfigRejectsMissingCredential(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{"account_name":"a"}`)); err == nil {
		t.Fatalf("expected error for missing credential")
	}
}

func TestValidateConfigAcceptsAccountKey(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{"account_name":"a","account_key":"k"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateConfigAcceptsSASToken(t *testing.T) {
	t.Parallel()
	if err := ValidateConfig(json.RawMessage(`{"account_name":"a","sas_token":"t"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiscoveryEmitsAzureIcebergSources(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{
		"account_name":"a","account_key":"k",
		"iceberg_tables":[{"selector":"db.t","metadata_location":"abfss://w@a.dfs/x.json"}]
	}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	got, err := New().DiscoverSources(context.Background(), conn, "")
	if err != nil {
		t.Fatalf("DiscoverSources: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].SourceKind != "azure_iceberg_table" {
		t.Fatalf("source_kind = %q", got[0].SourceKind)
	}
}

func TestDiscoveryFailsWhenNoTablesDeclared(t *testing.T) {
	t.Parallel()
	cfg := json.RawMessage(`{"account_name":"a","account_key":"k"}`)
	conn := &models.Connection{ConnectorType: ConnectorType, Config: cfg}
	_, err := New().DiscoverSources(context.Background(), conn, "")
	if err == nil {
		t.Fatalf("expected error when no inline tables declared")
	}
	if !strings.Contains(err.Error(), "did not expose any virtual tables") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsupportedCapabilitiesReturnNotImplemented(t *testing.T) {
	t.Parallel()
	a := New()
	conn := &models.Connection{}
	if _, err := a.QueryVirtualTable(context.Background(), conn, &adapters.Query{}, ""); !errors.Is(err, adapters.ErrNotImplemented) {
		t.Fatalf("QueryVirtualTable err = %v", err)
	}
	if _, err := a.StreamArrow(context.Background(), conn, &adapters.Query{}, ""); !errors.Is(err, adapters.ErrNotImplemented) {
		t.Fatalf("StreamArrow err = %v", err)
	}
}

func TestBuildIngestSpecEmitsAzureDescriptor(t *testing.T) {
	t.Parallel()
	conn := &models.Connection{Name: "azure-sync", ConnectorType: ConnectorType, Config: json.RawMessage(`{"account_name":"a","account_key":"k","iceberg_tables":[{"selector":"db.t","metadata_location":"abfss://c@a.dfs/x.json"}]}`)}
	spec, err := New().BuildIngestSpec(context.Background(), conn, &adapters.Source{Selector: "db.t", SourceKind: "azure_iceberg_table"})
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
	if cfg["account_name"] != "a" || cfg["selector"] != "db.t" {
		t.Fatalf("config = %#v", cfg)
	}
}

func TestFactoryReturnsConnectorAdapter(t *testing.T) {
	t.Parallel()
	var _ adapters.ConnectorAdapter = Factory().New()
}
