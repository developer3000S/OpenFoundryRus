// Package gcs is the Go port of
// `services/connector-management-service/src/connectors/gcs.rs` — the
// Google Cloud Storage object-store source.
//
// CMA-8 contract (passthrough Discover + Query delegate):
//
//   - DiscoverSources: passthrough via [opentable.Discover] (returns the
//     inline `iceberg_tables[]` / `delta_tables[]` declared in
//     `connection.config`). Live bucket listing (the `object_store::gcp`
//     path in Rust) is intentionally out of scope here — the Go port
//     relies on the connector-agent and the Iceberg REST catalog (see
//     internal/handlers/iceberg_catalog.go) to surface live data, mirroring
//     the S3 / Azure Blob ports.
//   - QueryVirtualTable: delegates to [catalogbridge.Bridge.QueryVirtualTable]
//     (CMA-13) so configurations carrying inline `tables[]` /
//     `sample_rows` or `base_url` + resource templates produce previews
//     through the same code path as the tabular bridge connectors.
//   - StreamArrow: returns [adapters.ErrNotImplemented] — `gcs.rs` does
//     not expose `stream_arrow_ipc`.
//   - BuildIngestSpec: deferred. Returns [adapters.ErrNotImplemented]
//     until the `gcs` variant of `IngestJobSpec` lands; the dispatcher
//     translates that into the existing
//     `"<capability> is not supported for connector type: gcs"` envelope.
//
// Required config keys (mirrors Rust `validate_config`):
//
//   - `bucket` — non-empty bucket name.
//   - One credential variant:
//   - `access_token`             — static OAuth2 bearer token.
//   - `service_account_json`     — inline service-account JSON (object or string).
//   - `application_default: true` — opt-in ADC / Workload Identity Federation.
//
// Optional:
//   - `iceberg_tables[]`, `delta_tables[]` — see internal/adapters/opentable.
//   - `tables[]` / `views[]` / `base_url` + `catalog_path` — see
//     internal/adapters/catalogbridge.
package gcs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/catalogbridge"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/opentable"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// ConnectorType is the `connections.connector_type` value the registry
// binds this adapter under. Mirrors the Rust module's implicit name.
const ConnectorType = "gcs"

const (
	storePrefix       = "gcs"
	defaultSourceKind = "gcs_object"
)

// Adapter is the gcs [adapters.ConnectorAdapter] implementation. It is
// safe for concurrent use; the embedded bridge holds an [http.Client]
// reused across calls.
type Adapter struct {
	bridge *catalogbridge.Bridge
	lister objectLister
}

type ObjectInfo struct {
	Location     string
	Size         int64
	ETag         *string
	LastModified time.Time
}

type ObjectListing struct {
	CommonPrefixes []string
	Objects        []ObjectInfo
}

type objectLister interface {
	ListObjects(ctx context.Context, prefix string) (*ObjectListing, error)
}

// New returns a gcs [Adapter] backed by [http.DefaultClient].
func New() *Adapter {
	return &Adapter{bridge: catalogbridge.New(ConnectorType, defaultSourceKind, nil)}
}

// Factory returns an [adapters.Factory] that yields fresh Adapters; the
// registry asks for an instance per request so per-connection state
// (HTTP client overrides used by tests) stays scoped to the constructed
// value.
func Factory() adapters.Factory {
	return adapters.FactoryFunc(func() adapters.ConnectorAdapter { return New() })
}

// SetHTTPClient overrides the bridge's [http.Client]. Used by tests that
// stand up an [httptest.Server]; the production path keeps
// [http.DefaultClient].
func (a *Adapter) SetHTTPClient(client *http.Client) {
	if client != nil {
		a.bridge.HTTPClient = client
	}
}

func (a *Adapter) SetObjectLister(lister objectLister) {
	a.lister = lister
}

type gcsConfig struct {
	Bucket             string          `json:"bucket"`
	AccessToken        json.RawMessage `json:"access_token"`
	ServiceAccountJSON json.RawMessage `json:"service_account_json"`
	ApplicationDefault *bool           `json:"application_default"`
}

// ValidateConfig mirrors Rust's `validate_config`: a non-empty `bucket`
// plus one of `access_token`, `service_account_json` or
// `application_default: true` is required. ADC is opt-in (matching the
// Rust security stance) so operators have to explicitly accept
// env-provided credentials.
func ValidateConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("gcs source requires 'bucket'")
	}
	var cfg gcsConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("gcs: invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return errors.New("gcs source requires 'bucket'")
	}
	hasADC := cfg.ApplicationDefault != nil && *cfg.ApplicationDefault
	if len(cfg.AccessToken) == 0 && len(cfg.ServiceAccountJSON) == 0 && !hasADC {
		return errors.New("gcs source requires 'access_token', 'service_account_json' or 'application_default: true'")
	}
	return nil
}

