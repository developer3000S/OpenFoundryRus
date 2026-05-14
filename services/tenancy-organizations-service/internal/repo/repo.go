// Package repo holds SQL queries + embedded migrations for tenancy-organizations-service.
package repo

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/tenancy-organizations-service/internal/models"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every embedded migration in lex order.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// Repo wraps the SQL surface.
type Repo struct{ Pool *pgxpool.Pool }

// ─── Organizations ──────────────────────────────────────────────────────

const orgSelectColumns = `id, slug, display_name, description, contact_email,
	organization_type, default_workspace, tenant_tier, status,
	metadata, settings, quotas, created_at, updated_at`

func (r *Repo) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+orgSelectColumns+`
		 FROM tenancy_organizations ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Organization, 0)
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (r *Repo) GetOrganization(ctx context.Context, id uuid.UUID) (*models.Organization, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+orgSelectColumns+`
		 FROM tenancy_organizations WHERE id = $1`, id)
	o, err := scanOrg(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return o, err
}

func (r *Repo) CreateOrganization(ctx context.Context, body *models.CreateOrganizationRequest) (*models.Organization, error) {
	id := ids.New()
	orgType := derefStrTrim(body.OrganizationType, "enterprise")
	status := derefStrTrim(body.Status, "active")
	now := time.Now().UTC()
	defaultWS := trimPtr(body.DefaultWorkspace)
	tier := trimPtr(body.TenantTier)
	description := ""
	if body.Description != nil {
		description = strings.TrimSpace(*body.Description)
	}
	contactEmail := trimPtr(body.ContactEmail)
	metadata := mustMarshalJSONMap(body.Metadata)
	settings := mustMarshalJSONMap(body.Settings)
	quotas := mustMarshalJSONMap(body.Quotas)
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO tenancy_organizations
		 (id, slug, display_name, description, contact_email,
		  organization_type, default_workspace, tenant_tier, status,
		  metadata, settings, quotas, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
		         $10::jsonb, $11::jsonb, $12::jsonb, $13, $13)`,
		id, strings.TrimSpace(body.Slug), strings.TrimSpace(body.DisplayName),
		description, contactEmail, orgType, defaultWS, tier, status,
		metadata, settings, quotas, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert organization: %w", err)
	}
	return r.GetOrganization(ctx, id)
}

