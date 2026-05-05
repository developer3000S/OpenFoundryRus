//! Shared harness for `retention-policy-service` integration tests.
//!
//! The harness boots a Postgres container, applies BOTH the retention
//! migrations AND the dataset-versioning ones (they share the dev DB,
//! so the preview path can SELECT from `dataset_transactions` /
//! `dataset_files`). Tests that don't care about the cross-service
//! tables can ignore the extra migrations.

#![allow(dead_code)]

use std::sync::Arc;

use auth_middleware::jwt::JwtConfig;
use axum::Router;
use audit_compliance_service::retention_policy::{AppState, build_router};
use sqlx::PgPool;
use tempfile::TempDir;
use testcontainers::{ContainerAsync, GenericImage};
use testing::{containers::boot_postgres, fixtures};

pub struct Harness {
    pub container: ContainerAsync<GenericImage>,
    pub pool: PgPool,
    pub router: Router,
    pub jwt_config: JwtConfig,
    pub token: String,
    pub _scratch: TempDir,
}

pub async fn spawn() -> Harness {
    let (container, pool, _url) = boot_postgres().await;

    // P4 — apply BOTH migration sets. Retention's preview SQL queries
    // `datasets`, `dataset_transactions` and `dataset_files`, all
    // owned by `dataset-versioning-service`. Sharing one Postgres for
    // tests mirrors the dev compose stack.
    //
    // sqlx's `_sqlx_migrations` table is shared across calls; the
    // first set we apply is then "missing" from the migrator we run
    // second. `set_ignore_missing(true)` is the documented escape
    // hatch (sqlx 0.8) for that exact case.
    let mut retention_migrator = sqlx::migrate!("./migrations");
    retention_migrator.set_ignore_missing(true);
    retention_migrator
        .run(&pool)
        .await
        .expect("apply retention-policy-service migrations");

    let mut dvs_migrator = sqlx::migrate!("../dataset-versioning-service/migrations");
    dvs_migrator.set_ignore_missing(true);
    dvs_migrator
        .run(&pool)
        .await
        .expect("apply dataset-versioning migrations");

    let jwt_config = fixtures::jwt_config();
    let token = fixtures::dev_token(
        &jwt_config,
        vec!["retention.read".into(), "retention.write".into()],
    );

    let state = AppState {
        db: pool.clone(),
        jwt_config: Arc::new(jwt_config.clone()),
    };
    let router = build_router(state);

    let scratch = tempfile::tempdir().expect("scratch");
    Harness {
        container,
        pool,
        router,
        jwt_config,
        token,
        _scratch: scratch,
    }
}

/// Insert a dataset row + master branch in the DVS-owned tables.
/// Mirrors `dataset-versioning-service`'s test helper but lives here
/// so the retention tests can stay self-contained.
pub async fn seed_dataset_with_master(pool: &PgPool, rid: &str) -> uuid::Uuid {
    let id = fixtures::seed_dataset(pool, rid, rid, "parquet").await;
    let branch_id = uuid::Uuid::now_v7();
    sqlx::query(
        r#"INSERT INTO dataset_branches
              (id, dataset_id, dataset_rid, name, parent_branch_id, head_transaction_id, is_default)
           VALUES ($1, $2, $3, 'master', NULL, NULL, TRUE)"#,
    )
    .bind(branch_id)
    .bind(id)
    .bind(rid)
    .execute(pool)
    .await
    .expect("seed master branch");
    id
}

/// Insert a retention policy row directly. Useful when a test wants
/// to skip the HTTP CRUD path and exercise the resolver directly.
pub async fn seed_policy(
    pool: &PgPool,
    name: &str,
    target_kind: &str,
    retention_days: i32,
    selector: serde_json::Value,
    criteria: serde_json::Value,
) -> uuid::Uuid {
    let id = uuid::Uuid::now_v7();
    sqlx::query(
        r#"INSERT INTO retention_policies (
                id, name, scope, target_kind, retention_days,
                legal_hold, purge_mode, rules, updated_by, active,
                is_system, selector, criteria, grace_period_minutes
           ) VALUES ($1, $2, '', $3, $4,
                     FALSE, 'hard-delete-after-ttl', '[]'::jsonb,
                     'test', TRUE, FALSE, $5::jsonb, $6::jsonb, 60)"#,
    )
    .bind(id)
    .bind(name)
    .bind(target_kind)
    .bind(retention_days)
    .bind(selector)
    .bind(criteria)
    .execute(pool)
    .await
    .expect("seed policy");
    id
}
