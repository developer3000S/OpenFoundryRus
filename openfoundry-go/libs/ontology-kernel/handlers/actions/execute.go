// validate + execute paths.
//
// Mirrors the core of `handlers/actions.rs::{validate_action,
// execute_action, plan_action, plan_preview, execute_plan}` for the
// Rust executable operation set: UpdateObject, DeleteObject,
// CreateLink, InvokeWebhook, InvokeFunction (HTTP + inline
// Python/sidecar), and the interface-typed action operations. Object
// updates route their primary writes through
// the same writeback substrate the rules + funnel handlers consume
// (`handlers/objects.ApplyObjectWrite`), so retries collapse on the
// shared deterministic event id.
//
// **What this file delivers (1:1)**:
//
//   - `planAction` for `update_object` and `delete_object`
//     operation kinds, including target loading + access-control
//     checks (claims-side admin / clearance / org_id) and
//     `UpdateObjectActionConfig` materialisation (property_mappings
//   - static_patch + optional input_name resolution).
//   - `planPreview` for the two kinds, byte-identical to the Rust
//     JSON output (`{"kind":"update_object","target_object_id":…,
//     "patch":…}` / `{"kind":"delete_object","target_object_id":…}`).
//   - `validateAction` endpoint — full envelope.
//   - `executeAction` endpoint — full path including the audit
//     event append on success.
//
// **What stays gated**:
//
//   - ExecuteActionBatch / ExecuteInlineEdit / ExecuteInlineEditBatch:
//     are implemented by the current execution and writeback paths.
package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/objects"
	ontologymetrics "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/metrics"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// ── ActionPlan tagged union ─────────────────────────────────────────

// actionPlanKind tags the variant carried by [actionPlan]. Mirrors
// the Rust `enum ActionPlan` discriminator.
type actionPlanKind int

const (
	planUpdateObject actionPlanKind = iota
	planDeleteObject
	planCreateLink
	planInvokeWebhook
	planInvokeFunction
	planCreateInterface
	planModifyInterface
	planDeleteInterface
	planCreateInterfaceLink
	planDeleteInterfaceLink
	planUnsupported
)

// httpInvocationConfig mirrors `struct HttpInvocationConfig`.
type httpInvocationConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

// actionPlan mirrors `enum ActionPlan`. The Rust enum carries
// per-variant payloads; the Go port uses one struct with optional
// fields keyed by `kind` so the call sites can switch cleanly.
type actionPlan struct {
	kind   actionPlanKind
	target *domain.ObjectInstance
	patch  map[string]json.RawMessage

	// CreateLink and interface-link fields.
	counterpart      *domain.ObjectInstance
	linkType         *models.LinkType
	linkProperties   json.RawMessage
	linkSourceObject uuid.UUID
	linkTargetObject uuid.UUID

	// Interface-typed operation fields. action.ObjectTypeID carries the
	// interface_id for single-interface operations; concreteObjectTypeID is
	// resolved from __object_type or __interface_ref.
	interfaceID          uuid.UUID
	concreteObjectTypeID uuid.UUID
	newObjectID          uuid.UUID

	// InvokeFunction (HTTP/inline) + InvokeWebhook fields.
	invocation     *httpInvocationConfig
	inlineFunction *domain.ResolvedInlineFunction
	payload        json.RawMessage
	parameters     map[string]json.RawMessage
	justification  *string
}

// ActionFunctionRuntime is the injectable inline-function runtime used
// by action execution. Production delegates to domain.ExecuteInlineFunction
// (which uses AppState.PythonRuntime for Python sidecar execution); tests
// can supply fakes without spawning a sidecar.
type ActionFunctionRuntime interface {
	ExecuteInlineFunction(ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims, action *models.ActionType, target *domain.ObjectInstance, parameters map[string]json.RawMessage, resolved *domain.ResolvedInlineFunction, justification *string) (json.RawMessage, error)
}

type defaultActionFunctionRuntime struct{}

func (defaultActionFunctionRuntime) ExecuteInlineFunction(ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims, action *models.ActionType, target *domain.ObjectInstance, parameters map[string]json.RawMessage, resolved *domain.ResolvedInlineFunction, justification *string) (json.RawMessage, error) {
	return domain.ExecuteInlineFunction(ctx, state, claims, action, target, parameters, resolved, justification)
}

// ── validate_action ─────────────────────────────────────────────────

// ValidateAction mirrors `pub async fn validate_action`.
func ValidateAction(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.ValidateActionRequest
		if err := decodeOptionalJSON(r, &body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		row, err := domain.GetActionRow(r.Context(), state.Stores.Definitions, id)
		if err != nil {
			dbError(w, "failed to load action type: "+err.Error())
			return
		}
		if row == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		action := row.IntoAction()
		if err := ensureActionActorPermission(claims, action); err != nil {
			forbidden(w, err.Error())
			return
		}
		plan, errs := planAction(r.Context(), state, claims, action, &body)
		if len(errs) > 0 {
			if allForbidden(errs) {
				forbidden(w, strings.Join(errs, "; "))
				return
			}
			writeJSON(w, http.StatusBadRequest, models.ValidateActionResponse{
				Valid:   false,
				Errors:  errs,
				Preview: json.RawMessage(`null`),
			})
			return
		}
		writeJSON(w, http.StatusOK, models.ValidateActionResponse{
			Valid:   true,
			Errors:  []string{},
			Preview: planPreview(plan),
		})
	}
}

// ── execute_action ──────────────────────────────────────────────────

// ExecuteAction mirrors `pub async fn execute_action`. Drives the full
// Rust cascade end-to-end:
//
//  1. Webhook writeback (synchronous, blocking) merges the response's
//     output_parameters into the action parameters.
//  2. Plan + execute via the shared substrate.
//  3. Structured audit POST to audit-service.
//  4. Notification fan-out to notification-service.
//  5. Webhook side-effect fan-out (best-effort).
func ExecuteAction(state *ontologykernel.AppState) http.HandlerFunc {
	return ExecuteActionWithRuntime(state, defaultActionFunctionRuntime{})
}

