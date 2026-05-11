// Package models holds wire types for ontology-definition-service.
//
// Foundation slice scope: object_types only. Properties, link_types,
// action_types, interfaces, shared property types, ontology_projects
// (~600 LOC of consolidated DDL) all land in follow-up slices once
// the Rust kernel handlers themselves migrate.
package models

import (
	"time"

	"github.com/google/uuid"
)

type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// ObjectType mirrors `ontology_schema.object_types` rows.
type ObjectType struct {
	ID                 uuid.UUID  `json:"id"`
	Name               string     `json:"name"`
	DisplayName        string     `json:"display_name"`
	PluralDisplayName  *string    `json:"plural_display_name,omitempty"`
	Description        string     `json:"description"`
	PrimaryKeyProperty *string    `json:"primary_key_property"`
	Icon               *string    `json:"icon"`
	Color              *string    `json:"color"`
	Editable           bool       `json:"editable"`
	BackingDatasetID   *uuid.UUID `json:"backing_dataset_id,omitempty"`
	BackingDatasetRID  *string    `json:"backing_dataset_rid,omitempty"`
	PipelineRID        *string    `json:"pipeline_rid,omitempty"`
	ManagedBy          *string    `json:"managed_by,omitempty"`
	OwnerID            uuid.UUID  `json:"owner_id"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type CreateObjectTypeRequest struct {
	ID                 *uuid.UUID `json:"id,omitempty"`
	Name               string     `json:"name"`
	DisplayName        string     `json:"display_name"`
	PluralDisplayName  *string    `json:"plural_display_name,omitempty"`
	Description        string     `json:"description,omitempty"`
	PrimaryKeyProperty *string    `json:"primary_key_property,omitempty"`
	Icon               *string    `json:"icon,omitempty"`
	Color              *string    `json:"color,omitempty"`
	Editable           *bool      `json:"editable,omitempty"`
	BackingDatasetID   *uuid.UUID `json:"backing_dataset_id,omitempty"`
	BackingDatasetRID  *string    `json:"backing_dataset_rid,omitempty"`
	PipelineRID        *string    `json:"pipeline_rid,omitempty"`
	ManagedBy          *string    `json:"managed_by,omitempty"`
}

type UpdateObjectTypeRequest struct {
	DisplayName        *string    `json:"display_name,omitempty"`
	PluralDisplayName  *string    `json:"plural_display_name,omitempty"`
	Description        *string    `json:"description,omitempty"`
	PrimaryKeyProperty *string    `json:"primary_key_property,omitempty"`
	Icon               *string    `json:"icon,omitempty"`
	Color              *string    `json:"color,omitempty"`
	Editable           *bool      `json:"editable,omitempty"`
	BackingDatasetID   *uuid.UUID `json:"backing_dataset_id,omitempty"`
	BackingDatasetRID  *string    `json:"backing_dataset_rid,omitempty"`
	PipelineRID        *string    `json:"pipeline_rid,omitempty"`
	ManagedBy          *string    `json:"managed_by,omitempty"`
}

// Property mirrors `ontology_schema.properties` rows.
type Property struct {
	ID               uuid.UUID `json:"id"`
	ObjectTypeID     uuid.UUID `json:"object_type_id"`
	Name             string    `json:"name"`
	DisplayName      string    `json:"display_name"`
	Description      string    `json:"description"`
	PropertyType     string    `json:"property_type"`
	Required         bool      `json:"required"`
	UniqueConstraint bool      `json:"unique_constraint"`
	TimeDependent    bool      `json:"time_dependent"`
	DefaultValue     any       `json:"default_value"`
	ValidationRules  any       `json:"validation_rules"`
	InlineEditConfig any       `json:"inline_edit_config"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CreatePropertyRequest struct {
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	Description      string `json:"description,omitempty"`
	PropertyType     string `json:"property_type"`
	Required         bool   `json:"required,omitempty"`
	UniqueConstraint bool   `json:"unique_constraint,omitempty"`
	TimeDependent    bool   `json:"time_dependent,omitempty"`
	DefaultValue     any    `json:"default_value,omitempty"`
	ValidationRules  any    `json:"validation_rules,omitempty"`
	InlineEditConfig any    `json:"inline_edit_config,omitempty"`
}

// LinkType mirrors `ontology_schema.link_types` rows.
type LinkType struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"display_name"`
	Description  string    `json:"description"`
	SourceTypeID uuid.UUID `json:"source_type_id"`
	TargetTypeID uuid.UUID `json:"target_type_id"`
	Cardinality  string    `json:"cardinality"`
	OwnerID      uuid.UUID `json:"owner_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateLinkTypeRequest struct {
	ID           *uuid.UUID `json:"id,omitempty"`
	Name         string     `json:"name"`
	DisplayName  string     `json:"display_name"`
	Description  string     `json:"description,omitempty"`
	SourceTypeID uuid.UUID  `json:"source_type_id"`
	TargetTypeID uuid.UUID  `json:"target_type_id"`
	Cardinality  string     `json:"cardinality,omitempty"`
}

type UpdateLinkTypeRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
	Cardinality *string `json:"cardinality,omitempty"`
}
