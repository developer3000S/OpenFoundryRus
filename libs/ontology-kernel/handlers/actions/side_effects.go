// Phase 5D — webhook writeback / side-effects + notification fan-out +
// audit-service POST. Closes the deferrals from Phase 5C so an action
// execution carries the same external-call cascade as the Rust impl.
//
// Mirrors:
//
//   - `fn split_action_config`, `fn split_webhook_configs`,
//     `fn parse_action_envelope`
//   - `fn invoke_registered_webhook`, `fn run_webhook_writeback`,
//     `fn run_webhook_side_effects`, `fn persist_webhook_side_effect_row`
//   - `fn resolve_notification_recipients`, `fn extract_uuid_values`,
//     `fn render_template`, `fn render_value_templates`,
//     `fn lookup_template_value`, `fn build_notification_metadata`,
//     `async fn emit_action_notifications`,
//     `async fn send_notification_request`
//   - `fn issue_service_token`, `fn classification_for_target`,
//     `async fn emit_action_audit_event`
//
// The Go port preserves the Rust shape: webhook writeback is
// synchronous before plan_action; webhook side-effects fan out
// fire-and-forget after a successful execution; notifications are
// best-effort with audit failures logged via WarnLogger.
package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// ── Action-config envelope ─────────────────────────────────────────

// actionConfigEnvelope mirrors `struct ActionConfigEnvelope`. Only
// the writeback / side-effect / notification slots are read here;
// `operation` and `batched_execution` are handled elsewhere.
type actionConfigEnvelope struct {
	Operation               json.RawMessage                `json:"operation,omitempty"`
	NotificationSideEffects []notificationSideEffectConfig `json:"notification_side_effects,omitempty"`
	WebhookWriteback        *webhookCallConfig             `json:"webhook_writeback,omitempty"`
	WebhookSideEffects      []webhookCallConfig            `json:"webhook_side_effects,omitempty"`
	BatchedExecution        bool                           `json:"batched_execution,omitempty"`
}

// webhookCallConfig mirrors `pub struct WebhookCallConfig`.
type webhookCallConfig struct {
	WebhookID            uuid.UUID              `json:"webhook_id"`
	InputMappings        []webhookInputMapping  `json:"input_mappings,omitempty"`
	OutputMappings       []webhookOutputMapping `json:"output_mappings,omitempty"`
	OutputParameterAlias *string                `json:"output_parameter_alias,omitempty"`
}

// webhookInputMapping mirrors `pub struct WebhookInputMapping`.
type webhookInputMapping struct {
	WebhookInputName string `json:"webhook_input_name"`
	ActionInputName  string `json:"action_input_name"`
}

// webhookOutputMapping copies a named webhook output parameter back into the
// action parameter bag. The aliases keep older client experiments working
// while the canonical keys stay explicit.
type webhookOutputMapping struct {
	WebhookOutputName   string `json:"webhook_output_name,omitempty"`
	WebhookOutputPath   string `json:"webhook_output_path,omitempty"`
	ActionInputName     string `json:"action_input_name,omitempty"`
	ActionParameterName string `json:"action_parameter_name,omitempty"`
}

// notificationSideEffectConfig mirrors
// `struct ActionNotificationSideEffectConfig`.
type notificationSideEffectConfig struct {
	Title                  string          `json:"title"`
	Body                   string          `json:"body"`
	Severity               *string         `json:"severity,omitempty"`
	Category               *string         `json:"category,omitempty"`
	Channels               *[]string       `json:"channels,omitempty"`
	UserIDs                []uuid.UUID     `json:"user_ids,omitempty"`
	UserIDInputName        *string         `json:"user_id_input_name,omitempty"`
	TargetUserPropertyName *string         `json:"target_user_property_name,omitempty"`
	SendToActor            bool            `json:"send_to_actor,omitempty"`
	SendToTargetCreator    bool            `json:"send_to_target_creator,omitempty"`
	Broadcast              bool            `json:"broadcast,omitempty"`
	Metadata               json.RawMessage `json:"metadata,omitempty"`
}

