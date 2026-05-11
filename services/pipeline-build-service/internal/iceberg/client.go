// Package iceberg provides the productive OutputCommitter / TransactionManager
// adapter for pipeline-build-service Iceberg outputs (ADR-0041).
package iceberg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

const DatasetRIDPrefix = "ri.foundry.main.iceberg-table."

var (
	ErrTransactionNotFound = errors.New("iceberg transaction not found")
	ErrTableNotFound       = errors.New("iceberg table not found")
	ErrSchemaMismatch      = errors.New("iceberg schema mismatch")
	ErrCommitFailed        = errors.New("iceberg commit failed")
	ErrRollbackFailed      = errors.New("iceberg rollback failed")
)

type FieldSpec struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type TableSpec struct {
	Catalog            string      `json:"catalog"`
	Warehouse          string      `json:"warehouse,omitempty"`
	Namespace          string      `json:"namespace"`
	Table              string      `json:"table"`
	PartitionTransform string      `json:"partition_transform,omitempty"`
	SortOrder          string      `json:"sort_order,omitempty"`
	Schema             []FieldSpec `json:"schema"`
}

type StagedTransaction struct {
	DatasetRID     string           `json:"dataset_rid"`
	TransactionRID string           `json:"transaction_rid"`
	Spec           TableSpec        `json:"spec"`
	Rows           []map[string]any `json:"rows"`
}

type AppendBatch struct {
	Spec           TableSpec        `json:"spec"`
	TransactionRID string           `json:"transaction_rid"`
	Rows           []map[string]any `json:"rows"`
}

type RollbackRequest struct {
	Spec           TableSpec `json:"spec"`
	TransactionRID string    `json:"transaction_rid"`
}

type TransactionStore interface {
	LoadTransaction(ctx context.Context, tx executor.OutputTransaction) (*StagedTransaction, error)
	MarkCommitted(ctx context.Context, tx executor.OutputTransaction) error
	MarkAborted(ctx context.Context, tx executor.OutputTransaction) error
}

type TableWriterCatalog interface {
	LoadTable(ctx context.Context, spec TableSpec) (TableWriter, error)
}

type TableWriter interface {
	Append(ctx context.Context, batch AppendBatch) error
	Rollback(ctx context.Context, req RollbackRequest) error
}

type OutputClient struct {
	Store   TransactionStore
	Catalog TableWriterCatalog
}

func NewOutputClient(store TransactionStore, catalog TableWriterCatalog) *OutputClient {
	return &OutputClient{Store: store, Catalog: catalog}
}

func (c *OutputClient) Commit(ctx context.Context, tx executor.OutputTransaction, _ executor.NodeResult) error {
	if !Handles(tx.DatasetRID) {
		return nil
	}
	if c == nil || c.Store == nil || c.Catalog == nil {
		return fmt.Errorf("%w: iceberg output client is not configured", ErrCommitFailed)
	}
	staged, err := c.Store.LoadTransaction(ctx, tx)
	if err != nil {
		return mapStoreError(err)
	}
	if staged == nil {
		return fmt.Errorf("%w: %s", ErrTransactionNotFound, tx.TransactionRID)
	}
	if staged.DatasetRID != "" && staged.DatasetRID != tx.DatasetRID {
		return fmt.Errorf("%w: transaction dataset %s does not match %s", ErrCommitFailed, staged.DatasetRID, tx.DatasetRID)
	}
	if staged.TransactionRID != "" && staged.TransactionRID != tx.TransactionRID {
		return fmt.Errorf("%w: staged transaction %s does not match %s", ErrCommitFailed, staged.TransactionRID, tx.TransactionRID)
	}
	if err := ValidateBatch(staged.Spec.Schema, staged.Rows); err != nil {
		return err
	}
	table, err := c.Catalog.LoadTable(ctx, staged.Spec)
	if err != nil {
		return mapCatalogError(err, ErrTableNotFound)
	}
	if err := table.Append(ctx, AppendBatch{Spec: staged.Spec, TransactionRID: tx.TransactionRID, Rows: staged.Rows}); err != nil {
		return mapCatalogError(err, ErrCommitFailed)
	}
	if err := c.Store.MarkCommitted(ctx, tx); err != nil {
		return fmt.Errorf("%w: mark committed: %v", ErrCommitFailed, err)
	}
	return nil
}

