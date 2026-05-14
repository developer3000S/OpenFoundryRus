package handlers

import (
	"net/http"
	"strings"
)

func (h *Handlers) GetIncrementalReadiness(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	out, err := h.Repo.GetDatasetIncrementalReadiness(r.Context(), datasetID, branch)
	if err != nil {
		writeTransactionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
