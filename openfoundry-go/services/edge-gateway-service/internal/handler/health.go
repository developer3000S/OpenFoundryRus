// Package handler hosts the gateway's small set of direct endpoints
// (everything else is proxied — see `internal/proxy`).
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
)

// Health returns the canonical /healthz handler.
func Health(serviceName, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(serviceName, version))
	}
}
