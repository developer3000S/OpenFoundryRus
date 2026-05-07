// Phase 5C — execute_inline_edit + execute_inline_edit_batch.
//
// Mirrors `pub async fn execute_inline_edit` and
// `pub async fn execute_inline_edit_batch` in
// `libs/ontology-kernel/src/handlers/actions.rs`.
//
// Inline edits let dashboards rewrite a single property on an object
// by re-using a configured update_object action. The endpoint resolves
// the action whose `property_mappings` covers the target property,
// builds an `ExecuteActionRequest` whose parameters mirror the edit
// shape, and runs it through the plan/execute substrate (writeback +
// audit fan-out shared with `ExecuteAction`).
//
// Bulk inline-edit (`execute_inline_edit_batch`) submits N edits per
// request; entries targeting the same `object_id` are rejected up
// front because Foundry forbids editing the same object twice within
// a single inline-edit batch (`Inline edits.md` "Invalid inline
// Actions").
package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
)

// ExecuteInlineEditHandler mirrors `pub async fn execute_inline_edit`.
func ExecuteInlineEditHandler(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		typeID, err := pathUUID(r, "type_id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		propertyID, err := pathUUID(r, "property_id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		objID, err := pathUUID(r, "obj_id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.ExecuteInlineEditRequest
		if err := decodeOptionalJSON(r, &body); err != nil {
			invalid(w, "invalid request body")
			return
		}

		property, err := domain.LoadPropertyForObjectTypeViaStore(r.Context(), state.Stores.Definitions, typeID, propertyID)
		if err != nil {
			dbError(w, "failed to load property: "+err.Error())
			return
		}
		if property == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		if property.InlineEditConfig == nil {
			invalid(w, "inline edit is not configured for this property")
			return
		}

		target, errs := loadAndAuthorizeTarget(r.Context(), state, claims, objID, typeID)
		if len(errs) > 0 {
			if allForbidden(errs) {
				forbidden(w, joinErrs(errs))
			} else {
				invalid(w, joinErrs(errs))
			}
			return
		}

		row, err := domain.GetActionRow(r.Context(), state.Stores.Definitions, property.InlineEditConfig.ActionTypeID)
		if err != nil {
			dbError(w, "failed to load inline edit action: "+err.Error())
			return
		}
		if row == nil {
			invalid(w, "configured inline edit action type was not found")
			return
		}
		action := row.IntoAction()
		if action.ObjectTypeID != typeID {
			invalid(w, "configured inline edit action type no longer belongs to this object type")
			return
		}

		params, err := buildInlineEditParameters(action, *property, target, *property.InlineEditConfig, body.Value)
		if err != nil {
			invalid(w, err.Error())
			return
		}
		executeLoadedAction(r.Context(), w, state, claims, action, models.ExecuteActionRequest{
			TargetObjectID: &objID,
			Parameters:     params,
			Justification:  body.Justification,
		})
	}
}

// ExecuteInlineEditBatchHandler mirrors
// `pub async fn execute_inline_edit_batch`.
func ExecuteInlineEditBatchHandler(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		typeID, err := pathUUID(r, "type_id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.ExecuteInlineEditBatchRequest
		if err := decodeOptionalJSON(r, &body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		if len(body.Edits) == 0 {
			invalid(w, "edits must not be empty")
			return
		}
		if len(body.Edits) > maxObjectsPerSubmission {
			scaleLimitResponse(w, fmt.Sprintf(
				"inline edit batch contains %d entries which exceeds the per-submission limit (%d max)",
				len(body.Edits), maxObjectsPerSubmission))
			return
		}
		for _, edit := range body.Edits {
			if estimateEditBytes(edit.Value) > maxEditBytes {
				scaleLimitResponse(w, fmt.Sprintf(
					"inline edit for object %s is %d bytes which exceeds the per-edit limit (%d bytes)",
					edit.ObjectID, estimateEditBytes(edit.Value), maxEditBytes))
				return
			}
		}
		seen := map[uuid.UUID]struct{}{}
		for _, edit := range body.Edits {
			if _, dup := seen[edit.ObjectID]; dup {
				invalid(w, fmt.Sprintf(
					"inline edit batch contains two edits targeting the same object %s (rejected; see Inline edits documentation)",
					edit.ObjectID))
				return
			}
			seen[edit.ObjectID] = struct{}{}
		}

		total := len(body.Edits)
		results := make([]json.RawMessage, 0, total)
		succeeded := 0

		for _, edit := range body.Edits {
			outcome := runSingleInlineEdit(r.Context(), state, claims, typeID, edit)
			if outcome.success {
				succeeded++
			}
			results = append(results, mustJSON(outcome.payload))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"total":     total,
			"succeeded": succeeded,
			"failed":    total - succeeded,
			"results":   results,
		})
	}
}

// inlineEditOutcome captures the per-entry result for the bulk path.
type inlineEditOutcome struct {
	success bool
	payload map[string]any
}

func runSingleInlineEdit(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	typeID uuid.UUID,
	edit models.ExecuteInlineEditBatchItem,
) inlineEditOutcome {
	property, err := domain.LoadPropertyForObjectTypeViaStore(ctx, state.Stores.Definitions, typeID, edit.PropertyID)
	if err != nil {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       "failed to load property: " + err.Error(),
		}}
	}
	if property == nil {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       "property not found",
		}}
	}
	if property.InlineEditConfig == nil {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       "inline edit not configured for this property",
		}}
	}

	target, errs := loadAndAuthorizeTarget(ctx, state, claims, edit.ObjectID, typeID)
	if len(errs) > 0 {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       joinErrs(errs),
		}}
	}

	row, err := domain.GetActionRow(ctx, state.Stores.Definitions, property.InlineEditConfig.ActionTypeID)
	if err != nil {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       "failed to load inline edit action: " + err.Error(),
		}}
	}
	if row == nil {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       "configured inline edit action type was not found",
		}}
	}
	action := row.IntoAction()
	if action.ObjectTypeID != typeID {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       "configured inline edit action no longer belongs to this object type",
		}}
	}
	params, err := buildInlineEditParameters(action, *property, target, *property.InlineEditConfig, edit.Value)
	if err != nil {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       err.Error(),
		}}
	}

	objID := edit.ObjectID
	req := models.ExecuteActionRequest{
		TargetObjectID: &objID,
		Parameters:     params,
		Justification:  edit.Justification,
	}
	if err := executeLoadedActionInline(ctx, state, claims, action, req); err != nil {
		return inlineEditOutcome{payload: map[string]any{
			"property_id": edit.PropertyID,
			"object_id":   edit.ObjectID,
			"status":      "failure",
			"error":       err.Error(),
		}}
	}
	return inlineEditOutcome{success: true, payload: map[string]any{
		"property_id": edit.PropertyID,
		"object_id":   edit.ObjectID,
		"status":      "success",
	}}
}

