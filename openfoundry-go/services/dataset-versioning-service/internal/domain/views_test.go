package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

func tsAt(secs int64) time.Time {
	return time.Unix(secs, 0).UTC()
}

func addOp(logical, physical string) FileOp {
	return FileOp{LogicalPath: logical, PhysicalPath: physical, SizeBytes: 1, Kind: FileOpAdd}
}

func replaceOp(logical, physical string) FileOp {
	return FileOp{LogicalPath: logical, PhysicalPath: physical, SizeBytes: 1, Kind: FileOpReplace}
}

func removeOp(logical string) FileOp {
	return FileOp{LogicalPath: logical, PhysicalPath: "", SizeBytes: 0, Kind: FileOpRemove}
}

func entry(secs int64, kind models.TransactionType, files []FileOp) TransactionEntry {
	return TransactionEntry{
		TxnID:       uuid.New(),
		Kind:        kind,
		CommittedAt: tsAt(secs),
		Files:       files,
	}
}

func logicalPaths(view []FileRef) []string {
	out := make([]string, 0, len(view))
	for _, f := range view {
		out = append(out, f.LogicalPath)
	}
	return out
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The worked example from Datasets.md:
// [SNAPSHOT(A,B), APPEND(C), UPDATE(A->A'), DELETE(B)] => {A',C}.
func TestComputeViewDocExampleSnapshotAppendUpdateDelete(t *testing.T) {
	txns := []TransactionEntry{
		entry(10, models.TransactionTypeSnapshot, []FileOp{addOp("A", "phys/A.v0"), addOp("B", "phys/B.v0")}),
		entry(20, models.TransactionTypeAppend, []FileOp{addOp("C", "phys/C.v0")}),
		entry(30, models.TransactionTypeUpdate, []FileOp{replaceOp("A", "phys/A.v1")}),
		entry(40, models.TransactionTypeDelete, []FileOp{removeOp("B")}),
	}

	view := ComputeView(txns, nil)
	if got := logicalPaths(view); !equalSlice(got, []string{"A", "C"}) {
		t.Fatalf("logical paths = %v, want [A C]", got)
	}

	var a *FileRef
	for i := range view {
		if view[i].LogicalPath == "A" {
			a = &view[i]
			break
		}
	}
	if a == nil {
		t.Fatal("A not found")
	}
	if a.PhysicalPath != "phys/A.v1" {
		t.Fatalf("A physical = %q, want phys/A.v1 (UPDATEd)", a.PhysicalPath)
	}
}

// A trailing SNAPSHOT must wipe everything before it.
func TestComputeViewTrailingSnapshotResetsView(t *testing.T) {
	txns := []TransactionEntry{
		entry(10, models.TransactionTypeSnapshot, []FileOp{addOp("A", "phys/A.v0"), addOp("B", "phys/B.v0")}),
		entry(20, models.TransactionTypeAppend, []FileOp{addOp("C", "phys/C.v0")}),
		entry(30, models.TransactionTypeUpdate, []FileOp{replaceOp("A", "phys/A.v1")}),
		entry(40, models.TransactionTypeDelete, []FileOp{removeOp("B")}),
		entry(50, models.TransactionTypeSnapshot, []FileOp{addOp("D", "phys/D.v0")}),
	}

	view := ComputeView(txns, nil)
	if got := logicalPaths(view); !equalSlice(got, []string{"D"}) {
		t.Fatalf("logical paths = %v, want [D]", got)
	}
}

// Time-travel: cut-off before the trailing SNAPSHOT must yield the
// original {A', C} view.
func TestComputeViewAtTsBeforeTrailingSnapshot(t *testing.T) {
	txns := []TransactionEntry{
		entry(10, models.TransactionTypeSnapshot, []FileOp{addOp("A", "phys/A.v0"), addOp("B", "phys/B.v0")}),
		entry(20, models.TransactionTypeAppend, []FileOp{addOp("C", "phys/C.v0")}),
		entry(30, models.TransactionTypeUpdate, []FileOp{replaceOp("A", "phys/A.v1")}),
		entry(40, models.TransactionTypeDelete, []FileOp{removeOp("B")}),
		entry(50, models.TransactionTypeSnapshot, []FileOp{addOp("D", "phys/D.v0")}),
	}

	at := tsAt(45)
	view := ComputeView(txns, &at)
	if got := logicalPaths(view); !equalSlice(got, []string{"A", "C"}) {
		t.Fatalf("logical paths = %v, want [A C]", got)
	}
}

// Empty input or cut-off before the first txn yields an empty view.
func TestComputeViewEmptyWhenNoTransactionsInWindow(t *testing.T) {
	txns := []TransactionEntry{
		entry(100, models.TransactionTypeSnapshot, []FileOp{addOp("A", "phys/A")}),
	}
	at := tsAt(50)
	if got := ComputeView(txns, &at); len(got) != 0 {
		t.Fatalf("expected empty view for cutoff-before-first, got %v", got)
	}
	if got := ComputeView(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty view for nil input, got %v", got)
	}
}

// When no SNAPSHOT exists, replay starts at the oldest txn (Foundry
// semantics: the first transaction is implicitly anchoring).
func TestComputeViewNoSnapshotReplaysFromOldest(t *testing.T) {
	txns := []TransactionEntry{
		entry(10, models.TransactionTypeAppend, []FileOp{addOp("A", "phys/A")}),
		entry(20, models.TransactionTypeAppend, []FileOp{addOp("B", "phys/B")}),
		entry(30, models.TransactionTypeUpdate, []FileOp{replaceOp("A", "phys/A.v1")}),
	}
	view := ComputeView(txns, nil)
	if got := logicalPaths(view); !equalSlice(got, []string{"A", "B"}) {
		t.Fatalf("logical paths = %v, want [A B]", got)
	}
	if view[0].PhysicalPath != "phys/A.v1" {
		t.Fatalf("A physical = %q, want phys/A.v1", view[0].PhysicalPath)
	}
}

// physical_path is decoupled from logical_path: an UPDATE rewrites the
// backing file but the dataset-relative key is unchanged.
func TestComputeViewPhysicalPathDecoupledFromLogical(t *testing.T) {
	txns := []TransactionEntry{
		entry(10, models.TransactionTypeSnapshot, []FileOp{addOp("data/part-0.parquet", "store/abcd.parquet")}),
		entry(20, models.TransactionTypeUpdate, []FileOp{replaceOp("data/part-0.parquet", "store/efgh.parquet")}),
	}
	view := ComputeView(txns, nil)
	if len(view) != 1 {
		t.Fatalf("len(view) = %d, want 1", len(view))
	}
	if view[0].LogicalPath != "data/part-0.parquet" {
		t.Fatalf("logical = %q", view[0].LogicalPath)
	}
	if view[0].PhysicalPath != "store/efgh.parquet" {
		t.Fatalf("physical = %q", view[0].PhysicalPath)
	}
}
