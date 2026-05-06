package searchabstraction

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repos "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// --- Mirror Rust #[test] cases ------------------------------------------

func TestBackendChoiceParses(t *testing.T) {
	t.Parallel()
	got, ok := ParseBackendChoice("vespa")
	require.True(t, ok)
	assert.Equal(t, BackendVespa, got)

	got, ok = ParseBackendChoice("OpenSearch")
	require.True(t, ok)
	assert.Equal(t, BackendOpenSearch, got)

	got, ok = ParseBackendChoice("os")
	require.True(t, ok)
	assert.Equal(t, BackendOpenSearch, got)

	_, ok = ParseBackendChoice("redis")
	assert.False(t, ok)

	_, ok = ParseBackendChoice("")
	assert.False(t, ok)

	_, ok = ParseBackendChoice("   ")
	assert.False(t, ok)
}

func TestSanitizeDocTypeLowercasesAndStrips(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "foo_bar_baz", SanitizeDocType("Foo-Bar.Baz"))
	assert.Equal(t, "already_ok", SanitizeDocType("ALREADY_OK"))
	// Non-ASCII letters get replaced with `_` per the Rust contract
	// (`is_ascii_alphanumeric` is the gate).
	assert.Equal(t, "caf__cl_sico", SanitizeDocType("Café Clásico"))
	assert.Equal(t, "alpha_3", SanitizeDocType("alpha_3"))
}

// --- Backend choice round-trip ------------------------------------------

func TestBackendChoiceStringRoundTrip(t *testing.T) {
	t.Parallel()
	for _, c := range []BackendChoice{BackendVespa, BackendOpenSearch} {
		got, ok := ParseBackendChoice(c.String())
		require.True(t, ok)
		assert.Equal(t, c, got)
	}
}

// --- Registry / factory --------------------------------------------------

type stubBackend struct{ endpoint string }

func (stubBackend) Search(_ context.Context, _ SearchQuery, _ repos.ReadConsistency) (repos.PagedResult[SearchHit], error) {
	return repos.PagedResult[SearchHit]{}, nil
}
func (stubBackend) Index(_ context.Context, _ IndexDoc) error { return nil }
func (stubBackend) Delete(_ context.Context, _ repos.TenantId, _ repos.ObjectId) (bool, error) {
	return false, nil
}
func (stubBackend) SearchVector(_ context.Context, _ VectorQuery, _ repos.ReadConsistency) ([]SearchHit, error) {
	return nil, repos.ErrVectorSearchUnsupported()
}
func (stubBackend) BulkIndex(ctx context.Context, docs []IndexDoc) (BulkOutcome, error) {
	return repos.DefaultBulkIndex(ctx, stubBackend{}, docs)
}

func TestSearchBackendFromEnvMissingEndpoint(t *testing.T) {
	t.Setenv("SEARCH_ENDPOINT", "")
	_, err := SearchBackendFromEnv()
	require.Error(t, err)
	assert.True(t, repos.IsInvalidArgument(err))
	assert.Contains(t, err.Error(), "SEARCH_ENDPOINT not set")
}

func TestSearchBackendFromEnvUnregisteredBackend(t *testing.T) {
	t.Setenv("SEARCH_ENDPOINT", "http://search:8080")
	t.Setenv("SEARCH_BACKEND", "vespa")
	// No backends registered in this isolated test → InvalidArgument.
	_, err := SearchBackendFromEnv()
	require.Error(t, err)
	assert.True(t, repos.IsInvalidArgument(err))
	assert.Contains(t, err.Error(), "vespa")
}

func TestSearchBackendFromEnvRegisteredBackend(t *testing.T) {
	// Cannot run in parallel: mutates the package-level registry.
	t.Setenv("SEARCH_ENDPOINT", "http://search:8080")
	t.Setenv("SEARCH_BACKEND", "opensearch")
	RegisterBackend(BackendOpenSearch, func(endpoint string) SearchBackend {
		return stubBackend{endpoint: endpoint}
	})
	defer func() {
		registryMu.Lock()
		delete(registry, BackendOpenSearch)
		registryMu.Unlock()
	}()
	got, err := SearchBackendFromEnv()
	require.NoError(t, err)
	require.NotNil(t, got)
	stub, ok := got.(stubBackend)
	require.True(t, ok)
	assert.Equal(t, "http://search:8080", stub.endpoint)
}

// --- In-memory backend (delegated to storage-abstraction) ---------------

