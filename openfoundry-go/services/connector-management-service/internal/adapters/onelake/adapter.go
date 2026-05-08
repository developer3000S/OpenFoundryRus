// Package onelake is the Go port of
// `services/connector-management-service/src/connectors/onelake.rs` —
// the Microsoft OneLake (Fabric Lakehouse) ABFS-compatible source.
//
// OneLake exposes Fabric Lakehouses at
// `https://onelake.dfs.fabric.microsoft.com/<workspace>/<lakehouse>.Lakehouse/...`,
// which Foundry models as a thin variant of the ABFS connector. This
// adapter mirrors that variant while keeping the CMA-8 contract (passthrough
// Discover + Query delegate):
//
//   - DiscoverSources: passthrough via [opentable.Discover] (returns the
//     inline `iceberg_tables[]` / `delta_tables[]` declared in
//     `connection.config`). Live ABFS listing (the `object_store::azure`
//     path in Rust) is intentionally out of scope for the Go port —
//     the connector-agent and the Iceberg REST catalog (see
//     internal/handlers/iceberg_catalog.go) surface live data instead,
//     mirroring the S3 / Azure Blob ports.
//   - QueryVirtualTable: delegates to
//     [catalogbridge.Bridge.QueryVirtualTable] (CMA-13) so configurations
//     carrying inline `tables[]` / `sample_rows` or
//     `base_url` + resource templates produce previews through the same
//     code path as the tabular bridge connectors.
//   - StreamArrow: returns [adapters.ErrNotImplemented] — `onelake.rs`
//     does not expose `stream_arrow_ipc`.
//   - BuildIngestSpec: deferred. Returns [adapters.ErrNotImplemented]
//     until the `onelake` variant of `IngestJobSpec` lands.
//
// Required config keys (mirrors Rust `validate_config`):
//
//   - `workspace` — Fabric workspace GUID or display name.
//   - `lakehouse` — lakehouse name (without the `.Lakehouse` suffix).
//   - One credential variant:
//   - `oauth_token`                                       — Entra ID bearer.
//   - `tenant_id` + `client_id` + `client_secret`         — service principal.
//
// Optional:
//   - `namespace`                       — `Files` (default) or `Tables`.
//   - `prefix`                          — additional path narrowing.
//   - `iceberg_tables[]`, `delta_tables[]` — see internal/adapters/opentable.
//   - `tables[]` / `views[]` / `base_url` + `catalog_path` — see
//     internal/adapters/catalogbridge.
package onelake

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
const ConnectorType = "onelake"

const (
	storePrefix       = "onelake"
	defaultSourceKind = "onelake_object"
)

// Adapter is the onelake [adapters.ConnectorAdapter] implementation. It
// is safe for concurrent use; the embedded bridge holds an [http.Client]
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

// New returns a onelake [Adapter] backed by [http.DefaultClient].
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

type onelakeConfig struct {
	Workspace    string          `json:"workspace"`
	Lakehouse    string          `json:"lakehouse"`
	OAuthToken   json.RawMessage `json:"oauth_token"`
	TenantID     string          `json:"tenant_id"`
	ClientID     string          `json:"client_id"`
	ClientSecret string          `json:"client_secret"`
}

// ValidateConfig mirrors Rust's `validate_config`: non-empty `workspace`
// and `lakehouse` plus one of `oauth_token` or the
// `tenant_id`+`client_id`+`client_secret` triplet is required.
func ValidateConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("onelake source requires 'workspace'")
	}
	var cfg onelakeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("onelake: invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.Workspace) == "" {
		return errors.New("onelake source requires 'workspace'")
	}
	if strings.TrimSpace(cfg.Lakehouse) == "" {
		return errors.New("onelake source requires 'lakehouse'")
	}
	hasToken := len(cfg.OAuthToken) > 0
	hasSP := strings.TrimSpace(cfg.TenantID) != "" &&
		strings.TrimSpace(cfg.ClientID) != "" &&
		strings.TrimSpace(cfg.ClientSecret) != ""
	if !hasToken && !hasSP {
		return errors.New("onelake source requires 'oauth_token' or ('tenant_id'+'client_id'+'client_secret')")
	}
	return nil
}

