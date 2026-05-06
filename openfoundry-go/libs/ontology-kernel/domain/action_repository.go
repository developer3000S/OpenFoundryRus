// Repository boundary for ontology action definitions and warm
// action views.
//
// Action definitions are declarative metadata; execution attempts,
// side effects and audit events live in
// [storageabstraction.ActionLogStore]. This module keeps HTTP
// handlers away from direct SQL while preserving the existing public
// action models.
//
// Mirrors `libs/ontology-kernel/src/domain/action_repository.rs`.

package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// Logical kinds used by this repository against DefinitionStore +
// ReadModelStore.
const (
	ActionTypeKind          = "action_type"
	ActionRepoObjectKind    = "object_type"
	ActionRepoPropertyKind  = "property"
	ActionRepoLinkTypeKind  = "link_type"
	ActionWhatIfKind        = "action_what_if_branch"
)

// ActionTypeListQuery mirrors `struct ActionTypeListQuery`.
type ActionTypeListQuery struct {
	ObjectTypeID *uuid.UUID
	Search       *string
	Page         storageabstraction.Page
}

// WhatIfListQuery mirrors `struct WhatIfListQuery`.
type WhatIfListQuery struct {
	Tenant         storageabstraction.TenantId
	ActionID       uuid.UUID
	TargetObjectID *uuid.UUID
	OwnerID        uuid.UUID
	ShowAll        bool
	Page           storageabstraction.Page
}

func actionDefKind(name string) storageabstraction.DefinitionKind {
	return storageabstraction.DefinitionKind(name)
}

func actionDefID(value uuid.UUID) storageabstraction.DefinitionId {
	return storageabstraction.DefinitionId(value.String())
}

func whatIfReadModelKind() storageabstraction.ReadModelKind {
	return storageabstraction.ReadModelKind(ActionWhatIfKind)
}

func whatIfReadModelID(value uuid.UUID) storageabstraction.ReadModelId {
	return storageabstraction.ReadModelId(value.String())
}

// ActionToRecord mirrors `pub fn action_to_record`. Encodes the
// action as a DefinitionRecord with the stable kind/id +
// owner/parent fields filled in.
func ActionToRecord(action models.ActionType) (storageabstraction.DefinitionRecord, error) {
	payload, err := json.Marshal(action)
	if err != nil {
		return storageabstraction.DefinitionRecord{}, storageabstraction.Backendf("action type serialize failed: %s", err)
	}
	owner := action.OwnerID.String()
	parent := actionDefID(action.ObjectTypeID)
	createdMs := action.CreatedAt.UnixMilli()
	updatedMs := action.UpdatedAt.UnixMilli()
	version := uint64(updatedMs)
	return storageabstraction.DefinitionRecord{
		Kind:        actionDefKind(ActionTypeKind),
		ID:          actionDefID(action.ID),
		OwnerID:     &owner,
		ParentID:    &parent,
		Version:     &version,
		Payload:     payload,
		CreatedAtMs: &createdMs,
		UpdatedAtMs: &updatedMs,
	}, nil
}

// RowFromRecord mirrors `pub fn row_from_record`. Re-projects the
// generic DefinitionRecord into the persisted-row shape used by the
// rest of the kernel.
func RowFromRecord(record storageabstraction.DefinitionRecord) (models.ActionTypeRow, error) {
	obj, err := payloadObject(record.Payload)
	if err != nil {
		return models.ActionTypeRow{}, err
	}
	row := models.ActionTypeRow{}

	id, err := payloadUUID(obj, "id")
	if err != nil {
		return row, err
	}
	row.ID = id
	row.Name = payloadOptString(obj, "name")
	row.DisplayName = payloadOptString(obj, "display_name")
	row.Description = payloadOptString(obj, "description")
	objectTypeID, err := payloadUUID(obj, "object_type_id")
	if err != nil {
		return row, err
	}
	row.ObjectTypeID = objectTypeID
	row.OperationKind = payloadOptString(obj, "operation_kind")
	row.InputSchema = payloadJSONOrEmptyArray(obj, "input_schema")
	row.FormSchema = payloadJSONOrEmptyObject(obj, "form_schema")
	row.Config = payloadJSONOrNull(obj, "config")
	row.ConfirmationRequired = payloadBool(obj, "confirmation_required")
	row.PermissionKey = payloadOptStringPtr(obj, "permission_key")
	row.AuthorizationPolicy = payloadJSONOrEmptyObject(obj, "authorization_policy")
	ownerID, err := payloadUUID(obj, "owner_id")
	if err != nil {
		return row, err
	}
	row.OwnerID = ownerID
	if err := payloadRFC3339(obj, "created_at", &row.CreatedAt); err != nil {
		return row, err
	}
	if err := payloadRFC3339(obj, "updated_at", &row.UpdatedAt); err != nil {
		return row, err
	}
	return row, nil
}