// ExecuteActionWithRuntime is the dependency-injected variant used by tests
// and by services that want to provide a custom function runtime.
func ExecuteActionWithRuntime(state *ontologykernel.AppState, fnRuntime ActionFunctionRuntime) http.HandlerFunc {
	if fnRuntime == nil {
		fnRuntime = defaultActionFunctionRuntime{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.ExecuteActionRequest
		if err := decodeOptionalJSON(r, &body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		row, err := domain.GetActionRow(r.Context(), state.Stores.Definitions, id)
		if err != nil {
			dbError(w, "failed to load action type: "+err.Error())
			return
		}
		if row == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		action := row.IntoAction()
		startedAt := time.Now()

		// Resolve the writeback + side-effect webhook configs once.
		writeback, sideEffects, err := splitWebhookConfigs(action.Config)
		if err != nil {
			invalid(w, err.Error())
			return
		}

		if err := ensureActionActorPermission(claims, action); err != nil {
			if auditErr := emitActionAuditEvent(r.Context(), state, claims, action, nil,
				body.TargetObjectID, "denied", "medium", err.Error(),
				body.Justification, body.Parameters, nil, nil); auditErr != nil {
				logAuditFailure(action.ID, auditErr)
			}
			failureType := "authentication"
			_ = emitActionAttemptEvent(r.Context(), state, claims, action, body.TargetObjectID, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
			forbidden(w, err.Error())
			return
		}
		if err := ensureConfirmationJustification(action, body.Justification); err != nil {
			failureType := "invalid_parameter"
			_ = emitActionAttemptEvent(r.Context(), state, claims, action, body.TargetObjectID, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
			invalid(w, err.Error())
			return
		}

		// 1. Webhook writeback before plan_action.
		params := body.Parameters
		if writeback != nil {
			if err := runWebhookWriteback(r.Context(), state, writeback, &params); err != nil {
				failureType := "invalid_parameter"
				_ = emitActionAttemptEvent(r.Context(), state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
				dbError(w, "webhook writeback failed: "+err.Error())
				return
			}
		}

		validationReq := models.ValidateActionRequest{
			TargetObjectID: body.TargetObjectID,
			Parameters:     params,
		}
		plan, errs := planAction(r.Context(), state, claims, action, &validationReq)
		if len(errs) > 0 {
			status := "failure"
			if allForbidden(errs) {
				status = "denied"
			}
			if auditErr := emitActionAuditEvent(r.Context(), state, claims, action, nil,
				body.TargetObjectID, status, "medium", strings.Join(errs, "; "),
				body.Justification, params, nil,
				map[string]any{"details": errs}); auditErr != nil {
				logAuditFailure(action.ID, auditErr)
			}
			failureType := "invalid_parameter"
			if status == "denied" {
				failureType = "authentication"
			}
			_ = emitActionAttemptEvent(r.Context(), state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
			httpStatus := http.StatusBadRequest
			if status == "denied" {
				httpStatus = http.StatusForbidden
			}
			writeJSON(w, httpStatus, map[string]any{
				"error":   "action validation failed",
				"details": errs,
			})
			return
		}

		plan.justification = body.Justification
		executed, err := executePlanWithRuntime(r.Context(), state, claims, action, plan, fnRuntime)
		if err != nil {
			if auditErr := emitActionAuditEvent(r.Context(), state, claims, action, plan.target,
				body.TargetObjectID, "failure", "high", err.Error(),
				body.Justification, params, nil, nil); auditErr != nil {
				logAuditFailure(action.ID, auditErr)
			}
			failureType := classifyExecutePlanError(err).AsStr()
			_ = emitActionAttemptEvent(r.Context(), state, claims, action, body.TargetObjectID, params, "failure", time.Since(startedAt).Milliseconds(), &failureType)
			if errors.Is(err, domain.ErrPythonRuntimeNotWired) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"error":  "python_runtime_not_wired",
					"detail": err.Error(),
				})
				return
			}
			if domain.IsVersionConflict(err) {
				writeJSON(w, http.StatusConflict, errBody(err.Error()))
				return
			}
			dbError(w, err.Error())
			return
		}

		// 3. Audit + 4. notifications + 5. side-effects (post-success).
		_ = emitActionAttemptEvent(r.Context(), state, claims, action, executed.targetObjectID, params, "success", time.Since(startedAt).Milliseconds(), nil)
		auditResult := map[string]any{
			"deleted": executed.deleted,
			"object":  jsonAsAny(executed.object),
			"link":    jsonAsAny(executed.link),
			"result":  jsonAsAny(executed.result),
		}
		if auditErr := emitActionAuditEvent(r.Context(), state, claims, action, plan.target,
			executed.targetObjectID, "success", "low", "",
			body.Justification, params, executed.preview, auditResult); auditErr != nil {
			logAuditFailure(action.ID, auditErr)
		}
		if notifErr := emitActionNotifications(r.Context(), state, claims, action, plan.target,
			params, body.Justification, executed); notifErr != nil {
			logNotificationFailure(action.ID, notifErr)
		}
		runWebhookSideEffects(r.Context(), state, claims, claims.Sub, action.ID,
			executed.targetObjectID, sideEffects, params)

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
}

// executedAction mirrors the Rust private struct. Carries the four
// optional response slots plus the bookkeeping the audit emitter
// needs.
type executedAction struct {
	targetObjectID *uuid.UUID
	deleted        bool
	preview        json.RawMessage
	object         json.RawMessage
	link           json.RawMessage
	result         json.RawMessage
	startedAt      time.Time
}

// executePlan mirrors `async fn execute_plan` for the two ported
// variants. Update routes through the writeback substrate; Delete
// uses ObjectStore.Delete directly (idempotent — empty-result
// deletes surface as "target object no longer exists").
func executePlan(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	plan actionPlan,
) (executedAction, error) {
	return executePlanWithRuntime(ctx, state, claims, action, plan, defaultActionFunctionRuntime{})
}

func executePlanWithRuntime(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	plan actionPlan,
	fnRuntime ActionFunctionRuntime,
) (executedAction, error) {
	if fnRuntime == nil {
		fnRuntime = defaultActionFunctionRuntime{}
	}
	preview := planPreview(plan)
	startedAt := time.Now().UTC()

	switch plan.kind {
	case planUpdateObject:
		updated, err := applyObjectPatchAction(ctx, state, claims, plan.target, plan.patch)
		if err != nil {
			return executedAction{}, err
		}
		body, _ := json.Marshal(updated)
		targetID := plan.target.ID
		return executedAction{
			targetObjectID: &targetID,
			preview:        preview,
			object:         body,
			startedAt:      startedAt,
		}, nil

	case planDeleteObject:
		if err := deleteObjectViaStore(ctx, state, claims, plan.target.ID); err != nil {
			return executedAction{}, err
		}
		targetID := plan.target.ID
		return executedAction{
			targetObjectID: &targetID,
			deleted:        true,
			preview:        preview,
			startedAt:      startedAt,
		}, nil

	case planCreateLink:
		link, err := persistLinkInstance(ctx, state, claims, plan.linkType,
			plan.linkSourceObject, plan.linkTargetObject, plan.linkProperties)
		if err != nil {
			return executedAction{}, err
		}
		body, _ := json.Marshal(link)
		targetID := plan.target.ID
		return executedAction{
			targetObjectID: &targetID,
			preview:        preview,
			link:           body,
			startedAt:      startedAt,
		}, nil

	case planCreateInterface:
		created, err := createInterfaceObjectAction(ctx, state, claims, action, plan)
		if err != nil {
			return executedAction{}, err
		}
		body, _ := json.Marshal(created)
		targetID := created.ID
		return executedAction{
			targetObjectID: &targetID,
			preview:        preview,
			object:         body,
			startedAt:      startedAt,
		}, nil

	case planModifyInterface:
		updated, err := applyObjectPatchAction(ctx, state, claims, plan.target, plan.patch)
		if err != nil {
			return executedAction{}, err
		}
		body, _ := json.Marshal(updated)
		targetID := plan.target.ID
		return executedAction{
			targetObjectID: &targetID,
			preview:        preview,
			object:         body,
			startedAt:      startedAt,
		}, nil

	case planDeleteInterface:
		if err := deleteObjectViaStore(ctx, state, claims, plan.target.ID); err != nil {
			return executedAction{}, err
		}
		targetID := plan.target.ID
		return executedAction{
			targetObjectID: &targetID,
			deleted:        true,
			preview:        preview,
			startedAt:      startedAt,
		}, nil

	case planCreateInterfaceLink:
		link, err := persistLinkInstance(ctx, state, claims, plan.linkType,
			plan.linkSourceObject, plan.linkTargetObject, plan.linkProperties)
		if err != nil {
			return executedAction{}, err
		}
		body, _ := json.Marshal(link)
		targetID := plan.linkSourceObject
		return executedAction{
			targetObjectID: &targetID,
			preview:        preview,
			link:           body,
			startedAt:      startedAt,
		}, nil

	case planDeleteInterfaceLink:
		deleted, err := state.Stores.Links.Delete(ctx, domain.TenantFromClaims(claims),
			storage.LinkTypeId(plan.linkType.ID.String()),
			storage.ObjectId(plan.linkSourceObject.String()),
			storage.ObjectId(plan.linkTargetObject.String()))
		if err != nil {
			return executedAction{}, fmt.Errorf("failed to execute delete_interface_link action: %w", err)
		}
		if !deleted {
			return executedAction{}, errors.New("interface link no longer exists")
		}
		targetID := plan.linkSourceObject
		return executedAction{
			targetObjectID: &targetID,
			deleted:        true,
			preview:        preview,
			startedAt:      startedAt,
		}, nil

	case planInvokeWebhook:
		if plan.invocation == nil {
			return executedAction{}, errors.New("missing HTTP invocation config")
		}
		result, err := invokeHTTPAction(ctx, state, plan.invocation, plan.payload)
		if err != nil {
			return executedAction{}, err
		}
		var targetID *uuid.UUID
		if plan.target != nil {
			id := plan.target.ID
			targetID = &id
		}
		return executedAction{
			targetObjectID: targetID,
			preview:        preview,
			result:         result,
			startedAt:      startedAt,
		}, nil

	case planInvokeFunction:
		if plan.invocation == nil {
			return executedAction{}, errors.New("missing HTTP invocation config")
		}
		var response json.RawMessage
		var err error
		if strings.HasPrefix(plan.invocation.URL, "inline://") {
			if plan.inlineFunction == nil {
				return executedAction{}, errors.New("missing inline function config")
			}
			actionCopy := action
			response, err = fnRuntime.ExecuteInlineFunction(ctx, state, claims, &actionCopy, plan.target, plan.parameters, plan.inlineFunction, plan.justification)
		} else {
			response, err = invokeHTTPAction(ctx, state, plan.invocation, plan.payload)
		}
		if err != nil {
			return executedAction{}, err
		}
		return applyFunctionEffects(ctx, state, claims, plan.target, preview, response, startedAt)
	}
	return executedAction{}, fmt.Errorf("unsupported operation_kind '%s'", action.OperationKind)
}

// functionLinkInstruction mirrors the Rust FunctionLinkInstruction that
// function responses can return under the `link` key.
type functionLinkInstruction struct {
	LinkTypeID     uuid.UUID       `json:"link_type_id"`
	TargetObjectID uuid.UUID       `json:"target_object_id"`
	SourceRole     string          `json:"source_role,omitempty"`
	Properties     json.RawMessage `json:"properties,omitempty"`
}

func applyFunctionEffects(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	target *domain.ObjectInstance,
	preview json.RawMessage,
	response json.RawMessage,
	startedAt time.Time,
) (executedAction, error) {
	result, objectPatch, linkInstruction, deleteObject, err := deriveFunctionEffects(response)
	if err != nil {
		return executedAction{}, fmt.Errorf("invalid function response: %w", err)
	}
	if target == nil {
		if objectPatch != nil || linkInstruction != nil || deleteObject {
			return executedAction{}, errors.New("function response requested ontology mutations but target_object_id was not provided")
		}
		if len(result) == 0 {
			result = response
		}
		return executedAction{preview: preview, result: normalizeJSONNull(result), startedAt: startedAt}, nil
	}

	var objectBody json.RawMessage
	if objectPatch != nil {
		var patchMap map[string]json.RawMessage
		if err := json.Unmarshal(objectPatch, &patchMap); err != nil {
			return executedAction{}, fmt.Errorf("object_patch must be a JSON object: %w", err)
		}
		updated, err := applyObjectPatchAction(ctx, state, claims, target, patchMap)
		if err != nil {
			return executedAction{}, err
		}
		objectBody, _ = json.Marshal(updated)
	}

	var linkBody json.RawMessage
	if linkInstruction != nil {
		link, err := createLinkFromInstruction(ctx, state, claims, target, linkInstruction)
		if err != nil {
			return executedAction{}, err
		}
		linkBody, _ = json.Marshal(link)
	}

	deleted := false
	if deleteObject {
		if err := deleteObjectViaStore(ctx, state, claims, target.ID); err != nil {
			return executedAction{}, fmt.Errorf("failed to delete object from function response via ObjectStore: %w", err)
		}
		deleted = true
	}

	targetID := target.ID
	if len(result) == 0 {
		result = response
	}
	return executedAction{
		targetObjectID: &targetID,
		deleted:        deleted,
		preview:        preview,
		object:         objectBody,
		link:           linkBody,
		result:         normalizeJSONNull(result),
		startedAt:      startedAt,
	}, nil
}

func normalizeJSONNull(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`null`)
	}
	return raw
}

// deriveFunctionEffects mirrors the Rust derive_function_effects helper. It
// preserves non-effect fields such as undo/revert/media metadata as the result
// when no explicit ontology mutation envelope is present.
func deriveFunctionEffects(response json.RawMessage) (json.RawMessage, json.RawMessage, *functionLinkInstruction, bool, error) {
	if len(response) == 0 {
		return json.RawMessage(`null`), nil, nil, false, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(response, &obj); err != nil {
		return response, nil, nil, false, nil
	}
	if obj == nil {
		return response, nil, nil, false, nil
	}

	var result json.RawMessage
	if raw, ok := obj["output"]; ok && string(raw) != "null" {
		result = raw
	}
	var objectPatch json.RawMessage
	if raw, ok := obj["object_patch"]; ok && string(raw) != "null" {
		var patchObj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &patchObj); err != nil || patchObj == nil {
			if err == nil {
				err = errors.New("object_patch must be a JSON object")
			}
			return nil, nil, nil, false, err
		}
		objectPatch = raw
	}
	var link *functionLinkInstruction
	if raw, ok := obj["link"]; ok && string(raw) != "null" {
		var decoded functionLinkInstruction
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, nil, nil, false, fmt.Errorf("invalid function link instruction: %w", err)
		}
		if decoded.SourceRole == "" {
			decoded.SourceRole = "source"
		}
		link = &decoded
	}
	deleteObject := false
	if raw, ok := obj["delete_object"]; ok {
		_ = json.Unmarshal(raw, &deleteObject)
	}
	if deleteObject && (objectPatch != nil || link != nil) {
		return nil, nil, nil, false, errors.New("function response cannot request delete_object together with object_patch or link")
	}
	if result == nil && objectPatch == nil && link == nil && !deleteObject {
		result = response
	}
	return result, objectPatch, link, deleteObject, nil
}

func createLinkFromInstruction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	target *domain.ObjectInstance,
	instruction *functionLinkInstruction,
) (domain.LinkInstance, error) {
	counterpart, err := objects.LoadObjectInstance(ctx, state, claims, instruction.TargetObjectID, storage.Strong())
	if err != nil {
		return domain.LinkInstance{}, fmt.Errorf("failed to load linked object: %w", err)
	}
	if counterpart == nil {
		return domain.LinkInstance{}, errors.New("linked object was not found")
	}
	if err := domain.EnsureObjectAccess(claims, counterpart); err != nil {
		return domain.LinkInstance{}, err
	}
	linkType, err := domain.LoadLinkTypeViaStore(ctx, state.Stores.Definitions, instruction.LinkTypeID)
	if err != nil {
		return domain.LinkInstance{}, fmt.Errorf("failed to load link type: %w", err)
	}
	if linkType == nil {
		return domain.LinkInstance{}, errors.New("configured link type was not found")
	}

	expectedTargetType := linkType.SourceTypeID
	if instruction.SourceRole != "source" {
		expectedTargetType = linkType.TargetTypeID
	}
	if target.ObjectTypeID != expectedTargetType {
		return domain.LinkInstance{}, errors.New("target object does not match configured link endpoint")
	}

	sourceObjectID := target.ID
	targetObjectID := counterpart.ID
	expectedCounterpartType := linkType.TargetTypeID
	if instruction.SourceRole != "source" {
		sourceObjectID = counterpart.ID
		targetObjectID = target.ID
		expectedCounterpartType = linkType.SourceTypeID
	}
	if counterpart.ObjectTypeID != expectedCounterpartType {
		return domain.LinkInstance{}, errors.New("linked object does not match configured link type")
	}
	return persistLinkInstance(ctx, state, claims, linkType, sourceObjectID, targetObjectID, instruction.Properties)
}

