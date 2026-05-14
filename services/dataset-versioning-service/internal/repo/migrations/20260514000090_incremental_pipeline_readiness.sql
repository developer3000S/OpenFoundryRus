-- DF.18 incremental pipeline readiness.
--
-- The readiness endpoint reconstructs committed branch history and needs
-- committed transactions in stable chronological order plus per-transaction
-- file counts.

CREATE INDEX IF NOT EXISTS idx_dataset_transactions_incremental_readiness
    ON dataset_transactions(dataset_id, branch_name, status, COALESCE(committed_at, started_at), started_at, id);

CREATE INDEX IF NOT EXISTS idx_dataset_transaction_files_readiness
    ON dataset_transaction_files(transaction_id);
