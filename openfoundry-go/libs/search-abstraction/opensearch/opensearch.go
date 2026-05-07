// Package opensearch is the OpenSearch-backed SearchBackend
// implementation. Mirrors libs/search-abstraction/src/opensearch.rs.
//
// Wire format:
//
//   - Index per (tenant, type_id) named `of-{tenant}-{type}`
//     (sanitized, lowercase).
//   - PUT  /{index}/_doc/{id}?version=…&version_type=external —
//     stale writes are silently dropped (HTTP 409).
//   - POST /{index}/_search for both lexical (`query_string`) and
//     vector (`knn`) queries.
//   - POST /_bulk for BulkIndex.
//
// Importing this package as a side-effect registers the backend
// with searchabstraction.SearchBackendFromEnv:
//
//	import _ "github.com/openfoundry/openfoundry-go/libs/search-abstraction/opensearch"
//
// We deliberately do not depend on the official `opensearch-go`
// client. The trait surface we exercise is small (six methods) and
// the JSON wire format is stable; pulling in the official client
// would drag in `aws-sdk-go-v2/aws/signer` and a parallel HTTP
// stack we already have in `net/http`. Documented divergence from
// S0.8.c (Rust note carries over).
package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	searchabstraction "github.com/openfoundry/openfoundry-go/libs/search-abstraction"
	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func init() {
	searchabstraction.RegisterBackend(searchabstraction.BackendOpenSearch,
		func(endpoint string) searchabstraction.SearchBackend { return New(endpoint) })
}

// Backend is the OpenSearch HTTP client. Construct with the cluster
// HTTP endpoint (typically `http://opensearch:9200` in `compose dev`).
type Backend struct {
	endpoint   string
	http       *http.Client
	authHeader string
}

// Option customises a Backend.
type Option func(*Backend)

// WithHTTPClient configures the HTTP client used by the backend.
func WithHTTPClient(client *http.Client) Option {
	return func(b *Backend) {
		if client != nil {
			b.http = client
		}
	}
}

// WithAuthHeader configures the Authorization header sent to OpenSearch.
func WithAuthHeader(value string) Option {
	return func(b *Backend) { b.authHeader = strings.TrimSpace(value) }
}

// New builds a Backend with a default *http.Client (30 s timeout).
func New(endpoint string) *Backend {
	return NewWithOptions(endpoint)
}

// NewWithOptions builds a Backend with optional transport/auth configuration.
func NewWithOptions(endpoint string, opts ...Option) *Backend {
	b := WithClient(endpoint, &http.Client{Timeout: 30 * time.Second})
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b
}

// WithClient builds a Backend with a caller-provided *http.Client.
func WithClient(endpoint string, client *http.Client) *Backend {
	if client == nil {
		client = http.DefaultClient
	}
	return &Backend{endpoint: strings.TrimRight(endpoint, "/"), http: client}
}

// Compile-time interface satisfaction.
var _ searchabstraction.SearchBackend = (*Backend)(nil)

// ─── helpers ───────────────────────────────────────────────────────────

func (b *Backend) indexName(tenant repos.TenantId, typeID repos.TypeId) string {
	return fmt.Sprintf("of-%s-%s",
		sanitizeIndex(string(tenant)),
		searchabstraction.SanitizeDocType(string(typeID)))
}

func (b *Backend) docURL(index string, id repos.ObjectId) string {
	return fmt.Sprintf("%s/%s/_doc/%s", b.endpoint, index, percentEncode(string(id)))
}

