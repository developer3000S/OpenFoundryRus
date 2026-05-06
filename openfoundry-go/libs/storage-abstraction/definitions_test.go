package storageabstraction

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// libs/storage-abstraction/src/repositories.rs `DefinitionRecord`
// JSON shape — the optional fields are gated on
// `skip_serializing_if = "Option::is_none"`, so the zero value
// renders without them.
func TestDefinitionRecordZeroValueOmitsOptionals(t *testing.T) {
	out, err := json.Marshal(DefinitionRecord{
		Kind:    "object_type",
		ID:      "123",
		Payload: json.RawMessage(`{"x":1}`),
	})
	require.NoError(t, err)
	s := string(out)
	for _, k := range []string{
		`"tenant"`, `"owner_id"`, `"parent_id"`, `"version"`,
		`"created_at_ms"`, `"updated_at_ms"`,
	} {
		assert.NotContains(t, s, k, "%s should be omitted on zero value", k)
	}
	assert.Contains(t, s, `"kind":"object_type"`)
	assert.Contains(t, s, `"id":"123"`)
	assert.Contains(t, s, `"payload":{"x":1}`)
}

// libs/storage-abstraction/src/repositories.rs `DefinitionQuery` —
// `filters` is `skip_serializing_if = "HashMap::is_empty"`, so an
// empty / nil map omits the key.
func TestDefinitionQueryEmptyFiltersOmits(t *testing.T) {
	out, err := json.Marshal(DefinitionQuery{
		Kind: "property",
		Page: Page{Size: 10},
	})
	require.NoError(t, err)
	s := string(out)
	assert.NotContains(t, s, `"filters"`)
	assert.NotContains(t, s, `"search"`)
	assert.NotContains(t, s, `"tenant"`)

	// Non-empty filters serialise.
	out, err = json.Marshal(DefinitionQuery{
		Kind:    "property",
		Filters: map[string]string{"name": "rating"},
		Page:    Page{Size: 10},
	})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"filters":{"name":"rating"}`)
}

// libs/storage-abstraction/src/repositories.rs `ReadModelRecord` is
// fully populated — every field is required (no `Option`).
func TestReadModelRecordRoundTrip(t *testing.T) {
	r := ReadModelRecord{
		Kind:        "function_run",
		Tenant:      "tenant-1",
		ID:          "run-1",
		Payload:     json.RawMessage(`{"k":"v"}`),
		Version:     7,
		UpdatedAtMs: 12345,
	}
	out, err := json.Marshal(r)
	require.NoError(t, err)
	for _, k := range []string{
		`"kind":"function_run"`, `"tenant":"tenant-1"`, `"id":"run-1"`,
		`"payload":{"k":"v"}`, `"version":7`, `"updated_at_ms":12345`,
	} {
		assert.Contains(t, string(out), k)
	}
	assert.NotContains(t, string(out), `"parent_id"`)

	var back ReadModelRecord
	require.NoError(t, json.Unmarshal(out, &back))
	assert.Equal(t, r, back)
}

// libs/storage-abstraction/src/repositories.rs
// `DefinitionStore::count` default impl is `list().items.len() as
// u64`. The Go helper [DefinitionCount] reproduces that via the
// public List method.
func TestDefinitionCountFallsBackToList(t *testing.T) {
	stub := &stubDefinitionStore{
		listResp: PagedResult[DefinitionRecord]{Items: []DefinitionRecord{
			{Kind: "x", ID: "a"},
			{Kind: "x", ID: "b"},
			{Kind: "x", ID: "c"},
		}},
	}
	n, err := DefinitionCount(context.Background(), stub, DefinitionQuery{Kind: "x"}, Strong())
	require.NoError(t, err)
	assert.Equal(t, uint64(3), n)

	// List error propagates verbatim.
	stub.listErr = errors.New("backend down")
	_, err = DefinitionCount(context.Background(), stub, DefinitionQuery{Kind: "x"}, Strong())
	require.Error(t, err)
	assert.Equal(t, "backend down", err.Error())
}

// stubDefinitionStore is the smallest possible test double for the
// helper above; full mocks live alongside their consumers.
type stubDefinitionStore struct {
	listResp PagedResult[DefinitionRecord]
	listErr  error
}

func (s *stubDefinitionStore) Get(_ context.Context, _ DefinitionKind, _ DefinitionId, _ ReadConsistency) (*DefinitionRecord, error) {
	return nil, nil
}
func (s *stubDefinitionStore) List(_ context.Context, _ DefinitionQuery, _ ReadConsistency) (PagedResult[DefinitionRecord], error) {
	return s.listResp, s.listErr
}
func (s *stubDefinitionStore) Put(_ context.Context, _ DefinitionRecord, _ *uint64) (PutOutcome, error) {
	return PutOutcome{}, nil
}
func (s *stubDefinitionStore) Delete(_ context.Context, _ DefinitionKind, _ DefinitionId) (bool, error) {
	return false, nil
}
func (s *stubDefinitionStore) Count(_ context.Context, _ DefinitionQuery, _ ReadConsistency) (uint64, error) {
	return 0, nil
}

// Compile-time pin: the stub satisfies the trait surface.
var _ DefinitionStore = (*stubDefinitionStore)(nil)
