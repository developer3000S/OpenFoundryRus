// Package models holds the wire-format types for application-composition-service.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PrimaryItem mirrors `composition_views`.
type PrimaryItem struct {
	ID        uuid.UUID       `json:"id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type CreatePrimaryRequest struct {
	Payload json.RawMessage `json:"payload"`
}

// SecondaryItem mirrors `composition_bindings`.
type SecondaryItem struct {
	ID        uuid.UUID       `json:"id"`
	ParentID  uuid.UUID       `json:"parent_id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type CreateSecondaryRequest struct {
	Payload json.RawMessage `json:"payload"`
}

// App mirrors the `apps` table created by the foundation migration.
// Shape matches the AppDefinition the web frontend (apps/web/src/lib/api/apps.ts)
// already consumes.
type App struct {
	ID                 uuid.UUID       `json:"id"`
	Name               string          `json:"name"`
	Slug               string          `json:"slug"`
	Description        string          `json:"description"`
	Status             string          `json:"status"`
	Pages              json.RawMessage `json:"pages"`
	Theme              json.RawMessage `json:"theme"`
	Settings           json.RawMessage `json:"settings"`
	TemplateKey        *string         `json:"template_key"`
	CreatedBy          *uuid.UUID      `json:"created_by"`
	PublishedVersionID *uuid.UUID      `json:"published_version_id"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type CreateAppRequest struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug,omitempty"`
	Description string          `json:"description,omitempty"`
	Status      string          `json:"status,omitempty"`
	Pages       json.RawMessage `json:"pages,omitempty"`
	Theme       json.RawMessage `json:"theme,omitempty"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	TemplateKey *string         `json:"template_key,omitempty"`
}

type UpdateAppRequest struct {
	Name        *string         `json:"name,omitempty"`
	Slug        *string         `json:"slug,omitempty"`
	Description *string         `json:"description,omitempty"`
	Status      *string         `json:"status,omitempty"`
	Pages       json.RawMessage `json:"pages,omitempty"`
	Theme       json.RawMessage `json:"theme,omitempty"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	TemplateKey *string         `json:"template_key,omitempty"`
}

// AppSummary is what listApps() returns; same shape the frontend AppListResponse expects.
type AppSummary struct {
	ID                 uuid.UUID  `json:"id"`
	Name               string     `json:"name"`
	Slug               string     `json:"slug"`
	Description        string     `json:"description"`
	Status             string     `json:"status"`
	PageCount          int        `json:"page_count"`
	WidgetCount        int        `json:"widget_count"`
	TemplateKey        *string    `json:"template_key"`
	PublishedVersionID *uuid.UUID `json:"published_version_id"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// AppVersion mirrors `app_versions`.
type AppVersion struct {
	ID            uuid.UUID       `json:"id"`
	AppID         uuid.UUID       `json:"app_id"`
	VersionNumber int             `json:"version_number"`
	Status        string          `json:"status"`
	AppSnapshot   json.RawMessage `json:"app_snapshot"`
	Notes         string          `json:"notes"`
	CreatedBy     *uuid.UUID      `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
	PublishedAt   *time.Time      `json:"published_at"`
}

type PublishAppRequest struct {
	Notes string `json:"notes"`
}

type AppTemplate struct {
	ID              uuid.UUID       `json:"id"`
	Key             string          `json:"key"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Category        string          `json:"category"`
	PreviewImageURL *string         `json:"preview_image_url"`
	Definition      json.RawMessage `json:"definition"`
	CreatedAt       time.Time       `json:"created_at"`
}

type WidgetDefaultSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type WidgetCatalogVariable struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type WidgetCatalogEvent struct {
	Name          string          `json:"name"`
	Label         string          `json:"label"`
	Description   string          `json:"description"`
	ActionKinds   []string        `json:"action_kinds"`
	PayloadSchema json.RawMessage `json:"payload_schema,omitempty"`
}

type WidgetCatalogDisplay struct {
	Icon            string   `json:"icon"`
	Accent          string   `json:"accent"`
	Tags            []string `json:"tags"`
	SortOrder       int      `json:"sort_order"`
	PreferredChrome string   `json:"preferred_chrome"`
}

type WidgetCatalogItem struct {
	CatalogVersion    string                  `json:"catalog_version"`
	SchemaVersion     string                  `json:"schema_version"`
	WidgetType        string                  `json:"widget_type"`
	WidgetKind        string                  `json:"widget_kind"`
	Label             string                  `json:"label"`
	Description       string                  `json:"description"`
	Category          string                  `json:"category"`
	ConfigSchema      json.RawMessage         `json:"config_schema"`
	InputVariables    []WidgetCatalogVariable `json:"input_variables"`
	OutputVariables   []WidgetCatalogVariable `json:"output_variables"`
	Events            []WidgetCatalogEvent    `json:"events"`
	Permissions       []string                `json:"permissions"`
	Display           WidgetCatalogDisplay    `json:"display"`
	DefaultProps      json.RawMessage         `json:"default_props"`
	DefaultSize       WidgetDefaultSize       `json:"default_size"`
	SupportedBindings []string                `json:"supported_bindings"`
	SupportsChildren  bool                    `json:"supports_children"`
}

type SlatePackageFile struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

type SlatePackageResponse struct {
	AppID       uuid.UUID          `json:"app_id"`
	AppSlug     string             `json:"app_slug"`
	Framework   string             `json:"framework"`
	PackageName string             `json:"package_name"`
	EntryFile   string             `json:"entry_file"`
	SDKImport   string             `json:"sdk_import"`
	Files       []SlatePackageFile `json:"files"`
}

type ImportSlatePackageRequest struct {
	PackageName    *string            `json:"package_name,omitempty"`
	EntryFile      *string            `json:"entry_file,omitempty"`
	SDKImport      *string            `json:"sdk_import,omitempty"`
	Framework      *string            `json:"framework,omitempty"`
	RepositoryID   *string            `json:"repository_id,omitempty"`
	Layout         *string            `json:"layout,omitempty"`
	Runtime        *string            `json:"runtime,omitempty"`
	DevCommand     *string            `json:"dev_command,omitempty"`
	PreviewCommand *string            `json:"preview_command,omitempty"`
	Files          []SlatePackageFile `json:"files"`
}

type SlateRoundTripResponse struct {
	App          *App                 `json:"app"`
	SlatePackage SlatePackageResponse `json:"slate_package"`
}
