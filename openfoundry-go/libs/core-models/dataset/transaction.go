// Package dataset hosts the canonical Foundry-style dataset primitives:
// dataset RIDs, branch names and transaction state.
//
// Wire format mirrors the Rust source of truth verbatim:
//   - TransactionID: bare UUID string
//   - DatasetRID:    "ri.foundry.main.dataset.<uuid>" string
//   - BranchName:    plain string
//   - TransactionType / State: lowercase tokens
package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
)

// ---------------------------------------------------------------------------
// TransactionID
// ---------------------------------------------------------------------------

// TransactionID is the internal primary key of a dataset transaction (UUID v7).
type TransactionID struct {
	uuid.UUID
}

// NewTransactionID mints a fresh time-ordered transaction id.
func NewTransactionID() TransactionID { return TransactionID{UUID: ids.New()} }

func (t TransactionID) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UUID.String())
}

func (t *TransactionID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return fmt.Errorf("transaction id: %w", err)
	}
	t.UUID = parsed
	return nil
}

// ---------------------------------------------------------------------------
// DatasetRID
// ---------------------------------------------------------------------------

// DatasetRIDPrefix is the canonical prefix every dataset RID carries.
const DatasetRIDPrefix = "ri.foundry.main.dataset."

// DatasetRID is the Foundry-style resource identifier for a dataset.
type DatasetRID string

// NewDatasetRID mints a brand-new RID backed by a v7 UUID.
func NewDatasetRID() DatasetRID {
	return DatasetRIDFromUUID(ids.New())
}

// DatasetRIDFromUUID builds the canonical "ri.foundry.main.dataset.<uuid>" form.
func DatasetRIDFromUUID(id uuid.UUID) DatasetRID {
	return DatasetRID(DatasetRIDPrefix + id.String())
}

// String returns the RID in its canonical form.
func (r DatasetRID) String() string { return string(r) }

// UUID extracts the UUID suffix; returns false if the RID is malformed.
func (r DatasetRID) UUID() (uuid.UUID, bool) {
	tail, ok := strings.CutPrefix(string(r), DatasetRIDPrefix)
	if !ok {
		return uuid.UUID{}, false
	}
	parsed, err := uuid.Parse(tail)
	if err != nil {
		return uuid.UUID{}, false
	}
	return parsed, true
}

// ParseDatasetRID validates and returns a DatasetRID.
func ParseDatasetRID(s string) (DatasetRID, error) {
	rid := DatasetRID(s)
	if _, ok := rid.UUID(); !ok {
		return "", &InvalidDatasetRIDError{Input: s}
	}
	return rid, nil
}

// InvalidDatasetRIDError is returned when ParseDatasetRID fails.
type InvalidDatasetRIDError struct{ Input string }

func (e *InvalidDatasetRIDError) Error() string {
	return fmt.Sprintf(
		"invalid dataset RID %q (expected %s<uuid>)",
		e.Input, DatasetRIDPrefix,
	)
}

// ---------------------------------------------------------------------------
// BranchName
// ---------------------------------------------------------------------------

// BranchName is a validated dataset branch name.
type BranchName string

const (
	// BranchMaster is the Foundry-default trunk branch.
	BranchMaster BranchName = "master"
	// BranchNameMaxLen is the upper bound enforced by ParseBranchName.
	BranchNameMaxLen = 200
)

// InvalidBranchNameError describes why a branch name failed validation.
type InvalidBranchNameError struct {
	Input  string
	Reason string
}

func (e *InvalidBranchNameError) Error() string {
	return fmt.Sprintf("invalid branch name %q: %s", e.Input, e.Reason)
}

// ParseBranchName validates the branch name and returns a typed value.
func ParseBranchName(s string) (BranchName, error) {
	switch {
	case len(s) == 0:
		return "", &InvalidBranchNameError{Input: s, Reason: "branch name must not be empty"}
	case len(s) > BranchNameMaxLen:
		return "", &InvalidBranchNameError{
			Input:  s,
			Reason: fmt.Sprintf("branch name too long (%d chars, max %d)", len(s), BranchNameMaxLen),
		}
	}
	for _, ch := range s {
		ok := (ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '_' || ch == '/' || ch == '.'
		if !ok {
			return "", &InvalidBranchNameError{
				Input:  s,
				Reason: fmt.Sprintf("invalid character %q (allowed: a-z A-Z 0-9 - _ / .)", ch),
			}
		}
	}
	return BranchName(s), nil
}

// ---------------------------------------------------------------------------
// TransactionType
// ---------------------------------------------------------------------------

// TransactionType is the operation taxonomy stored in `dataset_transactions.operation`.
type TransactionType string

const (
	TxSnapshot TransactionType = "snapshot"
	TxAppend   TransactionType = "append"
	TxUpdate   TransactionType = "update"
	TxDelete   TransactionType = "delete"
)

// ErrUnknownTransactionType is returned when a string token can't be parsed.
var ErrUnknownTransactionType = errors.New("unknown transaction type")

// ParseTransactionType is case-insensitive and matches the Rust FromStr impl.
func ParseTransactionType(s string) (TransactionType, error) {
	switch strings.ToLower(s) {
	case "snapshot":
		return TxSnapshot, nil
	case "append":
		return TxAppend, nil
	case "update":
		return TxUpdate, nil
	case "delete":
		return TxDelete, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownTransactionType, s)
	}
}

// ---------------------------------------------------------------------------
// TransactionState
// ---------------------------------------------------------------------------

// TransactionState is the lifecycle state of a dataset transaction.
type TransactionState string

const (
	TxOpen      TransactionState = "open"
	TxCommitted TransactionState = "committed"
	TxAborted   TransactionState = "aborted"
)

// ErrUnknownTransactionState is returned when a string token can't be parsed.
var ErrUnknownTransactionState = errors.New("unknown transaction state")

// ParseTransactionState is case-insensitive and matches the Rust FromStr impl.
func ParseTransactionState(s string) (TransactionState, error) {
	switch strings.ToLower(s) {
	case "open":
		return TxOpen, nil
	case "committed":
		return TxCommitted, nil
	case "aborted":
		return TxAborted, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownTransactionState, s)
	}
}

// IsTerminal reports whether the state forbids further transitions.
func (s TransactionState) IsTerminal() bool {
	return s == TxCommitted || s == TxAborted
}
