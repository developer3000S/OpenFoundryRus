// Package kafka is the Go port of
// `services/connector-management-service/src/connectors/kafka.rs` —
// the Kafka data-connection connector.
//
// The catalog-backed surface (parse `topics[]` from the connection
// config, return preview/sample messages) is always available. When
// `bootstrap_servers` (or `brokers`) is configured, the adapter
// upgrades discovery and virtual-table preview to live broker probes
// via libs/event-bus-control's segmentio/kafka-go helpers, mirroring
// Rust's `cfg(feature = "kafka-rdkafka")` gated path.
//
// Required config keys:
//   - `topics[]` — array of strings or objects (selector, display_name,
//     partitions, sample_messages, schema). At least one entry.
//
// Optional:
//   - `bootstrap_servers` (or `brokers`) — comma-separated broker list
//     enabling live mode.
//
// 1:1 mappings to Rust:
//   - validate_config       → [ValidateConfig]
//   - test_connection       → [Adapter.TestConnection]
//   - discover_sources      → [Adapter.DiscoverSources]
//   - query_virtual_table   → [Adapter.QueryVirtualTable]
//   - fetch_dataset         → [Adapter.FetchDataset]
//
// The 4-capability [adapters.ConnectorAdapter] interface keeps
// StreamArrow + BuildIngestSpec as [adapters.ErrNotImplemented]
// because Rust's `kafka.rs` exposes neither (Arrow streaming and
// ingest-spec construction live in dedicated modules).
package kafka

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	controlbus "github.com/openfoundry/openfoundry-go/libs/event-bus-control"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// ConnectorType is the `connections.connector_type` value the registry
// binds this adapter under. Mirrors the Rust module's implicit name.
const ConnectorType = "kafka"

const connectorLabel = "kafka connector"

const sourceKindTopic = "kafka_topic"

// LiveProbeTimeout is the default timeout for live broker probes,
// matching Rust's `LIVE_PROBE_TIMEOUT` (5 s).
const LiveProbeTimeout = 5 * time.Second

// SyncPayload mirrors Rust's `SyncPayload` (mod.rs) — the bytes the
// sync runtime hands to dataset-versioning-service for a new dataset
// version.
type SyncPayload struct {
	Bytes      []byte          `json:"-"`
	Format     string          `json:"format"`
	RowsSynced int64           `json:"rows_synced"`
	FileName   string          `json:"file_name"`
	Metadata   json.RawMessage `json:"metadata"`
}

// Adapter is the kafka [adapters.ConnectorAdapter] implementation. It
// is stateless apart from the [Adapter.Timeout] knob; safe for
// concurrent use.
type Adapter struct {
	// Timeout overrides [LiveProbeTimeout] for live broker probes. A
	// zero value falls back to the default. Tests inject shorter
	// timeouts via [Adapter.SetTimeout].
	Timeout time.Duration
}

// New returns a ready-to-use [Adapter] with [LiveProbeTimeout].
func New() *Adapter { return &Adapter{Timeout: LiveProbeTimeout} }

// Factory returns an [adapters.Factory] that yields fresh [Adapter]
// instances. Live-probe state (timeout) is per-instance, so we hand
// out a new value each request to avoid one caller's overrides
// leaking into another.
func Factory() adapters.Factory {
	return adapters.FactoryFunc(func() adapters.ConnectorAdapter { return New() })
}

// SetTimeout adjusts the live-probe timeout. Useful for tests that
// need a shorter dial deadline; production callers leave it at the
// default.
func (a *Adapter) SetTimeout(d time.Duration) {
	a.Timeout = d
}

// ValidateConfig mirrors Rust's `validate_config`: requires
// bootstrap_servers (or brokers) and at least one topic, and runs
// inline-schema validation on each topic that ships one.
func ValidateConfig(raw json.RawMessage) error {
	return controlbus.ValidateTopicConnectorConfig(raw, connectorLabel)
}