// persistLinkInstance mirrors `async fn persist_link_instance`.
// Routes through `domain.CreateLink` (the composition substrate ported
// alongside this phase) and returns the LinkInstance the executor
// echoes back to the client.
func persistLinkInstance(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	linkType *models.LinkType,
	sourceObjectID, targetObjectID uuid.UUID,
	properties json.RawMessage,
) (domain.LinkInstance, error) {
	createdAt := time.Now().UTC()
	payload := properties
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	tenant := domain.TenantFromClaims(claims)
	linkTypeID := storage.LinkTypeId(linkType.ID.String())
	from := storage.ObjectId(sourceObjectID.String())
	to := storage.ObjectId(targetObjectID.String())

	if _, err := domain.CreateLink(ctx, state.Stores.Links, tenant, linkTypeID,
		from, to, payload, createdAt.UnixMilli()); err != nil {
		return domain.LinkInstance{}, fmt.Errorf("failed to execute create_link action: %w", err)
	}

	return domain.LinkInstance{
		ID:             domain.StableLinkID(linkTypeID, from, to),
		LinkTypeID:     linkType.ID,
		SourceObjectID: sourceObjectID,
		TargetObjectID: targetObjectID,
		Properties:     properties,
		CreatedBy:      claims.Sub,
		CreatedAt:      createdAt,
	}, nil
}

// invokeHTTPAction mirrors `async fn invoke_http_action`. POSTs the
// payload to the configured URL with the supplied method + headers,
// surfaces non-2xx as a typed error so the executor can map it to
// HTTP 500 (matching Rust). Empty bodies decode to JSON null.
func invokeHTTPAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	invocation *httpInvocationConfig,
	payload json.RawMessage,
) (json.RawMessage, error) {
	body := payload
	if len(body) == 0 || string(body) == "null" {
		body = json.RawMessage(`{}`)
	}
	req, err := http.NewRequestWithContext(ctx, invocation.Method, invocation.URL,
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to build HTTP action request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	for k, v := range invocation.Headers {
		req.Header.Set(k, v)
	}
	client := state.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP action request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes := readAllLimited(resp.Body, 16<<20)
	if resp.StatusCode/100 != 2 {
		detail := string(respBytes)
		if strings.TrimSpace(detail) == "" {
			detail = resp.Status
		}
		return nil, fmt.Errorf("HTTP action returned %d: %s", resp.StatusCode, detail)
	}
	if len(strings.TrimSpace(string(respBytes))) == 0 {
		return json.RawMessage(`null`), nil
	}
	// Try to decode as JSON; fall back to a JSON-quoted string.
	var v any
	if err := json.Unmarshal(respBytes, &v); err != nil {
		quoted, _ := json.Marshal(string(respBytes))
		return quoted, nil
	}
	out, _ := json.Marshal(v)
	return out, nil
}

// readAllLimited reads up to `limit` bytes from r without pulling in
// io.ReadAll's unbounded variant.
func readAllLimited(r interface{ Read([]byte) (int, error) }, limit int64) []byte {
	var buf bytes.Buffer
	chunk := make([]byte, 32*1024)
	for int64(buf.Len()) < limit {
		n, err := r.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
		}
		if err != nil {
			break
		}
	}
	return buf.Bytes()
}

