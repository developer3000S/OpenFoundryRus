// Package handler hosts the HTTP handlers for the template service.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
)

// Health handles GET /healthz. The payload is byte-identical to the
// Rust services so platform tooling (k8s probes, dashboards) does not
// need a per-language template.
func Health(serviceName, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(serviceName, version))
	}
}
