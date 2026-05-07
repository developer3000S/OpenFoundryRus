// Package vespa is the Vespa-backed SearchBackend implementation,
// the production search target per ADR-0028. Mirrors
// libs/search-abstraction/src/vespa.rs.
//
// Wire format:
//
//   - Document API:
//     PUT /document/v1/{namespace}/{type}/group/{tenant}/{id}
//     with body {"fields": { …payload, "tenant", "version",
//     "embedding": {"values": […] } } }.
//   - Search API: POST /search/ with body
//     {"yql": "…", "hits": N, …}. Vector search uses
//     `nearestNeighbor(embedding, q)` with the query tensor passed
//     via `input.query(q)`.
//
// Stale-write protection uses Vespa's `condition` parameter
// (`condition={type}.version<{N}` ⇒ HTTP 412 on out-of-order PUT,
// silently treated as a no-op).
//
// Importing this package as a side-effect registers the backend
// with searchabstraction.SearchBackendFromEnv:
//
//	import _ "github.com/openfoundry/openfoundry-go/libs/search-abstraction/vespa"
package vespa

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

const defaultNamespace = "of"

func init() {
	searchabstraction.RegisterBackend(searchabstraction.BackendVespa,
		func(endpoint string) searchabstraction.SearchBackend { return New(endpoint) })
}

