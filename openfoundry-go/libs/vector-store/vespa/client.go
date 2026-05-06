package vespa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	vectorstore "github.com/openfoundry/openfoundry-go/libs/vector-store"
)

// Config is the configuration required to talk to a Vespa
// container/cluster.
//
// The defaults mirror Vespa's own defaults (Namespace = "default",
// DocType = "doc", RankProfile = "hybrid", EmbeddingField =
// "embedding", TextField = "text") so that consumers running the
// schema documented at the package level only need to provide the
// Endpoint.
type Config struct {
	// Endpoint is the base URL of the Vespa container, e.g.
	// "http://localhost:8080". No trailing slash.
	Endpoint string
	// Namespace is the Vespa document namespace (the <namespace>
	// segment of the Document v1 path).
	Namespace string
	// DocType is the document type (the <doctype> segment, also
	// the schema name).
	DocType string
	// TextField is the name of the indexed BM25 string field.
	TextField string
	// EmbeddingField is the name of the dense `tensor<float>(x[N])`
	// field.
	EmbeddingField string
	// RankProfile is the rank-profile to use for hybrid queries.
	RankProfile string
	// RequestTimeout is the HTTP timeout for individual Vespa
	// requests.
	RequestTimeout time.Duration
}

// NewConfig builds a config with sensible defaults for the schema
// documented in this package, pointing at endpoint.
func NewConfig(endpoint string) Config {
	return Config{
		Endpoint:       strings.TrimRight(endpoint, "/"),
		Namespace:      "default",
		DocType:        "doc",
		TextField:      "text",
		EmbeddingField: "embedding",
		RankProfile:    "hybrid",
		RequestTimeout: 10 * time.Second,
	}
}

// Backend is a [vectorstore.VectorBackend] implementation backed by
// Vespa.
//
// Cheap to copy by value — the embedded http.Client is itself
// reference-typed.
type Backend struct {
	cfg  Config
	http *http.Client
}

// NewBackend creates a new backend with the given configuration.
// Returns an error if the underlying http.Client cannot be built;
// kept as an error to mirror the Rust API surface, even though the
// Go std lib's `&http.Client{}` is effectively infallible.
func NewBackend(cfg Config) (*Backend, error) {
	return &Backend{cfg: cfg, http: &http.Client{Timeout: cfg.RequestTimeout}}, nil
}

// WithClient is the same as NewBackend but uses a caller-provided
// http.Client. Useful for sharing connection pools or installing
// custom middleware in tests.
func WithClient(cfg Config, client *http.Client) *Backend {
	if client == nil {
		client = &http.Client{Timeout: cfg.RequestTimeout}
	}
	return &Backend{cfg: cfg, http: client}
}

// DocumentURL builds the Document v1 URL for docID.
func (b *Backend) DocumentURL(docID string) string {
	return fmt.Sprintf(
		"%s/document/v1/%s/%s/docid/%s",
		b.cfg.Endpoint,
		b.cfg.Namespace,
		b.cfg.DocType,
		urlencode(docID),
	)
}

// SearchURL builds the search endpoint URL.
func (b *Backend) SearchURL() string {
	return b.cfg.Endpoint + "/search/"
}

