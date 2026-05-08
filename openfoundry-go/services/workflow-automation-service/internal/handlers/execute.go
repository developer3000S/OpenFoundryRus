// HTTP execution handlers — every inbound producer path
// (`manual`, `webhook`, `lineage_build`, internal-triggered) lands
// here and publishes an `automate.condition.v1` event via the
// transactional outbox plus an `automation_runs` row in the same
// Postgres transaction.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/outbox"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/domain/automationrun"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/event"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/state"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/topics"
)

// StartManualRun ports `handlers::execute::start_manual_run`.
func (h *CrudHandlers) StartManualRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	claims, _ := authmw.FromContext(r.Context())
	var startedBy *uuid.UUID
	if claims != nil {
		s := claims.Sub
		startedBy = &s
	}
	var body models.StartRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workflow, ok := loadOr404(w, r, h.State, id)
	if !ok {
		return
	}
	run, err := DispatchRun(r.Context(), h.State, workflow, "manual", startedBy, body.Context)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// TriggerWebhook ports `handlers::execute::trigger_webhook`.
func (h *CrudHandlers) TriggerWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.TriggerEventRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workflow, ok := loadOr404(w, r, h.State, id)
	if !ok {
		return
	}
	if workflow.TriggerType != "webhook" {
		writeError(w, http.StatusBadRequest, "workflow is not configured for webhook triggers")
		return
	}
	if workflow.WebhookSecret != nil {
		actual := r.Header.Get("x-openfoundry-webhook-secret")
		if actual != *workflow.WebhookSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	context, err := json.Marshal(map[string]any{
		"trigger": map[string]any{
			"type":        "webhook",
			"workflow_id": id,
		},
		"payload": rawOrNull(body.Context),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	run, err := DispatchRun(r.Context(), h.State, workflow, "webhook", nil, context)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// StartInternalLineageRun ports `handlers::execute::start_internal_lineage_run`.
func (h *CrudHandlers) StartInternalLineageRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.InternalLineageRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workflow, ok := loadOr404(w, r, h.State, id)
	if !ok {
		return
	}
	if workflow.Status != "active" {
		writeError(w, http.StatusBadRequest, "workflow must be active to run from lineage")
		return
	}
	run, err := DispatchRun(r.Context(), h.State, workflow, "lineage_build", nil, body.Context)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// StartInternalTriggeredRun ports `handlers::execute::start_internal_triggered_run`.
func (h *CrudHandlers) StartInternalTriggeredRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.InternalTriggeredRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	run, err := ExecuteInternalTriggeredRun(r.Context(), h.State, id, body)
	if err != nil {
		status := http.StatusBadRequest
		if isNotFoundError(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// ExecuteInternalTriggeredRun ports `execute::execute_internal_triggered_run`.
// Exported because the NATS workflow-run-requested consumer reuses
// the same dispatch path.
func ExecuteInternalTriggeredRun(ctx context.Context, s *state.AppState, workflowID uuid.UUID, body models.InternalTriggeredRunRequest) (*models.WorkflowRun, error) {
	workflow, err := LoadWorkflow(ctx, s.DB, workflowID)
	if err != nil {
		slog.Error("internal triggered run lookup failed", slog.String("error", err.Error()))
		return nil, err
	}
	if workflow == nil {
		return nil, fmt.Errorf("workflow %s not found", workflowID)
	}
	return DispatchRun(ctx, s, workflow, body.TriggerType, body.StartedBy, body.Context)
}

// DispatchRun ports `handlers::execute::dispatch_run`. Inserts the
// automation_runs row at state=Queued and enqueues the
// automate.condition.v1 outbox event in the same Postgres transaction.
func DispatchRun(ctx context.Context, s *state.AppState, workflow *models.WorkflowDefinition, triggerType string, startedBy *uuid.UUID, contextRaw json.RawMessage) (*models.WorkflowRun, error) {
	if workflow.Status != "active" && triggerType != "manual" {
		return nil, errors.New("workflow must be active for automatic execution")
	}
	correlationID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	runID := event.DeriveRunID(workflow.ID, correlationID)
	tenantIDStr := workflowTenantID(workflow)

	condition := event.AutomateConditionV1{
		DefinitionID:   workflow.ID,
		TenantID:       tenantIDStr,
		CorrelationID:  correlationID,
		TriggeredBy:    triggeredByOr(startedBy),
		TriggerType:    triggerType,
		TriggerPayload: contextRaw,
	}
	run := automationrun.New(runID, event.TenantUUIDFromStr(tenantIDStr), workflow.ID, correlationID, nil)

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := insertAutomationRunInTx(ctx, tx, run); err != nil {
		return nil, err
	}
	if err := enqueueConditionInTx(ctx, tx, runID, &condition); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE workflows SET last_triggered_at = NOW(), updated_at = NOW() WHERE id = $1`,
		workflow.ID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return acceptedRun(workflow.ID, runID, triggerType, startedBy, contextRaw, correlationID), nil
}

func insertAutomationRunInTx(ctx context.Context, tx pgx.Tx, run *automationrun.AutomationRun) error {
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (id, state, state_data, version, expires_at, created_at, updated_at)
		      VALUES ($1, $2, $3, 1, $4, now(), now())
		    ON CONFLICT (id) DO NOTHING`, automationrun.TableName,
	), run.AggregateID(), run.CurrentState(), payload, run.ExpiresAt())
	return err
}

func enqueueConditionInTx(ctx context.Context, tx pgx.Tx, runID uuid.UUID, condition *event.AutomateConditionV1) error {
	payload, err := json.Marshal(condition)
	if err != nil {
		return err
	}
	eventID := event.DeriveConditionEventID(condition.DefinitionID, condition.CorrelationID)
	out := outbox.New(eventID, "automation_run", runID.String(), topics.AutomateConditionV1, payload).
		WithHeader("x-audit-correlation-id", condition.CorrelationID.String()).
		WithHeader("ol-job", fmt.Sprintf("automation_run/%s", condition.TenantID)).
		WithHeader("ol-run-id", runID.String()).
		WithHeader("ol-producer", "workflow-automation-service")
	return outbox.Enqueue(ctx, tx, out)
}

func loadOr404(w http.ResponseWriter, r *http.Request, s *state.AppState, workflowID uuid.UUID) (*models.WorkflowDefinition, bool) {
	workflow, err := LoadWorkflow(r.Context(), s.DB, workflowID)
	if err != nil {
		slog.Error("workflow lookup failed", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return nil, false
	}
	if workflow == nil {
		w.WriteHeader(http.StatusNotFound)
		return nil, false
	}
	return workflow, true
}

func workflowTenantID(workflow *models.WorkflowDefinition) string {
	if len(workflow.TriggerConfig) > 0 {
		var holder map[string]any
		if err := json.Unmarshal(workflow.TriggerConfig, &holder); err == nil {
			if v, ok := holder["tenant_id"].(string); ok {
				return v
			}
		}
	}
	return workflow.OwnerID.String()
}

func triggeredByOr(startedBy *uuid.UUID) string {
	if startedBy == nil {
		return "system"
	}
	return startedBy.String()
}

func rawOrNull(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

// acceptedRun mirrors `handlers::execute::accepted_run`.
func acceptedRun(workflowID, runID uuid.UUID, triggerType string, startedBy *uuid.UUID, contextRaw json.RawMessage, correlationID uuid.UUID) *models.WorkflowRun {
	context, err := json.Marshal(map[string]any{
		"input": rawOrEmptyObject(contextRaw),
		"automate": map[string]any{
			"run_id":         runID,
			"correlation_id": correlationID,
			"topic":          topics.AutomateConditionV1,
			// Compatibility shim: the legacy field name
			// `temporal.authoritative` is what the UI currently
			// pattern-matches on. We map it to false so the front-end
			// falls back to reading the live row from `GET /workflows/
			// {id}/runs` (Postgres-backed, single source of truth).
			"authoritative": false,
		},
	})
	if err != nil {
		context = json.RawMessage(`{}`)
	}
	return &models.WorkflowRun{
		ID:          runID,
		WorkflowID:  workflowID,
		TriggerType: triggerType,
		Status:      "running",
		StartedBy:   startedBy,
		Context:     context,
		StartedAt:   time.Now().UTC(),
	}
}

func rawOrEmptyObject(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var holder any
	if err := json.Unmarshal(raw, &holder); err != nil {
		return map[string]any{}
	}
	return holder
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "not found")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