// Backend is the Vespa HTTP client. Construct with the cluster's
// HTTP endpoint (typically `https://vespa.search.svc.cluster.local:8080`).
type Backend struct {
	endpoint   string
	namespace  string
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

// WithAuthHeader configures the Authorization header sent to Vespa.
func WithAuthHeader(value string) Option {
	return func(b *Backend) { b.authHeader = strings.TrimSpace(value) }
}

// New builds a Backend with the default namespace (`of`) and a
// default *http.Client (30 s timeout).
func New(endpoint string) *Backend {
	return NewWithOptions(endpoint)
}

// NewWithOptions builds a Backend with optional transport/auth configuration.
func NewWithOptions(endpoint string, opts ...Option) *Backend {
	b := WithClient(endpoint, defaultNamespace, &http.Client{Timeout: 30 * time.Second})
	for _, opt := range opts {
		if opt != nil {
			opt(b)
		}
	}
	return b
}

// WithClient builds a Backend with a caller-provided namespace and
// *http.Client.
func WithClient(endpoint, namespace string, client *http.Client) *Backend {
	if client == nil {
		client = http.DefaultClient
	}
	if namespace == "" {
		namespace = defaultNamespace
	}
	return &Backend{
		endpoint:  strings.TrimRight(endpoint, "/"),
		namespace: namespace,
		http:      client,
	}
}

// Compile-time interface satisfaction.
var _ searchabstraction.SearchBackend = (*Backend)(nil)

// ─── helpers ───────────────────────────────────────────────────────────

func (b *Backend) docURL(docType string, tenant repos.TenantId, id repos.ObjectId) string {
	return fmt.Sprintf("%s/document/v1/%s/%s/group/%s/%s",
		b.endpoint, b.namespace, docType,
		percentEncode(string(tenant)), percentEncode(string(id)))
}

func (b *Backend) searchURL() string {
	return b.endpoint + "/search/"
}

func (b *Backend) buildYQL(query searchabstraction.SearchQuery) string {
	clauses := []string{
		fmt.Sprintf(`tenant contains "%s"`, yqlEscape(string(query.Tenant))),
	}
	if query.TypeID != nil {
		clauses = append(clauses, fmt.Sprintf(`type_id contains "%s"`, yqlEscape(string(*query.TypeID))))
	}
	for k, v := range query.Filters {
		clauses = append(clauses, fmt.Sprintf(`%s contains "%s"`, sanitizeField(k), yqlEscape(v)))
	}
	if query.Q != nil && *query.Q != "" {
		clauses = append(clauses, "userQuery()")
	}
	source := "*"
	if query.TypeID != nil {
		source = searchabstraction.SanitizeDocType(string(*query.TypeID))
	}
	limit := uint32(1)
	if query.Page.Size > 1 {
		limit = query.Page.Size
	}
	return fmt.Sprintf("select * from sources %s where %s limit %d",
		source, strings.Join(clauses, " and "), limit)
}

func yqlEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func sanitizeField(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// percentEncode mirrors the Rust impl: preserve [A-Za-z0-9-_.~] and
// percent-encode every other byte of the UTF-8 representation.
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
	return repos.Backend(fmt.Sprintf("vespa %s HTTP %d: %s",
		ctx, status, http.StatusText(status)))
}

// buildFields assembles the Vespa `{"fields": …}` body. If the
// payload is a JSON object the fields are merged in place; otherwise
// it is wrapped as `{"payload": …}` and then merged. The embedding
// is wrapped as `{"values": [...]}` per Vespa's tensor convention.
func buildFields(d searchabstraction.IndexDoc) (map[string]any, error) {
	fields := map[string]any{}
	if raw := bytes.TrimSpace([]byte(d.Payload)); len(raw) > 0 {
		if raw[0] == '{' {
			if err := json.Unmarshal(raw, &fields); err != nil {
				return nil, err
			}
		} else {
			var inner any
			if err := json.Unmarshal(raw, &inner); err != nil {
				return nil, err
			}
			fields = map[string]any{"payload": inner}
		}
	}
	fields["id"] = string(d.ID)
	fields["tenant"] = string(d.Tenant)
	fields["type_id"] = string(d.TypeID)
	fields["version"] = d.Version
	if len(d.Embedding) > 0 {
		fields["embedding"] = map[string]any{"values": d.Embedding}
	}
	return fields, nil
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

// Search runs a YQL lexical query. The user query is forwarded
// verbatim via the `query` parameter, which Vespa's `userQuery()`
// macro substitutes into the YQL `where` clause.
func (b *Backend) Search(
	ctx context.Context,
	query searchabstraction.SearchQuery,
	_ repos.ReadConsistency,
) (repos.PagedResult[searchabstraction.SearchHit], error) {
	hits := uint32(1)
	if query.Page.Size > 1 {
		hits = query.Page.Size
	}
	body := map[string]any{
		"yql":  b.buildYQL(query),
		"hits": hits,
	}
	if query.Q != nil {
		body["query"] = *query.Q
	}

	empty := repos.PagedResult[searchabstraction.SearchHit]{}
	resp, err := b.sendJSON(ctx, http.MethodPost, b.searchURL(), body)
	if err != nil {
		return empty, repos.Backend("vespa search send: " + err.Error())
	}
	defer resp.Body.Close()
	if err := mapStatus(resp.StatusCode, "search"); err != nil {
		return empty, err
	}
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return empty, repos.Backend("vespa search decode: " + err.Error())
	}
	return repos.PagedResult[searchabstraction.SearchHit]{Items: parseHits(v)}, nil
}

// Index PUTs a single document at /document/v1/{ns}/{type}/group/{tenant}/{id}
// guarded by `condition={type}.version<{N}`. HTTP 412 (condition
// failed = stale write) is silently swallowed.
func (b *Backend) Index(ctx context.Context, doc searchabstraction.IndexDoc) error {
	docType := searchabstraction.SanitizeDocType(string(doc.TypeID))
	u := b.docURL(docType, doc.Tenant, doc.ID)
	fields, err := buildFields(doc)
	if err != nil {
		return repos.Backend("vespa index marshal: " + err.Error())
	}
	body := map[string]any{"fields": fields}
	q := url.Values{}
	q.Set("condition", fmt.Sprintf("%s.version < %d", docType, doc.Version))
	u = u + "?" + q.Encode()

	buf, err := json.Marshal(body)
	if err != nil {
		return repos.Backend("vespa index marshal: " + err.Error())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(buf))
	if err != nil {
		return repos.Backend("vespa index req: " + err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	b.applyAuth(req)
	resp, err := b.http.Do(req)
	if err != nil {
		return repos.Backend("vespa index send: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return nil
	}
	return mapStatus(resp.StatusCode, "index")
}

// Delete first resolves the document's type via a lookup search
// (the trait does not expose type_id at delete-time), then issues a
// DELETE on the canonical document URL.
func (b *Backend) Delete(
	ctx context.Context,
	tenant repos.TenantId,
	id repos.ObjectId,
) (bool, error) {
	q := searchabstraction.SearchQuery{
		Tenant:  tenant,
		Filters: map[string]string{"id": string(id)},
		Page:    repos.Page{Size: 1},
	}
	hits, err := b.Search(ctx, q, repos.Eventual())
	if err != nil {
		return false, err
	}
	if len(hits.Items) == 0 {
		return false, nil
	}
	docType := searchabstraction.SanitizeDocType(string(hits.Items[0].TypeID))
	u := b.docURL(docType, tenant, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return false, repos.Backend("vespa delete req: " + err.Error())
	}
	b.applyAuth(req)
	resp, err := b.http.Do(req)
	if err != nil {
		return false, repos.Backend("vespa delete send: " + err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if err := mapStatus(resp.StatusCode, "delete"); err != nil {
		return false, err
	}
	return true, nil
}

// SearchVector emits a YQL query rooted at `nearestNeighbor` and
// passes the query tensor via `input.query(q)`. Filters degrade
// into additional `… contains "…"` clauses ANDed against the kNN
// targetHits constraint.
func (b *Backend) SearchVector(
	ctx context.Context,
	query searchabstraction.VectorQuery,
	_ repos.ReadConsistency,
) ([]searchabstraction.SearchHit, error) {
	k := uint32(1)
	if query.K > 1 {
		k = query.K
	}
	clauses := []string{
		fmt.Sprintf(`{targetHits:%d}nearestNeighbor(embedding, q)`, k),
		fmt.Sprintf(`tenant contains "%s"`, yqlEscape(string(query.Tenant))),
	}
	if query.TypeID != nil {
		clauses = append(clauses, fmt.Sprintf(`type_id contains "%s"`, yqlEscape(string(*query.TypeID))))
	}
	for fk, fv := range query.Filters {
		clauses = append(clauses, fmt.Sprintf(`%s contains "%s"`, sanitizeField(fk), yqlEscape(fv)))
	}
	source := "*"
	if query.TypeID != nil {
		source = searchabstraction.SanitizeDocType(string(*query.TypeID))
	}
	yql := fmt.Sprintf("select * from sources %s where %s limit %d",
		source, strings.Join(clauses, " and "), k)
	body := map[string]any{
		"yql":             yql,
		"input.query(q)":  query.Embedding,
		"ranking.profile": "embedding",
		"hits":            k,
	}
	resp, err := b.sendJSON(ctx, http.MethodPost, b.searchURL(), body)
	if err != nil {
		return nil, repos.Backend("vespa vector send: " + err.Error())
	}
	defer resp.Body.Close()
	if err := mapStatus(resp.StatusCode, "search_vector"); err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, repos.Backend("vespa vector decode: " + err.Error())
	}
	return parseHits(v), nil
}

// BulkIndex falls back to a sequential per-document loop. Vespa's
// /document/v1 HTTP gateway is per-document; the throughput knob is
// HTTP/2 multiplexing. Callers wanting parallelism should shard
// `docs` across goroutines themselves (the indexer service does).
func (b *Backend) BulkIndex(
	ctx context.Context,
	docs []searchabstraction.IndexDoc,
) (searchabstraction.BulkOutcome, error) {
	out := searchabstraction.BulkOutcome{Failed: []searchabstraction.BulkFail{}}
	for _, d := range docs {
		id := d.ID
		if err := b.Index(ctx, d); err != nil {
			out.Failed = append(out.Failed, searchabstraction.BulkFail{ID: id, Reason: err.Error()})
			continue
		}
		out.Indexed++
	}
	return out, nil
}

// parseHits decodes the Vespa `root.children[]` array. Mirrors
// fn parse_hits in the Rust impl.
func parseHits(v map[string]any) []searchabstraction.SearchHit {
	root, ok := v["root"].(map[string]any)
	if !ok {
		return []searchabstraction.SearchHit{}
	}
	arr, ok := root["children"].([]any)
	if !ok {
		return []searchabstraction.SearchHit{}
	}
	out := make([]searchabstraction.SearchHit, 0, len(arr))
	for _, raw := range arr {
		h, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		fields, ok := h["fields"].(map[string]any)
		if !ok {
			continue
		}
		id, _ := fields["id"].(string)
		if id == "" {
			continue
		}
		typeID, _ := fields["type_id"].(string)
		score := float32(0)
		if f, ok := h["relevance"].(float64); ok {
			score = float32(f)
		}
		snippet, err := json.Marshal(fields)
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