// parseActionEnvelope mirrors `fn parse_action_envelope`. Returns nil
// for legacy configs that don't carry the envelope keys.
func parseActionEnvelope(config json.RawMessage) (*actionConfigEnvelope, error) {
	if len(config) == 0 || string(config) == "null" {
		return nil, nil
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(config, &asMap); err != nil {
		return nil, nil
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
		return nil, nil
	}
	var env actionConfigEnvelope
	if err := json.Unmarshal(config, &env); err != nil {
		return nil, fmt.Errorf("invalid action config envelope: %w", err)
	}
	return &env, nil
}

// splitActionConfig mirrors `fn split_action_config`. Returns the
// `operation` slice and the configured notification side-effects. For
// legacy configs without an envelope, the full config is treated as
// the operation and notifications come back empty.
func splitActionConfig(config json.RawMessage) (json.RawMessage, []notificationSideEffectConfig, error) {
	env, err := parseActionEnvelope(config)
	if err != nil {
		return nil, nil, err
	}
	if env == nil {
		return config, nil, nil
	}
	op := env.Operation
	if len(op) == 0 {
		op = json.RawMessage("null")
	}
	return op, env.NotificationSideEffects, nil
}

// splitWebhookConfigs mirrors `fn split_webhook_configs`.
func splitWebhookConfigs(config json.RawMessage) (*webhookCallConfig, []webhookCallConfig, error) {
	env, err := parseActionEnvelope(config)
	if err != nil {
		return nil, nil, err
	}
	if env == nil {
		return nil, nil, nil
	}
	return env.WebhookWriteback, env.WebhookSideEffects, nil
}

// ── Webhook invocation ─────────────────────────────────────────────

// invokeRegisteredWebhook mirrors `async fn invoke_registered_webhook`.
// Posts the projected `inputs` envelope to
// `connector-management-service` and returns the JSON body when the
// service responds 2xx.
func invokeRegisteredWebhook(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	webhook *webhookCallConfig,
	actionParameters json.RawMessage,
) (json.RawMessage, error) {
	if strings.TrimSpace(state.ConnectorManagementServiceURL) == "" {
		return nil, fmt.Errorf("connector_management_service_url is not configured")
	}
	inputs := map[string]json.RawMessage{}
	var paramMap map[string]json.RawMessage
	_ = json.Unmarshal(actionParameters, &paramMap)
	for _, mapping := range webhook.InputMappings {
		if v, ok := lookupJSONRawPath(paramMap, mapping.ActionInputName); ok {
			inputs[mapping.WebhookInputName] = v
		}
	}
	body, _ := json.Marshal(map[string]any{"inputs": inputs})
	url := strings.TrimRight(state.ConnectorManagementServiceURL, "/") +
		"/api/v1/webhooks/" + webhook.WebhookID.String() + "/invoke"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("webhook invocation failed: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	if state.JWTConfig != nil && claims != nil {
		authHeader, err := issueServiceTokenFor(state, claims, "ontology-action-webhook")
		if err != nil {
			return nil, fmt.Errorf("failed to issue service token for webhook invocation: %w", err)
		}
		req.Header.Set("authorization", authHeader)
	}
	client := state.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webhook invocation failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes := readAllLimited(resp.Body, 16<<20)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("webhook returned %s: %s", resp.Status, string(respBytes))
	}
	if len(strings.TrimSpace(string(respBytes))) == 0 {
		return json.RawMessage("null"), nil
	}
	var v any
	if err := json.Unmarshal(respBytes, &v); err != nil {
		quoted, _ := json.Marshal(string(respBytes))
		return quoted, nil
	}
	out, _ := json.Marshal(v)
	return out, nil
}

// runWebhookWriteback mirrors `async fn run_webhook_writeback`. Mutates
// the parameters envelope in place, merging the webhook's
// `output_parameters` (or the full response when no key is present)
// under either the configured alias or `webhook_output`.
func runWebhookWriteback(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	config *webhookCallConfig,
	parameters *json.RawMessage,
) error {
	resp, err := invokeRegisteredWebhook(ctx, state, claims, config, *parameters)
	if err != nil {
		return err
	}
	var asObj map[string]json.RawMessage
	output := resp
	if err := json.Unmarshal(resp, &asObj); err == nil {
		if v, ok := asObj["output_parameters"]; ok {
			output = v
		}
	}
	var paramMap map[string]json.RawMessage
	if len(*parameters) > 0 {
		_ = json.Unmarshal(*parameters, &paramMap)
	}
	if paramMap == nil {
		paramMap = map[string]json.RawMessage{}
	}
	alias := "webhook_output"
	if config.OutputParameterAlias != nil {
		alias = *config.OutputParameterAlias
	}
	paramMap[alias] = output
	for _, mapping := range config.OutputMappings {
		path := strings.TrimSpace(mapping.WebhookOutputPath)
		if path == "" {
			path = strings.TrimSpace(mapping.WebhookOutputName)
		}
		name := strings.TrimSpace(mapping.ActionParameterName)
		if name == "" {
			name = strings.TrimSpace(mapping.ActionInputName)
		}
		if path == "" || name == "" {
			continue
		}
		if value, ok := lookupJSONRawPathInRaw(output, path); ok {
			paramMap[name] = value
		}
	}
	merged, err := json.Marshal(paramMap)
	if err != nil {
		return err
	}
	*parameters = merged
	return nil
}