// applyObjectPatchAction mirrors `async fn apply_object_patch`.
// Validates the patch values against the property schema, merges
// into the current object and routes through the writeback path.
func applyObjectPatchAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	target *domain.ObjectInstance,
	patch map[string]json.RawMessage,
) (*domain.ObjectInstance, error) {
	defs, err := loadEffectivePropertiesForAction(ctx, state, target.ObjectTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load property definitions: %w", err)
	}
	typeByName := map[string]string{}
	for _, p := range defs {
		typeByName[p.Name] = p.PropertyType
	}

	repoObject, err := objects.LoadRepoObjectFromStore(ctx, state, claims, target.ID, storage.Strong())
	if err != nil {
		return nil, fmt.Errorf("failed to load current object version: %w", err)
	}
	if repoObject == nil {
		return nil, errors.New("target object no longer exists")
	}

	merged := map[string]json.RawMessage{}
	if len(repoObject.Payload) > 0 {
		_ = json.Unmarshal(repoObject.Payload, &merged)
	}
	for name, value := range patch {
		propertyType, ok := typeByName[name]
		if !ok {
			return nil, fmt.Errorf("unknown property '%s' in patch", name)
		}
		if err := domain.ValidatePropertyValue(propertyType, value); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		merged[name] = value
	}
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode merged patch: %w", err)
	}
	normalized, err := domain.ValidateObjectProperties(defs, mergedJSON)
	if err != nil {
		return nil, fmt.Errorf("invalid action patch: %w", err)
	}

	updated := &domain.ObjectInstance{
		ID:             target.ID,
		ObjectTypeID:   target.ObjectTypeID,
		Properties:     normalized,
		CreatedBy:      target.CreatedBy,
		OrganizationID: target.OrganizationID,
		Marking:        target.Marking,
		CreatedAt:      target.CreatedAt,
		UpdatedAt:      time.Now().UTC(),
	}
	expected := repoObject.Version
	extra, _ := json.Marshal(map[string]any{"source": "ontology_action"})
	outcome, err := applyObjectWriteForAction(ctx, state, claims, updated, &expected, "update", extra)
	if err != nil {
		return nil, err
	}
	if err := objects.AppendObjectRevision(ctx, state, claims, updated, "update",
		int64(outcome.CommittedVersion), nil); err != nil {
		return nil, err
	}
	return updated, nil
}

func loadEffectivePropertiesForAction(ctx context.Context, state *ontologykernel.AppState, objectTypeID uuid.UUID) ([]domain.EffectivePropertyDefinition, error) {
	if state.DB != nil {
		return domain.LoadEffectiveProperties(ctx, state.DB, objectTypeID)
	}
	direct, err := domain.LoadEffectivePropertiesViaStore(ctx, state.Stores.Definitions, objectTypeID)
	if err != nil {
		return nil, err
	}
	byName := map[string]domain.EffectivePropertyDefinition{}
	for _, p := range direct {
		byName[p.Name] = p
	}
	bindings, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{Kind: storage.DefinitionKind("object_type_interface"), Page: storage.Page{Size: 10_000}}, storage.Strong())
	if err != nil {
		return nil, err
	}
	for _, rec := range bindings.Items {
		var binding models.ObjectTypeInterfaceBinding
		if err := json.Unmarshal(rec.Payload, &binding); err != nil || binding.ObjectTypeID != objectTypeID {
			continue
		}
		parent := storage.DefinitionId(binding.InterfaceID.String())
		props, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{Kind: storage.DefinitionKind("interface_property"), ParentID: &parent, Page: storage.Page{Size: 10_000}}, storage.Strong())
		if err != nil {
			return nil, err
		}
		for _, propRecord := range props.Items {
			var p models.InterfaceProperty
			if err := json.Unmarshal(propRecord.Payload, &p); err != nil {
				continue
			}
			if _, directWins := byName[p.Name]; directWins {
				continue
			}
			byName[p.Name] = domain.EffectivePropertyDefinition{
				Name:             p.Name,
				DisplayName:      p.DisplayName,
				Description:      p.Description,
				PropertyType:     p.PropertyType,
				Required:         p.Required,
				UniqueConstraint: p.UniqueConstraint,
				TimeDependent:    p.TimeDependent,
				DefaultValue:     p.DefaultValue,
				ValidationRules:  p.ValidationRules,
				Source:           "interface",
			}
		}
	}
	out := make([]domain.EffectivePropertyDefinition, 0, len(byName))
	for _, p := range byName {
		out = append(out, p)
	}
	return out, nil
}

func createInterfaceObjectAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	plan actionPlan,
) (*domain.ObjectInstance, error) {
	defs, err := loadEffectivePropertiesForAction(ctx, state, plan.concreteObjectTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load property definitions: %w", err)
	}
	patchJSON, err := json.Marshal(plan.patch)
	if err != nil {
		return nil, fmt.Errorf("encode interface create properties: %w", err)
	}
	normalized, err := domain.ValidateObjectProperties(defs, patchJSON)
	if err != nil {
		return nil, fmt.Errorf("invalid interface create properties: %w", err)
	}
	now := time.Now().UTC()
	obj := &domain.ObjectInstance{
		ID:             plan.newObjectID,
		ObjectTypeID:   plan.concreteObjectTypeID,
		Properties:     normalized,
		CreatedBy:      claims.Sub,
		OrganizationID: claims.OrgID,
		Marking:        "public",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	extra, _ := json.Marshal(map[string]any{"source": "ontology_action", "action_id": action.ID.String(), "interface_id": plan.interfaceID.String()})
	outcome, err := applyObjectWriteForAction(ctx, state, claims, obj, nil, "create", extra)
	if err != nil {
		return nil, err
	}
	if err := objects.AppendObjectRevision(ctx, state, claims, obj, "create", int64(outcome.CommittedVersion), nil); err != nil {
		return nil, err
	}
	return obj, nil
}

func applyObjectWriteForAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	object *domain.ObjectInstance,
	expectedVersion *uint64,
	operation string,
	extra json.RawMessage,
) (domain.WritebackOutcome, error) {
	if state.DB != nil {
		return objects.ApplyObjectWrite(ctx, state, claims, object, expectedVersion, operation, extra)
	}
	targetVersion := uint64(1)
	if expectedVersion != nil {
		targetVersion = *expectedVersion + 1
	}
	tenant := domain.TenantFromClaims(claims)
	repoObject := objects.InstanceToRepoObject(tenant, object, targetVersion, object.Properties, object.Marking)
	outcome, err := state.Stores.Objects.Put(ctx, repoObject, expectedVersion)
	if err != nil {
		return domain.WritebackOutcome{}, err
	}
	switch outcome.Kind {
	case storage.PutInserted:
		return domain.WritebackOutcome{CommittedVersion: 1, Created: true}, nil
	case storage.PutUpdated:
		return domain.WritebackOutcome{CommittedVersion: outcome.NewVersion}, nil
	case storage.PutVersionConflict:
		return domain.WritebackOutcome{}, &domain.WritebackError{Kind: domain.WritebackVersionConflict, ExpectedVersion: outcome.ExpectedVersion, ActualVersion: outcome.ActualVersion}
	default:
		return domain.WritebackOutcome{CommittedVersion: targetVersion}, nil
	}
}

// deleteObjectViaStore mirrors `async fn delete_object_via_store`.
func deleteObjectViaStore(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	objectID uuid.UUID,
) error {
	deleted, err := state.Stores.Objects.Delete(ctx,
		domain.TenantFromClaims(claims), storage.ObjectId(objectID.String()))
	if err != nil {
		return fmt.Errorf("failed to execute delete_object action via ObjectStore: %w", err)
	}
	if !deleted {
		return errors.New("target object no longer exists")
	}
	return nil
}

// ── plan_action ─────────────────────────────────────────────────────

