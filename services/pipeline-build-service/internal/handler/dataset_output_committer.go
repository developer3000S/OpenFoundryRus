package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

const datasetRIDPrefix = "ri.foundry.main.dataset."

// DatasetServiceOutputCommitter materializes successful output nodes into the
// dataset-versioning service, then delegates to the metadata committer so the
// pipeline-build job_outputs table remains an execution audit trail.
type DatasetServiceOutputCommitter struct {
	BaseURL     string
	BearerToken string
	Client      *http.Client
	Metadata    executor.OutputCommitter
}

func (c DatasetServiceOutputCommitter) Commit(ctx context.Context, tx executor.OutputTransaction, result executor.NodeResult) error {
	if !canCommitDatasetOutput(tx.DatasetRID) {
		return commitOutputMetadata(ctx, c.Metadata, tx, result)
	}
	body, err := datasetOutputCommitBody(tx, result)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint, err := c.commitEndpoint(tx.DatasetRID)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.BearerToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dataset_output_commit_failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("dataset_output_commit_failed: dataset-versioning returned %s: %s", resp.Status, strings.TrimSpace(string(detail)))
	}
	return commitOutputMetadata(ctx, c.Metadata, tx, result)
}

func (c DatasetServiceOutputCommitter) commitEndpoint(datasetRID string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("dataset_output_commit_not_configured: set DATASET_SERVICE_URL to commit dataset outputs")
	}
	if _, err := url.ParseRequestURI(base); err != nil {
		return "", fmt.Errorf("dataset_output_commit_not_configured: invalid DATASET_SERVICE_URL: %w", err)
	}
	return base + "/v1/datasets/" + url.PathEscape(routeDatasetRID(datasetRID)) + "/outputs:commit", nil
}

func commitOutputMetadata(ctx context.Context, committer executor.OutputCommitter, tx executor.OutputTransaction, result executor.NodeResult) error {
	if committer == nil {
		return nil
	}
	return committer.Commit(ctx, tx, result)
}

func canCommitDatasetOutput(datasetRID string) bool {
	trimmed := strings.TrimSpace(datasetRID)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, datasetRIDPrefix) {
		_, err := uuid.Parse(strings.TrimPrefix(trimmed, datasetRIDPrefix))
		return err == nil
	}
	_, err := uuid.Parse(trimmed)
	return err == nil
}

func routeDatasetRID(datasetRID string) string {
	trimmed := strings.TrimSpace(datasetRID)
	if strings.HasPrefix(trimmed, datasetRIDPrefix) {
		return trimmed
	}
	if _, err := uuid.Parse(trimmed); err == nil {
		return datasetRIDPrefix + trimmed
	}
	return trimmed
}

type datasetOutputCommitRequest struct {
	CreateIfMissing bool                       `json:"create_if_missing"`
	DatasetName     string                     `json:"dataset_name,omitempty"`
	Format          *string                    `json:"format,omitempty"`
	Branch          string                     `json:"branch,omitempty"`
	TransactionType string                     `json:"transaction_type,omitempty"`
	Summary         string                     `json:"summary,omitempty"`
	Provenance      json.RawMessage            `json:"provenance,omitempty"`
	Schema          *datasetOutputSchema       `json:"schema,omitempty"`
	Files           []datasetOutputFile        `json:"files,omitempty"`
	PreviewColumns  []string                   `json:"preview_columns,omitempty"`
	PreviewRows     [][]json.RawMessage        `json:"preview_rows,omitempty"`
	LineageLinks    []datasetOutputLineageLink `json:"lineage_links,omitempty"`
	Metadata        json.RawMessage            `json:"metadata,omitempty"`
}

type datasetOutputSchema struct {
	Fields     []datasetOutputField `json:"fields"`
	FileFormat string               `json:"file_format"`
}

type datasetOutputField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

