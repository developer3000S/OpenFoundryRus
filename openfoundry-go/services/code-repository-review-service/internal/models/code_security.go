package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CodeSecurityScan mirrors code_security_scans. Payload is intentionally
// opaque because the absorbed Rust schema stores scan metadata as JSONB.
type CodeSecurityScan struct {
	ID        uuid.UUID       `json:"id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// CodeSecurityFinding mirrors code_security_findings.
type CodeSecurityFinding struct {
	ID        uuid.UUID       `json:"id"`
	ParentID  uuid.UUID       `json:"parent_id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
