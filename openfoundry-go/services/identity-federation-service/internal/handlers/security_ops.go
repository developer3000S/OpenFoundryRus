package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/jwksrotation"
)

// SecurityOps wires the JWKS rotation + content-signing surface
// from services/identity-federation-service/src/handlers/
// security_ops.rs. The handlers themselves are framework-agnostic
// (plain http.HandlerFunc) — the Cedar AdminGuard middleware is
// mounted by the router (see internal/server) using the
// cedarauthz package.
type SecurityOps struct {
	// JWKS is the live rotation orchestrator. Nil means JWKS
	// rotation is not configured for this deployment; the
	// PublishJwks/RotateJwks/RollbackJwks endpoints respond with
	// 503 in that case (mirrors Rust json_error
	// SERVICE_UNAVAILABLE).
	JWKS *jwksrotation.Service
}

// PublishJwks handles `GET /.well-known/jwks.json`. Public
// endpoint — no claims check, no Cedar guard.
func (h *SecurityOps) PublishJwks(w http.ResponseWriter, r *http.Request) {
	if h.JWKS == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "jwks rotation is not configured")
		return
	}
	jwks, err := h.JWKS.PublishedJwks(r.Context(), time.Now().UTC())
	if err != nil {
		jwksErrorResponse(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jwks)
}

// RotateJwks handles `POST /_admin/jwks/rotate`. Mounted behind
// cedarauthz.AdminGuard(ActionRotateJwks, JwksKeyResource) +
// requireJwksRotation belt-and-braces claim check (mirrors the
// Rust dual-gate of Cedar + has_role/has_permission).
func (h *SecurityOps) RotateJwks(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "missing claims")
		return
	}
	if !requireJwksRotation(claims) {
		writeJSONErr(w, http.StatusForbidden, "missing permission jwks:rotate")
		return
	}
	if h.JWKS == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "jwks rotation is not configured")
		return
	}
	out, err := h.JWKS.Rotate(r.Context(), time.Now().UTC())
	if err != nil {
		jwksErrorResponse(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RotateJwksResponse{Rotation: out})
}

// RollbackJwksRequest is the body for POST /_admin/jwks/rollback.
type RollbackJwksRequest struct {
	TargetKid *string `json:"target_kid,omitempty"`
}

// RotateJwksResponse + RollbackJwksResponse mirror the Rust wire
// shapes verbatim.
type RotateJwksResponse struct {
	Rotation jwksrotation.RotationOutcome `json:"rotation"`
}

type RollbackJwksResponse struct {
	Rollback jwksrotation.RollbackOutcome `json:"rollback"`
}

// RollbackJwks handles `POST /_admin/jwks/rollback`. Mounted
// behind the same Cedar guard as RotateJwks.
func (h *SecurityOps) RollbackJwks(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "missing claims")
		return
	}
	if !requireJwksRotation(claims) {
		writeJSONErr(w, http.StatusForbidden, "missing permission jwks:rotate")
		return
	}
	if h.JWKS == nil {
		writeJSONErr(w, http.StatusServiceUnavailable, "jwks rotation is not configured")
		return
	}
	var body RollbackJwksRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid body")
			return
		}
	}
	out, err := h.JWKS.Rollback(r.Context(), body.TargetKid, time.Now().UTC())
	if err != nil {
		jwksErrorResponse(w, err)
		return
	}
	writeJSON(w, http.StatusOK, RollbackJwksResponse{Rollback: out})
}

// ─── Content-signing helpers ─────────────────────────────────────────

// HashContentRequest / HashContentResponse mirror the Rust shapes.
type HashContentRequest struct {
	Content string  `json:"content"`
	Salt    *string `json:"salt,omitempty"`
}

type HashContentResponse struct {
	Algorithm string `json:"algorithm"`
	Digest    string `json:"digest"`
}

// SignContentRequest / SignContentResponse.
type SignContentRequest struct {
	Content     string `json:"content"`
	KeyMaterial string `json:"key_material"`
}

type SignContentResponse struct {
	Algorithm string `json:"algorithm"`
	Signature string `json:"signature"`
}

// VerifySignatureRequest / VerifySignatureResponse.
type VerifySignatureRequest struct {
	Content     string `json:"content"`
	KeyMaterial string `json:"key_material"`
	Signature   string `json:"signature"`
}

type VerifySignatureResponse struct {
	Algorithm string `json:"algorithm"`
	Valid     bool   `json:"valid"`
}

