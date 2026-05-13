package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

func TestLogicFileValidationRequiresProjectFolderPlacement(t *testing.T) {
	t.Parallel()
	assert.False(t, validateLogicName("   "))
	assert.True(t, validateLogicName("Summarize complaint"))
	assert.True(t, isZeroUUID(uuid.Nil))
	assert.False(t, isZeroUUID(uuid.New()))
}

func TestValidateExecutionMode(t *testing.T) {
	t.Parallel()
	userScoped := "user_scoped"
	projectScoped := "project_scoped"
	bad := "personal_home"
	assert.True(t, validateExecutionMode(nil))
	assert.True(t, validateExecutionMode(&userScoped))
	assert.True(t, validateExecutionMode(&projectScoped))
	assert.False(t, validateExecutionMode(&bad))
}

func TestParseOptionalUUIDQuery(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	got, err := parseOptionalUUIDQuery(id.String())
	assert.NoError(t, err)
	assert.Equal(t, id, *got)

	got, err = parseOptionalUUIDQuery("   ")
	assert.NoError(t, err)
	assert.Nil(t, got)

	_, err = parseOptionalUUIDQuery("not-a-uuid")
	assert.Error(t, err)
}

func TestCreateLogicFileRequiresAuthBeforeBodyValidation(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/logic/files", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.CreateLogicFile(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "authentication required")
}

func TestCreateLogicFileRejectsPersonalOnlyPlacement(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	claims := &authmw.Claims{Sub: uuid.New()}
	req := httptest.NewRequest(http.MethodPost, "/logic/files", strings.NewReader(`{"name":"Draft"}`))
	req = req.WithContext(authmw.ContextWithClaims(req.Context(), claims))
	rec := httptest.NewRecorder()
	h.CreateLogicFile(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "project_id and folder_id")
}

func TestUpdateLogicFileRejectsInvalidExecutionModeBeforeRepo(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	claims := &authmw.Claims{Sub: uuid.New()}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	req := httptest.NewRequest(http.MethodPatch, "/logic/files/id", strings.NewReader(`{"execution_mode":"personal_home"}`))
	req = req.WithContext(authmw.ContextWithClaims(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), claims))
	rec := httptest.NewRecorder()
	h.UpdateLogicFileMetadata(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "execution_mode")
}

func TestDuplicateLogicFileRequiresProjectAndFolderTogether(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	claims := &authmw.Claims{Sub: uuid.New()}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())
	body := `{"project_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/logic/files/id/duplicate", strings.NewReader(body))
	req = req.WithContext(authmw.ContextWithClaims(context.WithValue(req.Context(), chi.RouteCtxKey, rctx), claims))
	rec := httptest.NewRecorder()
	h.DuplicateLogicFile(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "provided together")
}
