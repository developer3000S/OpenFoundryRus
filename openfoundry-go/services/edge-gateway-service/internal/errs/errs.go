// Package errs renders the gateway error envelope.
//
// Wire-shape MUST stay byte-identical to the Rust gateway:
//
//	{ "error": { "code": "<code>", "message": "<msg>" } }
//
// Frontend code branches on `code`; do not rename existing codes.
package errs

import (
	"encoding/json"
	"net/http"
)

// Body is the public error payload.
type Body struct {
	Error Inner `json:"error"`
}

// Inner is the inner object — `code` is stable, `message` is human-readable.
type Inner struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Stable error codes used across the gateway. Adding new ones is fine;
// renaming or removing existing codes breaks the frontend.
const (
	CodeUnknownServiceRoute      = "unknown_service_route"
	CodeInvalidUpstreamURI       = "invalid_upstream_uri"
	CodeBodyTooLarge             = "body_too_large"
	CodeRateLimitExceeded        = "rate_limit_exceeded"
	CodeScopedSessionMethodDenied = "scoped_session_method_denied"
	CodeScopedSessionPathDenied   = "scoped_session_path_denied"
	CodeUpstreamUnavailable       = "upstream_unavailable"
	CodeProxyResponseBuildFailed  = "proxy_response_build_failed"
)

// Write serialises the envelope and emits status + JSON to w.
func Write(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Body{Error: Inner{Code: code, Message: msg}})
}
