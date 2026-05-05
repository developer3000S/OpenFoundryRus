package proxy

import "strings"

// RewriteUpstreamPath maps the gateway-facing path to the upstream's
// internal path. Mirrors the Rust `rewrite_upstream_path` rules
// verbatim:
//
//   - /api/v1/datasets/catalog/facets → /v1/catalog/facets
//   - /api/v1/datasets/<x>/filesystem  → /v1/datasets/<x>/files
//   - /api/v1/datasets/...             → /v1/datasets/...
//   - everything else                  → unchanged
func RewriteUpstreamPath(path string) string {
	if path == "/api/v1/datasets/catalog/facets" {
		return "/v1/catalog/facets"
	}
	if rest, ok := strings.CutPrefix(path, "/api/v1/datasets"); ok {
		canonical := "/v1/datasets" + rest
		if prefix, ok := strings.CutSuffix(canonical, "/filesystem"); ok {
			return prefix + "/files"
		}
		return canonical
	}
	return path
}

// RewriteUpstreamPathAndQuery is the convenience that joins the
// rewritten path with the original query string.
func RewriteUpstreamPathAndQuery(path, rawQuery string) string {
	rewritten := RewriteUpstreamPath(path)
	if rawQuery == "" {
		return rewritten
	}
	return rewritten + "?" + rawQuery
}
