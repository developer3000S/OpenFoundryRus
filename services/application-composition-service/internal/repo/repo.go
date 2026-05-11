package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/models"
)

type Repo struct {
	Pool DB
}

type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ DB = (*pgxpool.Pool)(nil)

// appColumns is the canonical SELECT projection for the `apps` table.
const appColumns = `id, name, slug, description, status, pages, theme, settings,
	template_key, created_by, published_version_id, created_at, updated_at`

func (r *Repo) ListPrimary(ctx context.Context) ([]models.PrimaryItem, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, payload, created_at FROM composition_views ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.PrimaryItem, 0)
	for rows.Next() {
		var p models.PrimaryItem
		if err := rows.Scan(&p.ID, &p.Payload, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) CreatePrimary(ctx context.Context, payload []byte) (models.PrimaryItem, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO composition_views (id, payload) VALUES ($1, $2) RETURNING id, payload, created_at`,
		uuid.New(), payload)
	var p models.PrimaryItem
	err := row.Scan(&p.ID, &p.Payload, &p.CreatedAt)
	return p, err
}

func (r *Repo) GetPrimary(ctx context.Context, id uuid.UUID) (*models.PrimaryItem, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, payload, created_at FROM composition_views WHERE id = $1`, id)
	var p models.PrimaryItem
	err := row.Scan(&p.ID, &p.Payload, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repo) ListSecondary(ctx context.Context, parentID uuid.UUID) ([]models.SecondaryItem, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, parent_id, payload, created_at
           FROM composition_bindings
          WHERE parent_id = $1
          ORDER BY created_at DESC LIMIT 200`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SecondaryItem, 0)
	for rows.Next() {
		var s models.SecondaryItem
		if err := rows.Scan(&s.ID, &s.ParentID, &s.Payload, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) CreateSecondary(ctx context.Context, parentID uuid.UUID, payload []byte) (models.SecondaryItem, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO composition_bindings (id, parent_id, payload)
           VALUES ($1, $2, $3) RETURNING id, parent_id, payload, created_at`,
		uuid.New(), parentID, payload)
	var s models.SecondaryItem
	err := row.Scan(&s.ID, &s.ParentID, &s.Payload, &s.CreatedAt)
	return s, err
}

