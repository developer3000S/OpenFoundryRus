package models

import (
	"encoding/json"
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
// Optional fields preserve current values when nil. SG.4 added
// Username, Realm, OrganizationID, and Attributes to the patch
// surface so admins can re-home a user to a different org / realm
// without dropping and re-creating the row.
type UpdateUserRequest struct {
	Name           *string          `json:"name,omitempty"`
	Username       *string          `json:"username,omitempty"`
	Realm          *string          `json:"realm,omitempty"`
	IsActive       *bool            `json:"is_active,omitempty"`
	MFAEnforced    *bool            `json:"mfa_enforced,omitempty"`
	OrganizationID **uuid.UUID      `json:"organization_id,omitempty"`
	Attributes     *json.RawMessage `json:"attributes,omitempty"`
}

// PreregisterUserRequest is the body of POST /users/preregister.
//
// SG.4: admins can seed a row before the user signs up. The row
// carries an empty password hash and `preregistered = true`; when
// the user completes registration or signs in through SSO the
// existing user-resolution policy promotes the row to active.
type PreregisterUserRequest struct {
	Email          string          `json:"email"`
	Username       *string         `json:"username,omitempty"`
	Name           string          `json:"name"`
	Realm          *string         `json:"realm,omitempty"`
	OrganizationID *uuid.UUID      `json:"organization_id,omitempty"`
	Attributes     json.RawMessage `json:"attributes,omitempty"`
	Roles          []string        `json:"roles,omitempty"`
	Groups         []uuid.UUID     `json:"groups,omitempty"`
}

// ListUsersFilter is the query-string projection of GET /users.
//
// SG.4: search by email / username (case-insensitive substring),
// filter by organization, realm, and active/inactive/deleted state.
// IncludeDeleted defaults to false so admin listings hide soft-
// deleted users; pass `?include_deleted=true` to show them.
type ListUsersFilter struct {
	Query           string
	OrganizationID  *uuid.UUID
	Realm           string
	Status          string // "" | "active" | "inactive"
	IncludeDeleted  bool
	Limit           int
	Offset          int
}

// ListUsersResponse is the wire shape of GET /users. SG.4 swapped
// the bare-array response for an envelope so a future change can
// add `total` / `next_cursor` without re-breaking SDKs.
type ListUsersResponse struct {
	Items []User `json:"items"`
	Total int64  `json:"total"`
}

// UserInspection is the response of GET /users/{id}/inspect.
//
// SG.4: an "all the things" view for the admin UI — user core + role
// names + group names + a token-summary count and the most recent
// session. Cross-service guest memberships are populated by the UI
// (it lives in tenancy-organizations-service).
type UserInspection struct {
	User              User              `json:"user"`
	Roles             []string          `json:"roles"`
	Groups            []GroupBrief      `json:"groups"`
	Tokens            TokenSummary      `json:"tokens"`
	ExternalIdentities []ExternalBinding `json:"external_identities"`
}

// GroupBrief is the projection used inside UserInspection.
type GroupBrief struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// TokenSummary aggregates the refresh-token state for one user.
type TokenSummary struct {
	ActiveCount   int        `json:"active_count"`
	RevokedCount  int        `json:"revoked_count"`
	NextExpiresAt *time.Time `json:"next_expires_at,omitempty"`
	APIKeysActive int        `json:"api_keys_active"`
}

// ExternalBinding mirrors `repo.ExternalIdentity` — the IdP→user
// link rows from user_external_identities.
type ExternalBinding struct {
	Provider    string     `json:"provider"`
	ExternalID  string     `json:"external_id"`
	Email       string     `json:"email"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
