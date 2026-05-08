package adapters

import (
	"context"
	"encoding/json"
	"io"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// ConnectionTestResult mirrors Rust's `connectors::ConnectionTestResult`
// and is returned by adapters that can actively validate a configured
// connection. Details is a raw JSON value so connector-specific payloads
// pass through without the dispatcher knowing their schema.
type ConnectionTestResult struct {
	Success   bool            `json:"success"`
	Message   string          `json:"message"`
	LatencyMS int64           `json:"latency_ms"`
	Details   json.RawMessage `json:"details,omitempty"`
}

// Source is the discovered-source descriptor returned by
// [ConnectorAdapter.DiscoverSources]. It is an alias for
// [models.DiscoveredSource] so callers can pass adapter results directly
// into the existing discovery / registration plumbing in
// `internal/domain/discovery` without conversion.
type Source = models.DiscoveredSource

// Query is the virtual-table preview request consumed by
// [ConnectorAdapter.QueryVirtualTable] and [ConnectorAdapter.StreamArrow].
// Aliased to [models.VirtualTableQueryRequest] to keep the wire shape and
// JSON tags identical to Rust's `VirtualTableQueryRequest`.
type Query = models.VirtualTableQueryRequest

// Result is the JSON virtual-table preview response. Aliased to
// [models.VirtualTableQueryResponse] for the same parity reason as [Query].
type Result = models.VirtualTableQueryResponse

// IngestSpec is the build_ingest_spec output an adapter produces for a
// selected source. It mirrors Rust's `IngestJobSpec` (see
// `services/connector-management-service/src/ingestion_bridge.rs`) but is
// kept as a raw JSON envelope here so the adapters package does not pull
// in the handlers package — handlers/ingestion_bridge.go owns the typed
// shape and is responsible for marshalling per-source variants. Per-source
// fields are merged into [IngestSpec.Source] verbatim; the bridge only
// needs to forward the spec to ingestion-replication-service.
//
// Future per-connector slices will populate [IngestSpec] by calling into
// `handlers.BuildIngestSpec` (or its eventual successor) and re-marshalling
// the result. Defining the type here lets us evolve the bridge surface
// without breaking the adapter contract.
type IngestSpec struct {
	// Name is the ingest_job name; mirrors Rust's `IngestJobSpec.name`. The
	// connector-management dispatcher derives it from the connection name
	// + sync id (see `handlers.SyncJobName`).
	Name string `json:"name"`
	// Namespace scopes the job inside ingestion-replication-service.
	Namespace string `json:"namespace"`
	// Source identifies the per-connector spec variant ("postgres",
	// "kafka", …) and matches the discriminator field Rust's enum emits.
	Source string `json:"source"`
	// Config is the per-source payload, merged at the same JSON level as
	// `Source` when the spec is sent over the wire (e.g. the
	// `"postgres": { … }` block in Rust). Adapters return their already-
	// serialised payload here so the bridge can splice it without re-
	// touching the typed struct.
	Config json.RawMessage `json:"config,omitempty"`
}

// ArrowStream produces successive Arrow IPC byte chunks for a virtual-table
// preview. The contract mirrors Rust's
// `connectors::*::stream_arrow_ipc(...) -> impl Stream<Item = bytes::Bytes>`
// shape: each call to [ArrowStream.Next] returns one chunk (typically a
// schema message followed by record-batch messages). [io.EOF] signals
// completion; callers MUST [ArrowStream.Close] to release driver / HTTP
// resources. Adapters that have not implemented Arrow streaming yet should
// return [ErrNotImplemented] from [ConnectorAdapter.StreamArrow] before
// constructing a stream.
type ArrowStream interface {
	// Next returns the next IPC frame. The returned slice is owned by the
	// caller and must not be retained past the next call without copying.
	// Returns [io.EOF] when the stream is exhausted.
	Next(ctx context.Context) ([]byte, error)
	// Close releases the underlying transport. Calling Close after the
	// stream is exhausted is a no-op.
	Close() error
}

// EmptyArrowStream is the zero-value [ArrowStream] returned by adapters
// that need to satisfy the contract without producing any data (typically
// from [ErrNotImplemented] paths in tests). It returns [io.EOF] on the
// first [EmptyArrowStream.Next] call.
type EmptyArrowStream struct{}

// Next reports [io.EOF] immediately.
func (EmptyArrowStream) Next(context.Context) ([]byte, error) { return nil, io.EOF }

// Close is a no-op on [EmptyArrowStream].
func (EmptyArrowStream) Close() error { return nil }

// ConnectorAdapter is the per-connector capability surface dispatched to
// from `internal/domain/discovery.go` (read-side) and the sync-runtime in
// `internal/handlers/ingestion_bridge.go` (write-side). Each method maps
// one-to-one onto a function in the matching `services/connector-
// management-service/src/connectors/<name>.rs` Rust module:
//
//   - [DiscoverSources]      ← `connectors::<name>::discover_sources`
//   - [QueryVirtualTable]    ← `connectors::<name>::query_virtual_table`
//   - [StreamArrow]          ← `connectors::<name>::stream_arrow_ipc`
//   - [BuildIngestSpec]      ← `connectors::<name>::build_ingest_spec`
//
// Adapters that don't implement a capability MUST return
// [ErrNotImplemented] (wrapped with the connector name where helpful) so
// the dispatcher can translate the failure into a stable error envelope.
//
// `agentURL` is the optional connector-agent endpoint resolved upstream by
// `domain.AgentResolver`; an empty string means "no agent configured" and
// adapters that require an agent should return a descriptive error.
type ConnectorAdapter interface {
	// DiscoverSources returns the catalog of objects the connection
	// exposes (tables, topics, files, …). Mirrors the Rust signature
	// `discover_sources(state, &connection.config, agent_url) -> Vec<DiscoveredSource>`.
	DiscoverSources(ctx context.Context, c *models.Connection, agentURL string) ([]Source, error)

	// QueryVirtualTable returns a JSON-shaped preview of one virtual
	// table. Mirrors `query_virtual_table(state, &connection.config, request, agent_url)`.
	QueryVirtualTable(ctx context.Context, c *models.Connection, q *Query, agentURL string) (*Result, error)

	// StreamArrow returns an [ArrowStream] of IPC frames for the same
	// query. Adapters that only support JSON preview return
	// [ErrNotImplemented]; the dispatcher in handlers translates that
	// into the Rust-equivalent "Arrow streaming is not supported for
	// connector type: …" error.
	StreamArrow(ctx context.Context, c *models.Connection, q *Query, agentURL string) (ArrowStream, error)

	// BuildIngestSpec produces the per-source [IngestSpec] sent over the
	// HTTP bridge to ingestion-replication-service. Mirrors Rust's
	// `ingestion_bridge::build_spec` dispatch by connector type. Adapters
	// that don't yet feed the bridge return [ErrNotImplemented] (callers
	// surface this as the existing [handlers.ErrUnsupportedConnector]
	// envelope so the wire shape stays identical).
	BuildIngestSpec(ctx context.Context, c *models.Connection, src *Source) (*IngestSpec, error)
}

// Factory builds a [ConnectorAdapter]. The registry stores factories
// rather than singleton adapters so per-connector packages can hold
// per-instance state (HTTP clients with custom timeouts, gosnowflake
// driver pools, …) without the registry needing to know about it.
//
// Stateless adapters (postgres, csv, …) typically return the same
// singleton on every call; the contract permits that.
type Factory interface {
	// New returns a [ConnectorAdapter] ready to serve a request. The
	// returned adapter MAY be shared across goroutines; callers should
	// not assume otherwise.
	New() ConnectorAdapter
}

// FactoryFunc adapts a plain constructor function to [Factory], matching
// the `http.HandlerFunc` style used elsewhere in the service.
type FactoryFunc func() ConnectorAdapter

// New satisfies [Factory] by invoking the wrapped function.
func (f FactoryFunc) New() ConnectorAdapter { return f() }

// SingletonFactory wraps an existing [ConnectorAdapter] as a [Factory]
// that returns it unchanged on every call. Convenient for stateless
// adapters where construction is free.
func SingletonFactory(a ConnectorAdapter) Factory {
	return FactoryFunc(func() ConnectorAdapter { return a })
}
