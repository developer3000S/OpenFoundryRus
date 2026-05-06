// Package models holds wire types for tenancy-organizations-service.
//
// Foundation slice: Organization + Enrollment, mirroring the Rust
// `models/organization.rs` + `models/enrollment.rs`.
//
// Spaces / projects / sharing / trash / favorites land in follow-up
// slices (see INVENTORY-tenancy-organizations-service.md).
package models

import (
	"time"

	"github.com/google/uuid"
)

// Organization mirrors `models::organization::Organization`.
type Organization struct {
	ID                uuid.UUID `json:"id"`
	Slug              string    `json:"slug"`
	DisplayName       string    `json:"display_name"`
	OrganizationType  string    `json:"organization_type"`
	DefaultWorkspace  *string   `json:"default_workspace"`
	TenantTier        *string   `json:"tenant_tier"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CreateOrganizationRequest is the body of POST /organizations.
type CreateOrganizationRequest struct {
	Slug             string  `json:"slug"`
	DisplayName      string  `json:"display_name"`
	OrganizationType *string `json:"organization_type,omitempty"`
	DefaultWorkspace *string `json:"default_workspace,omitempty"`
	TenantTier       *string `json:"tenant_tier,omitempty"`
	Status           *string `json:"status,omitempty"`
}

// UpdateOrganizationRequest mirrors the Rust update request — every
// field optional, missing fields preserve current values.
type UpdateOrganizationRequest struct {
	DisplayName      *string `json:"display_name,omitempty"`
	OrganizationType *string `json:"organization_type,omitempty"`
	DefaultWorkspace *string `json:"default_workspace,omitempty"`
	TenantTier       *string `json:"tenant_tier,omitempty"`
	Status           *string `json:"status,omitempty"`
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