// buildInlineEditParameters mirrors `fn build_inline_edit_parameters`.
//
// Resolves the editable input slot (single mapping or
// `inline_edit_config.input_name`), copies the user-provided value
// in, and back-fills every other mapping from the target's current
// property values so the action sees a complete parameter envelope.
func buildInlineEditParameters(
	action models.ActionType,
	property models.Property,
	target *domain.ObjectInstance,
	inlineEditConfig models.PropertyInlineEditConfig,
	value json.RawMessage,
) (json.RawMessage, error) {
	editableInputName, err := resolveInlineEditInputName(action, property.Name, inlineEditConfig)
	if err != nil {
		return nil, err
	}
	var update models.UpdateObjectActionConfig
	if err := json.Unmarshal(operationConfigBytes(action.Config), &update); err != nil {
		return nil, fmt.Errorf("invalid inline edit action config: %w", err)
	}

	parameters := map[string]json.RawMessage{}
	parameters[editableInputName] = value

	var targetProps map[string]json.RawMessage
	if target != nil && len(target.Properties) > 0 {
		_ = json.Unmarshal(target.Properties, &targetProps)
	}

	for _, mapping := range update.PropertyMappings {
		if mapping.InputName == nil {
			continue
		}
		inputName := *mapping.InputName
		if inputName == editableInputName {
			continue
		}
		if _, exists := parameters[inputName]; exists {
			continue
		}
		if currentValue, ok := targetProps[mapping.PropertyName]; ok {
			parameters[inputName] = currentValue
		}
	}

	return json.Marshal(parameters)
}

