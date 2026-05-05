package controlbus

import (
	"encoding/json"

	"github.com/google/uuid"
)

// NotificationEvent is the canonical control-plane payload for the
// "notification.updated" subject. Matches the Rust generic
// `NotificationEvent<T>` with T expressed as RawMessage on the Go side.
type NotificationEvent struct {
	Kind         string          `json:"kind"`
	UserID       *uuid.UUID      `json:"user_id,omitempty"`
	Notification json.RawMessage `json:"notification,omitempty"`
	UnreadCount  int64           `json:"unread_count"`
}

// WorkflowTriggerRequested is the payload for "workflow.trigger.requested".
type WorkflowTriggerRequested struct {
	TriggerType string          `json:"trigger_type"`
	StartedBy   *uuid.UUID      `json:"started_by,omitempty"`
	Context     json.RawMessage `json:"context,omitempty"`
}

// DatasetQualityRefreshRequested is the payload for
// "dataset.quality.refresh.requested".
//
// The default reason "dataset_upload" matches the Rust
// `default_quality_refresh_reason` so cross-language consumers see
// identical event bodies.
type DatasetQualityRefreshRequested struct {
	DatasetID   uuid.UUID       `json:"dataset_id"`
	RequestedBy *uuid.UUID      `json:"requested_by,omitempty"`
	Reason      string          `json:"reason"`
	Context     json.RawMessage `json:"context,omitempty"`
}

// DatasetQualityRefreshForUpload is the helper the dataset-versioning
// service uses to publish a refresh request after an upload.
func DatasetQualityRefreshForUpload(datasetID uuid.UUID) DatasetQualityRefreshRequested {
	return DatasetQualityRefreshRequested{
		DatasetID: datasetID,
		Reason:    "dataset_upload",
		Context:   json.RawMessage(`{"trigger":{"type":"dataset_upload"}}`),
	}
}