// planAction mirrors `async fn plan_action` for UpdateObject + DeleteObject.
//
// The Rust impl returns `Result<ActionPlan, Vec<String>>` — Go uses
// `(actionPlan, []string)` where a non-empty error slice signals
// failure (mirrors the Rust `Vec<String>` shape so the
// `all_forbidden` partition remains identical).
func planAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	operationKind, err := parseOperationKind(action.OperationKind)
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}

	switch operationKind {
	case "update_object":
		if req.TargetObjectID == nil {
			return actionPlan{}, []string{"target_object_id is required for update_object actions"}
		}
		target, errs := loadAndAuthorizeTarget(ctx, state, claims, *req.TargetObjectID, action.ObjectTypeID)
		if len(errs) > 0 {
			return actionPlan{}, errs
		}
		if err := ensureActionTargetPermission(action, target); err != nil {
			return actionPlan{}, []string{err.Error()}
		}
		patch, errs := buildUpdateObjectPatch(ctx, state, action, req.Parameters)
		if len(errs) > 0 {
			return actionPlan{}, errs
		}
		return actionPlan{kind: planUpdateObject, target: target, patch: patch}, nil

	case "delete_object":
		if req.TargetObjectID == nil {
			return actionPlan{}, []string{"target_object_id is required for delete_object actions"}
		}
		target, errs := loadAndAuthorizeTarget(ctx, state, claims, *req.TargetObjectID, action.ObjectTypeID)
		if len(errs) > 0 {
			return actionPlan{}, errs
		}
		if err := ensureActionTargetPermission(action, target); err != nil {
			return actionPlan{}, []string{err.Error()}
		}
		return actionPlan{kind: planDeleteObject, target: target}, nil

	case "create_link":
		return planCreateLinkAction(ctx, state, claims, action, req)

	case "invoke_webhook":
		return planInvokeWebhookAction(ctx, state, claims, action, req)

	case "invoke_function":
		return planInvokeFunctionAction(ctx, state, claims, action, req)

	case "create_interface":
		return planCreateInterfaceAction(ctx, state, claims, action, req)

	case "modify_interface":
		return planModifyInterfaceAction(ctx, state, claims, action, req)

	case "delete_interface":
		return planDeleteInterfaceAction(ctx, state, claims, action, req)

	case "create_interface_link":
		return planCreateInterfaceLinkAction(ctx, state, claims, action, req)

	case "delete_interface_link":
		return planDeleteInterfaceLinkAction(ctx, state, claims, action, req)
	}
	return actionPlan{}, []string{"unsupported operation_kind '" + operationKind + "'"}
}

// planCreateLinkAction mirrors the Rust `ActionOperationKind::CreateLink`
// branch of `plan_action`.
func planCreateLinkAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	if req.TargetObjectID == nil {
		return actionPlan{}, []string{"target_object_id is required for create_link actions"}
	}
	target, errs := loadAndAuthorizeTarget(ctx, state, claims, *req.TargetObjectID, action.ObjectTypeID)
	if len(errs) > 0 {
		return actionPlan{}, errs
	}
	if err := ensureActionTargetPermission(action, target); err != nil {
		return actionPlan{}, []string{err.Error()}
	}

	var cfg createLinkActionConfig
	if err := json.Unmarshal(operationConfigBytes(action.Config), &cfg); err != nil {
		return actionPlan{}, []string{"invalid action config: " + err.Error()}
	}
	if cfg.SourceRole == "" {
		cfg.SourceRole = "source"
	}

	params := decodeParams(req.Parameters)
	counterpartID, err := resolveUUIDParameter(params, cfg.TargetInputName)
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	counterpart, err := objects.LoadObjectInstance(ctx, state, claims, counterpartID, storage.Strong())
	if err != nil {
		return actionPlan{}, []string{"failed to load linked object: " + err.Error()}
	}
	if counterpart == nil {
		return actionPlan{}, []string{"linked object was not found"}
	}
	if err := domain.EnsureObjectAccess(claims, counterpart); err != nil {
		return actionPlan{}, []string{forbiddenLine(err.Error())}
	}

	linkType, err := domain.LoadLinkTypeViaStore(ctx, state.Stores.Definitions, cfg.LinkTypeID)
	if err != nil {
		return actionPlan{}, []string{"failed to load link type: " + err.Error()}
	}
	if linkType == nil {
		return actionPlan{}, []string{"configured link type was not found"}
	}

	var sourceID, targetLinkID uuid.UUID
	var expectedCounterpartType uuid.UUID
	if cfg.SourceRole == "source" {
		sourceID = target.ID
		targetLinkID = counterpart.ID
		expectedCounterpartType = linkType.TargetTypeID
	} else {
		sourceID = counterpart.ID
		targetLinkID = target.ID
		expectedCounterpartType = linkType.SourceTypeID
	}
	if counterpart.ObjectTypeID != expectedCounterpartType {
		return actionPlan{}, []string{"linked object does not match configured link type"}
	}

	var linkProperties json.RawMessage
	if cfg.PropertiesInputName != nil {
		if v, ok := params[*cfg.PropertiesInputName]; ok {
			linkProperties = v
		}
	}

	return actionPlan{
		kind:             planCreateLink,
		target:           target,
		counterpart:      counterpart,
		linkType:         linkType,
		linkProperties:   linkProperties,
		linkSourceObject: sourceID,
		linkTargetObject: targetLinkID,
	}, nil
}

// planInvokeWebhookAction mirrors `ActionOperationKind::InvokeWebhook`.
func planInvokeWebhookAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	var target *domain.ObjectInstance
	if req.TargetObjectID != nil {
		t, errs := loadAndAuthorizeTarget(ctx, state, claims, *req.TargetObjectID, action.ObjectTypeID)
		if len(errs) > 0 {
			return actionPlan{}, errs
		}
		target = t
	}
	if err := ensureActionTargetPermission(action, target); err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	params := decodeParams(req.Parameters)
	payload := buildHTTPPayload(action, target, params)
	invocation, err := validateHTTPInvocationConfig(operationConfigBytes(action.Config))
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	return actionPlan{
		kind:       planInvokeWebhook,
		target:     target,
		invocation: invocation,
		payload:    payload,
		parameters: params,
	}, nil
}

// planInvokeFunctionAction mirrors `ActionOperationKind::InvokeFunction`.
//
// The Rust impl picks Inline (function-package) over Http when the
// config carries a `function_package_id` / `runtime`+`source` triple.
// The Go port routes the inline path through `domain.ResolveInlineFunctionConfig`
// (already 1:1 with Rust); the HTTP fallback uses the same shape as
// invoke_webhook.
func planInvokeFunctionAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	var target *domain.ObjectInstance
	if req.TargetObjectID != nil {
		t, errs := loadAndAuthorizeTarget(ctx, state, claims, *req.TargetObjectID, action.ObjectTypeID)
		if len(errs) > 0 {
			return actionPlan{}, errs
		}
		target = t
	}
	if err := ensureActionTargetPermission(action, target); err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	params := decodeParams(req.Parameters)
	payload := buildHTTPPayload(action, target, params)

	// Try the inline function path first (mirrors `resolve_inline_function_config`).
	resolved, err := domain.ResolveInlineFunctionConfig(ctx, state, operationConfigBytes(action.Config))
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	if resolved != nil {
		// Inline runtime — ExecutePlan delegates to ActionFunctionRuntime.
		return actionPlan{
			kind:           planInvokeFunction,
			target:         target,
			payload:        payload,
			parameters:     params,
			inlineFunction: resolved,
			// Inline invocation is captured via an internal URL scheme so
			// the executor recognises the path.
			invocation: &httpInvocationConfig{URL: "inline://" + resolved.RuntimeName(), Method: "POST"},
		}, nil
	}
	invocation, err := validateHTTPInvocationConfig(operationConfigBytes(action.Config))
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	return actionPlan{
		kind:       planInvokeFunction,
		target:     target,
		invocation: invocation,
		payload:    payload,
		parameters: params,
	}, nil
}

// ── interface-typed action planning ─────────────────────────────────

type interfaceLinkActionConfig struct {
	LinkTypeID          uuid.UUID  `json:"link_type_id"`
	SourceInputName     string     `json:"source_input_name,omitempty"`
	TargetInputName     string     `json:"target_input_name,omitempty"`
	PropertiesInputName *string    `json:"properties_input_name,omitempty"`
	SourceInterfaceID   *uuid.UUID `json:"source_interface_id,omitempty"`
	TargetInterfaceID   *uuid.UUID `json:"target_interface_id,omitempty"`
}

func planCreateInterfaceAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	params := decodeParams(req.Parameters)
	concreteTypeID, err := resolveConcreteObjectTypeForInterface(ctx, state, action.ObjectTypeID, params)
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	if err := ensureObjectTypeRecordExists(ctx, state, concreteTypeID); err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	actionForConcrete := action
	actionForConcrete.ObjectTypeID = concreteTypeID
	patch, errs := buildUpdateObjectPatch(ctx, state, actionForConcrete, req.Parameters)
	if len(errs) > 0 {
		return actionPlan{}, errs
	}
	newID := uuid.Nil
	if req.TargetObjectID != nil {
		newID = *req.TargetObjectID
	} else if id, ok, err := optionalUUIDParameter(params, "__object_id"); err != nil {
		return actionPlan{}, []string{err.Error()}
	} else if ok {
		newID = id
	} else {
		var genErr error
		newID, genErr = uuid.NewV7()
		if genErr != nil {
			newID = uuid.New()
		}
	}
	return actionPlan{kind: planCreateInterface, interfaceID: action.ObjectTypeID, concreteObjectTypeID: concreteTypeID, newObjectID: newID, patch: patch}, nil
}

func planModifyInterfaceAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	target, errs := resolveInterfaceTarget(ctx, state, claims, action.ObjectTypeID, req)
	if len(errs) > 0 {
		return actionPlan{}, errs
	}
	if err := ensureActionTargetPermission(action, target); err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	actionForConcrete := action
	actionForConcrete.ObjectTypeID = target.ObjectTypeID
	patch, errs := buildUpdateObjectPatch(ctx, state, actionForConcrete, req.Parameters)
	if len(errs) > 0 {
		return actionPlan{}, errs
	}
	return actionPlan{kind: planModifyInterface, interfaceID: action.ObjectTypeID, concreteObjectTypeID: target.ObjectTypeID, target: target, patch: patch}, nil
}

func planDeleteInterfaceAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	target, errs := resolveInterfaceTarget(ctx, state, claims, action.ObjectTypeID, req)
	if len(errs) > 0 {
		return actionPlan{}, errs
	}
	if err := ensureActionTargetPermission(action, target); err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	return actionPlan{kind: planDeleteInterface, interfaceID: action.ObjectTypeID, concreteObjectTypeID: target.ObjectTypeID, target: target}, nil
}

func planCreateInterfaceLinkAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	return planInterfaceLinkAction(ctx, state, claims, action, req, planCreateInterfaceLink)
}

func planDeleteInterfaceLinkAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
) (actionPlan, []string) {
	return planInterfaceLinkAction(ctx, state, claims, action, req, planDeleteInterfaceLink)
}

func planInterfaceLinkAction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	req *models.ValidateActionRequest,
	kind actionPlanKind,
) (actionPlan, []string) {
	var cfg interfaceLinkActionConfig
	if err := json.Unmarshal(operationConfigBytes(action.Config), &cfg); err != nil {
		return actionPlan{}, []string{"invalid interface link action config: " + err.Error()}
	}
	if cfg.SourceInputName == "" {
		cfg.SourceInputName = "__interface_ref"
	}
	if cfg.TargetInputName == "" {
		cfg.TargetInputName = "target_interface_ref"
	}
	params := decodeParams(req.Parameters)
	sourceID, err := resolveUUIDParameter(params, cfg.SourceInputName)
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	targetID, err := resolveUUIDParameter(params, cfg.TargetInputName)
	if err != nil {
		return actionPlan{}, []string{err.Error()}
	}
	sourceObj, errs := loadAndAuthorizeInterfaceObject(ctx, state, claims, sourceID, nil)
	if len(errs) > 0 {
		return actionPlan{}, errs
	}
	targetObj, errs := loadAndAuthorizeInterfaceObject(ctx, state, claims, targetID, nil)
	if len(errs) > 0 {
		return actionPlan{}, errs
	}
	if cfg.SourceInterfaceID != nil {
		if err := ensureObjectImplementsInterface(ctx, state, sourceObj.ObjectTypeID, *cfg.SourceInterfaceID); err != nil {
			return actionPlan{}, []string{err.Error()}
		}
	}
	if cfg.TargetInterfaceID != nil {
		if err := ensureObjectImplementsInterface(ctx, state, targetObj.ObjectTypeID, *cfg.TargetInterfaceID); err != nil {
			return actionPlan{}, []string{err.Error()}
		}
	}
	linkType, err := domain.LoadLinkTypeViaStore(ctx, state.Stores.Definitions, cfg.LinkTypeID)
	if err != nil {
		return actionPlan{}, []string{"failed to load link type: " + err.Error()}
	}
	if linkType == nil {
		return actionPlan{}, []string{"configured link type was not found"}
	}
	if sourceObj.ObjectTypeID != linkType.SourceTypeID || targetObj.ObjectTypeID != linkType.TargetTypeID {
		return actionPlan{}, []string{"interface link endpoints do not match configured link type"}
	}
	var linkProperties json.RawMessage
	if cfg.PropertiesInputName != nil {
		if v, ok := params[*cfg.PropertiesInputName]; ok {
			linkProperties = v
		}
	}
	return actionPlan{kind: kind, target: sourceObj, counterpart: targetObj, linkType: linkType, linkProperties: linkProperties, linkSourceObject: sourceObj.ID, linkTargetObject: targetObj.ID}, nil
}

func resolveInterfaceTarget(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	interfaceID uuid.UUID,
	req *models.ValidateActionRequest,
) (*domain.ObjectInstance, []string) {
	params := decodeParams(req.Parameters)
	objectID := uuid.Nil
	if req.TargetObjectID != nil {
		objectID = *req.TargetObjectID
	} else {
		id, err := resolveUUIDParameter(params, "__interface_ref")
		if err != nil {
			return nil, []string{err.Error()}
		}
		objectID = id
	}
	return loadAndAuthorizeInterfaceObject(ctx, state, claims, objectID, &interfaceID)
}

func loadAndAuthorizeInterfaceObject(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	objectID uuid.UUID,
	interfaceID *uuid.UUID,
) (*domain.ObjectInstance, []string) {
	obj, err := objects.LoadObjectInstance(ctx, state, claims, objectID, storage.Strong())
	if err != nil {
		return nil, []string{"failed to load interface object: " + err.Error()}
	}
	if obj == nil {
		return nil, []string{"interface object was not found"}
	}
	if err := domain.EnsureObjectAccess(claims, obj); err != nil {
		return nil, []string{forbiddenLine(err.Error())}
	}
	if interfaceID != nil {
		if err := ensureObjectImplementsInterface(ctx, state, obj.ObjectTypeID, *interfaceID); err != nil {
			return nil, []string{err.Error()}
		}
	}
	return obj, nil
}

func resolveConcreteObjectTypeForInterface(
	ctx context.Context,
	state *ontologykernel.AppState,
	interfaceID uuid.UUID,
	params map[string]json.RawMessage,
) (uuid.UUID, error) {
	if err := ensureInterfaceExists(ctx, state, interfaceID); err != nil {
		return uuid.Nil, err
	}
	implementations, err := loadInterfaceImplementations(ctx, state, interfaceID)
	if err != nil {
		return uuid.Nil, err
	}
	if len(implementations) == 0 {
		return uuid.Nil, fmt.Errorf("interface %s has no implementing object types", interfaceID)
	}
	if raw, ok := params["__object_type"]; ok {
		objectTypeID, err := parseUUIDRaw(raw, "__object_type")
		if err != nil {
			return uuid.Nil, err
		}
		for _, candidate := range implementations {
			if candidate == objectTypeID {
				return objectTypeID, nil
			}
		}
		return uuid.Nil, fmt.Errorf("object_type_id %s does not implement interface %s", objectTypeID, interfaceID)
	}
	if len(implementations) > 1 {
		return uuid.Nil, fmt.Errorf("ambiguous implementation for interface %s; provide __object_type", interfaceID)
	}
	return implementations[0], nil
}

func ensureObjectImplementsInterface(ctx context.Context, state *ontologykernel.AppState, objectTypeID, interfaceID uuid.UUID) error {
	if err := ensureInterfaceExists(ctx, state, interfaceID); err != nil {
		return err
	}
	implementations, err := loadInterfaceImplementations(ctx, state, interfaceID)
	if err != nil {
		return err
	}
	for _, candidate := range implementations {
		if candidate == objectTypeID {
			return nil
		}
	}
	return fmt.Errorf("object_type_id %s does not implement interface %s", objectTypeID, interfaceID)
}

func ensureInterfaceExists(ctx context.Context, state *ontologykernel.AppState, interfaceID uuid.UUID) error {
	rec, err := state.Stores.Definitions.Get(ctx, storage.DefinitionKind("interface"), storage.DefinitionId(interfaceID.String()), storage.Strong())
	if err != nil {
		return fmt.Errorf("failed to load interface: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("interface %s was not found", interfaceID)
	}
	return nil
}

func ensureObjectTypeRecordExists(ctx context.Context, state *ontologykernel.AppState, objectTypeID uuid.UUID) error {
	rec, err := state.Stores.Definitions.Get(ctx, storage.DefinitionKind("object_type"), storage.DefinitionId(objectTypeID.String()), storage.Strong())
	if err != nil {
		return fmt.Errorf("failed to load object type: %w", err)
	}
	if rec == nil {
		return fmt.Errorf("object_type_id %s was not found", objectTypeID)
	}
	return nil
}

func loadInterfaceImplementations(ctx context.Context, state *ontologykernel.AppState, interfaceID uuid.UUID) ([]uuid.UUID, error) {
	if state.DB != nil {
		rows, err := state.DB.Query(ctx, `SELECT object_type_id FROM object_type_interfaces WHERE interface_id = $1 ORDER BY created_at ASC, object_type_id ASC`, interfaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to load interface implementations: %w", err)
		}
		defer rows.Close()
		out := []uuid.UUID{}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	}
	page, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{Kind: storage.DefinitionKind("object_type_interface"), Page: storage.Page{Size: 10_000}}, storage.Strong())
	if err != nil {
		return nil, fmt.Errorf("failed to load interface implementations: %w", err)
	}
	out := []uuid.UUID{}
	for _, rec := range page.Items {
		var binding models.ObjectTypeInterfaceBinding
		if err := json.Unmarshal(rec.Payload, &binding); err != nil {
			continue
		}
		if binding.InterfaceID == interfaceID {
			out = append(out, binding.ObjectTypeID)
		}
	}
	return out, nil
}

