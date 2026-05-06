// Repository boundary for saved object set definitions.
//
// Object set definitions are declarative control-plane metadata, while
// evaluation/runtime rows are loaded through search/read-model stores.
// This module keeps handlers free of inline SQL and talks only to
// storage-abstraction DefinitionStore.
//
// Mirrors `libs/ontology-kernel/src/domain/object_set_repository.rs`.

package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// Logical DefinitionStore kinds used by this repository.
const (
	ObjectSetKind        = "object_set"
	ObjectSetParentKind  = "object_type"
)

// ObjectSetListQuery mirrors `struct ObjectSetListQuery`.
type ObjectSetListQuery struct {
	OwnerID                uuid.UUID
	IncludeRestrictedViews bool
	Page                   storageabstraction.Page
}

func objectSetKind() storageabstraction.DefinitionKind {
	return storageabstraction.DefinitionKind(ObjectSetKind)
}

func objectTypeKind() storageabstraction.DefinitionKind {
	return storageabstraction.DefinitionKind(ObjectSetParentKind)
}

func definitionID(id uuid.UUID) storageabstraction.DefinitionId {
	return storageabstraction.DefinitionId(id.String())
}

// ObjectSetDefinitionToRecord mirrors `definition_to_record`.
//
// `version` is the `updated_at` epoch in milliseconds (Rust
// `definition.updated_at.timestamp_millis() as u64`). Negative
// timestamps round-trip cleanly because Go's `time.Time.UnixMilli()`
// returns int64 and we do an unchecked cast.
func ObjectSetDefinitionToRecord(definition models.ObjectSetDefinition) (storageabstraction.DefinitionRecord, error) {
	payload, err := json.Marshal(definition)
	if err != nil {
		return storageabstraction.DefinitionRecord{}, storageabstraction.Backendf("object set serialize failed: %s", err)
	}
	owner := definition.OwnerID.String()
	parent := definitionID(definition.BaseObjectTypeID)
	createdMs := definition.CreatedAt.UnixMilli()
	updatedMs := definition.UpdatedAt.UnixMilli()
	version := uint64(updatedMs)
	return storageabstraction.DefinitionRecord{
		Kind:        objectSetKind(),
		ID:          definitionID(definition.ID),
		OwnerID:     &owner,
		ParentID:    &parent,
		Version:     &version,
		Payload:     payload,
		CreatedAtMs: &createdMs,
		UpdatedAtMs: &updatedMs,
	}, nil
}

// ObjectSetDefinitionFromRecord mirrors `definition_from_record`. The
// payload is parsed as a top-level object and re-projected field by
// field so unknown keys are tolerated and the Rust `join` /
// `join_config` alias is honoured.
func ObjectSetDefinitionFromRecord(record storageabstraction.DefinitionRecord) (models.ObjectSetDefinition, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return models.ObjectSetDefinition{}, storageabstraction.Backendf("invalid object set payload: %s", err)
	}

	def := models.ObjectSetDefinition{}

	if err := readUUID(raw, "id", &def.ID); err != nil {
		return def, err
	}
	def.Name = optString(raw, "name")
	def.Description = optString(raw, "description")
	if err := readUUID(raw, "base_object_type_id", &def.BaseObjectTypeID); err != nil {
		return def, err
	}
	if err := readJSON(raw, "filters", &def.Filters); err != nil {
		return def, err
	}
	if err := readJSON(raw, "traversals", &def.Traversals); err != nil {
		return def, err
	}

	// Rust accepts both `join` and `join_config`. The first non-null
	// hit wins.
	join := pickFirst(raw, "join", "join_config")
	if len(join) > 0 && string(join) != "null" {
		var j models.ObjectSetJoin
		if err := json.Unmarshal(join, &j); err != nil {
			return def, storageabstraction.Backendf("invalid object set join_config: %s", err)
		}
		def.Join = &j
	}

	if err := readJSON(raw, "projections", &def.Projections); err != nil {
		return def, err
	}
	if v, ok := raw["what_if_label"]; ok && string(v) != "null" {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return def, storageabstraction.Backendf("invalid object set what_if_label: %s", err)
		}
		def.WhatIfLabel = &s
	}
	if err := readJSON(raw, "policy", &def.Policy); err != nil {
		return def, err
	}

	if v, ok := raw["materialized_snapshot"]; ok && string(v) != "null" {
		def.MaterializedSnapshot = v
	}
	if v, ok := raw["materialized_at"]; ok && string(v) != "null" {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return def, storageabstraction.Backendf("invalid object set materialized_at: %s", err)
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			t = t.UTC()
			def.MaterializedAt = &t
		}
	}
	// `materialized_row_count` int → Rust does i32::try_from(i64) — we
	// just decode straight into int32 for byte-compat with i32 columns.
	if v, ok := raw["materialized_row_count"]; ok && string(v) != "null" {
		var n int64
		if err := json.Unmarshal(v, &n); err == nil {
			if n > 2147483647 {
				n = 2147483647
			} else if n < -2147483648 {
				n = -2147483648
			}
			def.MaterializedRowCount = int32(n)
		}
	}
	if err := readUUID(raw, "owner_id", &def.OwnerID); err != nil {
		return def, err
	}
	if err := readRFC3339(raw, "created_at", &def.CreatedAt); err != nil {
		return def, err
	}
	if err := readRFC3339(raw, "updated_at", &def.UpdatedAt); err != nil {
		return def, err
	}
	return def, nil
}

