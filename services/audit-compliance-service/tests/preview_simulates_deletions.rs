//! P4 — retention-preview simulates which transactions / files would
//! be purged if the runner fired now (or in `as_of_days` days).
//!
//! Seeds:
//!   * Two committed transactions: one ABORTED (eligible for the
//!     DELETE_ABORTED_TRANSACTIONS system policy after grace) and one
//!     COMMITTED (untouched by the system policy).
//!   * The ABORTED one has a single dataset_files row carrying real
//!     bytes, so the preview can count its size.
//!
//! Asserts that:
//!   * The ABORTED txn surfaces with `would_delete=true` and the
//!     system policy as the reason.
//!   * The COMMITTED txn is included with `would_delete=false`.
//!   * The files block aggregates the bytes of the purge-bound txn.
//!
//! Docker-gated.

mod common;

use axum::body::{Body, to_bytes};
use axum::http::Request;
use serde_json::Value;
use tower::ServiceExt;
use uuid::Uuid;

async fn seed_committed_transaction_with_file(
    pool: &sqlx::PgPool,
    dataset_id: Uuid,
    branch_id: Uuid,
    tx_type: &str,
    final_status: &str,
    file: Option<(&str, i64)>,
) -> Uuid {
    let txn_id = Uuid::now_v7();
    sqlx::query(
        r#"INSERT INTO dataset_transactions
              (id, dataset_id, branch_id, branch_name, tx_type, status,
               summary, started_at)
           VALUES ($1, $2, $3, 'master', $4, 'OPEN', '', NOW())"#,
    )
    .bind(txn_id)
    .bind(dataset_id)
    .bind(branch_id)
    .bind(tx_type)
    .execute(pool)
    .await
    .unwrap();
    if let Some((logical, size)) = file {
        let physical = format!("foundry/datasets/{txn_id}/{logical}");
        sqlx::query(
            r#"INSERT INTO dataset_transaction_files
                  (transaction_id, logical_path, physical_path, size_bytes, op)
               VALUES ($1, $2, $3, $4, 'ADD')"#,
        )
        .bind(txn_id)
        .bind(logical)
        .bind(physical)
        .bind(size)
        .execute(pool)
        .await
        .unwrap();
    }
    let column = match final_status {
        "ABORTED" => "aborted_at",
        _ => "committed_at",
    };
    let sql =
        format!("UPDATE dataset_transactions SET status = $2, {column} = NOW() WHERE id = $1");
    sqlx::query(&sql)
        .bind(txn_id)
        .bind(final_status)
        .execute(pool)
        .await
        .unwrap();
    txn_id
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "requires Docker; run with --include-ignored"]
async fn preview_marks_aborted_transactions_for_deletion_via_system_policy() {
    let h = common::spawn().await;
    let dataset_rid = "ri.foundry.main.dataset.preview";
    let dataset_id = common::seed_dataset_with_master(&h.pool, dataset_rid).await;
    let branch_id = sqlx::query_scalar::<_, Uuid>(
        "SELECT id FROM dataset_branches WHERE dataset_id = $1 AND name = 'master'",
    )
    .bind(dataset_id)
    .fetch_one(&h.pool)
    .await
    .unwrap();

    // Aborted txn — must be picked up by DELETE_ABORTED_TRANSACTIONS
    // (seeded by `20260502130000_retention_system_policies.sql`).
    let aborted = seed_committed_transaction_with_file(
        &h.pool,
        dataset_id,
        branch_id,
        "APPEND",
        "ABORTED",
        Some(("data/lost.parquet", 4096)),
    )
    .await;

    // Committed txn — should NOT be flagged by the system policy.
    let _committed = seed_committed_transaction_with_file(
        &h.pool,
        dataset_id,
        branch_id,
        "SNAPSHOT",
        "COMMITTED",
        Some(("data/keep.parquet", 1024)),
    )
    .await;

    let req = Request::builder()
        .method("GET")
        .uri(format!(
            "/v1/datasets/{dataset_rid}/retention-preview?as_of_days=0"
        ))
        .body(Body::empty())
        .unwrap();
    let resp = h.router.clone().oneshot(req).await.expect("router");
    assert_eq!(resp.status(), axum::http::StatusCode::OK);
    let bytes = to_bytes(resp.into_body(), 256 * 1024).await.unwrap();
    let body: Value = serde_json::from_slice(&bytes).unwrap();

    let txns = body["transactions"].as_array().expect("transactions array");
    assert_eq!(txns.len(), 2, "preview surfaces every txn: {body}");

    let aborted_row = txns
        .iter()
        .find(|t| t["id"].as_str().unwrap() == aborted.to_string())
        .expect("aborted txn in preview");
    assert_eq!(
        aborted_row["would_delete"], true,
        "ABORTED txn must be flagged: {body}"
    );
    assert_eq!(aborted_row["status"], "ABORTED");
    assert_eq!(
        aborted_row["policy_name"], "DELETE_ABORTED_TRANSACTIONS",
        "system policy must be the cited rule: {body}"
    );

    let committed_row = txns
        .iter()
        .find(|t| t["status"].as_str() == Some("COMMITTED"))
        .expect("committed txn in preview");
    assert_eq!(
        committed_row["would_delete"], false,
        "COMMITTED txn must NOT be flagged by the system policy: {body}"
    );

    // The summary aggregates the would-delete count + bytes from the
    // ABORTED txn's file (no files for COMMITTED in the purge bucket).
    assert_eq!(body["summary"]["transactions_total"], 2);
    assert_eq!(body["summary"]["transactions_would_delete"], 1);
    // The file from the ABORTED txn shows up in the purge file list.
    let files = body["files"].as_array().unwrap();
    assert_eq!(
        files.len(),
        1,
        "only the aborted txn's file is purged: {body}"
    );
    assert_eq!(files[0]["size_bytes"], 4096);

    // Effective policy at as_of=now is the system policy (most
    // restrictive: retention_days=0).
    assert_eq!(
        body["effective_policy"]["name"],
        "DELETE_ABORTED_TRANSACTIONS"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "requires Docker; run with --include-ignored"]
async fn preview_for_unknown_dataset_returns_warning_not_500() {
    let h = common::spawn().await;
    let req = Request::builder()
        .method("GET")
        .uri("/v1/datasets/ri.unknown.dataset/retention-preview?as_of_days=0")
        .body(Body::empty())
        .unwrap();
    let resp = h.router.clone().oneshot(req).await.expect("router");
    assert_eq!(resp.status(), axum::http::StatusCode::OK);
    let bytes = to_bytes(resp.into_body(), 64 * 1024).await.unwrap();
    let body: Value = serde_json::from_slice(&bytes).unwrap();
    assert!(
        body["warnings"]
            .as_array()
            .map(|w| !w.is_empty())
            .unwrap_or(false),
        "unknown dataset must surface a non-fatal warning, got {body}"
    );
}