// resolveInlineEditInputName mirrors `fn resolve_inline_edit_input_name`.
func resolveInlineEditInputName(
	action models.ActionType,
	propertyName string,
	inlineEditConfig models.PropertyInlineEditConfig,
) (string, error) {
	var update models.UpdateObjectActionConfig
	if err := json.Unmarshal(operationConfigBytes(action.Config), &update); err != nil {
		return "", fmt.Errorf("invalid inline edit action config: %w", err)
	}

	candidates := []string{}
	for _, mapping := range update.PropertyMappings {
		if mapping.PropertyName != propertyName || mapping.InputName == nil {
			continue
		}
		candidates = append(candidates, *mapping.InputName)
	}

	if inlineEditConfig.InputName != nil {
		want := *inlineEditConfig.InputName
		for _, candidate := range candidates {
			if candidate == want {
				return want, nil
			}
		}
		return "", fmt.Errorf(
			"inline edit action does not map property '%s' from input '%s'",
			propertyName, want)
	}

	uniq := map[string]struct{}{}
	for _, c := range candidates {
		uniq[c] = struct{}{}
	}
	switch len(uniq) {
	case 0:
		return "", fmt.Errorf(
			"inline edit action must map property '%s' from an input field", propertyName)
	case 1:
		for c := range uniq {
			return c, nil
		}
	}
	return "", fmt.Errorf(
		"inline edit action maps property '%s' from multiple input fields; configure inline_edit_config.input_name explicitly",
		propertyName)
}

// executeLoadedAction mirrors `async fn execute_loaded_action`. Drives
// the full cascade (writeback → plan → execute → audit + notifications
// + side-effects) for callers that already loaded the action row
// (inline-edit + batch fall-through). When the action is referenced as
// an inline edit, side-effect webhooks + notifications must remain
// disabled per the validator (`ensure_inline_edit_requirements_for_action`).
func executeLoadedAction(
	ctx context.Context,
	w http.ResponseWriter,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	body models.ExecuteActionRequest,
) {
	startedAt := time.Now()
	writeback, sideEffects, err := splitWebhookConfigs(action.Config)
	if err != nil {
		invalid(w, err.Error())
		return
	}
	if err := ensureActionActorPermission(claims, action); err != nil {
		if auditErr := emitActionAuditEvent(ctx, state, claims, action, nil,
			body.TargetObjectID, "denied", "medium", err.Error(),
			body.Justification, body.Parameters, nil, nil); auditErr != nil {
			logAuditFailure(action.ID, auditErr)
		}
		failureType := "authentication"
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		forbidden(w, err.Error())
		return
	}
	if err := ensureConfirmationJustification(action, body.Justification); err != nil {
		failureType := "invalid_parameter"
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		invalid(w, err.Error())
		return
	}

	params := body.Parameters
	if writeback != nil {
		if err := runWebhookWriteback(ctx, state, writeback, &params); err != nil {
			failureType := "invalid_parameter"
			_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
			dbError(w, "webhook writeback failed: "+err.Error())
			return
		}
	}

	plan, errs := planAction(ctx, state, claims, action, &models.ValidateActionRequest{
		TargetObjectID: body.TargetObjectID,
		Parameters:     params,
	})
	if len(errs) > 0 {
		status := http.StatusBadRequest
		auditStatus := "failure"
		if allForbidden(errs) {
			status = http.StatusForbidden
			auditStatus = "denied"
		}
		if auditErr := emitActionAuditEvent(ctx, state, claims, action, nil,
			body.TargetObjectID, auditStatus, "medium", "action validation failed",
			body.Justification, params, nil, map[string]any{"details": errs}); auditErr != nil {
			logAuditFailure(action.ID, auditErr)
		}
		failureType := "invalid_parameter"
		if status == http.StatusForbidden {
			failureType = "authentication"
		}
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		writeJSON(w, status, map[string]any{
			"error":   "action validation failed",
			"details": errs,
		})
		return
	}
	executed, err := executePlan(ctx, state, claims, action, plan)
	if err != nil {
		if auditErr := emitActionAuditEvent(ctx, state, claims, action, plan.target,
			body.TargetObjectID, "failure", "high", err.Error(),
			body.Justification, params, nil, nil); auditErr != nil {
			logAuditFailure(action.ID, auditErr)
		}
		failureType := classifyExecutePlanError(err).AsStr()
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		dbError(w, err.Error())
		return
	}
	auditResult := map[string]any{
		"deleted": executed.deleted,
		"object":  jsonAsAny(executed.object),
		"link":    jsonAsAny(executed.link),
		"result":  jsonAsAny(executed.result),
	}
	if auditErr := emitActionAuditEvent(ctx, state, claims, action, plan.target,
		executed.targetObjectID, "success", "low", "",
		body.Justification, params, executed.preview, auditResult); auditErr != nil {
		logAuditFailure(action.ID, auditErr)
	}
	if notifErr := emitActionNotifications(ctx, state, claims, action, plan.target,
		params, body.Justification, executed); notifErr != nil {
		logNotificationFailure(action.ID, notifErr)
	}
	runWebhookSideEffects(ctx, state, claims, claims.Sub, action.ID,
		executed.targetObjectID, sideEffects, params)
	_ = emitActionAttemptEvent(ctx, state, claims, action, executed.targetObjectID, params, "success", time.Since(startedAt).Milliseconds(), nil)
	writeJSON(w, http.StatusOK, models.ExecuteActionResponse{
		Action:         action,
		TargetObjectID: executed.targetObjectID,
		Deleted:        executed.deleted,
		Preview:        executed.preview,
		Object:         executed.object,
		Link:           executed.link,
		Result:         executed.result,
	})
}