// TestConnection mirrors Rust's `test_connection`. With
// `bootstrap_servers` configured, it dials the broker for cluster
// metadata; otherwise it returns a catalog-backed success summarising
// the configured topic count.
func (a *Adapter) TestConnection(ctx context.Context, raw json.RawMessage) (adapters.ConnectionTestResult, error) {
	if err := ValidateConfig(raw); err != nil {
		return adapters.ConnectionTestResult{}, err
	}
	topics, err := controlbus.ParseTopicEntries(raw, connectorLabel)
	if err != nil {
		return adapters.ConnectionTestResult{}, err
	}
	bootstrap, hasBootstrap := controlbus.BootstrapServers(raw)
	if hasBootstrap {
		outcome, err := controlbus.TestConnection(ctx, bootstrap, a.timeout())
		if err != nil {
			return adapters.ConnectionTestResult{}, err
		}
		details := mustMarshalJSON(map[string]any{
			"bootstrap_servers":      bootstrap,
			"broker_count":           outcome.BrokerCount,
			"cluster_topic_count":    outcome.TopicCount,
			"originating_broker":     outcome.OriginatingBroker,
			"configured_topic_count": len(topics),
			"mode":                   "live",
		})
		return adapters.ConnectionTestResult{
			Success: true,
			Message: fmt.Sprintf(
				"connected to kafka cluster (%d broker(s), %d topic(s))",
				outcome.BrokerCount, outcome.TopicCount,
			),
			LatencyMS: outcome.LatencyMS,
			Details:   details,
		}, nil
	}
	details := mustMarshalJSON(map[string]any{
		"bootstrap_servers": bootstrapDetail(bootstrap, hasBootstrap),
		"topic_count":       len(topics),
		"mode":              "catalog_backed",
	})
	return adapters.ConnectionTestResult{
		Success:   true,
		Message:   fmt.Sprintf("validated kafka catalog with %d topic(s)", len(topics)),
		LatencyMS: 0,
		Details:   details,
	}, nil
}