type datasetOutputFile struct {
	LogicalPath string          `json:"logical_path"`
	StoragePath string          `json:"storage_path,omitempty"`
	SizeBytes   int64           `json:"size_bytes"`
	ContentType *string         `json:"content_type,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Operation   string          `json:"operation,omitempty"`
}

type datasetOutputLineageLink struct {
	Direction    string          `json:"direction"`
	TargetRID    string          `json:"target_rid"`
	TargetKind   *string         `json:"target_kind,omitempty"`
	RelationKind *string         `json:"relation_kind,omitempty"`
	PipelineID   *string         `json:"pipeline_id,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

func datasetOutputCommitBody(tx executor.OutputTransaction, result executor.NodeResult) (datasetOutputCommitRequest, error) {
	rows := resultRows(result.Metadata)
	columns := resultColumns(result.Metadata)
	if len(columns) == 0 {
		columns = inferResultColumns(rows)
	}
	previewRows := orderedPreviewRows(columns, rows, 100)
	payload := encodeNDJSON(rows)
	sum := sha256.Sum256(payload)
	fileMeta, err := json.Marshal(map[string]any{
		"sha256":              hex.EncodeToString(sum[:]),
		"output_content_hash": result.OutputContentHash,
		"output_node_id":      tx.OutputNodeID,
		"source_node_id":      tx.SourceNodeID,
	})
	if err != nil {
		return datasetOutputCommitRequest{}, err
	}
	logicalPath := strings.Trim(strings.TrimSpace(tx.LogicalPath), "/")
	if logicalPath == "" {
		logicalPath = "part-00000.ndjson"
	}
	storagePath := "pipeline-build/" + strings.Trim(strings.TrimSpace(tx.TransactionRID), "/") + "/" + strings.Trim(strings.TrimSpace(tx.OutputNodeID), "/") + "/" + logicalPath
	storagePath = strings.Trim(strings.ReplaceAll(storagePath, "//", "/"), "/")
	contentType := "application/x-ndjson"
	format := normaliseOutputFileFormat(tx.FileFormat)
	bodyMeta, err := json.Marshal(datasetOutputCommitMetadata(tx, result, len(rows), len(payload)))
	if err != nil {
		return datasetOutputCommitRequest{}, err
	}
	provenance, err := json.Marshal(map[string]any{
		"source":          "pipeline-build-service",
		"pipeline_rid":    tx.PipelineRID,
		"output_node_id":  tx.OutputNodeID,
		"source_node_id":  tx.SourceNodeID,
		"transaction_rid": tx.TransactionRID,
	})
	if err != nil {
		return datasetOutputCommitRequest{}, err
	}
	return datasetOutputCommitRequest{
		CreateIfMissing: tx.CreateIfMissing,
		DatasetName:     defaultDatasetOutputName(tx),
		Format:          &format,
		Branch:          defaultString(tx.Branch, "main"),
		TransactionType: defaultString(strings.ToUpper(strings.TrimSpace(tx.WriteMode)), "SNAPSHOT"),
		Summary:         "Pipeline output commit for " + defaultString(tx.OutputNodeID, "output"),
		Provenance:      provenance,
		Schema:          inferOutputSchema(columns, rows, format),
		Files: []datasetOutputFile{{
			LogicalPath: logicalPath,
			StoragePath: storagePath,
			SizeBytes:   int64(len(payload)),
			ContentType: &contentType,
			Metadata:    fileMeta,
			Operation:   "ADD",
		}},
		PreviewColumns: columns,
		PreviewRows:    previewRows,
		LineageLinks:   outputLineageLinks(tx),
		Metadata:       bodyMeta,
	}, nil
}

func datasetOutputCommitMetadata(tx executor.OutputTransaction, result executor.NodeResult, rowCount int, sizeBytes int) map[string]any {
	meta := map[string]any{
		"pipeline_output":     true,
		"pipeline_rid":        tx.PipelineRID,
		"output_node_id":      tx.OutputNodeID,
		"source_node_id":      tx.SourceNodeID,
		"transaction_rid":     tx.TransactionRID,
		"output_content_hash": result.OutputContentHash,
		"row_count":           rowCount,
		"size_bytes":          sizeBytes,
	}
	for _, key := range []string{"runtime", "engine", "transform_type", "rows_affected"} {
		if value, ok := result.Metadata[key]; ok {
			meta[key] = value
		}
	}
	return meta
}

func defaultDatasetOutputName(tx executor.OutputTransaction) string {
	if strings.TrimSpace(tx.DatasetName) != "" {
		return strings.TrimSpace(tx.DatasetName)
	}
	if strings.TrimSpace(tx.OutputNodeID) != "" {
		return "Pipeline output " + strings.TrimSpace(tx.OutputNodeID)
	}
	return "Pipeline output"
}

func normaliseOutputFileFormat(format string) string {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case "AVRO":
		return "AVRO"
	case "TEXT", "CSV", "JSON", "NDJSON":
		return "TEXT"
	default:
		return "PARQUET"
	}
}