// runWebhookSideEffects mirrors `async fn run_webhook_side_effects`.
// Fan-out is sequential here (the Rust path uses
// `futures::future::join_all` for parallelism but Go's net/http already
// pools per-host); errors are logged + persisted but never returned
// because side effects are non-blocking by design.
func runWebhookSideEffects(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	actor uuid.UUID,
	actionID uuid.UUID,
	targetObjectID *uuid.UUID,
	configs []webhookCallConfig,
	parameters json.RawMessage,
) {
	if len(configs) == 0 {
		return
	}
	for _, cfg := range configs {
		c := cfg
		resp, err := invokeRegisteredWebhook(ctx, state, claims, &c, parameters)
		if err != nil {
			log.Printf("ontology action webhook side-effect failed action=%s webhook=%s err=%s",
				actionID, c.WebhookID, err.Error())
			persistWebhookSideEffectRow(ctx, state, claims, actionID, targetObjectID, c.WebhookID, actor, "failure", nil, err.Error())
			continue
		}
		persistWebhookSideEffectRow(ctx, state, claims, actionID, targetObjectID, c.WebhookID, actor, "success", resp, "")
	}
}

// persistWebhookSideEffectRow mirrors
// `async fn persist_webhook_side_effect_row`. Best-effort append to
// the action log so the metrics endpoint can spot side-effect runs.
func persistWebhookSideEffectRow(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	actionID uuid.UUID,
	targetObjectID *uuid.UUID,
	webhookID uuid.UUID,
	actor uuid.UUID,
	status string,
	response json.RawMessage,
	errMessage string,
) {
	payload := map[string]any{
		"action_type_id":   actionID,
		"side_effect_type": "webhook",
		"webhook_id":       webhookID,
		"actor_id":         actor,
		"status":           status,
		"response":         response,
		"error_message":    errMessage,
		"organization_id":  claims.OrgID,
	}
	body, _ := json.Marshal(payload)
	objRef := optionalObjectRef(targetObjectID)
	targetIDStr := "none"
	if targetObjectID != nil {
		targetIDStr = targetObjectID.String()
	}
	eventID := deterministicActionEventID([]string{
		"side_effect", actionID.String(), targetIDStr, webhookID.String(),
		actor.String(), status,
	})
	if err := state.Stores.Actions.Append(ctx, storage.ActionLogEntry{
		Tenant:       domain.TenantFromClaims(claims),
		EventID:      &eventID,
		ActionID:     actionID.String(),
		Kind:         "side_effect",
		Subject:      actor.String(),
		Object:       objRef,
		Payload:      body,
		RecordedAtMs: time.Now().UTC().UnixMilli(),
	}); err != nil {
		log.Printf("ontology action side-effect ledger append failed action=%s webhook=%s err=%s",
			actionID, webhookID, err.Error())
	}
}

func optionalObjectRef(id *uuid.UUID) *storage.ObjectId {
	if id == nil {
		return nil
	}
	r := storage.ObjectId(id.String())
	return &r
}

// deterministicActionEventID mirrors `fn deterministic_action_event_id`.
// Used to make side-effect log appends idempotent on retry.
func deterministicActionEventID(parts []string) string {
	material := "ontology/action/" + strings.Join(parts, "/")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(material)).String()
}

// ── Notification fan-out ───────────────────────────────────────────

