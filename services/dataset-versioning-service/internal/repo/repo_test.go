package repo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func TestListFilesReplaysCurrentViewAndHydratesMetadata(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	branchID := uuid.New()
	txnID := uuid.New()
	newerID := uuid.New()
	now := time.Now().UTC()
	sha := "abc123"
	mediaType := "application/parquet"
	rowCount := int64(250)
	storageLocation := []byte(`{"uri":"local:///new.parquet","logical_path":"daily/part-000.parquet"}`)

	branchRows := pgxmock.NewRows([]string{
		"id", "rid", "dataset_id", "dataset_rid", "name", "parent_branch_id", "head_transaction_id",
		"created_from_transaction_id", "last_activity_at", "labels", "fallback_chain", "created_at", "updated_at",
	}).AddRow(branchID, "ri.foundry.main.branch."+branchID.String(), datasetID, "ri.foundry.main.dataset."+datasetID.String(), "main", nil, &txnID, nil, now, []byte(`{}`), []string{}, now, now)
	mock.ExpectQuery("SELECT id, rid, dataset_id, dataset_rid, name").
		WithArgs(datasetID, "main").
		WillReturnRows(branchRows)
	mock.ExpectQuery("SELECT id, tx_type, started_at, committed_at").
		WithArgs(branchID, (*time.Time)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "tx_type", "started_at", "committed_at"}).
			AddRow(txnID, "SNAPSHOT", now, &now))
	mock.ExpectQuery("SELECT logical_path, physical_path, size_bytes, op").
		WithArgs(txnID).
		WillReturnRows(pgxmock.NewRows([]string{"logical_path", "physical_path", "size_bytes", "op"}).
			AddRow("daily/part-000.parquet", "local:///new.parquet", int64(42), "ADD").
			AddRow("other/part-001.parquet", "local:///other.parquet", int64(1), "ADD"))
	rows := pgxmock.NewRows([]string{
		"id", "dataset_id", "transaction_id", "transaction_rid", "logical_path", "physical_uri",
		"size_bytes", "media_type", "sha256", "row_count_hint", "storage_location",
		"created_at", "modified_at", "deleted_at", "status",
	}).
		AddRow(newerID, datasetID, txnID, "ri.foundry.main.transaction."+txnID.String(), "daily/part-000.parquet", "local:///new.parquet", int64(42), &mediaType, &sha, &rowCount, storageLocation, now, now, nil, "active")

	mock.ExpectQuery("SELECT df.id").
		WithArgs(datasetID, txnID, "daily/part-000.parquet").
		WillReturnRows(rows)

	r := &repo.Repo{Pool: mock}
	files, err := r.ListFiles(ctx, datasetID, "main", "daily/")
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, newerID, files[0].ID)
	require.Equal(t, "daily/part-000.parquet", files[0].LogicalPath)
	require.Equal(t, "daily/part-000.parquet", files[0].Path)
	require.Equal(t, "ri.foundry.main.transaction."+txnID.String(), files[0].TransactionRID)
	require.Equal(t, "active", files[0].Status)
	require.NotNil(t, files[0].SHA256)
	require.Equal(t, &mediaType, files[0].MediaType)
	require.Equal(t, &rowCount, files[0].RowCountHint)
	require.JSONEq(t, string(storageLocation), string(files[0].StorageLocation))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetViewAtTransactionCutoffMaterializesManifest(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	branchID := uuid.New()
	txn1 := uuid.New()
	txn2 := uuid.New()
	cacheID := uuid.New()
	now := time.Now().UTC()

	branchRows := pgxmock.NewRows([]string{
		"id", "rid", "dataset_id", "dataset_rid", "name", "parent_branch_id", "head_transaction_id",
		"created_from_transaction_id", "last_activity_at", "labels", "fallback_chain", "created_at", "updated_at",
	}).AddRow(branchID, "ri.foundry.main.branch."+branchID.String(), datasetID, "ri.foundry.main.dataset."+datasetID.String(), "main", nil, &txn2, nil, now, []byte(`{}`), []string{}, now, now)
	mock.ExpectQuery("SELECT id, rid, dataset_id, dataset_rid, name").
		WithArgs(datasetID, "main").
		WillReturnRows(branchRows)
	mock.ExpectQuery("SELECT id, tx_type, started_at, committed_at").
		WithArgs(branchID, (*time.Time)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "tx_type", "started_at", "committed_at"}).
			AddRow(txn1, "SNAPSHOT", now, &now).
			AddRow(txn2, "APPEND", now, &now))
	mock.ExpectQuery("SELECT logical_path, physical_path, size_bytes, op").
		WithArgs(txn1).
		WillReturnRows(pgxmock.NewRows([]string{"logical_path", "physical_path", "size_bytes", "op"}).
			AddRow("A", "phys/A", int64(10), "ADD"))
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO dataset_views").
		WithArgs(pgxmock.AnyArg(), datasetID, "__manifest__main__"+txn1.String(), "Cached dataset view manifest", "", "main", "manifest", pgxmock.AnyArg(), branchID, txn1, int32(1), int64(10), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(cacheID))
	mock.ExpectExec("DELETE FROM dataset_view_files").
		WithArgs(cacheID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO dataset_view_files").
		WithArgs(cacheID, "A", "phys/A", int64(10), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	r := &repo.Repo{Pool: mock}
	view, err := r.GetViewAt(ctx, datasetID, "main", nil, &txn1, nil)
	require.NoError(t, err)
	require.Equal(t, cacheID, view.ID)
	require.Equal(t, txn1, view.HeadTransactionID)
	require.Len(t, view.Files, 1)
	require.Equal(t, "A", view.Files[0].LogicalPath)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListViewFilesReadsCachedManifest(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	viewID := uuid.New()
	txnID := uuid.New()
	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM dataset_views").
		WithArgs(viewID, datasetID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT b.dataset_id").
		WithArgs(viewID).
		WillReturnRows(pgxmock.NewRows([]string{"dataset_id", "dataset_rid", "branch", "alias", "position", "schema_version_id", "created_at", "updated_at"}))
	mock.ExpectQuery("SELECT vf.logical_path").
		WithArgs(viewID, datasetID).
		WillReturnRows(pgxmock.NewRows([]string{"logical_path", "physical_path", "size_bytes", "introduced_by"}).
			AddRow("part-000.parquet", "local:///part-000.parquet", int64(42), &txnID))

	r := &repo.Repo{Pool: mock}
	files, err := r.ListViewFiles(ctx, datasetID, viewID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "part-000.parquet", files[0].LogicalPath)
	require.Equal(t, &txnID, files[0].IntroducedBy)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListViewFilesReturnsEmptyForLogicalView(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	viewID := uuid.New()
	backingID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM dataset_views").
		WithArgs(viewID, datasetID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT b.dataset_id").
		WithArgs(viewID).
		WillReturnRows(pgxmock.NewRows([]string{"dataset_id", "dataset_rid", "branch", "alias", "position", "schema_version_id", "created_at", "updated_at"}).
			AddRow(backingID, "ri.foundry.main.dataset."+backingID.String(), "main", "sales", int32(0), nil, now, now))

	r := &repo.Repo{Pool: mock}
	files, err := r.ListViewFiles(ctx, datasetID, viewID)
	require.NoError(t, err)
	require.Empty(t, files)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFilePropagatesQueryError(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	fileID := uuid.New()
	mock.ExpectQuery("SELECT df.id").
		WithArgs(datasetID, fileID).
		WillReturnError(errors.New("query cancelled"))

	r := &repo.Repo{Pool: mock}
	_, err = r.GetFile(ctx, datasetID, fileID)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTransactionStatus(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	txnID := uuid.New()
	mock.ExpectQuery("SELECT status FROM dataset_transactions").
		WithArgs(datasetID, txnID).
		WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow("OPEN"))

	r := &repo.Repo{Pool: mock}
	status, found, err := r.GetTransactionStatus(ctx, datasetID, txnID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "OPEN", status)
	require.NoError(t, mock.ExpectationsWereMet())
}