// scanApp consumes a row matching `appColumns` (in order) and produces models.App.
func scanApp(row pgx.Row) (*models.App, error) {
	var a models.App
	err := row.Scan(
		&a.ID, &a.Name, &a.Slug, &a.Description, &a.Status,
		&a.Pages, &a.Theme, &a.Settings,
		&a.TemplateKey, &a.CreatedBy, &a.PublishedVersionID,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAppsFilter is the wire-level filter coming from the SPA's listApps().
type ListAppsFilter struct {
	Search  string
	Status  string
	Page    int
	PerPage int
}

// ListApps returns AppSummary rows + total count. Pagination is best-effort:
// the SPA only uses page/per_page as hints (default 25 per page).
func (r *Repo) ListApps(ctx context.Context, f ListAppsFilter) ([]models.AppSummary, int, error) {
	conds := []string{}
	args := []any{}
	if strings.TrimSpace(f.Search) != "" {
		args = append(args, "%"+strings.ToLower(f.Search)+"%")
		conds = append(conds, fmt.Sprintf("(LOWER(name) LIKE $%d OR LOWER(slug) LIKE $%d OR LOWER(description) LIKE $%d)", len(args), len(args), len(args)))
	}
	if strings.TrimSpace(f.Status) != "" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM apps"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	perPage := f.PerPage
	if perPage <= 0 || perPage > 200 {
		perPage = 25
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage

	args = append(args, perPage, offset)
	q := fmt.Sprintf(`
        SELECT id, name, slug, description, status,
               COALESCE(jsonb_array_length(pages), 0) AS page_count,
               COALESCE((
                   SELECT SUM(COALESCE(jsonb_array_length(p->'widgets'), 0))
                   FROM jsonb_array_elements(pages) AS p
               ), 0) AS widget_count,
               template_key, published_version_id, created_at, updated_at
          FROM apps%s
         ORDER BY updated_at DESC
         LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args))

	rows, err := r.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]models.AppSummary, 0)
	for rows.Next() {
		var s models.AppSummary
		if err := rows.Scan(
			&s.ID, &s.Name, &s.Slug, &s.Description, &s.Status,
			&s.PageCount, &s.WidgetCount,
			&s.TemplateKey, &s.PublishedVersionID, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

func (r *Repo) ListAppTemplates(ctx context.Context) ([]models.AppTemplate, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, key, name, description, category, preview_image_url, definition, created_at
		   FROM app_templates ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AppTemplate, 0)
	for rows.Next() {
		var t models.AppTemplate
		if err := rows.Scan(&t.ID, &t.Key, &t.Name, &t.Description, &t.Category, &t.PreviewImageURL, &t.Definition, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repo) GetAppTemplateByKey(ctx context.Context, key string) (*models.AppTemplate, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, key, name, description, category, preview_image_url, definition, created_at
		   FROM app_templates WHERE key = $1`, key)
	var t models.AppTemplate
	err := row.Scan(&t.ID, &t.Key, &t.Name, &t.Description, &t.Category, &t.PreviewImageURL, &t.Definition, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repo) CreateAppFromTemplate(ctx context.Context, body *models.CreateAppRequest, creator *uuid.UUID) (*models.App, error) {
	if body.TemplateKey == nil || strings.TrimSpace(*body.TemplateKey) == "" {
		return nil, errors.New("template_key is required")
	}
	template, err := r.GetAppTemplateByKey(ctx, strings.TrimSpace(*body.TemplateKey))
	if err != nil {
		return nil, err
	}
	if template == nil {
		return nil, pgx.ErrNoRows
	}
	def := map[string]json.RawMessage{}
	if err := json.Unmarshal(template.Definition, &def); err != nil {
		return nil, fmt.Errorf("template definition is invalid: %w", err)
	}
	req := *body
	if strings.TrimSpace(req.Name) == "" {
		req.Name = template.Name
	}
	if strings.TrimSpace(req.Description) == "" {
		req.Description = template.Description
	}
	if len(req.Pages) == 0 {
		req.Pages = def["pages"]
	}
	if len(req.Theme) == 0 {
		req.Theme = def["theme"]
	}
	if len(req.Settings) == 0 {
		req.Settings = def["settings"]
	}
	return r.CreateApp(ctx, &req, creator)
}

func (r *Repo) GetApp(ctx context.Context, id uuid.UUID) (*models.App, error) {
	row := r.Pool.QueryRow(ctx, `SELECT `+appColumns+` FROM apps WHERE id = $1`, id)
	a, err := scanApp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *Repo) GetAppBySlug(ctx context.Context, slug string) (*models.App, error) {
	row := r.Pool.QueryRow(ctx, `SELECT `+appColumns+` FROM apps WHERE slug = $1`, slug)
	a, err := scanApp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func slugify(name string) string {
	out := strings.Builder{}
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			out.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash && out.Len() > 0 {
				out.WriteByte('-')
				prevDash = true
			}
		}
	}
	s := strings.TrimRight(out.String(), "-")
	if s == "" {
		s = "app-" + uuid.NewString()[:8]
	}
	return s
}

func (r *Repo) CreateApp(ctx context.Context, body *models.CreateAppRequest, creator *uuid.UUID) (*models.App, error) {
	id := uuid.New()
	slug := strings.TrimSpace(body.Slug)
	if slug == "" {
		slug = slugify(body.Name)
	}
	status := body.Status
	if status == "" {
		status = "draft"
	}
	pages := json.RawMessage(body.Pages)
	if len(pages) == 0 {
		pages = json.RawMessage(`[]`)
	}
	theme := json.RawMessage(body.Theme)
	if len(theme) == 0 {
		theme = json.RawMessage(`{}`)
	}
	settings := json.RawMessage(body.Settings)
	if len(settings) == 0 {
		settings = json.RawMessage(`{}`)
	}
	contract, err := models.NormalizeAppContract(body.Name, slug, status, pages, theme, settings)
	if err != nil {
		return nil, err
	}
	pages = contract.Pages
	theme = contract.Theme
	settings = contract.Settings
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO apps (id, name, slug, description, status, pages, theme, settings, template_key, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING `+appColumns,
		id, body.Name, slug, body.Description, status, []byte(pages), []byte(theme), []byte(settings),
		body.TemplateKey, creator,
	)
	return scanApp(row)
}

func (r *Repo) UpdateApp(ctx context.Context, id uuid.UUID, body *models.UpdateAppRequest) (*models.App, error) {
	current, err := r.GetApp(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}

	nextName := current.Name
	nextSlug := current.Slug
	nextStatus := current.Status
	nextPages := json.RawMessage(current.Pages)
	nextTheme := json.RawMessage(current.Theme)
	nextSettings := json.RawMessage(current.Settings)
	if body.Name != nil {
		nextName = *body.Name
	}
	if body.Slug != nil {
		nextSlug = strings.TrimSpace(*body.Slug)
	}
	if body.Status != nil {
		nextStatus = strings.TrimSpace(*body.Status)
	}
	if len(body.Pages) > 0 {
		nextPages = json.RawMessage(body.Pages)
	}
	if len(body.Theme) > 0 {
		nextTheme = json.RawMessage(body.Theme)
	}
	if len(body.Settings) > 0 {
		nextSettings = json.RawMessage(body.Settings)
	}
	contract, err := models.NormalizeAppContract(nextName, nextSlug, nextStatus, nextPages, nextTheme, nextSettings)
	if err != nil {
		return nil, err
	}

	sets := []string{}
	args := []any{}
	add := func(col string, v any) {
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if body.Name != nil {
		add("name", nextName)
	}
	if body.Slug != nil {
		add("slug", nextSlug)
	}
	if body.Description != nil {
		add("description", *body.Description)
	}
	if body.Status != nil {
		add("status", nextStatus)
	}
	add("pages", []byte(contract.Pages))
	add("theme", []byte(contract.Theme))
	add("settings", []byte(contract.Settings))
	if body.TemplateKey != nil {
		add("template_key", *body.TemplateKey)
	}
	if len(sets) == 0 {
		return r.GetApp(ctx, id)
	}
	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)
	q := fmt.Sprintf(`UPDATE apps SET %s WHERE id = $%d RETURNING %s`,
		strings.Join(sets, ", "), len(args), appColumns)
	row := r.Pool.QueryRow(ctx, q, args...)
	a, err := scanApp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *Repo) AddPage(ctx context.Context, appID uuid.UUID, page models.AppPage) (*models.App, error) {
	current, err := r.GetApp(ctx, appID)
	if err != nil || current == nil {
		return current, err
	}
	pages, err := decodeAppPages(current.Pages)
	if err != nil {
		return nil, err
	}
	pages = append(pages, page)
	pagesRaw, err := json.Marshal(pages)
	if err != nil {
		return nil, err
	}
	return r.UpdateApp(ctx, appID, &models.UpdateAppRequest{Pages: pagesRaw})
}

func (r *Repo) UpdatePage(ctx context.Context, appID uuid.UUID, pageID string, page models.AppPage) (*models.App, error) {
	current, err := r.GetApp(ctx, appID)
	if err != nil || current == nil {
		return current, err
	}
	pages, err := decodeAppPages(current.Pages)
	if err != nil {
		return nil, err
	}
	found := false
	page.ID = pageID
	for i := range pages {
		if pages[i].ID == pageID {
			pages[i] = page
			found = true
			break
		}
	}
	if !found {
		return nil, nil
	}
	pagesRaw, err := json.Marshal(pages)
	if err != nil {
		return nil, err
	}
	return r.UpdateApp(ctx, appID, &models.UpdateAppRequest{Pages: pagesRaw})
}

func (r *Repo) DeletePage(ctx context.Context, appID uuid.UUID, pageID string) (*models.App, error) {
	current, err := r.GetApp(ctx, appID)
	if err != nil || current == nil {
		return current, err
	}
	pages, err := decodeAppPages(current.Pages)
	if err != nil {
		return nil, err
	}
	next := make([]models.AppPage, 0, len(pages))
	found := false
	for _, page := range pages {
		if page.ID == pageID {
			found = true
			continue
		}
		next = append(next, page)
	}
	if !found {
		return nil, nil
	}
	pagesRaw, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	return r.UpdateApp(ctx, appID, &models.UpdateAppRequest{Pages: pagesRaw})
}

func (r *Repo) DeleteApp(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := r.Pool.Exec(ctx, `DELETE FROM apps WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) BuildSlatePackage(ctx context.Context, appID uuid.UUID) (*models.SlatePackageResponse, error) {
	app, err := r.GetApp(ctx, appID)
	if err != nil || app == nil {
		return nil, err
	}
	pkg, err := slatePackageFromApp(app)
	if err != nil {
		return nil, err
	}
	return &pkg, nil
}

func (r *Repo) ImportSlatePackage(ctx context.Context, appID uuid.UUID, body models.ImportSlatePackageRequest) (*models.SlateRoundTripResponse, error) {
	if len(body.Files) == 0 {
		return nil, errors.New("files is required")
	}
	for i, file := range body.Files {
		if strings.TrimSpace(file.Path) == "" || strings.TrimSpace(file.Language) == "" {
			return nil, fmt.Errorf("files[%d] requires path and language", i)
		}
	}
	current, err := r.GetApp(ctx, appID)
	if err != nil || current == nil {
		return nil, err
	}
	settings, err := jsonObject(current.Settings)
	if err != nil {
		return nil, err
	}
	slate := mapStringAny(settings["slate"])
	workspace := mapStringAny(slate["workspace"])
	slate["enabled"] = true
	setIfStringPtr(slate, "framework", body.Framework, "react")
	setIfStringPtr(slate, "package_name", body.PackageName, "@open-foundry/workshop-app")
	setIfStringPtr(slate, "entry_file", body.EntryFile, "src/App.tsx")
	setIfStringPtr(slate, "sdk_import", body.SDKImport, "@open-foundry/sdk/react")
	setIfStringPtr(workspace, "repository_id", body.RepositoryID, "")
	setIfStringPtr(workspace, "layout", body.Layout, "single-page")
	setIfStringPtr(workspace, "runtime", body.Runtime, "vite")
	setIfStringPtr(workspace, "dev_command", body.DevCommand, "npm run dev")
	setIfStringPtr(workspace, "preview_command", body.PreviewCommand, "npm run preview")
	workspace["enabled"] = true
	workspace["files"] = body.Files
	slate["workspace"] = workspace
	settings["slate"] = slate
	settingsRaw, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	updated, err := r.UpdateApp(ctx, appID, &models.UpdateAppRequest{Settings: settingsRaw})
	if err != nil || updated == nil {
		return nil, err
	}
	pkg, err := slatePackageFromApp(updated)
	if err != nil {
		return nil, err
	}
	return &models.SlateRoundTripResponse{App: updated, SlatePackage: pkg}, nil
}

// PublishApp creates a new app_versions row that snapshots the current app state,
// marks it as published, and points apps.published_version_id at it. Returns
// the new version row.
func (r *Repo) PublishApp(ctx context.Context, appID uuid.UUID, notes string, publisher *uuid.UUID) (*models.AppVersion, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	a, err := scanApp(tx.QueryRow(ctx, `SELECT `+appColumns+` FROM apps WHERE id = $1 FOR UPDATE`, appID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	contract, err := models.NormalizeAppContract(a.Name, a.Slug, a.Status, a.Pages, a.Theme, a.Settings)
	if err != nil {
		return nil, err
	}
	a.Pages = contract.Pages
	a.Theme = contract.Theme
	a.Settings = contract.Settings

	snapshot := map[string]any{
		"name":           a.Name,
		"slug":           a.Slug,
		"description":    a.Description,
		"status":         a.Status,
		"pages":          a.Pages,
		"theme":          a.Theme,
		"settings":       a.Settings,
		"template_key":   a.TemplateKey,
		"schema_version": models.WorkshopAppSchemaVersion,
	}
	snapBytes, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}

	var nextVersion int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(version_number), 0) + 1 FROM app_versions WHERE app_id = $1`, appID,
	).Scan(&nextVersion); err != nil {
		return nil, err
	}

	versionID := uuid.New()
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx,
		`INSERT INTO app_versions (id, app_id, version_number, status, app_snapshot, notes, created_by, created_at, published_at)
         VALUES ($1, $2, $3, 'published', $4, $5, $6, $7, $7)`,
		versionID, appID, nextVersion, snapBytes, notes, publisher, now,
	); err != nil {
		return nil, err
	}

	publishedContract, err := models.NormalizeAppContract(a.Name, a.Slug, "published", a.Pages, a.Theme, a.Settings)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE apps SET published_version_id = $1, status = 'published', pages = $2, theme = $3, settings = $4, updated_at = $5 WHERE id = $6`,
		versionID, []byte(publishedContract.Pages), []byte(publishedContract.Theme), []byte(publishedContract.Settings), now, appID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &models.AppVersion{
		ID:            versionID,
		AppID:         appID,
		VersionNumber: nextVersion,
		Status:        "published",
		AppSnapshot:   snapBytes,
		Notes:         notes,
		CreatedBy:     publisher,
		CreatedAt:     now,
		PublishedAt:   &now,
	}, nil
}

func (r *Repo) ListAppVersions(ctx context.Context, appID uuid.UUID) ([]models.AppVersion, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, app_id, version_number, status, app_snapshot, notes, created_by, created_at, published_at
		   FROM app_versions WHERE app_id = $1 ORDER BY version_number DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AppVersion, 0)
	for rows.Next() {
		var v models.AppVersion
		if err := rows.Scan(
			&v.ID, &v.AppID, &v.VersionNumber, &v.Status, &v.AppSnapshot,
			&v.Notes, &v.CreatedBy, &v.CreatedAt, &v.PublishedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetPublishedVersion returns the version row pointed to by apps.published_version_id.
func (r *Repo) GetPublishedVersion(ctx context.Context, appID uuid.UUID) (*models.AppVersion, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT v.id, v.app_id, v.version_number, v.status, v.app_snapshot, v.notes,
		        v.created_by, v.created_at, v.published_at
		   FROM apps a
		   JOIN app_versions v ON v.id = a.published_version_id
		  WHERE a.id = $1`, appID)
	var v models.AppVersion
	err := row.Scan(
		&v.ID, &v.AppID, &v.VersionNumber, &v.Status, &v.AppSnapshot,
		&v.Notes, &v.CreatedBy, &v.CreatedAt, &v.PublishedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func decodeAppPages(raw json.RawMessage) ([]models.AppPage, error) {
	var pages []models.AppPage
	if len(raw) == 0 {
		return pages, nil
	}
	if err := json.Unmarshal(raw, &pages); err != nil {
		return nil, fmt.Errorf("pages are invalid: %w", err)
	}
	return pages, nil
}

func slatePackageFromApp(app *models.App) (models.SlatePackageResponse, error) {
	settings, err := jsonObject(app.Settings)
	if err != nil {
		return models.SlatePackageResponse{}, err
	}
	slate := mapStringAny(settings["slate"])
	workspace := mapStringAny(slate["workspace"])
	files := []models.SlatePackageFile{}
	if rawFiles, ok := workspace["files"]; ok {
		bytes, err := json.Marshal(rawFiles)
		if err == nil {
			_ = json.Unmarshal(bytes, &files)
		}
	}
	if len(files) == 0 {
		files = []models.SlatePackageFile{{
			Path:     "src/App.tsx",
			Language: "tsx",
			Content:  fmt.Sprintf("import { AppRuntime } from '@open-foundry/sdk/react';\n\nexport default function App() {\n  return <AppRuntime slug=%q />;\n}\n", app.Slug),
		}}
	}
	return models.SlatePackageResponse{
		AppID:       app.ID,
		AppSlug:     app.Slug,
		Framework:   stringFromMap(slate, "framework", "react"),
		PackageName: stringFromMap(slate, "package_name", "@open-foundry/workshop-app"),
		EntryFile:   stringFromMap(slate, "entry_file", "src/App.tsx"),
		SDKImport:   stringFromMap(slate, "sdk_import", "@open-foundry/sdk/react"),
		Files:       files,
	}, nil
}

func jsonObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		obj = map[string]any{}
	}
	return obj, nil
}

func mapStringAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok && typed != nil {
		return typed
	}
	return map[string]any{}
}

func stringFromMap(value map[string]any, key, fallback string) string {
	if str, ok := value[key].(string); ok && strings.TrimSpace(str) != "" {
		return str
	}
	return fallback
}

func setIfStringPtr(target map[string]any, key string, value *string, fallback string) {
	if value != nil {
		target[key] = *value
		return
	}
	if _, ok := target[key]; !ok && fallback != "" {
		target[key] = fallback
	}
}
