-- DF.4 transaction lifecycle contract.
--
-- The application service owns validation for OPEN -> COMMITTED and
-- OPEN -> ABORTED. These trigger predicates make the derived projections
-- equally strict: staged files and schemas are only preserved when a
-- transaction closes from OPEN to COMMITTED. ABORTED transactions therefore
-- remain in dataset_transaction_files for audit/retention visibility but do
-- not enter latest dataset views or the public dataset_files projection.

DROP TRIGGER IF EXISTS trg_dataset_files_from_committed_txn
    ON dataset_transactions;

CREATE TRIGGER trg_dataset_files_from_committed_txn
    AFTER UPDATE OF status ON dataset_transactions
    FOR EACH ROW
    WHEN (OLD.status = 'OPEN' AND NEW.status = 'COMMITTED')
    EXECUTE FUNCTION fn_dataset_files_from_committed_txn();

DROP TRIGGER IF EXISTS trg_dataset_view_schemas_from_txn
    ON dataset_transactions;

CREATE TRIGGER trg_dataset_view_schemas_from_txn
    AFTER UPDATE OF status ON dataset_transactions
    FOR EACH ROW
    WHEN (OLD.status = 'OPEN' AND NEW.status = 'COMMITTED')
    EXECUTE FUNCTION fn_dataset_view_schemas_from_txn();