// GetActionRow mirrors `pub async fn get_action_row`.
func GetActionRow(ctx context.Context, store storageabstraction.DefinitionStore, actionID uuid.UUID) (*models.ActionTypeRow, error) {
	record, err := store.Get(ctx, actionDefKind(ActionTypeKind), actionDefID(actionID), storageabstraction.Strong())
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	row, err := RowFromRecord(*record)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListActionRows mirrors `pub async fn list_action_rows`. Forwards
// filters via `parent_id = object_type_id` + free-text search.
func ListActionRows(ctx context.Context, store storageabstraction.DefinitionStore, query ActionTypeListQuery) (storageabstraction.PagedResult[models.ActionTypeRow], error) {
	defQuery := storageabstraction.DefinitionQuery{
		Kind:   actionDefKind(ActionTypeKind),
		Search: query.Search,
		Page:   query.Page,
	}
	if query.ObjectTypeID != nil {
		parent := actionDefID(*query.ObjectTypeID)
		defQuery.ParentID = &parent
	}
	page, err := store.List(ctx, defQuery, storageabstraction.Strong())
	if err != nil {
		return storageabstraction.PagedResult[models.ActionTypeRow]{}, err
	}
	items := make([]models.ActionTypeRow, 0, len(page.Items))
	for _, record := range page.Items {
		row, err := RowFromRecord(record)
		if err != nil {
			return storageabstraction.PagedResult[models.ActionTypeRow]{}, err
		}
		items = append(items, row)
	}
	return storageabstraction.PagedResult[models.ActionTypeRow]{
		Items:     items,
		NextToken: page.NextToken,
	}, nil
}

// CountActionRows mirrors `pub async fn count_action_rows`.
func CountActionRows(ctx context.Context, store storageabstraction.DefinitionStore, objectTypeID *uuid.UUID, search *string) (uint64, error) {
	defQuery := storageabstraction.DefinitionQuery{
		Kind:   actionDefKind(ActionTypeKind),
		Search: search,
		Page:   storageabstraction.Page{Size: 1},
	}
	if objectTypeID != nil {
		parent := actionDefID(*objectTypeID)
		defQuery.ParentID = &parent
	}
	return store.Count(ctx, defQuery, storageabstraction.Strong())
}

// PutAction mirrors `pub async fn put_action`.
func PutAction(ctx context.Context, store storageabstraction.DefinitionStore, action models.ActionType) (storageabstraction.PutOutcome, error) {
	record, err := ActionToRecord(action)
	if err != nil {
		return storageabstraction.PutOutcome{}, err
	}
	return store.Put(ctx, record, nil)
}

// DeleteAction mirrors `pub async fn delete_action`.
func DeleteAction(ctx context.Context, store storageabstraction.DefinitionStore, actionID uuid.UUID) (bool, error) {
	return store.Delete(ctx, actionDefKind(ActionTypeKind), actionDefID(actionID))
}

// ActionRepoObjectTypeExists mirrors `pub async fn object_type_exists`.
// (Declared with the `ActionRepo` prefix so it doesn't collide with
// the same-name function exported by [object_set_repository.go] —
// Rust keeps them in separate modules; Go puts them in the same
// package.)
func ActionRepoObjectTypeExists(ctx context.Context, store storageabstraction.DefinitionStore, objectTypeID uuid.UUID) (bool, error) {
	record, err := store.Get(ctx, actionDefKind(ActionRepoObjectKind), actionDefID(objectTypeID), storageabstraction.Strong())
	if err != nil {
		return false, err
	}
	return record != nil, nil
}

// LoadPropertyForObjectTypeViaStore mirrors
// `pub async fn load_property_for_object_type` (the Rust source has a
// same-name function in `definition_queries.rs` that hits PG
// directly; the suffix disambiguates the two in this Go package).
func LoadPropertyForObjectTypeViaStore(ctx context.Context, store storageabstraction.DefinitionStore, objectTypeID, propertyID uuid.UUID) (*models.Property, error) {
	record, err := store.Get(ctx, actionDefKind(ActionRepoPropertyKind), actionDefID(propertyID), storageabstraction.Strong())
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	var property models.Property
	if err := json.Unmarshal(record.Payload, &property); err != nil {
		return nil, storageabstraction.Backendf("invalid property definition: %s", err)
	}
	if property.ObjectTypeID != objectTypeID {
		return nil, nil
	}
	return &property, nil
}

// LoadEffectivePropertiesViaStore mirrors
// `pub async fn load_effective_properties`. Returns the same
// EffectivePropertyDefinition shape produced by [LoadEffectiveProperties]
// in schema.go, but sourced from the DefinitionStore rather than PG.
//
// Suffix mirrors the disambiguation used elsewhere in this package
// when the same Rust name lives in multiple modules.
func LoadEffectivePropertiesViaStore(ctx context.Context, store storageabstraction.DefinitionStore, objectTypeID uuid.UUID) ([]EffectivePropertyDefinition, error) {
	parent := actionDefID(objectTypeID)
	page, err := store.List(ctx, storageabstraction.DefinitionQuery{
		Kind:     actionDefKind(ActionRepoPropertyKind),
		ParentID: &parent,
		Page:     storageabstraction.Page{Size: 1_000},
	}, storageabstraction.Strong())
	if err != nil {
		return nil, err
	}
	out := make([]EffectivePropertyDefinition, 0, len(page.Items))
	for _, record := range page.Items {
		var property models.Property
		if err := json.Unmarshal(record.Payload, &property); err != nil {
			return nil, storageabstraction.Backendf("invalid property definition: %s", err)
		}
		out = append(out, EffectivePropertyDefinition{
			Name:             property.Name,
			DisplayName:      property.DisplayName,
			Description:      property.Description,
			PropertyType:     property.PropertyType,
			Required:         property.Required,
			UniqueConstraint: property.UniqueConstraint,
			TimeDependent:    property.TimeDependent,
			DefaultValue:     property.DefaultValue,
			ValidationRules:  property.ValidationRules,
			Source:           "object_type",
		})
	}
	return out, nil
}

// LoadLinkTypeViaStore mirrors `pub async fn load_link_type`.
func LoadLinkTypeViaStore(ctx context.Context, store storageabstraction.DefinitionStore, linkTypeID uuid.UUID) (*models.LinkType, error) {
	record, err := store.Get(ctx, actionDefKind(ActionRepoLinkTypeKind), actionDefID(linkTypeID), storageabstraction.Strong())
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	var lt models.LinkType
	if err := json.Unmarshal(record.Payload, &lt); err != nil {
		return nil, storageabstraction.Backendf("invalid link type definition: %s", err)
	}
	return &lt, nil
}

// ActionHasInlineEditReferences mirrors
// `pub async fn action_has_inline_edit_references`. Pulls a 10k page
// of every property definition and scans its inline_edit_config for
// the action_type_id reference.
func ActionHasInlineEditReferences(ctx context.Context, store storageabstraction.DefinitionStore, actionID uuid.UUID) (bool, error) {
	page, err := store.List(ctx, storageabstraction.DefinitionQuery{
		Kind: actionDefKind(ActionRepoPropertyKind),
		Page: storageabstraction.Page{Size: 10_000},
	}, storageabstraction.Strong())
	if err != nil {
		return false, err
	}
	want := actionID.String()
	for _, record := range page.Items {
		obj, err := payloadObject(record.Payload)
		if err != nil {
			continue
		}
		raw, ok := obj["inline_edit_config"]
		if !ok || string(raw) == "null" {
			continue
		}
		var cfg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &cfg); err != nil {
			continue
		}
		idRaw, ok := cfg["action_type_id"]
		if !ok || string(idRaw) == "null" {
			continue
		}
		var s string
		if err := json.Unmarshal(idRaw, &s); err != nil {
			continue
		}
		if s == want {
			return true, nil
		}
	}
	return false, nil
}

