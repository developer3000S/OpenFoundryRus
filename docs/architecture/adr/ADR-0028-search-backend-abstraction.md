# ADR-0028: Search backend abstraction — Vespa in production, OpenSearch in dev/CI

- **Status:** Accepted
- **Date:** 2026-05-02
- **Deciders:** OpenFoundry platform architecture group
- **Supersedes / supplements:**
  - The "Vespa subchart is optional" note in the current
    [compose.yaml](../../../compose.yaml) and Helm composition.
- **Related ADRs:**
  - [ADR-0007](./ADR-0007-search-engine-choice.md) — original adoption of
    Vespa as the search engine of the platform.
  - [ADR-0009](./ADR-0009-internal-query-fabric-datafusion-flightsql.md) — DataFusion
    is the analytical engine of record; this ADR concerns serving
    search and ANN, not analytical scans.
  - [ADR-0020](./ADR-0020-cassandra-as-operational-store.md) — search
    indexes are projections of Cassandra-resident state.

## Context

OpenFoundry needs a serving search engine for:

- **Object search** — lexical (BM25) and filter-driven queries over
  ontology objects, projected from Cassandra.
- **ANN / vector search** — k-NN over embeddings on objects, dataset
  documents and knowledge chunks for RAG and recommendations.
- **Graph-light traversal** — small-fanout relationship lookups
  surfaced as part of search results.
- **Faceted browsing and aggregations** over the same projections.

Today the platform mounts a **Vespa subchart marked as optional**.
Services that need search either depend on Vespa being present or
silently degrade. There is no abstraction; Vespa client code is
inlined where it is used.

The platform also has a **dev / CI requirement** that the entire
backend stack come up under `docker compose` on a developer laptop
within reasonable resource bounds. Vespa's content / container /
config-server topology is heavyweight for that environment.

We need to pick:

1. **The production engine** (already implicitly Vespa — see
   [ADR-0007](./ADR-0007-search-engine-choice.md)).
2. **A dev / CI engine** that supports the same query shapes well
   enough to keep developers honest.
3. **An abstraction** so service code does not pin to either
   implementation.

## Options considered

### Production engine

#### Engine A — Vespa (chosen, already in use)

- BM25, k-NN with HNSW, hybrid retrieval, ranking expressions, custom
  ranking models, structured filters, faceting, multi-document
  joins, real-time indexing.
- The only open-source engine that genuinely co-locates lexical,
  vector and ranking-model serving with the latency profile we need.
- Operationally heavyweight — but the cost is paid once per
  environment and is amortised across every search-touching service.

#### Engine B — Elasticsearch / OpenSearch in production

- BM25 + k-NN HNSW are GA. Operational footprint is well-known.
- Reasons it is rejected as the **production** engine:
  - Hybrid retrieval (BM25 + ANN scoring fusion) is significantly
    weaker than Vespa's first-class hybrid ranker.
  - Custom ranking expressions require either painted-on script
    languages or a learn-to-rank plugin; Vespa treats ranking as a
    first-class artefact.
  - Cluster operations on the scale we need (multi-tenant, large
    object counts) push OpenSearch into territory where Vespa was
    designed to live.

#### Engine C — Meilisearch (rejected)

- Excellent developer ergonomics for lexical search. Production-quality
  BM25-style relevance and typo tolerance.
- **ANN support is immature** as of May 2026: the experimental vector
  index does not match the recall / latency profile of HNSW
  implementations in Vespa or OpenSearch, and the maintainers
  themselves flag it as evolving. The platform's RAG path needs
  production-grade k-NN; Meilisearch cannot serve it.
- Rejected as both production and as dev/CI engine.

#### Engine D — Qdrant / Weaviate / Milvus (vector-only, rejected)

- Pure-vector engines. Strong ANN, weak or non-existent lexical.
- Would force every query path to either issue two queries (one
  lexical, one vector) and merge in application code, or to push
  every projection through a separate lexical engine as well.
- Doubling the search infrastructure to recover hybrid retrieval
  defeats the purpose.

### Dev / CI engine

#### Dev engine A — OpenSearch single-node (chosen)

- Runs as a single container under `docker compose` with bounded
  memory and disk.
- BM25 and k-NN HNSW are both production-grade.
- Same query shapes as production for the **majority** of cases:
  filtered BM25, kNN with pre/post filters, faceting, aggregations.
- Hybrid ranking and Vespa-specific ranking expressions are **not**
  reproduced; the abstraction surface (see below) deliberately
  does not expose those features so a developer cannot accidentally
  rely on something that only works in production.
- Single-node mode is documented as not for production by the
  upstream project; this matches our intent (CI / dev only).

#### Dev engine B — Vespa, identical to production

- Reproduces production exactly.
- Resource cost on a developer laptop is uncomfortable; CI cost is
  excessive.
- Rejected.

#### Dev engine C — Meilisearch

- Rejected per ANN immaturity; tests that touch ANN would not
  exercise behaviour comparable to production.

#### Dev engine D — In-memory mock / stub

- Fast, free, and a recipe for production surprise. Rejected.

## Decision

We adopt the following triad:

- **Production search backend: Vespa.** The Vespa subchart becomes
  **mandatory** in every environment that runs a `prod` profile;
  the "optional" flag is removed from `infra/k8s/platform/charts/vespa/values.yaml`
  and from any Helm umbrella that consumes it.
- **Dev / CI search backend: OpenSearch single-node.** Provisioned
  via `docker compose` and via a Helm release in `dev` / `ci`
  Kubernetes profiles only. Never used in production.
- **Abstraction: a `SearchBackend` trait in a new workspace crate
  `libs/search-abstraction`.** Two implementations, selected by
  the environment variable `SEARCH_BACKEND` (values: `vespa`,
  `opensearch`).

