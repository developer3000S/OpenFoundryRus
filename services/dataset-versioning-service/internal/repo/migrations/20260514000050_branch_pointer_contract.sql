-- DF.6 branch pointer contract.
--
-- Branch.transactionRid mirrors Foundry's public Branch object: the pointer
-- is the most recent OPEN or COMMITTED transaction on the branch, never an
-- ABORTED transaction. The application updates this pointer when opening,
-- committing, and aborting transactions; this migration backfills existing
-- rows and repairs any branch that still points at an aborted transaction.

UPDATE dataset_branches b
   SET head_transaction_id = latest.id,
       updated_at = NOW()
  FROM LATERAL (
        SELECT t.id
          FROM dataset_transactions t
         WHERE t.branch_id = b.id
           AND t.status IN ('OPEN', 'COMMITTED')
         ORDER BY COALESCE(t.committed_at, t.started_at) DESC, t.started_at DESC
         LIMIT 1
       ) latest
 WHERE b.deleted_at IS NULL
   AND b.archived_at IS NULL
   AND (b.head_transaction_id IS DISTINCT FROM latest.id);

UPDATE dataset_branches b
   SET head_transaction_id = NULL,
       updated_at = NOW()
 WHERE b.head_transaction_id IS NOT NULL
   AND NOT EXISTS (
       SELECT 1
         FROM dataset_transactions t
        WHERE t.id = b.head_transaction_id
          AND t.branch_id = b.id
          AND t.status IN ('OPEN', 'COMMITTED')
   );

CREATE INDEX IF NOT EXISTS idx_dataset_branches_head_transaction
    ON dataset_branches(head_transaction_id)
    WHERE head_transaction_id IS NOT NULL AND deleted_at IS NULL;
