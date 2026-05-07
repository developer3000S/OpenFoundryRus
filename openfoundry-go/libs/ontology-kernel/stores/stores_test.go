package stores_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// libs/ontology-kernel/src/stores/mod.rs `Stores::in_memory()` —
// the constructor returns a fully-wired bag where every trait field
// is satisfied.
func TestNewInMemoryWiresAllStores(t *testing.T) {
	s := stores.NewInMemory()
	require.NotNil(t, s.Objects)
	require.NotNil(t, s.Links)
	require.NotNil(t, s.Actions)
	require.NotNil(t, s.ObjectSetMaterializations)

	// Compile-time pin doubled at runtime via interface assertion.
	var _ storageabstraction.ObjectStore = s.Objects
	var _ storageabstraction.LinkStore = s.Links
	var _ storageabstraction.ActionLogStore = s.Actions
	var _ stores.ObjectSetMaterializationStore = s.ObjectSetMaterializations
}

// Postgres stores surface backend errors when they are not wired with a pool.
// The adapters are real SQL implementations; nil pools fail fast instead of
// returning the historical NOT_YET stub.
func TestPostgresStoresRequirePool(t *testing.T) {
	ctx := context.Background()
	obj := &stores.PostgresObjectStore{Pool: nil}
	_, err := obj.Get(ctx, "tenant-1", "obj-1", storageabstraction.Strong())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgres pool is nil")
	assert.True(t, storageabstraction.IsBackendError(err))

	link := &stores.PostgresLinkStore{}
	require.Error(t, link.Put(ctx, storageabstraction.Link{}))

	action := &stores.PostgresActionLogStore{}
	require.Error(t, action.Append(ctx, storageabstraction.ActionLogEntry{}))
}

// libs/ontology-kernel/src/stores/mod.rs `Stores::in_memory()` —
// the in-memory ObjectStore round-trips a put/get and applies
// optimistic concurrency the same way the production stores do.
func TestInMemoryObjectStoreRoundTripAndConflict(t *testing.T) {
	ctx := context.Background()
	s := stores.NewInMemoryObjectStore()

	obj := storageabstraction.Object{
		Tenant:  "t",
		ID:      "o",
		TypeID:  "ty",
		Version: 1,
	}
	out, err := s.Put(ctx, obj, nil)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.PutInserted, out.Kind)

	// Insert-only of an existing key conflicts.
	out, err = s.Put(ctx, obj, nil)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.PutVersionConflict, out.Kind)

	// Update with matching expected_version succeeds.
	v := uint64(1)
	obj.Version = 2
	out, err = s.Put(ctx, obj, &v)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.PutUpdated, out.Kind)
	assert.Equal(t, uint64(1), out.PreviousVersion)
	assert.Equal(t, uint64(2), out.NewVersion)

	// Update with stale expected_version conflicts.
	stale := uint64(1)
	obj.Version = 3
	out, err = s.Put(ctx, obj, &stale)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.PutVersionConflict, out.Kind)
	assert.Equal(t, uint64(1), out.ExpectedVersion)
	assert.Equal(t, uint64(2), out.ActualVersion)

	// Get returns the latest committed copy.
	got, err := s.Get(ctx, "t", "o", storageabstraction.Strong())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, uint64(2), got.Version)

	// Delete is idempotent.
	ok, err := s.Delete(ctx, "t", "o")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.Delete(ctx, "t", "o")
	require.NoError(t, err)
	assert.False(t, ok)
}

// libs/ontology-kernel/src/stores/mod.rs InMemoryLinkStore — Put is
// idempotent on the (tenant, link_type, from, to) triple.
func TestInMemoryLinkStoreIdempotentPut(t *testing.T) {
	ctx := context.Background()
	s := stores.NewInMemoryLinkStore()

	link := storageabstraction.Link{
		Tenant:   "t",
		LinkType: "owns",
		From:     "a",
		To:       "b",
	}
	require.NoError(t, s.Put(ctx, link))
	require.NoError(t, s.Put(ctx, link)) // second Put is a no-op.

	out, err := s.ListOutgoing(ctx, "t", "owns", "a", storageabstraction.Page{}, storageabstraction.Strong())
	require.NoError(t, err)
	assert.Len(t, out.Items, 1)

	in, err := s.ListIncoming(ctx, "t", "owns", "b", storageabstraction.Page{}, storageabstraction.Strong())
	require.NoError(t, err)
	assert.Len(t, in.Items, 1)

	// Delete returns false on second call.
	ok, err := s.Delete(ctx, "t", "owns", "a", "b")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.Delete(ctx, "t", "owns", "a", "b")
	require.NoError(t, err)
	assert.False(t, ok)
}