func optionalUUIDParameter(params map[string]json.RawMessage, fieldName string) (uuid.UUID, bool, error) {
	raw, ok := params[fieldName]
	if !ok || string(raw) == "null" {
		return uuid.Nil, false, nil
	}
	id, err := parseUUIDRaw(raw, fieldName)
	return id, true, err
}

func parseUUIDRaw(raw json.RawMessage, fieldName string) (uuid.UUID, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		parsed, parseErr := uuid.Parse(asString)
		if parseErr != nil {
			return uuid.Nil, fmt.Errorf("%s must be a valid UUID", fieldName)
		}
		return parsed, nil
	}
	var asObject struct {
		ObjectID     *uuid.UUID `json:"object_id"`
		ID           *uuid.UUID `json:"id"`
		ObjectTypeID *uuid.UUID `json:"object_type_id"`
	}
	if err := json.Unmarshal(raw, &asObject); err == nil {
		if asObject.ObjectID != nil {
			return *asObject.ObjectID, nil
		}
		if asObject.ObjectTypeID != nil {
			return *asObject.ObjectTypeID, nil
		}
		if asObject.ID != nil {
			return *asObject.ID, nil
		}
	}
	return uuid.Nil, fmt.Errorf("%s must be a UUID string", fieldName)
}

// createLinkActionConfig mirrors the private Rust struct.
type createLinkActionConfig struct {
	LinkTypeID          uuid.UUID `json:"link_type_id"`
	TargetInputName     string    `json:"target_input_name"`
	SourceRole          string    `json:"source_role"`
	PropertiesInputName *string   `json:"properties_input_name,omitempty"`
}

func decodeParams(raw json.RawMessage) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	if len(raw) == 0 || string(raw) == "null" {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func resolveUUIDParameter(params map[string]json.RawMessage, fieldName string) (uuid.UUID, error) {
	raw, ok := params[fieldName]
	if !ok {
		return uuid.Nil, fmt.Errorf("%s must be a UUID string", fieldName)
	}
	return parseUUIDRaw(raw, fieldName)
}

func validateHTTPInvocationConfig(config json.RawMessage) (*httpInvocationConfig, error) {
	var inv httpInvocationConfig
	if err := json.Unmarshal(config, &inv); err != nil {
		return nil, errors.New("invalid HTTP action config: " + err.Error())
	}
	if strings.TrimSpace(inv.URL) == "" {
		return nil, errors.New("HTTP action config requires a non-empty url")
	}
	scheme := ""
	if i := strings.Index(inv.URL, "://"); i > 0 {
		scheme = strings.ToLower(inv.URL[:i])
	}
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("HTTP action url must use http or https")
	}
	if strings.TrimSpace(inv.Method) == "" {
		inv.Method = "POST"
	}
	inv.Method = strings.ToUpper(strings.TrimSpace(inv.Method))
	switch inv.Method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, errors.New("HTTP action method must be GET/POST/PUT/PATCH/DELETE")
	}
	if inv.Headers == nil {
		inv.Headers = map[string]string{}
	}
	return &inv, nil
}

func buildHTTPPayload(
	action models.ActionType,
	target *domain.ObjectInstance,
	parameters map[string]json.RawMessage,
) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"action": map[string]any{
			"id":             action.ID,
			"name":           action.Name,
			"display_name":   action.DisplayName,
			"object_type_id": action.ObjectTypeID,
			"operation_kind": action.OperationKind,
		},
		"target_object": target,
		"parameters":    parameters,
	})
	return out
}

// buildUpdateObjectPatch mirrors the inner section of plan_action's
// UpdateObject branch. Walks property_mappings + static_patch,
// resolves input_name lookups against the request parameters, and
// validates each value against the property schema before
// returning the merged patch.
func buildUpdateObjectPatch(
	ctx context.Context,
	state *ontologykernel.AppState,
	action models.ActionType,
	parameters json.RawMessage,
) (map[string]json.RawMessage, []string) {
	var cfg models.UpdateObjectActionConfig
	// Pull operation_config out of the wrapper if it exists; otherwise
	// the action's config is the operation config.
	configBytes := operationConfigBytes(action.Config)
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, []string{"invalid action config: " + err.Error()}
	}

	defs, err := loadEffectivePropertiesForAction(ctx, state, action.ObjectTypeID)
	if err != nil {
		return nil, []string{"failed to load property definitions: " + err.Error()}
	}
	typeByName := map[string]string{}
	for _, p := range defs {
		typeByName[p.Name] = p.PropertyType
	}

	var paramMap map[string]json.RawMessage
	_ = json.Unmarshal(parameters, &paramMap)

	patch := map[string]json.RawMessage{}
	for _, m := range cfg.PropertyMappings {
		propType, ok := typeByName[m.PropertyName]
		if !ok {
			return nil, []string{fmt.Sprintf("unknown property '%s' in update_object action config", m.PropertyName)}
		}
		var value json.RawMessage
		switch {
		case m.InputName != nil:
			v, ok := paramMap[*m.InputName]
			if !ok {
				return nil, []string{fmt.Sprintf("missing input '%s' for property mapping", *m.InputName)}
			}
			value = v
		case len(m.Value) > 0 && string(m.Value) != "null":
			value = m.Value
		default:
			value = json.RawMessage(`null`)
		}
		if err := domain.ValidatePropertyValue(propType, value); err != nil {
			return nil, []string{m.PropertyName + ": " + err.Error()}
		}
		patch[m.PropertyName] = value
	}

	if len(cfg.StaticPatch) > 0 && string(cfg.StaticPatch) != "null" {
		var staticMap map[string]json.RawMessage
		if err := json.Unmarshal(cfg.StaticPatch, &staticMap); err != nil {
			return nil, []string{"static_patch must be a JSON object"}
		}
		for name, value := range staticMap {
			propType, ok := typeByName[name]
			if !ok {
				return nil, []string{"unknown property '" + name + "' in static_patch"}
			}
			if err := domain.ValidatePropertyValue(propType, value); err != nil {
				return nil, []string{name + ": " + err.Error()}
			}
			patch[name] = value
		}
	}
	if len(patch) == 0 {
		return nil, []string{"update_object action requires property_mappings or static_patch"}
	}
	return patch, nil
}

