// Package models holds wire types for tenancy-organizations-service.
//
// Foundation slice: Organization + Enrollment, mirroring the Rust
// `models/organization.rs` + `models/enrollment.rs`.
//
// Spaces / projects / sharing / trash / favorites land in follow-up
// slices (see docs/archive/INVENTORY-tenancy-organizations-service.md).
package models

import (
	"time"

	"github.com/google/uuid"
)

// Organization mirrors `models::organization::Organization`.
//
// SG.2 (2026-05-14) added Description, ContactEmail, Metadata, Settings
// and Quotas. The five new fields default to their zero values when a
// row predates migration 0008 — they are always present in the wire
// payload so SDK/frontend code can rely on a stable shape.
type Organization struct {
	ID               uuid.UUID      `json:"id"`
	Slug             string         `json:"slug"`
	DisplayName      string         `json:"display_name"`
	Description      string         `json:"description"`
	ContactEmail     *string        `json:"contact_email"`
	OrganizationType string         `json:"organization_type"`
	DefaultWorkspace *string        `json:"default_workspace"`
	TenantTier       *string        `json:"tenant_tier"`
	Status           string         `json:"status"`
	Metadata         map[string]any `json:"metadata"`
	Settings         map[string]any `json:"settings"`
	Quotas           map[string]any `json:"quotas"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// CreateOrganizationRequest is the body of POST /organizations.
type CreateOrganizationRequest struct {
	Slug             string          `json:"slug"`
	DisplayName      string          `json:"display_name"`
	Description      *string         `json:"description,omitempty"`
	ContactEmail     *string         `json:"contact_email,omitempty"`
	OrganizationType *string         `json:"organization_type,omitempty"`
	DefaultWorkspace *string         `json:"default_workspace,omitempty"`
	TenantTier       *string         `json:"tenant_tier,omitempty"`
	Status           *string         `json:"status,omitempty"`
	Metadata         map[string]any  `json:"metadata,omitempty"`
	Settings         map[string]any  `json:"settings,omitempty"`
	Quotas           map[string]any  `json:"quotas,omitempty"`
}

// UpdateOrganizationRequest mirrors the Rust update request — every
// field optional, missing fields preserve current values.
type UpdateOrganizationRequest struct {
	DisplayName      *string         `json:"display_name,omitempty"`
	Description      *string         `json:"description,omitempty"`
	ContactEmail     *string         `json:"contact_email,omitempty"`
	OrganizationType *string         `json:"organization_type,omitempty"`
	DefaultWorkspace *string         `json:"default_workspace,omitempty"`
	TenantTier       *string         `json:"tenant_tier,omitempty"`
	Status           *string         `json:"status,omitempty"`
	Metadata         *map[string]any `json:"metadata,omitempty"`
	Settings         *map[string]any `json:"settings,omitempty"`
	Quotas           *map[string]any `json:"quotas,omitempty"`
}

// OrganizationAdmin describes one row in tenancy_organization_admins.
//
// SG.2: organizations expose an explicit administrators set, so org
// administration permissions are auditable and removable without
// touching the IdP-side group graph.
type OrganizationAdmin struct {
	OrganizationID uuid.UUID  `json:"organization_id"`
	UserID         uuid.UUID  `json:"user_id"`
	Scope          string     `json:"scope"`
	GrantedBy      *uuid.UUID `json:"granted_by"`
	CreatedAt      time.Time  `json:"created_at"`
}

// CreateOrganizationAdminRequest is the body of POST
// /organizations/{id}/admins.
type CreateOrganizationAdminRequest struct {
	UserID    uuid.UUID  `json:"user_id"`
	Scope     *string    `json:"scope,omitempty"`
	GrantedBy *uuid.UUID `json:"granted_by,omitempty"`
}

// OrganizationGuest describes one row in tenancy_organization_guests.
//
// SG.2: cross-organization collaboration requires a first-class guest
// record so the membership-enforcement check can distinguish a primary
// member from a guest visitor and apply the right visibility rules.
type OrganizationGuest struct {
	OrganizationID        uuid.UUID  `json:"organization_id"`
	UserID                uuid.UUID  `json:"user_id"`
	PrimaryOrganizationID uuid.UUID  `json:"primary_organization_id"`
	Status                string     `json:"status"`
	InvitedBy             *uuid.UUID `json:"invited_by"`
	ExpiresAt             *time.Time `json:"expires_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// CreateOrganizationGuestRequest is the body of POST
// /organizations/{id}/guests.
type CreateOrganizationGuestRequest struct {
	UserID                uuid.UUID  `json:"user_id"`
	PrimaryOrganizationID uuid.UUID  `json:"primary_organization_id"`
	Status                *string    `json:"status,omitempty"`
	InvitedBy             *uuid.UUID `json:"invited_by,omitempty"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
}

// TenancySpace is the Foundry-style "space": a store / administration
// boundary inside an organization. Distinct from `nexus_spaces`, which
// is a federation peer namespace.
type TenancySpace struct {
	ID             uuid.UUID      `json:"id"`
	OrganizationID uuid.UUID      `json:"organization_id"`
	Slug           string         `json:"slug"`
	DisplayName    string         `json:"display_name"`
	Description    string         `json:"description"`
	Settings       map[string]any `json:"settings"`
	Quotas         map[string]any `json:"quotas"`
	Status         string         `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// CreateTenancySpaceRequest is the body of POST
// /organizations/{id}/spaces.
type CreateTenancySpaceRequest struct {
	Slug        string         `json:"slug"`
	DisplayName string         `json:"display_name"`
	Description *string        `json:"description,omitempty"`
	Settings    map[string]any `json:"settings,omitempty"`
	Quotas      map[string]any `json:"quotas,omitempty"`
	Status      *string        `json:"status,omitempty"`
}

// UpdateTenancySpaceRequest is the body of PATCH /tenancy-spaces/{id}.
type UpdateTenancySpaceRequest struct {
	DisplayName *string         `json:"display_name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Settings    *map[string]any `json:"settings,omitempty"`
	Quotas      *map[string]any `json:"quotas,omitempty"`
	Status      *string         `json:"status,omitempty"`
}

// Enrollment mirrors `models::enrollment::Enrollment`.
type Enrollment struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	WorkspaceSlug  *string   `json:"workspace_slug"`
	RoleSlug       string    `json:"role_slug"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// CreateEnrollmentRequest is the body of POST /enrollments.
type CreateEnrollmentRequest struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	WorkspaceSlug  *string   `json:"workspace_slug,omitempty"`
	RoleSlug       string    `json:"role_slug"`
	Status         *string   `json:"status,omitempty"`
}

// ListResponse is the canonical envelope.
type ListResponse[T any] struct {
	Items []T `json:"items"`
}
