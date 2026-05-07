package handlers

import "net/http"

func (h *Handlers) notImplemented(w http.ResponseWriter, _ *http.Request, feature string) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error":   "not implemented",
		"feature": feature,
	})
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
