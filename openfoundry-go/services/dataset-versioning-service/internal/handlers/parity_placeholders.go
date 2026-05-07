package handlers

import "net/http"

func (h *Handlers) notImplemented(w http.ResponseWriter, _ *http.Request, feature string) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error":   "not implemented",
		"feature": feature,
	})
}
