package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

type localObjectStore interface {
	ReadLocalObject(key string) ([]byte, error)
	WriteLocalObject(key string, data []byte) error
	VerifyLocalSignature(key string, expires time.Time, sig string) bool
}

func (h *Handlers) LocalPresignProxy(w http.ResponseWriter, r *http.Request) {
	local, ok := h.BackingFS.(localObjectStore)
	if !ok || h.BackingFS == nil || h.BackingFS.FSID() != "local" {
		writeDependencyUnavailable(w, "local_backing_filesystem_unavailable", "local backing filesystem not configured")
		return
	}
	key := localFSKey(r)
	if !safeObjectKey(key) {
		writeJSONErr(w, http.StatusBadRequest, "invalid local object key")
		return
	}
	expires, err := parseLocalExpires(r.URL.Query().Get("expires"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid expires")
		return
	}
	if !local.VerifyLocalSignature(key, expires, r.URL.Query().Get("sig")) {
		writeJSONErr(w, http.StatusForbidden, "invalid or expired signature")
		return
	}
	bytes, err := local.ReadLocalObject(key)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			writeJSONErr(w, http.StatusNotFound, "object not found")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to read object")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bytes)
}

func (h *Handlers) StorageDetails(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	if h.BackingFS == nil {
		writeDependencyUnavailable(w, "backing_filesystem_unavailable", "backing filesystem not configured")
		return
	}
	ttl := h.PresignTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	fsID := h.BackingFS.FSID()
	driver := backingDriver(fsID)
	out, err := h.Repo.StorageDetails(r.Context(), datasetID, fsID, driver, h.BackingFS.BaseDirectory(), uint64(ttl/time.Second))
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load storage details")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) UploadData(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireDatasetWrite(w, r, datasetID); !ok {
		return
	}
	local, ok := h.BackingFS.(localObjectStore)
	if !ok || h.BackingFS == nil || h.BackingFS.FSID() != "local" {
		writeDependencyUnavailable(w, "local_backing_filesystem_unavailable", "local backing filesystem not configured")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid multipart body")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "file part is required")
		return
	}
	defer file.Close()
	logical := uploadLogicalPath(r.MultipartForm, header)
	if !safeObjectKey(logical) {
		writeJSONErr(w, http.StatusBadRequest, "invalid logical_path")
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "failed to read file")
		return
	}
	objectKey := stableUploadObjectKey(h.BackingFS.BaseDirectory(), datasetID.String(), logical)
	if err := local.WriteLocalObject(objectKey, data); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to write object")
		return
	}
	physical := storageabstraction.PhysicalLocation{FSID: h.BackingFS.FSID(), RelativePath: objectKey}
	now := time.Now().UTC()
	sum := sha256.Sum256(data)
	metadata, _ := models.MarshalJSONValue(map[string]any{"sha256": hex.EncodeToString(sum[:])})
	contentType := header.Header.Get("Content-Type")
	entryType := "file"
	size := int64(len(data))
	current, err := h.Repo.ListDatasetFileIndex(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load dataset file index")
		return
	}
	merged := make([]models.PutDatasetFileIndexEntry, 0, len(current)+1)
	seen := false
	for _, item := range current {
		entry := models.PutDatasetFileIndexEntry{Path: item.Path, StoragePath: item.StoragePath, EntryType: &item.EntryType, SizeBytes: &item.SizeBytes, ContentType: item.ContentType, Metadata: item.Metadata, LastModified: item.LastModified}
		if item.Path == logical {
			entry = models.PutDatasetFileIndexEntry{Path: logical, StoragePath: physical.URI(), EntryType: &entryType, SizeBytes: &size, ContentType: emptyStringPtr(contentType), Metadata: metadata, LastModified: &now}
			seen = true
		}
		merged = append(merged, entry)
	}
	if !seen {
		merged = append(merged, models.PutDatasetFileIndexEntry{Path: logical, StoragePath: physical.URI(), EntryType: &entryType, SizeBytes: &size, ContentType: emptyStringPtr(contentType), Metadata: metadata, LastModified: &now})
	}
	if err := h.Repo.ReplaceDatasetFileIndex(r.Context(), datasetID, merged); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to update dataset file index")
		return
	}
	items, err := h.Repo.ListDatasetFileIndex(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset file index")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"path": logical, "physical_uri": physical.URI(), "size_bytes": size, "files": items})
}

