package handlers

// projects.go ports services/tenancy-organizations-service/src/handlers/projects.rs.
//
// The Rust handler exposes the ontology-project CRUD surface plus folder
// management, membership upserts and resource-binding lifecycle. Slug
// normalisation, folder-name canonicalisation and error strings are
// byte-exact with the Rust source so federated callers see identical
// payloads regardless of which language emitted the response.
//
// The handler holds a *pgxpool.Pool directly (mirroring the Rust
// `state.ontology_db` field) instead of going through the foundation
// repo.Repo — this matches Rust's inline-SQL style and keeps the SQL
// adjacency to the handler logic.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/domain"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/models"
)

// ProjectsHandlers owns the ontology-project HTTP surface. The Pool is
// the same shared pgxpool used by the rest of the service — the Rust
// crate carries a separate `ontology_db` handle, but the Go port runs
// against a single DATABASE_URL so we reuse the foundation pool.
type ProjectsHandlers struct {
	Pool *pgxpool.Pool
}

// ─── slug + folder-name normalisation ───────────────────────────────────

// asciiLower returns s with every ASCII A–Z byte lowered to a–z.
// Non-ASCII bytes pass through unchanged so we mirror Rust's
// str::to_ascii_lowercase byte-exactly.
func asciiLower(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

// normalizeSlug enforces the Rust slug invariants verbatim:
// trim → ASCII lower-case → non-empty → only [a-z0-9-] →
// no leading or trailing hyphen.
func normalizeSlug(value, fieldName string) (string, error) {
	normalized := asciiLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	for _, ch := range normalized {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-'
		if !ok {
			return "", fmt.Errorf("%s must contain only lowercase letters, digits, and hyphens", fieldName)
		}
	}
	if strings.HasPrefix(normalized, "-") || strings.HasSuffix(normalized, "-") {
		return "", fmt.Errorf("%s cannot start or end with a hyphen", fieldName)
	}
	return normalized, nil
}

// normalizeOptionalSlug mirrors normalize_optional_slug: a nil or
// whitespace-only input collapses to nil; otherwise the value is run
// through normalizeSlug.
func normalizeOptionalSlug(value *string, fieldName string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	out, err := normalizeSlug(trimmed, fieldName)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// normalizeFolderName collapses Unicode whitespace runs to a single
// space and rejects whitespace-only input. Mirrors Rust's
// `value.split_whitespace().collect::<Vec<_>>().join(" ")`.
func normalizeFolderName(value string) (string, error) {
	parts := strings.Fields(value)
	normalized := strings.Join(parts, " ")
	if normalized == "" {
		return "", errors.New("Folder name is required.")
	}
	return normalized, nil
}

// folderSlugFromName ports the Rust slug derivation: walk runes, lower
// ASCII letters, keep ASCII alphanumerics, collapse every other run of
// characters into a single hyphen, strip trailing hyphens, error on an
// empty result.
func folderSlugFromName(value string) (string, error) {
	var slug strings.Builder
	slug.Grow(len(value))
	lastWasHyphen := false
	for _, ch := range strings.TrimSpace(value) {
		normalized := ch
		if 'A' <= ch && ch <= 'Z' {
			normalized = ch + ('a' - 'A')
		}
		if isASCIIAlphaNum(normalized) {
			slug.WriteRune(normalized)
			lastWasHyphen = false
		} else if !lastWasHyphen && slug.Len() > 0 {
			slug.WriteByte('-')
			lastWasHyphen = true
		}
	}
	out := strings.TrimRight(slug.String(), "-")
	if out == "" {
		return "", errors.New("folder name must contain letters or numbers")
	}
	return out, nil
}

func isASCIIAlphaNum(r rune) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')
}

// ─── helpers ────────────────────────────────────────────────────────────

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name, label string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid "+label)
		return uuid.Nil, false
	}
	return id, true
}

