// Package middleware groups the small wrappers the gateway chains
// around the proxy handler.
//
// Order (mirrors Rust): request_id → cors → ratelimit → audit → proxy.
package middleware

import (
	"net/http"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
)

// HeaderXRequestID is the canonical request-id header.
const HeaderXRequestID = "X-Request-Id"

// RequestID injects an X-Request-Id (UUID v7) when missing and copies
// it onto the response, so downstream services + frontend logs share
// the same correlation id.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderXRequestID)
		if id == "" {
			id = ids.New().String()
			r.Header.Set(HeaderXRequestID, id)
		}
		w.Header().Set(HeaderXRequestID, id)
		next.ServeHTTP(w, r)
	})
}