// GetObjectSet mirrors `pub async fn get`. Returns nil when the
// store reports no record.
func GetObjectSet(ctx context.Context, store storageabstraction.DefinitionStore, id uuid.UUID) (*models.ObjectSetDefinition, error) {
	record, err := store.Get(ctx, objectSetKind(), definitionID(id), storageabstraction.Strong())
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	def, err := ObjectSetDefinitionFromRecord(*record)
	if err != nil {
		return nil, err
	}
	return &def, nil
}

// ListObjectSets mirrors `pub async fn list`. The Rust source filters
// at the store level on `owner_id` when restricted views are not
// requested, then re-filters in memory so views with a required
// restricted_view_id can leak through when requested.
func ListObjectSets(ctx context.Context, store storageabstraction.DefinitionStore, query ObjectSetListQuery) (storageabstraction.PagedResult[models.ObjectSetDefinition], error) {
	filters := map[string]string{}
	if !query.IncludeRestrictedViews {
		filters["owner_id"] = query.OwnerID.String()
	}
	page, err := store.List(ctx, storageabstraction.DefinitionQuery{
		Kind:    objectSetKind(),
		Filters: filters,
		Page:    query.Page,
	}, storageabstraction.Strong())
	if err != nil {
		return storageabstraction.PagedResult[models.ObjectSetDefinition]{}, err
	}

	items := make([]models.ObjectSetDefinition, 0, len(page.Items))
	for _, record := range page.Items {
		def, err := ObjectSetDefinitionFromRecord(record)
		if err != nil {
			return storageabstraction.PagedResult[models.ObjectSetDefinition]{}, err
		}
		isOwner := def.OwnerID == query.OwnerID
		isRestrictedView := query.IncludeRestrictedViews && def.Policy.RequiredRestrictedViewID != nil
		if isOwner || isRestrictedView {
			items = append(items, def)
		}
	}
	return storageabstraction.PagedResult[models.ObjectSetDefinition]{
		Items:     items,
		NextToken: page.NextToken,
	}, nil
}

// CreateObjectSet mirrors `pub async fn create`.
func CreateObjectSet(ctx context.Context, store storageabstraction.DefinitionStore, definition models.ObjectSetDefinition) (storageabstraction.PutOutcome, error) {
	record, err := ObjectSetDefinitionToRecord(definition)
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	return store.Put(ctx, record, nil)
}

// UpdateObjectSet mirrors `pub async fn update`. Same upsert path as
// create on the Rust side.
func UpdateObjectSet(ctx context.Context, store storageabstraction.DefinitionStore, definition models.ObjectSetDefinition) (storageabstraction.PutOutcome, error) {
	return CreateObjectSet(ctx, store, definition)
}

// DeleteObjectSet mirrors `pub async fn delete`.
func DeleteObjectSet(ctx context.Context, store storageabstraction.DefinitionStore, id uuid.UUID) (bool, error) {
	return store.Delete(ctx, objectSetKind(), definitionID(id))
}

// ObjectTypeExistsInDefinitionStore mirrors `pub async fn object_type_exists`.
func ObjectTypeExistsInDefinitionStore(ctx context.Context, store storageabstraction.DefinitionStore, objectTypeID uuid.UUID) (bool, error) {
	record, err := store.Get(ctx, objectTypeKind(), definitionID(objectTypeID), storageabstraction.Strong())
	if err != nil {
		return false, err
	}
	return record != nil, nil
}

// ---- raw-payload helpers --------------------------------------------------

func readUUID(raw map[string]json.RawMessage, field string, dst *uuid.UUID) error {
	v, ok := raw[field]
	if !ok || string(v) == "null" {
		return storageabstraction.Backendf("object set missing %s", field)
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return storageabstraction.Backendf("invalid object set %s: %s", field, err)
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return storageabstraction.Backendf("invalid object set %s: %s", field, err)
	}
	*dst = id
	return nil
}

func readJSON(raw map[string]json.RawMessage, field string, dst any) error {
	v, ok := raw[field]
	if !ok || string(v) == "null" {
		// Mirror Rust `payload.get(field).cloned().unwrap_or(Value::Null)` →
		// JSON null is decoded into the destination, leaving zero/nil on
		// non-Option types.
		return nil
	}
	if err := json.Unmarshal(v, dst); err != nil {
		return storageabstraction.Backendf("invalid object set %s: %s", field, err)
	}
	return nil
}

func readRFC3339(raw map[string]json.RawMessage, field string, dst *time.Time) error {
	v, ok := raw[field]
	if !ok || string(v) == "null" {
		return storageabstraction.Backendf("object set missing %s", field)
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return storageabstraction.Backendf("invalid object set %s: %s", field, err)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return storageabstraction.Backendf("invalid object set %s: %s", field, err)
	}
	*dst = t.UTC()
	return nil
}

func optString(raw map[string]json.RawMessage, field string) string {
	v, ok := raw[field]
	if !ok || string(v) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

func pickFirst(raw map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, k := range keys {
		if v, ok := raw[k]; ok {
			return v
		}
	}
	return nil
}

