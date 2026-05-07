package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

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
		writeJSONErr(w, http.StatusServiceUnavailable, "local backing filesystem not configured")
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
		writeJSONErr(w, http.StatusServiceUnavailable, "backing filesystem not configured")
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
		writeJSONErr(w, http.StatusServiceUnavailable, "local backing filesystem not configured")
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
