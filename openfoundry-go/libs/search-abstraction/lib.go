// Package searchabstraction is the search backend abstraction for
// OpenFoundry. Mirrors libs/search-abstraction/src/lib.rs:
//
//   - Re-exports the canonical SearchBackend trait surface from
//     storage-abstraction so services can depend on this package
//     when they only need search.
//   - Ships SanitizeDocType + BackendChoice + the env-driven
//     factory.
//
// Backend implementations (Vespa for production, OpenSearch for
// dev/CI) land in their own files (vespa.go, opensearch.go) — the
// HTTP wiring is substantial and only really validatable against
// real backends, so it ports alongside the consuming service
// (ontology-indexer) when that wires search in. Pure-logic
// surface lives here.
//
// The in-memory test fake (the Rust `noop::InMemorySearchBackend`
// from storage-abstraction's contract suite) is re-exposed via
// `repos.NewInMemorySearchBackend()` since it lives in the
// storage-abstraction package on the Go side; importers can use
// it directly.
package searchabstraction

import (
	"context"
	"os"
	"strings"
	"sync"

	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// Re-export the search trait surface so consumers only need to
// import this package.

// SearchBackend re-exports repos.SearchBackend.
type SearchBackend = repos.SearchBackend

// SearchQuery re-exports repos.SearchQuery.
type SearchQuery = repos.SearchQuery

// SearchHit re-exports repos.SearchHit.
type SearchHit = repos.SearchHit

// IndexDoc re-exports repos.IndexDoc.
type IndexDoc = repos.IndexDoc

// VectorQuery re-exports repos.VectorQuery.
type VectorQuery = repos.VectorQuery

// BulkOutcome re-exports repos.BulkOutcome.
type BulkOutcome = repos.BulkOutcome

// BulkFail re-exports repos.BulkFail.
type BulkFail = repos.BulkFail

// SanitizeDocType normalises an arbitrary string into a Vespa /
// OpenSearch-friendly identifier (lower-case, only `[a-z0-9_]`
// retained; everything else replaced with `_`). Used by both
// backends when computing document type names + index names.
// Mirrors fn sanitize_doc_type.
func SanitizeDocType(s string) string {
	out := make([]byte, 0, len(s))
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, byte(c))
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// BackendChoice selects a concrete SearchBackend at runtime.
type BackendChoice uint8

const (
	// BackendVespa is the production target.
	BackendVespa BackendChoice = iota
	// BackendOpenSearch is the dev / CI fallback.
	BackendOpenSearch
)

// String returns the canonical lower-case wire form of the choice
// (matches the values accepted by ParseBackendChoice).
func (c BackendChoice) String() string {
	switch c {
	case BackendVespa:
		return "vespa"
	case BackendOpenSearch:
		return "opensearch"
	}
	return "unknown"
}

// ParseBackendChoice mirrors fn parse: case-insensitive, accepts
// "vespa" / "opensearch" / "os". Returns (zero, false) for unset
// or unrecognised values; callers decide the default for their
// environment.
func ParseBackendChoice(s string) (BackendChoice, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "vespa":
		return BackendVespa, true
	case "opensearch", "os":
		return BackendOpenSearch, true
	}
	return 0, false
}

// BackendChoiceFromEnv resolves the choice from SEARCH_BACKEND.
// Defaults to Vespa (production-safe; dev/CI must opt out).
// Mirrors fn from_env.
func BackendChoiceFromEnv() BackendChoice {
	if v := os.Getenv("SEARCH_BACKEND"); v != "" {
		if c, ok := ParseBackendChoice(v); ok {
			return c
		}
	}
	return BackendVespa
}

// FactoryFunc is the constructor a backend implementation
// registers via RegisterBackend. Receives the SEARCH_ENDPOINT URL
// and returns a SearchBackend.
type FactoryFunc func(endpoint string) SearchBackend

var (
	registryMu sync.RWMutex
	registry   = map[BackendChoice]FactoryFunc{}
)

// RegisterBackend wires `factory` for `choice`. Backend ports
// (vespa.go, opensearch.go) call this in `init()` so importers
// only need to import the backend package as a side-effect:
//
//	import (
//	    _ "github.com/openfoundry/openfoundry-go/libs/search-abstraction/vespa"
//	)
//
// Mirrors the Rust `cfg(feature = "vespa")` gate.
func RegisterBackend(choice BackendChoice, factory FactoryFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[choice] = factory
}

// SearchBackendFromEnv builds a SearchBackend from SEARCH_BACKEND
// + SEARCH_ENDPOINT. Returns RepoInvalidArgument when the chosen
// backend has not been registered (i.e. its package wasn't
// imported), or when SEARCH_ENDPOINT is unset.
//
// Mirrors fn search_backend_from_env. The Go feature-flag
// equivalent is "did the consumer import the backend package".
func SearchBackendFromEnv() (SearchBackend, error) {
	endpoint := os.Getenv("SEARCH_ENDPOINT")
	if endpoint == "" {
		return nil, repos.Invalid("SEARCH_ENDPOINT not set")
	}
	choice := BackendChoiceFromEnv()
	registryMu.RLock()
	factory, ok := registry[choice]
	registryMu.RUnlock()
	if !ok {
		return nil, repos.Invalidf(
			"search-abstraction was built without the `%s` backend (import the package to register it)",
			choice)
	}
	return factory(endpoint), nil
}

// NewInMemoryBackend returns a fresh in-memory SearchBackend.
// Useful for tests and local-first dev when neither Vespa nor
// OpenSearch is reachable. Thin re-export of
// repos.NewInMemorySearchBackend so consumers only need to depend
// on this package.
func NewInMemoryBackend() SearchBackend {
	return repos.NewInMemorySearchBackend()
}

// _ = context.Context — silence unused-import lint when no other
// Go code in this file directly references context (consumers
// will, via the interface methods).
var _ = context.Background
