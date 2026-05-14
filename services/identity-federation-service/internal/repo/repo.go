// Package repo wraps the SQL surface for identity-federation-service slice 1.
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

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
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

// Repo bundles the queries this service needs.
type Repo struct{ Pool *pgxpool.Pool }

// ─── Users ──────────────────────────────────────────────────────────────

// CountUsers returns the total user count (used by /auth/bootstrap-status
// to decide whether the first-admin path is still open).
func (r *Repo) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := r.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// userSelectColumns is the canonical column projection used by every
// users-table SELECT in this package. SG.4 (2026-05-14) extended the
// projection with username, realm, last_login_at, last_login_ip,
// deleted_at, preregistered and invited_by.
const userSelectColumns = `id, email, username, name, password_hash,
	is_active, auth_source, realm, mfa_enforced, organization_id, attributes,
	last_login_at, last_login_ip, preregistered, invited_by, deleted_at,
	created_at, updated_at`

// FindUserByEmail returns the user row or nil when absent.
func (r *Repo) FindUserByEmail(ctx context.Context, email string) (*models.User, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+userSelectColumns+`
		 FROM users WHERE email = $1`,
		email,
	)
	return scanUser(row)
}

// FindUserByID returns the user row or nil when absent.
func (r *Repo) FindUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+userSelectColumns+`
		 FROM users WHERE id = $1`,
		id,
	)
	return scanUser(row)
}

// FindUserByUsername returns the user row keyed by case-insensitive
// username, or nil when absent.
func (r *Repo) FindUserByUsername(ctx context.Context, username string) (*models.User, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT `+userSelectColumns+`
		 FROM users WHERE LOWER(username) = LOWER($1)`,
		username,
	)
	return scanUser(row)
}

func scanUser(row pgx.Row) (*models.User, error) {
	u := &models.User{}
	err := row.Scan(
		&u.ID, &u.Email, &u.Username, &u.Name, &u.PasswordHash,
		&u.IsActive, &u.AuthSource, &u.Realm, &u.MFAEnforced, &u.OrganizationID, &u.Attributes,
		&u.LastLoginAt, &u.LastLoginIP, &u.Preregistered, &u.InvitedBy, &u.DeletedAt,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// CreateUserAndAssignRole inserts a user + assigns a role inside a
// single transaction. Mirrors the Rust register-flow's pg advisory
// lock + COUNT(*)-based first-admin election.
//
// Returns:
//   - the persisted user
//   - the assigned role name
//   - errUserExists when the email is already taken
func (r *Repo) CreateUserAndAssignRole(
	ctx context.Context,
	id uuid.UUID,
	email, name, passwordHash string,
) (*models.User, string, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Advisory lock: serialises first-user bootstrap across replicas.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(8_514_200_001)); err != nil {
		return nil, "", fmt.Errorf("advisory lock: %w", err)
	}

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).
		Scan(&exists); err != nil {
		return nil, "", fmt.Errorf("check existing email: %w", err)
	}
	if exists {
		return nil, "", ErrUserExists
	}

	var existingCount int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&existingCount); err != nil {
		return nil, "", fmt.Errorf("count users: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO users (id, email, name, password_hash, is_active, auth_source)
		 VALUES ($1, $2, $3, $4, true, 'local')`,
		id, email, name, passwordHash,
	); err != nil {
		return nil, "", fmt.Errorf("insert user: %w", err)
	}

	roleName := "viewer"
	if existingCount == 0 {
		roleName = "admin"
	}

	var roleID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID); err != nil {
		return nil, "", fmt.Errorf("lookup role %s: %w", roleName, err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		id, roleID,
	); err != nil {
		return nil, "", fmt.Errorf("assign role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", fmt.Errorf("commit: %w", err)
	}

	user, err := r.FindUserByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	return user, roleName, nil
}

// ErrUserExists signals the email is already registered.
var ErrUserExists = errors.New("email already registered")

// ─── Refresh tokens ─────────────────────────────────────────────────────

// InsertRefreshToken persists a hashed refresh token.
func (r *Repo) InsertRefreshToken(ctx context.Context, t *models.RefreshTokenRow) error {
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, family_id, issued_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		t.ID, t.UserID, t.TokenHash, t.FamilyID, t.IssuedAt, t.ExpiresAt,
	)
	return err
}

// FindRefreshToken returns the row keyed by its token hash, or nil when absent.
func (r *Repo) FindRefreshToken(ctx context.Context, tokenHash string) (*models.RefreshTokenRow, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, user_id, token_hash, family_id, issued_at, expires_at, revoked_at
		 FROM refresh_tokens WHERE token_hash = $1`,
		tokenHash,
	)
	t := &models.RefreshTokenRow{}
	err := row.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.FamilyID, &t.IssuedAt, &t.ExpiresAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// RevokeRefreshFamily marks every token in a family as revoked.
// Called on replay detection: if a token from a family is presented
// after a sibling has already been used, the whole family is killed
// to defend against stolen-token reuse.
func (r *Repo) RevokeRefreshFamily(ctx context.Context, familyID uuid.UUID, at time.Time) error {
	_, err := r.Pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2
		 WHERE family_id = $1 AND revoked_at IS NULL`,
		familyID, at,
	)
	return err
}

// MarkRefreshUsed marks a single refresh token as revoked (one-time use).
func (r *Repo) MarkRefreshUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.Pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`,
		id, at,
	)
	return err
}
