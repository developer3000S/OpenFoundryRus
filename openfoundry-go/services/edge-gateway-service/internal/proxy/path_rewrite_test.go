package proxy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/proxy"
)

func TestRewriteUpstreamPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"/api/v1/datasets", "/v1/datasets"},
		{"/api/v1/datasets/abc/preview", "/v1/datasets/abc/preview"},
		{"/api/v1/datasets/abc/schema", "/v1/datasets/abc/schema"},
		{"/api/v1/datasets/abc/filesystem", "/v1/datasets/abc/files"},
		{"/api/v1/datasets/abc/files", "/v1/datasets/abc/files"},
		{"/api/v1/datasets/catalog/facets", "/v1/catalog/facets"},
		{"/api/v1/pipelines", "/api/v1/pipelines"}, // not rewritten
		{"/healthz", "/healthz"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, proxy.RewriteUpstreamPath(c.in), "input=%s", c.in)
	}
}

func TestRewriteUpstreamPathAndQuery(t *testing.T) {
	t.Parallel()
	assert.Equal(t,
		"/v1/datasets/abc/files?path=%2Fa%2Fb",
		proxy.RewriteUpstreamPathAndQuery("/api/v1/datasets/abc/filesystem", "path=%2Fa%2Fb"),
	)
	assert.Equal(t,
		"/v1/datasets",
		proxy.RewriteUpstreamPathAndQuery("/api/v1/datasets", ""),
	)
}
