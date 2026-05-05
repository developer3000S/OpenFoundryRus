package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/proxy"
)

func newGateway(t *testing.T, dvsURL, catalogURL string) (*httptest.Server, *authmw.JWTConfig) {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "edge-gateway-service"
	cfg.JWT.Secret = "test-secret-32bytes-test-secret-3"
	cfg.Upstream = config.UpstreamURLs{
		DatasetVersioning: dvsURL,
		DataAssetCatalog:  catalogURL,
		// rest left empty — every other route should yield 404 from the gateway
	}
	jwt := authmw.NewJWTConfig(cfg.JWT.Secret)
	h := proxy.NewHandler(cfg, jwt)
	return httptest.NewServer(h), jwt
}

// fakeUpstream records every inbound request and returns 200 with the
// echoed path so the test can assert routing + path rewriting at once.
func fakeUpstream(t *testing.T) (*httptest.Server, *http.Request) {
	t.Helper()
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clone := r.Clone(context.Background())
		clone.Body = io.NopCloser(strings.NewReader(""))
		captured = clone
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"upstream_path":"` + r.URL.Path + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

func TestProxyRoutesDatasetsToVersioning(t *testing.T) {
	t.Parallel()
	upstream, _ := fakeUpstream(t)
	gw, _ := newGateway(t, upstream.URL, upstream.URL)
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/api/v1/datasets/123/files")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", resp.Header.Get("X-Upstream"))

	var envelope struct{ UpstreamPath string `json:"upstream_path"` }
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, "/v1/datasets/123/files", envelope.UpstreamPath,
		"path rewriting must strip /api prefix")
}

func TestProxyFilesystemAlias(t *testing.T) {
	t.Parallel()
	upstream, _ := fakeUpstream(t)
	gw, _ := newGateway(t, upstream.URL, upstream.URL)
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/api/v1/datasets/abc/filesystem?path=/x")
	require.NoError(t, err)
	defer resp.Body.Close()

	var envelope struct{ UpstreamPath string `json:"upstream_path"` }
	body, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.Equal(t, "/v1/datasets/abc/files", envelope.UpstreamPath)
}

func TestProxyUnknownRouteIs404WithCanonicalErrorEnvelope(t *testing.T) {
	t.Parallel()
	upstream, _ := fakeUpstream(t)
	gw, _ := newGateway(t, upstream.URL, upstream.URL)
	defer gw.Close()

	resp, err := http.Get(gw.URL + "/api/v1/totally-unknown")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	var env struct {
		Error struct{ Code, Message string }
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, "unknown_service_route", env.Error.Code)
}

func TestProxyZeroTrustScopeBlocksDisallowedMethod(t *testing.T) {
	t.Parallel()
	upstream, _ := fakeUpstream(t)
	gw, jwt := newGateway(t, upstream.URL, upstream.URL)
	defer gw.Close()

	allowedPath := "/api/v1/datasets"
	allowedMethod := "GET"
	c := &authmw.Claims{
		Sub:   uuid.New(),
		IAT:   time.Now().Unix(),
		EXP:   time.Now().Add(time.Hour).Unix(),
		JTI:   uuid.New(),
		Email: "guest@example.com",
		Name:  "Guest",
		Roles: []string{"guest"},
		SessionScope: &authmw.SessionScope{
			AllowedMethods:      []string{allowedMethod},
			AllowedPathPrefixes: []string{allowedPath},
		},
	}
	tok, err := authmw.EncodeToken(jwt, c)
	require.NoError(t, err)

	// Allowed: GET /api/v1/datasets
	req, _ := http.NewRequest("GET", gw.URL+"/api/v1/datasets", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Disallowed: POST /api/v1/datasets
	req, _ = http.NewRequest("POST", gw.URL+"/api/v1/datasets", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	var env struct {
		Error struct{ Code, Message string }
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, "scoped_session_method_denied", env.Error.Code)

	// Disallowed: GET /api/v1/pipelines (path not in scope)
	req, _ = http.NewRequest("GET", gw.URL+"/api/v1/pipelines", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, json.Unmarshal(body, &env))
	assert.Equal(t, "scoped_session_path_denied", env.Error.Code)
}

func TestProxyInjectsTenantHeaders(t *testing.T) {
	t.Parallel()
	var capturedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	gw, jwt := newGateway(t, upstream.URL, upstream.URL)
	defer gw.Close()

	orgID := uuid.New()
	c := &authmw.Claims{
		Sub:        uuid.New(),
		IAT:        time.Now().Unix(),
		EXP:        time.Now().Add(time.Hour).Unix(),
		JTI:        uuid.New(),
		Email:      "user@example.com",
		Roles:      []string{"member"},
		OrgID:      &orgID,
		Attributes: json.RawMessage(`{"tenant_tier":"team"}`),
	}
	tok, err := authmw.EncodeToken(jwt, c)
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", gw.URL+"/api/v1/datasets/abc/files", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.NotNil(t, capturedHeaders)
	assert.Equal(t, orgID.String(), capturedHeaders.Get(proxy.HdrTenantScope))
	assert.Equal(t, "team", capturedHeaders.Get(proxy.HdrTenantTier))
	assert.Equal(t, "900", capturedHeaders.Get(proxy.HdrQuotaRequestsPerMin))
	assert.Equal(t, "user@example.com", capturedHeaders.Get(proxy.HdrAuthEmail))
	assert.Equal(t, orgID.String(), capturedHeaders.Get(proxy.HdrOrgID))
	assert.Equal(t, "standard", capturedHeaders.Get(proxy.HdrZeroTrust))
	// Host header is stripped — the outbound URL owns it.
	assert.Empty(t, capturedHeaders.Get("Host"))
}

func TestProxyBodyTooLargeReturns413(t *testing.T) {
	t.Parallel()
	upstream, _ := fakeUpstream(t)
	gw, _ := newGateway(t, upstream.URL, upstream.URL)
	defer gw.Close()

	// 10MiB + 1 byte — anonymous tenant has the standard 10MiB cap.
	overSize := 10*1024*1024 + 1
	body := strings.NewReader(strings.Repeat("x", overSize))
	resp, err := http.Post(gw.URL+"/api/v1/datasets/abc/files", "application/octet-stream", body)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	var env struct {
		Error struct{ Code, Message string }
	}
	require.NoError(t, json.Unmarshal(respBody, &env))
	assert.Equal(t, "body_too_large", env.Error.Code)
}

// noopRoundTripper exists so we don't accidentally talk to the real network in test setup helpers.
var _ = url.Parse