// executeLoadedActionInline is the variant the bulk inline-edit loop
// uses. Skips writing to a ResponseWriter so the caller can collect
// per-entry results and emit a single batched response envelope.
func executeLoadedActionInline(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	body models.ExecuteActionRequest,
) error {
	startedAt := time.Now()
	writeback, sideEffects, err := splitWebhookConfigs(action.Config)
	if err != nil {
		return err
	}
	if err := ensureActionActorPermission(claims, action); err != nil {
		failureType := "authentication"
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		return err
	}
	if err := ensureConfirmationJustification(action, body.Justification); err != nil {
		failureType := "invalid_parameter"
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		return err
	}
	params := body.Parameters
	if writeback != nil {
		if err := runWebhookWriteback(ctx, state, writeback, &params); err != nil {
			failureType := "invalid_parameter"
			_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
			return fmt.Errorf("webhook writeback failed: %w", err)
		}
	}
	plan, errs := planAction(ctx, state, claims, action, &models.ValidateActionRequest{
		TargetObjectID: body.TargetObjectID,
		Parameters:     params,
	})
	if len(errs) > 0 {
		failureType := "invalid_parameter"
		if allForbidden(errs) {
			failureType = "authentication"
		}
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		return fmt.Errorf("%s", joinErrs(errs))
	}
	executed, err := executePlan(ctx, state, claims, action, plan)
	if err != nil {
		failureType := classifyExecutePlanError(err).AsStr()
		_ = emitActionAttemptEvent(ctx, state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
		return err
	}
	auditResult := map[string]any{
		"deleted": executed.deleted,
		"object":  jsonAsAny(executed.object),
		"link":    jsonAsAny(executed.link),
		"result":  jsonAsAny(executed.result),
	}
	if auditErr := emitActionAuditEvent(ctx, state, claims, action, plan.target,
		executed.targetObjectID, "success", "low", "",
		body.Justification, params, executed.preview, auditResult); auditErr != nil {
		logAuditFailure(action.ID, auditErr)
	}
	if notifErr := emitActionNotifications(ctx, state, claims, action, plan.target,
		params, body.Justification, executed); notifErr != nil {
		logNotificationFailure(action.ID, notifErr)
	}
	runWebhookSideEffects(ctx, state, claims, claims.Sub, action.ID,
		executed.targetObjectID, sideEffects, params)
	_ = emitActionAttemptEvent(ctx, state, claims, action, executed.targetObjectID, params, "success", time.Since(startedAt).Milliseconds(), nil)
	return nil
}

func joinErrs(errs []string) string {
	out := ""
	for i, e := range errs {
		if i > 0 {
			out += "; "
		}
		out += e
	}
	return out
}