// DiscoverSources mirrors Rust's `discover_sources`. Live mode lists
// topics from the broker; catalog mode returns the configured
// `topics[]` entries.
func (a *Adapter) DiscoverSources(ctx context.Context, c *models.Connection, _ string) ([]adapters.Source, error) {
	if c == nil {
		return nil, fmt.Errorf("%s: connection is nil", connectorLabel)
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	bootstrap, hasBootstrap := controlbus.BootstrapServers(c.Config)
	if hasBootstrap {
		topics, err := controlbus.DiscoverTopics(ctx, bootstrap, a.timeout())
		if err != nil {
			return nil, err
		}
		out := make([]adapters.Source, 0, len(topics))
		for _, t := range topics {
			meta := mustMarshalJSON(map[string]any{
				"topic":          t.Name,
				"partitions":     t.Partitions,
				"discovered_via": "kafka_metadata",
			})
			out = append(out, basicDiscoveredSource(t.Name, t.Name, sourceKindTopic, meta))
		}
		return out, nil
	}
	topics, err := controlbus.ParseTopicEntries(c.Config, connectorLabel)
	if err != nil {
		return nil, err
	}
	out := make([]adapters.Source, 0, len(topics))
	for _, t := range topics {
		out = append(out, basicDiscoveredSource(t.Selector, t.DisplayName, sourceKindTopic, t.Metadata))
	}
	return out, nil
}

// QueryVirtualTable mirrors Rust's `query_virtual_table`. The limit
// is clamped to [1, 500] with a default of 50 — same bounds as Rust.
// Live mode tails messages off partition 0; catalog mode returns the
// configured `sample_messages`.
func (a *Adapter) QueryVirtualTable(ctx context.Context, c *models.Connection, q *adapters.Query, _ string) (*adapters.Result, error) {
	if c == nil {
		return nil, fmt.Errorf("%s: connection is nil", connectorLabel)
	}
	if q == nil {
		return nil, fmt.Errorf("%s: query is nil", connectorLabel)
	}
	if err := ValidateConfig(c.Config); err != nil {
		return nil, err
	}
	limit := clampLimit(q.Limit)
	bootstrap, hasBootstrap := controlbus.BootstrapServers(c.Config)
	if hasBootstrap {
		rows, err := controlbus.TailMessages(ctx, bootstrap, q.Selector, limit, a.timeout())
		if err != nil {
			return nil, err
		}
		meta := mustMarshalJSON(map[string]any{
			"bootstrap_servers": bootstrap,
			"topic":             q.Selector,
			"mode":              "live_tail",
			"limit":             limit,
		})
		resp := virtualTableResponse(q.Selector, rows, meta)
		return &resp, nil
	}
	topic, err := controlbus.FindTopicEntry(c.Config, q.Selector, connectorLabel)
	if err != nil {
		return nil, err
	}
	rows := topic.SampleMessages
	if len(rows) > limit {
		rows = rows[:limit]
	}
	meta := mustMarshalJSON(map[string]any{
		"bootstrap_servers": bootstrapDetail(bootstrap, hasBootstrap),
		"partitions":        topic.Partitions,
		"entry":             json.RawMessage(topic.Metadata),
		"mode":              "catalog_backed",
	})
	resp := virtualTableResponse(q.Selector, rows, meta)
	return &resp, nil
}

// StreamArrow returns [adapters.ErrNotImplemented]. Rust's `kafka.rs`
// has no `stream_arrow_ipc`; Arrow streaming for Kafka would land in
// a separate ingestion-replication slice.
func (a *Adapter) StreamArrow(_ context.Context, _ *models.Connection, _ *adapters.Query, _ string) (adapters.ArrowStream, error) {
	return nil, fmt.Errorf("%w: kafka arrow streaming", adapters.ErrNotImplemented)
}

// BuildIngestSpec returns [adapters.ErrNotImplemented]. Rust's
// `kafka.rs` ships no `build_ingest_spec`; ingestion-replication
// owns the streaming spec for Kafka topics.
func (a *Adapter) BuildIngestSpec(_ context.Context, _ *models.Connection, _ *adapters.Source) (*adapters.IngestSpec, error) {
	return nil, fmt.Errorf("%w: kafka ingest spec", adapters.ErrNotImplemented)
}

// FetchDataset mirrors Rust's `fetch_dataset`. Returns a JSON
// [SyncPayload] of the configured `sample_messages` for the topic so
// the sync runtime can ship it to dataset-versioning-service.
func (a *Adapter) FetchDataset(_ context.Context, raw json.RawMessage, selector string) (SyncPayload, error) {
	if err := ValidateConfig(raw); err != nil {
		return SyncPayload{}, err
	}
	topic, err := controlbus.FindTopicEntry(raw, selector, connectorLabel)
	if err != nil {
		return SyncPayload{}, err
	}
	bytes, err := json.Marshal(topic.SampleMessages)
	if err != nil {
		return SyncPayload{}, fmt.Errorf("%s: %w", connectorLabel, err)
	}
	bootstrap, hasBootstrap := controlbus.BootstrapServers(raw)
	stem := controlbus.SanitizeFileStem(selector, "kafka_sync")
	meta := mustMarshalJSON(map[string]any{
		"bootstrap_servers": bootstrapDetail(bootstrap, hasBootstrap),
		"topic":             selector,
		"partitions":        topic.Partitions,
		"entry":             json.RawMessage(topic.Metadata),
	})
	payload := SyncPayload{
		Bytes:      bytes,
		Format:     "json",
		RowsSynced: int64(len(topic.SampleMessages)),
		FileName:   stem + ".json",
		Metadata:   meta,
	}
	addSourceSignature(&payload)
	return payload, nil
}

// ─── internals ─────────────────────────────────────────────────────────

func (a *Adapter) timeout() time.Duration {
	if a.Timeout <= 0 {
		return LiveProbeTimeout
	}
	return a.Timeout
}

// clampLimit applies the same default/clamp Rust uses in
// query_virtual_table: nil → 50, then clamp to [1, 500].
func clampLimit(limit *int) int {
	value := 50
	if limit != nil {
		value = *limit
	}
	if value < 1 {
		value = 1
	}
	if value > 500 {
		value = 500
	}
	return value
}

// basicDiscoveredSource mirrors Rust's `basic_discovered_source`
// (mod.rs).
func basicDiscoveredSource(selector, displayName, sourceKind string, metadata json.RawMessage) adapters.Source {
	if metadata == nil {
		metadata = json.RawMessage("null")
	}
	return adapters.Source{
		Selector:         selector,
		DisplayName:      displayName,
		SourceKind:       sourceKind,
		SupportsSync:     true,
		SupportsZeroCopy: true,
		SourceSignature:  nil,
		Metadata:         metadata,
	}
}

// virtualTableResponse mirrors Rust's `virtual_table_response`
// (mod.rs). Mode is hardcoded to "zero_copy"; per-call mode (live_tail
// vs catalog_backed) is carried inside `metadata`.
func virtualTableResponse(selector string, rows []json.RawMessage, metadata json.RawMessage) adapters.Result {
	if rows == nil {
		rows = []json.RawMessage{}
	}
	columns := firstObjectKeys(rows)
	signature := signatureFromRows(rows)
	if metadata == nil {
		metadata = json.RawMessage("null")
	}
	return adapters.Result{
		Selector:        selector,
		Mode:            "zero_copy",
		Columns:         columns,
		RowCount:        len(rows),
		Rows:            rows,
		SourceSignature: signature,
		Metadata:        metadata,
	}
}

// firstObjectKeys returns the keys of the first row that decodes as a
// JSON object, preserving insertion order. Mirrors Rust's
// `rows.iter().find_map(|row| row.as_object().map(|o| o.keys().cloned().collect()))`.
func firstObjectKeys(rows []json.RawMessage) []string {
	for _, row := range rows {
		keys, ok := orderedObjectKeys(row)
		if ok {
			return keys
		}
	}
	return []string{}
}

// orderedObjectKeys decodes `raw` as a JSON object and returns its
// keys in source order. Returns ok=false when `raw` is not an object.
//
// We implement the streaming decode here because encoding/json's
// map[string]json.RawMessage loses key order — Rust's serde_json::Map
// preserves insertion order and the contract leans on it.
func orderedObjectKeys(raw json.RawMessage) ([]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, false
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, false
	}
	keys := []string{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, ok := t.(string)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
		// Skip the value associated with the key.
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, false
		}
	}
	return keys, true
}

