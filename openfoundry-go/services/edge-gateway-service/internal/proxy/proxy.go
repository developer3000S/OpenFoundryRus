// Package proxy is the gateway's reverse-proxy handler.
//
// Request flow (matches Rust `proxy::service_router::proxy_handler`):
//
//  1. Extract bearer token (optional) and decode claims.
//  2. Enforce zero-trust scope (HTTP method + path prefix) — 403 on miss.
//  3. Resolve TenantContext from claims (or anonymous fallback).
//  4. Pick upstream URL via SelectUpstream — 404 with code `unknown_service_route` if no rule matches.
//  5. Rewrite path (`/api/v1/datasets/...` → `/v1/datasets/...`).
//  6. Read body up to clamp(tenant.MaxRequestBodyBytes, 10MiB).
//  7. Build outbound request, strip Host, inject tenant + auth context headers.
//  8. Forward via http.Client.
//  9. Stream response status + headers + body back to client.
//
// Error codes / status codes match the Rust gateway 1:1 — see
// `internal/errs/errs.go` for the wire shape.
package proxy

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/errs"
)

// Handler holds the dependencies the proxy needs.
type Handler struct {
	Cfg    *config.Config
	JWT    *authmw.JWTConfig
	Client *http.Client
}

// NewHandler builds a Handler with a sensible default http.Client
// (30s overall timeout, idle connection pooling enabled).
func NewHandler(cfg *config.Config, jwt *authmw.JWTConfig) *Handler {
	return &Handler{
		Cfg: cfg,
		JWT: jwt,
		Client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        128,
				MaxIdleConnsPerHost: 16,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ServeHTTP implements net/http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	claims := h.tryDecodeClaims(r)

	// Zero-trust scope enforcement (only when a session_scope is present).
	if claims != nil {
		if !claims.AllowsHTTPMethod(r.Method) {
			errs.Write(w, http.StatusForbidden,
				errs.CodeScopedSessionMethodDenied,
				"session scope does not allow this HTTP method")
			return
		}
		if !claims.AllowsPath(r.URL.Path) {
			errs.Write(w, http.StatusForbidden,
				errs.CodeScopedSessionPathDenied,
				"session scope does not allow this path")
			return
		}
	}

	tenant := authmw.AnonymousTenant()
	if claims != nil {
		tenant = authmw.TenantContextFromClaims(claims)
	}

	upstreamBase := SelectUpstream(r.URL.Path, h.Cfg.Upstream)
	if upstreamBase == "" {
		errs.Write(w, http.StatusNotFound,
			errs.CodeUnknownServiceRoute, "unknown service route")
		return
	}

	// Build the outbound URL by joining base + rewritten path/query.
	target, err := buildUpstreamURL(upstreamBase, r.URL)
	if err != nil {
		errs.Write(w, http.StatusBadGateway,
			errs.CodeInvalidUpstreamURI, "invalid upstream URI")
		return
	}

	// Clamp body to tenant's per-request budget (with 10MiB fallback).
	bodyLimit := tenant.ClampRequestBodyBytes(10 * 1024 * 1024)
	bodyBytes, err := readLimited(r.Body, bodyLimit)
	if err != nil {
		errs.Write(w, http.StatusRequestEntityTooLarge,
			errs.CodeBodyTooLarge, "body too large")
		return
	}

	// Build the outbound request.
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		errs.Write(w, http.StatusBadGateway,
			errs.CodeInvalidUpstreamURI, "invalid upstream URI")
		return
	}
	copyHeadersExceptHost(r.Header, out.Header)
	ApplyTenantHeaders(out, &tenant)
	if claims != nil {
		ApplyAuthContextHeaders(out, claims)
	}

	resp, err := h.Client.Do(out)
	if err != nil {
		slog.Error("upstream request failed",
			slog.String("upstream", upstreamBase),
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()))
		errs.Write(w, http.StatusBadGateway,
			errs.CodeUpstreamUnavailable, "upstream unavailable")
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		// Headers + status already sent; nothing left to do but log.
		slog.Warn("upstream body copy interrupted",
			slog.String("error", err.Error()))
	}
}

// tryDecodeClaims pulls the bearer token, ignoring any decode failure
// (the gateway treats unauthenticated requests as anonymous; downstream
// services enforce auth where they require it).
func (h *Handler) tryDecodeClaims(r *http.Request) *authmw.Claims {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil
	}
	tok, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return nil
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return nil
	}
	claims, err := authmw.DecodeToken(h.JWT, tok)
	if err != nil {
		return nil
	}
	return claims
}

// buildUpstreamURL joins the upstream base with the rewritten path + original query.
func buildUpstreamURL(base string, in *url.URL) (*url.URL, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, errors.New("upstream base missing scheme or host")
	}
	u.Path = RewriteUpstreamPath(in.Path)
	u.RawQuery = in.RawQuery
	return u, nil
}

// readLimited reads up to `limit` bytes from r and returns the buffered
// content. If the body is longer, returns ErrBodyTooLarge.
func readLimited(r io.ReadCloser, limit uint64) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	defer r.Close()
	// Read one extra byte so we can detect overflow.
	buf, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if uint64(len(buf)) > limit {
		return nil, ErrBodyTooLarge
	}
	return buf, nil
}

// ErrBodyTooLarge is the sentinel readLimited returns on overflow.
var ErrBodyTooLarge = errors.New("body too large")

// copyHeadersExceptHost copies every header from src to dst except Host
// (which the outbound request already owns via the URL).
func copyHeadersExceptHost(src, dst http.Header) {
	for k, vs := range src {
		if strings.EqualFold(k, "Host") {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
