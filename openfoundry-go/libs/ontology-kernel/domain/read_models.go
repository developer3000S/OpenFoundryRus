// Read-model helpers shared across kernel handlers. Mirrors the
// subset of `libs/ontology-kernel/src/domain/read_models.rs` needed
// by the storage handler today: tenant derivation + a converter
// from `Object` (storage) to `ObjectInstance` (handler view).
//
// `search_hit_to_object_instance`, `load_object_instance_from_read_model`
// and `list_accessible_objects_by_type` land alongside the search
// handler when its bounded context is ported (they need the
// SearchBackend interface that is not used by storage.rs).
package domain

import (
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// TenantFromClaims mirrors `pub fn tenant_from_claims`.
//
// Falls back to "default" when the claim has no `org_id` so the
// kernel never panics on smoke tokens. The tenant string round-trips
// 1:1 with the Rust impl — important because Cassandra row keys
// derive from this value.
func TenantFromClaims(claims *authmw.Claims) storage.TenantId {
	if claims == nil || claims.OrgID == nil {
		return storage.TenantId("default")
	}
	return storage.TenantId(claims.OrgID.String())
}

// ObjectStoreToObjectInstance mirrors
// `pub fn object_store_to_object_instance`.
//
// Returns nil when the underlying ID/type-id cannot be parsed as a
// UUID or when the `updated_at_ms` timestamp is out of range — same
// invariants the Rust `Option`-typed return enforces.
func ObjectStoreToObjectInstance(object storage.Object, fallbackOrgID *uuid.UUID) *ObjectInstance {
	id, err := uuid.Parse(string(object.ID))
	if err != nil {
		return nil
	}
	typeID, err := uuid.Parse(string(object.TypeID))
	if err != nil {
		return nil
	}
	timestamp := time.UnixMilli(object.UpdatedAtMs).UTC()

	createdBy := uuid.Nil
	if object.Owner != nil {
		if parsed, err := uuid.Parse(string(*object.Owner)); err == nil {
			createdBy = parsed
		}
	}

	marking := "public"
	if len(object.Markings) > 0 {
		marking = string(object.Markings[0])
	}

	return &ObjectInstance{
		ID:             id,
		ObjectTypeID:   typeID,
		Properties:     object.Payload,
		CreatedBy:      createdBy,
		OrganizationID: fallbackOrgID,
		Marking:        marking,
		CreatedAt:      timestamp,
		UpdatedAt:      timestamp,
	}
}
