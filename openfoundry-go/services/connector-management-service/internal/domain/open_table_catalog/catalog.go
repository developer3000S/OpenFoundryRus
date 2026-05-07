// Package open_table_catalog is the Go port of the Rust
// `services/connector-management-service/src/connectors/open_table_catalog.rs`
// helper. It is a utility package, not a user-facing connector — per-store
// adapters (S3, Azure Blob / ADLS, GCS, OneLake, …) wrap [HasCatalog] and
// [Discover] from their own validators and discovery dispatchers.
//
// Foundry-aligned: any object-store backed source can carry inline
// `iceberg_tables[]` and/or `delta_tables[]` arrays in its config to
// advertise tables that already live in the lake. Discovery surfaces one
// [models.DiscoveredSource] per entry with:
//
//   - SourceKind        = "<store>_<format>_table"  (e.g. "azure_iceberg_table")
//   - SupportsSync      = false  (zero-copy only)
//   - SupportsZeroCopy  = true
//   - SourceSignature   = the entry's `snapshot_id` if present
//   - Metadata.upstream.metadata_location / table_location = upstream
//     pointers the Iceberg REST `LoadTable` handler forwards verbatim —
//     fulfilling the zero-copy promise from
//     `docs_original_palantir_foundry/foundry-docs/Data connectivity & integration/Core concepts/Virtual tables.md`.
package open_table_catalog

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// HasCatalog reports whether `config` declares at least one inline
// open-table entry. Mirrors Rust's `has_open_table_catalog`. Used by
// per-store validators to skip the inline HTTP-catalog requirement when
// the source operates exclusively as a metadata pointer for Iceberg /
// Delta tables.
func HasCatalog(config map[string]any) bool {
	return hasNonEmptyArray(config, "iceberg_tables") ||
		hasNonEmptyArray(config, "delta_tables")
}

// HasCatalogJSON is a convenience wrapper that accepts a JSON-encoded
// config blob (mirroring the [json.RawMessage] inputs the connector
// adapters receive).
func HasCatalogJSON(config json.RawMessage) bool {
	if len(config) == 0 {
		return false
	}
	var probe struct {
		IcebergTables []json.RawMessage `json:"iceberg_tables"`
		DeltaTables   []json.RawMessage `json:"delta_tables"`
	}
	if err := json.Unmarshal(config, &probe); err != nil {
		return false
	}
	return len(probe.IcebergTables) > 0 || len(probe.DeltaTables) > 0
}

// Discover builds [models.DiscoveredSource] entries for every
// `iceberg_tables[]` and `delta_tables[]` entry declared in `config`.
// `storePrefix` is prepended to the SourceKind so callers can distinguish
// e.g. "s3_iceberg_table" from "azure_iceberg_table" downstream.
//
// Mirrors Rust's `open_table_catalog::discover`. De-duplicates by selector
// (last entry wins) and returns the result sorted by selector for
// deterministic output.
func Discover(config map[string]any, storePrefix string) []models.DiscoveredSource {
	if config == nil {
		return nil
	}
	dedup := make(map[string]models.DiscoveredSource)
	for _, entry := range openTableEntries(config, "iceberg_tables") {
		if src, ok := entry.toSource(storePrefix, "iceberg"); ok {
			dedup[src.Selector] = src
		}
	}
	for _, entry := range openTableEntries(config, "delta_tables") {
		if src, ok := entry.toSource(storePrefix, "delta"); ok {
			dedup[src.Selector] = src
		}
	}
	if len(dedup) == 0 {
		return nil
	}
	out := make([]models.DiscoveredSource, 0, len(dedup))
	for _, src := range dedup {
		out = append(out, src)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Selector < out[j].Selector })
	return out
}

// DiscoverJSON is a convenience wrapper that accepts a JSON-encoded config
// blob.
func DiscoverJSON(config json.RawMessage, storePrefix string) ([]models.DiscoveredSource, error) {
	if len(config) == 0 {
		return nil, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(config, &raw); err != nil {
		return nil, err
	}
	return Discover(raw, storePrefix), nil
}

type tableEntry struct {
	Selector         string
	DisplayName      string
	MetadataLocation string
	TableLocation    string
	SnapshotID       any
}

func (e tableEntry) toSource(storePrefix, format string) (models.DiscoveredSource, bool) {
	if e.Selector == "" {
		return models.DiscoveredSource{}, false
	}
	display := e.DisplayName
	if display == "" {
		display = e.Selector
	}
	meta, _ := json.Marshal(map[string]any{
		"format": format,
		"upstream": map[string]any{
			"metadata_location": stringOrNil(e.MetadataLocation),
			"table_location":    stringOrNil(e.TableLocation),
		},
	})
	src := models.DiscoveredSource{
		Selector:         e.Selector,
		DisplayName:      display,
		SourceKind:       storePrefix + "_" + format + "_table",
		SupportsSync:     false,
		SupportsZeroCopy: true,
		Metadata:         meta,
	}
	if sig := decodeSignature(e.SnapshotID); sig != "" {
		src.SourceSignature = &sig
	}
	return src, true
}

func openTableEntries(config map[string]any, key string) []tableEntry {
	raw, ok := config[key].([]any)
	if !ok {
		return nil
	}
	out := make([]tableEntry, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, tableEntry{
			Selector:         stringFromObj(obj, "selector"),
			DisplayName:      stringFromObj(obj, "display_name"),
			MetadataLocation: stringFromObj(obj, "metadata_location"),
			TableLocation:    stringFromObj(obj, "table_location"),
			SnapshotID:       obj["snapshot_id"],
		})
	}
	return out
}

func hasNonEmptyArray(config map[string]any, key string) bool {
	if config == nil {
		return false
	}
	list, ok := config[key].([]any)
	if !ok {
		return false
	}
	return len(list) > 0
}

func stringFromObj(obj map[string]any, key string) string {
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

func stringOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// decodeSignature accepts either a JSON string or a JSON integer, mirroring
// Rust's `v.as_str().or(v.as_i64().map(|n| n.to_string()))`.
func decodeSignature(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return strconv.FormatInt(i, 10)
		}
		return ""
	case float64:
		// JSON numbers decode as float64 by default; convert to int when
		// it's a whole number.
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return ""
	default:
		return ""
	}
}
