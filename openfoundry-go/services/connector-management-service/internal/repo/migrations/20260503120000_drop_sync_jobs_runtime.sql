-- S6.6: connector-management-service owns connector definitions only.
-- Sync execution state moved to ingestion-replication-service's Kubernetes-
-- native control plane and its non-Postgres runtime stores.
DROP TABLE IF EXISTS sync_jobs;
