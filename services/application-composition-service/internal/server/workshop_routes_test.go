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
	require.Equal(t, "2026-05-11.ws.3", catalogRR.Header().Get("X-OpenFoundry-Widget-Catalog-Version"))
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
	require.Equal(t, http.StatusOK, previewRR.Code)
	var preview map[string]any
	require.NoError(t, json.NewDecoder(previewRR.Body).Decode(&preview))
	require.Contains(t, preview, "app")
	require.Contains(t, preview, "embed")

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

func authedRequest(t *testing.T, router http.Handler, jwt *authmw.JWTConfig, subject uuid.UUID, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testWorkshopToken(t, jwt, subject))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(rr, req)
	return rr
}

func appRows(appID uuid.UUID, pages, theme, settings []byte, now time.Time) *pgxmock.Rows {
	return pgxmock.NewRows([]string{
		"id", "name", "slug", "description", "status", "pages", "theme", "settings",
		"template_key", "created_by", "published_version_id", "created_at", "updated_at",
	}).AddRow(appID, "Trail Demo", "trail-demo", "Trail running demo", "draft", pages, theme, settings, nil, nil, nil, now, now)
}