// CreateWhatIfBranch mirrors `pub async fn create_what_if_branch`.
func CreateWhatIfBranch(ctx context.Context, store storageabstraction.ReadModelStore, tenant storageabstraction.TenantId, branch models.ActionWhatIfBranch) (models.ActionWhatIfBranch, error) {
	payload, err := json.Marshal(branch)
	if err != nil {
		return models.ActionWhatIfBranch{}, storageabstraction.Backendf("what-if branch serialize failed: %s", err)
	}
	parent := whatIfReadModelID(branch.ActionID)
	updated := branch.UpdatedAt.UnixMilli()
	if _, err := store.Put(ctx, storageabstraction.ReadModelRecord{
		Kind:        whatIfReadModelKind(),
		Tenant:      tenant,
		ID:          whatIfReadModelID(branch.ID),
		ParentID:    &parent,
		Payload:     payload,
		Version:     uint64(updated),
		UpdatedAtMs: updated,
	}); err != nil {
		return models.ActionWhatIfBranch{}, err
	}
	return branch, nil
}

// ListWhatIfBranches mirrors `pub async fn list_what_if_branches`.
func ListWhatIfBranches(ctx context.Context, store storageabstraction.ReadModelStore, query WhatIfListQuery) (storageabstraction.PagedResult[models.ActionWhatIfBranch], error) {
	filters := map[string]string{}
	if query.TargetObjectID != nil {
		filters["target_object_id"] = query.TargetObjectID.String()
	}
	if !query.ShowAll {
		filters["owner_id"] = query.OwnerID.String()
	}
	parent := whatIfReadModelID(query.ActionID)
	page, err := store.List(ctx, storageabstraction.ReadModelQuery{
		Kind:     whatIfReadModelKind(),
		Tenant:   query.Tenant,
		ParentID: &parent,
		Filters:  filters,
		Page:     query.Page,
	}, storageabstraction.Strong())
	if err != nil {
		return storageabstraction.PagedResult[models.ActionWhatIfBranch]{}, err
	}
	items := make([]models.ActionWhatIfBranch, 0, len(page.Items))
	for _, record := range page.Items {
		var branch models.ActionWhatIfBranch
		if err := json.Unmarshal(record.Payload, &branch); err != nil {
			return storageabstraction.PagedResult[models.ActionWhatIfBranch]{}, storageabstraction.Backendf("invalid what-if branch read model: %s", err)
		}
		items = append(items, branch)
	}
	return storageabstraction.PagedResult[models.ActionWhatIfBranch]{
		Items:     items,
		NextToken: page.NextToken,
	}, nil
}