// extractUUIDValues mirrors `fn extract_uuid_values`.
func extractUUIDValues(value json.RawMessage, source string) ([]uuid.UUID, error) {
	if len(value) == 0 || string(value) == "null" {
		return nil, nil
	}
	// Try string.
	var asString string
	if err := json.Unmarshal(value, &asString); err == nil {
		id, err := uuid.Parse(asString)
		if err != nil {
			return nil, fmt.Errorf("%s must contain UUID strings", source)
		}
		return []uuid.UUID{id}, nil
	}
	// Try array of strings.
	var asArr []string
	if err := json.Unmarshal(value, &asArr); err == nil {
		out := make([]uuid.UUID, 0, len(asArr))
		for _, raw := range asArr {
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("%s must contain UUID strings", source)
			}
			out = append(out, id)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s must be a UUID string or list of UUID strings", source)
}

// resolveNotificationRecipients mirrors
// `fn resolve_notification_recipients`. Enforces the
// MAX_NOTIFICATION_RECIPIENTS / FROM_FUNCTION caps.
func resolveNotificationRecipients(
	config notificationSideEffectConfig,
	parameters map[string]json.RawMessage,
	target *domain.ObjectInstance,
	actorID uuid.UUID,
) ([]uuid.UUID, bool, error) {
	recipients := map[uuid.UUID]struct{}{}
	for _, id := range config.UserIDs {
		recipients[id] = struct{}{}
	}
	if config.UserIDInputName != nil {
		raw, ok := parameters[*config.UserIDInputName]
		if !ok {
			return nil, false, fmt.Errorf(
				"notification recipient input '%s' is missing", *config.UserIDInputName)
		}
		ids, err := extractUUIDValues(raw, *config.UserIDInputName)
		if err != nil {
			return nil, false, err
		}
		for _, id := range ids {
			recipients[id] = struct{}{}
		}
	}
	if config.TargetUserPropertyName != nil {
		if target == nil {
			return nil, false, fmt.Errorf(
				"notification recipient target property '%s' requires a target object",
				*config.TargetUserPropertyName)
		}
		var props map[string]json.RawMessage
		_ = json.Unmarshal(target.Properties, &props)
		raw, ok := props[*config.TargetUserPropertyName]
		if !ok {
			return nil, false, fmt.Errorf(
				"target property '%s' is missing", *config.TargetUserPropertyName)
		}
		ids, err := extractUUIDValues(raw, *config.TargetUserPropertyName)
		if err != nil {
			return nil, false, err
		}
		for _, id := range ids {
			recipients[id] = struct{}{}
		}
	}
	if config.SendToActor {
		recipients[actorID] = struct{}{}
	}
	if config.SendToTargetCreator {
		if target == nil {
			return nil, false, fmt.Errorf(
				"send_to_target_creator requires a target object for notification side effect")
		}
		recipients[target.CreatedBy] = struct{}{}
	}
	if len(recipients) == 0 && !config.Broadcast {
		return nil, false, fmt.Errorf("notification side effect resolved no recipients")
	}

	fromFunction := config.TargetUserPropertyName != nil
	cap := maxNotificationRecipients
	if fromFunction {
		cap = maxNotificationRecipientsFromFunc
	}
	if len(recipients) > cap {
		suffix := ""
		if fromFunction {
			suffix = ", recipients from a function"
		}
		return nil, false, fmt.Errorf(
			"notification side effect resolved %d recipients which exceeds the scale limit (%d max%s)",
			len(recipients), cap, suffix)
	}

	out := make([]uuid.UUID, 0, len(recipients))
	for id := range recipients {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, config.Broadcast, nil
}

// renderTemplate mirrors `fn render_template`. Replaces `{{path.to.value}}`
// tokens by walking the JSON context. Unknown tokens drop to empty.
func renderTemplate(template string, context any) string {
	var rendered strings.Builder
	rendered.Grow(len(template))
	remaining := template
	for {
		start := strings.Index(remaining, "{{")
		if start < 0 {
			rendered.WriteString(remaining)
			break
		}
		rendered.WriteString(remaining[:start])
		afterStart := remaining[start+2:]
		end := strings.Index(afterStart, "}}")
		if end < 0 {
			rendered.WriteString(remaining[start:])
			break
		}
		token := strings.TrimSpace(afterStart[:end])
		if v, ok := lookupTemplateValue(context, token); ok {
			rendered.WriteString(v)
		}
		remaining = afterStart[end+2:]
	}
	return rendered.String()
}

// lookupTemplateValue mirrors `fn lookup_template_value`. Walks
// dot-delimited paths into the context object and renders the leaf
// using the same shape Rust's `serde_json::to_string` produces for
// scalars (string, number, bool, null).
func lookupTemplateValue(context any, path string) (string, bool) {
	cur := context
	for _, segment := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[segment]
			if !ok {
				return "", false
			}
			cur = next
		default:
			return "", false
		}
	}
	switch v := cur.(type) {
	case string:
		return v, true
	case float64:
		return jsonNumberString(v), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case nil:
		return "", true
	default:
		out, err := json.Marshal(v)
		if err != nil {
			return "", false
		}
		return string(out), true
	}
}

