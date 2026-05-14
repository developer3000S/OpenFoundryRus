package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func (h *Handlers) CommitDatasetOutput(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !canWriteDataset(claims) {
		writePermissionDenied(w, datasetWriteScope, datasetIDParam(r))
		return
	}
	var body models.CommitDatasetOutputRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	dataset, err := h.resolveOrCreateOutputDataset(r, &body, claims.Sub)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
			return
		}
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, err.Error())
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dataset == nil {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return
	}
	if err := h.Repo.EnsureDefaultBranch(r.Context(), dataset); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to ensure default branch")
		return
	}
	branchName := strings.TrimSpace(body.Branch)
	if branchName == "" {
		branchName = dataset.ActiveBranch
	}
	if branchName == "" {
		branchName = "main"
	}
	branch, err := h.Repo.GetRuntimeBranch(r.Context(), dataset.ID, branchName)
	if err != nil || branch == nil {
		if branchName != "main" {
			if fallback, fallbackErr := h.Repo.GetRuntimeBranch(r.Context(), dataset.ID, "main"); fallbackErr == nil && fallback != nil {
				branchName = "main"
				branch = fallback
				err = nil
			}
		}
	}
	if err != nil || branch == nil {
		writeBranchError(w, firstErr(err, repo.ErrNotFound))
		return
	}

	txType := body.TransactionType
	if txType == "" {
		txType = models.TransactionTypeSnapshot
	}
	summary := strings.TrimSpace(body.Summary)
	if summary == "" {
		summary = "Pipeline dataset output commit"
	}
	provenance := body.Provenance
	if len(provenance) == 0 {
		provenance = models.JSONValue([]byte(`{"source":"pipeline-build-service"}`))
	}
	txn, err := h.Repo.StartTransaction(r.Context(), dataset.ID, branch.ID, branch.Name, txType, summary, provenance, claims.Sub)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = h.Repo.AbortTransaction(r.Context(), dataset.ID, txn.ID)
		}
	}()

	staged, indexEntries, err := outputCommitFiles(dataset.ID, txn.ID, body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Repo.StageTransactionFiles(r.Context(), dataset.ID, txn.ID, staged); err != nil {
		writeTransactionError(w, err)
		return
	}
	metadataPatch, err := outputCommitMetadata(body, len(indexEntries))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Repo.MergeTransactionMetadata(r.Context(), dataset.ID, txn.ID, metadataPatch); err != nil {
		writeTransactionError(w, err)
		return
	}
	if err := h.Repo.CommitTransaction(r.Context(), dataset.ID, txn.ID); err != nil {
		writeTransactionError(w, err)
		return
	}
	committed = true
	if len(indexEntries) > 0 {
		if err := h.Repo.ReplaceDatasetFileIndex(r.Context(), dataset.ID, indexEntries); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, "failed to update dataset file index")
			return
		}
	}
	if len(body.LineageLinks) > 0 {
		if err := h.Repo.ReplaceDatasetLineageLinks(r.Context(), dataset.ID, body.LineageLinks); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, "failed to update dataset lineage links")
			return
		}
	}
	after, err := h.Repo.GetRuntimeTransaction(r.Context(), dataset.ID, txn.ID)
	if err != nil || after == nil {
		writeTransactionError(w, firstErr(err, repo.ErrNotFound))
		return
	}
	files, err := h.Repo.ListDatasetFileIndex(r.Context(), dataset.ID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset file index")
		return
	}
	previewBranch := branch.Name
	preview, err := h.Repo.PreviewData(r.Context(), dataset.ID, nil, models.PreviewQuery{Branch: &previewBranch})
	if err != nil {
		writeViewError(w, err)
		return
	}
	links := []models.DatasetLineageLink{}
	if len(body.LineageLinks) > 0 {
		links, _ = h.Repo.ListDatasetLineageLinks(r.Context(), dataset.ID)
	}
	writeJSON(w, http.StatusCreated, models.CommitDatasetOutputResponse{
		DatasetID:    dataset.ID,
		DatasetRID:   "ri.foundry.main.dataset." + dataset.ID.String(),
		Branch:       branch.Name,
		Transaction:  *after,
		Schema:       body.Schema,
		Files:        files,
		Preview:      *preview,
		LineageLinks: links,
	})
}