// DiscoverSources turns the inline iceberg_tables[] / delta_tables[]
// entries into [adapters.Source] descriptors. Mirrors the open-table
// branch of Rust's `discover_sources`, including the "must declare at
// least one table" failure mode (the live ABFS-listing branch is out
// of scope for the Go port — see package doc).
func (a *Adapter) DiscoverSources(ctx context.Context, c *models.Connection, _ string) ([]adapters.Source, error) {
	if c == nil {
		return nil, errors.New("onelake: connection is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	sources, err := opentable.Discover(c.Config, storePrefix)
	if err != nil {
		return nil, fmt.Errorf("onelake: %w", err)
	}
	listing, err := a.ListObjects(ctx, c)
	if err != nil {
		if len(sources) > 0 {
			return sources, nil
		}
		return nil, fmt.Errorf("onelake list_with_delimiter failed: %w", err)
	}
	sources = append(sources, objectSources(c.Config, listing)...)
	if len(sources) == 0 {
		return nil, errors.New("onelake source did not expose any virtual tables, prefixes or objects")
	}
	return sources, nil
}

func (a *Adapter) ListObjects(ctx context.Context, c *models.Connection) (*ObjectListing, error) {
	if c == nil {
		return nil, errors.New("onelake: connection is nil")
	}
	if a.lister == nil {
		return nil, errors.New("onelake object lister is not configured")
	}
	var cfg struct {
		Namespace string `json:"namespace"`
		Prefix    string `json:"prefix"`
	}
	_ = json.Unmarshal(c.Config, &cfg)
	ns := strings.TrimSpace(cfg.Namespace)
	if ns == "" {
		ns = "Files"
	}
	prefix := ns
	if extra := strings.TrimLeft(strings.TrimSpace(cfg.Prefix), "/"); extra != "" {
		prefix += "/" + extra
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
		return nil, errors.New("onelake: connection is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	return a.bridge.QueryVirtualTable(ctx, c, q)
}

// StreamArrow is unsupported — `onelake.rs` does not expose
// `stream_arrow_ipc`.
func (a *Adapter) StreamArrow(_ context.Context, _ *models.Connection, _ *adapters.Query, _ string) (adapters.ArrowStream, error) {
	return nil, fmt.Errorf("%w: onelake arrow streaming", adapters.ErrNotImplemented)
}

// BuildIngestSpec emits the object-storage descriptor forwarded to
// ingestion-replication-service for the selected OneLake source.
func (a *Adapter) BuildIngestSpec(_ context.Context, c *models.Connection, src *adapters.Source) (*adapters.IngestSpec, error) {
	if c == nil {
		return nil, errors.New("onelake: connection is nil")
	}
	if src == nil {
		return nil, errors.New("onelake: source is nil")
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(c.Config, &cfg); err != nil {
		return nil, fmt.Errorf("onelake: invalid config: %w", err)
	}
	specCfg := map[string]any{"selector": src.Selector, "source_kind": src.SourceKind}
	for _, k := range []string{"workspace", "lakehouse", "namespace", "prefix", "oauth_token", "tenant_id", "client_id", "client_secret"} {
		if v, ok := cfg[k]; ok {
			specCfg[k] = v
		}
	}
	if len(src.Metadata) > 0 {
		specCfg["metadata"] = json.RawMessage(src.Metadata)
	}
	raw, err := json.Marshal(specCfg)
	if err != nil {
		return nil, fmt.Errorf("onelake: marshal ingest spec: %w", err)
	}
	return &adapters.IngestSpec{Name: c.Name, Namespace: "default", Source: ConnectorType, Config: raw}, nil
}

func objectSources(raw json.RawMessage, listing *ObjectListing) []adapters.Source {
	if listing == nil {
		return nil
	}
	var cfg struct {
		Workspace string `json:"workspace"`
		Lakehouse string `json:"lakehouse"`
	}
	_ = json.Unmarshal(raw, &cfg)
	root := "abfss://" + cfg.Workspace + "@onelake.dfs.fabric.microsoft.com/" + cfg.Lakehouse + ".Lakehouse"
	out := make([]adapters.Source, 0, len(listing.CommonPrefixes)+len(listing.Objects))
	for _, prefix := range listing.CommonPrefixes {
		selector := prefix
		meta, _ := json.Marshal(map[string]any{"uri": root + "/" + selector, "kind": "prefix"})
		out = append(out, adapters.Source{Selector: selector, DisplayName: selector, SourceKind: storePrefix + "_prefix", SupportsSync: true, SupportsZeroCopy: false, Metadata: meta})
	}
	for _, object := range listing.Objects {
		selector := object.Location
		format := detectFormat(selector)
		meta, _ := json.Marshal(map[string]any{"uri": root + "/" + selector, "size": object.Size, "format": format, "last_modified": object.LastModified.UTC().Format(time.RFC3339)})
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