// CountWhatIfBranches mirrors `pub async fn count_what_if_branches`.
// The Rust source delegates to list and counts the items — we do the
// same so behaviour stays identical.
func CountWhatIfBranches(ctx context.Context, store storageabstraction.ReadModelStore, query WhatIfListQuery) (uint64, error) {
	page, err := ListWhatIfBranches(ctx, store, query)
	if err != nil {
		return 0, err
	}
	return uint64(len(page.Items)), nil
}

// DeleteWhatIfBranch mirrors `pub async fn delete_what_if_branch`.
// Returns false (without deleting) when the branch resolves to a
// different action_id, or when the caller is not the owner and
// `show_all` is false.
func DeleteWhatIfBranch(ctx context.Context, store storageabstraction.ReadModelStore, tenant storageabstraction.TenantId, actionID, branchID, ownerID uuid.UUID, showAll bool) (bool, error) {
	kind := whatIfReadModelKind()
	record, err := store.Get(ctx, kind, tenant, whatIfReadModelID(branchID), storageabstraction.Strong())
	if err != nil {
		return false, err
	}
	if record == nil {
		return false, nil
	}
	var branch models.ActionWhatIfBranch
	if err := json.Unmarshal(record.Payload, &branch); err != nil {
		return false, storageabstraction.Backendf("invalid what-if branch read model: %s", err)
	}
	if branch.ActionID != actionID || (!showAll && branch.OwnerID != ownerID) {
		return false, nil
	}
	return store.Delete(ctx, kind, tenant, whatIfReadModelID(branchID))
}

// payloadRFC3339 is the action_repository-flavour of [readRFC3339]
// (object_set_repository.go) — it surfaces the verbatim Rust
// `definition payload missing X` / `invalid X: ...` messages.
func payloadRFC3339(payload map[string]json.RawMessage, field string, dst *time.Time) error {
	v, ok := payload[field]
	if !ok || string(v) == "null" {
		return storageabstraction.Backendf("definition payload missing %s", field)
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return storageabstraction.Backendf("invalid %s: %s", field, err)
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return storageabstraction.Backendf("invalid %s: %s", field, err)
	}
	*dst = t.UTC()
	return nil
}