func (h *Handlers) resolveOrCreateOutputDataset(r *http.Request, body *models.CommitDatasetOutputRequest, actor uuid.UUID) (*models.Dataset, error) {
	raw := datasetIDParam(r)
	datasetID, err := h.Repo.ResolveDatasetID(r.Context(), raw)
	if err == nil {
		return h.Repo.GetDataset(r.Context(), datasetID)
	}
	if !errors.Is(err, repo.ErrNotFound) || !body.CreateIfMissing {
		return nil, err
	}
	id, parseErr := outputDatasetIDFromRID(raw)
	if parseErr != nil {
		return nil, err
	}
	name := strings.TrimSpace(body.DatasetName)
	if name == "" {
		name = "Pipeline output " + id.String()[:8]
	}
	format := "parquet"
	if body.Format != nil && strings.TrimSpace(*body.Format) != "" {
		format = strings.TrimSpace(*body.Format)
	}
	health := "unknown"
	return h.Repo.CreateDataset(r.Context(), &models.CreateDatasetRequest{ID: &id, Name: name, Description: body.Description, Format: &format, HealthStatus: &health, Metadata: body.Metadata}, actor)
}

func outputDatasetIDFromRID(raw string) (uuid.UUID, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "ri.foundry.main.dataset.")
	return uuid.Parse(trimmed)
}

func outputCommitFiles(datasetID uuid.UUID, txnID uuid.UUID, body models.CommitDatasetOutputRequest) ([]models.StageTransactionFile, []models.PutDatasetFileIndexEntry, error) {
	now := time.Now().UTC()
	files := body.Files
	if len(files) == 0 {
		files = []models.CommitOutputFile{{LogicalPath: "part-00000.ndjson", SizeBytes: int64(len(body.PreviewRows))}}
	}
	staged := make([]models.StageTransactionFile, 0, len(files))
	indexEntries := make([]models.PutDatasetFileIndexEntry, 0, len(files))
	for _, file := range files {
		logical := strings.Trim(strings.TrimSpace(file.LogicalPath), "/")
		if logical == "" {
			return nil, nil, errString("logical_path is required")
		}
		storagePath := strings.TrimSpace(file.StoragePath)
		if storagePath == "" {
			storagePath = "outputs/" + txnID.String() + "/" + logical
		}
		entryType := models.FileEntryTypeFile
		if err := validateFileIndexEntry(logical, storagePath, entryType, file.SizeBytes); err != nil {
			return nil, nil, err
		}
		op := file.Operation
		if op == "" {
			op = models.FileOperationAdd
		}
		meta, err := outputFileMetadata(datasetID, txnID, file, body)
		if err != nil {
			return nil, nil, err
		}
		staged = append(staged, models.StageTransactionFile{LogicalPath: logical, PhysicalPath: storagePath, SizeBytes: file.SizeBytes, MediaType: file.ContentType, Operation: op})
		indexEntries = append(indexEntries, models.PutDatasetFileIndexEntry{Path: logical, StoragePath: storagePath, EntryType: &entryType, SizeBytes: &file.SizeBytes, ContentType: file.ContentType, Metadata: meta, LastModified: &now})
	}
	return staged, indexEntries, nil
}

func outputFileMetadata(datasetID uuid.UUID, txnID uuid.UUID, file models.CommitOutputFile, body models.CommitDatasetOutputRequest) (models.JSONValue, error) {
	meta := map[string]any{}
	if len(file.Metadata) > 0 {
		if err := json.Unmarshal(file.Metadata, &meta); err != nil {
			return nil, err
		}
	}
	meta["dataset_id"] = datasetID.String()
	meta["transaction_id"] = txnID.String()
	meta["preview_columns"] = body.PreviewColumns
	meta["preview_rows"] = body.PreviewRows
	if body.Schema != nil {
		meta["schema"] = body.Schema
	}
	raw, err := json.Marshal(meta)
	return models.JSONValue(raw), err
}

func outputCommitMetadata(body models.CommitDatasetOutputRequest, fileCount int) (models.JSONValue, error) {
	meta := map[string]any{"pipeline_output": true}
	if len(body.Metadata) > 0 {
		if err := json.Unmarshal(body.Metadata, &meta); err != nil {
			return nil, err
		}
		meta["pipeline_output"] = true
	}
	if body.Schema != nil {
		meta["schema"] = body.Schema
	}
	meta["preview_columns"] = body.PreviewColumns
	meta["preview_rows"] = body.PreviewRows
	meta["file_count"] = fileCount
	raw, err := json.Marshal(meta)
	return models.JSONValue(raw), err
}

func firstErr(err error, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
