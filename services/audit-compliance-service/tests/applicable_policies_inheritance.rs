//! P4 — applicable-policies inheritance + winner resolution.
//!
//! Foundry "View retention policies for a dataset [Beta]" surfaces a
//! 4-level chain (Org → Space → Project → Dataset). The resolver must:
//!   * Bucket each matching policy under the level it inherits from.
//!   * Pick a single "effective" winner using most-restrictive +
//!     specificity tie-breaks (legal_hold > lower retention_days >
//!     explicit > project > space > org).
//!
//! Docker-gated: the resolver runs in-memory but pulls the policy set
//! from the same Postgres the binary uses.

mod common;

use axum::body::{Body, to_bytes};
use axum::http::Request;
use serde_json::{Value, json};
use tower::ServiceExt;
use uuid::Uuid;

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "requires Docker; run with --include-ignored"]
async fn applicable_policies_buckets_inheritance_and_picks_most_restrictive_winner() {
    let h = common::spawn().await;
    let dataset_rid = "ri.foundry.main.dataset.applicable";
    let _ = common::seed_dataset_with_master(&h.pool, dataset_rid).await;

    let project_id = Uuid::now_v7();
    let space_id = Uuid::now_v7();

    // 1) Org-wide platform policy: 365 days, all_datasets=true.
    let org_policy = common::seed_policy(
        &h.pool,
        "org-365-days",
        "transaction",
        365,
        json!({ "all_datasets": true }),
        json!({}),
    )
    .await;

    // 2) Space (marking) policy: 90 days.
    let space_policy = common::seed_policy(
        &h.pool,
        "space-90-days",
        "transaction",
        90,
        json!({ "marking_id": space_id }),
        json!({}),
    )
    .await;

    // 3) Project policy: 30 days.
    let project_policy = common::seed_policy(
        &h.pool,
        "project-30-days",
        "transaction",
        30,
        json!({ "project_id": project_id }),
        json!({}),
    )
    .await;

    // 4) Explicit dataset policy: 7 days. This is the most restrictive
    //    + most specific → must win.
    let explicit_policy = common::seed_policy(
        &h.pool,
        "explicit-7-days",
        "transaction",
        7,
        json!({ "dataset_rid": dataset_rid }),
        json!({}),
    )
    .await;

    // 5) Foreign project policy that should NOT match.
    let _foreign = common::seed_policy(
        &h.pool,
        "foreign-project",
        "transaction",
        1,
        json!({ "project_id": Uuid::now_v7() }),
        json!({}),
    )
    .await;

    let uri = format!(
        "/v1/datasets/{dataset_rid}/applicable-policies?project_id={project_id}&space_id={space_id}"
    );
    let req = Request::builder()
        .method("GET")
        .uri(&uri)
        .body(Body::empty())
        .unwrap();
    let resp = h.router.clone().oneshot(req).await.expect("router");
    assert_eq!(resp.status(), axum::http::StatusCode::OK);
    let bytes = to_bytes(resp.into_body(), 256 * 1024).await.unwrap();
    let body: Value = serde_json::from_slice(&bytes).unwrap();

    // Inheritance buckets cover org / space / project levels.
    let org_ids: Vec<Uuid> = body["inherited"]["org"]
        .as_array()
        .unwrap()
        .iter()
        .map(|p| p["id"].as_str().unwrap().parse().unwrap())
        .collect();
    let space_ids: Vec<Uuid> = body["inherited"]["space"]
        .as_array()
        .unwrap()
        .iter()
        .map(|p| p["id"].as_str().unwrap().parse().unwrap())
        .collect();
    let project_ids: Vec<Uuid> = body["inherited"]["project"]
        .as_array()
        .unwrap()
        .iter()
        .map(|p| p["id"].as_str().unwrap().parse().unwrap())
        .collect();
    let explicit_ids: Vec<Uuid> = body["explicit"]
        .as_array()
        .unwrap()
        .iter()
        .map(|p| p["id"].as_str().unwrap().parse().unwrap())
        .collect();

    assert!(org_ids.contains(&org_policy), "org bucket: {body}");
    assert!(space_ids.contains(&space_policy), "space bucket: {body}");
    assert!(
        project_ids.contains(&project_policy),
        "project bucket: {body}"
    );
    assert!(
        explicit_ids.contains(&explicit_policy),
        "explicit bucket: {body}"
    );

    // Winner: explicit-7-days (lowest retention_days + highest specificity).
    let winner: Uuid = body["effective"]["id"].as_str().unwrap().parse().unwrap();
    assert_eq!(
        winner, explicit_policy,
        "winner must be the explicit policy: {body}"
    );
    let winner_days = body["effective"]["retention_days"].as_i64().unwrap();
    assert_eq!(winner_days, 7);

    // The conflict list mentions every other matching policy as a loser.
    let conflict_losers: Vec<Uuid> = body["conflicts"]
        .as_array()
        .unwrap()
        .iter()
        .map(|c| c["loser_id"].as_str().unwrap().parse().unwrap())
        .collect();
    for losing in [org_policy, space_policy, project_policy] {
        assert!(
            conflict_losers.contains(&losing),
            "every losing policy must surface a conflict row: {body}"
        );
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
#[ignore = "requires Docker; run with --include-ignored"]
async fn legal_hold_overrides_lower_retention_days() {
    let h = common::spawn().await;
    let dataset_rid = "ri.foundry.main.dataset.legal-hold";
    let _ = common::seed_dataset_with_master(&h.pool, dataset_rid).await;

    // Explicit 7-day policy.
    let _ = common::seed_policy(
        &h.pool,
        "explicit-7-days",
        "transaction",
        7,
        json!({ "dataset_rid": dataset_rid }),
        json!({}),
    )
    .await;

    // Org-wide legal hold (retention 365 but legal_hold=true). Insert
    // directly to set the flag.
    let legal_id = Uuid::now_v7();
    sqlx::query(
        r#"INSERT INTO retention_policies (
              id, name, scope, target_kind, retention_days,
              legal_hold, purge_mode, rules, updated_by, active,
              is_system, selector, criteria, grace_period_minutes
           ) VALUES ($1, 'global-legal-hold', '', 'transaction', 365,
                     TRUE, 'hard-delete-after-ttl', '[]'::jsonb,
                     'test', TRUE, FALSE,
                     '{"all_datasets": true}'::jsonb,
                     '{}'::jsonb, 60)"#,
    )
    .bind(legal_id)
    .execute(&h.pool)
    .await
    .unwrap();

    let req = Request::builder()
        .method("GET")
        .uri(format!("/v1/datasets/{dataset_rid}/applicable-policies"))
        .body(Body::empty())
        .unwrap();
    let resp = h.router.clone().oneshot(req).await.expect("router");
    let bytes = to_bytes(resp.into_body(), 64 * 1024).await.unwrap();
    let body: Value = serde_json::from_slice(&bytes).unwrap();

    let winner: Uuid = body["effective"]["id"].as_str().unwrap().parse().unwrap();
    assert_eq!(
        winner, legal_id,
        "legal_hold=true must win even when retention_days is higher: {body}"
    );
    assert_eq!(body["effective"]["legal_hold"], true);
}