func sanitizeIndex(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			out = append(out, byte(c))
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// percentEncode mirrors the Rust impl: preserve the unreserved set
// (RFC 3986: [A-Za-z0-9-_.~]) and percent-encode every other byte
// of the UTF-8 representation. Stays compatible with OpenSearch's
// path normaliser.
func percentEncode(s string) string {
	var w strings.Builder
	for _, c := range []byte(s) {
		switch {
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~':
			w.WriteByte(c)
		default:
			fmt.Fprintf(&w, "%%%02X", c)
		}
	}
	return w.String()
}

func mapStatus(status int, ctx string) error {
	if status >= 200 && status < 300 {
		return nil
	}
	return repos.Backend(fmt.Sprintf("opensearch %s HTTP %d: %s",
		ctx, status, http.StatusText(status)))
}

// augmentPayload is the shared serialiser for Index + BulkIndex.
// Mirrors the Rust pattern: if the payload is a JSON object, mutate
// it in place; otherwise wrap as `{"payload": original}`. Then
// inject id / tenant / type_id / version / embedding.
func augmentPayload(d searchabstraction.IndexDoc) ([]byte, error) {
	obj := map[string]any{}
	if raw := bytes.TrimSpace([]byte(d.Payload)); len(raw) > 0 {
		if raw[0] == '{' {
			if err := json.Unmarshal(raw, &obj); err != nil {
				return nil, err
			}
		} else {
			var inner any
			if err := json.Unmarshal(raw, &inner); err != nil {
				return nil, err
			}
			obj = map[string]any{"payload": inner}
		}
	}
	obj["id"] = string(d.ID)
	obj["tenant"] = string(d.Tenant)
	obj["type_id"] = string(d.TypeID)
	obj["version"] = d.Version
	if len(d.Embedding) > 0 {
		obj["embedding"] = d.Embedding
	}
	return json.Marshal(obj)
}

func (b *Backend) applyAuth(req *http.Request) {
	if b.authHeader != "" {
		req.Header.Set("Authorization", b.authHeader)
	}
}

func (b *Backend) sendJSON(ctx context.Context, method, u string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	b.applyAuth(req)
	return b.http.Do(req)
}

// ─── SearchBackend impl ────────────────────────────────────────────────

// Search runs a tenant-scoped lexical query. Targets the concrete
// index when type_id is set, otherwise the tenant-wide alias.
func (b *Backend) Search(
	ctx context.Context,
	query searchabstraction.SearchQuery,
	_ repos.ReadConsistency,
) (repos.PagedResult[searchabstraction.SearchHit], error) {
	target := fmt.Sprintf("of-%s-*", sanitizeIndex(string(query.Tenant)))
	if query.TypeID != nil {
		target = b.indexName(query.Tenant, *query.TypeID)
	}
	u := fmt.Sprintf("%s/%s/_search", b.endpoint, target)

	filter := []any{map[string]any{"term": map[string]string{"tenant": string(query.Tenant)}}}
	if query.TypeID != nil {
		filter = append(filter, map[string]any{"term": map[string]string{"type_id": string(*query.TypeID)}})
	}
	for k, v := range query.Filters {
		filter = append(filter, map[string]any{"term": map[string]string{k: v}})
	}

	must := []any{}
	if query.Q != nil && *query.Q != "" {
		must = append(must, map[string]any{"query_string": map[string]string{"query": *query.Q}})
	}
	if len(must) == 0 {
		must = append(must, map[string]any{"match_all": map[string]any{}})
	}

	size := uint32(1)
	if query.Page.Size > 1 {
		size = query.Page.Size
	}
	body := map[string]any{
		"size":  size,
		"query": map[string]any{"bool": map[string]any{"must": must, "filter": filter}},
	}

	empty := repos.PagedResult[searchabstraction.SearchHit]{}
	resp, err := b.sendJSON(ctx, http.MethodPost, u, body)
	if err != nil {
		return empty, repos.Backend("opensearch search send: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return repos.PagedResult[searchabstraction.SearchHit]{Items: []searchabstraction.SearchHit{}}, nil
	}
	if err := mapStatus(resp.StatusCode, "search"); err != nil {
		return empty, err
	}
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return empty, repos.Backend("opensearch search decode: " + err.Error())
	}
	return repos.PagedResult[searchabstraction.SearchHit]{Items: parseHits(v)}, nil
}

// Index PUTs a single document with `version_type=external`. HTTP
// 409 (stale external version) is silently swallowed.
func (b *Backend) Index(ctx context.Context, doc searchabstraction.IndexDoc) error {
	index := b.indexName(doc.Tenant, doc.TypeID)
	u := b.docURL(index, doc.ID)

	payload, err := augmentPayload(doc)
	if err != nil {
		return repos.Backend("opensearch index marshal: " + err.Error())
	}
	q := url.Values{}
	q.Set("version", fmt.Sprintf("%d", doc.Version))
	q.Set("version_type", "external")
	u = u + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(payload))
	if err != nil {
		return repos.Backend("opensearch index req: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	b.applyAuth(req)
	resp, err := b.http.Do(req)
	if err != nil {
		return repos.Backend("opensearch index send: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil
	}
	return mapStatus(resp.StatusCode, "index")
}

// Delete issues a tenant-wide _delete_by_query because the trait
// does not expose type_id at delete-time. Returns whether the
// document was present.
func (b *Backend) Delete(
	ctx context.Context,
	tenant repos.TenantId,
	id repos.ObjectId,
) (bool, error) {
	target := fmt.Sprintf("of-%s-*", sanitizeIndex(string(tenant)))
	u := fmt.Sprintf("%s/%s/_delete_by_query", b.endpoint, target)
	body := map[string]any{
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]string{"tenant": string(tenant)}},
					map[string]any{"term": map[string]string{"id": string(id)}},
				},
			},
		},
	}
	resp, err := b.sendJSON(ctx, http.MethodPost, u, body)
	if err != nil {
		return false, repos.Backend("opensearch delete send: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if err := mapStatus(resp.StatusCode, "delete"); err != nil {
		return false, err
	}
	var v struct {
		Deleted uint64 `json:"deleted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return false, repos.Backend("opensearch delete decode: " + err.Error())
	}
	return v.Deleted > 0, nil
}

// SearchVector runs an OpenSearch knn query. Filters are applied
// before the kNN search via the boolean `filter` clause.
func (b *Backend) SearchVector(
	ctx context.Context,
	query searchabstraction.VectorQuery,
	_ repos.ReadConsistency,
) ([]searchabstraction.SearchHit, error) {
	target := fmt.Sprintf("of-%s-*", sanitizeIndex(string(query.Tenant)))
	if query.TypeID != nil {
		target = b.indexName(query.Tenant, *query.TypeID)
	}
	u := fmt.Sprintf("%s/%s/_search", b.endpoint, target)

	filter := []any{map[string]any{"term": map[string]string{"tenant": string(query.Tenant)}}}
	if query.TypeID != nil {
		filter = append(filter, map[string]any{"term": map[string]string{"type_id": string(*query.TypeID)}})
	}
	for k, v := range query.Filters {
		filter = append(filter, map[string]any{"term": map[string]string{k: v}})
	}

	k := uint32(1)
	if query.K > 1 {
		k = query.K
	}
	body := map[string]any{
		"size": k,
		"query": map[string]any{
			"knn": map[string]any{
				"embedding": map[string]any{
					"vector": query.Embedding,
					"k":      k,
					"filter": map[string]any{"bool": map[string]any{"filter": filter}},
				},
			},
		},
	}
	resp, err := b.sendJSON(ctx, http.MethodPost, u, body)
	if err != nil {
		return nil, repos.Backend("opensearch vector send: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return []searchabstraction.SearchHit{}, nil
	}
	if err := mapStatus(resp.StatusCode, "search_vector"); err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, repos.Backend("opensearch vector decode: " + err.Error())
	}
	return parseHits(v), nil
}

// BulkIndex writes a single _bulk request with one
// `{"index": …}` action per document plus the augmented payload. A
// per-document HTTP status of 200..=299 or 409 (stale write) counts
// as success — anything else is collected into BulkOutcome.Failed.
func (b *Backend) BulkIndex(
	ctx context.Context,
	docs []searchabstraction.IndexDoc,
) (searchabstraction.BulkOutcome, error) {
	out := searchabstraction.BulkOutcome{Failed: []searchabstraction.BulkFail{}}
	if len(docs) == 0 {
		return out, nil
	}
	var body bytes.Buffer
	for _, d := range docs {
		header := map[string]any{
			"index": map[string]any{
				"_index":       b.indexName(d.Tenant, d.TypeID),
				"_id":          string(d.ID),
				"version":      d.Version,
				"version_type": "external",
			},
		}
		hb, err := json.Marshal(header)
		if err != nil {
			return out, repos.Backend("opensearch bulk header marshal: " + err.Error())
		}
		body.Write(hb)
		body.WriteByte('\n')
		pb, err := augmentPayload(d)
		if err != nil {
			return out, repos.Backend("opensearch bulk payload marshal: " + err.Error())
		}
		body.Write(pb)
		body.WriteByte('\n')
	}

	u := fmt.Sprintf("%s/_bulk", b.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &body)
	if err != nil {
		return out, repos.Backend("opensearch bulk req: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	b.applyAuth(req)
	resp, err := b.http.Do(req)
	if err != nil {
		return out, repos.Backend("opensearch bulk send: " + err.Error())
	}
	defer resp.Body.Close()
	if err := mapStatus(resp.StatusCode, "bulk_index"); err != nil {
		return out, err
	}
	var v struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return out, repos.Backend("opensearch bulk decode: " + err.Error())
	}
	for i, entry := range v.Items {
		raw, ok := entry["index"]
		if !ok {
			raw = entry["create"]
		}
		var id repos.ObjectId
		if i < len(docs) {
			id = docs[i].ID
		}
		if len(raw) == 0 {
			out.Failed = append(out.Failed, searchabstraction.BulkFail{ID: id, Reason: "missing status"})
			continue
		}
		var item struct {
			Status uint64 `json:"status"`
		}
		if err := json.Unmarshal(raw, &item); err != nil || item.Status == 0 {
			out.Failed = append(out.Failed, searchabstraction.BulkFail{ID: id, Reason: "missing status"})
			continue
		}
		switch {
		case item.Status >= 200 && item.Status < 300, item.Status == 409:
			out.Indexed++
		default:
			out.Failed = append(out.Failed, searchabstraction.BulkFail{
				ID: id, Reason: fmt.Sprintf("status %d", item.Status),
			})
		}
	}
	return out, nil
}

// parseHits decodes the OpenSearch `hits.hits[]` array. Mirrors
// fn parse_hits in the Rust impl.
func parseHits(v map[string]any) []searchabstraction.SearchHit {
	hitsRaw, ok := v["hits"].(map[string]any)
	if !ok {
		return []searchabstraction.SearchHit{}
	}
	arr, ok := hitsRaw["hits"].([]any)
	if !ok {
		return []searchabstraction.SearchHit{}
	}
	out := make([]searchabstraction.SearchHit, 0, len(arr))
	for _, raw := range arr {
		h, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		src, ok := h["_source"].(map[string]any)
		if !ok {
			continue
		}
		id, _ := src["id"].(string)
		if id == "" {
			id, _ = h["_id"].(string)
		}
		if id == "" {
			continue
		}
		typeID, _ := src["type_id"].(string)
		score := float32(0)
		if f, ok := h["_score"].(float64); ok {
			score = float32(f)
		}
		snippet, err := json.Marshal(src)
		if err != nil {
			snippet = nil
		}
		out = append(out, searchabstraction.SearchHit{
			ID:      repos.ObjectId(id),
			TypeID:  repos.TypeId(typeID),
			Score:   score,
			Snippet: snippet,
		})
	}
	return out
}
