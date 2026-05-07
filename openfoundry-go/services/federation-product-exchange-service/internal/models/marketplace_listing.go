package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PackageType is the marketplace listing kind discriminator. Values mirror
// Rust marketplace::models::package::PackageType and the package_kind column.
type PackageType string

const (
	PackageTypeConnector   PackageType = "connector"
	PackageTypeTransform   PackageType = "transform"
	PackageTypeWidget      PackageType = "widget"
	PackageTypeAppTemplate PackageType = "app_template"
	PackageTypeMLModel     PackageType = "ml_model"
	PackageTypeAIAgent     PackageType = "ai_agent"
	PackageTypeMediaSet    PackageType = "media_set"
)

func (p PackageType) Valid() bool {
	switch p {
	case PackageTypeConnector, PackageTypeTransform, PackageTypeWidget, PackageTypeAppTemplate, PackageTypeMLModel, PackageTypeAIAgent, PackageTypeMediaSet:
		return true
	default:
		return false
	}
}

// ListingDefinition is the public marketplace listing JSON shape.
type ListingDefinition struct {
	ID             uuid.UUID   `json:"id"`
	Name           string      `json:"name"`
	Slug           string      `json:"slug"`
	Summary        string      `json:"summary"`
	Description    string      `json:"description"`
	Publisher      string      `json:"publisher"`
	CategorySlug   string      `json:"category_slug"`
	PackageKind    PackageType `json:"package_kind"`
	RepositorySlug string      `json:"repository_slug"`
	Visibility     string      `json:"visibility"`
	Tags           []string    `json:"tags"`
	Capabilities   []string    `json:"capabilities"`
	InstallCount   int64       `json:"install_count"`
	AverageRating  float64     `json:"average_rating"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type CreateListingRequest struct {
	Name           string      `json:"name"`
	Slug           string      `json:"slug"`
	Summary        string      `json:"summary"`
	Description    string      `json:"description"`
	Publisher      string      `json:"publisher"`
	CategorySlug   string      `json:"category_slug"`
	PackageKind    PackageType `json:"package_kind"`
	RepositorySlug string      `json:"repository_slug"`
	Visibility     string      `json:"visibility"`
	Tags           []string    `json:"tags"`
	Capabilities   []string    `json:"capabilities"`
}

type UpdateListingRequest struct {
	Name           *string   `json:"name"`
	Summary        *string   `json:"summary"`
	Description    *string   `json:"description"`
	CategorySlug   *string   `json:"category_slug"`
	RepositorySlug *string   `json:"repository_slug"`
	Visibility     *string   `json:"visibility"`
	Tags           *[]string `json:"tags"`
	Capabilities   *[]string `json:"capabilities"`
}

// PackageVersion mirrors the package version payload returned in listing detail
// and by publish-version.
type PackageVersion struct {
	ID                uuid.UUID       `json:"id"`
	ListingID         uuid.UUID       `json:"listing_id"`
	Version           string          `json:"version"`
	ReleaseChannel    string          `json:"release_channel"`
	Changelog         string          `json:"changelog"`
	DependencyMode    string          `json:"dependency_mode"`
	Dependencies      json.RawMessage `json:"dependencies"`
	PackagedResources json.RawMessage `json:"packaged_resources"`
	Manifest          json.RawMessage `json:"manifest"`
	PublishedAt       time.Time       `json:"published_at"`
}

type PublishVersionRequest struct {
	Version           string          `json:"version"`
	ReleaseChannel    string          `json:"release_channel"`
	Changelog         string          `json:"changelog"`
	DependencyMode    string          `json:"dependency_mode"`
	Dependencies      json.RawMessage `json:"dependencies"`
	PackagedResources json.RawMessage `json:"packaged_resources"`
	Manifest          json.RawMessage `json:"manifest"`
}

type ListingDetail struct {
	Listing       ListingDefinition `json:"listing"`
	LatestVersion *PackageVersion   `json:"latest_version"`
	Versions      []PackageVersion  `json:"versions"`
	Reviews       []ListingReview   `json:"reviews"`
}

type ListingReview struct {
	ID          uuid.UUID `json:"id"`
	ListingID   uuid.UUID `json:"listing_id"`
	Author      string    `json:"author"`
	Rating      int       `json:"rating"`
	Headline    string    `json:"headline"`
	Body        string    `json:"body"`
	Recommended bool      `json:"recommended"`
	CreatedAt   time.Time `json:"created_at"`
}

type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type PaginatedListResponse[T any] struct {
	Items      []T        `json:"items"`
	Pagination Pagination `json:"pagination"`
}