func TestNewInMemoryBackendIsTenantScoped(t *testing.T) {
	t.Parallel()
	be := NewInMemoryBackend()
	ctx := context.Background()
	t1, t2 := repos.TenantId("t1"), repos.TenantId("t2")
	tyDoc := repos.TypeId("doc")
	for _, d := range []IndexDoc{
		{Tenant: t1, ID: "a1", TypeID: tyDoc, Version: 1, Payload: json.RawMessage(`{"color":"blue","note":"alpha"}`)},
		{Tenant: t1, ID: "a2", TypeID: tyDoc, Version: 1, Payload: json.RawMessage(`{"color":"red"}`)},
		{Tenant: t2, ID: "b1", TypeID: tyDoc, Version: 1, Payload: json.RawMessage(`{"color":"blue","note":"tenant2-only-text"}`)},
	} {
		require.NoError(t, be.Index(ctx, d))
	}
	// Tenant isolation: t1 search must only return t1 rows.
	resT1, err := be.Search(ctx, SearchQuery{Tenant: t1, Page: repos.Page{Size: 10}}, repos.Eventual())
	require.NoError(t, err)
	assert.Len(t, resT1.Items, 2)
	for _, h := range resT1.Items {
		assert.NotEqual(t, repos.ObjectId("b1"), h.ID)
	}
	// Cross-tenant text leak blocked.
	leakQ := "tenant2-only-text"
	resLeak, err := be.Search(ctx, SearchQuery{Tenant: t1, Q: &leakQ, Page: repos.Page{Size: 10}}, repos.Eventual())
	require.NoError(t, err)
	assert.Empty(t, resLeak.Items, "tenant2-only payload must not leak into t1")
}

func TestNewInMemoryBackendStaleWriteDiscarded(t *testing.T) {
	t.Parallel()
	be := NewInMemoryBackend()
	ctx := context.Background()
	tenant := repos.TenantId("t1")
	id := repos.ObjectId("a1")
	require.NoError(t, be.Index(ctx, IndexDoc{
		Tenant: tenant, ID: id, TypeID: "doc", Version: 5, Payload: json.RawMessage(`{"x":1}`),
	}))
	// Stale write (version 3 < 5) must be silently discarded.
	require.NoError(t, be.Index(ctx, IndexDoc{
		Tenant: tenant, ID: id, TypeID: "doc", Version: 3, Payload: json.RawMessage(`{"x":99}`),
	}))
	// Search returns the original payload.
	res, err := be.Search(ctx, SearchQuery{Tenant: tenant, Page: repos.Page{Size: 10}}, repos.Eventual())
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.JSONEq(t, `{"x":1}`, string(res.Items[0].Snippet))
}

func TestNewInMemoryBackendVectorTopK(t *testing.T) {
	t.Parallel()
	be := NewInMemoryBackend()
	ctx := context.Background()
	tenant := repos.TenantId("t1")
	tyDoc := repos.TypeId("doc")
	for _, d := range []IndexDoc{
		{Tenant: tenant, ID: "a1", TypeID: tyDoc, Version: 1, Embedding: []float32{1, 0, 0}},
		{Tenant: tenant, ID: "a2", TypeID: tyDoc, Version: 1, Embedding: []float32{0.9, 0.1, 0}},
		{Tenant: tenant, ID: "a3", TypeID: tyDoc, Version: 1, Embedding: []float32{0, 1, 0}},
	} {
		require.NoError(t, be.Index(ctx, d))
	}
	hits, err := be.SearchVector(ctx, VectorQuery{
		Tenant: tenant, Embedding: []float32{1, 0, 0}, K: 2,
	}, repos.Eventual())
	require.NoError(t, err)
	require.Len(t, hits, 2)
	// a1 (perfect match) must rank above a2 (near-match), a3 dropped.
	assert.Equal(t, repos.ObjectId("a1"), hits[0].ID)
	assert.Equal(t, repos.ObjectId("a2"), hits[1].ID)
	assert.InDelta(t, 1.0, hits[0].Score, 1e-5)
}

func TestNewInMemoryBackendDeleteSemantics(t *testing.T) {
	t.Parallel()
	be := NewInMemoryBackend()
	ctx := context.Background()
	tenant := repos.TenantId("t1")
	id := repos.ObjectId("a1")
	require.NoError(t, be.Index(ctx, IndexDoc{
		Tenant: tenant, ID: id, TypeID: "doc", Version: 1, Payload: json.RawMessage(`{}`),
	}))
	deleted, err := be.Delete(ctx, tenant, id)
	require.NoError(t, err)
	assert.True(t, deleted)
	// Second delete reports missing.
	deleted, err = be.Delete(ctx, tenant, id)
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestNewInMemoryBackendBulkIndexCollectsFailures(t *testing.T) {
	t.Parallel()
	be := NewInMemoryBackend()
	out, err := be.BulkIndex(context.Background(), []IndexDoc{
		{Tenant: "t", ID: "a", TypeID: "x", Version: 1, Payload: json.RawMessage(`{}`)},
		{Tenant: "t", ID: "b", TypeID: "x", Version: 1, Payload: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(2), out.Indexed)
	assert.Empty(t, out.Failed)
}

// --- Default search-vector helper ---------------------------------------

func TestErrVectorSearchUnsupportedClassifies(t *testing.T) {
	t.Parallel()
	err := repos.ErrVectorSearchUnsupported()
	assert.True(t, repos.IsBackendError(err))
	assert.Contains(t, err.Error(), "vector search not supported")
	// Should remain a RepoBackend through an errors.Join wrap.
	wrapped := errors.Join(errors.New("transport"), err)
	assert.True(t, repos.IsBackendError(wrapped))
}
