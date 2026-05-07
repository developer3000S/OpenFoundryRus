package funnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func sampleSource(status string) models.OntologyFunnelSource {
	return models.OntologyFunnelSource{
		ID:               uuid.New(),
		Name:             "tickets-batch",
		ObjectTypeID:     uuid.New(),
		DatasetID:        uuid.New(),
		PreviewLimit:     100,
		DefaultMarking:   "public",
		Status:           status,
		PropertyMappings: []models.OntologyFunnelPropertyMapping{},
		OwnerID:          uuid.New(),
	}
}

func sampleMetrics(latest *string, totalRuns int64, lastRunAt *time.Time) models.OntologyFunnelHealthMetricsRow {
	out := models.OntologyFunnelHealthMetricsRow{
		TotalRuns:       totalRuns,
		LatestRunStatus: latest,
		LastRunAt:       lastRunAt,
		RowsRead:        100,
		InsertedCount:   40,
		UpdatedCount:    60,
	}
	if latest != nil {
		switch *latest {
		case "completed", "dry_run":
			out.SuccessfulRuns = totalRuns
			out.LastSuccessAt = lastRunAt
		case "failed":
			out.FailedRuns = 1
			out.LastFailureAt = lastRunAt
		case "completed_with_errors", "dry_run_with_errors":
			out.WarningRuns = 1
			out.LastWarningAt = lastRunAt
			out.ErrorCount = 3
		}
	}
	return out
}

// Mirrors `classifies_healthy_source_when_latest_run_completed`.
func TestBuildSourceHealth_HealthyOnLatestCompleted(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	completed := "completed"
	got := BuildSourceHealth(sampleSource("active"), sampleMetrics(&completed, 4, &now), 24)
	if got.HealthStatus != "healthy" {
		t.Fatalf("expected healthy, got %s", got.HealthStatus)
	}
}

// Mirrors `classifies_failing_source_when_latest_run_failed`.
func TestBuildSourceHealth_FailingOnLatestFailed(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	failed := "failed"
	got := BuildSourceHealth(sampleSource("active"), sampleMetrics(&failed, 4, &now), 24)
	if got.HealthStatus != "failing" {
		t.Fatalf("expected failing, got %s", got.HealthStatus)
	}
}

// Mirrors `classifies_stale_source_when_last_run_is_too_old`.
func TestBuildSourceHealth_StaleWhenLastRunTooOld(t *testing.T) {
	t.Parallel()
	old := time.Now().UTC().Add(-48 * time.Hour)
	completed := "completed"
	got := BuildSourceHealth(sampleSource("active"), sampleMetrics(&completed, 4, &old), 24)
	if got.HealthStatus != "stale" {
		t.Fatalf("expected stale, got %s (last_run_at=%v)", got.HealthStatus, got.LastRunAt)
	}
}

// Mirrors `classifies_paused_source_before_considering_runs`.
func TestBuildSourceHealth_PausedBeforeRunsConsidered(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	failed := "failed"
	got := BuildSourceHealth(sampleSource("paused"), sampleMetrics(&failed, 4, &now), 24)
	if got.HealthStatus != "paused" {
		t.Fatalf("expected paused, got %s", got.HealthStatus)
	}
}

// Mirrors `classifies_never_run_source_without_history`.
func TestBuildSourceHealth_NeverRunWithoutHistory(t *testing.T) {
	t.Parallel()
	got := BuildSourceHealth(sampleSource("active"), sampleMetrics(nil, 0, nil), 24)
	if got.HealthStatus != "never_run" {
		t.Fatalf("expected never_run, got %s", got.HealthStatus)
	}
}

func TestFunnelHealthSortRank(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		"failing":   0,
		"degraded":  1,
		"stale":     2,
		"never_run": 3,
		"paused":    4,
		"healthy":   5,
		"unknown":   6,
	}
	for status, want := range cases {
		if got := funnelHealthSortRank(status); got != want {
			t.Errorf("rank(%s) = %d, want %d", status, got, want)
		}
	}
}

