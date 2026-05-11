-- H4 — Foundry-style branching for media sets.
--
-- ## Why this number
--
-- The H4 spec asked for `0004_branching.sql`, but the migration tree
-- already has `0004_item_markings.sql` (H3 — granular Cedar) and
-- `0005_outbox.sql` (H3 closure — ADR-0022). Renumbering here so the
-- branching migration runs after both is the only change vs the spec;
-- the resulting schema is identical.
--
-- ## What this migration adds
--
-- 1. `media_set_branches` gains the parent / head-transaction / actor
--    fields described in `Core concepts/Branching.md` ("Every non-root
--    branch has exactly one parent branch") plus a generated
--    `branch_rid` so neighbours can address a branch by RID alone
--    (mirrors `dataset_branches.rid` from
--    `services/dataset-versioning-service/migrations/`).
-- 2. `media_items` gains FKs to the parent branch + transaction rows.
--    The `transaction_rid` column was previously a free-form `''`
--    placeholder for transactionless writes; we relax it to NULL so
--    the FK can be enforced (transactionless items get NULL) and
--    backfill the existing rows in the same migration.
--
-- The `(media_set_rid, branch_name)` PK already enforces the unique
-- index the H4 spec calls for; we keep it as the authoritative
-- composite key and add `branch_rid` as a stable secondary identifier.

-- ── 1. media_set_branches: branch_rid + parent + head + actor ────────

ALTER TABLE media_set_branches
    ADD COLUMN IF NOT EXISTS branch_rid TEXT
        GENERATED ALWAYS AS (
            'ri.foundry.main.media_branch.' || md5(media_set_rid || ':' || branch_name)
        ) STORED;

-- A separate UNIQUE constraint on the generated column lets `media_items`
-- (and `media_set_branches.parent_branch_rid`) reference it via FK.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'media_set_branches_branch_rid_key'
  ) THEN
    ALTER TABLE media_set_branches
      ADD CONSTRAINT media_set_branches_branch_rid_key UNIQUE (branch_rid);
  END IF;
END $$;

-- Parent branch RID. Self-referential so deleting an intermediate
-- branch can re-parent its children under the deleted branch's parent
-- (per the Foundry guarantee in `Core concepts/Branching.md`). NULL =
-- root branch.
ALTER TABLE media_set_branches
    ADD COLUMN IF NOT EXISTS parent_branch_rid TEXT
        REFERENCES media_set_branches(branch_rid) ON DELETE SET NULL;

-- Head transaction pointer (Git-like "tip"). NULL until the first
-- commit lands on the branch. References `media_set_transactions(rid)`
-- so dropping a transaction nulls the head; the application is
-- responsible for advancing the head on every commit.
ALTER TABLE media_set_branches
    ADD COLUMN IF NOT EXISTS head_transaction_rid TEXT
        REFERENCES media_set_transactions(rid) ON DELETE SET NULL;

-- Actor who minted the branch.
ALTER TABLE media_set_branches
    ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';

-- Backfill `parent_branch_rid` from the legacy `parent_branch` text
-- column where it was populated. The pre-H4 column held only the
-- branch *name*; we re-derive the RID from `(media_set_rid, name)`.
-- Only run the UPDATE if the legacy column still exists — when the
-- migration is re-applied after a partial failure the column is
-- already gone and the JOIN clause must not reference it.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name   = 'media_set_branches'
      AND column_name  = 'parent_branch'
  ) THEN
    EXECUTE $sql$
      UPDATE media_set_branches AS child
         SET parent_branch_rid = parent.branch_rid
        FROM media_set_branches AS parent
       WHERE child.media_set_rid = parent.media_set_rid
         AND child.parent_branch = parent.branch_name
         AND child.parent_branch IS NOT NULL
         AND child.parent_branch_rid IS NULL
    $sql$;
  END IF;
END $$;

-- Drop the legacy column now that the data has migrated. Keeps the
-- table schema lean — every consumer reads `parent_branch_rid` going
-- forward.
ALTER TABLE media_set_branches DROP COLUMN IF EXISTS parent_branch;

CREATE INDEX IF NOT EXISTS idx_media_set_branches_parent
    ON media_set_branches(parent_branch_rid)
    WHERE parent_branch_rid IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_media_set_branches_head
    ON media_set_branches(head_transaction_rid)
    WHERE head_transaction_rid IS NOT NULL;

-- ── 2. media_set_transactions: write mode for replace/modify ─────────
--
-- Per `Incremental media sets.md` ("Incremental write modes"), a
-- transactional commit can run in either `MODIFY` (append + dedup)
-- or `REPLACE` (surface only the items written in the transaction)
-- mode. The mode is decided at `OpenTransaction` time and locked in
-- so the `Commit` handler honours the original intent.
ALTER TABLE media_set_transactions
    ADD COLUMN IF NOT EXISTS write_mode TEXT NOT NULL DEFAULT 'MODIFY'
        CHECK (write_mode IN ('MODIFY', 'REPLACE'));

-- ── 3. media_items: branch + transaction FKs ─────────────────────────

-- Make `transaction_rid` nullable so the FK can fire. Transactionless
-- items now carry NULL instead of an empty string; the application
-- treats both forms identically when reading legacy rows.
ALTER TABLE media_items
    ALTER COLUMN transaction_rid DROP DEFAULT;
ALTER TABLE media_items
    ALTER COLUMN transaction_rid DROP NOT NULL;
UPDATE media_items
   SET transaction_rid = NULL
 WHERE transaction_rid = '';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'media_items_transaction_rid_fkey'
  ) THEN
    ALTER TABLE media_items
      ADD CONSTRAINT media_items_transaction_rid_fkey
          FOREIGN KEY (transaction_rid)
          REFERENCES media_set_transactions(rid)
          ON DELETE SET NULL;
  END IF;
END $$;

-- New `branch_rid` column references the generated branch identifier.
-- Backfilled from `(media_set_rid, branch)` for every existing row;
-- the application keeps both fields in sync going forward (writes go
-- through the same insert helper that fills `branch_rid` from the
-- normalised branch name).
ALTER TABLE media_items
    ADD COLUMN IF NOT EXISTS branch_rid TEXT;

UPDATE media_items AS i
   SET branch_rid = b.branch_rid
  FROM media_set_branches AS b
 WHERE i.media_set_rid = b.media_set_rid
   AND i.branch = b.branch_name
   AND i.branch_rid IS NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'media_items_branch_rid_fkey'
  ) THEN
    ALTER TABLE media_items
      ADD CONSTRAINT media_items_branch_rid_fkey
          FOREIGN KEY (branch_rid)
          REFERENCES media_set_branches(branch_rid)
          ON DELETE CASCADE;
  END IF;
END $$;

-- The denormalised `(media_set_rid, branch)` columns stay around so
-- the partial-unique path-dedup index keeps working unchanged. The FK
-- on `branch_rid` is the new authoritative reference.
CREATE INDEX IF NOT EXISTS idx_media_items_branch_rid
    ON media_items(branch_rid)
    WHERE branch_rid IS NOT NULL;
