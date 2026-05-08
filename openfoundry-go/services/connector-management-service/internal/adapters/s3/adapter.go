// Package s3 is the Go port of
// `services/connector-management-service/src/connectors/s3.rs` — the Amazon
// S3 (and S3-compatible) "open table" object-store source.
//
// Foundry-aligned thin wrapper. The adapter itself does not read object
// payloads: bytes are streamed by the connector agent (or by clients
// consuming the Iceberg REST catalog at /iceberg/v1/* — see
// internal/handlers/iceberg_catalog.go).
//
// Discovery turns inline `iceberg_tables[]` / `delta_tables[]` declared in
// `connection.config` into [adapters.Source] entries with the upstream
// `s3://…/metadata.json` pointer attached. The catalog forwards that
// pointer verbatim to clients via `LoadTable`, fulfilling the zero-copy
// promise documented in
// `docs_original_palantir_foundry/foundry-docs/Data connectivity & integration/Core concepts/Virtual tables.md`.
//
// Required config keys:
//   - `url` (canonical, e.g. `s3://bucket/prefix/`) or `bucket` (string).
//
// Optional:
//   - `endpoint`, `region`, `access_key_id`, `secret_access_key`,
//     `path_style`, `subfolder` — interpreted by the agent / Iceberg catalog
//     consumers, not by this adapter.
//   - `iceberg_tables[]`, `delta_tables[]` — see internal/adapters/opentable.
//
// The HTTP-bridge / `catalog_bridge` flavour used by Rust for inline
// `tables[]` / `datasets[]` arrays is not yet ported; the adapter surfaces
// query / arrow / ingest as [adapters.ErrNotImplemented] so callers route
// through the Iceberg REST catalog instead.
package s3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/catalogbridge"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/opentable"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// ConnectorType is the `connections.connector_type` value the registry
// binds this adapter under. Mirrors the Rust `CONNECTOR_NAME` constant.
const ConnectorType = "s3"

const storePrefix = "s3"

const defaultSourceKind = "s3_object"

// Adapter is the s3 [adapters.ConnectorAdapter] implementation. It is
// stateless and safe for concurrent use.
type Adapter struct {
	bridge *catalogbridge.Bridge
}

// New returns a ready-to-use [Adapter].
func New() *Adapter {
	return &Adapter{bridge: catalogbridge.New(ConnectorType, defaultSourceKind, nil)}
}

// SetHTTPClient overrides the bridge's [http.Client]. Used by tests that
// stand up an [httptest.Server]; the production path keeps
// [http.DefaultClient].
func (a *Adapter) SetHTTPClient(client *http.Client) {
	if client != nil {
		a.bridge.HTTPClient = client
	}
}

// Factory returns an [adapters.Factory] that yields the singleton Adapter.
// Inline-catalog adapters carry no per-connection state, so a single
// instance is shared across requests.
func Factory() adapters.Factory { return adapters.SingletonFactory(New()) }

type s3Config struct {
	URL    string `json:"url"`
	Bucket string `json:"bucket"`
}

// ValidateConfig mirrors Rust's `validate_config`: a non-empty `url` (or
// fallback `bucket`) identity field, plus either an inline open-table catalog
// (`iceberg_tables[]`/`delta_tables[]`) or a catalog-bridge table catalog.
func ValidateConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("s3 connector requires 'url' or 'bucket'")
	}
	var cfg s3Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("s3: invalid config: %w", err)
	}
	identity := identityField(&cfg)
	if identity == "" {
		return errors.New("s3 connector requires 'url' or 'bucket'")
	}
	if opentable.HasCatalog(raw) {
		return nil
	}
	return catalogbridge.New(ConnectorType, defaultSourceKind, []string{identity}).ValidateConfig(raw)
}

func identityField(cfg *s3Config) string {
	if strings.TrimSpace(cfg.URL) != "" {
		return "url"
	}
	if strings.TrimSpace(cfg.Bucket) != "" {
		return "bucket"
	}
	return ""
}

// DiscoverSources turns the inline iceberg_tables[] / delta_tables[]
// entries into [adapters.Source] descriptors. Mirrors Rust's
// `discover_sources` open-table branch, including the "must declare at
// least one table" failure mode.
func (a *Adapter) DiscoverSources(ctx context.Context, c *models.Connection, _ string) ([]adapters.Source, error) {
	if c == nil {
		return nil, errors.New("s3: connection is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	var sources []adapters.Source
	if !opentable.HasCatalog(c.Config) {
		bridgeSources, err := a.bridge.DiscoverSources(ctx, c)
		if err != nil {
			return nil, err
		}
		sources = append(sources, bridgeSources...)
	}
	openTables, err := opentable.Discover(c.Config, storePrefix)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}
	sources = append(sources, openTables...)
	if len(sources) == 0 {
		return nil, errors.New("S3 source did not expose any virtual tables")
	}
	seen := make(map[string]adapters.Source, len(sources))
	for _, source := range sources {
		seen[source.Selector] = source
	}
	out := make([]adapters.Source, 0, len(seen))
	for _, source := range seen {
		out = append(out, source)
	}
	return out, nil
}

// QueryVirtualTable is unsupported for inline open-table sources: clients
// resolve the upstream metadata pointer through the Iceberg REST
// `LoadTable` path instead. Mirrors Rust by returning the
// unsupported-capability envelope.
func (a *Adapter) QueryVirtualTable(ctx context.Context, c *models.Connection, q *adapters.Query, _ string) (*adapters.Result, error) {
	if c == nil {
		return nil, errors.New("s3: connection is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	return a.bridge.QueryVirtualTable(ctx, c, q)
}

// StreamArrow is unsupported for the same reason as QueryVirtualTable.
func (a *Adapter) StreamArrow(_ context.Context, _ *models.Connection, _ *adapters.Query, _ string) (adapters.ArrowStream, error) {
	return nil, fmt.Errorf("%w: s3 arrow streaming", adapters.ErrNotImplemented)
}

// BuildIngestSpec emits the object-storage descriptor forwarded to
// ingestion-replication-service for the selected S3 source.
func (a *Adapter) BuildIngestSpec(_ context.Context, c *models.Connection, src *adapters.Source) (*adapters.IngestSpec, error) {
	if c == nil {
		return nil, errors.New("s3: connection is nil")
	}
	if src == nil {
		return nil, errors.New("s3: source is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return nil, fmt.Errorf("s3: invalid config: %w", err)
	}
	specCfg := map[string]any{"selector": src.Selector, "source_kind": src.SourceKind}
	for _, k := range []string{"url", "bucket", "endpoint", "region", "access_key_id", "secret_access_key", "path_style", "subfolder"} {
		if v, ok := cfg[k]; ok {
			specCfg[k] = v
		}
	}
	if len(src.Metadata) > 0 {
		specCfg["metadata"] = json.RawMessage(src.Metadata)
	}
	raw, err := json.Marshal(specCfg)
	if err != nil {
		return nil, fmt.Errorf("s3: marshal ingest spec: %w", err)
	}
	return &adapters.IngestSpec{Name: c.Name, Namespace: "default", Source: ConnectorType, Config: raw}, nil
}
