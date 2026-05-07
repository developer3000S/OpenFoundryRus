package models

import (
	"time"

	"github.com/google/uuid"
)

type Permission struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
	Resource    string     `json:"resource"`
	Action      string     `json:"action"`
	Description *string    `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Role struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Group struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    *uuid.UUID `json:"tenant_id,omitempty"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
}

type RoleResponse struct {
	ID            uuid.UUID   `json:"id"`
	TenantID      *uuid.UUID  `json:"tenant_id,omitempty"`
	Name          string      `json:"name"`
	Description   *string     `json:"description"`
	CreatedAt     time.Time   `json:"created_at"`
	PermissionIDs []uuid.UUID `json:"permission_ids"`
	Permissions   []string    `json:"permissions"`
}

type GroupResponse struct {
	ID          uuid.UUID   `json:"id"`
	TenantID    *uuid.UUID  `json:"tenant_id,omitempty"`
	Name        string      `json:"name"`
	Description *string     `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	MemberCount int64       `json:"member_count"`
	RoleIDs     []uuid.UUID `json:"role_ids"`
	Roles       []string    `json:"roles"`
}

type CreatePermissionRequest struct {
	Resource    string  `json:"resource"`
	Action      string  `json:"action"`
	Description *string `json:"description,omitempty"`
}

type CreateRoleRequest struct {
	Name          string      `json:"name"`
	Description   *string     `json:"description,omitempty"`
	PermissionIDs []uuid.UUID `json:"permission_ids,omitempty"`
}

type UpdateRoleRequest struct {
	Description   *string     `json:"description,omitempty"`
	PermissionIDs []uuid.UUID `json:"permission_ids,omitempty"`
}

type CreateGroupRequest struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	RoleIDs     []uuid.UUID `json:"role_ids,omitempty"`
}

type UpdateGroupRequest struct {
	Description *string     `json:"description,omitempty"`
	RoleIDs     []uuid.UUID `json:"role_ids,omitempty"`
}

type AssignRoleRequest struct {
	RoleID uuid.UUID `json:"role_id"`
}

type UserGroupRequest struct {
	GroupID uuid.UUID `json:"group_id"`
}
