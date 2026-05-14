// Package models holds wire types for identity-federation-service:
// `User` plus request/response types. Future slices add session,
// role, group, permission, policy etc.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// User mirrors `models::user::User`. SG.4 (2026-05-14) extended the
// shape with Username, Realm, LastLoginAt, LastLoginIP, DeletedAt,
// Preregistered and InvitedBy. Existing rows backfill Username from
// the email localpart and Realm from AuthSource via migration 0011.
type User struct {
	ID             uuid.UUID       `json:"id"`
	Email          string          `json:"email"`
	Username       *string         `json:"username,omitempty"`
	Name           string          `json:"name"`
	PasswordHash   string          `json:"-"` // never serialised
	IsActive       bool            `json:"is_active"`
	AuthSource     string          `json:"auth_source"`
	Realm          string          `json:"realm"`
	MFAEnforced    bool            `json:"mfa_enforced"`
	OrganizationID *uuid.UUID      `json:"organization_id,omitempty"`
	Attributes     json.RawMessage `json:"attributes,omitempty"`
	LastLoginAt    *time.Time      `json:"last_login_at,omitempty"`
	LastLoginIP    *string         `json:"last_login_ip,omitempty"`
	Preregistered  bool            `json:"preregistered"`
	InvitedBy      *uuid.UUID      `json:"invited_by,omitempty"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// RegisterRequest mirrors `handlers::register::RegisterRequest`.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// RegisterResponse mirrors `handlers::register::RegisterResponse`.
type RegisterResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

// BootstrapStatusResponse mirrors `handlers::register::BootstrapStatusResponse`.
type BootstrapStatusResponse struct {
	RequiresInitialAdmin bool `json:"requires_initial_admin"`
}

// LoginRequest mirrors `handlers::login::LoginRequest`.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginStatus is the discriminator on the LoginResponse wire envelope.
//
// The Rust enum uses `#[serde(tag = "status", rename_all = "snake_case")]`
// so the wire shape is `{"status":"authenticated", ...}` /
// `{"status":"mfa_required", ...}`.
type LoginStatus string

const (
	LoginStatusAuthenticated LoginStatus = "authenticated"
	LoginStatusMFARequired   LoginStatus = "mfa_required"
)

// LoginResponse is the discriminated union of login outcomes.
//
// Slice 1 only emits `authenticated` (MFA arrives in slice 3). The
// MFA fields stay in the type so the wire shape stays stable when the
// slice 3 path lights up.
type LoginResponse struct {
	Status          LoginStatus `json:"status"`
	AccessToken     string      `json:"access_token,omitempty"`
	RefreshToken    string      `json:"refresh_token,omitempty"`
	TokenType       string      `json:"token_type,omitempty"`
	ExpiresIn       int64       `json:"expires_in,omitempty"`
	ChallengeToken  string      `json:"challenge_token,omitempty"`
	Methods         []string    `json:"methods,omitempty"`
}

// RefreshRequest mirrors `handlers::token::RefreshRequest`.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// TokenResponse mirrors `handlers::login::TokenResponse`.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// RefreshTokenRow is the persisted row in `refresh_tokens`.
type RefreshTokenRow struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	FamilyID   uuid.UUID
	IssuedAt   time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}
