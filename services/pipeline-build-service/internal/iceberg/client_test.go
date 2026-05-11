package iceberg

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
)

const icebergRID = DatasetRIDPrefix + "00000000-0000-0000-0000-000000000001"

func TestOutputClientCommitOK(t *testing.T) {
	store := newFakeStore(staged("txn-1", []map[string]any{{"id": "row-1", "count": 1}}))
	table := &fakeTable{}
	client := NewOutputClient(store, &fakeCatalog{table: table})

	if err := client.Commit(context.Background(), outTxn("txn-1"), executor.NodeResult{}); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if len(table.appends) != 1 || table.appends[0].TransactionRID != "txn-1" {
		t.Fatalf("appends = %#v", table.appends)
	}
	if !store.committed["txn-1"] {
		t.Fatalf("transaction was not marked committed")
	}
}

func TestOutputClientTableMissing(t *testing.T) {
	client := NewOutputClient(newFakeStore(staged("txn-missing", []map[string]any{{"id": "row-1", "count": 1}})), &fakeCatalog{err: ErrTableNotFound})
	if err := client.Commit(context.Background(), outTxn("txn-missing"), executor.NodeResult{}); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("Commit() error = %v", err)
	}
}

func TestOutputClientSchemaMismatch(t *testing.T) {
	client := NewOutputClient(newFakeStore(staged("txn-schema", []map[string]any{{"id": 123, "count": 1}})), &fakeCatalog{table: &fakeTable{}})
	if err := client.Commit(context.Background(), outTxn("txn-schema"), executor.NodeResult{}); !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("Commit() error = %v", err)
	}
}

func TestExecutorPartialMultiOutputCommitFailureRollsBackCommittedOutput(t *testing.T) {
	store := newFakeStore(
		stagedFor(icebergRID, "txn-ok", []map[string]any{{"id": "row-1", "count": 1}}),
		stagedFor(DatasetRIDPrefix+"00000000-0000-0000-0000-000000000002", "txn-bad", []map[string]any{{"id": "row-2", "count": 2}}),
	)
	table := &fakeTable{appendErrByTxn: map[string]error{"txn-bad": ErrCommitFailed}}
	client := NewOutputClient(store, &fakeCatalog{table: table})
	plan := executor.Plan{BuildID: mustUUID(), Nodes: []executor.Node{{ID: "multi", Outputs: []executor.OutputTransaction{outTxn("txn-ok"), {DatasetRID: DatasetRIDPrefix + "00000000-0000-0000-0000-000000000002", TransactionRID: "txn-bad"}}}}}

	outcome, err := executor.Execute(context.Background(), plan, okRunner{}, client, client, nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if outcome.FinalState == "BUILD_COMPLETED" {
		t.Fatalf("expected failed outcome, got %#v", outcome)
	}
	if !store.aborted["txn-ok"] || !store.aborted["txn-bad"] {
		t.Fatalf("expected both committed and failed transactions to be aborted, aborted=%#v", store.aborted)
	}
	if len(table.rollbacks) != 2 {
		t.Fatalf("rollbacks = %#v", table.rollbacks)
	}
}

func TestOutputClientRollbackFailure(t *testing.T) {
	store := newFakeStore(staged("txn-rollback", []map[string]any{{"id": "row-1", "count": 1}}))
	client := NewOutputClient(store, &fakeCatalog{table: &fakeTable{rollbackErrByTxn: map[string]error{"txn-rollback": ErrRollbackFailed}}})
	if err := client.Abort(context.Background(), outTxn("txn-rollback")); !errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("Abort() error = %v", err)
	}
	if store.aborted["txn-rollback"] {
		t.Fatalf("rollback failure must not mark transaction aborted")
	}
}

type okRunner struct{}

func (okRunner) Run(context.Context, executor.NodeContext) (executor.NodeResult, error) {
	return executor.NodeResult{OutputContentHash: "ok"}, nil
}

type fakeStore struct {
	byTxn     map[string]*StagedTransaction
	committed map[string]bool
	aborted   map[string]bool
}

func newFakeStore(staged ...*StagedTransaction) *fakeStore {
	f := &fakeStore{byTxn: map[string]*StagedTransaction{}, committed: map[string]bool{}, aborted: map[string]bool{}}
	for _, s := range staged {
		f.byTxn[s.TransactionRID] = s
	}
	return f
}

func (f *fakeStore) LoadTransaction(_ context.Context, tx executor.OutputTransaction) (*StagedTransaction, error) {
	return f.byTxn[tx.TransactionRID], nil
}
func (f *fakeStore) MarkCommitted(_ context.Context, tx executor.OutputTransaction) error {
	f.committed[tx.TransactionRID] = true
	return nil
}
func (f *fakeStore) MarkAborted(_ context.Context, tx executor.OutputTransaction) error {
	f.aborted[tx.TransactionRID] = true
	return nil
}

type fakeCatalog struct {
	table TableWriter
	err   error
}

func (f *fakeCatalog) LoadTable(_ context.Context, _ TableSpec) (TableWriter, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.table, nil
}

type fakeTable struct {
	appends          []AppendBatch
	rollbacks        []RollbackRequest
	appendErrByTxn   map[string]error
	rollbackErrByTxn map[string]error
}

func (f *fakeTable) Append(_ context.Context, batch AppendBatch) error {
	f.appends = append(f.appends, batch)
	if err := f.appendErrByTxn[batch.TransactionRID]; err != nil {
		return err
	}
	return nil
}
func (f *fakeTable) Rollback(_ context.Context, req RollbackRequest) error {
	f.rollbacks = append(f.rollbacks, req)
	if err := f.rollbackErrByTxn[req.TransactionRID]; err != nil {
		return err
	}
	return nil
}

func staged(txn string, rows []map[string]any) *StagedTransaction {
	return stagedFor(icebergRID, txn, rows)
}

func stagedFor(datasetRID, txn string, rows []map[string]any) *StagedTransaction {
	return &StagedTransaction{DatasetRID: datasetRID, TransactionRID: txn, Spec: TableSpec{Catalog: "lakekeeper", Namespace: "of_pipeline", Table: "outputs", Schema: []FieldSpec{{ID: 1, Name: "id", Type: "string", Required: true}, {ID: 2, Name: "count", Type: "int64", Required: true}}}, Rows: rows}
}

func outTxn(txn string) executor.OutputTransaction {
	return executor.OutputTransaction{DatasetRID: icebergRID, TransactionRID: txn}
}

func mustUUID() uuid.UUID { return uuid.UUID{1} }
