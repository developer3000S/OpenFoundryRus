package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
)

const (
	auditCatalog            = "lakekeeper"
	auditPartitionTransform = "day(at)"
	auditSortOrder          = "at ASC"
)

var (
	ErrEmptyBatch     = errors.New("iceberg append batch is empty")
	ErrTableNotFound  = errors.New("iceberg table not found")
	ErrSchemaMismatch = errors.New("iceberg table schema mismatch")
	ErrCommitFailed   = errors.New("iceberg commit failed")
)

type FieldSpec struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type TableSpec struct {
	Catalog            string      `json:"catalog"`
	CatalogURL         string      `json:"catalog_url,omitempty"`
	Warehouse          string      `json:"warehouse,omitempty"`
	Namespace          string      `json:"namespace"`
	Table              string      `json:"table"`
	PartitionTransform string      `json:"partition_transform"`
	SortOrder          string      `json:"sort_order"`
	Schema             []FieldSpec `json:"schema"`
}

type AppendBatch struct {
	Spec TableSpec        `json:"spec"`
	Rows []map[string]any `json:"rows"`
}

type IcebergCatalog interface {
	LoadTable(ctx context.Context, spec TableSpec) (IcebergTableAppender, error)
}

type IcebergTableAppender interface {
	Append(ctx context.Context, batch AppendBatch) error
}

// IcebergWriter appends audit batches to `lakekeeper/of_audit/events`.
//
// Contract parity with the Rust sink:
//   - namespace/table: `of_audit.events`
//   - schema: event_id uuid, at timestamptz, correlation_id string?, kind string, payload string
//   - partition/sort: `day(at)`, `at ASC`
//   - batch append: one durable table append per Writer.Append call
//
// The default implementation is an explicit OpenFoundry HTTP table-writer
// adapter because iceberg-go does not yet provide a stable, complete
// write-side path equivalent to Rust's `append_record_batches`. The adapter
// must atomically write Parquet data files and commit the Iceberg snapshot;
// HTTP 404 maps to ErrTableNotFound, 409/422 to ErrSchemaMismatch, and
// all other non-2xx responses to ErrCommitFailed. Tests can inject a fake catalog with
// NewIcebergWriterWithCatalog.
type IcebergWriter struct {
	CatalogURL string
	Warehouse  string
	Namespace  string
	Table      string
	catalog    IcebergCatalog
}

func NewIcebergWriter(catalogURL, warehouse, namespace, table string) *IcebergWriter {
	return NewIcebergWriterWithCatalog(
		catalogURL, warehouse, namespace, table,
		NewHTTPTableWriterCatalog(catalogURL, warehouse),
	)
}

func NewIcebergWriterWithCatalog(catalogURL, warehouse, namespace, table string, catalog IcebergCatalog) *IcebergWriter {
	return &IcebergWriter{
		CatalogURL: catalogURL,
		Warehouse:  warehouse,
		Namespace:  namespace,
		Table:      table,
		catalog:    catalog,
	}
}

func (i *IcebergWriter) Append(ctx context.Context, events []envelope.AuditEnvelope) error {
	if len(events) == 0 {
		return ErrEmptyBatch
	}
	spec := i.tableSpec()
	table, err := i.catalog.LoadTable(ctx, spec)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(events))
	for _, event := range events {
		row := map[string]any{
			"event_id":       event.EventID.String(),
			"at":             event.At,
			"correlation_id": nil,
			"kind":           event.Kind,
			"payload":        string(event.Payload),
		}
		if event.CorrelationID != nil {
			row["correlation_id"] = *event.CorrelationID
		}
		rows = append(rows, row)
	}
	if err := table.Append(ctx, AppendBatch{Spec: spec, Rows: rows}); err != nil {
		return err
	}
	return nil
}

func (i *IcebergWriter) Close() error { return nil }

func (i *IcebergWriter) tableSpec() TableSpec {
	return TableSpec{
		Catalog:            auditCatalog,
		CatalogURL:         i.CatalogURL,
		Warehouse:          i.Warehouse,
		Namespace:          i.Namespace,
		Table:              i.Table,
		PartitionTransform: auditPartitionTransform,
		SortOrder:          auditSortOrder,
		Schema:             auditSchema(),
	}
}

func auditSchema() []FieldSpec {
	return []FieldSpec{
		{ID: 1, Name: "event_id", Type: "uuid", Required: true},
		{ID: 2, Name: "at", Type: "timestamptz", Required: true},
		{ID: 3, Name: "correlation_id", Type: "string", Required: false},
		{ID: 4, Name: "kind", Type: "string", Required: true},
		{ID: 5, Name: "payload", Type: "string", Required: true},
	}
}

type HTTPTableWriterCatalog struct {
	baseURL    string
	catalogURL string
	warehouse  string
	client     *http.Client
}

func NewHTTPTableWriterCatalog(catalogURL, warehouse string) *HTTPTableWriterCatalog {
	return &HTTPTableWriterCatalog{
		baseURL:    strings.TrimRight(catalogURL, "/"),
		catalogURL: strings.TrimRight(catalogURL, "/"),
		warehouse:  warehouse,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HTTPTableWriterCatalog) LoadTable(_ context.Context, spec TableSpec) (IcebergTableAppender, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("%w: table-writer URL must be non-empty", ErrCommitFailed)
	}
	if spec.CatalogURL == "" {
		spec.CatalogURL = c.catalogURL
	}
	if spec.Warehouse == "" {
		spec.Warehouse = c.warehouse
	}
	if spec.Namespace == "" || spec.Table == "" {
		return nil, fmt.Errorf("%w: namespace/table must be non-empty", ErrTableNotFound)
	}
	return &httpTableWriter{catalog: c, spec: spec}, nil
}

type httpTableWriter struct {
	catalog *HTTPTableWriterCatalog
	spec    TableSpec
}

func (t *httpTableWriter) Append(ctx context.Context, batch AppendBatch) error {
	if batch.Spec.Catalog == "" || batch.Spec.Namespace == "" || batch.Spec.Table == "" {
		batch.Spec = t.spec
	}
	if batch.Spec.CatalogURL == "" {
		batch.Spec.CatalogURL = t.spec.CatalogURL
	}
	if batch.Spec.Warehouse == "" {
		batch.Spec.Warehouse = t.spec.Warehouse
	}
	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	url := t.catalog.baseURL + "/openfoundry/iceberg/v1/append"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.catalog.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCommitFailed, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	detail := tableWriterErrorDetail(resp)
	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w%s", ErrTableNotFound, detail)
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return fmt.Errorf("%w%s", ErrSchemaMismatch, detail)
	default:
		return fmt.Errorf("%w: HTTP %d from table-writer adapter%s", ErrCommitFailed, resp.StatusCode, detail)
	}
}

func tableWriterErrorDetail(resp *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return ""
	}
	return ": " + string(bytes.TrimSpace(body))
}
