package workspace_test

// Validation tests for the trash handlers. Mirror the `mod tests`
// surface of services/tenancy-organizations-service/src/handlers/trash.rs
// — every code path that does not require a database is exercised here
// so the request taxonomy matches Rust byte-exact (status code + JSON
// envelope). DB-dependent paths (LIST/UPDATE/DELETE) are covered by
// integration tests in this package.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/workspace"
)

// ─── JSON shape pinning ──────────────────────────────────────────────

func TestTrashEntryJSONShape(t *testing.T) {
	t.Parallel()
	deletedBy := uuid.New()
	projectID := uuid.New()
	e := workspace.TrashEntry{
		ResourceKind: "ontology_folder",
		ResourceID:   uuid.New(),
		ProjectID:    &projectID,
		DisplayName:  "demo",
		DeletedAt:    time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		DeletedBy:    &deletedBy,
	}
	out, err := json.Marshal(e)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"resource_kind", "resource_id", "project_id",
		"display_name", "deleted_at", "deleted_by",
	} {
		assert.Contains(t, view, k)
	}
	assert.Equal(t, "ontology_folder", view["resource_kind"])
}

func TestTrashEntryJSONNullsForUnset(t *testing.T) {
	t.Parallel()
	// Trashed projects have NULL project_id; legacy rows may have NULL
	// deleted_by. Both must serialize as JSON null (not be omitted) so
	// the UI can rely on the field always being present.
	e := workspace.TrashEntry{
		ResourceKind: "ontology_project",
		ResourceID:   uuid.New(),
		DisplayName:  "p",
		DeletedAt:    time.Now().UTC(),
	}
	out, err := json.Marshal(e)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	assert.Contains(t, view, "project_id")
	assert.Nil(t, view["project_id"])
	assert.Contains(t, view, "deleted_by")
	assert.Nil(t, view["deleted_by"])
}

func TestListTrashEnvelopeIsData(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(workspace.ListTrashResponse{Data: []workspace.TrashEntry{}})
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	assert.Contains(t, view, "data",
		"workspace surface uses {data:[...]} (NOT {items}) — pinned for wire-compat with Rust impl")
	assert.NotContains(t, view, "items")
}

// ─── ListTrash ───────────────────────────────────────────────────────

func TestListTrashRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	req := httptest.NewRequest("GET", "/workspace/trash", nil)
	rec := httptest.NewRecorder()
	h.ListTrash(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestListTrashRejectsBadKind(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest("GET", "/workspace/trash?kind=banana", nil)
	req = req.WithContext(authmw.ContextWithClaims(context.Background(), c))
	rec := httptest.NewRecorder()
	h.ListTrash(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "banana")
}

// ─── RestoreResource ─────────────────────────────────────────────────

func TestRestoreResourceRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	req := httptest.NewRequest("POST", "/x/restore", nil)
	rec := httptest.NewRecorder()
	h.RestoreResource(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestRestoreResourceRejectsBadKind(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("kind", "banana")
	rctx.URLParams.Add("id", uuid.New().String())
	req := httptest.NewRequest("POST", "/x/restore", nil)
	req = req.WithContext(authmw.ContextWithClaims(
		context.WithValue(req.Context(), chi.RouteCtxKey, rctx), c))
	rec := httptest.NewRecorder()
	h.RestoreResource(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "banana")
}

func TestRestoreResourceRejectsBadID(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("kind", "ontology_project")
	rctx.URLParams.Add("id", "not-a-uuid")
	req := httptest.NewRequest("POST", "/x/restore", nil)
	req = req.WithContext(authmw.ContextWithClaims(
		context.WithValue(req.Context(), chi.RouteCtxKey, rctx), c))
	rec := httptest.NewRecorder()
	h.RestoreResource(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid resource id")
}

// ─── PurgeResource ───────────────────────────────────────────────────

func TestPurgeResourceRequiresAuth(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	req := httptest.NewRequest("DELETE", "/x/purge", nil)
	rec := httptest.NewRecorder()
	h.PurgeResource(rec, req)
	assert.Equal(t, 401, rec.Code)
}

func TestPurgeResourceRejectsBadKind(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("kind", "banana")
	rctx.URLParams.Add("id", uuid.New().String())
	req := httptest.NewRequest("DELETE", "/x/purge", nil)
	req = req.WithContext(authmw.ContextWithClaims(
		context.WithValue(req.Context(), chi.RouteCtxKey, rctx), c))
	rec := httptest.NewRecorder()
	h.PurgeResource(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "banana")
}

func TestPurgeResourceRejectsBadID(t *testing.T) {
	t.Parallel()
	h := &workspace.Handlers{}
	c := &authmw.Claims{Sub: uuid.New()}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("kind", "ontology_folder")
	rctx.URLParams.Add("id", "not-a-uuid")
	req := httptest.NewRequest("DELETE", "/x/purge", nil)
	req = req.WithContext(authmw.ContextWithClaims(
		context.WithValue(req.Context(), chi.RouteCtxKey, rctx), c))
	rec := httptest.NewRecorder()
	h.PurgeResource(rec, req)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid resource id")
}
