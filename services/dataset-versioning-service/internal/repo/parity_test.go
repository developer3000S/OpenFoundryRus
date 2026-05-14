package repo_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func TestGoMigrationsExistInEmbeddedRepo(t *testing.T) {
	migrations := migrationNames(t, "migrations")
	require.NotEmpty(t, migrations)
	require.Contains(t, migrations, "20260419100001_initial_datasets.sql")
	require.Contains(t, migrations, "20260501000001_versioning_init.sql")
	require.Contains(t, migrations, "20260501120000_dataset_rid_compat.sql")
}

func migrationNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func TestGetCatalogFacetsAggregatesTagsAndOwners(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	ownerID := uuid.New()
	mock.ExpectQuery("SELECT tag AS value").WillReturnRows(pgxmock.NewRows([]string{"value", "count"}).AddRow("finance", int64(2)).AddRow("daily", int64(1)))
	mock.ExpectQuery("SELECT owner_id, COUNT").WillReturnRows(pgxmock.NewRows([]string{"owner_id", "count"}).AddRow(ownerID, int64(3)))

	r := &repo.Repo{Pool: mock}
	facets, err := r.GetCatalogFacets(ctx)
	require.NoError(t, err)
	require.Equal(t, []models.CatalogTagFacet{{Value: "finance", Count: 2}, {Value: "daily", Count: 1}}, facets.Tags)
	require.Equal(t, []models.CatalogOwnerFacet{{OwnerID: ownerID, Count: 3}}, facets.Owners)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetInternalDatasetMetadataUsesStoragePathDirectMarkings(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	ownerID := uuid.New()
	markingID := uuid.New()
	now := time.Now().UTC()
	storagePath := "ri.foundry.main.dataset." + datasetID.String()
	rid := "ri.foundry.main.dataset." + datasetID.String()
	mock.ExpectQuery("SELECT id, rid, name, format").WithArgs(datasetID).WillReturnRows(pgxmock.NewRows([]string{"id", "rid", "name", "format", "tags", "current_version", "active_branch", "owner_id", "parent_folder_rid", "folder_path", "project_id", "project_rid", "path", "resource_visibility", "updated_at", "storage_path"}).AddRow(datasetID, rid, "orders", "parquet", []string{"finance"}, int32(7), "master", ownerID, "ri.openfoundry.main.folder.root", "/datasets", "default", "ri.openfoundry.main.project.default", "/datasets/orders", "private", now, storagePath))
	mock.ExpectQuery("SELECT marking_id FROM dataset_markings").WithArgs(storagePath).WillReturnRows(pgxmock.NewRows([]string{"marking_id"}).AddRow(markingID))

	r := &repo.Repo{Pool: mock}
	metadata, err := r.GetInternalDatasetMetadata(ctx, datasetID)
	require.NoError(t, err)
	require.Equal(t, datasetID, metadata.ID)
	require.Equal(t, []uuid.UUID{markingID}, metadata.Markings)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPatchDatasetMetadataScansCatalogDataset(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	ownerID := uuid.New()
	viewID := uuid.New()
	now := time.Now().UTC()
	name := "orders-v2"
	metadata := models.JSONValue(`{"tier":"gold"}`)

	selectRows := pgxmock.NewRows([]string{"id", "name", "description", "format", "storage_path", "size_bytes", "row_count", "owner_id", "tags", "current_version", "active_branch", "metadata", "health_status", "current_view_id", "rid", "parent_folder_rid", "folder_path", "project_id", "project_rid", "path", "resource_visibility", "deleted_at", "created_at", "updated_at"}).
		AddRow(datasetID, "orders", "desc", "parquet", "s3://lake/orders", int64(1), int64(2), ownerID, []string{"gold"}, int32(3), "master", []byte(metadata), "HEALTHY", &viewID, "ri.foundry.main.dataset."+datasetID.String(), "ri.openfoundry.main.folder.root", "/datasets", "default", "ri.openfoundry.main.project.default", "/datasets/orders", "private", nil, now, now)
	updateRows := pgxmock.NewRows([]string{"id", "name", "description", "format", "storage_path", "size_bytes", "row_count", "owner_id", "tags", "current_version", "active_branch", "metadata", "health_status", "current_view_id", "rid", "parent_folder_rid", "folder_path", "project_id", "project_rid", "path", "resource_visibility", "deleted_at", "created_at", "updated_at"}).
		AddRow(datasetID, name, "desc", "parquet", "s3://lake/orders", int64(1), int64(2), ownerID, []string{"gold"}, int32(3), "master", []byte(metadata), "HEALTHY", &viewID, "ri.foundry.main.dataset."+datasetID.String(), "ri.openfoundry.main.folder.root", "/datasets", "default", "ri.openfoundry.main.project.default", "/datasets/"+name, "private", nil, now, now)
	mock.ExpectQuery("SELECT id, name, description").WithArgs(datasetID).WillReturnRows(selectRows)
	mock.ExpectQuery("UPDATE datasets SET").WithArgs(datasetID, name, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), []byte(metadata), pgxmock.AnyArg(), pgxmock.AnyArg(), "ri.openfoundry.main.folder.root", "/datasets", "default", "ri.openfoundry.main.project.default", "/datasets/"+name, "private").WillReturnRows(updateRows)

	r := &repo.Repo{Pool: mock}
	got, err := r.PatchDatasetMetadata(ctx, datasetID, &models.DatasetMetadataPatch{Name: &name, Metadata: metadata})
	require.NoError(t, err)
	require.Equal(t, name, got.Name)
	require.JSONEq(t, string(metadata), string(got.Metadata))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceDatasetPermissionsUsesDeleteAndUpsert(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	source := "direct"
	perm := models.PutDatasetPermissionEdge{PrincipalKind: "user", PrincipalID: "u1", Role: "viewer", Actions: []string{"read"}, Source: &source}
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM dataset_permission_edges").WithArgs(datasetID).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("INSERT INTO dataset_permission_edges").WithArgs(pgxmock.AnyArg(), datasetID, perm.PrincipalKind, perm.PrincipalID, perm.Role, perm.Actions, source, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	r := &repo.Repo{Pool: mock}
	require.NoError(t, r.ReplaceDatasetPermissions(ctx, datasetID, []models.PutDatasetPermissionEdge{perm}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStartTransactionRejectsConcurrentOpen(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	branchID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(branchID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	r := &repo.Repo{Pool: mock}
	_, err = r.StartTransaction(ctx, uuid.New(), branchID, "master", models.TransactionTypeAppend, "", nil, uuid.New())
	require.ErrorIs(t, err, repo.ErrConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStartTransactionMovesBranchPointerToOpenTransaction(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	branchID := uuid.New()
	actor := uuid.New()
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(branchID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("INSERT INTO dataset_transactions").
		WithArgs(pgxmock.AnyArg(), datasetID, branchID, "main", models.TransactionTypeAppend, "work", []byte(`{}`), actor).
		WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "branch_id", "branch_name", "tx_type", "status", "summary", "metadata", "providence", "started_by", "started_at", "committed_at", "aborted_at"}).
			AddRow(uuid.New(), datasetID, branchID, "main", models.TransactionTypeAppend, models.TransactionStatusOpen, "work", []byte(`{}`), []byte(`{}`), &actor, now, nil, nil))
	mock.ExpectExec("UPDATE dataset_branches").
		WithArgs(branchID, pgxmock.AnyArg(), datasetID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	r := &repo.Repo{Pool: mock}
	tx, err := r.StartTransaction(ctx, datasetID, branchID, "main", models.TransactionTypeAppend, "work", nil, actor)
	require.NoError(t, err)
	require.Equal(t, models.TransactionStatusOpen, tx.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCommitTransactionRejectsNonOpenTransaction(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	branchID := uuid.New()
	txnID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, dataset_id, branch_id").WithArgs(txnID).WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "branch_id", "branch_name", "tx_type", "status", "summary"}).AddRow(txnID, datasetID, branchID, "master", models.TransactionTypeAppend, models.TransactionStatusCommitted, "done"))
	mock.ExpectRollback()

	r := &repo.Repo{Pool: mock}
	err = r.CommitTransaction(ctx, datasetID, txnID)
	require.ErrorIs(t, err, repo.ErrInvalidTransition)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAbortTransactionRejectsNonOpenTransaction(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	branchID := uuid.New()
	txnID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, dataset_id, branch_id").WithArgs(txnID).WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "branch_id", "branch_name", "tx_type", "status", "summary"}).AddRow(txnID, datasetID, branchID, "master", models.TransactionTypeAppend, models.TransactionStatusAborted, "cancelled"))
	mock.ExpectRollback()

	r := &repo.Repo{Pool: mock}
	err = r.AbortTransaction(ctx, datasetID, txnID)
	require.ErrorIs(t, err, repo.ErrInvalidTransition)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAbortTransactionRestoresBranchPointerToLatestNonAborted(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	branchID := uuid.New()
	txnID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, dataset_id, branch_id").
		WithArgs(txnID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "branch_id", "branch_name", "tx_type", "status", "summary"}).
			AddRow(txnID, datasetID, branchID, "main", models.TransactionTypeAppend, models.TransactionStatusOpen, "cancel"))
	mock.ExpectExec("UPDATE dataset_transactions").
		WithArgs(txnID, datasetID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE dataset_branches").
		WithArgs(branchID, datasetID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	r := &repo.Repo{Pool: mock}
	err = r.AbortTransaction(ctx, datasetID, txnID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPutViewSchemaUpsertsAndScansResponse(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	viewID := uuid.New()
	datasetID := uuid.New()
	branch := "master"
	now := time.Now().UTC()
	schema := models.DatasetSchema{Fields: []models.Field{{Name: "id", Type: models.FieldTypeString, Nullable: false}}, FileFormat: models.FileFormatParquet}
	schemaJSON, err := models.MarshalJSONValue(schema)
	require.NoError(t, err)
	rows := pgxmock.NewRows([]string{"view_id", "dataset_id", "branch", "schema_json", "content_hash", "created_at", "unchanged"}).
		AddRow(viewID, datasetID, &branch, []byte(schemaJSON), "hash", now, false)
	mock.ExpectQuery("INSERT INTO dataset_view_schemas").WithArgs(viewID, datasetID, &branch, schemaJSON, "hash").WillReturnRows(rows)

	r := &repo.Repo{Pool: mock}
	got, err := r.PutViewSchema(ctx, viewID, datasetID, &branch, schema, "hash")
	require.NoError(t, err)
	require.Equal(t, schema.FileFormat, got.Schema.FileFormat)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStorageDetailsAggregatesSoftDeleteBuckets(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(CASE WHEN deleted_at IS NULL").WithArgs(datasetID).
		WillReturnRows(pgxmock.NewRows([]string{"total_active_bytes", "total_active_files", "total_deleted_bytes", "total_deleted_files"}).AddRow(int64(42), int64(1), int64(9), int64(2)))

	r := &repo.Repo{Pool: mock}
	got, err := r.StorageDetails(ctx, datasetID, "local", "local", "/tmp", 900)
	require.NoError(t, err)
	require.Equal(t, int64(42), got.TotalActiveBytes)
	require.Equal(t, int64(2), got.TotalDeletedFiles)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArchiveBranchForRetentionUsesSoftArchive(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	branchID := uuid.New()
	grace := time.Now().UTC().Add(7 * 24 * time.Hour)
	mock.ExpectExec("UPDATE dataset_branches SET archived_at").WithArgs(branchID, grace).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	r := &repo.Repo{Pool: mock}
	require.NoError(t, r.ArchiveBranchForRetention(ctx, branchID, grace))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDatasetHealthDecodesNullPctJSON(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT dataset_rid, dataset_id").WithArgs("ri.dataset").WillReturnRows(
		pgxmock.NewRows([]string{"dataset_rid", "dataset_id", "row_count", "col_count", "null_pct_by_column", "freshness_seconds", "last_commit_at", "txn_failure_rate_24h", "last_build_status", "schema_drift_flag", "extras", "last_computed_at"}).
			AddRow("ri.dataset", &datasetID, int64(10), int32(2), []byte(`{"a":0.5}`), int64(1), nil, 0.1, "SUCCESS", false, []byte(`{"x":1}`), now))

	r := &repo.Repo{Pool: mock}
	got, err := r.GetDatasetHealth(ctx, "ri.dataset")
	require.NoError(t, err)
	require.Equal(t, 0.5, got.NullPctByColumn["a"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStartTransactionPropagatesInsertError(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	branchID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(branchID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("INSERT INTO dataset_transactions").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), branchID, "master", models.TransactionTypeAppend, "", []byte(`{}`), pgxmock.AnyArg()).WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	r := &repo.Repo{Pool: mock}
	_, err = r.StartTransaction(ctx, uuid.New(), branchID, "master", models.TransactionTypeAppend, "", nil, uuid.New())
	require.ErrorContains(t, err, "insert failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCompareBranchesLoadsLCACommitSummariesAndConflicts(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	baseID := uuid.New()
	compareID := uuid.New()
	now := time.Now().UTC()
	baseHead := uuid.New()
	compareHead := uuid.New()
	branchCols := []string{"id", "rid", "dataset_id", "dataset_rid", "name", "parent_branch_id", "head_transaction_id", "created_from_transaction_id", "last_activity_at", "labels", "fallback_chain", "created_at", "updated_at"}
	mock.ExpectQuery("FROM dataset_branches WHERE dataset_id").WithArgs(datasetID, "base").WillReturnRows(
		pgxmock.NewRows(branchCols).AddRow(baseID, "ri.branch.base", datasetID, "ri.dataset", "base", nil, &baseHead, nil, now, []byte(`{}`), []string{}, now, now))
	mock.ExpectQuery("FROM dataset_branches WHERE dataset_id").WithArgs(datasetID, "feature").WillReturnRows(
		pgxmock.NewRows(branchCols).AddRow(compareID, "ri.branch.feature", datasetID, "ri.dataset", "feature", &baseID, &compareHead, nil, now, []byte(`{}`), []string{}, now, now))
	lca := "ri.branch.base"
	mock.ExpectQuery("WITH RECURSIVE base_chain").WithArgs(baseID, compareID).WillReturnRows(pgxmock.NewRows([]string{"rid"}).AddRow(lca))
	mock.ExpectQuery("LEFT JOIN dataset_transaction_files").WithArgs(baseID, "base", 200).WillReturnRows(
		pgxmock.NewRows([]string{"transaction_rid", "id", "branch", "tx_type", "status", "committed_at", "files_changed"}).AddRow("ri.tx.a", uuid.New(), "base", "APPEND", "COMMITTED", &now, 1))
	mock.ExpectQuery("LEFT JOIN dataset_transaction_files").WithArgs(compareID, "feature", 200).WillReturnRows(
		pgxmock.NewRows([]string{"transaction_rid", "id", "branch", "tx_type", "status", "committed_at", "files_changed"}).AddRow("ri.tx.b", uuid.New(), "feature", "UPDATE", "COMMITTED", &now, 2))
	hashA := "a"
	hashB := "a"
	mock.ExpectQuery("WITH a AS").WithArgs(baseID, compareID).WillReturnRows(
		pgxmock.NewRows([]string{"logical_path", "a_transaction_rid", "b_transaction_rid", "content_hash_a", "content_hash_b"}).AddRow("orders.parquet", "ri.tx.a", "ri.tx.b", &hashA, &hashB))

	r := &repo.Repo{Pool: mock}
	got, err := r.CompareBranches(ctx, datasetID, "base", "feature")
	require.NoError(t, err)
	require.Equal(t, &lca, got.LCABranchRID)
	require.Len(t, got.AOnlyTransactions, 1)
	require.Len(t, got.ConflictingFiles, 1)
	require.Equal(t, hashA, *got.ConflictingFiles[0].ContentHashA)
	require.Equal(t, hashB, *got.ConflictingFiles[0].ContentHashB)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBranchAncestryWalksChildToRoot(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	rootID := uuid.New()
	childID := uuid.New()
	now := time.Now().UTC()
	branchCols := []string{"id", "rid", "dataset_id", "dataset_rid", "name", "parent_branch_id", "head_transaction_id", "created_from_transaction_id", "last_activity_at", "labels", "fallback_chain", "created_at", "updated_at"}
	mock.ExpectQuery("WITH RECURSIVE ancestry").WithArgs(datasetID, "feature").WillReturnRows(
		pgxmock.NewRows(branchCols).
			AddRow(childID, "ri.foundry.main.branch.feature", datasetID, "ri.dataset", "feature", &rootID, nil, nil, now, []byte(`{}`), []string{}, now, now).
			AddRow(rootID, "ri.foundry.main.branch.master", datasetID, "ri.dataset", "master", nil, nil, nil, now, []byte(`{}`), []string{}, now, now))

	r := &repo.Repo{Pool: mock}
	got, err := r.BranchAncestry(ctx, datasetID, "feature")
	require.NoError(t, err)
	require.Equal(t, []string{"feature", "master"}, []string{got[0].Name, got[1].Name})
	require.Nil(t, got[1].ParentBranchID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceFallbacksNormalizesPersistsAndRejectsCycles(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	branchID := uuid.New()
	mock.ExpectQuery("SELECT dataset_id, name FROM dataset_branches").WithArgs(branchID).WillReturnRows(pgxmock.NewRows([]string{"dataset_id", "name"}).AddRow(datasetID, "feature"))
	mock.ExpectQuery("WITH RECURSIVE fallback_walk").WithArgs(datasetID, "feature", []string{"master"}).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM dataset_branch_fallbacks").WithArgs(branchID).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec("INSERT INTO dataset_branch_fallbacks").WithArgs(branchID, int32(0), "master").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE dataset_branches SET fallback_chain").WithArgs(branchID, []string{"master"}).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	r := &repo.Repo{Pool: mock}
	require.NoError(t, r.ReplaceFallbacks(ctx, branchID, []string{" master "}))
	require.NoError(t, mock.ExpectationsWereMet())

	mock2, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock2.Close()
	mock2.ExpectQuery("SELECT dataset_id, name FROM dataset_branches").WithArgs(branchID).WillReturnRows(pgxmock.NewRows([]string{"dataset_id", "name"}).AddRow(datasetID, "feature"))
	mock2.ExpectQuery("WITH RECURSIVE fallback_walk").WithArgs(datasetID, "feature", []string{"develop"}).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

	r = &repo.Repo{Pool: mock2}
	err = r.ReplaceFallbacks(ctx, branchID, []string{"develop"})
	require.ErrorIs(t, err, repo.ErrValidation)
	require.NoError(t, mock2.ExpectationsWereMet())
}

func TestGetDatasetQualityUsesRustProfileTablesAndLoadsChildren(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	datasetID := uuid.New()
	now := time.Now().UTC()
	score := 0.99
	profile := []byte(`{"row_count":10,"column_count":2,"duplicate_rows":0,"completeness_ratio":1,"uniqueness_ratio":1,"generated_at":"` + now.Format(time.RFC3339Nano) + `","columns":[],"rule_results":[]}`)
	mock.ExpectQuery("SELECT profile, score, profiled_at FROM dataset_profiles").WithArgs(datasetID).WillReturnRows(pgxmock.NewRows([]string{"profile", "score", "profiled_at"}).AddRow(profile, &score, &now))
	mock.ExpectQuery("FROM dataset_quality_history").WithArgs(datasetID, 20).WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "score", "passed_rules", "failed_rules", "alerts_count", "created_at"}).AddRow(uuid.New(), datasetID, 0.99, int32(1), int32(0), int32(0), now))
	mock.ExpectQuery("FROM dataset_quality_alerts").WithArgs(datasetID, pgxmock.AnyArg(), 100).WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "level", "kind", "message", "status", "details", "created_at", "resolved_at"}))
	mock.ExpectQuery("FROM dataset_quality_rules").WithArgs(datasetID).WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "name", "rule_type", "severity", "config", "enabled", "created_at", "updated_at"}))

	r := &repo.Repo{Pool: mock}
	got, err := r.GetDatasetQuality(ctx, datasetID)
	require.NoError(t, err)
	require.NotNil(t, got.Profile)
	require.Equal(t, int64(10), got.Profile.RowCount)
	require.Len(t, got.History, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestArchiveBranchForRetentionWithOutboxReparentsArchivesAndEmits(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	branchID := uuid.New()
	parentID := uuid.New()
	grace := time.Now().UTC().Add(7 * 24 * time.Hour)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE dataset_branches SET parent_branch_id").WithArgs(branchID, &parentID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE dataset_branches SET archived_at").WithArgs(branchID, grace).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO outbox.events").WithArgs(pgxmock.AnyArg(), branchID.String(), []byte(`{"reason":"ttl"}`)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	r := &repo.Repo{Pool: mock}
	archived, err := r.ArchiveBranchForRetentionWithOutbox(ctx, models.RetentionRow{ID: branchID, ParentBranchID: &parentID}, grace, models.JSONValue(`{"reason":"ttl"}`))
	require.NoError(t, err)
	require.True(t, archived)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListRuntimeTransactionsAppliesRustFilters(t *testing.T) {
	ctx := context.Background()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	datasetID := uuid.New()
	branch := "master"
	before := time.Now().UTC()
	started := before.Add(-time.Hour)
	txnID := uuid.New()
	branchID := uuid.New()
	startedBy := uuid.New()
	mock.ExpectQuery("SELECT id, dataset_id, branch_id, branch_name, tx_type, status, summary, metadata, providence, started_by, started_at, committed_at, aborted_at FROM dataset_transactions").
		WithArgs(datasetID, &branch, &before, 200).
		WillReturnRows(pgxmock.NewRows([]string{"id", "dataset_id", "branch_id", "branch_name", "tx_type", "status", "summary", "metadata", "providence", "started_by", "started_at", "committed_at", "aborted_at"}).
			AddRow(txnID, datasetID, branchID, branch, models.TransactionTypeAppend, models.TransactionStatusOpen, "load", []byte(`{}`), []byte(`{"source":"test"}`), &startedBy, started, nil, nil))

	r := &repo.Repo{Pool: mock}
	got, err := r.ListRuntimeTransactions(ctx, datasetID, &branch, &before, 200)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, txnID, got[0].ID)
	require.Equal(t, models.TransactionStatusOpen, got[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}
