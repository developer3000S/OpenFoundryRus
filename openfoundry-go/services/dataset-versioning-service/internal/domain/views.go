// Package domain holds the pure (Postgres-free) algorithms behind the
// dataset-versioning service: the Foundry "Dataset views" replay loop and
// the parameterized union-view SQL composer.
//
// Pure dataset-view computation algorithm (T1.3).
//
// Implements the Foundry "Dataset views" algorithm:
//
//  1. Start from an empty file set.
//  2. Locate the **last** SNAPSHOT whose timestamp is <= at_ts. If none
//     exists, replay every committed transaction from the oldest one.
//  3. From that anchor, replay each subsequent committed transaction in
//     timestamp order applying:
//     * SNAPSHOT / APPEND -> insert/overwrite the file in the set.
//     * UPDATE            -> overwrite (or insert) the file.
//     * DELETE            -> drop the file from the set.
//
// ComputeView is intentionally pure: it consumes a flat slice of
// TransactionEntry and a cut-off timestamp and returns the deterministic
// file set. Postgres I/O, branch resolution and view caching live in the
// handler / repo layer.
package domain

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

// FileRef is a reference to a single physical file inside a dataset view.
//
// LogicalPath is the dataset-relative key (stable across snapshots and
// transactions). PhysicalPath is the backing-store key produced by the
// writer; the two are decoupled so that compaction / Iceberg snapshotting
// can rewrite files without breaking view stability.
type FileRef struct {
	LogicalPath  string
	PhysicalPath string
	SizeBytes    int64
	IntroducedBy uuid.UUID
}

// FileOpKind identifies the per-file op staged inside a transaction.
type FileOpKind int

const (
	// FileOpAdd is a new file added by the transaction (APPEND / SNAPSHOT).
	FileOpAdd FileOpKind = iota
	// FileOpReplace is an existing file overwritten (UPDATE / SNAPSHOT-after-collision).
	FileOpReplace
	// FileOpRemove is a file logically removed (DELETE). PhysicalPath is
	// preserved so auditors can still locate the orphaned blob.
	FileOpRemove
)

// FileOp is a per-file op staged inside a transaction.
type FileOp struct {
	LogicalPath  string
	PhysicalPath string
	SizeBytes    int64
	Kind         FileOpKind
}

// TransactionEntry is one committed transaction ready to be replayed.
type TransactionEntry struct {
	TxnID uuid.UUID
	Kind  models.TransactionType
	// CommittedAt is the effective timestamp used for ordering
	// (COALESCE(committed_at, started_at)).
	CommittedAt time.Time
	Files       []FileOp
}

// ComputeView computes the current view of a dataset/branch as of atTs.
//
// transactions MUST contain only COMMITTED transactions for the branch and
// MUST be sorted by CommittedAt ascending. Aborted/open transactions and
// other branches must be filtered out by the caller.
//
// atTs == nil means "no cut-off" (replay all committed transactions).
func ComputeView(transactions []TransactionEntry, atTs *time.Time) []FileRef {
	inWindow := make([]*TransactionEntry, 0, len(transactions))
	for i := range transactions {
		t := &transactions[i]
		if atTs == nil || !t.CommittedAt.After(*atTs) {
			inWindow = append(inWindow, t)
		}
	}

	if len(inWindow) == 0 {
		return []FileRef{}
	}

	startIdx := 0
	for i := len(inWindow) - 1; i >= 0; i-- {
		if inWindow[i].Kind == models.TransactionTypeSnapshot {
			startIdx = i
			break
		}
	}

	files := map[string]FileRef{}

	for _, entry := range inWindow[startIdx:] {
		switch entry.Kind {
		case models.TransactionTypeSnapshot:
			files = map[string]FileRef{}
			for _, op := range entry.Files {
				files[op.LogicalPath] = FileRef{
					LogicalPath:  op.LogicalPath,
					PhysicalPath: op.PhysicalPath,
					SizeBytes:    op.SizeBytes,
					IntroducedBy: entry.TxnID,
				}
			}
		case models.TransactionTypeAppend:
			for _, op := range entry.Files {
				if _, exists := files[op.LogicalPath]; !exists {
					files[op.LogicalPath] = FileRef{
						LogicalPath:  op.LogicalPath,
						PhysicalPath: op.PhysicalPath,
						SizeBytes:    op.SizeBytes,
						IntroducedBy: entry.TxnID,
					}
				}
			}
		case models.TransactionTypeUpdate:
			for _, op := range entry.Files {
				files[op.LogicalPath] = FileRef{
					LogicalPath:  op.LogicalPath,
					PhysicalPath: op.PhysicalPath,
					SizeBytes:    op.SizeBytes,
					IntroducedBy: entry.TxnID,
				}
			}
		case models.TransactionTypeDelete:
			for _, op := range entry.Files {
				delete(files, op.LogicalPath)
			}
		}
	}

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]FileRef, 0, len(keys))
	for _, k := range keys {
		out = append(out, files[k])
	}
	return out
}
