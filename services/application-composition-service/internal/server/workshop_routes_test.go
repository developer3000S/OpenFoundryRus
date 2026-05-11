package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/repo"
)

func TestWorkshopWidgetCatalogAndTemplateRoutes(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router, jwt := testWorkshopRouter(mock)
	subject := uuid.New()

	catalogRR := httptest.NewRecorder()
	catalogReq := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/catalog", nil)
	catalogReq.Header.Set("Authorization", "Bearer "+testWorkshopToken(t, jwt, subject))
	router.ServeHTTP(catalogRR, catalogReq)
	require.Equal(t, http.StatusOK, catalogRR.Code)
	require.Equal(t, "2026-05-11.ws.22", catalogRR.Header().Get("X-OpenFoundry-Widget-Catalog-Version"))
	var catalog []map[string]any
	require.NoError(t, json.NewDecoder(catalogRR.Body).Decode(&catalog))
	require.NotEmpty(t, catalog)
	require.Equal(t, "text", catalog[0]["widget_type"])
	require.Equal(t, "content", catalog[0]["widget_kind"])
	require.Contains(t, catalog[0], "config_schema")
	require.Contains(t, catalog[0], "input_variables")
	require.Contains(t, catalog[0], "output_variables")
	require.Contains(t, catalog[0], "events")
	require.Contains(t, catalog[0], "permissions")
	require.Contains(t, catalog[0], "display")

	templateID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, key, name, description, category, preview_image_url, definition, created_at")).
		WillReturnRows(pgxmock.NewRows([]string{"id", "key", "name", "description", "category", "preview_image_url", "definition", "created_at"}).
			AddRow(templateID, "trail-demo", "Trail Demo", "Trail running starter", "demo", nil, []byte(`{"pages":[],"theme":{},"settings":{}}`), now))

	templatesRR := httptest.NewRecorder()
	templatesReq := httptest.NewRequest(http.MethodGet, "/api/v1/apps/templates", nil)
	templatesReq.Header.Set("Authorization", "Bearer "+testWorkshopToken(t, jwt, subject))
	router.ServeHTTP(templatesRR, templatesReq)
	require.Equal(t, http.StatusOK, templatesRR.Code)
	var templates map[string][]map[string]any
	require.NoError(t, json.NewDecoder(templatesRR.Body).Decode(&templates))
	require.Len(t, templates["data"], 1)
	require.Equal(t, "trail-demo", templates["data"][0]["key"])

	templateKey := "trail-demo"
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, key, name, description, category, preview_image_url, definition, created_at")).
		WithArgs(templateKey).
		WillReturnRows(pgxmock.NewRows([]string{"id", "key", "name", "description", "category", "preview_image_url", "definition", "created_at"}).
			AddRow(templateID, templateKey, "Trail Demo", "Trail running starter", "demo", nil, []byte(`{"pages":[{"id":"main","name":"Main","path":"/","layout":{"kind":"grid"},"widgets":[],"visible":true}],"theme":{},"settings":{}}`), now))
	createdAppID := uuid.New()
	mock.ExpectQuery("INSERT INTO apps").
		WithArgs(pgxmock.AnyArg(), "Trail Starter", "trail-starter", "Trail running starter", "draft", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(appRows(createdAppID, []byte(`[{"id":"main","name":"Main","path":"/","layout":{"kind":"grid","columns":12},"widgets":[],"visible":true}]`), []byte(`{}`), []byte(`{"schema_version":"2026-05-11.ws.1"}`), now))
	fromTemplateRR := authedRequest(t, router, jwt, subject, http.MethodPost, "/api/v1/apps/from-template", []byte(`{"name":"Trail Starter","template_key":"trail-demo"}`))
	require.Equal(t, http.StatusCreated, fromTemplateRR.Code)
	var created map[string]any
	require.NoError(t, json.NewDecoder(fromTemplateRR.Body).Decode(&created))
	require.Equal(t, "Trail Demo", created["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkshopPreviewPageAndSlateRoutes(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router, jwt := testWorkshopRouter(mock)
	subject := uuid.New()
	appID := uuid.New()
	now := time.Now().UTC()
	pageMain := []byte(`[{"id":"main","name":"Main","path":"/","layout":{"kind":"grid","columns":12},"widgets":[],"visible":true}]`)
	pageDetail := []byte(`[{"id":"main","name":"Main","path":"/","layout":{"kind":"grid","columns":12},"widgets":[],"visible":true},{"id":"detail","name":"Detail","path":"/detail","layout":{"kind":"grid","columns":12},"widgets":[],"visible":true}]`)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1")).
		WithArgs(appID).
		WillReturnRows(appRows(appID, pageMain, []byte(`{}`), []byte(`{}`), now))
	previewRR := authedRequest(t, router, jwt, subject, http.MethodGet, "/api/v1/apps/"+appID.String()+"/preview", nil)
	if previewRR.Code != http.StatusOK {
		t.Logf("preview body: %q", previewRR.Body.String())
	}
	require.Equalf(t, http.StatusOK, previewRR.Code, "body: %q", previewRR.Body.String())
	var preview map[string]any
	require.NoError(t, json.NewDecoder(previewRR.Body).Decode(&preview))
	require.Contains(t, preview, "app")
	require.Contains(t, preview, "embed")
	require.Equal(t, "draft", preview["preview_mode"])
	require.NotEmpty(t, preview["widget_catalog"])

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1")).
		WithArgs(appID).
		WillReturnRows(appRows(appID, pageMain, []byte(`{}`), []byte(`{}`), now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1")).
		WithArgs(appID).
		WillReturnRows(appRows(appID, pageMain, []byte(`{}`), []byte(`{}`), now))
	mock.ExpectQuery("UPDATE apps SET").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), appID).
		WillReturnRows(appRows(appID, pageDetail, []byte(`{}`), []byte(`{"schema_version":"2026-05-11.ws.1"}`), now))
	addPageRR := authedRequest(t, router, jwt, subject, http.MethodPost, "/api/v1/apps/"+appID.String()+"/pages", []byte(`{"id":"detail","name":"Detail","path":"/detail","layout":{"kind":"grid"},"widgets":[],"visible":true}`))
	require.Equal(t, http.StatusOK, addPageRR.Code)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1")).
		WithArgs(appID).
		WillReturnRows(appRows(appID, pageMain, []byte(`{}`), []byte(`{}`), now))
	slateRR := authedRequest(t, router, jwt, subject, http.MethodGet, "/api/v1/apps/"+appID.String()+"/slate-package", nil)
	require.Equal(t, http.StatusOK, slateRR.Code)
	var slate map[string]any
	require.NoError(t, json.NewDecoder(slateRR.Body).Decode(&slate))
	require.Equal(t, "trail-demo", slate["app_slug"])
	require.NotEmpty(t, slate["files"])

	importedSettings := []byte(`{"slate":{"enabled":true,"framework":"react","package_name":"@open-foundry/workshop-app","entry_file":"src/App.tsx","sdk_import":"@open-foundry/sdk/react","workspace":{"enabled":true,"files":[{"path":"src/App.tsx","language":"tsx","content":"export default function App() { return null; }"}]}}}`)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1")).
		WithArgs(appID).
		WillReturnRows(appRows(appID, pageMain, []byte(`{}`), []byte(`{}`), now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1")).
		WithArgs(appID).
		WillReturnRows(appRows(appID, pageMain, []byte(`{}`), []byte(`{}`), now))
	mock.ExpectQuery("UPDATE apps SET").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), appID).
		WillReturnRows(appRows(appID, pageMain, []byte(`{}`), importedSettings, now))
	importRR := authedRequest(t, router, jwt, subject, http.MethodPost, "/api/v1/apps/"+appID.String()+"/slate-package", []byte(`{"files":[{"path":"src/App.tsx","language":"tsx","content":"export default function App() { return null; }"}]}`))
	require.Equal(t, http.StatusOK, importRR.Code)
	var imported map[string]any
	require.NoError(t, json.NewDecoder(importRR.Body).Decode(&imported))
	require.Contains(t, imported, "slate_package")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkshopPreviewUsesDraftAndPublicRuntimeUsesPublishedSnapshot(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router, jwt := testWorkshopRouter(mock)
	subject := uuid.New()
	appID := uuid.New()
	versionID := uuid.New()
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Hour)
	draftPages := []byte(`[{"id":"main","name":"Draft Main","path":"/","layout":{"kind":"grid","columns":12},"widgets":[{"id":"draft-text","widget_type":"text","title":"Draft text","description":"","position":{"x":0,"y":0,"width":12,"height":2},"props":{"content":"Draft preview"},"binding":null,"events":[],"children":[]}],"visible":true}]`)
	publishedPages := json.RawMessage(`[{"id":"main","name":"Published Main","path":"/","layout":{"kind":"grid","columns":12},"widgets":[{"id":"published-text","widget_type":"text","title":"Published text","description":"","position":{"x":0,"y":0,"width":12,"height":2},"props":{"content":"Published runtime"},"binding":null,"events":[],"children":[]}],"visible":true}]`)
	publishedSnapshot, err := json.Marshal(map[string]any{
		"schema_version": "2026-05-11.ws.1",
		"name":           "Trail Demo",
		"slug":           "trail-demo",
		"description":    "Trail running demo",
		"status":         "published",
		"pages":          publishedPages,
		"theme":          json.RawMessage(`{}`),
		"settings":       json.RawMessage(`{"schema_version":"2026-05-11.ws.1","home_page_id":"main"}`),
		"template_key":   nil,
	})
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1")).
		WithArgs(appID).
		WillReturnRows(appRowsWithPublished(appID, draftPages, []byte(`{}`), []byte(`{"schema_version":"2026-05-11.ws.1","home_page_id":"main"}`), &versionID, now))
	previewRR := authedRequest(t, router, jwt, subject, http.MethodGet, "/api/v1/apps/"+appID.String()+"/preview", nil)
	if previewRR.Code != http.StatusOK {
		t.Logf("preview body: %q", previewRR.Body.String())
	}
	require.Equalf(t, http.StatusOK, previewRR.Code, "body: %q", previewRR.Body.String())
	var preview map[string]any
	require.NoError(t, json.NewDecoder(previewRR.Body).Decode(&preview))
	require.Equal(t, "draft", preview["preview_mode"])
	previewApp := preview["app"].(map[string]any)
	previewPages := previewApp["pages"].([]any)
	require.Equal(t, "Draft Main", previewPages[0].(map[string]any)["name"])

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE slug = $1")).
		WithArgs("trail-demo").
		WillReturnRows(appRowsWithPublished(appID, draftPages, []byte(`{}`), []byte(`{"schema_version":"2026-05-11.ws.1","home_page_id":"main"}`), &versionID, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT v.id, v.app_id, v.version_number, v.status, v.app_snapshot, v.notes,\n\t\t        v.created_by, v.created_at, v.published_at\n\t\t   FROM apps a\n\t\t   JOIN app_versions v ON v.id = a.published_version_id\n\t\t  WHERE a.id = $1")).
		WithArgs(appID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "app_id", "version_number", "status", "app_snapshot", "notes", "created_by", "created_at", "published_at"}).
			AddRow(versionID, appID, 3, "published", publishedSnapshot, "Release notes", nil, publishedAt, nil))
	publicRR := httptest.NewRecorder()
	publicReq := httptest.NewRequest(http.MethodGet, "/api/v1/apps/public/trail-demo", nil)
	router.ServeHTTP(publicRR, publicReq)
	if publicRR.Code != http.StatusOK {
		t.Logf("public body: %q", publicRR.Body.String())
	}
	require.Equalf(t, http.StatusOK, publicRR.Code, "body: %q", publicRR.Body.String())
	var public map[string]any
	require.NoError(t, json.NewDecoder(publicRR.Body).Decode(&public))
	require.Equal(t, float64(3), public["published_version_number"])
	publicApp := public["app"].(map[string]any)
	publicPages := publicApp["pages"].([]any)
	require.Equal(t, "Published Main", publicPages[0].(map[string]any)["name"])
	require.NotEqual(t, "Draft Main", publicPages[0].(map[string]any)["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkshopPromoteVersionCreatesNewPublishedSnapshot(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router, jwt := testWorkshopRouter(mock)
	subject := uuid.New()
	appID := uuid.New()
	sourceVersionID := uuid.New()
	now := time.Now().UTC()
	draftPages := []byte(`[{"id":"main","name":"Draft Main","path":"/","layout":{"kind":"grid","columns":12},"widgets":[],"visible":true}]`)
	rollbackPages := json.RawMessage(`[{"id":"main","name":"Published Rollback","path":"/","layout":{"kind":"grid","columns":12},"widgets":[],"visible":true}]`)
	rollbackSnapshot, err := json.Marshal(map[string]any{
		"schema_version": "2026-05-11.ws.1",
		"name":           "Trail Demo",
		"slug":           "trail-demo",
		"description":    "Stable published trail app",
		"status":         "published",
		"pages":          rollbackPages,
		"theme":          json.RawMessage(`{}`),
		"settings":       json.RawMessage(`{"schema_version":"2026-05-11.ws.1","home_page_id":"main"}`),
		"template_key":   nil,
	})
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE id = $1 FOR UPDATE")).
		WithArgs(appID).
		WillReturnRows(appRows(appID, draftPages, []byte(`{}`), []byte(`{"schema_version":"2026-05-11.ws.1","home_page_id":"main"}`), now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, app_id, version_number, status, app_snapshot, notes, created_by, created_at, published_at\n\t\t   FROM app_versions WHERE app_id = $1 AND id = $2")).
		WithArgs(appID, sourceVersionID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "app_id", "version_number", "status", "app_snapshot", "notes", "created_by", "created_at", "published_at"}).
			AddRow(sourceVersionID, appID, 2, "published", rollbackSnapshot, "Stable release", nil, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(version_number), 0) + 1 FROM app_versions WHERE app_id = $1")).
		WithArgs(appID).
		WillReturnRows(pgxmock.NewRows([]string{"next"}).AddRow(4))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO app_versions (id, app_id, version_number, status, app_snapshot, notes, created_by, created_at, published_at)\n         VALUES ($1, $2, $3, 'published', $4, $5, $6, $7, $7)")).
		WithArgs(pgxmock.AnyArg(), appID, 4, pgxmock.AnyArg(), "Rollback to v2 for release train", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE apps\n\t\t    SET name = $1, slug = $2, description = $3, status = 'published',\n\t\t        pages = $4, theme = $5, settings = $6, template_key = $7,\n\t\t        published_version_id = $8, updated_at = $9\n\t\t  WHERE id = $10")).
		WithArgs("Trail Demo", "trail-demo", "Stable published trail app", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), appID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	rr := authedRequest(t, router, jwt, subject, http.MethodPost,
		"/api/v1/apps/"+appID.String()+"/versions/"+sourceVersionID.String()+"/promote",
		[]byte(`{"changelog":"Rollback to v2 for release train"}`))
	if rr.Code != http.StatusCreated {
		t.Logf("promote body: %q", rr.Body.String())
	}
	require.Equalf(t, http.StatusCreated, rr.Code, "body: %q", rr.Body.String())
	var promoted map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&promoted))
	require.Equal(t, float64(4), promoted["version_number"])
	require.Equal(t, "published", promoted["status"])
	require.Equal(t, "Rollback to v2 for release train", promoted["notes"])
	snapshot := promoted["app_snapshot"].(map[string]any)
	pages := snapshot["pages"].([]any)
	require.Equal(t, "Published Rollback", pages[0].(map[string]any)["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkshopAppPermissionsDenyEditAndPublishAndAudit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router, jwt := testWorkshopRouter(mock)
	subject := uuid.New()
	appID := uuid.New()

	mock.ExpectExec("INSERT INTO app_audit_events").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), "", pgxmock.AnyArg(), pgxmock.AnyArg(), "app.update", "denied", "apps:edit", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	updateRR := authedRequestWithClaims(t, router, jwt, &authmw.Claims{
		Sub:   subject,
		Email: "viewer@example.com",
		Name:  "Viewer",
		Roles: []string{"viewer"},
	}, http.MethodPatch, "/api/v1/apps/"+appID.String(), []byte(`{"name":"Nope"}`))
	require.Equal(t, http.StatusForbidden, updateRR.Code)
	require.Contains(t, updateRR.Body.String(), "apps:edit")

	mock.ExpectExec("INSERT INTO app_audit_events").
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), "", pgxmock.AnyArg(), pgxmock.AnyArg(), "app.publish", "denied", "apps:publish", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	publishRR := authedRequestWithClaims(t, router, jwt, &authmw.Claims{
		Sub:   subject,
		Email: "viewer@example.com",
		Name:  "Viewer",
		Roles: []string{"viewer"},
	}, http.MethodPost, "/api/v1/apps/"+appID.String()+"/publish", []byte(`{"notes":"nope"}`))
	require.Equal(t, http.StatusForbidden, publishRR.Code)
	require.Contains(t, publishRR.Body.String(), "apps:publish")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkshopPublicRuntimeRejectsDraftPublishedSnapshot(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router, _ := testWorkshopRouter(mock)
	appID := uuid.New()
	versionID := uuid.New()
	now := time.Now().UTC()
	draftPages := []byte(`[{"id":"main","name":"Mutable Draft","path":"/","layout":{"kind":"grid","columns":12},"widgets":[],"visible":true}]`)
	draftSnapshot, err := json.Marshal(map[string]any{
		"schema_version": "2026-05-11.ws.1",
		"name":           "Trail Demo",
		"slug":           "trail-demo",
		"description":    "Draft should not be public",
		"status":         "draft",
		"pages":          json.RawMessage(draftPages),
		"theme":          json.RawMessage(`{}`),
		"settings":       json.RawMessage(`{"schema_version":"2026-05-11.ws.1","home_page_id":"main"}`),
	})
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, slug, description, status, pages, theme, settings,\n\ttemplate_key, created_by, published_version_id, created_at, updated_at FROM apps WHERE slug = $1")).
		WithArgs("trail-demo").
		WillReturnRows(appRowsWithPublished(appID, draftPages, []byte(`{}`), []byte(`{"schema_version":"2026-05-11.ws.1","home_page_id":"main"}`), &versionID, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT v.id, v.app_id, v.version_number, v.status, v.app_snapshot, v.notes,\n\t\t        v.created_by, v.created_at, v.published_at\n\t\t   FROM apps a\n\t\t   JOIN app_versions v ON v.id = a.published_version_id\n\t\t  WHERE a.id = $1")).
		WithArgs(appID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "app_id", "version_number", "status", "app_snapshot", "notes", "created_by", "created_at", "published_at"}).
			AddRow(versionID, appID, 4, "draft", draftSnapshot, "Draft save", nil, now, nil))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/public/trail-demo", nil)
	router.ServeHTTP(rr, req)
	require.Equalf(t, http.StatusNotFound, rr.Code, "body: %q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "no public published version")
	require.NoError(t, mock.ExpectationsWereMet())
}

func testWorkshopRouter(mock pgxmock.PgxPoolIface) (http.Handler, *authmw.JWTConfig) {
	cfg := &config.Config{}
	cfg.Service.Name = "application-composition-service"
	cfg.Service.Version = "test"
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0
	jwt := authmw.NewJWTConfig("workshop-routes-test-secret")
	h := &handlers.Handlers{Repo: &repo.Repo{Pool: mock}}
	return BuildRouter(cfg, jwt, h, nil), jwt
}

func testWorkshopToken(t *testing.T, jwt *authmw.JWTConfig, subject uuid.UUID) string {
	t.Helper()
	now := time.Now()
	token, err := authmw.EncodeToken(jwt, &authmw.Claims{
		Sub:   subject,
		IAT:   now.Unix(),
		EXP:   now.Add(time.Hour).Unix(),
		JTI:   uuid.New(),
		Email: "builder@example.com",
		Name:  "Builder",
		Roles: []string{"builder"},
	})
	require.NoError(t, err)
	return token
}

func testWorkshopTokenWithClaims(t *testing.T, jwt *authmw.JWTConfig, claims *authmw.Claims) string {
	t.Helper()
	now := time.Now()
	copyClaims := *claims
	if copyClaims.Sub == uuid.Nil {
		copyClaims.Sub = uuid.New()
	}
	if copyClaims.IAT == 0 {
		copyClaims.IAT = now.Unix()
	}
	if copyClaims.EXP == 0 {
		copyClaims.EXP = now.Add(time.Hour).Unix()
	}
	if copyClaims.JTI == uuid.Nil {
		copyClaims.JTI = uuid.New()
	}
	token, err := authmw.EncodeToken(jwt, &copyClaims)
	require.NoError(t, err)
	return token
}

func authedRequest(t *testing.T, router http.Handler, jwt *authmw.JWTConfig, subject uuid.UUID, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	return authedRequestWithClaims(t, router, jwt, &authmw.Claims{
		Sub:   subject,
		Email: "builder@example.com",
		Name:  "Builder",
		Roles: []string{"builder"},
	}, method, path, body)
}

func authedRequestWithClaims(t *testing.T, router http.Handler, jwt *authmw.JWTConfig, claims *authmw.Claims, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testWorkshopTokenWithClaims(t, jwt, claims))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(rr, req)
	return rr
}

func appRows(appID uuid.UUID, pages, theme, settings []byte, now time.Time) *pgxmock.Rows {
	return appRowsWithPublished(appID, pages, theme, settings, nil, now)
}

func appRowsWithPublished(appID uuid.UUID, pages, theme, settings []byte, publishedVersionID *uuid.UUID, now time.Time) *pgxmock.Rows {
	var publishedArg any
	if publishedVersionID != nil {
		publishedArg = pgtype.UUID{Bytes: *publishedVersionID, Valid: true}
	}
	return pgxmock.NewRows([]string{
		"id", "name", "slug", "description", "status", "pages", "theme", "settings",
		"template_key", "created_by", "published_version_id", "created_at", "updated_at",
	}).AddRow(appID, "Trail Demo", "trail-demo", "Trail running demo", "draft", pages, theme, settings, nil, nil, publishedArg, now, now)
}
