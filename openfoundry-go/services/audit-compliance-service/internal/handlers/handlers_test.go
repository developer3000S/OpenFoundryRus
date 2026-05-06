package handlers_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/audit-compliance-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/audit-compliance-service/internal/models"
)

// Wire-format pinning: AuditEvent has the full hash-chain field set.
func TestAuditEventJSONShape(t *testing.T) {
	t.Parallel()
	subj := "subject-1"
	e := models.AuditEvent{
		ID: uuid.New(), Sequence: 42,
		PreviousHash: "AUD-prev", EntryHash: "AUD-curr",
		SourceService: "gateway", Channel: "http",
		Actor: "user:x", Action: "read", ResourceType: "dataset", ResourceID: "ds-1",
		Status: "success", Severity: "low", Classification: "public",
		SubjectID: &subj,
		Metadata:  json.RawMessage(`{}`), Labels: json.RawMessage(`[]`),
		RetentionUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		OccurredAt:     time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		IngestedAt:     time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(e)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"id", "sequence", "previous_hash", "entry_hash", "source_service",
		"channel", "actor", "action", "resource_type", "resource_id",
		"status", "severity", "classification", "subject_id", "ip_address",
		"location", "metadata", "labels", "retention_until",
		"occurred_at", "ingested_at",
	} {
		assert.Contains(t, view, k)
	}
}

// RetentionPolicy carries is_system + grace + selector/criteria from the
// 0005_retention_system_policies migration.
func TestRetentionPolicyJSONShape(t *testing.T) {
	t.Parallel()
	p := models.RetentionPolicy{
		ID: uuid.New(), Name: "p", Scope: "system",
		TargetKind: "transaction", RetentionDays: 0, LegalHold: false,
		PurgeMode: "hard-delete-after-ttl",
		Rules:     json.RawMessage(`["x"]`),
		UpdatedBy: "system", Active: true, IsSystem: true,
		Selector: json.RawMessage(`{"all_datasets":true}`),
		Criteria: json.RawMessage(`{"transaction_state":"ABORTED"}`),
		GracePeriodMinutes: 15,
		CreatedAt:          time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(p)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"is_system", "selector", "criteria", "grace_period_minutes",
		"last_applied_at", "next_run_at",
	} {
		assert.Contains(t, view, k)
	}
}

// Validation paths (no DB).

func TestCreateRetentionPolicyRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("POST", "/retention-policies",
		strings.NewReader(`{"name":"x","target_kind":"dataset","purge_mode":"archive"}`))
	rec := httptest.NewRecorder()
	h.CreateRetentionPolicy(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestCreateRetentionPolicyRejectsEmptyFields(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("POST", "/retention-policies",
		strings.NewReader(`{"name":"","target_kind":"","purge_mode":""}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.CreateRetentionPolicy(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "name, target_kind, purge_mode required")
}

func TestCreateRetentionPolicyRejectsNegativeDays(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("POST", "/retention-policies",
		strings.NewReader(`{"name":"x","target_kind":"d","purge_mode":"archive","retention_days":-1}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.CreateRetentionPolicy(rec, req)
	assert.Equal(t, 400, rec.Code)
}

func TestCreateLineageDeletionRequiresDatasetID(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("POST", "/lineage-deletion-requests",
		strings.NewReader(`{}`))
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.CreateLineageDeletionRequest(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "dataset_id required")
}

func TestListAuditEventsRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &handlers.Handlers{}
	req := httptest.NewRequest("GET", "/audit-events", nil)
	rec := httptest.NewRecorder()
	h.ListAuditEvents(rec, req)
	assert.Equal(t, 401, rec.Code)
}
