package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// ActionLogObjectTypeID is the stable synthetic object type used to expose
// action submissions as queryable ontology objects. The ID is derived once
// from OpenFoundry's ontology namespace so every service sees the same type.
var ActionLogObjectTypeID = uuid.NewSHA1(domain.OntologyNamespace, []byte("ontology/action-log-record/object-type"))

const actionLogObjectTypeName = "ActionLogRecord"

type actionLogMaterializationInput struct {
	action          models.ActionType
	claims          *authmw.Claims
	targetObjectID  *uuid.UUID
	targetObjectIDs []uuid.UUID
	parameters      json.RawMessage
	preview         json.RawMessage
	validation      any
	edits           any
	status          string
	failureType     *string
	errorMessage    string
	justification   *string
	startedAt       time.Time
}

func materializeActionLogObject(ctx context.Context, state *ontologykernel.AppState, input actionLogMaterializationInput) error {
	if state == nil || state.Stores.Definitions == nil || state.Stores.Objects == nil || input.claims == nil {
		return nil
	}
	if err := ensureActionLogObjectType(ctx, state); err != nil {
		return err
	}

	appliedAt := time.Now().UTC()
	durationMs := int64(0)
	if !input.startedAt.IsZero() {
		durationMs = time.Since(input.startedAt).Milliseconds()
	}
	operationID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("create action log operation id: %w", err)
	}
	targetObjectIDs := actionLogTargetIDs(input.targetObjectID, input.targetObjectIDs)
	payload := map[string]any{
		"operation_id":          operationID.String(),
		"action_id":             input.action.ID.String(),
		"action_name":           input.action.Name,
		"action_display_name":   input.action.DisplayName,
		"action_object_type_id": input.action.ObjectTypeID.String(),
		"operation_kind":        input.action.OperationKind,
		"status":                input.status,
		"failure_type":          optionalFailureType(input.failureType),
		"error":                 optionalString(input.errorMessage),
		"applied_by":            input.claims.Sub.String(),
		"applied_by_email":      input.claims.Email,
		"applied_by_name":       input.claims.Name,
		"organization_id":       optionalOrgID(input.claims),
		"target_object_id":      firstTargetID(targetObjectIDs),
		"target_object_ids":     targetObjectIDs,
		"parameters":            jsonAsObjectOrValue(input.parameters),
		"validation":            coalesceActionLogValue(input.validation),
		"edits":                 coalesceActionLogValue(input.edits),
		"preview":               jsonAsAny(input.preview),
		"justification":         optionalStringPtr(input.justification),
		"duration_ms":           durationMs,
		"applied_at":            appliedAt.Format(time.RFC3339Nano),
		"summary":               actionLogSummary(input.action, input.status, targetObjectIDs, input.claims),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal action log object payload: %w", err)
	}
	tenant := domain.TenantFromClaims(input.claims)
	owner := storage.OwnerId(input.claims.Sub.String())
	_, err = state.Stores.Objects.Put(ctx, storage.Object{
		Tenant:         tenant,
		ID:             storage.ObjectId(operationID.String()),
		TypeID:         storage.TypeId(ActionLogObjectTypeID.String()),
		Payload:        body,
		OrganizationID: optionalOrgID(input.claims),
		CreatedAtMs:    ptrInt64(appliedAt.UnixMilli()),
		UpdatedAtMs:    appliedAt.UnixMilli(),
		Owner:          &owner,
		Markings:       []storage.MarkingId{storage.MarkingId("public")},
	}, nil)
	if err != nil {
		return fmt.Errorf("persist action log object: %w", err)
	}
	return nil
}