// HashContent handles `POST /_admin/security/hash`. Mirrors fn
// hash_content. Requires control_panel:write or admin role.
func (h *SecurityOps) HashContent(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "missing claims")
		return
	}
	if !requireSecurityWrite(claims) {
		writeJSONErr(w, http.StatusForbidden, "missing permission control_panel:write")
		return
	}
	var body HashContentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeJSONErr(w, http.StatusBadRequest, "content is required")
		return
	}
	writeJSON(w, http.StatusOK, HashContentResponse{
		Algorithm: "sha256",
		Digest:    HashContent(body.Content, body.Salt),
	})
}

// SignContent handles `POST /_admin/security/sign`.
func (h *SecurityOps) SignContent(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "missing claims")
		return
	}
	if !requireSecurityWrite(claims) {
		writeJSONErr(w, http.StatusForbidden, "missing permission control_panel:write")
		return
	}
	var body SignContentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeJSONErr(w, http.StatusBadRequest, "content is required")
		return
	}
	if strings.TrimSpace(body.KeyMaterial) == "" {
		writeJSONErr(w, http.StatusBadRequest, "key_material is required")
		return
	}
	writeJSON(w, http.StatusOK, SignContentResponse{
		Algorithm: "hmac-sha256",
		Signature: SignContent(body.Content, body.KeyMaterial),
	})
}

// VerifySignature handles `POST /_admin/security/verify`.
func (h *SecurityOps) VerifySignature(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "missing claims")
		return
	}
	if !requireSecurityWrite(claims) {
		writeJSONErr(w, http.StatusForbidden, "missing permission control_panel:write")
		return
	}
	var body VerifySignatureRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(body.Content) == "" {
		writeJSONErr(w, http.StatusBadRequest, "content is required")
		return
	}
	if strings.TrimSpace(body.KeyMaterial) == "" {
		writeJSONErr(w, http.StatusBadRequest, "key_material is required")
		return
	}
	if strings.TrimSpace(body.Signature) == "" {
		writeJSONErr(w, http.StatusBadRequest, "signature is required")
		return
	}
	writeJSON(w, http.StatusOK, VerifySignatureResponse{
		Algorithm: "hmac-sha256",
		Valid:     VerifySignature(body.Content, body.KeyMaterial, body.Signature),
	})
}

// ─── Pure helpers (mirrors src/domain/security.rs) ───────────────────

// HashContent computes the SHA-256 of `content`, optionally
// prefixed with `salt`, URL-safe-base64-encoded without padding.
// Mirrors fn hash_content.
func HashContent(content string, salt *string) string {
	h := sha256.New()
	if salt != nil {
		_, _ = h.Write([]byte(*salt))
	}
	_, _ = h.Write([]byte(content))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// SignContent computes an HMAC-SHA-256 over `content` with
// `keyMaterial` as the secret. Mirrors fn sign_content.
func SignContent(content, keyMaterial string) string {
	mac := hmac.New(sha256.New, []byte(keyMaterial))
	_, _ = mac.Write([]byte(content))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifySignature is a constant-time comparison of an
// HMAC-SHA-256 signature. Mirrors fn verify_signature.
func VerifySignature(content, keyMaterial, signature string) bool {
	want, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(keyMaterial))
	_, _ = mac.Write([]byte(content))
	return hmac.Equal(want, mac.Sum(nil))
}

// ─── Claim-side gates ────────────────────────────────────────────────

// requireJwksRotation mirrors fn require_jwks_rotation —
// admin OR jwks:rotate OR control_panel:write.
func requireJwksRotation(claims *authmw.Claims) bool {
	return claims.HasRole("admin") ||
		claims.HasPermission("jwks", "rotate") ||
		claims.HasPermission("control_panel", "write")
}

// requireSecurityWrite mirrors fn require_security_write —
// admin OR control_panel:write.
func requireSecurityWrite(claims *authmw.Claims) bool {
	return claims.HasRole("admin") || claims.HasPermission("control_panel", "write")
}

// ─── JwksRotationError → HTTP envelope ───────────────────────────────

// jwksErrorResponse maps the typed JwksRotationError tree onto the
// expected HTTP status codes. Mirrors fn jwks_error.
func jwksErrorResponse(w http.ResponseWriter, err error) {
	var je *jwksrotation.JwksRotationError
	if !errors.As(err, &je) {
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	switch je.Kind {
	case jwksrotation.ErrJwksState:
		writeJSONErr(w, http.StatusConflict, je.Message)
	case jwksrotation.ErrJwksStore:
		writeJSONErr(w, http.StatusInternalServerError, "jwks store error")
	case jwksrotation.ErrJwksVault:
		writeJSONErr(w, http.StatusBadGateway, "vault transit error")
	default:
		writeJSONErr(w, http.StatusInternalServerError, "internal error")
	}
}