func (h *Handlers) UploadTransactionFileContent(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	txnID, ok := h.requireOpenTransaction(w, r, dataset.ID)
	if !ok {
		return
	}
	local, ok := h.BackingFS.(localObjectStore)
	if !ok || h.BackingFS == nil || h.BackingFS.FSID() != "local" {
		writeDependencyUnavailable(w, "local_backing_filesystem_unavailable", "local backing filesystem not configured")
		return
	}
	logical, data, mediaType, rowHint, ok := uploadContentPayload(w, r)
	if !ok {
		return
	}
	physical := storageabstraction.PhysicalLocation{
		FSID:          h.BackingFS.FSID(),
		BaseDirectory: h.BackingFS.BaseDirectory(),
		RelativePath:  "transactions/" + txnID.String() + "/" + logical,
	}
	objectKey := physicalObjectKey(physical)
	if err := local.WriteLocalObject(objectKey, data); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to write object")
		return
	}
	sum := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sum[:])
	if rowHint == nil {
		rowHint = inferRowCountHint(logical, mediaType, data)
	}
	storageLocation := storageLocationJSON(physical, logical)
	if err := h.Repo.StageTransactionFiles(r.Context(), dataset.ID, txnID, []models.StageTransactionFile{{
		LogicalPath:     logical,
		PhysicalPath:    objectKey,
		PhysicalURI:     physical.URI(),
		SizeBytes:       int64(len(data)),
		MediaType:       &mediaType,
		SHA256:          &shaHex,
		RowCountHint:    rowHint,
		StorageLocation: storageLocation,
		Operation:       uploadFileOperation(r),
	}}); err != nil {
		writeTransactionError(w, err)
		return
	}
	now := time.Now().UTC()
	h.emitAudit(r.Context(), AuditEvent{Actor: actorFromRequest(r), Action: "files.upload", DatasetRID: dataset.ID.String(), Details: map[string]any{
		"transaction_id": txnID.String(), "transaction_rid": transactionRID(txnID), "logical_path": logical, "size_bytes": len(data), "media_type": mediaType, "sha256": shaHex, "physical_uri": physical.URI(),
	}})
	writeJSON(w, http.StatusCreated, models.UploadDatasetFileContentResponse{
		Path:            logical,
		LogicalPath:     logical,
		TransactionID:   txnID,
		TransactionRID:  transactionRID(txnID),
		PhysicalURI:     physical.URI(),
		SizeBytes:       int64(len(data)),
		MediaType:       mediaType,
		SHA256:          shaHex,
		RowCountHint:    rowHint,
		StorageLocation: storageLocation,
		UpdatedTime:     now,
	})
}

func (h *Handlers) DeleteTransactionFile(w http.ResponseWriter, r *http.Request) {
	_, dataset, ok := h.ownedDataset(w, r)
	if !ok {
		return
	}
	txnID, ok := h.requireOpenTransaction(w, r, dataset.ID)
	if !ok {
		return
	}
	logical, ok := deleteLogicalPath(w, r)
	if !ok {
		return
	}
	if err := h.Repo.StageTransactionFiles(r.Context(), dataset.ID, txnID, []models.StageTransactionFile{{
		LogicalPath: logical,
		Operation:   models.FileOperationRemove,
	}}); err != nil {
		writeTransactionError(w, err)
		return
	}
	now := time.Now().UTC()
	h.emitAudit(r.Context(), AuditEvent{Actor: actorFromRequest(r), Action: "files.delete", DatasetRID: dataset.ID.String(), Details: map[string]any{
		"transaction_id": txnID.String(), "transaction_rid": transactionRID(txnID), "logical_path": logical,
	}})
	writeJSON(w, http.StatusOK, models.DeleteDatasetFileContentResponse{
		Path:           logical,
		LogicalPath:    logical,
		TransactionID:  txnID,
		TransactionRID: transactionRID(txnID),
		Operation:      string(models.FileOperationRemove),
		UpdatedTime:    now,
	})
}

func (h *Handlers) requireOpenTransaction(w http.ResponseWriter, r *http.Request, datasetID uuid.UUID) (uuid.UUID, bool) {
	txnID, err := uuid.Parse(transactionIDParam(r))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid transaction id")
		return uuid.Nil, false
	}
	status, found, err := h.Repo.GetTransactionStatus(r.Context(), datasetID, txnID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load transaction")
		return uuid.Nil, false
	}
	if !found {
		writeJSONErr(w, http.StatusNotFound, "transaction not found")
		return uuid.Nil, false
	}
	if !strings.EqualFold(status, "OPEN") {
		writeTransactionNotOpen(w)
		return uuid.Nil, false
	}
	return txnID, true
}

func uploadContentPayload(w http.ResponseWriter, r *http.Request) (string, []byte, string, *int64, bool) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid multipart body")
			return "", nil, "", nil, false
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "file part is required")
			return "", nil, "", nil, false
		}
		defer file.Close()
		logical := uploadLogicalPath(r.MultipartForm, header)
		if !safeObjectKey(logical) {
			writeJSONErr(w, http.StatusBadRequest, "invalid logical_path")
			return "", nil, "", nil, false
		}
		data, err := io.ReadAll(file)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "failed to read file")
			return "", nil, "", nil, false
		}
		mediaType := header.Header.Get("Content-Type")
		if mediaType == "" {
			mediaType = http.DetectContentType(data)
		}
		return logical, data, mediaType, rowCountHintFromValues(r.MultipartForm.Value), true
	}
	logical := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path")), "/")
	if logical == "" {
		logical = strings.Trim(strings.TrimSpace(r.URL.Query().Get("logical_path")), "/")
	}
	if logical == "" {
		writeJSONErr(w, http.StatusBadRequest, "path required")
		return "", nil, "", nil, false
	}
	if !safeObjectKey(logical) {
		writeJSONErr(w, http.StatusBadRequest, "invalid logical_path")
		return "", nil, "", nil, false
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "failed to read body")
		return "", nil, "", nil, false
	}
	mediaType := contentType
	if mediaType == "" || strings.EqualFold(mediaType, "application/octet-stream") {
		mediaType = http.DetectContentType(data)
	}
	return logical, data, mediaType, int64Query(r, "row_count_hint"), true
}

func deleteLogicalPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	logical := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path")), "/")
	if logical == "" {
		logical = strings.Trim(strings.TrimSpace(r.URL.Query().Get("logical_path")), "/")
	}
	if logical == "" && r.Body != nil {
		var body struct {
			Path        string `json:"path"`
			LogicalPath string `json:"logical_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			logical = strings.Trim(strings.TrimSpace(firstText(body.Path, body.LogicalPath)), "/")
		}
	}
	if logical == "" {
		writeJSONErr(w, http.StatusBadRequest, "path required")
		return "", false
	}
	if !safeObjectKey(logical) {
		writeJSONErr(w, http.StatusBadRequest, "invalid logical_path")
		return "", false
	}
	return logical, true
}

func localFSKey(r *http.Request) string {
	if key := chi.URLParam(r, "*"); key != "" {
		return strings.Trim(key, "/")
	}
	if key := chi.URLParam(r, "key"); key != "" {
		return strings.Trim(key, "/")
	}
	return strings.TrimPrefix(r.URL.Path, "/v1/_internal/local-fs/")
}

func parseLocalExpires(raw string) (time.Time, error) {
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Unix(n, 0).UTC(), nil
	}
	return time.Parse(time.RFC3339, raw)
}

func safeObjectKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return false
	}
	clean := path.Clean("/" + key)[1:]
	if clean != strings.Trim(key, "/") || clean == "." {
		return false
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." || part == "" {
			return false
		}
	}
	return true
}

func backingDriver(fsID string) string {
	switch {
	case strings.HasPrefix(fsID, "s3:"):
		return "s3"
	case strings.HasPrefix(fsID, "hdfs:"):
		return "hdfs"
	default:
		return "local"
	}
}

func uploadLogicalPath(form *multipart.Form, header *multipart.FileHeader) string {
	for _, name := range []string{"logical_path", "path"} {
		if vals := form.Value[name]; len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			return strings.Trim(strings.TrimSpace(vals[0]), "/")
		}
	}
	return strings.Trim(strings.TrimSpace(header.Filename), "/")
}

func stableUploadObjectKey(baseDir string, datasetID string, logical string) string {
	return strings.Trim(path.Join(baseDir, "datasets", datasetID, logical), "/")
}

func emptyStringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func transactionRID(id uuid.UUID) string {
	return "ri.foundry.main.transaction." + id.String()
}

func actorFromRequest(r *http.Request) string {
	claims, _ := authmw.FromContext(r.Context())
	if claims == nil {
		return "anonymous"
	}
	return claims.Sub.String()
}

func physicalObjectKey(location storageabstraction.PhysicalLocation) string {
	return strings.Trim(strings.TrimPrefix(location.URI(), "local:///"), "/")
}

func storageLocationJSON(location storageabstraction.PhysicalLocation, logicalPath string) models.JSONValue {
	out, err := models.MarshalJSONValue(map[string]any{
		"uri":            location.URI(),
		"fs_id":          location.FSID,
		"base_directory": location.BaseDirectory,
		"relative_path":  location.RelativePath,
		"logical_path":   logicalPath,
	})
	if err != nil {
		return []byte(`{}`)
	}
	return out
}

func uploadFileOperation(r *http.Request) models.FileOperation {
	raw := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("operation")))
	switch models.FileOperation(raw) {
	case models.FileOperationReplace:
		return models.FileOperationReplace
	default:
		return models.FileOperationAdd
	}
}

func rowCountHintFromValues(values map[string][]string) *int64 {
	for _, key := range []string{"row_count_hint", "row_count"} {
		if raw := values[key]; len(raw) > 0 {
			if n, err := strconv.ParseInt(strings.TrimSpace(raw[0]), 10, 64); err == nil && n >= 0 {
				return &n
			}
		}
	}
	return nil
}

func int64Query(r *http.Request, key string) *int64 {
	if raw := strings.TrimSpace(r.URL.Query().Get(key)); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n >= 0 {
			return &n
		}
	}
	return nil
}

func inferRowCountHint(logicalPath string, mediaType string, data []byte) *int64 {
	lowerPath := strings.ToLower(logicalPath)
	lowerType := strings.ToLower(mediaType)
	if !(strings.HasSuffix(lowerPath, ".csv") ||
		strings.HasSuffix(lowerPath, ".tsv") ||
		strings.HasSuffix(lowerPath, ".jsonl") ||
		strings.HasSuffix(lowerPath, ".ndjson") ||
		strings.Contains(lowerType, "csv") ||
		strings.Contains(lowerType, "json") ||
		strings.HasPrefix(lowerType, "text/")) {
		return nil
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		zero := int64(0)
		return &zero
	}
	count := int64(strings.Count(trimmed, "\n") + 1)
	return &count
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...*string) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		if trimmed := strings.TrimSpace(*value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