func authClaims(w http.ResponseWriter, r *http.Request) (*authmw.Claims, bool) {
	user, ok := authmw.AuthUserFromRequest(r)
	if !ok || user.Claims == nil {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	return user.Claims, true
}

func loadProject(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*models.OntologyProject, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, slug, display_name, description, workspace_slug, owner_id, created_at, updated_at
		 FROM ontology_projects
		 WHERE id = $1`,
		id,
	)
	p := &models.OntologyProject{}
	err := row.Scan(&p.ID, &p.Slug, &p.DisplayName, &p.Description,
		&p.WorkspaceSlug, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load ontology project: %s", err)
	}
	return p, nil
}

func loadProjectFolder(ctx context.Context, pool *pgxpool.Pool, projectID, folderID uuid.UUID) (*models.OntologyProjectFolder, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, project_id, parent_folder_id, name, slug, description, created_by, created_at, updated_at
		 FROM ontology_project_folders
		 WHERE project_id = $1 AND id = $2`,
		projectID, folderID,
	)
	f := &models.OntologyProjectFolder{}
	err := row.Scan(&f.ID, &f.ProjectID, &f.ParentFolderID, &f.Name, &f.Slug,
		&f.Description, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load ontology project folder: %s", err)
	}
	return f, nil
}

func insertProjectFolder(
	ctx context.Context,
	tx pgx.Tx,
	projectID, createdBy uuid.UUID,
	folder *models.CreateOntologyProjectFolderRequest,
) (*models.OntologyProjectFolder, error) {
	name, err := normalizeFolderName(folder.Name)
	if err != nil {
		return nil, err
	}
	slug, err := folderSlugFromName(name)
	if err != nil {
		return nil, err
	}
	description := ""
	if folder.Description != nil {
		description = *folder.Description
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO ontology_project_folders (
		     id, project_id, parent_folder_id, name, slug, description, created_by
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, project_id, parent_folder_id, name, slug, description, created_by, created_at, updated_at`,
		ids.New(), projectID, folder.ParentFolderID, name, slug, description, createdBy,
	)
	out := &models.OntologyProjectFolder{}
	if err := row.Scan(&out.ID, &out.ProjectID, &out.ParentFolderID, &out.Name,
		&out.Slug, &out.Description, &out.CreatedBy, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return nil, fmt.Errorf("failed to create ontology project folder: %s", err)
	}
	return out, nil
}

func ensureProjectOwnerOrAdmin(project *models.OntologyProject, claims *authmw.Claims) error {
	if claims.HasRole("admin") || project.OwnerID == claims.Sub {
		return nil
	}
	return errors.New("forbidden: only the ontology project owner can manage memberships or delete the project")
}

// ─── list / create / get / update / delete ─────────────────────────────

// ListProjects mirrors Rust `list_projects`: filter by search, paginate
// by (page, per_page), then trim to the visible set unless the caller
// is an admin.
func (h *ProjectsHandlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	accessible, err := domain.ListAccessibleProjects(r.Context(), h.Pool, claims)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to evaluate project access: %s", err))
		return
	}

	page := int64(1)
	perPage := int64(20)
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			perPage = n
		}
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 1
	}
	if perPage > 100 {
		perPage = 100
	}
	search := r.URL.Query().Get("search")
	pattern := "%" + search + "%"

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, slug, display_name, description, workspace_slug, owner_id, created_at, updated_at
		 FROM ontology_projects
		 WHERE slug ILIKE $1 OR display_name ILIKE $1
		 ORDER BY created_at DESC`,
		pattern,
	)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology projects: %s", err))
		return
	}
	defer rows.Close()

	projects := make([]models.OntologyProject, 0)
	for rows.Next() {
		var p models.OntologyProject
		if err := rows.Scan(&p.ID, &p.Slug, &p.DisplayName, &p.Description,
			&p.WorkspaceSlug, &p.OwnerID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology projects: %s", err))
			return
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology projects: %s", err))
		return
	}

	visible := projects
	if !claims.HasRole("admin") {
		visible = visible[:0]
		for _, p := range projects {
			if _, allowed := accessible[p.ID]; allowed {
				visible = append(visible, p)
			}
		}
	}

	total := int64(len(visible))
	offset := (page - 1) * perPage
	if offset > int64(len(visible)) {
		offset = int64(len(visible))
	}
	end := offset + perPage
	if end > int64(len(visible)) {
		end = int64(len(visible))
	}
	data := append([]models.OntologyProject{}, visible[offset:end]...)

	writeJSON(w, http.StatusOK, models.ListOntologyProjectsResponse{
		Data:    data,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	})
}

// CreateProject mirrors Rust `create_project`. Folder creation is part
// of the same transaction so an inserted project always sees its
// declared folders or nothing at all.
func (h *ProjectsHandlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	var body models.CreateOntologyProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	slug, err := normalizeSlug(body.Slug, "slug")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	workspaceSlug, err := normalizeOptionalSlug(body.WorkspaceSlug, "workspace_slug")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	displayName := slug
	if body.DisplayName != nil {
		displayName = *body.DisplayName
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to start ontology project transaction: %s", err))
		return
	}
	defer tx.Rollback(context.Background())

	row := tx.QueryRow(r.Context(),
		`INSERT INTO ontology_projects (id, slug, display_name, description, workspace_slug, owner_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, slug, display_name, description, workspace_slug, owner_id, created_at, updated_at`,
		ids.New(), slug, displayName, description, workspaceSlug, claims.Sub,
	)
	project := &models.OntologyProject{}
	if err := row.Scan(&project.ID, &project.Slug, &project.DisplayName, &project.Description,
		&project.WorkspaceSlug, &project.OwnerID, &project.CreatedAt, &project.UpdatedAt); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to create ontology project: %s", err))
		return
	}

	for i := range body.Folders {
		if _, err := insertProjectFolder(r.Context(), tx, project.ID, claims.Sub, &body.Folders[i]); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to commit ontology project transaction: %s", err))
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

// GetProject mirrors Rust `get_project`.
func (h *ProjectsHandlers) GetProject(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	project, err := loadProject(r.Context(), h.Pool, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if _, err := domain.EnsureProjectViewAccess(r.Context(), h.Pool, claims, id); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, project)
}

// UpdateProject mirrors Rust `update_project`. The `workspace_slug`
// field follows the Rust `Option<Option<String>>` triple-state:
// absent → keep current, explicit null → clear, present string →
// normalise. Detection requires re-decoding the body as
// map[string]json.RawMessage because Go's typed `*string` cannot
// distinguish absent from explicit null on its own.
func (h *ProjectsHandlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	existing, err := loadProject(r.Context(), h.Pool, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if err := ensureProjectOwnerOrAdmin(existing, claims); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	raw, err := readJSONBody(r)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	displayName, err := optionalStringField(raw, "display_name")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	description, err := optionalStringField(raw, "description")
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}

	workspaceSlug := existing.WorkspaceSlug
	if rawWS, present := raw["workspace_slug"]; present {
		trimmed := strings.TrimSpace(string(rawWS))
		switch {
		case trimmed == "null":
			workspaceSlug = nil
		default:
			var s string
			if err := json.Unmarshal(rawWS, &s); err != nil {
				writeJSONErr(w, http.StatusBadRequest, "workspace_slug must be a string")
				return
			}
			normalized, err := normalizeOptionalSlug(&s, "workspace_slug")
			if err != nil {
				writeJSONErr(w, http.StatusBadRequest, err.Error())
				return
			}
			workspaceSlug = normalized
		}
	}

	row := h.Pool.QueryRow(r.Context(),
		`UPDATE ontology_projects
		 SET display_name = COALESCE($2, display_name),
		     description = COALESCE($3, description),
		     workspace_slug = $4,
		     updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, slug, display_name, description, workspace_slug, owner_id, created_at, updated_at`,
		id, displayName, description, workspaceSlug,
	)
	updated := &models.OntologyProject{}
	if err := row.Scan(&updated.ID, &updated.Slug, &updated.DisplayName, &updated.Description,
		&updated.WorkspaceSlug, &updated.OwnerID, &updated.CreatedAt, &updated.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONErr(w, http.StatusNotFound, "ontology project not found")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to update ontology project: %s", err))
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// DeleteProject mirrors Rust `delete_project`.
func (h *ProjectsHandlers) DeleteProject(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	project, err := loadProject(r.Context(), h.Pool, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if err := ensureProjectOwnerOrAdmin(project, claims); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	cmd, err := h.Pool.Exec(r.Context(), `DELETE FROM ontology_projects WHERE id = $1`, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete ontology project: %s", err))
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── memberships ───────────────────────────────────────────────────────

// ListProjectMemberships mirrors Rust `list_project_memberships`.
func (h *ProjectsHandlers) ListProjectMemberships(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	project, err := loadProject(r.Context(), h.Pool, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if _, err := domain.EnsureProjectViewAccess(r.Context(), h.Pool, claims, id); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT project_id, user_id, role, created_at, updated_at
		 FROM ontology_project_memberships
		 WHERE project_id = $1
		 ORDER BY created_at ASC`,
		id,
	)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project memberships: %s", err))
		return
	}
	defer rows.Close()

	out := make([]models.OntologyProjectMembership, 0)
	for rows.Next() {
		var m models.OntologyProjectMembership
		if err := rows.Scan(&m.ProjectID, &m.UserID, &m.Role, &m.CreatedAt, &m.UpdatedAt); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project memberships: %s", err))
			return
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project memberships: %s", err))
		return
	}
	writeJSON(w, http.StatusOK, models.ListOntologyProjectMembershipsResponse{Data: out})
}

// UpsertProjectMembership mirrors Rust `upsert_project_membership`.
// Idempotent: a repeat upsert with the same (project, user, role) is a
// no-op; an upsert with a new role updates the row in place.
func (h *ProjectsHandlers) UpsertProjectMembership(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	var body models.UpsertOntologyProjectMembershipRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	role, err := models.ParseOntologyProjectRole(string(body.Role))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}

	project, err := loadProject(r.Context(), h.Pool, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if err := ensureProjectOwnerOrAdmin(project, claims); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ontology_project_memberships (project_id, user_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (project_id, user_id)
		 DO UPDATE SET role = EXCLUDED.role, updated_at = NOW()
		 RETURNING project_id, user_id, role, created_at, updated_at`,
		id, body.UserID, string(role),
	)
	out := &models.OntologyProjectMembership{}
	if err := row.Scan(&out.ProjectID, &out.UserID, &out.Role, &out.CreatedAt, &out.UpdatedAt); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to upsert ontology project membership: %s", err))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteProjectMembership mirrors Rust `delete_project_membership`.
func (h *ProjectsHandlers) DeleteProjectMembership(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	projectID, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	userID, ok := parseUUIDParam(w, r, "user_id", "user_id")
	if !ok {
		return
	}
	project, err := loadProject(r.Context(), h.Pool, projectID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if err := ensureProjectOwnerOrAdmin(project, claims); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	cmd, err := h.Pool.Exec(r.Context(),
		`DELETE FROM ontology_project_memberships
		 WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete ontology project membership: %s", err))
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSONErr(w, http.StatusNotFound, "ontology project membership not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── folders ───────────────────────────────────────────────────────────

// ListProjectFolders mirrors Rust `list_project_folders`.
func (h *ProjectsHandlers) ListProjectFolders(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	project, err := loadProject(r.Context(), h.Pool, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if _, err := domain.EnsureProjectViewAccess(r.Context(), h.Pool, claims, id); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT id, project_id, parent_folder_id, name, slug, description, created_by, created_at, updated_at
		 FROM ontology_project_folders
		 WHERE project_id = $1
		 ORDER BY created_at ASC`,
		id,
	)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project folders: %s", err))
		return
	}
	defer rows.Close()

	out := make([]models.OntologyProjectFolder, 0)
	for rows.Next() {
		var f models.OntologyProjectFolder
		if err := rows.Scan(&f.ID, &f.ProjectID, &f.ParentFolderID, &f.Name,
			&f.Slug, &f.Description, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project folders: %s", err))
			return
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project folders: %s", err))
		return
	}
	writeJSON(w, http.StatusOK, models.ListOntologyProjectFoldersResponse{Data: out})
}

// CreateProjectFolder mirrors Rust `create_project_folder`. Parent
// folder presence is validated before insert so the FK can stay
// `ON DELETE SET NULL` without surprising callers.
func (h *ProjectsHandlers) CreateProjectFolder(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	projectID, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	var body models.CreateOntologyProjectFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	project, err := loadProject(r.Context(), h.Pool, projectID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if _, err := domain.EnsureProjectEditAccess(r.Context(), h.Pool, claims, projectID); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	if body.ParentFolderID != nil {
		parent, err := loadProjectFolder(r.Context(), h.Pool, projectID, *body.ParentFolderID)
		if err != nil {
			writeJSONErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if parent == nil {
			writeJSONErr(w, http.StatusNotFound, "ontology project parent folder not found")
			return
		}
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to start ontology folder transaction: %s", err))
		return
	}
	defer tx.Rollback(context.Background())

	folder, err := insertProjectFolder(r.Context(), tx, projectID, claims.Sub, &body)
	if err != nil {
		// normalize_folder_name / folder_slug_from_name surface as 500 in
		// the Rust source (db_error). We mirror that to keep parity.
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to commit ontology folder transaction: %s", err))
		return
	}
	writeJSON(w, http.StatusCreated, folder)
}

// ─── resource bindings ─────────────────────────────────────────────────

// ListProjectResources mirrors Rust `list_project_resources`.
func (h *ProjectsHandlers) ListProjectResources(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	project, err := loadProject(r.Context(), h.Pool, id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if _, err := domain.EnsureProjectViewAccess(r.Context(), h.Pool, claims, id); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	rows, err := h.Pool.Query(r.Context(),
		`SELECT project_id, resource_kind, resource_id, bound_by, created_at
		 FROM ontology_project_resources
		 WHERE project_id = $1
		 ORDER BY created_at DESC`,
		id,
	)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project resources: %s", err))
		return
	}
	defer rows.Close()

	out := make([]models.OntologyProjectResourceBinding, 0)
	for rows.Next() {
		var b models.OntologyProjectResourceBinding
		if err := rows.Scan(&b.ProjectID, &b.ResourceKind, &b.ResourceID, &b.BoundBy, &b.CreatedAt); err != nil {
			writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project resources: %s", err))
			return
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to list ontology project resources: %s", err))
		return
	}
	writeJSON(w, http.StatusOK, models.ListOntologyProjectResourcesResponse{Data: out})
}

// BindProjectResource mirrors Rust `bind_project_resource`.
// The (resource_kind, resource_id) primary key makes the UPSERT
// idempotent: re-binding moves the resource between projects without
// orphan rows.
func (h *ProjectsHandlers) BindProjectResource(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	projectID, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	var body models.BindOntologyProjectResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	resourceKind, err := domain.ParseOntologyResourceKind(body.ResourceKind)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}

	project, err := loadProject(r.Context(), h.Pool, projectID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if project == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology project not found")
		return
	}
	if _, err := domain.EnsureProjectEditAccess(r.Context(), h.Pool, claims, projectID); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	ownerID, err := domain.LoadResourceOwnerID(r.Context(), h.Pool, resourceKind, body.ResourceID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ownerID == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology resource not found")
		return
	}
	existingProjectID, err := domain.LoadResourceProjectID(r.Context(), h.Pool, resourceKind, body.ResourceID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to load ontology resource binding: %s", err))
		return
	}
	if err := domain.EnsureResourceManageAccess(r.Context(), h.Pool, claims, *ownerID, existingProjectID); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ontology_project_resources (project_id, resource_kind, resource_id, bound_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (resource_kind, resource_id)
		 DO UPDATE SET project_id = EXCLUDED.project_id, bound_by = EXCLUDED.bound_by, created_at = NOW()
		 RETURNING project_id, resource_kind, resource_id, bound_by, created_at`,
		projectID, resourceKind.String(), body.ResourceID, claims.Sub,
	)
	binding := &models.OntologyProjectResourceBinding{}
	if err := row.Scan(&binding.ProjectID, &binding.ResourceKind, &binding.ResourceID,
		&binding.BoundBy, &binding.CreatedAt); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to bind ontology resource to project: %s", err))
		return
	}
	writeJSON(w, http.StatusOK, binding)
}

// UnbindProjectResource mirrors Rust `unbind_project_resource`. The
// route shape is `/projects/{id}/resources/{kind}/{resource_id}`.
func (h *ProjectsHandlers) UnbindProjectResource(w http.ResponseWriter, r *http.Request) {
	claims, ok := authClaims(w, r)
	if !ok {
		return
	}
	projectID, ok := parseUUIDParam(w, r, "id", "id")
	if !ok {
		return
	}
	resourceKind, err := domain.ParseOntologyResourceKind(chi.URLParam(r, "kind"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resourceID, ok := parseUUIDParam(w, r, "resource_id", "resource_id")
	if !ok {
		return
	}
	if _, err := domain.EnsureProjectEditAccess(r.Context(), h.Pool, claims, projectID); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	row := h.Pool.QueryRow(r.Context(),
		`SELECT project_id, resource_kind, resource_id, bound_by, created_at
		 FROM ontology_project_resources
		 WHERE project_id = $1 AND resource_kind = $2 AND resource_id = $3`,
		projectID, resourceKind.String(), resourceID,
	)
	binding := &models.OntologyProjectResourceBinding{}
	if err := row.Scan(&binding.ProjectID, &binding.ResourceKind, &binding.ResourceID,
		&binding.BoundBy, &binding.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONErr(w, http.StatusNotFound, "ontology project resource binding not found")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to load ontology project resource binding: %s", err))
		return
	}

	ownerID, err := domain.LoadResourceOwnerID(r.Context(), h.Pool, resourceKind, resourceID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ownerID == nil {
		writeJSONErr(w, http.StatusNotFound, "ontology resource not found")
		return
	}
	bound := binding.ProjectID
	if err := domain.EnsureResourceManageAccess(r.Context(), h.Pool, claims, *ownerID, &bound); err != nil {
		writeJSONErr(w, http.StatusForbidden, err.Error())
		return
	}

	cmd, err := h.Pool.Exec(r.Context(),
		`DELETE FROM ontology_project_resources
		 WHERE project_id = $1 AND resource_kind = $2 AND resource_id = $3`,
		projectID, resourceKind.String(), resourceID,
	)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, fmt.Sprintf("failed to unbind ontology resource from project: %s", err))
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSONErr(w, http.StatusNotFound, "ontology project resource binding not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── body helpers ──────────────────────────────────────────────────────

func readJSONBody(r *http.Request) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if r.Body == nil {
		return out, nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// optionalStringField extracts a JSON string at `key` if present and
// non-null. Returns nil/nil when absent or null. Returns an error when
// the value is a non-string JSON type.
func optionalStringField(raw map[string]json.RawMessage, key string) (*string, error) {
	value, present := raw[key]
	if !present {
		return nil, nil
	}
	if strings.TrimSpace(string(value)) == "null" {
		return nil, nil
	}
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return nil, fmt.Errorf("%s must be a string", key)
	}
	return &s, nil
}