func (c *OutputClient) Abort(ctx context.Context, tx executor.OutputTransaction) error {
	if !Handles(tx.DatasetRID) {
		return nil
	}
	if c == nil || c.Store == nil || c.Catalog == nil {
		return fmt.Errorf("%w: iceberg output client is not configured", ErrRollbackFailed)
	}
	staged, err := c.Store.LoadTransaction(ctx, tx)
	if err != nil {
		return mapStoreError(err)
	}
	if staged == nil {
		return fmt.Errorf("%w: %s", ErrTransactionNotFound, tx.TransactionRID)
	}
	table, err := c.Catalog.LoadTable(ctx, staged.Spec)
	if err != nil {
		return mapCatalogError(err, ErrTableNotFound)
	}
	if err := table.Rollback(ctx, RollbackRequest{Spec: staged.Spec, TransactionRID: tx.TransactionRID}); err != nil {
		return mapCatalogError(err, ErrRollbackFailed)
	}
	if err := c.Store.MarkAborted(ctx, tx); err != nil {
		return fmt.Errorf("%w: mark aborted: %v", ErrRollbackFailed, err)
	}
	return nil
}

func Handles(datasetRID string) bool { return strings.HasPrefix(datasetRID, DatasetRIDPrefix) }

func ValidateBatch(schema []FieldSpec, rows []map[string]any) error {
	if len(schema) == 0 {
		return fmt.Errorf("%w: schema must not be empty", ErrSchemaMismatch)
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: append batch must contain at least one row", ErrCommitFailed)
	}
	for _, field := range schema {
		if field.Name == "" || field.Type == "" {
			return fmt.Errorf("%w: field id %d has empty name/type", ErrSchemaMismatch, field.ID)
		}
	}
	for i, row := range rows {
		for _, field := range schema {
			value, ok := row[field.Name]
			if field.Required && (!ok || value == nil) {
				return fmt.Errorf("%w: row %d missing required field %s", ErrSchemaMismatch, i, field.Name)
			}
			if ok && value != nil && !typeMatches(field.Type, value) {
				return fmt.Errorf("%w: row %d field %s expected %s", ErrSchemaMismatch, i, field.Name, field.Type)
			}
		}
	}
	return nil
}

func typeMatches(t string, value any) bool {
	switch strings.ToLower(t) {
	case "string", "uuid", "timestamptz", "timestamp", "date":
		_, ok := value.(string)
		return ok
	case "int", "int32", "long", "int64", "uint32", "uint64":
		switch value.(type) {
		case int, int32, int64, uint, uint32, uint64, float64, json.Number:
			return true
		default:
			return false
		}
	case "bool", "boolean":
		_, ok := value.(bool)
		return ok
	case "json", "string_json":
		return true
	default:
		return true
	}
}

func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrTransactionNotFound) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrCommitFailed, err)
}

func mapCatalogError(err error, fallback error) error {
	if err == nil {
		return nil
	}
	for _, sentinel := range []error{ErrTableNotFound, ErrSchemaMismatch, ErrCommitFailed, ErrRollbackFailed} {
		if errors.Is(err, sentinel) {
			return err
		}
	}
	return fmt.Errorf("%w: %v", fallback, err)
}

type HTTPTableWriterCatalog struct {
	baseURL string
	client  *http.Client
	bearer  string
}

func NewHTTPTableWriterCatalog(baseURL, bearer string) *HTTPTableWriterCatalog {
	return &HTTPTableWriterCatalog{baseURL: strings.TrimRight(baseURL, "/"), bearer: bearer, client: &http.Client{Timeout: 30 * time.Second}}
}

func (c *HTTPTableWriterCatalog) LoadTable(_ context.Context, spec TableSpec) (TableWriter, error) {
	if spec.Namespace == "" || spec.Table == "" {
		return nil, fmt.Errorf("%w: namespace/table must be non-empty", ErrTableNotFound)
	}
	return &httpTableWriter{catalog: c, spec: spec}, nil
}

type httpTableWriter struct {
	catalog *HTTPTableWriterCatalog
	spec    TableSpec
}

func (w *httpTableWriter) Append(ctx context.Context, batch AppendBatch) error {
	return w.post(ctx, "/openfoundry/iceberg/v1/append", batch, ErrCommitFailed)
}

func (w *httpTableWriter) Rollback(ctx context.Context, req RollbackRequest) error {
	return w.post(ctx, "/openfoundry/iceberg/v1/rollback", req, ErrRollbackFailed)
}

func (w *httpTableWriter) post(ctx context.Context, path string, body any, fallback error) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.catalog.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if w.catalog.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+w.catalog.bearer)
	}
	resp, err := w.catalog.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", fallback, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return ErrTableNotFound
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return ErrSchemaMismatch
	default:
		return fmt.Errorf("%w: HTTP %d from table-writer adapter", fallback, resp.StatusCode)
	}
}
