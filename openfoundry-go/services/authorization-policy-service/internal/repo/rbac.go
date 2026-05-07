package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/authorization-policy-service/internal/models"
)

func tenantPredicate(alias string, tenantID *uuid.UUID, startArg int) (string, []any) {
	col := "tenant_id"
	if alias != "" {
		col = alias + ".tenant_id"
	}
	if tenantID == nil {
		return col + " IS NULL", nil
	}
	return fmt.Sprintf("%s = $%d", col, startArg), []any{*tenantID}
}

func (r *Repo) ListPermissions(ctx context.Context, tenantID *uuid.UUID) ([]models.Permission, error) {
	pred, args := tenantPredicate("", tenantID, 1)
	rows, err := r.Pool.Query(ctx, `SELECT id, tenant_id, resource, action, description, created_at FROM permissions WHERE `+pred+` ORDER BY resource, action`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Permission{}
	for rows.Next() {
		var p models.Permission
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Resource, &p.Action, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) CreatePermission(ctx context.Context, tenantID *uuid.UUID, body *models.CreatePermissionRequest) (*models.Permission, error) {
	row := r.Pool.QueryRow(ctx, `INSERT INTO permissions (id, tenant_id, resource, action, description) VALUES ($1,$2,$3,$4,$5) RETURNING id, tenant_id, resource, action, description, created_at`, uuid.New(), tenantID, body.Resource, body.Action, body.Description)
	p := &models.Permission{}
	if err := row.Scan(&p.ID, &p.TenantID, &p.Resource, &p.Action, &p.Description, &p.CreatedAt); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repo) ListRoles(ctx context.Context, tenantID *uuid.UUID) ([]models.RoleResponse, error) {
	pred, args := tenantPredicate("", tenantID, 1)
	rows, err := r.Pool.Query(ctx, `SELECT id, tenant_id, name, description, created_at FROM roles WHERE `+pred+` ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.RoleResponse{}
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
			return nil, err
		}
		rr, err := r.buildRoleResponse(ctx, role)
		if err != nil {
			return nil, err
		}
		out = append(out, *rr)
	}
	return out, rows.Err()
}

func (r *Repo) GetRole(ctx context.Context, tenantID *uuid.UUID, id uuid.UUID) (*models.RoleResponse, error) {
	pred, args := tenantPredicate("", tenantID, 2)
	args = append([]any{id}, args...)
	row := r.Pool.QueryRow(ctx, `SELECT id, tenant_id, name, description, created_at FROM roles WHERE id = $1 AND `+pred, args...)
	var role models.Role
	if err := row.Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return r.buildRoleResponse(ctx, role)
}

func (r *Repo) CreateRole(ctx context.Context, tenantID *uuid.UUID, body *models.CreateRoleRequest) (*models.RoleResponse, error) {
	if ok, err := r.permissionsInTenant(ctx, tenantID, body.PermissionIDs); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	var role models.Role
	if err := tx.QueryRow(ctx, `INSERT INTO roles (id, tenant_id, name, description) VALUES ($1,$2,$3,$4) RETURNING id, tenant_id, name, description, created_at`, id, tenantID, body.Name, body.Description).Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
		return nil, err
	}
	if err := replaceRolePermissionsTx(ctx, tx, id, body.PermissionIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.buildRoleResponse(ctx, role)
}

func (r *Repo) UpdateRole(ctx context.Context, tenantID *uuid.UUID, id uuid.UUID, body *models.UpdateRoleRequest) (*models.RoleResponse, error) {
	if ok, err := r.permissionsInTenant(ctx, tenantID, body.PermissionIDs); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	pred, args := tenantPredicate("", tenantID, 3)
	args = append([]any{id, body.Description}, args...)
	var role models.Role
	if err := tx.QueryRow(ctx, `UPDATE roles SET description = $2 WHERE id = $1 AND `+pred+` RETURNING id, tenant_id, name, description, created_at`, args...).Scan(&role.ID, &role.TenantID, &role.Name, &role.Description, &role.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := replaceRolePermissionsTx(ctx, tx, id, body.PermissionIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.buildRoleResponse(ctx, role)
}

func (r *Repo) DeleteRole(ctx context.Context, tenantID *uuid.UUID, id uuid.UUID) (bool, error) {
	pred, args := tenantPredicate("", tenantID, 2)
	args = append([]any{id}, args...)
	cmd, err := r.Pool.Exec(ctx, `DELETE FROM roles WHERE id = $1 AND `+pred, args...)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func replaceRolePermissionsTx(ctx context.Context, tx pgx.Tx, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	for _, pid := range permissionIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, pid); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) buildRoleResponse(ctx context.Context, role models.Role) (*models.RoleResponse, error) {
	rows, err := r.Pool.Query(ctx, `SELECT p.id, p.resource, p.action FROM permissions p INNER JOIN role_permissions rp ON rp.permission_id = p.id WHERE rp.role_id = $1 ORDER BY p.resource, p.action`, role.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resp := &models.RoleResponse{ID: role.ID, TenantID: role.TenantID, Name: role.Name, Description: role.Description, CreatedAt: role.CreatedAt, PermissionIDs: []uuid.UUID{}, Permissions: []string{}}
	for rows.Next() {
		var id uuid.UUID
		var res, act string
		if err := rows.Scan(&id, &res, &act); err != nil {
			return nil, err
		}
		resp.PermissionIDs = append(resp.PermissionIDs, id)
		resp.Permissions = append(resp.Permissions, res+":"+act)
	}
	return resp, rows.Err()
}

func (r *Repo) permissionsInTenant(ctx context.Context, tenantID *uuid.UUID, ids []uuid.UUID) (bool, error) {
	for _, id := range ids {
		pred, args := tenantPredicate("", tenantID, 2)
		args = append([]any{id}, args...)
		var ok bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM permissions WHERE id = $1 AND `+pred+`)`, args...).Scan(&ok); err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func (r *Repo) rolesInTenant(ctx context.Context, tenantID *uuid.UUID, ids []uuid.UUID) (bool, error) {
	for _, id := range ids {
		ok, err := r.roleInTenant(ctx, tenantID, id)
		if err != nil || !ok {
			return ok, err
		}
	}
	return true, nil
}

func (r *Repo) AssignRole(ctx context.Context, tenantID *uuid.UUID, userID, roleID uuid.UUID) error {
	if ok, err := r.roleInTenant(ctx, tenantID, roleID); err != nil || !ok {
		if err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	_, err := r.Pool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, roleID)
	return err
}
func (r *Repo) RemoveRole(ctx context.Context, tenantID *uuid.UUID, userID, roleID uuid.UUID) error {
	if ok, err := r.roleInTenant(ctx, tenantID, roleID); err != nil || !ok {
		if err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	_, err := r.Pool.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1 AND role_id = $2`, userID, roleID)
	return err
}
func (r *Repo) roleInTenant(ctx context.Context, tenantID *uuid.UUID, roleID uuid.UUID) (bool, error) {
	pred, args := tenantPredicate("", tenantID, 2)
	args = append([]any{roleID}, args...)
	var ok bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles WHERE id = $1 AND `+pred+`)`, args...).Scan(&ok)
	return ok, err
}

func (r *Repo) ListGroups(ctx context.Context, tenantID *uuid.UUID) ([]models.GroupResponse, error) {
	pred, args := tenantPredicate("", tenantID, 1)
	rows, err := r.Pool.Query(ctx, `SELECT id, tenant_id, name, description, created_at FROM groups WHERE `+pred+` ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.GroupResponse{}
	for rows.Next() {
		var g models.Group
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			return nil, err
		}
		gr, err := r.buildGroupResponse(ctx, g)
		if err != nil {
			return nil, err
		}
		out = append(out, *gr)
	}
	return out, rows.Err()
}
func (r *Repo) GetGroup(ctx context.Context, tenantID *uuid.UUID, id uuid.UUID) (*models.GroupResponse, error) {
	pred, args := tenantPredicate("", tenantID, 2)
	args = append([]any{id}, args...)
	row := r.Pool.QueryRow(ctx, `SELECT id, tenant_id, name, description, created_at FROM groups WHERE id = $1 AND `+pred, args...)
	var g models.Group
	if err := row.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return r.buildGroupResponse(ctx, g)
}
func (r *Repo) CreateGroup(ctx context.Context, tenantID *uuid.UUID, body *models.CreateGroupRequest) (*models.GroupResponse, error) {
	if ok, err := r.rolesInTenant(ctx, tenantID, body.RoleIDs); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	var g models.Group
	if err := tx.QueryRow(ctx, `INSERT INTO groups (id, tenant_id, name, description) VALUES ($1,$2,$3,$4) RETURNING id, tenant_id, name, description, created_at`, id, tenantID, body.Name, body.Description).Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
		return nil, err
	}
	if err := replaceGroupRolesTx(ctx, tx, id, body.RoleIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.buildGroupResponse(ctx, g)
}
func (r *Repo) UpdateGroup(ctx context.Context, tenantID *uuid.UUID, id uuid.UUID, body *models.UpdateGroupRequest) (*models.GroupResponse, error) {
	if ok, err := r.rolesInTenant(ctx, tenantID, body.RoleIDs); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	pred, args := tenantPredicate("", tenantID, 3)
	args = append([]any{id, body.Description}, args...)
	var g models.Group
	if err := tx.QueryRow(ctx, `UPDATE groups SET description = $2 WHERE id = $1 AND `+pred+` RETURNING id, tenant_id, name, description, created_at`, args...).Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := replaceGroupRolesTx(ctx, tx, id, body.RoleIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.buildGroupResponse(ctx, g)
}
func (r *Repo) DeleteGroup(ctx context.Context, tenantID *uuid.UUID, id uuid.UUID) (bool, error) {
	pred, args := tenantPredicate("", tenantID, 2)
	args = append([]any{id}, args...)
	cmd, err := r.Pool.Exec(ctx, `DELETE FROM groups WHERE id = $1 AND `+pred, args...)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}
func replaceGroupRolesTx(ctx context.Context, tx pgx.Tx, groupID uuid.UUID, roleIDs []uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM group_roles WHERE group_id = $1`, groupID); err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO group_roles (group_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, groupID, rid); err != nil {
			return err
		}
	}
	return nil
}
func (r *Repo) buildGroupResponse(ctx context.Context, g models.Group) (*models.GroupResponse, error) {
	rows, err := r.Pool.Query(ctx, `SELECT r.id, r.name FROM roles r INNER JOIN group_roles gr ON gr.role_id = r.id WHERE gr.group_id = $1 ORDER BY r.name`, g.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resp := &models.GroupResponse{ID: g.ID, TenantID: g.TenantID, Name: g.Name, Description: g.Description, CreatedAt: g.CreatedAt, RoleIDs: []uuid.UUID{}, Roles: []string{}}
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		resp.RoleIDs = append(resp.RoleIDs, id)
		resp.Roles = append(resp.Roles, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = r.Pool.QueryRow(ctx, `SELECT COUNT(*)::BIGINT FROM group_members WHERE group_id = $1`, g.ID).Scan(&resp.MemberCount)
	return resp, nil
}
func (r *Repo) AddGroupMember(ctx context.Context, tenantID *uuid.UUID, userID, groupID uuid.UUID) error {
	if ok, err := r.groupInTenant(ctx, tenantID, groupID); err != nil || !ok {
		if err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	_, err := r.Pool.Exec(ctx, `INSERT INTO group_members (group_id, user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, groupID, userID)
	return err
}
func (r *Repo) RemoveGroupMember(ctx context.Context, tenantID *uuid.UUID, userID, groupID uuid.UUID) error {
	if ok, err := r.groupInTenant(ctx, tenantID, groupID); err != nil || !ok {
		if err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	_, err := r.Pool.Exec(ctx, `DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	return err
}
func (r *Repo) groupInTenant(ctx context.Context, tenantID *uuid.UUID, groupID uuid.UUID) (bool, error) {
	pred, args := tenantPredicate("", tenantID, 2)
	args = append([]any{groupID}, args...)
	var ok bool
	err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1 AND `+pred+`)`, args...).Scan(&ok)
	return ok, err
}
