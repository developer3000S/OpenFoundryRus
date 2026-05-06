// Package repo holds SQL queries + embedded migrations for tenancy-organizations-service.
package repo

import (
	"context"
	"embed"
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

func (r *Repo) ListOrganizations(ctx context.Context) ([]models.Organization, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, slug, display_name, organization_type, default_workspace, tenant_tier,
		        status, created_at, updated_at
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
		`SELECT id, slug, display_name, organization_type, default_workspace, tenant_tier,
		        status, created_at, updated_at
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
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO tenancy_organizations
		 (id, slug, display_name, organization_type, default_workspace, tenant_tier, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		id, strings.TrimSpace(body.Slug), strings.TrimSpace(body.DisplayName),
		orgType, defaultWS, tier, status, now,
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
	_, err = r.Pool.Exec(ctx,
		`UPDATE tenancy_organizations
		 SET display_name=$2, organization_type=$3, default_workspace=$4, tenant_tier=$5,
		     status=$6, updated_at=$7
		 WHERE id=$1`,
		id, dn, ot, dw, tt, st, time.Now().UTC(),
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
	err := r.Scan(&o.ID, &o.Slug, &o.DisplayName, &o.OrganizationType,
		&o.DefaultWorkspace, &o.TenantTier, &o.Status, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return o, nil
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