func jsonNumberString(v float64) string {
	out, _ := json.Marshal(v)
	return string(out)
}

// renderValueTemplates mirrors `fn render_value_templates`. Walks a
// JSON value, rendering every string leaf as a template.
func renderValueTemplates(value any, context any) any {
	switch v := value.(type) {
	case string:
		return renderTemplate(v, context)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = renderValueTemplates(item, context)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, item := range v {
			out[k] = renderValueTemplates(item, context)
		}
		return out
	default:
		return v
	}
}

// buildNotificationMetadata mirrors `fn build_notification_metadata`.
// Renders templated `metadata` then merges canonical action/exec
// fields on top.
func buildNotificationMetadata(
	config notificationSideEffectConfig,
	context any,
	action models.ActionType,
	executed executedAction,
) map[string]any {
	var custom any = map[string]any{}
	if len(config.Metadata) > 0 && string(config.Metadata) != "null" {
		var raw any
		_ = json.Unmarshal(config.Metadata, &raw)
		custom = renderValueTemplates(raw, context)
	}
	header := map[string]any{
		"action_id":        action.ID,
		"action_name":      action.Name,
		"operation_kind":   action.OperationKind,
		"target_object_id": executed.targetObjectID,
		"deleted":          executed.deleted,
	}
	switch v := custom.(type) {
	case map[string]any:
		for k, val := range header {
			v[k] = val
		}
		return v
	default:
		header["custom"] = v
		return header
	}
}