// libs/ontology-kernel/src/stores/mod.rs InMemoryActionLogStore —
// ListRecent / ListForObject / ListForAction return time-DESC and
// honour tenant scoping. Page{} (size 0) caps at 1 entry per the
// Rust noop's `page.size.max(1)` convention; the multi-item subtests
// pass an explicit Size so they pull every match.
func TestInMemoryActionLogScopingAndOrdering(t *testing.T) {
	ctx := context.Background()
	s := stores.NewInMemoryActionLogStore()
	objA := storageabstraction.ObjectId("A")
	bigPage := storageabstraction.Page{Size: 100}
	mk := func(tenant, action string, obj *storageabstraction.ObjectId, recordedAt int64) storageabstraction.ActionLogEntry {
		return storageabstraction.ActionLogEntry{
			Tenant:       storageabstraction.TenantId(tenant),
			ActionID:     action,
			Object:       obj,
			RecordedAtMs: recordedAt,
		}
	}
	require.NoError(t, s.Append(ctx, mk("t", "act-1", &objA, 1)))
	require.NoError(t, s.Append(ctx, mk("t", "act-2", nil, 2)))
	require.NoError(t, s.Append(ctx, mk("other", "act-3", &objA, 3)))

	recent, err := s.ListRecent(ctx, "t", bigPage, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, recent.Items, 2)
	// Newest-first: act-2 (recorded_at=2) precedes act-1 (recorded_at=1).
	assert.Equal(t, "act-2", recent.Items[0].ActionID)
	assert.Equal(t, "act-1", recent.Items[1].ActionID)

	forObj, err := s.ListForObject(ctx, "t", objA, bigPage, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, forObj.Items, 1)
	assert.Equal(t, "act-1", forObj.Items[0].ActionID)

	forAct, err := s.ListForAction(ctx, "t", "act-1", bigPage, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, forAct.Items, 1)

	// Other tenant is invisible.
	otherTenant, err := s.ListRecent(ctx, "other", bigPage, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, otherTenant.Items, 1)
	assert.Equal(t, "act-3", otherTenant.Items[0].ActionID)
}

// libs/ontology-kernel/src/stores/mock.rs `mockall::mock!`
// equivalent — record-and-return surfaces queue responses and
// expose every call for assertion.
func TestMockObjectStoreRecordsAndReturns(t *testing.T) {
	ctx := context.Background()
	m := stores.NewMockObjectStore()
	m.QueueGet(&storageabstraction.Object{ID: "o", Version: 7}, nil)
	m.QueuePut(storageabstraction.Updated(6, 7), nil)

	got, err := m.Get(ctx, "t", "o", storageabstraction.Strong())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, uint64(7), got.Version)
	assert.Len(t, m.GetCalls, 1)
	assert.Equal(t, storageabstraction.TenantId("t"), m.GetCalls[0].Tenant)

	out, err := m.Put(ctx, storageabstraction.Object{ID: "o", Version: 7}, nil)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.PutUpdated, out.Kind)
	assert.Len(t, m.PutCalls, 1)

	// No queued response → zero values, but the call is still recorded.
	_, err = m.Delete(ctx, "t", "o")
	require.NoError(t, err)
	assert.Len(t, m.DeleteCalls, 1)
}

// libs/ontology-kernel/src/stores/mock.rs MockLinkStore +
// MockActionLogStore satisfy their respective trait surfaces.
func TestMockLinkAndActionStoresImplementInterfaces(t *testing.T) {
	var _ storageabstraction.LinkStore = stores.NewMockLinkStore()
	var _ storageabstraction.ActionLogStore = stores.NewMockActionLogStore()
}

// libs/storage-abstraction/src/repositories.rs noop::InMemoryObjectStore::put
// — the in-memory fake forces version=1 on insert and
// version=existing+1 on update, regardless of what the caller passed.
// Mirrors the cassandra-kernel CAS write contract: the row-side bump
// is authoritative.
func TestInMemoryObjectStoreForcesVersionBump(t *testing.T) {
	ctx := context.Background()
	s := stores.NewInMemoryObjectStore()

	// Caller-supplied Version=99 on insert; fake stores version=1.
	obj := storageabstraction.Object{Tenant: "t", ID: "o", TypeID: "ty", Version: 99}
	out, err := s.Put(ctx, obj, nil)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.PutInserted, out.Kind)
	assert.Equal(t, uint64(1), out.NewVersion)
	got, err := s.Get(ctx, "t", "o", storageabstraction.Strong())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), got.Version)

	// Update with caller-supplied Version=99 + matching expected_version=1
	// — fake bumps to existing+1=2, ignoring the caller's 99.
	v := uint64(1)
	obj.Version = 99
	out, err = s.Put(ctx, obj, &v)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.PutUpdated, out.Kind)
	assert.Equal(t, uint64(1), out.PreviousVersion)
	assert.Equal(t, uint64(2), out.NewVersion)
	got, err = s.Get(ctx, "t", "o", storageabstraction.Strong())
	require.NoError(t, err)
	assert.Equal(t, uint64(2), got.Version)
}