func resultColumns(meta map[string]any) []string {
	if meta == nil {
		return nil
	}
	switch value := meta["columns"].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, entry := range value {
			if text, ok := entry.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

func resultRows(meta map[string]any) []map[string]json.RawMessage {
	if meta == nil {
		return nil
	}
	if rows := decodeResultRows(meta["data_rows"]); len(rows) > 0 {
		return rows
	}
	return decodeResultRows(meta["sample_rows"])
}

func decodeResultRows(value any) []map[string]json.RawMessage {
	switch rows := value.(type) {
	case []map[string]json.RawMessage:
		out := make([]map[string]json.RawMessage, 0, len(rows))
		for _, row := range rows {
			out = append(out, cloneRawRow(row))
		}
		return out
	case []map[string]any:
		out := make([]map[string]json.RawMessage, 0, len(rows))
		for _, row := range rows {
			out = append(out, anyRowToRaw(row))
		}
		return out
	case []any:
		out := make([]map[string]json.RawMessage, 0, len(rows))
		for _, entry := range rows {
			switch row := entry.(type) {
			case map[string]json.RawMessage:
				out = append(out, cloneRawRow(row))
			case map[string]any:
				out = append(out, anyRowToRaw(row))
			}
		}
		return out
	case json.RawMessage:
		var decoded []map[string]json.RawMessage
		if err := json.Unmarshal(rows, &decoded); err == nil {
			return decoded
		}
	case []byte:
		var decoded []map[string]json.RawMessage
		if err := json.Unmarshal(rows, &decoded); err == nil {
			return decoded
		}
	}
	return nil
}

func cloneRawRow(row map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(row))
	for key, value := range row {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}

func anyRowToRaw(row map[string]any) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(row))
	for key, value := range row {
		raw, err := json.Marshal(value)
		if err != nil {
			raw = []byte("null")
		}
		out[key] = raw
	}
	return out
}

func inferResultColumns(rows []map[string]json.RawMessage) []string {
	seen := map[string]struct{}{}
	for _, row := range rows {
		for column := range row {
			seen[column] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for column := range seen {
		out = append(out, column)
	}
	sort.Strings(out)
	return out
}

func orderedPreviewRows(columns []string, rows []map[string]json.RawMessage, limit int) [][]json.RawMessage {
	if limit <= 0 || limit > len(rows) {
		limit = len(rows)
	}
	out := make([][]json.RawMessage, 0, limit)
	for _, row := range rows[:limit] {
		next := make([]json.RawMessage, 0, len(columns))
		for _, column := range columns {
			value := row[column]
			if len(value) == 0 {
				value = []byte("null")
			}
			next = append(next, append(json.RawMessage(nil), value...))
		}
		out = append(out, next)
	}
	return out
}

func encodeNDJSON(rows []map[string]json.RawMessage) []byte {
	var buf bytes.Buffer
	for _, row := range rows {
		raw, err := json.Marshal(row)
		if err != nil {
			continue
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func inferOutputSchema(columns []string, rows []map[string]json.RawMessage, format string) *datasetOutputSchema {
	fields := make([]datasetOutputField, 0, len(columns))
	for _, column := range columns {
		fields = append(fields, datasetOutputField{Name: column, Type: inferSchemaType(column, rows), Nullable: inferNullable(column, rows)})
	}
	return &datasetOutputSchema{Fields: fields, FileFormat: format}
}

func inferSchemaType(column string, rows []map[string]json.RawMessage) string {
	for _, row := range rows {
		raw := bytes.TrimSpace(row[column])
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			continue
		}
		var value any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			return "STRING"
		}
		switch typed := value.(type) {
		case bool:
			return "BOOLEAN"
		case json.Number:
			text := typed.String()
			if strings.ContainsAny(text, ".eE") {
				return "DOUBLE"
			}
			return "LONG"
		case string:
			return "STRING"
		default:
			return "STRING"
		}
	}
	return "STRING"
}

func inferNullable(column string, rows []map[string]json.RawMessage) bool {
	if len(rows) == 0 {
		return true
	}
	for _, row := range rows {
		raw, ok := row[column]
		if !ok || len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return true
		}
	}
	return false
}

func outputLineageLinks(tx executor.OutputTransaction) []datasetOutputLineageLink {
	targetKind := "dataset"
	relationKind := "derived_from"
	var pipelineID *string
	if strings.TrimSpace(tx.PipelineRID) != "" {
		value := strings.TrimSpace(tx.PipelineRID)
		pipelineID = &value
	}
	out := make([]datasetOutputLineageLink, 0, len(tx.InputDatasetRIDs))
	for _, input := range tx.InputDatasetRIDs {
		target := strings.TrimSpace(input)
		if target == "" {
			continue
		}
		if _, err := uuid.Parse(target); err == nil {
			target = datasetRIDPrefix + target
		}
		meta, _ := json.Marshal(map[string]any{
			"output_node_id": tx.OutputNodeID,
			"source_node_id": tx.SourceNodeID,
		})
		out = append(out, datasetOutputLineageLink{Direction: "upstream", TargetRID: target, TargetKind: &targetKind, RelationKind: &relationKind, PipelineID: pipelineID, Metadata: meta})
	}
	return out
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
