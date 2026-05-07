// Phase 5C — execute_action_batch (N-target orchestration).
//
// Mirrors `pub async fn execute_action_batch` and the helper
// `execute_batched_function_invocation` in
// `libs/ontology-kernel/src/handlers/actions.rs`.
//
// The Rust impl gates two distinct execution shapes behind the
// `batched_execution` flag pulled out of the action's config envelope:
//
//   - **Per-target loop** (the default and only mode for non-function
//     operation kinds): every target_object_id is validated +
//     planned + executed independently; failures are collected into
//     the result vec but the loop continues so partial successes
//     remain visible.
//   - **Batched function invocation** (function-backed actions that
//     opted in): every target collapses into a single HTTP function
//     call carrying `parameters.batch = [...]` so the user function
//     handles fan-out itself.
//
// Scale limits (`Scale and property limits.md`):
//
//   - Function single-call: 20 targets max.
//   - Batched / standard: 10 000 targets max.
//   - 3 MB per-edit ceiling on the parameters payload.
//   - Per-field list caps (1 000 for object_reference_list,
//     10 000 for primitive arrays).
package actions

import (
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
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
)

// ── Scale limits (mirror `mod scale_limits` in Rust) ────────────────

const (
	maxObjectsPerSubmission           = 10_000
	maxEditBytes                      = 3 * 1024 * 1024
	maxListPrimitive                  = 10_000
	maxObjectReferenceList            = 1_000
	defaultBatchMaxTargets            = 20
	maxNotificationRecipients         = 500
	maxNotificationRecipientsFromFunc = 50
)

// ── ExecuteActionBatch endpoint ─────────────────────────────────────