// signatureFromRows mirrors Rust's `serde_json::to_vec(&rows)? -> source_signature(bytes)`.
func signatureFromRows(rows []json.RawMessage) *string {
	bytes, err := json.Marshal(rows)
	if err != nil {
		return nil
	}
	sig := sourceSignature(bytes)
	return &sig
}

// sourceSignature mirrors Rust's `source_signature(bytes)` — a
// `sha256:<hex>` digest.
func sourceSignature(bytes []byte) string {
	digest := sha256.Sum256(bytes)
	return fmt.Sprintf("sha256:%x", digest[:])
}

// addSourceSignature mirrors Rust's `add_source_signature` — splices a
// `source_signature` field into the payload's metadata object.
func addSourceSignature(p *SyncPayload) {
	signature := sourceSignature(p.Bytes)
	var obj map[string]json.RawMessage
	if len(p.Metadata) == 0 {
		obj = map[string]json.RawMessage{}
	} else if err := json.Unmarshal(p.Metadata, &obj); err != nil {
		// Metadata isn't a JSON object — Rust's `as_object_mut`
		// returns None in that case and the field is left alone.
		return
	}
	sigBytes, err := json.Marshal(signature)
	if err != nil {
		return
	}
	obj["source_signature"] = sigBytes
	if next, err := json.Marshal(obj); err == nil {
		p.Metadata = next
	}
}

// bootstrapDetail returns the value Rust's catalog-backed details
// emit for `bootstrap_servers` — the string when set, otherwise nil
// (rendered as JSON null).
func bootstrapDetail(bootstrap string, present bool) any {
	if !present {
		return nil
	}
	return bootstrap
}

// mustMarshalJSON marshals v to JSON, falling back to `null` on
// (effectively impossible) error. Local helper to keep call sites
// compact; the global `must` style isn't used elsewhere in this
// package.
func mustMarshalJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
