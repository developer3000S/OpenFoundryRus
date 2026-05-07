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

func (h *Handlers) StartTransaction(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port transaction start semantics.
	h.notImplemented(w, r, "start transaction")
}

func (h *Handlers) GetTransaction(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port transaction lookup semantics.
	h.notImplemented(w, r, "get transaction")
}

func (h *Handlers) TransactionAction(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port transaction commit/abort action dispatch.
	h.notImplemented(w, r, "transaction action")
}

func (h *Handlers) CommitTransaction(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port transaction commit suffix route.
	h.notImplemented(w, r, "commit transaction")
}

func (h *Handlers) AbortTransaction(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port transaction abort suffix route.
	h.notImplemented(w, r, "abort transaction")
}

func (h *Handlers) ListTransactions(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port dataset transaction listing and filters.
	h.notImplemented(w, r, "list transactions")
}

func (h *Handlers) BatchGetTransactions(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port 207 Multi-Status batch transaction lookup.
	h.notImplemented(w, r, "batch get transactions")
}

func (h *Handlers) GetDatasetQuality(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port dataset quality lookup.
	h.notImplemented(w, r, "get dataset quality")
}

func (h *Handlers) RefreshDatasetQuality(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port dataset quality profiling refresh.
	h.notImplemented(w, r, "refresh dataset quality")
}

func (h *Handlers) CreateQualityRule(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port quality rule creation.
	h.notImplemented(w, r, "create quality rule")
}

func (h *Handlers) UpdateQualityRule(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port quality rule update.
	h.notImplemented(w, r, "update quality rule")
}

func (h *Handlers) DeleteQualityRule(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port quality rule deletion.
	h.notImplemented(w, r, "delete quality rule")
}

func (h *Handlers) GetDatasetLint(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port dataset lint response.
	h.notImplemented(w, r, "get dataset lint")
}

func (h *Handlers) GetDatasetHealth(w http.ResponseWriter, r *http.Request) {
	// TODO(dataset-versioning parity): port dataset health snapshot.
	h.notImplemented(w, r, "get dataset health")
}