// emitActionNotifications mirrors `async fn emit_action_notifications`.
// Best-effort: failures are logged + propagated to the caller (the
// caller's choice to suppress) so the audit trail can record the gap.
func emitActionNotifications(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	target *domain.ObjectInstance,
	parameters json.RawMessage,
	justification *string,
	executed executedAction,
) error {
	_, configs, err := splitActionConfig(action.Config)
	if err != nil {
		return err
	}
	if len(configs) == 0 {
		return nil
	}
	var paramMap map[string]json.RawMessage
	_ = json.Unmarshal(parameters, &paramMap)
	paramMapAny := map[string]any{}
	for k, v := range paramMap {
		var any any
		_ = json.Unmarshal(v, &any)
		paramMapAny[k] = any
	}
	context := map[string]any{
		"action": map[string]any{
			"id":             action.ID,
			"name":           action.Name,
			"display_name":   action.DisplayName,
			"description":    action.Description,
			"operation_kind": action.OperationKind,
			"object_type_id": action.ObjectTypeID,
		},
		"actor": map[string]any{
			"id":              claims.Sub,
			"email":           claims.Email,
			"roles":           claims.Roles,
			"organization_id": claims.OrgID,
		},
		"target":        objectInstanceAsAny(target),
		"parameters":    paramMapAny,
		"justification": justification,
		"preview":       jsonAsAny(executed.preview),
		"execution": map[string]any{
			"target_object_id": executed.targetObjectID,
			"deleted":          executed.deleted,
		},
		"object": jsonAsAny(executed.object),
		"link":   jsonAsAny(executed.link),
		"result": jsonAsAny(executed.result),
	}

	for _, cfg := range configs {
		recipients, broadcast, err := resolveNotificationRecipients(cfg, paramMap, target, claims.Sub)
		if err != nil {
			return err
		}
		title := renderTemplate(cfg.Title, context)
		body := renderTemplate(cfg.Body, context)
		metadata := buildNotificationMetadata(cfg, context, action, executed)

		if broadcast {
			if err := sendNotificationRequest(ctx, state, internalSendNotificationRequest{
				UserID:   nil,
				Title:    title,
				Body:     body,
				Severity: cfg.Severity,
				Category: cfg.Category,
				Channels: cfg.Channels,
				Metadata: metadata,
			}); err != nil {
				return err
			}
		}
		for _, uid := range recipients {
			id := uid
			if err := sendNotificationRequest(ctx, state, internalSendNotificationRequest{
				UserID:   &id,
				Title:    title,
				Body:     body,
				Severity: cfg.Severity,
				Category: cfg.Category,
				Channels: cfg.Channels,
				Metadata: metadata,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// internalSendNotificationRequest mirrors
// `struct InternalSendNotificationRequest`.
type internalSendNotificationRequest struct {
	UserID   *uuid.UUID `json:"user_id,omitempty"`
	Title    string     `json:"title"`
	Body     string     `json:"body"`
	Severity *string    `json:"severity,omitempty"`
	Category *string    `json:"category,omitempty"`
	Channels *[]string  `json:"channels,omitempty"`
	Metadata any        `json:"metadata,omitempty"`
}

// sendNotificationRequest mirrors `async fn send_notification_request`.
func sendNotificationRequest(
	ctx context.Context,
	state *ontologykernel.AppState,
	request internalSendNotificationRequest,
) error {
	url := strings.TrimRight(state.NotificationServiceURL, "/") + "/internal/notifications"
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode notification request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send action notification: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	client := state.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send action notification: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		respBytes := readAllLimited(resp.Body, 1<<20)
		return fmt.Errorf("notification service returned %s: %s", resp.Status, string(respBytes))
	}
	return nil
}

// ── Audit pipeline ─────────────────────────────────────────────────

// issueServiceToken mirrors `fn issue_service_token`. Mints a service
// JWT impersonating the actor so audit-service can attribute the
// event to the right org / classification level.
func issueServiceTokenFor(state *ontologykernel.AppState, claims *authmw.Claims, service string) (string, error) {
	id, _ := uuid.NewV7()
	if strings.TrimSpace(service) == "" {
		service = "ontology-service"
	}
	attrs, _ := json.Marshal(map[string]any{
		"service":                  service,
		"classification_clearance": "pii",
		"impersonated_actor_id":    claims.Sub,
	})
	c := authmw.BuildAccessClaims(state.JWTConfig, authmw.AccessClaimsInput{
		UserID:      id,
		Email:       service + "@internal.openfoundry",
		Name:        service,
		Roles:       []string{"admin"},
		Permissions: []string{"*:*"},
		OrgID:       claims.OrgID,
		Attributes:  attrs,
		AuthMethods: []string{"service"},
	})
	tok, err := authmw.EncodeToken(state.JWTConfig, &c)
	if err != nil {
		return "", fmt.Errorf("failed to issue service token for audit: %w", err)
	}
	return "Bearer " + tok, nil
}

func issueServiceToken(state *ontologykernel.AppState, claims *authmw.Claims) (string, error) {
	return issueServiceTokenFor(state, claims, "ontology-service")
}

// classificationForTarget mirrors `fn classification_for_target`.
func classificationForTarget(target *domain.ObjectInstance) string {
	if target == nil {
		return "public"
	}
	switch strings.ToLower(target.Marking) {
	case "confidential":
		return "confidential"
	case "pii":
		return "pii"
	}
	return "public"
}

// emitActionAuditEvent mirrors `async fn emit_action_audit_event`.
// Posts a structured audit event to audit-service. Failures are
// returned so the caller can log via WarnLogger.
func emitActionAuditEvent(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	target *domain.ObjectInstance,
	targetObjectID *uuid.UUID,
	status, severity, message string,
	justification *string,
	parameters json.RawMessage,
	preview json.RawMessage,
	result any,
) error {
	if strings.TrimSpace(state.AuditServiceURL) == "" {
		return nil
	}
	token, err := issueServiceToken(state, claims)
	if err != nil {
		return err
	}
	resourceID := action.ID.String()
	if targetObjectID != nil {
		resourceID = targetObjectID.String()
	}
	resourceType := "ontology_action"
	if targetObjectID != nil {
		resourceType = "ontology_object"
	}
	var paramAny any
	_ = json.Unmarshal(parameters, &paramAny)
	var previewAny any
	_ = json.Unmarshal(preview, &previewAny)
	metadata := map[string]any{
		"action_id":            action.ID,
		"action_name":          action.Name,
		"operation_kind":       action.OperationKind,
		"object_type_id":       action.ObjectTypeID,
		"permission_key":       action.PermissionKey,
		"authorization_policy": action.AuthorizationPolicy,
		"target_object_id":     targetObjectID,
		"justification":        justification,
		"parameters":           paramAny,
		"preview":              previewAny,
		"result":               result,
		"message":              optionalString(message),
		"actor_id":             claims.Sub,
		"actor_roles":          claims.Roles,
		"organization_id":      claims.OrgID,
	}
	body, _ := json.Marshal(map[string]any{
		"source_service": "ontology-service",
		"channel":        "api",
		"actor":          claims.Email,
		"action":         "ontology.action.execute",
		"resource_type":  resourceType,
		"resource_id":    resourceID,
		"status":         status,
		"severity":       severity,
		"classification": classificationForTarget(target),
		"subject_id":     claims.Sub.String(),
		"metadata":       metadata,
		"labels":         []string{"ontology", "action", status, action.OperationKind},
	})
	url := strings.TrimRight(state.AuditServiceURL, "/") + "/api/v1/audit/events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send audit event: %w", err)
	}
	req.Header.Set("authorization", token)
	req.Header.Set("content-type", "application/json")
	client := state.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send audit event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		respBytes := readAllLimited(resp.Body, 1<<20)
		return fmt.Errorf("audit service returned %s: %s", resp.Status, string(respBytes))
	}
	return nil
}

// logAuditFailure / logNotificationFailure mirror the warn loggers.
func logAuditFailure(actionID uuid.UUID, err error) {
	log.Printf("ontology action audit emit failed action=%s err=%s", actionID, err.Error())
}

func logNotificationFailure(actionID uuid.UUID, err error) {
	log.Printf("ontology action notification emit failed action=%s err=%s", actionID, err.Error())
}

func logKafkaAuditFailure(actionID uuid.UUID, err error) {
	log.Printf("ontology action kafka audit publish failed action=%s err=%s", actionID, err.Error())
}

// publishActionAuditToKafka emits a flat envelope to the
// `ontology.actions.applied.v1` topic so a Spark Structured Streaming sink
// (see infra/dev/action-log-sink.yaml) can append it to the Iceberg table
// `lakekeeper.default.action_log` for time-travel auditability.
//
// No-op when state.ActionAuditPublisher is nil. Failures are returned so the
// caller can warn-log; the kernel never short-circuits the action on a Kafka
// failure (audit-service HTTP path + revision log keep working).
func publishActionAuditToKafka(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action models.ActionType,
	target *domain.ObjectInstance,
	targetObjectID *uuid.UUID,
	status string,
	parameters json.RawMessage,
	previousState json.RawMessage,
	newState any,
) error {
	if state == nil || state.ActionAuditPublisher == nil {
		return nil
	}
	var orgID string
	if claims.OrgID != nil {
		orgID = claims.OrgID.String()
	}
	tenant := domain.TenantFromClaims(claims)
	objIDStr := ""
	if targetObjectID != nil {
		objIDStr = targetObjectID.String()
	}
	var paramAny, prevAny, newAny any
	_ = json.Unmarshal(parameters, &paramAny)
	_ = json.Unmarshal(previousState, &prevAny)
	switch v := newState.(type) {
	case json.RawMessage:
		_ = json.Unmarshal(v, &newAny)
	default:
		newAny = v
	}
	envelope := map[string]any{
		"event_id":        uuid.New().String(),
		"action_type_id":  action.ID.String(),
		"action_name":     action.Name,
		"object_type_id":  action.ObjectTypeID.String(),
		"object_id":       objIDStr,
		"tenant":          string(tenant),
		"actor_sub":       claims.Sub.String(),
		"actor_email":     claims.Email,
		"organization_id": orgID,
		"status":          status,
		"parameters":      paramAny,
		"previous_state":  prevAny,
		"new_state":       newAny,
		"applied_at_ms":   time.Now().UnixMilli(),
	}
	if target != nil {
		envelope["target_classification"] = classificationForTarget(target)
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal action audit envelope: %w", err)
	}
	key := []byte(objIDStr)
	if len(key) == 0 {
		key = []byte(action.ID.String())
	}
	return state.ActionAuditPublisher.PublishActionAudit(ctx, key, payload)
}

// ── helpers ────────────────────────────────────────────────────────

func optionalString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func objectInstanceAsAny(target *domain.ObjectInstance) any {
	if target == nil {
		return nil
	}
	out, _ := json.Marshal(target)
	var v any
	_ = json.Unmarshal(out, &v)
	return v
}

func jsonAsAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}
