package models

import (
	"time"

	"github.com/google/uuid"
)

// Role mirrors `models::role::Role` in Rust.
type Role struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateRoleRequest / UpdateRoleRequest — both name + description.
type CreateRoleRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type UpdateRoleRequest = CreateRoleRequest

// Permission mirrors `models::permission::Permission`.
type Permission struct {
	ID        uuid.UUID `json:"id"`
	Resource  string    `json:"resource"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"created_at"`
}

// CreatePermissionRequest is `{resource, action}`.
type CreatePermissionRequest struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// Group mirrors `models::group::Group`.
type Group struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateGroupRequest / UpdateGroupRequest match the Rust shape.
type CreateGroupRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type UpdateGroupRequest = CreateGroupRequest

// APIKey mirrors `models::api_key::ApiKey`. The plaintext token is
// never persisted; `key_hash` is the SHA-256 of it.
type APIKey struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// CreateAPIKeyRequest / Response. Response includes the plaintext
// token ONCE — clients must persist it.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreateAPIKeyResponse struct {
	APIKey APIKey `json:"api_key"`
	Token  string `json:"token"` // plaintext, returned ONCE
}

// UpdateUserRequest is the body of PATCH /users/{id}.
//
// Optional fields preserve current values when nil.
type UpdateUserRequest struct {
	Name        *string `json:"name,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
	MFAEnforced *bool   `json:"mfa_enforced,omitempty"`
}
