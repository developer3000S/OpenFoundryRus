// Package excel is the placeholder Go side of
// `services/connector-management-service/src/connectors/excel.rs`.
//
// The Rust module is an empty file today: no `discover_sources`,
// `query_virtual_table`, `stream_arrow_ipc`, or `build_ingest_spec`
// implementation exists upstream. To keep the connector matrix advertised
// by [adapters.Registry] complete (so route-audit and `/capabilities`
// surfaces enumerate every Rust connector type), this package mounts an
// [Adapter] whose four capability methods return [adapters.ErrNotImplemented].
//
// When Rust grows a real Excel connector, the file-by-file port lands here
// behind the same [ConnectorType] string; callers that resolved through the
// registry switch over without having to re-register.
package excel

import (
	"context"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// ConnectorType is the `connections.connector_type` value the registry
// binds this adapter under, matching Rust's `connectors::excel` module name.
const ConnectorType = "excel"

// Adapter is the skeleton [adapters.ConnectorAdapter] for Excel. It holds
// no state and is safe for concurrent use.
type Adapter struct{}

// New returns a ready-to-use [Adapter].
func New() *Adapter { return &Adapter{} }

// Factory returns an [adapters.Factory] that yields the singleton Adapter.
// Stateless skeleton — sharing one instance across requests is correct.
func Factory() adapters.Factory { return adapters.SingletonFactory(New()) }

// DiscoverSources returns [adapters.ErrNotImplemented]: Rust's
// `connectors::excel` module is an empty placeholder.
func (*Adapter) DiscoverSources(context.Context, *models.Connection, string) ([]adapters.Source, error) {
	return nil, adapters.ErrNotImplemented
}

// QueryVirtualTable returns [adapters.ErrNotImplemented].
func (*Adapter) QueryVirtualTable(context.Context, *models.Connection, *adapters.Query, string) (*adapters.Result, error) {
	return nil, adapters.ErrNotImplemented
}

// StreamArrow returns [adapters.ErrNotImplemented].
func (*Adapter) StreamArrow(context.Context, *models.Connection, *adapters.Query, string) (adapters.ArrowStream, error) {
	return nil, adapters.ErrNotImplemented
}

// BuildIngestSpec returns [adapters.ErrNotImplemented].
func (*Adapter) BuildIngestSpec(context.Context, *models.Connection, *adapters.Source) (*adapters.IngestSpec, error) {
	return nil, adapters.ErrNotImplemented
}
