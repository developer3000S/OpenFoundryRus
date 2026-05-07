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

func TestListFilesScansAndFiltersLatestLogicalPath(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	txnID := uuid.New()
	olderID := uuid.New()
	newerID := uuid.New()
	now := time.Now().UTC()
	sha := "abc123"

	rows := pgxmock.NewRows([]string{
		"id", "dataset_id", "transaction_id", "logical_path", "physical_uri",
		"size_bytes", "sha256", "created_at", "modified_at", "deleted_at", "status",
	}).
		AddRow(newerID, datasetID, txnID, "daily/part-000.parquet", "local:///new.parquet", int64(42), &sha, now, now, nil, "active").
		AddRow(olderID, datasetID, txnID, "daily/part-000.parquet", "local:///old.parquet", int64(41), nil, now.Add(-time.Minute), now.Add(-time.Minute), nil, "active")

	mock.ExpectQuery("SELECT df.id").
		WithArgs(datasetID, "main", "daily/").
		WillReturnRows(rows)

	r := &repo.Repo{Pool: mock}
	files, err := r.ListFiles(ctx, datasetID, "main", "daily/")
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, newerID, files[0].ID)
	require.Equal(t, "daily/part-000.parquet", files[0].LogicalPath)
	require.Equal(t, "active", files[0].Status)
	require.NotNil(t, files[0].SHA256)
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
