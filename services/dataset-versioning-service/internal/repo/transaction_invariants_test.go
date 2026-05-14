package repo

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

func TestTransactionTypeMatrixMatchesRustViewReplay(t *testing.T) {
	view, err := applyTransaction(nil, models.TransactionTypeSnapshot, []stagedFileRow{
		{LogicalPath: "A", PhysicalPath: "phys/A.v0", SizeBytes: 10, Op: models.FileOperationAdd},
		{LogicalPath: "B", PhysicalPath: "phys/B.v0", SizeBytes: 20, Op: models.FileOperationAdd},
	})
	require.NoError(t, err)
	require.Len(t, view, 2)

	require.NoError(t, validateCommit(models.TransactionTypeAppend, []stagedFileRow{{LogicalPath: "C", PhysicalPath: "phys/C.v0", SizeBytes: 30, Op: models.FileOperationAdd}}, view))
	view, err = applyTransaction(view, models.TransactionTypeAppend, []stagedFileRow{{LogicalPath: "C", PhysicalPath: "phys/C.v0", SizeBytes: 30, Op: models.FileOperationAdd}})
	require.NoError(t, err)
	require.Len(t, view, 3)

	updateRows := []stagedFileRow{
		{LogicalPath: "A", PhysicalPath: "phys/A.v1", SizeBytes: 11, Op: models.FileOperationReplace},
		{LogicalPath: "D", PhysicalPath: "phys/D.v0", SizeBytes: 40, Op: models.FileOperationAdd},
	}
	require.NoError(t, validateCommit(models.TransactionTypeUpdate, updateRows, view))
	view, err = applyTransaction(view, models.TransactionTypeUpdate, updateRows)
	require.NoError(t, err)
	require.Equal(t, int64(11), view["A"].SizeBytes)
	require.Equal(t, int64(40), view["D"].SizeBytes)

	require.NoError(t, validateCommit(models.TransactionTypeDelete, []stagedFileRow{{LogicalPath: "B", Op: models.FileOperationRemove}}, view))
	view, err = applyTransaction(view, models.TransactionTypeDelete, []stagedFileRow{{LogicalPath: "B", Op: models.FileOperationRemove}})
	require.NoError(t, err)
	require.NotContains(t, view, "B")
	require.Contains(t, view, "A")
	require.Contains(t, view, "C")
	require.Contains(t, view, "D")

	require.NoError(t, validateCommit(models.TransactionTypeSnapshot, []stagedFileRow{{LogicalPath: "E", PhysicalPath: "phys/E.v0", SizeBytes: 50, Op: models.FileOperationAdd}}, view))
	view, err = applyTransaction(view, models.TransactionTypeSnapshot, []stagedFileRow{{LogicalPath: "E", PhysicalPath: "phys/E.v0", SizeBytes: 50, Op: models.FileOperationAdd}})
	require.NoError(t, err)
	require.Len(t, view, 1)
	require.Equal(t, int64(50), view["E"].SizeBytes)
}

func TestTransactionCommitValidationMatchesRustErrors(t *testing.T) {
	current := map[string]viewFileRow{"existing": {PhysicalPath: "phys/existing", SizeBytes: 1}}

	require.ErrorIs(t,
		validateCommit(models.TransactionTypeAppend, []stagedFileRow{{LogicalPath: "existing", PhysicalPath: "phys/new", SizeBytes: 2, Op: models.FileOperationAdd}}, current),
		ErrConflict,
	)
	require.ErrorIs(t,
		validateCommit(models.TransactionTypeAppend, []stagedFileRow{{LogicalPath: "new", PhysicalPath: "phys/new", SizeBytes: 2, Op: models.FileOperationReplace}}, current),
		ErrConflict,
	)
	require.ErrorIs(t,
		validateCommit(models.TransactionTypeDelete, []stagedFileRow{{LogicalPath: "new", PhysicalPath: "phys/new", SizeBytes: 2, Op: models.FileOperationAdd}}, current),
		ErrValidation,
	)
	require.ErrorIs(t,
		validateCommit(models.TransactionTypeUpdate, []stagedFileRow{{LogicalPath: "existing", Op: models.FileOperationRemove}}, current),
		ErrValidation,
	)
	require.ErrorIs(t,
		validateCommit(models.TransactionTypeSnapshot, []stagedFileRow{{LogicalPath: "old", Op: models.FileOperationRemove}}, current),
		ErrValidation,
	)
}

func TestDeleteTransactionDoesNotPurgeStorageFromViewReplay(t *testing.T) {
	view := map[string]viewFileRow{
		"keep":   {PhysicalPath: "phys/keep", SizeBytes: 1},
		"remove": {PhysicalPath: "phys/remove", SizeBytes: 2},
	}
	physical := view["remove"].PhysicalPath

	view, err := applyTransaction(view, models.TransactionTypeDelete, []stagedFileRow{{LogicalPath: "remove", PhysicalPath: physical, Op: models.FileOperationRemove}})
	require.NoError(t, err)
	require.NotContains(t, view, "remove")
	require.Equal(t, "phys/remove", physical, "DELETE removes the logical reference only; retention owns physical purge")
}
