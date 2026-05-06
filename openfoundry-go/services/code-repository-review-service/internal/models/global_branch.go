// Package models holds the wire-format types for code-repository-review-service.
package models

import (
	"time"

	"github.com/google/uuid"
)

// GlobalBranch mirrors the `global_branches` row.
type GlobalBranch struct {
	ID                 uuid.UUID  `json:"id"`
	RID                string     `json:"rid"`
	Name               string     `json:"name"`
	ParentGlobalBranch *uuid.UUID `json:"parent_global_branch"`
	Description        string     `json:"description"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	ArchivedAt         *time.Time `json:"archived_at"`
}

// GlobalBranchLink mirrors `global_branch_resource_links`.
type GlobalBranchLink struct {
	GlobalBranchID uuid.UUID `json:"global_branch_id"`
	ResourceType   string    `json:"resource_type"`
	ResourceRID    string    `json:"resource_rid"`
	BranchRID      string    `json:"branch_rid"`
	Status         string    `json:"status"`
	LastSyncedAt   time.Time `json:"last_synced_at"`
}

// CreateGlobalBranchRequest is the POST /v1/global-branches body.
type CreateGlobalBranchRequest struct {
	Name               string     `json:"name"`
	Description        *string    `json:"description,omitempty"`
	ParentGlobalBranch *uuid.UUID `json:"parent_global_branch,omitempty"`
}

// CreateGlobalBranchLinkRequest is the POST .../links body.
type CreateGlobalBranchLinkRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceRID  string `json:"resource_rid"`
	BranchRID    string `json:"branch_rid"`
}

// GlobalBranchSummary is GET /v1/global-branches/{id} — branch + counters.
//
// Rust uses `#[serde(flatten)]` to inline `branch` so the JSON has the
// branch fields and the counters at the same level. We replicate that
// by repeating the fields here. NOTE: keep this in sync with GlobalBranch.
type GlobalBranchSummary struct {
	ID                 uuid.UUID  `json:"id"`
	RID                string     `json:"rid"`
	Name               string     `json:"name"`
	ParentGlobalBranch *uuid.UUID `json:"parent_global_branch"`
	Description        string     `json:"description"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	ArchivedAt         *time.Time `json:"archived_at"`
	LinkCount          int64      `json:"link_count"`
	DriftedCount       int64      `json:"drifted_count"`
	ArchivedCount      int64      `json:"archived_count"`
}

// PromoteResponse is POST /v1/global-branches/{id}/promote.
type PromoteResponse struct {
	EventID        uuid.UUID `json:"event_id"`
	GlobalBranchID uuid.UUID `json:"global_branch_id"`
	Topic          string    `json:"topic"`
}

// SummaryFromBranch builds a summary from a branch + counts.
func SummaryFromBranch(b GlobalBranch, link, drifted, archived int64) GlobalBranchSummary {
	return GlobalBranchSummary{
		ID:                 b.ID,
		RID:                b.RID,
		Name:               b.Name,
		ParentGlobalBranch: b.ParentGlobalBranch,
		Description:        b.Description,
		CreatedBy:          b.CreatedBy,
		CreatedAt:          b.CreatedAt,
		ArchivedAt:         b.ArchivedAt,
		LinkCount:          link,
		DriftedCount:       drifted,
		ArchivedCount:      archived,
	}
}
