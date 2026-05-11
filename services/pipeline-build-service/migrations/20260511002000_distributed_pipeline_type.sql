-- PB.15 — Spark/Flink distributed pipeline type.
-- Public-docs parity reference: Palantir Pipeline Builder overview for graph
-- authoring and build execution semantics.

ALTER TABLE pipelines
    ADD COLUMN IF NOT EXISTS distributed_config JSONB;

ALTER TABLE pipelines
    DROP CONSTRAINT IF EXISTS pipelines_pipeline_type_chk;

ALTER TABLE pipelines
    ADD CONSTRAINT pipelines_pipeline_type_chk
    CHECK (pipeline_type IN ('BATCH', 'FASTER', 'INCREMENTAL', 'STREAMING', 'EXTERNAL', 'DISTRIBUTED'));

ALTER TABLE pipelines
    ADD CONSTRAINT pipelines_distributed_engine_chk
    CHECK (
        pipeline_type <> 'DISTRIBUTED'
        OR distributed_config IS NULL
        OR NOT (distributed_config ? 'engine')
        OR lower(distributed_config->>'engine') IN ('spark', 'pyspark', 'flink')
    );