// libs/storage-abstraction/src/repositories.rs noop::InMemoryObjectStore::list_by_*
// — items sort by UpdatedAtMs desc and truncate to page.size.max(1).
func TestInMemoryObjectStoreListSortsAndTruncates(t *testing.T) {
	ctx := context.Background()
	s := stores.NewInMemoryObjectStore()
	mk := func(id storageabstraction.ObjectId, updatedAt int64) storageabstraction.Object {
		return storageabstraction.Object{Tenant: "t", ID: id, TypeID: "ty", UpdatedAtMs: updatedAt}
	}
	for _, o := range []storageabstraction.Object{mk("a", 1), mk("b", 3), mk("c", 2)} {
		_, err := s.Put(ctx, o, nil)
		require.NoError(t, err)
	}

	// Page.Size = 0 → caps at 1 (max(1)) — newest entry only.
	out, err := s.ListByType(ctx, "t", "ty", storageabstraction.Page{}, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	assert.Equal(t, storageabstraction.ObjectId("b"), out.Items[0].ID)

	// Page.Size = 2 → newest two in desc order.
	out, err = s.ListByType(ctx, "t", "ty", storageabstraction.Page{Size: 2}, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, out.Items, 2)
	assert.Equal(t, storageabstraction.ObjectId("b"), out.Items[0].ID)
	assert.Equal(t, storageabstraction.ObjectId("c"), out.Items[1].ID)
}

// libs/storage-abstraction/src/repositories.rs noop::InMemoryLinkStore — list
// methods truncate to page.size.max(1).
func TestInMemoryLinkStoreListTruncates(t *testing.T) {
	ctx := context.Background()
	s := stores.NewInMemoryLinkStore()
	for _, to := range []storageabstraction.ObjectId{"x", "y", "z"} {
		require.NoError(t, s.Put(ctx, storageabstraction.Link{Tenant: "t", LinkType: "owns", From: "a", To: to}))
	}
	out, err := s.ListOutgoing(ctx, "t", "owns", "a", storageabstraction.Page{Size: 2}, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, out.Items, 2)

	// Page.Size = 0 → 1 item (max(1)).
	out, err = s.ListOutgoing(ctx, "t", "owns", "a", storageabstraction.Page{}, storageabstraction.Strong())
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
}

// libs/storage-abstraction/src/repositories.rs noop::InMemoryActionLogStore::append
// — duplicate Append on the same (tenant, effective_event_id) is a no-op.
// effective_event_id falls back to action_id when event_id is nil.
func TestInMemoryActionLogStoreAppendDedupes(t *testing.T) {
	ctx := context.Background()
	s := stores.NewInMemoryActionLogStore()
	bigPage := storageabstraction.Page{Size: 100}

	// No event_id → dedupe key is action_id.
	e := storageabstraction.ActionLogEntry{Tenant: "t", ActionID: "act-1", RecordedAtMs: 1}
	require.NoError(t, s.Append(ctx, e))
	require.NoError(t, s.Append(ctx, e)) // dedup
	out, err := s.ListRecent(ctx, "t", bigPage, storageabstraction.Strong())
	require.NoError(t, err)
	assert.Len(t, out.Items, 1)

	// Explicit event_id wins over action_id.
	eid := "evt-1"
	e2 := storageabstraction.ActionLogEntry{Tenant: "t", ActionID: "act-2", EventID: &eid, RecordedAtMs: 2}
	e3 := storageabstraction.ActionLogEntry{Tenant: "t", ActionID: "act-3", EventID: &eid, RecordedAtMs: 3}
	require.NoError(t, s.Append(ctx, e2))
	require.NoError(t, s.Append(ctx, e3)) // dedup on event_id="evt-1"
	out, err = s.ListRecent(ctx, "t", bigPage, storageabstraction.Strong())
	require.NoError(t, err)
	assert.Len(t, out.Items, 2) // act-1 + act-2 (act-3 deduped)
}
