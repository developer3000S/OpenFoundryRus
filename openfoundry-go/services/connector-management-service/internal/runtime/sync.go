// Package runtime contains ports and HTTP adapters for sync execution owned by
// neighbouring services.
package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// IngestionPort dispatches a materialised sync request to ingestion-replication-service.
type IngestionPort interface {
	Dispatch(ctx context.Context, req IngestionRequest) (*IngestionResult, error)
}

// DatasetVersioningPort registers successful sync payloads with dataset-versioning-service.
type DatasetVersioningPort interface {
	Register(ctx context.Context, req DatasetVersionRequest) (*DatasetVersionResult, error)
}

// IngestionRequest is the local control-plane envelope sent to ingestion-replication.
type IngestionRequest struct {
	RunID           uuid.UUID       `json:"run_id"`
	SyncDefID       uuid.UUID       `json:"sync_def_id"`
	SourceID        uuid.UUID       `json:"source_id"`
	OutputDatasetID uuid.UUID       `json:"output_dataset_id"`
	Name            string          `json:"name"`
	Namespace       string          `json:"namespace"`
	ConnectorType   string          `json:"connector_type"`
	Connection      json.RawMessage `json:"connection"`
	FileGlob        *string         `json:"file_glob,omitempty"`
	Spec            json.RawMessage `json:"spec"`
	Materialized    []byte          `json:"-"`
	ContentHash     string          `json:"content_hash"`
}

// IngestionResult captures the remote job id and sync accounting returned by the port.
type IngestionResult struct {
	IngestJobID  string
	BytesWritten int64
	FilesWritten int64
	RowsWritten  int64
	Payload      []byte
}

// DatasetVersionRequest is the append payload submitted to dataset-versioning.
type DatasetVersionRequest struct {
	SyncDefID       uuid.UUID       `json:"sync_def_id"`
	RunID           uuid.UUID       `json:"run_id"`
	SourceID        uuid.UUID       `json:"source_id"`
	OutputDatasetID uuid.UUID       `json:"output_dataset_id"`
	ContentHash     string          `json:"content_hash"`
	RowCount        int64           `json:"row_count"`
	SizeBytes       int64           `json:"size_bytes"`
	Schema          json.RawMessage `json:"schema"`
	Message         string          `json:"message"`
}

// DatasetVersionResult is the version id returned by dataset-versioning-service.
type DatasetVersionResult struct {
	DatasetVersionID uuid.UUID
}

// Materialize builds the deterministic ingestion payload and content hash for a sync run.
func Materialize(runID uuid.UUID, job *models.SyncJob, conn *models.Connection) (IngestionRequest, error) {
	if job == nil || conn == nil {
		return IngestionRequest{}, fmt.Errorf("sync job and connection are required")
	}
	namespace := strings.TrimSpace(conn.Name)
	if namespace == "" {
		namespace = conn.ID.String()
	}
	name := fmt.Sprintf("sync-%s", job.ID.String())
	specMap := map[string]any{
		"run_id":            runID,
		"sync_def_id":       job.ID,
		"source_id":         conn.ID,
		"output_dataset_id": job.OutputDatasetID,
		"connector_type":    conn.ConnectorType,
		"connection_config": json.RawMessage(conn.Config),
	}
	if job.FileGlob != nil {
		specMap["file_glob"] = *job.FileGlob
	}
	spec, err := json.Marshal(specMap)
	if err != nil {
		return IngestionRequest{}, err
	}
	envelope := map[string]any{
		"name":      name,
		"namespace": namespace,
		"spec":      json.RawMessage(spec),
	}
	materialized, err := json.Marshal(envelope)
	if err != nil {
		return IngestionRequest{}, err
	}
	digest := sha256.Sum256(materialized)
	return IngestionRequest{RunID: runID, SyncDefID: job.ID, SourceID: conn.ID, OutputDatasetID: job.OutputDatasetID, Name: name, Namespace: namespace, ConnectorType: conn.ConnectorType, Connection: conn.Config, FileGlob: job.FileGlob, Spec: spec, Materialized: materialized, ContentHash: fmt.Sprintf("%x", digest[:])}, nil
}

// HTTPIngestionClient is the production adapter for ingestion-replication's REST control plane.
type HTTPIngestionClient struct {
	BaseURL string
	Client  *http.Client
}

func (c HTTPIngestionClient) Dispatch(ctx context.Context, req IngestionRequest) (*IngestionResult, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("ingestion-replication url is not configured")
	}
	body, err := json.Marshal(map[string]any{"name": req.Name, "namespace": req.Namespace, "spec": json.RawMessage(req.Spec)})
	if err != nil {
		return nil, err
	}
	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/ingest-jobs", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ingestion-replication responded with status %d", resp.StatusCode)
	}
	var decoded struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return &IngestionResult{IngestJobID: decoded.ID, BytesWritten: int64(len(req.Materialized)), FilesWritten: 1, Payload: req.Materialized}, nil
}

// HTTPDatasetVersioningClient is the production adapter for dataset-versioning-service.
type HTTPDatasetVersioningClient struct {
	BaseURL string
	Client  *http.Client
}

func (c HTTPDatasetVersioningClient) Register(ctx context.Context, req DatasetVersionRequest) (*DatasetVersionResult, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("dataset-versioning url is not configured")
	}
	body, err := json.Marshal(map[string]any{
		"branch_name":      nil,
		"message":          req.Message,
		"row_delta":        req.RowCount,
		"size_delta_bytes": req.SizeBytes,
		"metadata": map[string]any{
			"content_hash": req.ContentHash,
			"schema":       json.RawMessage(req.Schema),
			"lineage":      map[string]any{"source_id": req.SourceID, "sync_def_id": req.SyncDefID, "sync_run_id": req.RunID},
		},
	})
	if err != nil {
		return nil, err
	}
	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/v1/datasets/%s/append", base, req.OutputDatasetID), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("dataset-versioning responded with status %d", resp.StatusCode)
	}
	var decoded struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	return &DatasetVersionResult{DatasetVersionID: decoded.ID}, nil
}
