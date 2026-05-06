package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/jwksrotation"
)

// Helper: build a SecurityOps with an in-memory JWKS service that
// uses the FakeTransitKeyClient. No DB, no live Vault.
func newTestSecurityOps(t *testing.T) *handlers.SecurityOps {
	t.Helper()
	store := jwksrotation.NewInMemoryStore()
	transit := jwksrotation.NewFakeTransitKeyClient("openfoundry-jwt")
	svc := jwksrotation.NewService(store, transit, jwksrotation.ASVSL2Default)
	return &handlers.SecurityOps{JWKS: svc}
}

func newClaimsWith(t *testing.T, role string, permissions ...string) *authmw.Claims {
	t.Helper()
	c := &authmw.Claims{Sub: uuid.New(), Roles: []string{role}}
	if len(permissions) > 0 {
		// Permissions are encoded inside Attributes, but the
		// has_permission_key helper also supports the SessionScope
		// path. For tests we lean on `Roles` since admin grants
		// everything.
		_ = permissions
	}
	return c
}

func ctxWithClaims(t *testing.T, claims *authmw.Claims) context.Context {
	return authmw.ContextWithClaims(context.Background(), claims)
}

// --- PublishJwks --------------------------------------------------------

func TestPublishJwksReturnsActiveAndGrace(t *testing.T) {
	t.Parallel()
	h := newTestSecurityOps(t)
	// Seed by calling PublishedJwks once (it auto-seeds).
	_, err := h.JWKS.PublishedJwks(context.Background(), time.Now().UTC())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rec := httptest.NewRecorder()
	h.PublishJwks(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var jwks jwksrotation.Jwks
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&jwks))
	require.Len(t, jwks.Keys, 1)
	assert.Equal(t, "active", jwks.Keys[0].Status)
}

func TestPublishJwksReturns503WhenServiceMissing(t *testing.T) {
	t.Parallel()
	h := &handlers.SecurityOps{JWKS: nil}
	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rec := httptest.NewRecorder()
	h.PublishJwks(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), "jwks rotation is not configured")
}

// --- RotateJwks ---------------------------------------------------------

func TestRotateJwksAdminFlowSucceeds(t *testing.T) {
	t.Parallel()
	h := newTestSecurityOps(t)
	claims := newClaimsWith(t, "admin")

	req := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rotate", nil).
		WithContext(ctxWithClaims(t, claims))
	rec := httptest.NewRecorder()
	h.RotateJwks(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "got body=%s", rec.Body.String())

	var resp handlers.RotateJwksResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "openfoundry-jwt-v1", resp.Rotation.PreviousActiveKid)
	assert.Equal(t, "openfoundry-jwt-v2", resp.Rotation.ActiveKid)
}