func TestEnsureOwnerOrAdmin(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	other := uuid.New()
	// admin bypass.
	if err := ensureOwnerOrAdmin(owner, mustClaims(other, []string{"admin"})); err != nil {
		t.Fatalf("admin must bypass: %v", err)
	}
	// owner OK.
	if err := ensureOwnerOrAdmin(owner, mustClaims(owner, []string{"member"})); err != nil {
		t.Fatalf("owner must pass: %v", err)
	}
	// non-owner non-admin → forbidden.
	if err := ensureOwnerOrAdmin(owner, mustClaims(other, []string{"member"})); err == nil {
		t.Fatal("expected forbidden")
	}
}

func TestValidateSourceStatus(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"active", "paused", "  active  "} {
		if err := validateSourceStatus(ok); err != nil {
			t.Errorf("expected %q to be valid: %v", ok, err)
		}
	}
	if err := validateSourceStatus("draft"); err == nil {
		t.Fatal("expected draft to fail")
	}
}

func TestClampPreviewLimit(t *testing.T) {
	t.Parallel()
	cases := map[int32]int32{0: 1, 500: 500, 5000: 1000}
	for in, want := range cases {
		if got := clampPreviewLimit(in); got != want {
			t.Errorf("clamp(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestFunnelSourceLifecycleAndDryRunRunWithInMemoryStores(t *testing.T) {
	ctx := context.Background()
	state := &ontologykernel.AppState{Stores: stores.NewInMemory(), JWTConfig: authmw.NewJWTConfig("test-secret")}
	objectTypeID := uuid.New()
	datasetID := uuid.New()
	owner := uuid.New()
	seedFunnelObjectTypeDefinition(t, state, objectTypeID, "external_id")
	seedFunnelPropertyDefinition(t, state, objectTypeID, "external_id", "string")
	seedFunnelPropertyDefinition(t, state, objectTypeID, "status", "string")

	datasetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/datasets/"+datasetID.String()+"/preview") {
			t.Fatalf("dataset preview path drift: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "25" {
			t.Fatalf("preview limit drift: %s", r.URL.RawQuery)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"total_rows":1,"rows":[{"external_id":"ticket-1","status":"open"}],"warnings":[],"errors":[]}`))
	}))
	defer datasetServer.Close()
	state.DatasetServiceURL = datasetServer.URL
	state.HTTPClient = datasetServer.Client()

	createBody := fmt.Sprintf(`{"name":"Tickets","object_type_id":"%s","dataset_id":"%s","preview_limit":25,"property_mappings":[{"source_field":"external_id","target_property":"external_id"},{"source_field":"status","target_property":"status"}]}`, objectTypeID, datasetID)
	createRec := httptest.NewRecorder()
	CreateFunnelSource(state).ServeHTTP(createRec, funnelRequest(ctx, http.MethodPost, "/funnel/sources", createBody, nil, owner, []string{"member"}))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var source models.OntologyFunnelSource
	if err := json.Unmarshal(createRec.Body.Bytes(), &source); err != nil {
		t.Fatalf("created source json: %v", err)
	}

	getRec := httptest.NewRecorder()
	GetFunnelSource(state).ServeHTTP(getRec, funnelRequest(ctx, http.MethodGet, "/funnel/sources/"+source.ID.String(), ``, map[string]string{"id": source.ID.String()}, owner, []string{"member"}))
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRec.Code, getRec.Body.String())
	}

	patchRec := httptest.NewRecorder()
	UpdateFunnelSource(state).ServeHTTP(patchRec, funnelRequest(ctx, http.MethodPatch, "/funnel/sources/"+source.ID.String(), `{"status":"paused"}`, map[string]string{"id": source.ID.String()}, owner, []string{"member"}))
	if patchRec.Code != http.StatusOK || !strings.Contains(patchRec.Body.String(), `"status":"paused"`) {
		t.Fatalf("patch status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}
	UpdateFunnelSource(state).ServeHTTP(httptest.NewRecorder(), funnelRequest(ctx, http.MethodPatch, "/funnel/sources/"+source.ID.String(), `{"status":"active"}`, map[string]string{"id": source.ID.String()}, owner, []string{"member"}))

	runRec := httptest.NewRecorder()
	TriggerFunnelRun(state).ServeHTTP(runRec, funnelRequest(ctx, http.MethodPost, "/funnel/sources/"+source.ID.String()+"/run", `{"dry_run":true,"skip_pipeline":true}`, map[string]string{"id": source.ID.String()}, owner, []string{"member"}))
	if runRec.Code != http.StatusOK {
		t.Fatalf("run status=%d body=%s", runRec.Code, runRec.Body.String())
	}
	if !strings.Contains(runRec.Body.String(), `"status":"dry_run"`) || !strings.Contains(runRec.Body.String(), `"inserted_count":1`) {
		t.Fatalf("run response drift: %s", runRec.Body.String())
	}

	listRunsRec := httptest.NewRecorder()
	ListFunnelRuns(state).ServeHTTP(listRunsRec, funnelRequest(ctx, http.MethodGet, "/funnel/sources/"+source.ID.String()+"/runs", ``, map[string]string{"id": source.ID.String()}, owner, []string{"member"}))
	if listRunsRec.Code != http.StatusOK || !strings.Contains(listRunsRec.Body.String(), `"total":1`) {
		t.Fatalf("list runs status=%d body=%s", listRunsRec.Code, listRunsRec.Body.String())
	}

	healthRec := httptest.NewRecorder()
	GetFunnelSourceHealth(state).ServeHTTP(healthRec, funnelRequest(ctx, http.MethodGet, "/funnel/sources/"+source.ID.String()+"/health", ``, map[string]string{"id": source.ID.String()}, owner, []string{"member"}))
	if healthRec.Code != http.StatusOK || !strings.Contains(healthRec.Body.String(), `"health_status":"healthy"`) {
		t.Fatalf("source health status=%d body=%s", healthRec.Code, healthRec.Body.String())
	}

	deleteRec := httptest.NewRecorder()
	DeleteFunnelSource(state).ServeHTTP(deleteRec, funnelRequest(ctx, http.MethodDelete, "/funnel/sources/"+source.ID.String(), ``, map[string]string{"id": source.ID.String()}, owner, []string{"member"}))
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func funnelRequest(ctx context.Context, method, path, body string, params map[string]string, sub uuid.UUID, roles []string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	claims := &authmw.Claims{Sub: sub, Email: "funnel@example.com", Roles: roles}
	ctx = context.WithValue(authmw.ContextWithClaims(ctx, claims), chi.RouteCtxKey, rctx)
	return httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
}

func seedFunnelObjectTypeDefinition(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, primaryKey string) {
	t.Helper()
	now := time.Now().UTC()
	payload, _ := json.Marshal(models.ObjectType{ID: objectTypeID, Name: "ticket", DisplayName: "Ticket", PrimaryKeyProperty: &primaryKey, OwnerID: uuid.New(), CreatedAt: now, UpdatedAt: now})
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{Kind: storage.DefinitionKind(domain.ActionRepoObjectKind), ID: storage.DefinitionId(objectTypeID.String()), Payload: payload}, nil)
	if err != nil {
		t.Fatalf("seed object type: %v", err)
	}
}

func seedFunnelPropertyDefinition(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, name, propertyType string) {
	t.Helper()
	now := time.Now().UTC()
	propertyID := uuid.New()
	payload, _ := json.Marshal(models.Property{ID: propertyID, ObjectTypeID: objectTypeID, Name: name, DisplayName: name, PropertyType: propertyType, CreatedAt: now, UpdatedAt: now})
	parent := storage.DefinitionId(objectTypeID.String())
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{Kind: storage.DefinitionKind(domain.ActionRepoPropertyKind), ID: storage.DefinitionId(propertyID.String()), ParentID: &parent, Payload: payload}, nil)
	if err != nil {
		t.Fatalf("seed property: %v", err)
	}
}