func ensureActionLogObjectType(ctx context.Context, state *ontologykernel.AppState) error {
	if state == nil || state.Stores.Definitions == nil {
		return nil
	}
	now := time.Now().UTC()
	primaryKey := "operation_id"
	title := "summary"
	icon := "history"
	color := "#2f6fed"
	plural := "Action Log Records"
	objectType := models.ObjectType{
		ID:                 ActionLogObjectTypeID,
		Name:               actionLogObjectTypeName,
		APIName:            actionLogObjectTypeName,
		DisplayName:        "Action Log Record",
		PluralDisplayName:  &plural,
		Description:        "Materialized ontology action submissions for Workshop and object set analysis.",
		PrimaryKeyProperty: &primaryKey,
		PrimaryKey:         primaryKey,
		TitleProperty:      &title,
		Icon:               &icon,
		Color:              &color,
		Status:             "active",
		Visibility:         "normal",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	payload, err := json.Marshal(objectType)
	if err != nil {
		return fmt.Errorf("marshal action log object type: %w", err)
	}
	created := now.UnixMilli()
	updated := created
	if _, err := state.Stores.Definitions.Put(ctx, storage.DefinitionRecord{
		Kind:        storage.DefinitionKind(domain.ActionRepoObjectKind),
		ID:          storage.DefinitionId(ActionLogObjectTypeID.String()),
		Payload:     payload,
		CreatedAtMs: &created,
		UpdatedAtMs: &updated,
	}, nil); err != nil {
		return fmt.Errorf("persist action log object type: %w", err)
	}
	parent := storage.DefinitionId(ActionLogObjectTypeID.String())
	for _, spec := range actionLogPropertySpecs() {
		propertyID := uuid.NewSHA1(ActionLogObjectTypeID, []byte(spec.name))
		property := models.Property{
			ID:               propertyID,
			ObjectTypeID:     ActionLogObjectTypeID,
			Name:             spec.name,
			DisplayName:      spec.displayName,
			Description:      spec.description,
			PropertyType:     spec.propertyType,
			Required:         spec.required,
			UniqueConstraint: spec.unique,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		models.EnrichPropertyMetadata(&property)
		propertyPayload, err := json.Marshal(property)
		if err != nil {
			return fmt.Errorf("marshal action log property %s: %w", spec.name, err)
		}
		if _, err := state.Stores.Definitions.Put(ctx, storage.DefinitionRecord{
			Kind:        storage.DefinitionKind(domain.ActionRepoPropertyKind),
			ID:          storage.DefinitionId(propertyID.String()),
			ParentID:    &parent,
			Payload:     propertyPayload,
			CreatedAtMs: &created,
			UpdatedAtMs: &updated,
		}, nil); err != nil {
			return fmt.Errorf("persist action log property %s: %w", spec.name, err)
		}
	}
	return nil
}

type actionLogPropertySpec struct {
	name         string
	displayName  string
	description  string
	propertyType string
	required     bool
	unique       bool
}

func actionLogPropertySpecs() []actionLogPropertySpec {
	return []actionLogPropertySpec{
		{name: "operation_id", displayName: "Operation ID", propertyType: "string", required: true, unique: true},
		{name: "action_id", displayName: "Action ID", propertyType: "string", required: true},
		{name: "action_name", displayName: "Action Name", propertyType: "string", required: true},
		{name: "action_display_name", displayName: "Action Display Name", propertyType: "string"},
		{name: "action_object_type_id", displayName: "Action Object Type ID", propertyType: "string"},
		{name: "operation_kind", displayName: "Operation Kind", propertyType: "string", required: true},
		{name: "status", displayName: "Status", propertyType: "string", required: true},
		{name: "failure_type", displayName: "Failure Type", propertyType: "string"},
		{name: "error", displayName: "Error", propertyType: "string"},
		{name: "applied_by", displayName: "Applied By", propertyType: "string", required: true},
		{name: "applied_by_email", displayName: "Applied By Email", propertyType: "string"},
		{name: "applied_by_name", displayName: "Applied By Name", propertyType: "string"},
		{name: "organization_id", displayName: "Organization ID", propertyType: "string"},
		{name: "target_object_id", displayName: "Target Object ID", propertyType: "string"},
		{name: "target_object_ids", displayName: "Target Object IDs", propertyType: "json"},
		{name: "parameters", displayName: "Parameters", propertyType: "json"},
		{name: "validation", displayName: "Validation", propertyType: "json"},
		{name: "edits", displayName: "Edits", propertyType: "json"},
		{name: "preview", displayName: "Preview", propertyType: "json"},
		{name: "justification", displayName: "Justification", propertyType: "string"},
		{name: "duration_ms", displayName: "Duration Ms", propertyType: "long"},
		{name: "applied_at", displayName: "Applied At", propertyType: "timestamp", required: true},
		{name: "summary", displayName: "Summary", propertyType: "string", required: true},
	}
}

func actionLogTargetIDs(targetObjectID *uuid.UUID, targetObjectIDs []uuid.UUID) []string {
	seen := map[string]struct{}{}
	out := []string{}
	appendID := func(id uuid.UUID) {
		if id == uuid.Nil {
			return
		}
		s := id.String()
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if targetObjectID != nil {
		appendID(*targetObjectID)
	}
	for _, id := range targetObjectIDs {
		appendID(id)
	}
	return out
}

func firstTargetID(targetObjectIDs []string) any {
	if len(targetObjectIDs) == 0 {
		return nil
	}
	return targetObjectIDs[0]
}

func optionalFailureType(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func optionalStringPtr(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func optionalOrgID(claims *authmw.Claims) *string {
	if claims == nil || claims.OrgID == nil {
		return nil
	}
	v := claims.OrgID.String()
	return &v
}

func jsonAsObjectOrValue(raw json.RawMessage) any {
	value := jsonAsAny(raw)
	if value == nil {
		return map[string]any{}
	}
	return value
}

func coalesceActionLogValue(value any) any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func ptrInt64(value int64) *int64 {
	return &value
}

func actionLogSummary(action models.ActionType, status string, targetObjectIDs []string, claims *authmw.Claims) string {
	actionName := action.DisplayName
	if actionName == "" {
		actionName = action.Name
	}
	actor := ""
	if claims != nil {
		actor = claims.Email
		if actor == "" {
			actor = claims.Sub.String()
		}
	}
	target := "no target"
	if len(targetObjectIDs) == 1 {
		target = targetObjectIDs[0]
	} else if len(targetObjectIDs) > 1 {
		target = fmt.Sprintf("%d targets", len(targetObjectIDs))
	}
	if actor == "" {
		return fmt.Sprintf("%s %s on %s", actionName, status, target)
	}
	return fmt.Sprintf("%s %s on %s by %s", actionName, status, target, actor)
}

func logActionLogMaterializationFailure(actionID uuid.UUID, err error) {
	if err == nil {
		return
	}
	log.Printf("ontology action log materialization failed action=%s err=%s", actionID, err.Error())
}
