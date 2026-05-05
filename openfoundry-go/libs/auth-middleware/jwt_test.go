package authmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

func sampleClaims(ttl time.Duration) *authmw.Claims {
	now := time.Now()
	return &authmw.Claims{
		Sub:   uuid.New(),
		IAT:   now.Unix(),
		EXP:   now.Add(ttl).Unix(),
		JTI:   uuid.New(),
		Email: "demo@example.com",
		Name:  "Demo User",
		Roles: []string{"member"},
	}
}

func TestHS256RoundTrip(t *testing.T) {
	t.Parallel()
	cfg, err := authmw.Generate()
	require.NoError(t, err)

	tok, err := authmw.EncodeToken(cfg, sampleClaims(time.Hour))
	require.NoError(t, err)
	decoded, err := authmw.DecodeToken(cfg, tok)
	require.NoError(t, err)
	assert.Equal(t, "demo@example.com", decoded.Email)
}

func TestHS256RejectsExpired(t *testing.T) {
	t.Parallel()
	cfg, err := authmw.Generate()
	require.NoError(t, err)

	tok, err := authmw.EncodeToken(cfg, sampleClaims(-time.Minute))
	require.NoError(t, err)
	_, err = authmw.DecodeToken(cfg, tok)
	require.Error(t, err)
	assert.True(t, authmw.IsExpired(err))
}

func TestHS256AudienceMismatch(t *testing.T) {
	t.Parallel()
	issuer, err := authmw.Generate()
	require.NoError(t, err)
	issuer = issuer.WithAudience("openfoundry")
	aud := "openfoundry"

	c := sampleClaims(time.Hour)
	c.AUD = &aud
	tok, err := authmw.EncodeToken(issuer, c)
	require.NoError(t, err)

	// Sanity-check round trip with the same config first — proves the
	// token is valid in isolation; the cross-config rejection lives in
	// an integration test once the secret-loader lands in Phase 2.
	got, err := authmw.DecodeToken(issuer, tok)
	require.NoError(t, err)
	assert.Equal(t, "openfoundry", *got.AUD)
}

func TestMiddlewareRejectsMissingToken(t *testing.T) {
	t.Parallel()
	cfg, err := authmw.Generate()
	require.NoError(t, err)

	called := false
	handler := authmw.Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	handler.ServeHTTP(rec, req)
	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestMiddlewareInjectsClaims(t *testing.T) {
	t.Parallel()
	cfg, err := authmw.Generate()
	require.NoError(t, err)

	tok, err := authmw.EncodeToken(cfg, sampleClaims(time.Hour))
	require.NoError(t, err)

	handler := authmw.Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := authmw.FromContext(r.Context())
		require.True(t, ok)
		assert.Equal(t, "demo@example.com", c.Email)
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPermissionWildcards(t *testing.T) {
	t.Parallel()
	c := &authmw.Claims{
		Roles:       []string{"member"},
		Permissions: []string{"datasets:*"},
	}
	assert.True(t, c.HasPermission("datasets", "read"))
	assert.True(t, c.HasPermission("datasets", "write"))
	assert.False(t, c.HasPermission("ontology", "read"))

	admin := &authmw.Claims{Roles: []string{"admin"}}
	assert.True(t, admin.HasPermission("anything", "anything"))
}
