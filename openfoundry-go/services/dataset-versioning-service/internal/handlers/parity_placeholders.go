package handlers

import "net/http"

func (h *Handlers) notImplemented(w http.ResponseWriter, _ *http.Request, feature string) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error":   "not implemented",
		"feature": feature,
	})
}

func (h *Handlers) GetCatalogFacets(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port catalog facet aggregation from Rust data_asset_catalog.
	h.notImplemented(w, r, "catalog facets")
}

func (h *Handlers) GetDatasetMetadata(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port internal dataset metadata lookup.
	h.notImplemented(w, r, "dataset metadata")
}

func (h *Handlers) CompareViews(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port dataset view comparison.
	h.notImplemented(w, r, "compare views")
}
