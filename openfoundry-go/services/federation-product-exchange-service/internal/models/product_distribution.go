package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PeerOrganization is the product-distribution peer registry record.
type PeerOrganization struct {
	ID                   uuid.UUID  `json:"id"`
	Slug                 string     `json:"slug"`
	DisplayName          string     `json:"display_name"`
	OrganizationType     string     `json:"organization_type"`
	Region               string     `json:"region"`
	EndpointURL          string     `json:"endpoint_url"`
	AuthMode             string     `json:"auth_mode"`
	TrustLevel           string     `json:"trust_level"`
	PublicKeyFingerprint string     `json:"public_key_fingerprint"`
	SharedScopes         []string   `json:"shared_scopes"`
	Status               string     `json:"status"`
	LifecycleStage       string     `json:"lifecycle_stage"`
	AdminContacts        []string   `json:"admin_contacts"`
	LastHandshakeAt      *time.Time `json:"last_handshake_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type CreatePeerRequest struct {
	Slug                 string   `json:"slug"`
	DisplayName          string   `json:"display_name"`
	OrganizationType     string   `json:"organization_type"`
	Region               string   `json:"region"`
	EndpointURL          string   `json:"endpoint_url"`
	AuthMode             string   `json:"auth_mode"`
	TrustLevel           string   `json:"trust_level"`
	PublicKeyFingerprint string   `json:"public_key_fingerprint"`
	SharedScopes         []string `json:"shared_scopes"`
	AdminContacts        []string `json:"admin_contacts"`
}

type UpdatePeerRequest struct {
	DisplayName      *string   `json:"display_name"`
	OrganizationType *string   `json:"organization_type"`
	Region           *string   `json:"region"`
	EndpointURL      *string   `json:"endpoint_url"`
	TrustLevel       *string   `json:"trust_level"`
	SharedScopes     *[]string `json:"shared_scopes"`
	Status           *string   `json:"status"`
	LifecycleStage   *string   `json:"lifecycle_stage"`
	AdminContacts    *[]string `json:"admin_contacts"`
}

type SharedDataset struct {
	ID              uuid.UUID       `json:"id"`
	ContractID      uuid.UUID       `json:"contract_id"`
	ProviderPeerID  uuid.UUID       `json:"provider_peer_id"`
	ConsumerPeerID  uuid.UUID       `json:"consumer_peer_id"`
	ProviderSpaceID *uuid.UUID      `json:"provider_space_id"`
	ConsumerSpaceID *uuid.UUID      `json:"consumer_space_id"`
	DatasetName     string          `json:"dataset_name"`
	Selector        json.RawMessage `json:"selector"`
	ProviderSchema  json.RawMessage `json:"provider_schema"`
	ConsumerSchema  json.RawMessage `json:"consumer_schema"`
	SampleRows      json.RawMessage `json:"sample_rows"`
	ReplicationMode string          `json:"replication_mode"`
	Status          string          `json:"status"`
	LastSyncAt      *time.Time      `json:"last_sync_at"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type ShareManifest struct {
	Share      SharedDataset `json:"share"`
	SyncStatus *SyncStatus   `json:"sync_status"`
}

type CreateShareRequest struct {
	ContractID      uuid.UUID       `json:"contract_id"`
	ProviderPeerID  uuid.UUID       `json:"provider_peer_id"`
	ConsumerPeerID  uuid.UUID       `json:"consumer_peer_id"`
	ProviderSpaceID *uuid.UUID      `json:"provider_space_id"`
	ConsumerSpaceID *uuid.UUID      `json:"consumer_space_id"`
	DatasetName     string          `json:"dataset_name"`
	Selector        json.RawMessage `json:"selector"`
	ProviderSchema  json.RawMessage `json:"provider_schema"`
	ConsumerSchema  json.RawMessage `json:"consumer_schema"`
	SampleRows      json.RawMessage `json:"sample_rows"`
	ReplicationMode string          `json:"replication_mode"`
}

type SyncStatusUpdateRequest struct {
	Status             *string    `json:"status"`
	RowsReplicated     *int64     `json:"rows_replicated"`
	BacklogRows        *int64     `json:"backlog_rows"`
	EncryptedInTransit *bool      `json:"encrypted_in_transit"`
	EncryptedAtRest    *bool      `json:"encrypted_at_rest"`
	KeyVersion         *string    `json:"key_version"`
	LastSyncAt         *time.Time `json:"last_sync_at"`
	NextSyncAt         *time.Time `json:"next_sync_at"`
	AuditCursor        *string    `json:"audit_cursor"`
}

type CreateContractRequest struct {
	PeerID            uuid.UUID `json:"peer_id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	DatasetLocator    string    `json:"dataset_locator"`
	AllowedPurposes   []string  `json:"allowed_purposes"`
	DataClasses       []string  `json:"data_classes"`
	ResidencyRegion   string    `json:"residency_region"`
	QueryTemplate     string    `json:"query_template"`
	MaxRowsPerQuery   int64     `json:"max_rows_per_query"`
	ReplicationMode   string    `json:"replication_mode"`
	EncryptionProfile string    `json:"encryption_profile"`
	RetentionDays     int32     `json:"retention_days"`
	Status            string    `json:"status"`
	ExpiresAt         time.Time `json:"expires_at"`
}

type UpdateContractRequest struct {
	Name              *string    `json:"name"`
	Description       *string    `json:"description"`
	DatasetLocator    *string    `json:"dataset_locator"`
	AllowedPurposes   *[]string  `json:"allowed_purposes"`
	DataClasses       *[]string  `json:"data_classes"`
	ResidencyRegion   *string    `json:"residency_region"`
	QueryTemplate     *string    `json:"query_template"`
	MaxRowsPerQuery   *int64     `json:"max_rows_per_query"`
	ReplicationMode   *string    `json:"replication_mode"`
	EncryptionProfile *string    `json:"encryption_profile"`
	RetentionDays     *int32     `json:"retention_days"`
	Status            *string    `json:"status"`
	ExpiresAt         *time.Time `json:"expires_at"`
}

// FederatedQueryRequest mirrors `models::access_grant::FederatedQueryRequest`
// from Rust. It is the wire shape for `POST /api/v1/product-distribution/queries`.
// `Limit` is optional and clamped against the access grant's
// max_rows_per_query at runtime.
type FederatedQueryRequest struct {
	ShareID uuid.UUID `json:"share_id"`
	SQL     string    `json:"sql"`
	Purpose string    `json:"purpose"`
	Limit   *int      `json:"limit,omitempty"`
}

// FederatedQueryResult mirrors `models::access_grant::FederatedQueryResult`
// from Rust. Field order, names, and JSON tags match the Rust struct so
// the federation UI deserializes the Go response without changes.
type FederatedQueryResult struct {
	ShareID     uuid.UUID         `json:"share_id"`
	DatasetName string            `json:"dataset_name"`
	SourcePeer  string            `json:"source_peer"`
	ExecutedSQL string            `json:"executed_sql"`
	QueryMode   string            `json:"query_mode"`
	Limit       int               `json:"limit"`
	Columns     []string          `json:"columns"`
	Rows        []json.RawMessage `json:"rows"`
}
