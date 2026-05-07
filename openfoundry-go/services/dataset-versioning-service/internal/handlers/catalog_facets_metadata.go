package handlers

import (
	"errors"
	"net/http"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func (h *Handlers) GetCatalogFacets(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.Repo == nil {
		writeJSONErr(w, http.StatusInternalServerError, "repository unavailable")
		return
	}
	facets, err := h.Repo.GetCatalogFacets(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list catalog facets")
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func (h *Handlers) GetDatasetMetadata(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.Repo == nil {
		writeJSONErr(w, http.StatusInternalServerError, "repository unavailable")
		return
	}
	datasetID, err := h.Repo.ResolveDatasetID(r.Context(), datasetIDParam(r))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to resolve dataset")
		return
	}
	metadata, err := h.Repo.GetInternalDatasetMetadata(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load dataset metadata")
		return
	}
	if metadata == nil {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return
	}
	writeJSON(w, http.StatusOK, metadata)
}
