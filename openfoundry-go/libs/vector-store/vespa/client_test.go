package vespa_test

import (
	"encoding/json"
	"strings"
	"testing"

	vectorstore "github.com/openfoundry/openfoundry-go/libs/vector-store"
	"github.com/openfoundry/openfoundry-go/libs/vector-store/vespa"
)

func backend(t *testing.T) *vespa.Backend {
	t.Helper()
	b, err := vespa.NewBackend(vespa.NewConfig("http://vespa.test:8080"))
	if err != nil {
		t.Fatalf("client must build with default config: %v", err)
	}
	return b
}

func TestDocumentURLEncodesIDAndUsesNamespace(t *testing.T) {
	b := backend(t)
	got := b.DocumentURL("doc/with space")
	want := "http://vespa.test:8080/document/v1/default/doc/docid/doc%2Fwith%20space"
	if got != want {
		t.Fatalf("document url:\n got=%s\nwant=%s", got, want)
	}
}

func TestSearchURLIsWellFormed(t *testing.T) {
	b := backend(t)
	if got := b.SearchURL(); got != "http://vespa.test:8080/search/" {
		t.Fatalf("search url: %s", got)
	}
}

func TestBuildSearchBodyIncludesYQLRankingAndTensor(t *testing.T) {
	b := backend(t)
	body := b.BuildSearchBody("hello", []float32{0.1, 0.2, 0.3}, vectorstore.Filter{}, 5)

	if hits, _ := body["hits"].(int); hits != 5 {
		t.Fatalf("hits: %v", body["hits"])
	}
	if profile, _ := body["ranking.profile"].(string); profile != "hybrid" {
		t.Fatalf("ranking.profile: %v", body["ranking.profile"])
	}
	yql, _ := body["yql"].(string)
	if !strings.Contains(yql, `text contains "hello"`) {
		t.Fatalf("yql missing text clause: %s", yql)
	}
	if !strings.Contains(yql, "nearestNeighbor(embedding,q_embedding)") {
		t.Fatalf("yql missing ANN clause: %s", yql)
	}
	tensor, ok := body["input.query(q_embedding)"].([]any)
	if !ok {
		t.Fatalf("tensor missing or wrong type: %T", body["input.query(q_embedding)"])
	}
	if len(tensor) != 3 {
		t.Fatalf("tensor length: %d", len(tensor))
	}
}

func TestBuildSearchBodyWithFilterAppendsAndClause(t *testing.T) {
	b := backend(t)
	body := b.BuildSearchBody("", []float32{0.1, 0.2}, vectorstore.FilterEq("tenant_id", "acme"), 3)
	yql, _ := body["yql"].(string)
	if !strings.Contains(yql, `and (tenant_id contains "acme")`) {
		t.Fatalf("yql missing and-clause: %s", yql)
	}
}

func TestBuildSearchBodyTextOnlyOmitsTensorInput(t *testing.T) {
	b := backend(t)
	body := b.BuildSearchBody("hello", nil, vectorstore.Filter{}, 4)
	if _, ok := body["input.query(q_embedding)"]; ok {
		t.Fatalf("tensor input should be omitted when embedding is empty")
	}
	yql, _ := body["yql"].(string)
	if strings.Contains(yql, "nearestNeighbor") {
		t.Fatalf("yql should not contain ANN clause: %s", yql)
	}
}

func TestYQLEscapeQuotesSafely(t *testing.T) {
	b := backend(t)
	body := b.BuildSearchBody(`she said "hi"`, nil, vectorstore.Filter{}, 1)
	yql, _ := body["yql"].(string)
	if !strings.Contains(yql, `text contains "she said \"hi\""`) {
		t.Fatalf("yql escape failed: %s", yql)
	}
}

func TestParseSearchResponseExtractsIDScoreAndFields(t *testing.T) {
	raw := json.RawMessage(`{
		"root": {
			"children": [
				{"id": "id:default:doc::abc-1", "relevance": 1.42, "fields": {"text": "hello world"}},
				{"id": "id:default:doc::abc-2", "relevance": 0.71, "fields": {}}
			]
		}
	}`)
	hits, err := vespa.ParseSearchResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits: %d", len(hits))
	}
	if hits[0].ID != "abc-1" {
		t.Fatalf("hits[0].ID: %s", hits[0].ID)
	}
	if diff := hits[0].Score - 1.42; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("hits[0].Score: %v", hits[0].Score)
	}
	if got := hits[0].Fields["text"]; string(got) != `"hello world"` {
		t.Fatalf("hits[0].Fields[text]: %s", got)
	}
	if hits[1].ID != "abc-2" {
		t.Fatalf("hits[1].ID: %s", hits[1].ID)
	}
}

func TestParseSearchResponseSplitsOnLastDoubleColon(t *testing.T) {
	// User-supplied id contains "::"; mirrors Rust's rsplit_once
	// which splits at the LAST occurrence so the trailing
	// segment is the doc id, not an internal piece.
	raw := json.RawMessage(`{
		"root": {
			"children": [
				{"id": "id:default:doc::abc::user-1", "relevance": 0.5, "fields": {}}
			]
		}
	}`)
	hits, err := vespa.ParseSearchResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if hits[0].ID != "user-1" {
		t.Fatalf("expected user-1, got %q", hits[0].ID)
	}
}

func TestParseSearchResponseHandlesEmptyChildren(t *testing.T) {
	raw := json.RawMessage(`{"root": {}}`)
	hits, err := vespa.ParseSearchResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits, got %d", len(hits))
	}
}