// BuildSearchBody composes the YQL statement and search request
// body for a hybrid query. Exposed so unit tests can assert the
// shape without needing a live Vespa.
func (b *Backend) BuildSearchBody(text string, embedding []float32, filter vectorstore.Filter, topK int) map[string]any {
	whereClauses := make([]string, 0, 2)

	if text != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(
			"(%s contains \"%s\")",
			b.cfg.TextField,
			yqlEscape(text),
		))
	}
	if len(embedding) > 0 {
		// `targetHits` is a hint to the ANN operator; we ask for
		// at least topK candidates so the rank-profile has
		// material to re-rank.
		hits := topK
		if hits < 1 {
			hits = 1
		}
		whereClauses = append(whereClauses, fmt.Sprintf(
			"({targetHits:%d}nearestNeighbor(%s,q_embedding))",
			hits,
			b.cfg.EmbeddingField,
		))
	}

	// Combine text + ANN with OR so either signal can surface a
	// hit; the rank-profile is what actually fuses them.
	var yql string
	if len(whereClauses) == 0 {
		yql = fmt.Sprintf("select * from sources %s where true", b.cfg.DocType)
	} else {
		yql = fmt.Sprintf(
			"select * from sources %s where %s",
			b.cfg.DocType,
			strings.Join(whereClauses, " or "),
		)
	}

	// Stable iteration over Equals so the YQL string is
	// deterministic across runs (Go map iteration would
	// otherwise randomise it).
	keys := make([]string, 0, len(filter.Equals))
	for k := range filter.Equals {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, field := range keys {
		yql += fmt.Sprintf(
			" and (%s contains \"%s\")",
			field,
			yqlEscape(jsonToString(filter.Equals[field])),
		)
	}

	body := map[string]any{
		"yql":             yql,
		"hits":            topK,
		"ranking.profile": b.cfg.RankProfile,
	}
	if len(embedding) > 0 {
		// Vespa accepts indexed tensors as plain JSON arrays
		// under `input.query(<name>)` when posted via the Search
		// API.
		body["input.query(q_embedding)"] = embeddingToJSON(embedding)
	}
	return body
}