// DiscoverSources turns the inline iceberg_tables[] / delta_tables[]
// entries into [adapters.Source] descriptors. Mirrors the open-table
// branch of Rust's `discover_sources`, including the "must declare at
// least one table" failure mode (the live bucket-listing branch is out
// of scope for the Go port — see package doc).
func (a *Adapter) DiscoverSources(ctx context.Context, c *models.Connection, _ string) ([]adapters.Source, error) {
	if c == nil {
		return nil, errors.New("gcs: connection is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	sources, err := opentable.Discover(c.Config, storePrefix)
	if err != nil {
		return nil, fmt.Errorf("gcs: %w", err)
	}
	listing, err := a.ListObjects(ctx, c)
	if err != nil {
		if len(sources) > 0 {
			return sources, nil
		}
		return nil, fmt.Errorf("gcs list_with_delimiter failed: %w", err)
	}
	sources = append(sources, objectSources(c.Config, listing)...)
	if len(sources) == 0 {
		return nil, errors.New("gcs source did not expose any virtual tables, prefixes or objects")
	}
	return sources, nil
}

func (a *Adapter) ListObjects(ctx context.Context, c *models.Connection) (*ObjectListing, error) {
	if c == nil {
		return nil, errors.New("gcs: connection is nil")
	}
	if a.lister == nil {
		return nil, errors.New("gcs object lister is not configured")
	}
	var cfg struct {
		Prefix    string `json:"prefix"`
		Subfolder string `json:"subfolder"`
	}
	_ = json.Unmarshal(c.Config, &cfg)
	prefix := strings.TrimSpace(cfg.Prefix)
	if prefix == "" {
		prefix = strings.TrimSpace(cfg.Subfolder)
	}
	return a.lister.ListObjects(ctx, prefix)
}

// QueryVirtualTable delegates to [catalogbridge.Bridge.QueryVirtualTable].
// Configurations that only carry inline open-table pointers will surface
// the bridge's "requires 'base_url'" envelope — clients then fall back
// to the Iceberg REST `LoadTable` path. Mirrors the CMA-8 "Query:
// delegate to catalog_bridge" contract.
func (a *Adapter) QueryVirtualTable(ctx context.Context, c *models.Connection, q *adapters.Query, _ string) (*adapters.Result, error) {
	if c == nil {
		return nil, errors.New("gcs: connection is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	return a.bridge.QueryVirtualTable(ctx, c, q)
}

// StreamArrow is unsupported — `gcs.rs` does not expose
// `stream_arrow_ipc`. Sync rows go through the connector-agent path
// instead.
func (a *Adapter) StreamArrow(_ context.Context, _ *models.Connection, _ *adapters.Query, _ string) (adapters.ArrowStream, error) {
	return nil, fmt.Errorf("%w: gcs arrow streaming", adapters.ErrNotImplemented)
}

// BuildIngestSpec emits the object-storage descriptor forwarded to
// ingestion-replication-service for the selected GCS source.
func (a *Adapter) BuildIngestSpec(_ context.Context, c *models.Connection, src *adapters.Source) (*adapters.IngestSpec, error) {
	if c == nil {
		return nil, errors.New("gcs: connection is nil")
	}
	if src == nil {
		return nil, errors.New("gcs: source is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return nil, fmt.Errorf("gcs: invalid config: %w", err)
	}
	specCfg := map[string]any{"selector": src.Selector, "source_kind": src.SourceKind}
	for _, k := range []string{"bucket", "prefix", "subfolder", "access_token", "service_account_json", "application_default"} {
		if v, ok := cfg[k]; ok {
			specCfg[k] = v
		}
	}
	if len(src.Metadata) > 0 {
		specCfg["metadata"] = json.RawMessage(src.Metadata)
	}
	raw, err := json.Marshal(specCfg)
	if err != nil {
		return nil, fmt.Errorf("gcs: marshal ingest spec: %w", err)
	}
	return &adapters.IngestSpec{Name: c.Name, Namespace: "default", Source: ConnectorType, Config: raw}, nil
}

func objectSources(raw json.RawMessage, listing *ObjectListing) []adapters.Source {
	if listing == nil {
		return nil
	}
	var cfg struct {
		Bucket string `json:"bucket"`
	}
	_ = json.Unmarshal(raw, &cfg)
	out := make([]adapters.Source, 0, len(listing.CommonPrefixes)+len(listing.Objects))
	for _, prefix := range listing.CommonPrefixes {
		selector := prefix
		meta, _ := json.Marshal(map[string]any{"bucket": cfg.Bucket, "uri": "gs://" + cfg.Bucket + "/" + selector, "kind": "prefix"})
		out = append(out, adapters.Source{Selector: selector, DisplayName: selector, SourceKind: storePrefix + "_prefix", SupportsSync: true, SupportsZeroCopy: false, Metadata: meta})
	}
	for _, object := range listing.Objects {
		selector := object.Location
		format := detectFormat(selector)
		meta, _ := json.Marshal(map[string]any{"bucket": cfg.Bucket, "uri": "gs://" + cfg.Bucket + "/" + selector, "size": object.Size, "format": format, "last_modified": object.LastModified.UTC().Format(time.RFC3339)})
		out = append(out, adapters.Source{Selector: selector, DisplayName: selector, SourceKind: storePrefix + "_object", SupportsSync: true, SupportsZeroCopy: format == "csv" || format == "json", SourceSignature: object.ETag, Metadata: meta})
	}
	return out
}

func detectFormat(selector string) string {
	lowered := strings.ToLower(selector)
	switch {
	case strings.HasSuffix(lowered, ".parquet"):
		return "parquet"
	case strings.HasSuffix(lowered, ".csv") || strings.HasSuffix(lowered, ".csv.gz"):
		return "csv"
	case strings.HasSuffix(lowered, ".json") || strings.HasSuffix(lowered, ".ndjson") || strings.HasSuffix(lowered, ".jsonl"):
		return "json"
	default:
		return "binary"
	}
}
