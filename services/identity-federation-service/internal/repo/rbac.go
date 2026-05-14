package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
)

// ─── Users ──────────────────────────────────────────────────────────────

// ListUsers returns the most recent 200 non-deleted users. SG.4
// callers prefer ListUsersFiltered for search + pagination.
func (r *Repo) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT `+userSelectColumns+`
		 FROM users WHERE deleted_at IS NULL
		 ORDER BY created_at DESC LIMIT 200`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUserRows(rows)
}

// ListUsersFiltered returns the users matching `f`. Supports
// case-insensitive substring search on email + username, exact filter
// on organization_id and realm, and active/inactive/deleted status.
// SG.4: drives the admin users UI.
func (r *Repo) ListUsersFiltered(ctx context.Context, f *models.ListUsersFilter) ([]models.User, int64, error) {
	if f == nil {
		f = &models.ListUsersFilter{}
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	args := make([]any, 0, 8)
	conds := make([]string, 0, 6)
	if !f.IncludeDeleted {
		conds = append(conds, "deleted_at IS NULL")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+strings.ToLower(q)+"%")
		conds = append(conds,
			fmt.Sprintf("(LOWER(email) LIKE $%d OR LOWER(COALESCE(username,'')) LIKE $%d OR LOWER(name) LIKE $%d)",
				len(args), len(args), len(args)))
	}
	if f.OrganizationID != nil {
		args = append(args, *f.OrganizationID)
		conds = append(conds, fmt.Sprintf("organization_id = $%d", len(args)))
	}
	if realm := strings.TrimSpace(f.Realm); realm != "" {
		args = append(args, realm)
		conds = append(conds, fmt.Sprintf("realm = $%d", len(args)))
	}
	switch f.Status {
	case "active":
		conds = append(conds, "is_active = true")
	case "inactive":
		conds = append(conds, "is_active = false")
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	// Total before pagination.
	countRow := r.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM users "+where, args...)
	var total int64
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	args = append(args, limit, offset)
	query := "SELECT " + userSelectColumns + " FROM users " + where +
		fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := r.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users, err := scanUserRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// PreregisterUser inserts an admin-invited row. The password hash
// stays empty so login(local) fails until the user completes
// registration; SSO callbacks promote the row by linking the IdP
// subject and setting password_hash to the placeholder marker.
func (r *Repo) PreregisterUser(ctx context.Context, invitedBy uuid.UUID, body *models.PreregisterUserRequest) (*models.User, error) {
	if body == nil || body.Email == "" || body.Name == "" {
		return nil, fmt.Errorf("email and name are required")
	}
	id := uuid.New()
	realm := "local"
	if body.Realm != nil && strings.TrimSpace(*body.Realm) != "" {
		realm = strings.TrimSpace(*body.Realm)
	}
	username := body.Username
	if username == nil || strings.TrimSpace(*username) == "" {
		// Default username to the email localpart.
		local := strings.SplitN(body.Email, "@", 2)[0]
		username = &local
	}
	attrs := body.Attributes
	if len(attrs) == 0 {
		attrs = []byte("{}")
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`INSERT INTO users
		   (id, email, username, name, password_hash, is_active, auth_source, realm,
		    organization_id, attributes, preregistered, invited_by)
		 VALUES ($1, $2, $3, $4, '', TRUE, $5, $5, $6, $7::jsonb, TRUE, $8)`,
		id, strings.TrimSpace(body.Email), strings.TrimSpace(*username), strings.TrimSpace(body.Name),
		realm, body.OrganizationID, attrs, invitedBy,
	); err != nil {
		return nil, fmt.Errorf("insert preregistered user: %w", err)
	}
	for _, roleName := range body.Roles {
		var roleID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, roleName).Scan(&roleID); err != nil {
			return nil, fmt.Errorf("lookup role %s: %w", roleName, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			id, roleID,
		); err != nil {
			return nil, fmt.Errorf("assign role: %w", err)
		}
	}
	for _, gid := range body.Groups {
		if _, err := tx.Exec(ctx,
			`INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			gid, id,
		); err != nil {
			return nil, fmt.Errorf("assign group: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return r.FindUserByID(ctx, id)
}

// SoftDeleteUser marks a user as deleted_at = NOW() and inactivates
// the row. Refresh tokens are revoked in the same transaction (SG.4:
// "Disable or invalidate tokens for inactive/disabled users").
func (r *Repo) SoftDeleteUser(ctx context.Context, id uuid.UUID) error {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx,
		`UPDATE users SET deleted_at = $2, is_active = FALSE, updated_at = $2
		 WHERE id = $1 AND deleted_at IS NULL`,
		id, now,
	); err != nil {
		return fmt.Errorf("soft delete user: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2
		 WHERE user_id = $1 AND revoked_at IS NULL`,
		id, now,
	); err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return tx.Commit(ctx)
}

// UndeleteUser clears the deleted_at tombstone but leaves is_active
// = false so an admin must explicitly reactivate the user.
func (r *Repo) UndeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`UPDATE users SET deleted_at = NULL, updated_at = NOW()
		 WHERE id = $1`,
		id,
	)
	return err
}

// RevokeAllUserRefreshTokens marks every active refresh token for
// the user as revoked. SG.4 calls this whenever a user is
// deactivated (is_active flipped to false), soft-deleted, or has
// their account locked.
func (r *Repo) RevokeAllUserRefreshTokens(ctx context.Context, userID uuid.UUID, at time.Time) (int64, error) {
	tag, err := r.Pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2
		 WHERE user_id = $1 AND revoked_at IS NULL`,
		userID, at,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// StampLogin updates last_login_at / last_login_ip. Called from the
// Login handler in a follow-up patch; safe to invoke from SSO
// callbacks too.
func (r *Repo) StampLogin(ctx context.Context, userID uuid.UUID, at time.Time, ip string) error {
	var ipPtr *string
	if ip != "" {
		ipPtr = &ip
	}
	_, err := r.Pool.Exec(ctx,
		`UPDATE users SET last_login_at = $2, last_login_ip = $3, updated_at = $2
		 WHERE id = $1`,
		userID, at, ipPtr,
	)
	return err
}

// UserTokenSummary aggregates refresh-token state for the SG.4
// inspect endpoint. The caller turns this into the wire-shape
// `TokenSummary` (the repo result includes a few extra fields that
// help internal callers without leaking to wire).
type UserTokenSummary struct {
	ActiveCount   int
	RevokedCount  int
	NextExpiresAt *time.Time
}

// SummarizeUserTokens returns the refresh-token aggregate for the
// SG.4 inspect view.
func (r *Repo) SummarizeUserTokens(ctx context.Context, userID uuid.UUID) (*UserTokenSummary, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT
		   COUNT(*) FILTER (WHERE revoked_at IS NULL AND expires_at > NOW()) AS active,
		   COUNT(*) FILTER (WHERE revoked_at IS NOT NULL OR expires_at <= NOW()) AS revoked,
		   MIN(expires_at) FILTER (WHERE revoked_at IS NULL AND expires_at > NOW()) AS next_expires
		 FROM refresh_tokens
		 WHERE user_id = $1`,
		userID,
	)
	out := &UserTokenSummary{}
	if err := row.Scan(&out.ActiveCount, &out.RevokedCount, &out.NextExpiresAt); err != nil {
		return nil, err
	}
	return out, nil
}

// ListUserExternalIdentities returns every IdP binding row for a
// user. SG.4 inspect view uses this to surface realm membership.
func (r *Repo) ListUserExternalIdentities(ctx context.Context, userID uuid.UUID) ([]ExternalIdentity, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, user_id, provider, external_id, COALESCE(email, ''), last_login_at, created_at
		 FROM user_external_identities
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ExternalIdentity, 0)
	for rows.Next() {
		e := ExternalIdentity{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.Provider, &e.ExternalID, &e.Email, &e.LastLoginAt, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListUserGroups returns the group rows the user belongs to,
// projected to id+name+description for the SG.4 inspect view.
func (r *Repo) ListUserGroups(ctx context.Context, userID uuid.UUID) ([]models.Group, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT g.id, g.name, g.description, g.created_at
		 FROM groups g
		 INNER JOIN group_members gm ON gm.group_id = g.id
		 WHERE gm.user_id = $1
		 ORDER BY g.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Group, 0)
	for rows.Next() {
		g := models.Group{}
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// CountActiveAPIKeys returns the count of non-revoked, non-expired
// api_keys for a user.
func (r *Repo) CountActiveAPIKeys(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM api_keys
		 WHERE user_id = $1 AND revoked_at IS NULL
		   AND (expires_at IS NULL OR expires_at > NOW())`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// scanUserRows is the shared streaming scan used by ListUsers and
// ListUsersFiltered.
func scanUserRows(rows pgx.Rows) ([]models.User, error) {
	out := make([]models.User, 0)
	for rows.Next() {
		u := models.User{}
		if err := rows.Scan(
			&u.ID, &u.Email, &u.Username, &u.Name, &u.PasswordHash,
			&u.IsActive, &u.AuthSource, &u.Realm, &u.MFAEnforced, &u.OrganizationID, &u.Attributes,
			&u.LastLoginAt, &u.LastLoginIP, &u.Preregistered, &u.InvitedBy, &u.DeletedAt,
			&u.CreatedAt, &u.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUser applies non-nil fields of `body`. SG.4 extended the
// patch surface to include username, realm, organization_id, and
// attributes.
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
	username := current.Username
	if body.Username != nil {
		if v := strings.TrimSpace(*body.Username); v != "" {
			username = &v
		}
	}
	realm := current.Realm
	if body.Realm != nil {
		if v := strings.TrimSpace(*body.Realm); v != "" {
			realm = v
		}
	}
	active := current.IsActive
	if body.IsActive != nil {
		active = *body.IsActive
	}
	mfa := current.MFAEnforced
	if body.MFAEnforced != nil {
		mfa = *body.MFAEnforced
	}
	orgID := current.OrganizationID
	if body.OrganizationID != nil {
		orgID = *body.OrganizationID
	}
	attrs := current.Attributes
	if body.Attributes != nil {
		attrs = *body.Attributes
	}
	if len(attrs) == 0 {
		attrs = []byte("{}")
	}
	_, err = r.Pool.Exec(ctx,
		`UPDATE users SET
		   name = $2, username = $3, realm = $4, is_active = $5, mfa_enforced = $6,
		   organization_id = $7, attributes = $8::jsonb, updated_at = NOW()
		 WHERE id = $1`,
		id, name, username, realm, active, mfa, orgID, attrs,
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