func (r *Repo) UpdateOrganization(ctx context.Context, id uuid.UUID, body *models.UpdateOrganizationRequest) (*models.Organization, error) {
	current, err := r.GetOrganization(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	dn := current.DisplayName
	if body.DisplayName != nil {
		dn = *body.DisplayName
	}
	desc := current.Description
	if body.Description != nil {
		desc = *body.Description
	}
	contact := current.ContactEmail
	if body.ContactEmail != nil {
		contact = body.ContactEmail
	}
	ot := current.OrganizationType
	if body.OrganizationType != nil {
		ot = *body.OrganizationType
	}
	dw := current.DefaultWorkspace
	if body.DefaultWorkspace != nil {
		dw = body.DefaultWorkspace
	}
	tt := current.TenantTier
	if body.TenantTier != nil {
		tt = body.TenantTier
	}
	st := current.Status
	if body.Status != nil {
		st = *body.Status
	}
	metadata := mustMarshalJSONMap(current.Metadata)
	if body.Metadata != nil {
		metadata = mustMarshalJSONMap(*body.Metadata)
	}
	settings := mustMarshalJSONMap(current.Settings)
	if body.Settings != nil {
		settings = mustMarshalJSONMap(*body.Settings)
	}
	quotas := mustMarshalJSONMap(current.Quotas)
	if body.Quotas != nil {
		quotas = mustMarshalJSONMap(*body.Quotas)
	}
	_, err = r.Pool.Exec(ctx,
		`UPDATE tenancy_organizations
		 SET display_name=$2, description=$3, contact_email=$4,
		     organization_type=$5, default_workspace=$6, tenant_tier=$7,
		     status=$8, metadata=$9::jsonb, settings=$10::jsonb, quotas=$11::jsonb,
		     updated_at=$12
		 WHERE id=$1`,
		id, dn, desc, contact, ot, dw, tt, st, metadata, settings, quotas, time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	return r.GetOrganization(ctx, id)
}

func (r *Repo) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM tenancy_organizations WHERE id = $1`, id)
	return err
}

// ─── Enrollments ────────────────────────────────────────────────────────

func (r *Repo) ListEnrollmentsByOrg(ctx context.Context, orgID uuid.UUID) ([]models.Enrollment, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, organization_id, user_id, workspace_slug, role_slug, status, created_at, updated_at
		 FROM tenancy_enrollments WHERE organization_id = $1 ORDER BY created_at DESC LIMIT 500`,
		orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Enrollment, 0)
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (r *Repo) CreateEnrollment(ctx context.Context, body *models.CreateEnrollmentRequest) (*models.Enrollment, error) {
	id := ids.New()
	status := "active"
	if body.Status != nil && *body.Status != "" {
		status = *body.Status
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO tenancy_enrollments (id, organization_id, user_id, workspace_slug, role_slug, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, organization_id, user_id, workspace_slug, role_slug, status, created_at, updated_at`,
		id, body.OrganizationID, body.UserID, body.WorkspaceSlug, body.RoleSlug, status,
	)
	return scanEnrollment(row)
}

func (r *Repo) DeleteEnrollment(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM tenancy_enrollments WHERE id = $1`, id)
	return err
}

// ─── helpers ────────────────────────────────────────────────────────────

type rowLikeT interface{ Scan(...any) error }

func scanOrg(r rowLikeT) (*models.Organization, error) {
	o := &models.Organization{}
	var metadata, settings, quotas []byte
	err := r.Scan(
		&o.ID, &o.Slug, &o.DisplayName, &o.Description, &o.ContactEmail,
		&o.OrganizationType, &o.DefaultWorkspace, &o.TenantTier, &o.Status,
		&metadata, &settings, &quotas, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	o.Metadata = unmarshalJSONMap(metadata)
	o.Settings = unmarshalJSONMap(settings)
	o.Quotas = unmarshalJSONMap(quotas)
	return o, nil
}

// mustMarshalJSONMap returns the JSON encoding of m, defaulting to
// `{}` when the input is nil or unmarshalable. The DB columns are
// JSONB NOT NULL DEFAULT '{}' so we keep the same invariant on writes.
func mustMarshalJSONMap(m map[string]any) []byte {
	if m == nil {
		return []byte("{}")
	}
	out, err := json.Marshal(m)
	if err != nil || len(out) == 0 {
		return []byte("{}")
	}
	return out
}

// unmarshalJSONMap inverts mustMarshalJSONMap. A NULL/empty/invalid
// column collapses to an empty map so the wire payload always carries
// the object (never null).
func unmarshalJSONMap(b []byte) map[string]any {
	if len(b) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	if out == nil {
		return map[string]any{}
	}
	return out
}

// ─── Organization admins ────────────────────────────────────────────────

func (r *Repo) ListOrganizationAdmins(ctx context.Context, orgID uuid.UUID) ([]models.OrganizationAdmin, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT organization_id, user_id, scope, granted_by, created_at
		 FROM tenancy_organization_admins
		 WHERE organization_id = $1
		 ORDER BY created_at DESC LIMIT 500`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OrganizationAdmin, 0)
	for rows.Next() {
		a := models.OrganizationAdmin{}
		if err := rows.Scan(&a.OrganizationID, &a.UserID, &a.Scope, &a.GrantedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repo) UpsertOrganizationAdmin(ctx context.Context, orgID uuid.UUID, body *models.CreateOrganizationAdminRequest) (*models.OrganizationAdmin, error) {
	scope := derefStrTrim(body.Scope, "enrollment_admin")
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO tenancy_organization_admins (organization_id, user_id, scope, granted_by)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (organization_id, user_id, scope) DO UPDATE SET granted_by = EXCLUDED.granted_by
		 RETURNING organization_id, user_id, scope, granted_by, created_at`,
		orgID, body.UserID, scope, body.GrantedBy,
	)
	a := &models.OrganizationAdmin{}
	if err := row.Scan(&a.OrganizationID, &a.UserID, &a.Scope, &a.GrantedBy, &a.CreatedAt); err != nil {
		return nil, err
	}
	return a, nil
}

func (r *Repo) DeleteOrganizationAdmin(ctx context.Context, orgID, userID uuid.UUID, scope string) error {
	_, err := r.Pool.Exec(ctx,
		`DELETE FROM tenancy_organization_admins
		 WHERE organization_id = $1 AND user_id = $2 AND scope = $3`,
		orgID, userID, scope,
	)
	return err
}

// IsOrganizationAdmin returns true when the user holds any admin scope
// on the organization. Used by membership-enforcement.
func (r *Repo) IsOrganizationAdmin(ctx context.Context, orgID, userID uuid.UUID) (bool, error) {
	var count int
	err := r.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tenancy_organization_admins
		 WHERE organization_id = $1 AND user_id = $2`, orgID, userID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ─── Organization guests ────────────────────────────────────────────────

func (r *Repo) ListOrganizationGuests(ctx context.Context, orgID uuid.UUID) ([]models.OrganizationGuest, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT organization_id, user_id, primary_organization_id, status,
		        invited_by, expires_at, created_at, updated_at
		 FROM tenancy_organization_guests
		 WHERE organization_id = $1
		 ORDER BY created_at DESC LIMIT 500`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.OrganizationGuest, 0)
	for rows.Next() {
		g := models.OrganizationGuest{}
		if err := rows.Scan(&g.OrganizationID, &g.UserID, &g.PrimaryOrganizationID,
			&g.Status, &g.InvitedBy, &g.ExpiresAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *Repo) UpsertOrganizationGuest(ctx context.Context, orgID uuid.UUID, body *models.CreateOrganizationGuestRequest) (*models.OrganizationGuest, error) {
	status := derefStrTrim(body.Status, "active")
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO tenancy_organization_guests
		   (organization_id, user_id, primary_organization_id, status, invited_by, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (organization_id, user_id) DO UPDATE
		   SET primary_organization_id = EXCLUDED.primary_organization_id,
		       status = EXCLUDED.status,
		       invited_by = EXCLUDED.invited_by,
		       expires_at = EXCLUDED.expires_at,
		       updated_at = NOW()
		 RETURNING organization_id, user_id, primary_organization_id, status,
		           invited_by, expires_at, created_at, updated_at`,
		orgID, body.UserID, body.PrimaryOrganizationID, status, body.InvitedBy, body.ExpiresAt,
	)
	g := &models.OrganizationGuest{}
	err := row.Scan(&g.OrganizationID, &g.UserID, &g.PrimaryOrganizationID,
		&g.Status, &g.InvitedBy, &g.ExpiresAt, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *Repo) DeleteOrganizationGuest(ctx context.Context, orgID, userID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`DELETE FROM tenancy_organization_guests
		 WHERE organization_id = $1 AND user_id = $2`, orgID, userID,
	)
	return err
}

// IsOrganizationMember reports whether a user has any enrolment, admin
// grant, or active guest record on an organization. Used by
// membership-enforcement to gate resource discovery (SG.2).
func (r *Repo) IsOrganizationMember(ctx context.Context, orgID, userID uuid.UUID) (bool, error) {
	var count int
	err := r.Pool.QueryRow(ctx,
		`SELECT
		   (SELECT COUNT(*) FROM tenancy_enrollments
		      WHERE organization_id = $1 AND user_id = $2 AND status = 'active')
		 + (SELECT COUNT(*) FROM tenancy_organization_admins
		      WHERE organization_id = $1 AND user_id = $2)
		 + (SELECT COUNT(*) FROM tenancy_organization_guests
		      WHERE organization_id = $1 AND user_id = $2 AND status = 'active'
		            AND (expires_at IS NULL OR expires_at > NOW()))`,
		orgID, userID,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ─── Tenancy spaces ─────────────────────────────────────────────────────

const tenancySpaceColumns = `id, organization_id, slug, display_name, description,
	settings, quotas, status, created_at, updated_at`

func (r *Repo) ListTenancySpaces(ctx context.Context, orgID uuid.UUID) ([]models.TenancySpace, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+tenancySpaceColumns+`
		 FROM tenancy_spaces WHERE organization_id = $1
		 ORDER BY created_at DESC LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.TenancySpace, 0)
	for rows.Next() {
		s, err := scanTenancySpace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *Repo) GetTenancySpace(ctx context.Context, id uuid.UUID) (*models.TenancySpace, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+tenancySpaceColumns+`
		 FROM tenancy_spaces WHERE id = $1`, id)
	s, err := scanTenancySpace(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func (r *Repo) CreateTenancySpace(ctx context.Context, orgID uuid.UUID, body *models.CreateTenancySpaceRequest) (*models.TenancySpace, error) {
	id := ids.New()
	now := time.Now().UTC()
	description := ""
	if body.Description != nil {
		description = strings.TrimSpace(*body.Description)
	}
	status := derefStrTrim(body.Status, "active")
	settings := mustMarshalJSONMap(body.Settings)
	quotas := mustMarshalJSONMap(body.Quotas)
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO tenancy_spaces
		   (id, organization_id, slug, display_name, description, settings, quotas, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $9, $9)`,
		id, orgID, strings.TrimSpace(body.Slug), strings.TrimSpace(body.DisplayName),
		description, settings, quotas, status, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert tenancy_space: %w", err)
	}
	return r.GetTenancySpace(ctx, id)
}

func (r *Repo) UpdateTenancySpace(ctx context.Context, id uuid.UUID, body *models.UpdateTenancySpaceRequest) (*models.TenancySpace, error) {
	current, err := r.GetTenancySpace(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	dn := current.DisplayName
	if body.DisplayName != nil {
		dn = *body.DisplayName
	}
	desc := current.Description
	if body.Description != nil {
		desc = *body.Description
	}
	st := current.Status
	if body.Status != nil {
		st = *body.Status
	}
	settings := mustMarshalJSONMap(current.Settings)
	if body.Settings != nil {
		settings = mustMarshalJSONMap(*body.Settings)
	}
	quotas := mustMarshalJSONMap(current.Quotas)
	if body.Quotas != nil {
		quotas = mustMarshalJSONMap(*body.Quotas)
	}
	_, err = r.Pool.Exec(ctx,
		`UPDATE tenancy_spaces
		 SET display_name=$2, description=$3, status=$4, settings=$5::jsonb,
		     quotas=$6::jsonb, updated_at=$7
		 WHERE id=$1`,
		id, dn, desc, st, settings, quotas, time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	return r.GetTenancySpace(ctx, id)
}

func (r *Repo) DeleteTenancySpace(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM tenancy_spaces WHERE id = $1`, id)
	return err
}

func scanTenancySpace(r rowLikeT) (*models.TenancySpace, error) {
	s := &models.TenancySpace{}
	var settings, quotas []byte
	err := r.Scan(
		&s.ID, &s.OrganizationID, &s.Slug, &s.DisplayName, &s.Description,
		&settings, &quotas, &s.Status, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.Settings = unmarshalJSONMap(settings)
	s.Quotas = unmarshalJSONMap(quotas)
	return s, nil
}

func scanEnrollment(r rowLikeT) (*models.Enrollment, error) {
	e := &models.Enrollment{}
	err := r.Scan(&e.ID, &e.OrganizationID, &e.UserID, &e.WorkspaceSlug,
		&e.RoleSlug, &e.Status, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func derefStrTrim(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return fallback
	}
	return v
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}
