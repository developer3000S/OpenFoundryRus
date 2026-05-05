package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
)

// ─── Users ──────────────────────────────────────────────────────────────

// ListUsers returns the most recent 200 users.
func (r *Repo) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, email, name, password_hash, is_active, auth_source,
		        mfa_enforced, organization_id, attributes, created_at, updated_at
		 FROM users ORDER BY created_at DESC LIMIT 200`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.User, 0)
	for rows.Next() {
		u := models.User{}
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsActive, &u.AuthSource,
			&u.MFAEnforced, &u.OrganizationID, &u.Attributes, &u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUser applies non-nil fields of `body`.
func (r *Repo) UpdateUser(ctx context.Context, id uuid.UUID, body *models.UpdateUserRequest) (*models.User, error) {
	current, err := r.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	name := current.Name
	if body.Name != nil {
		name = *body.Name
	}
	active := current.IsActive
	if body.IsActive != nil {
		active = *body.IsActive
	}
	mfa := current.MFAEnforced
	if body.MFAEnforced != nil {
		mfa = *body.MFAEnforced
	}
	_, err = r.Pool.Exec(ctx,
		`UPDATE users SET name = $2, is_active = $3, mfa_enforced = $4, updated_at = NOW()
		 WHERE id = $1`,
		id, name, active, mfa,
	)
	if err != nil {
		return nil, err
	}
	return r.FindUserByID(ctx, id)
}

// DeleteUser removes a user (cascades user_roles, group_members, api_keys, etc.).
func (r *Repo) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

// ListUserRoles returns roles assigned to a user.
func (r *Repo) ListUserRoles(ctx context.Context, userID uuid.UUID) ([]models.Role, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT r.id, r.name, r.description, r.created_at
		 FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1
		 ORDER BY r.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Role, 0)
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

// AssignRoleToUser is idempotent.
func (r *Repo) AssignRoleToUser(ctx context.Context, userID, roleID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, roleID,
	)
	return err
}

// RevokeRoleFromUser is a no-op when the assignment doesn't exist.
func (r *Repo) RevokeRoleFromUser(ctx context.Context, userID, roleID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`,
		userID, roleID,
	)
	return err
}

// ─── Roles ──────────────────────────────────────────────────────────────

func (r *Repo) ListRoles(ctx context.Context) ([]models.Role, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, name, description, created_at FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Role, 0)
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

func (r *Repo) GetRole(ctx context.Context, id uuid.UUID) (*models.Role, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, name, description, created_at FROM roles WHERE id = $1`, id)
	role := &models.Role{}
	if err := row.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return role, nil
}

func (r *Repo) CreateRole(ctx context.Context, body *models.CreateRoleRequest) (*models.Role, error) {
	id := uuid.New()
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO roles (id, name, description) VALUES ($1, $2, $3)`,
		id, body.Name, body.Description,
	)
	if err != nil {
		return nil, err
	}
	return r.GetRole(ctx, id)
}

func (r *Repo) UpdateRole(ctx context.Context, id uuid.UUID, body *models.UpdateRoleRequest) (*models.Role, error) {
	_, err := r.Pool.Exec(ctx,
		`UPDATE roles SET name = $2, description = $3 WHERE id = $1`,
		id, body.Name, body.Description,
	)
	if err != nil {
		return nil, err
	}
	return r.GetRole(ctx, id)
}

func (r *Repo) DeleteRole(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	return err
}

// ─── Permissions ────────────────────────────────────────────────────────

func (r *Repo) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, resource, action, created_at FROM permissions ORDER BY resource, action`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Permission, 0)
	for rows.Next() {
		var p models.Permission
		if err := rows.Scan(&p.ID, &p.Resource, &p.Action, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) CreatePermission(ctx context.Context, body *models.CreatePermissionRequest) (*models.Permission, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO permissions (resource, action) VALUES ($1, $2)
		 ON CONFLICT (resource, action) DO UPDATE SET resource = EXCLUDED.resource
		 RETURNING id, resource, action, created_at`,
		body.Resource, body.Action,
	)
	p := &models.Permission{}
	if err := row.Scan(&p.ID, &p.Resource, &p.Action, &p.CreatedAt); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repo) DeletePermission(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM permissions WHERE id = $1`, id)
	return err
}

func (r *Repo) AssignPermissionToRole(ctx context.Context, roleID, permID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		roleID, permID,
	)
	return err
}

func (r *Repo) RevokePermissionFromRole(ctx context.Context, roleID, permID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`DELETE FROM role_permissions WHERE role_id = $1 AND permission_id = $2`,
		roleID, permID,
	)
	return err
}

// ─── Groups ─────────────────────────────────────────────────────────────

func (r *Repo) ListGroups(ctx context.Context) ([]models.Group, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, name, description, created_at FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Group, 0)
	for rows.Next() {
		var g models.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *Repo) GetGroup(ctx context.Context, id uuid.UUID) (*models.Group, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, name, description, created_at FROM groups WHERE id = $1`, id)
	g := &models.Group{}
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return g, nil
}

func (r *Repo) CreateGroup(ctx context.Context, body *models.CreateGroupRequest) (*models.Group, error) {
	id := uuid.New()
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO groups (id, name, description) VALUES ($1, $2, $3)`,
		id, body.Name, body.Description,
	)
	if err != nil {
		return nil, err
	}
	return r.GetGroup(ctx, id)
}

func (r *Repo) UpdateGroup(ctx context.Context, id uuid.UUID, body *models.UpdateGroupRequest) (*models.Group, error) {
	_, err := r.Pool.Exec(ctx,
		`UPDATE groups SET name = $2, description = $3 WHERE id = $1`,
		id, body.Name, body.Description,
	)
	if err != nil {
		return nil, err
	}
	return r.GetGroup(ctx, id)
}

func (r *Repo) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	return err
}

func (r *Repo) AddGroupMember(ctx context.Context, groupID, userID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		groupID, userID,
	)
	return err
}

func (r *Repo) RemoveGroupMember(ctx context.Context, groupID, userID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`,
		groupID, userID,
	)
	return err
}

// ─── API keys ───────────────────────────────────────────────────────────

func (r *Repo) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]models.APIKey, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, user_id, name, last_used_at, expires_at, created_at, revoked_at
		 FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.APIKey, 0)
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CreateAPIKey persists a hashed token. Returns the row + caller is
// expected to render the plaintext (which it generated).
func (r *Repo) CreateAPIKey(ctx context.Context, userID uuid.UUID, name, keyHash string, expiresAt *time.Time) (*models.APIKey, error) {
	id := uuid.New()
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, name, key_hash, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, userID, name, keyHash, expiresAt,
	)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx,
		`SELECT id, user_id, name, last_used_at, expires_at, created_at, revoked_at
		 FROM api_keys WHERE id = $1`, id)
	k := &models.APIKey{}
	if err := row.Scan(&k.ID, &k.UserID, &k.Name, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt, &k.RevokedAt); err != nil {
		return nil, err
	}
	return k, nil
}

func (r *Repo) RevokeAPIKey(ctx context.Context, userID, id uuid.UUID, at time.Time) error {
	_, err := r.Pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = $3 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		id, userID, at,
	)
	return err
}
