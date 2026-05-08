package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/workflow-automation-service/internal/topics"
)

func TestAcceptedRunReturnsAPICompatibleState(t *testing.T) {
	workflowID := uuid.New()
	runID := uuid.New()
	startedBy := uuid.New()
	correlationID := uuid.New()

	run := acceptedRun(
		workflowID,
		runID,
		"manual",
		&startedBy,
		json.RawMessage(`{"customer_id":"c-1"}`),
		correlationID,
	)

	if run.ID != runID {
		t.Fatalf("ID = %s, want %s", run.ID, runID)
	}
	if run.WorkflowID != workflowID {
		t.Fatalf("WorkflowID = %s, want %s", run.WorkflowID, workflowID)
	}
	if run.Status != "running" {
		t.Fatalf("Status = %q, want running", run.Status)
	}
	if run.StartedBy == nil || *run.StartedBy != startedBy {
		t.Fatalf("StartedBy = %v, want %s", run.StartedBy, startedBy)
	}

	var context struct {
		Input struct {
			CustomerID string `json:"customer_id"`
		} `json:"input"`
		Automate struct {
			RunID         uuid.UUID `json:"run_id"`
			CorrelationID uuid.UUID `json:"correlation_id"`
			Topic         string    `json:"topic"`
			Authoritative bool      `json:"authoritative"`
		} `json:"automate"`
	}
	if err := json.Unmarshal(run.Context, &context); err != nil {
		t.Fatalf("unmarshal run context: %v", err)
	}
	if context.Input.CustomerID != "c-1" {
		t.Fatalf("input.customer_id = %q, want c-1", context.Input.CustomerID)
	}
	if context.Automate.RunID != runID {
		t.Fatalf("automate.run_id = %s, want %s", context.Automate.RunID, runID)
	}
	if context.Automate.CorrelationID != correlationID {
		t.Fatalf("automate.correlation_id = %s, want %s", context.Automate.CorrelationID, correlationID)
	}
	if context.Automate.Topic != topics.AutomateConditionV1 {
		t.Fatalf("automate.topic = %q, want %q", context.Automate.Topic, topics.AutomateConditionV1)
	}
	if context.Automate.Authoritative {
		t.Fatal("automate.authoritative = true, want false")
	}
}

func TestWorkflowTenantIDPrefersTriggerConfigTenant(t *testing.T) {
	ownerID := uuid.New()
	workflow := &models.WorkflowDefinition{
		OwnerID:       ownerID,
		TriggerConfig: json.RawMessage(`{"tenant_id":"tenant-a"}`),
	}
	if got := workflowTenantID(workflow); got != "tenant-a" {
		t.Fatalf("workflowTenantID = %q, want tenant-a", got)
	}
}

func TestWorkflowTenantIDFallsBackToOwnerID(t *testing.T) {
	ownerID := uuid.New()
	workflow := &models.WorkflowDefinition{
		OwnerID:       ownerID,
		TriggerConfig: json.RawMessage(`{"tenant_id":42}`),
	}
	if got := workflowTenantID(workflow); got != ownerID.String() {
		t.Fatalf("workflowTenantID = %q, want %s", got, ownerID)
	}
}

func TestStartManualRunRejectsInvalidWorkflowID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/workflows/not-a-uuid/runs", strings.NewReader(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req = req.WithContext(contextWithRoute(req, rctx))
	rr := httptest.NewRecorder()

	NewCrudHandlers(nil).StartManualRun(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestTriggerWebhookRejectsInvalidWorkflowID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/not-a-uuid", strings.NewReader(`{}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "not-a-uuid")
	req = req.WithContext(contextWithRoute(req, rctx))
	rr := httptest.NewRecorder()

	NewCrudHandlers(nil).TriggerWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func contextWithRoute(req *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
}
