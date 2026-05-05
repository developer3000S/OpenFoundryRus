// Package timestamp provides the standard created_at / updated_at
// envelope embedded in every persistent entity.
package timestamp

import "time"

// Timestamps mirrors the Rust `core_models::timestamp::Timestamps`.
type Timestamps struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Now returns a fresh pair where both fields equal the current UTC time.
func Now() Timestamps {
	now := time.Now().UTC()
	return Timestamps{CreatedAt: now, UpdatedAt: now}
}
