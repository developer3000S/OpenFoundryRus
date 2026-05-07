package domain

import (
	"time"

	"github.com/google/uuid"
)

// WindowDefinition mirrors event_streaming::models::window::WindowDefinition.
//
// The engine consumes the rich form (aggregation_keys + measure_fields).
// The wire-shape models.WindowDefinition is a lightweight subset; richer
// fields land via the engine adapter when richer DB scans are wired.
type WindowDefinition struct {
	ID                     uuid.UUID `json:"id"`
	Name                   string    `json:"name"`
	Description            string    `json:"description"`
	Status                 string    `json:"status"`
	WindowType             string    `json:"window_type"`
	DurationSeconds        int32     `json:"duration_seconds"`
	SlideSeconds           int32     `json:"slide_seconds"`
	SessionGapSeconds      int32     `json:"session_gap_seconds"`
	AllowedLatenessSeconds int32     `json:"allowed_lateness_seconds"`
	AggregationKeys        []string  `json:"aggregation_keys"`
	MeasureFields          []string  `json:"measure_fields"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}