// operationConfigBytes mirrors `split_action_config`'s "extract the
// nested `operation` envelope" behaviour. When the config wraps the
// real operation under a `{ "operation": ..., "notification_side_effects": ... }`
// envelope, we pull just the `operation` value. Otherwise the
// config IS the operation config.
func operationConfigBytes(config json.RawMessage) json.RawMessage {
	if len(config) == 0 || string(config) == "null" {
		return json.RawMessage(`{}`)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(config, &asMap); err != nil {
		return config
	}
	hasEnvelope := false
	for _, key := range []string{"operation", "notification_side_effects",
		"webhook_writeback", "webhook_side_effects", "batched_execution"} {
		if _, ok := asMap[key]; ok {
			hasEnvelope = true
			break
		}
	}
	if !hasEnvelope {
		return config
	}
	if op, ok := asMap["operation"]; ok && len(op) > 0 && string(op) != "null" {
		return op
	}
	return json.RawMessage(`{}`)
}

// loadAndAuthorizeTarget mirrors `async fn load_and_authorize_target`.
// Loads the object via the read-side helper, runs the access check
// (claims-side admin / clearance / org_id), then enforces that the
// object's type matches the action's declared object_type_id.
func loadAndAuthorizeTarget(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	targetID, objectTypeID uuid.UUID,
) (*domain.ObjectInstance, []string) {
	obj, err := objects.LoadObjectInstance(ctx, state, claims, targetID, storage.Strong())
	if err != nil {
		return nil, []string{"failed to load target object: " + err.Error()}
	}
	if obj == nil {
		return nil, []string{"target object was not found"}
	}
	if err := domain.EnsureObjectAccess(claims, obj); err != nil {
		return nil, []string{forbiddenLine(err.Error())}
	}
	if obj.ObjectTypeID != objectTypeID {
		return nil, []string{"target object does not match the action's object_type_id"}
	}
	return obj, nil
}

// ensureActionTargetPermission mirrors
// `pub(crate) fn ensure_action_target_permission`. When the policy
// declares allowed_markings, the target's marking must intersect.
func ensureActionTargetPermission(action models.ActionType, target *domain.ObjectInstance) error {
	if len(action.AuthorizationPolicy.AllowedMarkings) == 0 {
		return nil
	}
	if target == nil {
		return errors.New("forbidden: target object is required by the action authorization policy")
	}
	for _, m := range action.AuthorizationPolicy.AllowedMarkings {
		if m == target.Marking {
			return nil
		}
	}
	return errors.New("forbidden: target marking is not permitted by the action authorization policy")
}

// ensureConfirmationJustification mirrors
// `fn ensure_confirmation_justification`. Actions flagged as
// confirmation_required reject calls without a justification string.
func ensureConfirmationJustification(action models.ActionType, justification *string) error {
	if !action.ConfirmationRequired {
		return nil
	}
	if justification == nil || strings.TrimSpace(*justification) == "" {
		return errors.New("justification is required for confirmation-gated actions")
	}
	return nil
}

// ── plan_preview ────────────────────────────────────────────────────

func planPreview(plan actionPlan) json.RawMessage {
	switch plan.kind {
	case planUpdateObject, planModifyInterface:
		kind := "update_object"
		if plan.kind == planModifyInterface {
			kind = "modify_interface"
		}
		out, _ := json.Marshal(map[string]any{
			"kind":             kind,
			"interface_id":     optionalUUIDValue(plan.interfaceID),
			"object_type_id":   optionalUUIDValue(plan.concreteObjectTypeID),
			"target_object_id": plan.target.ID,
			"patch":            plan.patch,
		})
		return out
	case planDeleteObject, planDeleteInterface:
		kind := "delete_object"
		if plan.kind == planDeleteInterface {
			kind = "delete_interface"
		}
		out, _ := json.Marshal(map[string]any{
			"kind":             kind,
			"interface_id":     optionalUUIDValue(plan.interfaceID),
			"object_type_id":   optionalUUIDValue(plan.concreteObjectTypeID),
			"target_object_id": plan.target.ID,
		})
		return out
	case planCreateInterface:
		out, _ := json.Marshal(map[string]any{
			"kind":             "create_interface",
			"interface_id":     plan.interfaceID,
			"object_type_id":   plan.concreteObjectTypeID,
			"target_object_id": plan.newObjectID,
			"patch":            plan.patch,
		})
		return out
	case planCreateLink, planCreateInterfaceLink, planDeleteInterfaceLink:
		kind := "create_link"
		if plan.kind == planCreateInterfaceLink {
			kind = "create_interface_link"
		} else if plan.kind == planDeleteInterfaceLink {
			kind = "delete_interface_link"
		}
		out, _ := json.Marshal(map[string]any{
			"kind":                  kind,
			"target_object_id":      plan.target.ID,
			"counterpart_object_id": plan.counterpart.ID,
			"link_type_id":          plan.linkType.ID,
			"source_object_id":      plan.linkSourceObject,
			"linked_object_id":      plan.linkTargetObject,
			"properties":            plan.linkProperties,
		})
		return out
	case planInvokeWebhook:
		var targetID *uuid.UUID
		if plan.target != nil {
			id := plan.target.ID
			targetID = &id
		}
		out, _ := json.Marshal(map[string]any{
			"kind":             "invoke_webhook",
			"target_object_id": targetID,
			"request": map[string]any{
				"url":     plan.invocation.URL,
				"method":  plan.invocation.Method,
				"headers": plan.invocation.Headers,
				"payload": json.RawMessage(plan.payload),
			},
		})
		return out
	case planInvokeFunction:
		var targetID *uuid.UUID
		if plan.target != nil {
			id := plan.target.ID
			targetID = &id
		}
		runtime := "http"
		if strings.HasPrefix(plan.invocation.URL, "inline://") {
			runtime = strings.TrimPrefix(plan.invocation.URL, "inline://")
		}
		out, _ := json.Marshal(map[string]any{
			"kind":             "invoke_function",
			"runtime":          runtime,
			"target_object_id": targetID,
			"request": map[string]any{
				"url":     plan.invocation.URL,
				"method":  plan.invocation.Method,
				"headers": plan.invocation.Headers,
				"payload": json.RawMessage(plan.payload),
			},
		})
		return out
	}
	return json.RawMessage(`null`)
}

func optionalUUIDValue(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}

// ── action-attempt metrics + ledger emitter ─────────────────────────

// emitActionAttemptEvent mirrors Rust `record_action_*_metric`: it updates
// Prometheus counters/histograms when the service registered them and appends
// a deterministic `action_attempt` row for the JSON metrics endpoint.
func emitActionAttemptEvent(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	targetObjectID *uuid.UUID,
	parameters json.RawMessage,
	status string,
	durationMs int64,
	failureType *string,
) error {
	if m := ontologymetrics.ActionMetricsSingleton(); m != nil {
		seconds := float64(durationMs) / 1000.0
		if status == "success" {
			m.RecordSuccess(action.ID.String(), seconds)
		} else {
			m.RecordFailure(action.ID.String(), classifyFailureTypeString(failureType), seconds)
		}
	}
	payload := map[string]any{
		"action_type_id":   action.ID.String(),
		"target_object_id": targetObjectID,
		"parameters":       jsonAsAny(parameters),
		"status":           status,
		"duration_ms":      durationMs,
		"organization_id":  claims.OrgID,
	}
	if failureType != nil {
		payload["failure_type"] = *failureType
	}
	body, _ := json.Marshal(payload)
	var objRef *storage.ObjectId
	targetIDStr := "none"
	if targetObjectID != nil {
		ref := storage.ObjectId(targetObjectID.String())
		objRef = &ref
		targetIDStr = targetObjectID.String()
	}
	eventID := deterministicActionEventID([]string{
		"attempt", action.ID.String(), targetIDStr, claims.Sub.String(), status, string(parameters),
	})
	return state.Stores.Actions.Append(ctx, storage.ActionLogEntry{
		Tenant:       domain.TenantFromClaims(claims),
		EventID:      &eventID,
		ActionID:     action.ID.String(),
		Kind:         "action_attempt",
		Subject:      claims.Sub.String(),
		Object:       objRef,
		Payload:      body,
		RecordedAtMs: time.Now().UTC().UnixMilli(),
	})
}

func classifyFailureTypeString(failureType *string) ontologymetrics.FailureType {
	if failureType == nil {
		return ontologymetrics.FailureTypeUnclassified
	}
	switch *failureType {
	case "invalid_parameter":
		return ontologymetrics.FailureTypeInvalidParameter
	case "scale_limit":
		return ontologymetrics.FailureTypeScaleLimit
	case "authentication":
		return ontologymetrics.FailureTypeAuthentication
	case "side_effect":
		return ontologymetrics.FailureTypeSideEffect
	case "function":
		return ontologymetrics.FailureTypeFunction
	case "user_facing_function":
		return ontologymetrics.FailureTypeUserFacingFunction
	case "conflict":
		return ontologymetrics.FailureTypeConflict
	default:
		return ontologymetrics.FailureTypeUnclassified
	}
}

func classifyExecutePlanError(err error) ontologymetrics.FailureType {
	if err == nil {
		return ontologymetrics.FailureTypeUnclassified
	}
	if domain.IsVersionConflict(err) {
		return ontologymetrics.FailureTypeConflict
	}
	if errors.Is(err, domain.ErrPythonRuntimeNotWired) {
		return ontologymetrics.FailureTypeSideEffect
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "forbidden") || strings.Contains(lower, "permission") || strings.Contains(lower, "unauthorized"):
		return ontologymetrics.FailureTypeAuthentication
	case strings.Contains(lower, "conflict") || strings.Contains(lower, "unique") || strings.Contains(lower, "duplicate"):
		return ontologymetrics.FailureTypeConflict
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many") || strings.Contains(lower, "quota"):
		return ontologymetrics.FailureTypeScaleLimit
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "upstream") || strings.Contains(lower, "network") || strings.Contains(lower, "webhook") || strings.Contains(lower, "http action"):
		return ontologymetrics.FailureTypeSideEffect
	case strings.Contains(lower, "function"):
		if strings.Contains(lower, "user") {
			return ontologymetrics.FailureTypeUserFacingFunction
		}
		return ontologymetrics.FailureTypeFunction
	case strings.Contains(lower, "invalid") || strings.Contains(lower, "missing") || strings.Contains(lower, "required"):
		return ontologymetrics.FailureTypeInvalidParameter
	default:
		return ontologymetrics.FailureTypeUnclassified
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

// allForbidden mirrors `fn all_forbidden`. Returns true when every
// error string starts with the `forbidden:` prefix.
func allForbidden(errs []string) bool {
	if len(errs) == 0 {
		return false
	}
	for _, e := range errs {
		if !strings.HasPrefix(e, "forbidden:") {
			return false
		}
	}
	return true
}

// forbiddenLine ensures access errors carry the prefix `allForbidden`
// keys off, matching the Rust impl that prefixes every authorisation
// error with `"forbidden:"` exactly.
func forbiddenLine(msg string) string {
	if strings.HasPrefix(msg, "forbidden:") {
		return msg
	}
	return "forbidden: " + msg
}

// decodeOptionalJSON tolerates an empty body — the Rust handlers
// accept default-constructed request envelopes via `axum::Json` +
// `#[serde(default)]`.
func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}
