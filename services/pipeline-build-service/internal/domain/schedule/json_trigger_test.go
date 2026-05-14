package schedule

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvaluateJSONTimeTriggerUsesAPISerdeShape(t *testing.T) {
	raw := json.RawMessage(`{"kind":{"time":{"cron":"0 * * * *","time_zone":"UTC","flavor":"UNIX_5"}}}`)
	created := time.Date(2026, 5, 14, 8, 15, 0, 0, time.UTC)
	now := time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC)

	got, err := EvaluateJSONTrigger(raw, JSONTriggerContext{Now: now, CreatedAt: created})
	if err != nil {
		t.Fatalf("EvaluateJSONTrigger returned error: %v", err)
	}
	if !got.Fires {
		t.Fatalf("time trigger should fire at first cron boundary after creation")
	}
	if got.TriggerType != "TIME" {
		t.Fatalf("TriggerType = %q, want TIME", got.TriggerType)
	}
	if got.Snapshot["scheduled_for"] == "" {
		t.Fatalf("scheduled_for snapshot missing")
	}
}

func TestEvaluateJSONCompoundPersistsEventObservationUntilTimeFires(t *testing.T) {
	raw := json.RawMessage(`{"kind":{"compound":{"op":"AND","components":[
		{"kind":{"event":{"type":"DATA_UPDATED","target_rid":"ri.dataset.sales","branch_filter":["master"]}}},
		{"kind":{"time":{"cron":"0 * * * *","time_zone":"UTC","flavor":"UNIX_5"}}}
	]}}}`)
	event := JSONTriggerEvent{
		EventType:  EventTypeDataUpdated,
		TargetRID:  "ri.dataset.sales",
		Branch:     "master",
		OccurredAt: time.Date(2026, 5, 14, 8, 30, 0, 0, time.UTC),
	}

	observations, err := MatchingJSONEventObservations(raw, event)
	if err != nil {
		t.Fatalf("MatchingJSONEventObservations returned error: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("observations = %d, want 1", len(observations))
	}

	created := time.Date(2026, 5, 14, 8, 15, 0, 0, time.UTC)
	now := time.Date(2026, 5, 14, 9, 0, 0, 0, time.UTC)
	got, err := EvaluateJSONTrigger(raw, JSONTriggerContext{
		Now:       now,
		CreatedAt: created,
		Observed:  map[string]bool{observations[0].Path: true},
	})
	if err != nil {
		t.Fatalf("EvaluateJSONTrigger returned error: %v", err)
	}
	if !got.Fires {
		t.Fatalf("compound trigger should fire once persisted event and time condition are both satisfied")
	}
	if got.TriggerType != "COMPOUND" {
		t.Fatalf("TriggerType = %q, want COMPOUND", got.TriggerType)
	}
}
