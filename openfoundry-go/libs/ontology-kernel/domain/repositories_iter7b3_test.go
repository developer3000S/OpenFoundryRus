package domain

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// ---- pg_repository -------------------------------------------------------

// libs/ontology-kernel/src/domain/pg_repository.rs `definition_table`
// must accept all 14 declared kinds (each with its plural alias) and
// reject anything else with the verbatim Rust string.
func TestDefinitionTableForKnownKinds(t *testing.T) {
	known := []string{
		"object_type", "object_types",
		"property", "properties",
		"shared_property_type", "shared_property_types",
		"interface", "interfaces",
		"interface_property", "interface_properties",
		"link_type", "link_types",
		"action_type", "action_types",
		"rule", "rules",
		"function_package", "function_packages",
		"object_set", "object_sets",
		"quiver_visual_function", "quiver_visual_functions",
		"project", "projects",
		"funnel_source", "funnel_sources",
	}
	for _, kind := range known {
		_, ok := definitionTableFor(storageabstraction.DefinitionKind(kind))
		assert.True(t, ok, "kind %q must be known", kind)
	}

	_, ok := definitionTableFor(storageabstraction.DefinitionKind("nope"))
	assert.False(t, ok)

	err := unsupportedKind(storageabstraction.DefinitionKind("bogus"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ontology definition kind 'bogus'")
}

// libs/ontology-kernel/src/domain/pg_repository.rs `page_offset`.
// Numeric token → offset; missing token → 0; non-numeric → verbatim
// "definition page token is invalid".
func TestPageOffsetTokenValidation(t *testing.T) {
	zero, err := pageOffset(storageabstraction.Page{Size: 10})
	require.NoError(t, err)
	assert.Equal(t, uint32(0), zero)

	tok := "42"
	n, err := pageOffset(storageabstraction.Page{Size: 10, Token: &tok})
	require.NoError(t, err)
	assert.Equal(t, uint32(42), n)

	bad := "not-a-number"
	_, err = pageOffset(storageabstraction.Page{Size: 10, Token: &bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definition page token is invalid")
}

// libs/ontology-kernel/src/domain/pg_repository.rs
// `push_definition_filters` — owner_col + parent_col + filter_cols
// + search_cols. The Go [pgQueryBuilder] renumbers $N placeholders
// as parameters are bound; the test pins the resulting SQL +
// args verbatim.
func TestPushDefinitionFiltersComposesSQL(t *testing.T) {
	table, ok := definitionTableFor("action_type")
	require.True(t, ok)

	owner := "owner-123"
	parent := storageabstraction.DefinitionId("type-456")
	search := "  hello  "

	q := storageabstraction.DefinitionQuery{
		Kind:     "action_type",
		OwnerID:  &owner,
		ParentID: &parent,
		Filters:  map[string]string{"name": "alpha"},
		Search:   &search,
	}

	b := newPGQueryBuilder()
	b.WriteString("WHERE TRUE")
	pushDefinitionFilters(b, table, q)

	sql := b.SQL()
	args := b.Args()

	// owner ($1) + parent ($2) + name ($3) + search across name +
	// display_name + operation_kind ($4 reused 3 times).
	assert.Contains(t, sql, "owner_id::text = $1")
	assert.Contains(t, sql, "object_type_id::text = $2")
	assert.Contains(t, sql, "name::text = $3")
	assert.Contains(t, sql, "name ILIKE $4")
	assert.Contains(t, sql, "display_name ILIKE $5")
	assert.Contains(t, sql, "operation_kind ILIKE $6")
	require.Len(t, args, 6)
	assert.Equal(t, "owner-123", args[0])
	assert.Equal(t, "type-456", args[1])
	assert.Equal(t, "alpha", args[2])
	// Search wrapper trims whitespace before wrapping in `%...%`.
	assert.Equal(t, "%hello%", args[3])
}

// libs/ontology-kernel/src/domain/pg_repository.rs
// `push_definition_filters` — filters whose column is NOT in
// `filter_cols` are silently dropped (defence in depth against
// arbitrary client filters).
func TestPushDefinitionFiltersDropsUnknownColumn(t *testing.T) {
	table, _ := definitionTableFor("project")
	q := storageabstraction.DefinitionQuery{
		Kind:    "project",
		Filters: map[string]string{"sneaky": "x", "owner_id": "y"},
	}
	b := newPGQueryBuilder()
	b.WriteString("WHERE TRUE")
	pushDefinitionFilters(b, table, q)
	assert.NotContains(t, b.SQL(), "sneaky")
	assert.Contains(t, b.SQL(), "owner_id::text =")
}

// libs/ontology-kernel/src/domain/pg_repository.rs
// `record_from_payload` — projects id / owner / parent / version
// from the row JSON. Missing id rejects with verbatim Rust error.
func TestRecordFromPayloadProjectsCanonicalFields(t *testing.T) {
	table, _ := definitionTableFor("action_type")
	rec, err := recordFromPayload("action_type", table, json.RawMessage(`{
        "id": "11111111-1111-1111-1111-111111111111",
        "owner_id": "22222222-2222-2222-2222-222222222222",
        "object_type_id": "33333333-3333-3333-3333-333333333333",
        "version": 7
    }`))
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.DefinitionKind("action_type"), rec.Kind)
	assert.Equal(t, storageabstraction.DefinitionId("11111111-1111-1111-1111-111111111111"), rec.ID)
	require.NotNil(t, rec.OwnerID)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", *rec.OwnerID)
	require.NotNil(t, rec.ParentID)
	assert.Equal(t, storageabstraction.DefinitionId("33333333-3333-3333-3333-333333333333"), *rec.ParentID)
	require.NotNil(t, rec.Version)
	assert.Equal(t, uint64(7), *rec.Version)

	// Missing id → verbatim rejection.
	_, err = recordFromPayload("action_type", table, json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "definition row did not project an id")
}

// libs/ontology-kernel/src/domain/pg_repository.rs
// `record_from_payload` — falls back to `revision` when `version`
// is absent.
func TestRecordFromPayloadFallsBackToRevision(t *testing.T) {
	table, _ := definitionTableFor("project")
	rec, err := recordFromPayload("project", table, json.RawMessage(`{
        "id": "11111111-1111-1111-1111-111111111111",
        "revision": 42
    }`))
	require.NoError(t, err)
	require.NotNil(t, rec.Version)
	assert.Equal(t, uint64(42), *rec.Version)
}

// libs/ontology-kernel/src/domain/pg_repository.rs payload helpers —
// the four shape-specific helpers must reproduce the Rust fallback
// semantics: empty array, empty object, null, opt-string.
func TestPayloadHelpersFallbacks(t *testing.T) {
	obj := map[string]json.RawMessage{
		"present": json.RawMessage(`{"k":1}`),
	}
	assert.Equal(t, json.RawMessage("[]"), payloadJSONOrEmptyArray(obj, "absent"))
	assert.JSONEq(t, `{"k":1}`, string(payloadJSONOrEmptyArray(obj, "present")))
	assert.Equal(t, json.RawMessage("{}"), payloadJSONOrEmptyObject(obj, "absent"))
	assert.Equal(t, json.RawMessage("null"), payloadJSONOrNull(obj, "absent"))

	// First-non-null helper picks the first present non-null field.
	obj2 := map[string]json.RawMessage{
		"join":        json.RawMessage("null"),
		"join_config": json.RawMessage(`{"x":1}`),
	}
	got := payloadFirstJSONNonNull(obj2, "join", "join_config")
	assert.JSONEq(t, `{"x":1}`, string(got))

	obj3 := map[string]json.RawMessage{}
	assert.Nil(t, payloadFirstJSONNonNull(obj3, "join", "join_config"))
}

// ---- object_set_repository -----------------------------------------------

// libs/ontology-kernel/src/domain/object_set_repository.rs
// `definition_to_record` round-trip: every metadata field surfaces
// on the resulting [DefinitionRecord], and the payload is the
// JSON-encoded ObjectSetDefinition itself.
func TestObjectSetDefinitionToRecord(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	owner := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	base := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	created := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	def := models.ObjectSetDefinition{
		ID:               id,
		Name:             "saved view",
		BaseObjectTypeID: base,
		OwnerID:          owner,
		CreatedAt:        created,
		UpdatedAt:        updated,
		Policy:           models.ObjectSetPolicy{},
	}

	rec, err := ObjectSetDefinitionToRecord(def)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.DefinitionKind("object_set"), rec.Kind)
	assert.Equal(t, storageabstraction.DefinitionId(id.String()), rec.ID)
	require.NotNil(t, rec.OwnerID)
	assert.Equal(t, owner.String(), *rec.OwnerID)
	require.NotNil(t, rec.ParentID)
	assert.Equal(t, storageabstraction.DefinitionId(base.String()), *rec.ParentID)
	require.NotNil(t, rec.Version)
	assert.Equal(t, uint64(updated.UnixMilli()), *rec.Version)
	require.NotNil(t, rec.CreatedAtMs)
	assert.Equal(t, created.UnixMilli(), *rec.CreatedAtMs)

	// Round-trip: from-record yields the same definition (modulo
	// JSON-stable defaults like AllowedMarkings: []string{}).
	back, err := ObjectSetDefinitionFromRecord(rec)
	require.NoError(t, err)
	assert.Equal(t, def.ID, back.ID)
	assert.Equal(t, def.OwnerID, back.OwnerID)
	assert.Equal(t, def.BaseObjectTypeID, back.BaseObjectTypeID)
	assert.Equal(t, def.Name, back.Name)
}

// libs/ontology-kernel/src/domain/object_set_repository.rs
// `definition_from_record` — accepts both `join` and `join_config`
// payload aliases.
func TestObjectSetDefinitionFromRecordJoinAliases(t *testing.T) {
	base := uuid.New()
	owner := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	mk := func(joinKey string) json.RawMessage {
		body, err := json.Marshal(map[string]any{
			"id":                  uuid.New(),
			"name":                "x",
			"description":         "",
			"base_object_type_id": base,
			"filters":             []any{},
			"traversals":          []any{},
			joinKey: map[string]any{
				"secondary_object_type_id": uuid.New(),
				"left_field":               "a",
				"right_field":              "b",
				"join_kind":                "inner",
			},
			"projections": []any{},
			"policy":      map[string]any{},
			"owner_id":    owner,
			"created_at":  now.Format(time.RFC3339),
			"updated_at":  now.Format(time.RFC3339),
		})
		require.NoError(t, err)
		return body
	}

	for _, joinKey := range []string{"join", "join_config"} {
		def, err := ObjectSetDefinitionFromRecord(storageabstraction.DefinitionRecord{
			Kind:    "object_set",
			Payload: mk(joinKey),
		})
		require.NoError(t, err)
		require.NotNil(t, def.Join, "join key %q should populate Join", joinKey)
		assert.Equal(t, "inner", def.Join.JoinKind)
	}
}

// libs/ontology-kernel/src/domain/object_set_repository.rs
// `pub async fn list` — when `include_restricted_views=false` the
// store-side filter pins owner_id; in-memory re-filter still applies
// so a row owned by a different owner gets dropped.
func TestListObjectSetsAppliesOwnerFilter(t *testing.T) {
	owner := uuid.New()
	other := uuid.New()
	base := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	mkRecord := func(ownerID uuid.UUID, restrictedViewID *uuid.UUID) storageabstraction.DefinitionRecord {
		def := models.ObjectSetDefinition{
			ID:               uuid.New(),
			BaseObjectTypeID: base,
			OwnerID:          ownerID,
			CreatedAt:        now,
			UpdatedAt:        now,
			Policy:           models.ObjectSetPolicy{RequiredRestrictedViewID: restrictedViewID},
		}
		rec, err := ObjectSetDefinitionToRecord(def)
		require.NoError(t, err)
		return rec
	}

	store := &fakeDefinitionStore{
		listResult: storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{
			Items: []storageabstraction.DefinitionRecord{
				mkRecord(owner, nil),
				mkRecord(other, nil), // non-owner, no restricted view → drops
			},
		},
	}

	page, err := ListObjectSets(context.Background(), store, ObjectSetListQuery{
		OwnerID: owner,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, owner, page.Items[0].OwnerID)

	// Filter passed to the store must pin owner_id when
	// include_restricted_views=false.
	require.NotNil(t, store.lastQuery)
	assert.Equal(t, owner.String(), store.lastQuery.Filters["owner_id"])

	// With include_restricted_views=true: owner_id filter dropped
	// from the store query AND non-owner rows with a restricted_view
	// leak through.
	view := uuid.New()
	store = &fakeDefinitionStore{
		listResult: storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{
			Items: []storageabstraction.DefinitionRecord{
				mkRecord(other, &view),
			},
		},
	}
	page, err = ListObjectSets(context.Background(), store, ObjectSetListQuery{
		OwnerID:                owner,
		IncludeRestrictedViews: true,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "restricted view leaks through")
	assert.NotContains(t, store.lastQuery.Filters, "owner_id")
}

// ---- action_repository ---------------------------------------------------

// libs/ontology-kernel/src/domain/action_repository.rs
// `pub fn action_to_record` + `pub fn row_from_record` — exercise
// the encode / decode pair on a populated ActionType.
func TestActionToRecordRoundTrip(t *testing.T) {
	id := uuid.New()
	owner := uuid.New()
	objType := uuid.New()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	action := models.ActionType{
		ID:                   id,
		Name:                 "approve",
		DisplayName:          "Approve",
		Description:          "Approve a request",
		ObjectTypeID:         objType,
		OperationKind:        "update_object",
		ConfirmationRequired: true,
		OwnerID:              owner,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	rec, err := ActionToRecord(action)
	require.NoError(t, err)
	assert.Equal(t, storageabstraction.DefinitionKind("action_type"), rec.Kind)
	require.NotNil(t, rec.ParentID)
	assert.Equal(t, storageabstraction.DefinitionId(objType.String()), *rec.ParentID)

	row, err := RowFromRecord(rec)
	require.NoError(t, err)
	assert.Equal(t, action.ID, row.ID)
	assert.Equal(t, action.Name, row.Name)
	assert.Equal(t, action.OperationKind, row.OperationKind)
	assert.True(t, row.ConfirmationRequired)
	assert.Equal(t, action.OwnerID, row.OwnerID)
}

// libs/ontology-kernel/src/domain/action_repository.rs
// `pub async fn list_action_rows` — surfaces ParentID = object_type_id
// to the DefinitionStore.
func TestListActionRowsForwardsObjectTypeFilter(t *testing.T) {
	objType := uuid.New()
	action := models.ActionType{
		ID:           uuid.New(),
		Name:         "x",
		ObjectTypeID: objType,
		OwnerID:      uuid.New(),
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	rec, err := ActionToRecord(action)
	require.NoError(t, err)

	store := &fakeDefinitionStore{
		listResult: storageabstraction.PagedResult[storageabstraction.DefinitionRecord]{
			Items: []storageabstraction.DefinitionRecord{rec},
		},
	}
	page, err := ListActionRows(context.Background(), store, ActionTypeListQuery{
		ObjectTypeID: &objType,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, objType, page.Items[0].ObjectTypeID)

	require.NotNil(t, store.lastQuery)
	require.NotNil(t, store.lastQuery.ParentID)
	assert.Equal(t, storageabstraction.DefinitionId(objType.String()), *store.lastQuery.ParentID)
}

// libs/ontology-kernel/src/domain/action_repository.rs
// `pub async fn delete_what_if_branch` — non-owner attempting delete
// when show_all=false returns false (without delete).
func TestDeleteWhatIfBranchOwnershipGuard(t *testing.T) {
	tenant := storageabstraction.TenantId("tenant")
	branchID := uuid.New()
	actionID := uuid.New()
	owner := uuid.New()
	other := uuid.New()

	branch := models.ActionWhatIfBranch{
		ID:        branchID,
		ActionID:  actionID,
		OwnerID:   owner,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	body, err := json.Marshal(branch)
	require.NoError(t, err)

	store := &fakeReadModelStore{
		getResult: &storageabstraction.ReadModelRecord{
			Kind:    storageabstraction.ReadModelKind(ActionWhatIfKind),
			Tenant:  tenant,
			ID:      whatIfReadModelID(branchID),
			Payload: body,
		},
	}

	deleted, err := DeleteWhatIfBranch(context.Background(), store, tenant, actionID, branchID, other, false)
	require.NoError(t, err)
	assert.False(t, deleted, "non-owner without show_all must not delete")
	assert.False(t, store.deleted, "Delete must not be invoked")

	// show_all=true bypasses the ownership check.
	deleted, err = DeleteWhatIfBranch(context.Background(), store, tenant, actionID, branchID, other, true)
	require.NoError(t, err)
	assert.True(t, deleted)
	assert.True(t, store.deleted)
}

// libs/ontology-kernel/src/domain/action_repository.rs
// `pub async fn list_what_if_branches` — owner_id filter is dropped
// when show_all=true; target_object_id pin is honoured.
func TestListWhatIfBranchesFilters(t *testing.T) {
	tenant := storageabstraction.TenantId("tenant")
	owner := uuid.New()
	target := uuid.New()
	store := &fakeReadModelStore{
		listResult: storageabstraction.PagedResult[storageabstraction.ReadModelRecord]{},
	}
	_, err := ListWhatIfBranches(context.Background(), store, WhatIfListQuery{
		Tenant:         tenant,
		ActionID:       uuid.New(),
		TargetObjectID: &target,
		OwnerID:        owner,
		ShowAll:        true,
	})
	require.NoError(t, err)
	require.NotNil(t, store.lastListQuery)
	assert.Equal(t, target.String(), store.lastListQuery.Filters["target_object_id"])
	assert.NotContains(t, store.lastListQuery.Filters, "owner_id")

	store = &fakeReadModelStore{}
	_, err = ListWhatIfBranches(context.Background(), store, WhatIfListQuery{
		Tenant:   tenant,
		ActionID: uuid.New(),
		OwnerID:  owner,
		ShowAll:  false,
	})
	require.NoError(t, err)
	assert.Equal(t, owner.String(), store.lastListQuery.Filters["owner_id"])
}

// libs/ontology-kernel/src/domain/action_repository.rs
// `pub async fn count_action_rows` — propagates object_type_id +
// search down into DefinitionStore::Count.
func TestCountActionRowsPropagatesQuery(t *testing.T) {
	store := &fakeDefinitionStore{countResult: 3}
	objType := uuid.New()
	search := "approve"
	n, err := CountActionRows(context.Background(), store, &objType, &search)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), n)
	require.NotNil(t, store.lastCountQuery)
	require.NotNil(t, store.lastCountQuery.Search)
	assert.Equal(t, search, *store.lastCountQuery.Search)
	require.NotNil(t, store.lastCountQuery.ParentID)
	assert.Equal(t, storageabstraction.DefinitionId(objType.String()), *store.lastCountQuery.ParentID)

	// Error from the store propagates verbatim.
	store.countErr = errors.New("backend offline")
	_, err = CountActionRows(context.Background(), store, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend offline")
}

// ---- test doubles ---------------------------------------------------------

type fakeDefinitionStore struct {
	getResult       *storageabstraction.DefinitionRecord
	getErr          error
	listResult      storageabstraction.PagedResult[storageabstraction.DefinitionRecord]
	listErr         error
	countResult     uint64
	countErr        error
	lastQuery       *storageabstraction.DefinitionQuery
	lastCountQuery  *storageabstraction.DefinitionQuery
}

func (s *fakeDefinitionStore) Get(_ context.Context, _ storageabstraction.DefinitionKind, _ storageabstraction.DefinitionId, _ storageabstraction.ReadConsistency) (*storageabstraction.DefinitionRecord, error) {
	return s.getResult, s.getErr
}
func (s *fakeDefinitionStore) List(_ context.Context, q storageabstraction.DefinitionQuery, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.DefinitionRecord], error) {
	saved := q
	s.lastQuery = &saved
	return s.listResult, s.listErr
}
func (s *fakeDefinitionStore) Put(_ context.Context, _ storageabstraction.DefinitionRecord, _ *uint64) (storageabstraction.PutOutcome, error) {
	return storageabstraction.PutOutcome{}, nil
}
func (s *fakeDefinitionStore) Delete(_ context.Context, _ storageabstraction.DefinitionKind, _ storageabstraction.DefinitionId) (bool, error) {
	return false, nil
}
func (s *fakeDefinitionStore) Count(_ context.Context, q storageabstraction.DefinitionQuery, _ storageabstraction.ReadConsistency) (uint64, error) {
	saved := q
	s.lastCountQuery = &saved
	return s.countResult, s.countErr
}

var _ storageabstraction.DefinitionStore = (*fakeDefinitionStore)(nil)

type fakeReadModelStore struct {
	getResult     *storageabstraction.ReadModelRecord
	getErr        error
	listResult    storageabstraction.PagedResult[storageabstraction.ReadModelRecord]
	listErr       error
	deleted       bool
	deleteResult  bool
	lastListQuery *storageabstraction.ReadModelQuery
}

func (s *fakeReadModelStore) Get(_ context.Context, _ storageabstraction.ReadModelKind, _ storageabstraction.TenantId, _ storageabstraction.ReadModelId, _ storageabstraction.ReadConsistency) (*storageabstraction.ReadModelRecord, error) {
	return s.getResult, s.getErr
}
func (s *fakeReadModelStore) List(_ context.Context, q storageabstraction.ReadModelQuery, _ storageabstraction.ReadConsistency) (storageabstraction.PagedResult[storageabstraction.ReadModelRecord], error) {
	saved := q
	s.lastListQuery = &saved
	return s.listResult, s.listErr
}
func (s *fakeReadModelStore) Put(_ context.Context, _ storageabstraction.ReadModelRecord) (storageabstraction.PutOutcome, error) {
	return storageabstraction.PutOutcome{}, nil
}
func (s *fakeReadModelStore) Delete(_ context.Context, _ storageabstraction.ReadModelKind, _ storageabstraction.TenantId, _ storageabstraction.ReadModelId) (bool, error) {
	s.deleted = true
	if s.deleteResult {
		return true, nil
	}
	return true, nil
}

var _ storageabstraction.ReadModelStore = (*fakeReadModelStore)(nil)