// Upsert inserts or replaces a document.
func (b *Backend) Upsert(ctx context.Context, docID string, fields map[string]json.RawMessage, embedding []float32) error {
	payloadFields := make(map[string]any, len(fields)+1)
	for k, v := range fields {
		payloadFields[k] = v
	}
	if len(embedding) > 0 {
		// Indexed tensors go on the wire as plain JSON arrays
		// under the field name; Vespa parses them according to
		// the schema.
		payloadFields[b.cfg.EmbeddingField] = embeddingToJSON(embedding)
	}
	body := map[string]any{"fields": payloadFields}

	url := b.DocumentURL(docID)
	resp, err := b.do(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	_, err = checkStatus(resp)
	return err
}

// Delete removes a document by id. Vespa returns 200 on delete
// even when the doc was missing; we treat 404 as no-op for
// portability.
func (b *Backend) Delete(ctx context.Context, docID string) error {
	url := b.DocumentURL(docID)
	resp, err := b.do(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		drainAndClose(resp.Body)
		return nil
	}
	_, err = checkStatus(resp)
	return err
}

// HybridQuery runs a hybrid (lexical + dense) query.
func (b *Backend) HybridQuery(ctx context.Context, text string, embedding []float32, filter vectorstore.Filter, topK int) ([]vectorstore.QueryHit, error) {
	body := b.BuildSearchBody(text, embedding, filter, topK)
	url := b.SearchURL()
	resp, err := b.do(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	value, err := checkStatus(resp)
	if err != nil {
		return nil, err
	}
	return ParseSearchResponse(value)
}

// Compile-time assertion that *Backend satisfies the interface.
var _ vectorstore.VectorBackend = (*Backend)(nil)

// do performs an HTTP request with optional JSON body, returning
// the raw response (caller is responsible for status checking via
// checkStatus).
func (b *Backend) do(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, vectorstore.NewSerializationError("%s", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, vectorstore.NewTransportError("%s", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, vectorstore.NewTransportError("%s", err)
	}
	return resp, nil
}

// checkStatus validates 2xx responses; converts anything else into
// a [vectorstore.BackendErrBackend] with the body for context.
// Returns the parsed JSON body (Null on empty body).
func checkStatus(resp *http.Response) (json.RawMessage, error) {
	defer drainAndClose(resp.Body)
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, vectorstore.NewTransportError("%s", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, vectorstore.NewBackendError("%s: %s", resp.Status, bodyBytes)
	}
	if len(bodyBytes) == 0 {
		return json.RawMessage("null"), nil
	}
	// Validate that it parses as JSON; the Rust impl returns the
	// parsed Value, but downstream consumers only need the bytes.
	var probe any
	if err := json.Unmarshal(bodyBytes, &probe); err != nil {
		return nil, vectorstore.NewSerializationError("%s", err)
	}
	return bodyBytes, nil
}

func drainAndClose(rc io.ReadCloser) {
	if rc == nil {
		return
	}
	_, _ = io.Copy(io.Discard, rc)
	_ = rc.Close()
}

// ParseSearchResponse parses a Vespa Search API JSON response into
// [vectorstore.QueryHit]s.
//
// Lives at package scope so unit tests can call it on captured
// fixtures without a live Vespa.
func ParseSearchResponse(value json.RawMessage) ([]vectorstore.QueryHit, error) {
	type child struct {
		ID        *string                    `json:"id"`
		Relevance *float64                   `json:"relevance"`
		Fields    map[string]json.RawMessage `json:"fields"`
	}
	type inner struct {
		Children []child `json:"children"`
	}
	type root struct {
		Root inner `json:"root"`
	}

	var parsed root
	if err := json.Unmarshal(value, &parsed); err != nil {
		return nil, vectorstore.NewSerializationError("%s", err)
	}
	out := make([]vectorstore.QueryHit, 0, len(parsed.Root.Children))
	for _, c := range parsed.Root.Children {
		// Vespa returns ids like "id:<namespace>:<doctype>::<docid>";
		// the user-supplied id is the trailing segment after the
		// LAST `::` (mirrors Rust's `rsplit_once("::")`).
		raw := ""
		if c.ID != nil {
			raw = *c.ID
		}
		id := raw
		if idx := strings.LastIndex(raw, "::"); idx >= 0 {
			id = raw[idx+2:]
		}
		score := 0.0
		if c.Relevance != nil {
			score = *c.Relevance
		}
		hit := vectorstore.QueryHit{
			ID:     id,
			Score:  score,
			Fields: c.Fields,
		}
		if hit.Fields == nil {
			hit.Fields = map[string]json.RawMessage{}
		}
		out = append(out, hit)
	}
	return out, nil
}

// embeddingToJSON converts a dense float32 embedding into the JSON
// array Vespa expects on the wire.
//
// JSON has no representation for non-finite floats, so any NaN,
// +Inf or -Inf component is mapped to nil (which encoding/json
// emits as `null`). Vespa will in turn reject the document/query
// (4xx with a clear message), which surfaces as
// [vectorstore.BackendErrBackend]. Callers should ensure their
// embeddings are finite — every model worth using already emits
// finite values.
func embeddingToJSON(embedding []float32) []any {
	out := make([]any, len(embedding))
	for i, f := range embedding {
		v := float64(f)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			out[i] = nil
			continue
		}
		out[i] = v
	}
	return out
}

// urlencode applies a minimal percent-encoding to the path segment
// of Document v1 URLs. We only encode the characters that would
// otherwise change the path's structural meaning; everything else
// passes through unchanged so user ids stay readable in logs.
func urlencode(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case b >= 'A' && b <= 'Z',
			b >= 'a' && b <= 'z',
			b >= '0' && b <= '9',
			b == '-' || b == '_' || b == '.' || b == '~':
			sb.WriteByte(b)
		default:
			fmt.Fprintf(&sb, "%%%02X", b)
		}
	}
	return sb.String()
}

// yqlEscape escapes a string value so it is safe to embed inside a
// YQL double-quoted literal.
func yqlEscape(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))
	for _, c := range s {
		switch c {
		case '\\':
			sb.WriteString(`\\`)
		case '"':
			sb.WriteString(`\"`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// jsonToString renders a JSON scalar as a string for use in a YQL
// `contains` clause. Non-scalar values are JSON-stringified, which
// is intentional: callers shouldn't be passing arrays/objects to
// an equality filter, and if they do we still produce *something*
// deterministic instead of panicking.
func jsonToString(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	var b bool
	if err := json.Unmarshal(v, &b); err == nil {
		return fmt.Sprintf("%t", b)
	}
	var n json.Number
	if err := json.Unmarshal(v, &n); err == nil {
		return n.String()
	}
	if string(v) == "null" {
		return ""
	}
	return string(v)
}

// sortStrings is the local equivalent of sort.Strings without
// pulling the whole sort package; it's only used for small slices
// of map keys so a simple insertion sort is sufficient.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