func TestRotateJwksRejectsWithoutClaims(t *testing.T) {
	t.Parallel()
	h := newTestSecurityOps(t)
	req := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rotate", nil)
	rec := httptest.NewRecorder()
	h.RotateJwks(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRotateJwksRejectsNonAdminWithoutPermission(t *testing.T) {
	t.Parallel()
	h := newTestSecurityOps(t)
	claims := &authmw.Claims{Sub: uuid.New(), Roles: []string{"viewer"}}
	req := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rotate", nil).
		WithContext(ctxWithClaims(t, claims))
	rec := httptest.NewRecorder()
	h.RotateJwks(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing permission jwks:rotate")
}

func TestRotateJwksReturns503WhenServiceMissing(t *testing.T) {
	t.Parallel()
	h := &handlers.SecurityOps{JWKS: nil}
	claims := newClaimsWith(t, "admin")
	req := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rotate", nil).
		WithContext(ctxWithClaims(t, claims))
	rec := httptest.NewRecorder()
	h.RotateJwks(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// --- RollbackJwks -------------------------------------------------------

func TestRollbackJwksHappyPath(t *testing.T) {
	t.Parallel()
	h := newTestSecurityOps(t)
	claims := newClaimsWith(t, "admin")

	// Rotate first so there's a grace key to roll back to.
	rotateReq := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rotate", nil).
		WithContext(ctxWithClaims(t, claims))
	rec1 := httptest.NewRecorder()
	h.RotateJwks(rec1, rotateReq)
	require.Equal(t, http.StatusOK, rec1.Code)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rollback", body).
		WithContext(ctxWithClaims(t, claims))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.RollbackJwks(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "got body=%s", rec.Body.String())

	var resp handlers.RollbackJwksResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "openfoundry-jwt-v1", resp.Rollback.RestoredActiveKid)
	assert.Equal(t, "openfoundry-jwt-v2", resp.Rollback.DemotedKid)
}

func TestRollbackJwksConflictWhenNoGrace(t *testing.T) {
	t.Parallel()
	h := newTestSecurityOps(t)
	claims := newClaimsWith(t, "admin")

	// Force the JWKS service to seed first; no rotation happened
	// so there's no grace key.
	_, err := h.JWKS.PublishedJwks(context.Background(), time.Now().UTC())
	require.NoError(t, err)

	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rollback", body).
		WithContext(ctxWithClaims(t, claims))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.RollbackJwks(rec, req)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "no grace key")
}

func TestRollbackJwksRejectsBadJSON(t *testing.T) {
	t.Parallel()
	h := newTestSecurityOps(t)
	claims := newClaimsWith(t, "admin")
	req := httptest.NewRequest(http.MethodPost, "/_admin/jwks/rollback",
		strings.NewReader("not-json")).
		WithContext(ctxWithClaims(t, claims))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.RollbackJwks(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid body")
}

// --- Content signing ---------------------------------------------------

func TestHashContentRoundTrips(t *testing.T) {
	t.Parallel()
	h := &handlers.SecurityOps{}
	claims := newClaimsWith(t, "admin")
	body := strings.NewReader(`{"content":"hello world"}`)
	req := httptest.NewRequest(http.MethodPost, "/_admin/security/hash", body).
		WithContext(ctxWithClaims(t, claims))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HashContent(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "got body=%s", rec.Body.String())

	var resp handlers.HashContentResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "sha256", resp.Algorithm)
	assert.NotEmpty(t, resp.Digest)
	// Digest is determinstic; the pure helper produces the same
	// value as the handler.
	salt := (*string)(nil)
	assert.Equal(t, handlers.HashContent("hello world", salt), resp.Digest)
}

func TestHashContentRejectsEmpty(t *testing.T) {
	t.Parallel()
	h := &handlers.SecurityOps{}
	claims := newClaimsWith(t, "admin")
	body := strings.NewReader(`{"content":"   "}`)
	req := httptest.NewRequest(http.MethodPost, "/_admin/security/hash", body).
		WithContext(ctxWithClaims(t, claims))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HashContent(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "content is required")
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()
	h := &handlers.SecurityOps{}
	claims := newClaimsWith(t, "admin")

	signBody := strings.NewReader(`{"content":"alpha","key_material":"secret"}`)
	signReq := httptest.NewRequest(http.MethodPost, "/_admin/security/sign", signBody).
		WithContext(ctxWithClaims(t, claims))
	signReq.Header.Set("Content-Type", "application/json")
	signRec := httptest.NewRecorder()
	h.SignContent(signRec, signReq)
	require.Equal(t, http.StatusOK, signRec.Code)
	var signResp handlers.SignContentResponse
	require.NoError(t, json.NewDecoder(signRec.Body).Decode(&signResp))

	verifyBody := strings.NewReader(`{
        "content":"alpha",
        "key_material":"secret",
        "signature":"` + signResp.Signature + `"
    }`)
	verifyReq := httptest.NewRequest(http.MethodPost, "/_admin/security/verify", verifyBody).
		WithContext(ctxWithClaims(t, claims))
	verifyReq.Header.Set("Content-Type", "application/json")
	verifyRec := httptest.NewRecorder()
	h.VerifySignature(verifyRec, verifyReq)
	require.Equal(t, http.StatusOK, verifyRec.Code)
	var verifyResp handlers.VerifySignatureResponse
	require.NoError(t, json.NewDecoder(verifyRec.Body).Decode(&verifyResp))
	assert.True(t, verifyResp.Valid)

	// Wrong key material → invalid.
	mismatched := strings.NewReader(`{
        "content":"alpha",
        "key_material":"other",
        "signature":"` + signResp.Signature + `"
    }`)
	mreq := httptest.NewRequest(http.MethodPost, "/_admin/security/verify", mismatched).
		WithContext(ctxWithClaims(t, claims))
	mreq.Header.Set("Content-Type", "application/json")
	mrec := httptest.NewRecorder()
	h.VerifySignature(mrec, mreq)
	require.Equal(t, http.StatusOK, mrec.Code)
	var mresp handlers.VerifySignatureResponse
	require.NoError(t, json.NewDecoder(mrec.Body).Decode(&mresp))
	assert.False(t, mresp.Valid)
}

// --- Pure helpers (unit tests on hash/sign/verify) ----------------------

func TestHashContentDeterministic(t *testing.T) {
	t.Parallel()
	d1 := handlers.HashContent("payload", nil)
	d2 := handlers.HashContent("payload", nil)
	assert.Equal(t, d1, d2, "sha256 output must be deterministic")

	salt := "salty"
	dWithSalt := handlers.HashContent("payload", &salt)
	assert.NotEqual(t, d1, dWithSalt, "salt must change the digest")
}

func TestSignAndVerifyPureHelpers(t *testing.T) {
	t.Parallel()
	sig := handlers.SignContent("alpha", "secret")
	assert.True(t, handlers.VerifySignature("alpha", "secret", sig))
	assert.False(t, handlers.VerifySignature("alpha", "wrong-secret", sig))
	assert.False(t, handlers.VerifySignature("beta", "secret", sig))
	assert.False(t, handlers.VerifySignature("alpha", "secret", "not-base64!!"),
		"malformed signature must be invalid")
}
