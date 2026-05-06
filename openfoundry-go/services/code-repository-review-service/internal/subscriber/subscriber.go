// Package subscriber holds the Kafka subscriber port for
// `foundry.branch.events.v1`. The Rust binary intentionally exposes
// the port without linking rdkafka so the binary stays slim; tests
// drive the port directly via Handle.
//
// Foundation slice: port + a Postgres-backed implementation that
// projects per-plane branch events onto resource-link rows. The
// kafka-go consumer wiring lands in a follow-up slice.
package subscriber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/repo"
)

// Topic is the canonical Kafka topic name. Preserved verbatim from Rust.
const Topic = "foundry.branch.events.v1"

// GlobalRIDPrefix is the prefix on RIDs of `global_branches`. Preserved
// verbatim from the Rust subscriber (`ri.foundry.main.globalbranch.`).
const GlobalRIDPrefix = "ri.foundry.main.globalbranch."

// SubscriberError sentinel cases mirror the Rust enum.
var (
	ErrMissingField = errors.New("subscriber: payload missing required field")
	ErrMalformed    = errors.New("subscriber: payload field malformed")
)

// MissingFieldError carries the field name through errors.Is.
type MissingFieldError struct{ Field string }

func (e *MissingFieldError) Error() string {
	return fmt.Sprintf("payload missing required field %s", e.Field)
}
func (e *MissingFieldError) Unwrap() error { return ErrMissingField }

// MalformedFieldError carries the field name through errors.Is.
type MalformedFieldError struct{ Field string }

func (e *MalformedFieldError) Error() string {
	return fmt.Sprintf("payload field %s is malformed", e.Field)
}
func (e *MalformedFieldError) Unwrap() error { return ErrMalformed }

// SubscriberPort decouples the Postgres write path from the Kafka
// driver — handlers / tests call Handle directly with a decoded event.
type SubscriberPort interface {
	Handle(ctx context.Context, event json.RawMessage) error
}

// PostgresSubscriber projects branch events onto resource-link rows.
type PostgresSubscriber struct {
	Repo *repo.GlobalBranchRepo
}

// Handle dispatches by `event_type`. Unknown event types are ignored.
func (s *PostgresSubscriber) Handle(ctx context.Context, raw json.RawMessage) error {
	eventType, branchRID, err := readRequiredString(raw, "event_type", "branch_rid")
	if err != nil {
		return err
	}
	switch eventType {
	case "dataset.branch.created.v1":
		globalRIDLabel, ok := readLabel(raw, "global_branch")
		if !ok {
			// No global label → ignored, matches Rust (the if-let).
			return nil
		}
		globalID, ok := ParseGlobalRID(globalRIDLabel)
		if !ok {
			return &MalformedFieldError{Field: "labels.global_branch"}
		}
		datasetRID, err := readRequiredField(raw, "dataset_rid")
		if err != nil {
			return err
		}
		_, err = s.Repo.AddLink(ctx, globalID, models.CreateGlobalBranchLinkRequest{
			ResourceType: "dataset",
			ResourceRID:  datasetRID,
			BranchRID:    branchRID,
		})
		return err
	case "dataset.branch.archived.v1":
		_, err := s.Repo.UpdateLinksForBranch(ctx, branchRID, "archived")
		return err
	case "dataset.branch.restored.v1":
		_, err := s.Repo.UpdateLinksForBranch(ctx, branchRID, "in_sync")
		return err
	case "dataset.branch.reparented.v1", "dataset.branch.markings.updated.v1":
		_, err := s.Repo.UpdateLinksForBranch(ctx, branchRID, "drifted")
		return err
	default:
		return nil
	}
}

// ParseGlobalRID accepts either `ri.foundry.main.globalbranch.<uuid>`
// or a bare UUID. Mirrors Rust `parse_global_rid`.
func ParseGlobalRID(s string) (uuid.UUID, bool) {
	t := strings.TrimSpace(s)
	if rest, ok := strings.CutPrefix(t, GlobalRIDPrefix); ok {
		t = rest
	}
	id, err := uuid.Parse(t)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// --- payload helpers ----------------------------------------------------

func readRequiredString(raw json.RawMessage, fields ...string) (string, string, error) {
	if len(fields) != 2 {
		return "", "", fmt.Errorf("readRequiredString expects 2 fields")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", "", err
	}
	a, err := scalarString(m, fields[0])
	if err != nil {
		return "", "", err
	}
	b, err := scalarString(m, fields[1])
	if err != nil {
		return "", "", err
	}
	return a, b, nil
}

func readRequiredField(raw json.RawMessage, field string) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	return scalarString(m, field)
}

func scalarString(m map[string]json.RawMessage, field string) (string, error) {
	v, ok := m[field]
	if !ok {
		return "", &MissingFieldError{Field: field}
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", &MissingFieldError{Field: field}
	}
	return s, nil
}

// readLabel extracts `labels.<key>` as string. Returns ok=false when
// the labels object is absent or the key is missing/non-string.
func readLabel(raw json.RawMessage, key string) (string, bool) {
	var outer struct {
		Labels map[string]json.RawMessage `json:"labels"`
	}
	if err := json.Unmarshal(raw, &outer); err != nil || outer.Labels == nil {
		return "", false
	}
	v, ok := outer.Labels[key]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", false
	}
	return s, true
}