// ExecuteActionBatchHandler mirrors `pub async fn execute_action_batch`.
//
// Wires the per-target loop or the batched function invocation path
// depending on the action config + operation kind, enforcing the
// documented Foundry scale limits before doing any work. Failures are
// classified consistently with `FailureType::ScaleLimit` (HTTP 429,
// body carries `{"error": …, "failure_type": "scale_limit"}`).
func ExecuteActionBatchHandler(state *ontologykernel.AppState) http.HandlerFunc {
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
		var body models.ExecuteBatchActionRequest
		if err := decodeOptionalJSON(r, &body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		if len(body.TargetObjectIDs) == 0 {
			invalid(w, "target_object_ids must not be empty")
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

		batched := extractBatchedExecutionFlag(action.Config)
		opKind, _ := parseOperationKind(action.OperationKind)
		functionBacked := opKind == "invoke_function"

		limit := maxObjectsPerSubmission
		modeName := "standard"
		if functionBacked && !batched {
			limit = defaultBatchMaxTargets
			modeName = "function single-call"
		} else if batched {
			modeName = "batched"
		}
		if len(body.TargetObjectIDs) > limit {
			scaleLimitResponse(w, fmt.Sprintf(
				"target_object_ids exceeds the per-call scale limit (%d max in %s mode)",
				limit, modeName))
			return
		}

		paramsBytes := estimateEditBytes(body.Parameters)
		if paramsBytes > maxEditBytes {
			scaleLimitResponse(w, fmt.Sprintf(
				"parameters payload of %d bytes exceeds the per-edit scale limit (%d bytes)",
				paramsBytes, maxEditBytes))
			return
		}
		if msg := validateParameterListSizes(action.InputSchema, body.Parameters); msg != "" {
			scaleLimitResponse(w, msg)
			return
		}

		if err := ensureActionActorPermission(claims, action); err != nil {
			forbidden(w, err.Error())
			return
		}
		if err := ensureConfirmationJustification(action, body.Justification); err != nil {
			invalid(w, err.Error())
			return
		}

		total := len(body.TargetObjectIDs)
		results := make([]json.RawMessage, 0, total)
		succeeded := 0

		// ── Batched function-invocation path (function-backed only) ──
		if batched && functionBacked {
			startedAt := time.Now()
			value, err := executeBatchedFunctionInvocation(
				r.Context(), state, claims, action,
				body.TargetObjectIDs, body.Parameters, body.Justification,
			)
			if err != nil {
				failureType := classifyExecutePlanError(err).AsStr()
				_ = emitActionAttemptEvent(r.Context(), state, claims, action, nil, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
				dbError(w, err.Error())
				return
			}
			_ = emitActionAttemptEvent(r.Context(), state, claims, action, nil, body.Parameters, "success", time.Since(startedAt).Milliseconds(), nil)
			writeJSON(w, http.StatusOK, map[string]any{
				"total":     total,
				"succeeded": total,
				"failed":    0,
				"batched":   true,
				"result":    json.RawMessage(value),
			})
			return
		}

		// ── Per-target loop ─────────────────────────────────────────
		_, sideEffects, _ := splitWebhookConfigs(action.Config)
		for _, targetID := range body.TargetObjectIDs {
			startedAt := time.Now()
			tid := targetID
			validationReq := models.ValidateActionRequest{
				TargetObjectID: &tid,
				Parameters:     body.Parameters,
			}
			plan, errs := planAction(r.Context(), state, claims, action, &validationReq)
			if len(errs) > 0 {
				status := "failed"
				audit := "failure"
				if allForbidden(errs) {
					status = "denied"
					audit = "denied"
				}
				if auditErr := emitActionAuditEvent(r.Context(), state, claims, action, nil,
					&tid, audit, "medium", "action validation failed",
					body.Justification, body.Parameters, nil,
					map[string]any{"details": errs, "batch": true}); auditErr != nil {
					logAuditFailure(action.ID, auditErr)
				}
				ft := "invalid_parameter"
				if status == "denied" {
					ft = "authentication"
				}
				_ = emitActionAttemptEvent(r.Context(), state, claims, action, &tid, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &ft)
				results = append(results, mustJSON(map[string]any{
					"target_object_id": tid,
					"status":           status,
					"errors":           errs,
				}))
				continue
			}
			executed, err := executePlan(r.Context(), state, claims, action, plan)
			if err != nil {
				if auditErr := emitActionAuditEvent(r.Context(), state, claims, action, plan.target,
					&tid, "failure", "high", err.Error(),
					body.Justification, body.Parameters, nil,
					map[string]any{"batch": true}); auditErr != nil {
					logAuditFailure(action.ID, auditErr)
				}
				failureType := classifyExecutePlanError(err).AsStr()
				_ = emitActionAttemptEvent(r.Context(), state, claims, action, &tid, body.Parameters, "failure", time.Since(startedAt).Milliseconds(), &failureType)
				results = append(results, mustJSON(map[string]any{
					"target_object_id": tid,
					"status":           "failed",
					"error":            err.Error(),
				}))
				continue
			}
			succeeded++
			auditResult := map[string]any{
				"deleted": executed.deleted,
				"object":  jsonAsAny(executed.object),
				"link":    jsonAsAny(executed.link),
				"result":  jsonAsAny(executed.result),
				"batch":   true,
			}
			if auditErr := emitActionAuditEvent(r.Context(), state, claims, action, plan.target,
				executed.targetObjectID, "success", "low", "",
				body.Justification, body.Parameters, executed.preview, auditResult); auditErr != nil {
				logAuditFailure(action.ID, auditErr)
			}
			if notifErr := emitActionNotifications(r.Context(), state, claims, action, plan.target,
				body.Parameters, body.Justification, executed); notifErr != nil {
				logNotificationFailure(action.ID, notifErr)
			}
			runWebhookSideEffects(r.Context(), state, claims, claims.Sub, action.ID,
				executed.targetObjectID, sideEffects, body.Parameters)
			_ = emitActionAttemptEvent(r.Context(), state, claims, action, executed.targetObjectID, body.Parameters, "success", time.Since(startedAt).Milliseconds(), nil)
			results = append(results, mustJSON(map[string]any{
				"target_object_id": tid,
				"status":           "succeeded",
				"deleted":          executed.deleted,
				"preview":          executed.preview,
				"object":           executed.object,
				"link":             executed.link,
				"result":           executed.result,
			}))
		}

		writeJSON(w, http.StatusOK, models.ExecuteBatchActionResponse{
			Action:    action,
			Total:     total,
			Succeeded: succeeded,
			Failed:    total - succeeded,
			Results:   results,
		})
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

// scaleLimitResponse mirrors `fn scale_limit_response`. HTTP 429 with
// the canonical `failure_type: "scale_limit"` body.
func scaleLimitResponse(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":        message,
		"failure_type": "scale_limit",
	})
}

// estimateEditBytes mirrors `pub fn estimate_edit_bytes`. Uses the
// canonical JSON encoding because that is what eventually crosses the
// writeback wire.
func estimateEditBytes(value json.RawMessage) int {
	if len(value) == 0 {
		return 0
	}
	// Round-trip through the decoder so callers cannot inflate the count
	// by sending whitespace-padded payloads (matches Rust's serde_json).
	var anyVal any
	if err := json.Unmarshal(value, &anyVal); err != nil {
		return len(value)
	}
	out, err := json.Marshal(anyVal)
	if err != nil {
		return 0
	}
	return len(out)
}

// validateParameterListSizes mirrors `pub fn validate_parameter_list_sizes`.
// Returns an empty string when the parameters fit within Foundry's caps.
func validateParameterListSizes(schema []models.ActionInputField, parameters json.RawMessage) string {
	if len(parameters) == 0 {
		return ""
	}
	var paramMap map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &paramMap); err != nil {
		return ""
	}
	for _, field := range schema {
		raw, ok := paramMap[field.Name]
		if !ok {
			continue
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			continue
		}
		var limit int
		switch {
		case field.PropertyType == "object_reference_list":
			limit = maxObjectReferenceList
		case field.PropertyType == "array" || field.PropertyType == "vector":
			limit = maxListPrimitive
		case strings.HasSuffix(field.PropertyType, "_list"):
			limit = maxListPrimitive
		default:
			continue
		}
		if len(arr) > limit {
			return fmt.Sprintf(
				"parameter '%s' exceeds the scale limit (%d items, max %d)",
				field.Name, len(arr), limit)
		}
	}
	return ""
}

// extractBatchedExecutionFlag mirrors `fn extract_batched_execution_flag`.
// Inspects the `batched_execution` key on the action config envelope;
// returns false when the envelope is missing, malformed or absent.
func extractBatchedExecutionFlag(config json.RawMessage) bool {
	if len(config) == 0 || string(config) == "null" {
		return false
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(config, &asMap); err != nil {
		return false
	}
	raw, ok := asMap["batched_execution"]
	if !ok {
		return false
	}
	var flag bool
	if err := json.Unmarshal(raw, &flag); err != nil {
		return false
	}
	return flag
}

// executeBatchedFunctionInvocation mirrors
// `async fn execute_batched_function_invocation`. Collapses N targets
// into a single function call carrying `parameters.batch = [...]`. The
// inline runtime is rejected — only HTTP-backed function configs
// support batched execution today.
func executeBatchedFunctionInvocation(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	targetObjectIDs []uuid.UUID,
	parameters json.RawMessage,
	justification *string,
) (json.RawMessage, error) {
	opCfg := operationConfigBytes(action.Config)
	resolved, err := domain.ResolveInlineFunctionConfig(ctx, state, opCfg)
	if err != nil {
		return nil, err
	}
	if resolved != nil {
		return nil, errors.New("batched_execution requires an HTTP-backed function invocation")
	}
	invocation, err := validateHTTPInvocationConfig(opCfg)
	if err != nil {
		return nil, err
	}

	var paramMap map[string]json.RawMessage
	_ = json.Unmarshal(parameters, &paramMap)
	batchItems := make([]map[string]json.RawMessage, 0, len(targetObjectIDs))
	for _, id := range targetObjectIDs {
		entry := map[string]json.RawMessage{}
		idJSON, _ := json.Marshal(id)
		entry["target_object_id"] = idJSON
		for k, v := range paramMap {
			entry[k] = v
		}
		batchItems = append(batchItems, entry)
	}

	payload := map[string]any{
		"action": map[string]any{
			"id":             action.ID,
			"name":           action.Name,
			"display_name":   action.DisplayName,
			"object_type_id": action.ObjectTypeID,
			"operation_kind": action.OperationKind,
		},
		"actor":         claims.Sub,
		"justification": justification,
		"batched":       true,
		"parameters": map[string]any{
			"batch": batchItems,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode batched function payload: %w", err)
	}
	return invokeHTTPAction(ctx, state, invocation, body)
}

func mustJSON(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return out
}