Meilisearch is explicitly rejected for both production and dev/CI
on ANN-maturity grounds.

## Trait surface

The trait deliberately exposes the **intersection** of features that
both backends can serve faithfully. Vespa-specific extensions are
either pushed into a separate, optional `VespaExt` trait that
production-only code paths can use (and that returns
`Err(Unsupported)` on OpenSearch) or modelled at a level both
engines can implement.

```rust
#[async_trait]
pub trait SearchBackend: Send + Sync {
    async fn upsert(&self, doc: SearchDoc) -> Result<()>;
    async fn delete(&self, id: &DocId) -> Result<()>;

    async fn lexical(&self, q: &LexicalQuery) -> Result<SearchResults>;
    async fn vector(&self, q: &VectorQuery) -> Result<SearchResults>;
    async fn hybrid(&self, q: &HybridQuery) -> Result<SearchResults>;

    async fn facet(&self, q: &FacetQuery) -> Result<FacetResults>;

    async fn health(&self) -> Result<BackendHealth>;
}
```

Where:

- `SearchDoc` carries the document id, namespace, fields, vectors and
  a set of typed values that both backends accept (`text`, `keyword`,
  `int`, `double`, `bool`, `geo`, `vector(f32, dim)`).
- `LexicalQuery` is BM25 + filters + sort + paging.
- `VectorQuery` is k-NN with optional pre/post filters and a configurable
  similarity (cosine, dot, L2).
- `HybridQuery` is a fixed schema (BM25 + ANN with a normalised score
  fusion). On Vespa it lowers to a Vespa ranking profile; on OpenSearch
  it lowers to a pair of queries combined by the engine's hybrid query
  feature when available, or by client-side normalised RRF when it is
  not.

`VespaExt` (optional, lives in the same crate, only the Vespa impl
implements it):

```rust
#[async_trait]
pub trait VespaExt {
    async fn ranking_profile(&self, q: &RankingProfileQuery) -> Result<SearchResults>;
}
```

Code paths that import `VespaExt` are required by lint to live behind
a `cfg(feature = "vespa-ext")` and to provide a documented graceful
fallback for OpenSearch dev environments.

## Selection and configuration

- `SEARCH_BACKEND` env var: `vespa` (default in `prod`) or `opensearch`
  (default in `dev` / `ci`).
- Each backend reads its own connection settings (`VESPA_ENDPOINT`,
  `OPENSEARCH_ENDPOINT`).
- A constructor in `libs/search-abstraction` returns an
  `Arc<dyn SearchBackend>` based on the env var. Services hold the
  `Arc` and never reference an implementation type directly.

## Indexing pipeline

Search indexes are **projections of Cassandra-resident state**, not
sources of truth. The pipeline is:

1. A handler writes to Cassandra and to the outbox
   ([ADR-0022](./ADR-0022-transactional-outbox-postgres-debezium.md)).
2. Debezium publishes the event to Kafka.
3. A search-indexer consumer in `workers-go/reindex/` reads the event
   and calls `SearchBackend::upsert` against whichever backend is
   configured.
4. A nightly reconciliation Workflow reads the canonical state from
   Cassandra and re-projects any drift.

Backfill and re-projection use Temporal Workflows in
`workers-go/reindex/` so that a schema change can be rolled out with
bounded parallelism, retry and visibility
([ADR-0021](./ADR-0021-temporal-on-cassandra-go-workers.md)).

## Operational consequences

- Vespa subchart `optional: false` in production overlays.
- New Helm release `infra/k8s/opensearch/dev/` for the single-node
  dev cluster (also used by CI).
- New `compose.yaml` service `opensearch` (single-node).
- New workspace crate `libs/search-abstraction` with the trait, the
  two impls and a feature flag for `VespaExt`.
- New CI matrix dimension: every search-touching test runs against
  `SEARCH_BACKEND=opensearch`. A nightly job runs the same tests
  against `SEARCH_BACKEND=vespa` in a dedicated environment.
- New CI lint: any service that imports `vespa::*` directly (rather
  than `search_abstraction::*`) fails the build.
- New runbook `infra/runbooks/search.md` covering reindex,
  backfill, schema evolution, capacity and the dev/CI parity story.

## Consequences

### Positive

- Production retains the engine that best serves hybrid retrieval at
  our latency target.
- Dev and CI run the full search test surface on a backend that
  fits on a laptop, with the same trait calls and the same query
  shapes.
- Service code does not pin to a backend; replacing one in the
  future is a constructor change.
- The abstraction is honest: the intersection of features is the
  trait, and the Vespa-specific extension is segregated and
  policed by lint.

### Negative

- Hybrid ranking and Vespa-specific ranking profiles are **not**
  reproducible in dev/CI. Tests that depend on relevance order
  beyond "the right document is in the top-k" must run in the
  nightly Vespa job. This is a real cost; the alternative
  (running Vespa in CI) is worse.
- Two backends to keep in sync. Mitigated by the trait being the
  contract and by the indexer being the only writer.

### Neutral

- Search remains a projection. Authority lives in Cassandra and
  Iceberg; an index loss is recoverable by re-projection from
  source.

## Follow-ups

- Implement migration plan task **S0.1.i** (this ADR) and the
  related items under **S0.4.b** (the trait) and **S0.6.x**
  (Helm + compose).
- Author `libs/search-abstraction` with both impls.
- Flip the Vespa subchart to `required` in production overlays.
- Add the CI lint that forbids direct `vespa::*` imports outside
  `libs/search-abstraction`.
- Author `infra/runbooks/search.md`.
- Re-evaluate this ADR if Meilisearch's ANN reaches production-grade
  parity, or if OpenSearch closes the hybrid-ranking gap with
  Vespa.
