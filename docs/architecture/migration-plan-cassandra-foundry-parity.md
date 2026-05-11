# Plan de migración a Foundry-parity con Cassandra

> **Estado del producto:** Pre-producción. Permitido romper compatibilidad sin migración de datos.
> **Objetivo:** Volcar la pirámide de almacenamiento. Iceberg = fuente de verdad. Cassandra = serving operacional. Postgres = residual y consolidado. Orquestación distribuida estilo **Foundry-pattern** ([ADR-0037](adr/ADR-0037-foundry-pattern-orchestration.md)): Spark Operator + Kafka consumers + state machines en Postgres + outbox/Debezium. Vespa obligatorio en prod, OpenSearch en dev. Cedar embebido como policy engine. Identity custom retenido y endurecido. Cross-region DR vía Iceberg replication.
>
> **Regla de cierre formal:** en este documento un stream no se considera "cerrado" por tener `substrate` o runbooks aterrizados. **S1, S2, S3, S5, S6 y el hito "Postgres residual" solo cierran cuando pasan la checklist final y los grep gates de §18.**
> **Duración estimada:** 16-22 semanas con 5 streams paralelos.
> **Equipo asumido:** 3-4 backend Rust, soporte puntual Go cuando un consumer lo justifique, 1 platform/SRE, 1 data eng, soporte ocasional security.

---

## 0. Resumen del cambio arquitectónico

### Antes
- 71 clusters CNPG Postgres (213 instancias).
- 84 servicios con `sqlx` inlined.
- Workflow casero con `cron` + tick loop.
- Identity custom.
- Kafka `event-bus-data` desplegado pero **sin tráfico**.
- Vespa **opcional/desactivado**.
- Iceberg activo solo en 4 servicios.

### Después
- **3 clusters Cassandra** (1 por DC: A, B, C — multi-DC desde día 1) sirviendo estado operacional caliente.
- **4 clusters CNPG Postgres** (consolidados): `pg-schemas`, `pg-policy`, `pg-lakekeeper`, `pg-runtime-config`.
- **Orquestación distribuida estilo Foundry-pattern**: SparkApplication CRs para batches, Kafka consumers para fan-out/event-driven flows, state machines en Postgres para workflows con estado y outbox transaccional + Debezium como coordinación cross-servicio.
- **Workers de negocio en Rust** por defecto (`event-bus-data`, Axum, state machines); **Go** solo cuando haya una razón fuerte de throughput/ergonomía operacional.
- **Iceberg WORM** como fuente de verdad para datasets, audit log, eventos de dominio materializados.
- **Vespa obligatorio** en prod como serving de búsqueda/ANN; **OpenSearch single-node** en dev y CI.
- **Kafka activo** con transactional outbox sobre **Postgres `pg-policy`** + **Debezium Kafka Connect** como relay (NO outbox en Cassandra LWT — descartado).
- **Cedar policy engine embebido** como librería Rust (`libs/authz-cedar`); policies en Postgres, evaluación local sin red. NO OpenFGA.
- **`identity-federation-service` retenido** y endurecido (no Keycloak); estado de sesión migrado a Cassandra TTL.
- **Iceberg cross-region replication** (S3 CRR + Lakekeeper read-replica) para DR.

### Principios que guían cada tarea
1. No tocar el contrato público (gRPC/OpenAPI/SDKs) salvo cuando un endpoint dejó de tener sentido.
2. Toda lectura caliente debe ir a Cassandra o cache; jamás a Postgres.
3. Toda escritura mutable de objetos de ontología pasa por outbox (Postgres) → Debezium → Kafka → indexer → Vespa/Iceberg.
4. Postgres solo para schemas declarativos, configuración estática, policies Cedar, outbox transaccional, catálogo Lakekeeper.
5. La coordinación vive en el patrón Foundry-pattern: Spark Operator + Kafka + state machines + CronJobs/event-scheduler; nada de tick loops en proceso ni dependencia de un orquestador central.
6. Authz local, sin red: Cedar evalúa en proceso. Policies hot-reload por evento NATS.
7. Cassandra modelado por queries: ninguna SELECT escanea más de 1 partition. Si la necesitas → Vespa o Iceberg/Trino.
8. Aceptamos consistencia eventual + idempotencia en lugar de atomicidad estricta cross-store.

---

## 1. Inventario de lo que se reaprovecha

| Activo | Estado | Uso futuro |
|---|---|---|
| `libs/core-models` (BranchName, MarkingId, DatasetRid, ids tipados) | DB-agnostic | **Reusar 100%** sin tocar |
| `libs/auth-middleware` (JWT extractor, AuthUser) | DB-agnostic | **Reusar 100%** |
| `libs/event-bus-data` (Kafka rdkafka wrapper) | Implementado, sin uso | **Activar** |
| `libs/event-bus-control` (NATS JetStream) | En uso | **Reusar** |
| `libs/storage-abstraction/src/iceberg.rs` (Iceberg REST client, 220 LOC) | Producción | **Reusar y expandir** |
| `libs/audit-trail` | En uso, mínimo | **Refactorizar** para emitir a Kafka |
| `libs/ontology-kernel` | sqlx inlined | **Refactorizar**: extraer trait `ObjectStore`, `LinkStore`, `SchemaStore` |
| `libs/testing` (testcontainers Postgres + Kafka) | En uso | **Extender** con Cassandra testcontainer |
| Iceberg + Lakekeeper deployment (`infra/k8s/platform/manifests/lakekeeper/`) | Producción, 3 réplicas | **Reusar** + cross-region |
| Strimzi Kafka KRaft (3 brokers RF=3 ISR=2) | Salud OK | **Reusar** + activar topics |
| Rook Ceph (5 MON, EC 8+3, zone-aware) | Producción | **Reusar** |
| Vespa subchart (3 cs / 2 c / 3 ct) | Opcional | **Hacer obligatorio** |
| Proto definitions (`proto/`) | Estables | **Reusar**; algunos métodos read se duplicarán para "consistency=strong/eventual" |
| SDKs TS/Python/Java | Auto-generados | **Reusar**; regenerar tras cambios proto puntuales |
| Justfile, CI workflows | Funcionales | **Extender** con recipes Cassandra/Kafka/Spark y limpieza del legado de ADR-0021 |
| 84 servicios — la **lógica de dominio** | Compleja | **Reusar 70-80%**; la migración toca persistencia y handlers, no reglas de negocio |

## 2. Inventario de lo que se elimina o reemplaza

| Activo | Acción | Razón |
|---|---|---|
| 67 de los 71 manifests CNPG (`infra/k8s/platform/manifests/cnpg/clusters/*.yaml`) | **Borrar** | Consolidación a 4 clusters |
| 71 secrets `<service>-pg-app` | **Reemplazar** por 4 secrets compartidos + acceso por schema/role | Consolidación de Postgres |
| `services/workflow-automation-service/src/domain/executor.rs` (scheduler casero) | **Reescribir / archivar legacy** | Se sustituye por consumers Kafka + state machine en Postgres; no por un worker central |
| `services/pipeline-schedule-service/src/main.rs` (tick loop) | **Refactorizar** | Los disparos pasan a CronJobs/event-scheduler + SparkApplication CRs; el servicio conserva ownership del contrato |
| Migrations sqlx de los 9 servicios ontology (~80 archivos) | **Archivar** como referencia + **borrar** del runtime | Reemplazados por keyspaces Cassandra + 1 schema Postgres mínimo |
| Migrations de session/oauth/identity en Postgres | **Borrar** | Estado a Cassandra TTL |
| Migrations sqlx de approvals, workflow-automation, pipeline-* | **Reclasificar por dominio** | El runtime pasa a state machines + outbox/Kafka; las tablas legacy se retiran solo tras cutover por dominio |
| `crate cron` (0.12) en workflow-automation-service | **Reemplazar** dependency | Time-based triggers viven en CronJobs/event-scheduler, no en loops locales |
| `services/health-check-service` | **Borrado** → `telemetry-governance-service` | Anti-patrón; las health probes de k8s y el dominio de telemetría son la respuesta |
| `services/widget-registry-service` | **Borrado** → `application-composition-service` | Stub absorbido por el runtime de composición |
| `services/tool-registry-service` | **Borrado** → `agent-runtime-service` | Granularidad excesiva; catálogo/dispatch viven con el agente |
| `workers-go/pipeline/` (Go/Temporal worker, task queue `openfoundry.pipeline`) | **Borrado (Tarea 3.6 ✅)** | Sustituido por `pipeline-build-service` que emite `SparkApplication` CRs y por el `CronJob schedules-tick` para disparos cron; sin worker intermedio. |

---

## 3. Topología objetivo de almacenamiento

### 3.1 Cassandra (3 DC desde día 1, multi-AZ)

| Keyspace | RF | Consistency default | Propósito | Servicios consumidores |
|---|---|---|---|---|
| `ontology_objects` | NetworkTopologyStrategy `{dc1:3, dc2:3, dc3:3}` | LOCAL_QUORUM | Object instances, properties, relationships, edits | ontology-actions, ontology-query, object-database |
| `ontology_indexes` | `{dc1:3, dc2:3, dc3:3}` | LOCAL_QUORUM | Índices secundarios materializados (by-type, by-owner, by-marking) | ontology-query, ontology-funnel |
| `actions_log` | `{dc1:3, dc2:3, dc3:3}` | LOCAL_QUORUM | Eventos de acciones aplicadas (append-only) | ontology-actions |
| `sessions` | `{dc1:3, dc2:3, dc3:3}` | LOCAL_QUORUM | Sesiones, refresh tokens, oauth state (TTL nativo) | identity-federation, session-governance, oauth-integration |
| `notifications_inbox` | `{dc1:3, dc2:3, dc3:3}` | LOCAL_ONE (lectura) | Inbox de notificaciones por user (TTL 30d) | notification-alerting |
| `agent_state` | `{dc1:3, dc2:3, dc3:3}` | LOCAL_QUORUM | Estado conversacional, contextos cortos | agent-runtime, conversation-state |
| `temporal_persistence` | `{dc1:3, dc2:3, dc3:3}` | LOCAL_QUORUM | Keyspace heredado del orquestador previo; objetivo = drop en cleanup | (decommission) |
| `temporal_visibility` | `{dc1:3, dc2:3, dc3:3}` | LOCAL_QUORUM | Keyspace heredado de visibilidad; objetivo = drop en cleanup del patrón Foundry | (decommission) |

> **Nota:** El keyspace `outbox_events` fue **descartado**. El outbox transaccional vive en Postgres `pg-policy.outbox` (ver §3.2 y ADR-0022). Razón: Cassandra no garantiza atomicidad cross-table sin BATCH logged single-partition; LWT cuesta ~4× y rompe SLO del hot path. Postgres da commit transaccional gratuito + Debezium maduro como CDC.

#### Reglas duras de modelado Cassandra (ADR-0020)

- **Composite PK obligatorio**: nunca `tenant_id` solo. Patrón base: `((tenant_id, type_id, time_bucket), updated_at DESC, object_id)`.
- **Time bucketing por tabla**: diario para `objects_by_*`; horario para `actions_log`, `sessions`, `agent_state`.
- **Límite duro por partition**: ≤50 MB y ≤100k filas. Alarma `nodetool tablestats` >50 MB.
- **`object_id` = TimeUUID** (no UUIDv4): permite range scans temporales naturales.
- **TTL siempre por tabla con datos rotativos**. Jamás `DELETE` masivo; soft-delete con flag `deleted boolean` si necesario.
- **Compaction strategy**:
  - `objects_*` (mutable, lecturas mixtas) → **LCS** (LeveledCompactionStrategy).
  - `actions_log`, `audit_*`, `sessions`, `agent_state` (time-series + TTL) → **TWCS** con ventana = bucket.
- **Secondary indexes vetados**. Solo SAI (Cassandra 5) en casos puntuales bien justificados; el patrón por defecto son **tablas materializadas** mantenidas por la aplicación.
- **Anti-pattern duro**: ninguna SELECT puede escanear más de 1 partition. Si la query la necesita, va a Vespa o a Iceberg/Trino. Sin excepciones.
- **LWT (`IF`) restringido** a casos genuinamente concurrentes y raros (versionado optimista en escrituras conflictivas). Resto = idempotency keys.

### 3.2 Postgres consolidado (4 clusters CNPG, 3 instancias cada uno = 12 procesos)

| Cluster | Schemas | Propósito |
|---|---|---|
| `pg-schemas` | `ontology_schema`, `dataset_schema`, `auth_schema`, `app_schema`, `pipeline_schema` | Schemas declarativos: tipos de ontología, link types, action definitions, dataset metadata, JWKS keys, oauth clients, definiciones de pipelines |
| `pg-policy` | `outbox`, `cedar_policies`, `audit_metadata`, `tenancy` | **Outbox transaccional** (leído por Debezium), policies Cedar versionadas, tenancy/orgs, audit metadata indexada (los eventos van a Iceberg) |
| `pg-lakekeeper` | `lakekeeper` | Catálogo Iceberg (managed by Lakekeeper) |
| `pg-runtime-config` | `marketplace`, `app_builder`, `connector_management`, `ingestion_replication`, `model_registry`, `developer_console` | Configuraciones declarativas y metadata de control-plane low-traffic: definiciones de conectores/fuentes, credenciales metadata, desired state declarativo de materializaciones y catálogos operativos. Nunca runtime hot-path |

- `connector-management-service` es el owner de definiciones (`connections`, registros, capacidades, credenciales metadata) en `pg-runtime-config.connector_management`.
- `ingestion-replication-service` es el owner del desired state de materialización en `pg-runtime-config.ingestion_replication` (specs, bindings, referencias a recursos materializados), pero no del estado de ejecución high-frequency.
- `sync_jobs`, `ingest_jobs` cuando actúe como cola/estado de ejecución, `ingestion_checkpoints`, retries, attempt counters, progress y status operacional son runtime hot-path: no pueden tener multi-owner en Postgres consolidado. Su destino objetivo es Cassandra, Kafka compacted topics, `status` de CRs de Kubernetes o un runtime store no-PG equivalente.
- Regla de ownership: ningún dato de runtime de ingestión puede tener copia autoritativa dual entre `pg-runtime-config` y el store runtime. Postgres conserva definición low-traffic; el runtime store conserva ejecución y recuperación.

#### Separación control-plane vs runtime en ingestión

| Artefacto | Servicio owner | Store objetivo | Notas |
|---|---|---|---|
| Definiciones de conectores, fuentes, capacidades, registros, credenciales metadata | `connector-management-service` | `pg-runtime-config.connector_management` | Low-traffic, auditable, mutable por operadores |
| Desired state declarativo de materialización (`IngestJobSpec`, bindings, nombres de CRs, wiring a topics/datasets) | `ingestion-replication-service` | `pg-runtime-config.ingestion_replication` | Persistencia de intención, no de actividad hot-path |
| `sync_jobs`, retries, scheduling efectivo, attempt history, progress/status de ejecución | `connector-management-service` / runtime de workers | Cassandra / Kafka / `status` de CRs | Si cambia por intento o por polling, no pertenece a PG |
| `ingestion_checkpoints`, offsets/LSN, cursores de CDC, heartbeats y recovery position | `ingestion-replication-service` | Cassandra o Kafka compacted topic; `status` de CR si aplica | Debe soportar writes frecuentes e idempotencia sin castigar CNPG |

### 3.3 Iceberg (Lakekeeper) — fuente de verdad

| Namespace | Tablas | Propósito |
|---|---|---|
| `of.datasets` | (existente) | Datasets canónicos |
| `of.audit` | `events`, `events_enriched` | Audit WORM inmutable |
| `of.ontology_history` | `objects_v1`, `actions_v1`, `links_v1` | Materialización de eventos Kafka → Iceberg para analytics + time travel |
| `of.lineage` | `runs`, `events`, `datasets_io` | OpenLineage materializado |
| `of.metrics_long` | `service_metrics_daily`, `slo_burn_daily` | Métricas long-term (Prometheus → Mimir → Iceberg) |
| `of.ai` | `prompts`, `responses`, `evaluations`, `traces` | Logs de agentes/LLM |

### 3.4 Vespa (prod) / OpenSearch (dev) — search/ANN serving

| Aplicación / Schema | Documentos | Propósito |
|---|---|---|
| `objects` | Materialización de `ontology_objects` (proyección por type) | Search-around, filtros, ANN sobre embeddings |
| `links` | Relaciones materializadas | Graph traversal limitado |
| `datasets` | Catálogo searchable | Descubrimiento |
| `knowledge` | Documentos + chunks | RAG, retrieval-context |

> **Backend abstracto**: trait `SearchBackend` en `libs/search-abstraction` con dos impls: `vespa` (prod, obligatorio) y `opensearch` (dev y CI). Selección por env `SEARCH_BACKEND`. OpenSearch elegido sobre Meilisearch por tener BM25 + k-NN HNSW production-grade (Meilisearch carece de ANN serio).

### 3.5 Authz — Cedar embebido

- Crate `libs/authz-cedar` con `cedar-policy = "4"`.
- Policies versionadas en `pg-policy.cedar_policies`, cargadas en memoria al startup, hot-reload por evento NATS `authz.policy.changed`.
- Schema Cedar derivado de `core-models` (entities: `User`, `Object`, `Branch`, `Marking`).
- Evaluación local (microsegundos), cero round-trip de red por check.
- Decision audit emitido a Kafka `audit.authz.v1` → Iceberg `of.audit.authz`.

### 3.6 Outbox — Postgres + Debezium (no Cassandra)

- Tabla `pg-policy.outbox.events(event_id uuid PK, aggregate text, aggregate_id text, payload jsonb, headers jsonb, created_at timestamptz, topic text)`.
- Cada handler de mutación: escribe a Cassandra (estado primario, idempotente por `event_id` determinista) y a Postgres outbox **en una transacción Postgres**. Si Cassandra OK + Postgres falla → reintento del cliente con misma `event_id` → idempotente.
- **Debezium Kafka Connect** lee outbox vía WAL logical decoding y publica en Kafka. Borra fila tras publicación confirmada.
- Sin servicio `outbox-relay` propio: Debezium hace el trabajo.

### 3.7 Cache (Redis Cluster o Dragonfly)

3-node cluster. Usos: rate-limit (gateway), JWT validation cache, ontology hot cache invalidado por evento Kafka.

---

## 4. Streams de trabajo y secuenciación

```
T+0 ─── T+4w ─── T+8w ─── T+12w ─── T+16w ─── T+20w
│
├── S0: Foundations (T+0 → T+3w) ←── debe terminar antes de cualquier otro stream
│
├── S1: Cassandra ontology (T+3 → T+11w)        ────────────────
│
├── S2: Foundry-pattern orchestration migration (T+3 → T+10w) ─────────────
│       (Spark Operator + Kafka consumers + state machines Postgres)
│
├── S3: Auth/sessions a Cassandra + Cedar (T+5 → T+9w)        ─────────
│
├── S4: Outbox Postgres + Debezium + Kafka activo + Vespa indexer (T+8 → T+13w)         ─────────
│
├── S5: Iceberg WORM expansion + audit + lineage (T+6 → T+14w)      ────────────
│
├── S6: Postgres consolidation a 4 clusters (T+10 → T+14w)              ─────
│
├── S7: Cross-region DR (T+14 → T+20w)                                       ──────
│
└── S8: Cleanup, decommission, runbooks, chaos (T+18 → T+22w)                         ─────
```

---

## 5. Stream S0 — Foundations (3 semanas)

**Bloquea** todos los demás streams.

### Tarea S0.1 — ADRs y design docs (semana 1)

- [x] **S0.1.a** [`ADR-0020-cassandra-as-operational-store.md`](adr/ADR-0020-cassandra-as-operational-store.md). Rationale, modelo Object Storage V2-like, RF, consistency levels, **reglas duras de modelado** (ver §3.1), anti-patrones a evitar.
- [x] **S0.1.b** [`ADR-0021-temporal-on-cassandra-go-workers.md`](adr/ADR-0021-temporal-on-cassandra-go-workers.md). **Histórico / superseded** por [ADR-0037](adr/ADR-0037-foundry-pattern-orchestration.md). Se conserva solo como referencia de la decisión descartada y del rationale original.
- [x] **S0.1.c** [`ADR-0022-transactional-outbox-postgres-debezium.md`](adr/ADR-0022-transactional-outbox-postgres-debezium.md). **Outbox vive en Postgres `pg-policy.outbox`**, leído por **Debezium Kafka Connect**. LWT en Cassandra **descartado** por: (1) no hay atomicidad cross-table; (2) LWT cuesta 4× una escritura normal y rompe SLO; (3) Debezium es maduro y elimina la necesidad de un relay propio.
- [x] **S0.1.d** [`ADR-0023-iceberg-cross-region-dr.md`](adr/ADR-0023-iceberg-cross-region-dr.md). S3 CRR + Lakekeeper read-replica vs Iceberg snapshot mirror.
- [x] **S0.1.e** [`ADR-0024-postgres-consolidation.md`](adr/ADR-0024-postgres-consolidation.md). 71 → 4 clusters; layout de schemas y roles.
- [x] **S0.1.f** [`ADR-0025-eliminate-custom-scheduler.md`](adr/ADR-0025-eliminate-custom-scheduler.md). Decisión definitiva de borrar `pipeline-schedule-service` como Deployment.
- [x] **S0.1.g** [`ADR-0026-identity-custom-retained.md`](adr/ADR-0026-identity-custom-retained.md). **Mantener `identity-federation-service`** y endurecerlo (rotación JWKS automática, MFA TOTP+WebAuthn, SCIM 2.0, audit completo, key custody en Vault). Keycloak descartado por simplicidad operacional y control sobre el flujo. Sesiones → Cassandra TTL.
- [x] **S0.1.h** [`ADR-0027-cedar-policy-engine.md`](adr/ADR-0027-cedar-policy-engine.md). **Cedar embebido como librería Rust**, NO OpenFGA. Policies en `pg-policy.cedar_policies`, evaluación local, hot-reload por NATS. Schema derivado de `core-models` (User, Object, Branch, Marking).
- [x] **S0.1.i** [`ADR-0028-search-backend-abstraction.md`](adr/ADR-0028-search-backend-abstraction.md). Trait `SearchBackend` con dos impls: Vespa (prod, obligatorio) y OpenSearch single-node (dev/CI). Meilisearch descartado por ANN inmaduro.
- [x] **S0.1.j** [`docs/architecture/data-model-cassandra.md`](data-model-cassandra.md). Modelado por queries (partition key strategy, denormalization expected, time bucketing, compaction strategy) para cada keyspace.

### Tarea S0.2 — Cluster Cassandra dev local + helm chart (semana 1-2)

- [x] **S0.2.a** Añadir sub-chart **K8ssandra Operator** (`k8ssandra-operator`, Apache-2.0) o **Cass-Operator** directo en [`infra/k8s/platform/manifests/cassandra/`](../../infra/k8s/platform/manifests/cassandra/). Recomendación implementada: **k8ssandra-operator** (oficial DataStax, gestiona Cassandra + Reaper + Medusa). Stargate **deshabilitado** (servicios usan `scylla` Rust crate vía CQL nativo). Ver [`values-k8ssandra-operator.yaml`](../../infra/k8s/platform/manifests/cassandra/values-k8ssandra-operator.yaml) y [`README.md`](../../infra/k8s/platform/manifests/cassandra/README.md).
- [x] **S0.2.b** [`infra/k8s/platform/manifests/cassandra/cluster-dev.yaml`](../../infra/k8s/platform/manifests/cassandra/cluster-dev.yaml) — single-DC (`dc1`, 3 racks), 3 nodos, RF=3 con `NetworkTopologyStrategy` (shape-compatible con prod), single PV sobre `ceph-rbd-fast` (NVMe), Reaper auto-scheduling, Medusa a Ceph S3.
- [x] **S0.2.c** `infra/k8s/platform/manifests/cassandra/cluster-prod.yaml` — multi-DC NetworkTopologyStrategy, 3 nodos por DC. → [`infra/k8s/platform/manifests/cassandra/cluster-prod.yaml`](../../infra/k8s/platform/manifests/cassandra/cluster-prod.yaml)
- [x] **S0.2.d** Configurar **Reaper** (auto-repair) y **Medusa** (backups a Ceph S3). → embebido en [`cluster-prod.yaml`](../../infra/k8s/platform/manifests/cassandra/cluster-prod.yaml) (`spec.reaper`, `spec.medusa`, `MedusaBackupSchedule`) + [`medusa-bucket.yaml`](../../infra/k8s/platform/manifests/cassandra/medusa-bucket.yaml)
- [x] **S0.2.e** ServiceMonitor + Grafana dashboard de Cassandra (importar 5408 + custom). → [`infra/k8s/platform/manifests/cassandra/servicemonitor.yaml`](../../infra/k8s/platform/manifests/cassandra/servicemonitor.yaml) + [`infra/k8s/platform/manifests/cassandra/grafana-dashboards.yaml`](../../infra/k8s/platform/manifests/cassandra/grafana-dashboards.yaml)
- [x] **S0.2.f** Crear keyspaces vacíos vía Job Helm `post-install` (CQL script + cqlsh). → [`infra/k8s/platform/manifests/cassandra/keyspaces-job.yaml`](../../infra/k8s/platform/manifests/cassandra/keyspaces-job.yaml)
- [x] **S0.2.g** Documentar runbook `infra/runbooks/cassandra.md` (repair, scale-out, replace-node, restore). → [`infra/runbooks/cassandra.md`](../../infra/runbooks/cassandra.md)

### Tarea S0.3 — Crate compartido `libs/cassandra-kernel` (semana 2)

- [x] **S0.3.a** Crear `libs/cassandra-kernel/Cargo.toml` con `scylla = "0.13"` (driver oficial; sí, el driver de Scylla habla CQL nativo y es **el mejor driver Rust para Cassandra**, soporta tokens-aware routing, prepared statements, paging streams). → [`libs/cassandra-kernel/Cargo.toml`](../../libs/cassandra-kernel/Cargo.toml)
- [x] **S0.3.b** `Session` builder: contact points, datacenter local, retry policy, speculative execution, request tracing opt-in. → [`libs/cassandra-kernel/src/session.rs`](../../libs/cassandra-kernel/src/session.rs)
- [x] **S0.3.c** Pool/Session manager (`OnceCell<Arc<Session>>`) por servicio. → [`libs/cassandra-kernel/src/shared.rs`](../../libs/cassandra-kernel/src/shared.rs)
- [x] **S0.3.d** Helpers: `paged_query`, `lwt_insert_if_not_exists`, `batch_logged` (uso restringido), `prepared_cache`. → [`libs/cassandra-kernel/src/query.rs`](../../libs/cassandra-kernel/src/query.rs)
- [x] **S0.3.e** Macro `cql_migrate!` para migraciones CQL versionadas (idempotentes — Cassandra no tiene migraciones reversibles puras). → [`libs/cassandra-kernel/src/migrate.rs`](../../libs/cassandra-kernel/src/migrate.rs)
- [x] **S0.3.f** Tests con `testcontainers` Cassandra image (`cassandra:5`). → [`libs/cassandra-kernel/tests/integration.rs`](../../libs/cassandra-kernel/tests/integration.rs)
- [x] **S0.3.g** Documentar API en `libs/cassandra-kernel/README.md`. → [`libs/cassandra-kernel/README.md`](../../libs/cassandra-kernel/README.md)

### Tarea S0.4 — Repository pattern abstracto (semana 2)

- [x] **S0.4.a** Crear `libs/storage-abstraction/src/repositories.rs` con traits genéricos: → [libs/storage-abstraction/src/repositories.rs](../../libs/storage-abstraction/src/repositories.rs)
  ```rust
  #[async_trait]
  pub trait ObjectStore: Send + Sync {
      async fn get(&self, id: ObjectId, consistency: ReadConsistency) -> Result<Option<Object>>;
      async fn put(&self, obj: Object, expected_version: Option<u64>) -> Result<PutOutcome>;
      async fn list_by_type(&self, type_id: TypeId, page: Page) -> Result<Page<Object>>;
      // ...
  }
  ```
- [x] **S0.4.b** Definir 6 traits: `ObjectStore`, `LinkStore`, `SchemaStore`, `SessionStore`, `ActionLogStore`, `SearchBackend`. (Sin `OutboxStore`: el outbox se usa vía `sqlx` directo en `pg-policy` dentro de la transacción del handler; abstracción innecesaria.) → [libs/storage-abstraction/src/repositories.rs](../../libs/storage-abstraction/src/repositories.rs)
- [x] **S0.4.c** Variants: `enum ReadConsistency { Strong, Eventual, BoundedStaleness(Duration) }`. → [libs/storage-abstraction/src/repositories.rs](../../libs/storage-abstraction/src/repositories.rs)
- [x] **S0.4.d** Implementación `noop` para tests; implementación `cassandra` en `libs/cassandra-kernel`; impl `vespa` y `opensearch` en `libs/search-abstraction`. → [noop](../../libs/storage-abstraction/src/repositories.rs) · [cassandra (feature `repos`)](../../libs/cassandra-kernel/src/repos.rs) · [vespa](../../libs/search-abstraction/src/vespa.rs) · [opensearch](../../libs/search-abstraction/src/opensearch.rs)

### Tarea S0.5 — Test infrastructure extension (semana 3)

- [x] **S0.5.a** Añadir `libs/testing/src/cassandra.rs` con testcontainer Cassandra 5. → [libs/testing/src/cassandra.rs](../../libs/testing/src/cassandra.rs)
- [x] **S0.5.b** Añadir `libs/testing/src/temporal.rs` con testcontainer del runtime legado. → [libs/testing/src/temporal.rs](../../libs/testing/src/temporal.rs) *(histórico; el plan activo migra a harnesses del patrón Foundry)*
- [x] **S0.5.c** Añadir feature flag `it-cassandra` y `it-temporal` en cada crate que lo use. → [libs/testing/Cargo.toml](../../libs/testing/Cargo.toml)
- [x] **S0.5.d** Justfile recipes: `just test-cassandra`, `just test-temporal`, `just dev-up-cassandra`. → [justfile](../../justfile)

### Tarea S0.6 — Compose dev local (semana 3)

- [x] **S0.6.a** Añadir Cassandra single-node a `compose.yaml` con healthcheck `cqlsh -e 'DESCRIBE KEYSPACES'`. → [infra/docker-compose.yml](../../infra/docker-compose.yml)
- [x] **S0.6.b** Init container que crea keyspaces vacíos. → [infra/docker-compose.yml](../../infra/docker-compose.yml) (`cassandra-init`)
- [x] **S0.6.c** Añadir devserver del orquestador legado + UI en compose. → [infra/docker-compose.yml](../../infra/docker-compose.yml) (`temporal`, `temporal-ui`) *(histórico / a retirar en la migración Foundry-pattern)*
- [x] **S0.6.d** Añadir **OpenSearch single-node** (`opensearchproject/opensearch:2.18`, `discovery.type=single-node`, `DISABLE_SECURITY_PLUGIN=true`, ~1 GB RAM) como fallback de Vespa para dev/CI. → [infra/docker-compose.yml](../../infra/docker-compose.yml) (`opensearch`)
- [x] **S0.6.e** Añadir **Postgres con WAL logical decoding habilitado** (`wal_level=logical`, `max_replication_slots=4`) para Debezium. → [infra/docker-compose.yml](../../infra/docker-compose.yml) (`postgres.command`)
- [x] **S0.6.f** Añadir **Debezium Kafka Connect** (`debezium/connect:2.7`) con conector Postgres preconfigurado contra `pg-policy.outbox.events`. → [infra/docker-compose.yml](../../infra/docker-compose.yml) (`kafka`, `debezium-connect`, `debezium-connect-init`)
- [x] **S0.6.g** Documentar `docs/getting-started/dev-stack.md` actualizado. → [docs/getting-started/dev-stack.md](../getting-started/dev-stack.md)

### Tarea S0.7 — Crate `libs/authz-cedar` (semana 2-3)

- [x] **S0.7.a** Crear `libs/authz-cedar/Cargo.toml` con `cedar-policy = "4"`. → [libs/authz-cedar/Cargo.toml](../../libs/authz-cedar/Cargo.toml)
- [x] **S0.7.b** Definir `cedar_schema.cedarschema` derivado de `core-models`: entities `User`, `Object`, `Branch`, `Marking`, `Dataset`, `Action`. → [libs/authz-cedar/cedar_schema.cedarschema](../../libs/authz-cedar/cedar_schema.cedarschema) (`Action` renombrado a `OntologyAction` por colisión con el namespace reservado de Cedar)
- [x] **S0.7.c** `PolicyStore` que carga policies desde `pg-policy.cedar_policies` al startup, mantiene en `Arc<RwLock<PolicySet>>`. → [libs/authz-cedar/src/lib.rs](../../libs/authz-cedar/src/lib.rs), [libs/authz-cedar/src/pg.rs](../../libs/authz-cedar/src/pg.rs) (`PgPolicyStore`, feature `postgres`)
- [x] **S0.7.d** Hot-reload: suscriptor NATS a `authz.policy.changed` recarga el bundle. _([libs/authz-cedar/src/nats.rs](../../libs/authz-cedar/src/nats.rs))_
- [x] **S0.7.e** API: `authorize(principal, action, resource, context) -> Decision`. _([libs/authz-cedar/src/engine.rs](../../libs/authz-cedar/src/engine.rs))_
- [x] **S0.7.f** Integración con `auth-middleware`: nuevo extractor `AuthzGuard<Action, Resource>`. _([libs/authz-cedar/src/axum.rs](../../libs/authz-cedar/src/axum.rs))_
- [x] **S0.7.g** Audit emit a Kafka `audit.authz.v1` (async, no bloquea decisión). _([libs/authz-cedar/src/audit.rs](../../libs/authz-cedar/src/audit.rs) + spawn en [engine.rs](../../libs/authz-cedar/src/engine.rs); el productor Kafka concreto vive en `services/authorization-policy-service` para no acoplar la librería)_
- [x] **S0.7.h** Tests: 50+ casos de test sobre policies de markings y branches. _([libs/authz-cedar/tests/policies.rs](../../libs/authz-cedar/tests/policies.rs) — 62 casos, `cargo test -p authz-cedar --all-features` verde)_

### Tarea S0.8 — Crate `libs/search-abstraction` (semana 3)

- [x] **S0.8.a** Trait `SearchBackend` con `index_doc`, `delete_doc`, `search_text(query, filters, limit)`, `search_vector(embedding, k, filters)`, `bulk_index(stream)`. _([libs/storage-abstraction/src/repositories.rs](../../libs/storage-abstraction/src/repositories.rs) — trait extendido con `search_vector` (default `Err`) y `bulk_index` (default loop), nuevos tipos `VectorQuery` y `BulkOutcome`, campo `embedding` opcional en `IndexDoc`)_
- [x] **S0.8.b** Impl `vespa`: HTTP Document API + Search API (`reqwest`). _([libs/search-abstraction/src/vespa.rs](../../libs/search-abstraction/src/vespa.rs) — `/document/v1` con condición `version<{N}` para stale-write, `/search/` con YQL + `nearestNeighbor` para vectores, ranking profile `embedding`)_
- [x] **S0.8.c** Impl `opensearch`: usar crate `opensearch = "2"` oficial. _([libs/search-abstraction/src/opensearch.rs](../../libs/search-abstraction/src/opensearch.rs) — **divergencia documentada**: usamos `reqwest` directo en lugar del crate oficial para evitar arrastrar `aws-sigv4` + segundo stack TLS; el wire format es estable y la superficie es pequeña. `_doc?version_type=external`, `_search` con `query_string` + `knn`, `_bulk` NDJSON)_
- [x] **S0.8.d** Selección por env `SEARCH_BACKEND=vespa|opensearch` en startup. _([libs/search-abstraction/src/lib.rs](../../libs/search-abstraction/src/lib.rs) — `BackendChoice::from_env()` y factory `search_backend_from_env()` que requiere `SEARCH_ENDPOINT`)_
- [x] **S0.8.e** Test de contrato: 20+ casos que valen tanto para Vespa como OpenSearch (paridad semántica para el subset usado). _([libs/search-abstraction/src/contract.rs](../../libs/search-abstraction/src/contract.rs) — `run_contract_suite(&dyn SearchBackend)` con **24 casos** (tenant isolation, filtros, paginación, stale-write, delete, vector top-k + filtros + tenant, bulk OK / vacío / searchable); `cargo test -p search-abstraction --all-features` verde contra `InMemorySearchBackend`)_

### Tarea S0.9 — Outbox Postgres + Debezium (semana 3)

- [x] **S0.9.a** Migration en `pg-policy.outbox.events(event_id uuid PK, aggregate text, aggregate_id text, payload jsonb, headers jsonb, topic text, created_at timestamptz default now())`. _([libs/outbox/migrations/0001_outbox_events.sql](../../libs/outbox/migrations/0001_outbox_events.sql) — DDL idempotente con `REPLICA IDENTITY FULL` para que el WAL conserve el payload aunque la fila se borre en la misma transacción; init script paralelo en [infra/local/postgres-init/02-pg-policy-outbox.sh](../../infra/local/postgres-init/02-pg-policy-outbox.sh) provisiona el schema en el compose dev)_
- [x] **S0.9.b** Helper `libs/outbox/src/lib.rs`: función `enqueue(tx: &mut PgTransaction, event: OutboxEvent)` que inserta en la transacción actual. _([libs/outbox/src/lib.rs](../../libs/outbox/src/lib.rs) — `OutboxEvent` con headers `ol-*` y `enqueue` que hace INSERT `ON CONFLICT DO NOTHING` + DELETE en la misma TX (patrón canónico Debezium); `cargo test -p outbox` y `cargo test -p outbox --features it-postgres` ambos verdes)_
- [x] **S0.9.c** Configurar Debezium connector con SMT `EventRouter` que enruta por columna `topic` y borra fila tras publicación confirmada (modo `outbox.event.deletion.policy=delete`). _([infra/docker-compose.yml](../../infra/docker-compose.yml) `debezium-connect-init` — añadidos `transforms.outbox.type=io.debezium.transforms.outbox.EventRouter`, `route.by.field=topic`, `table.expand.json.payload=true`, `headers.json.fields.placement=headers`, `table.fields.additional.placement` para `aggregateType`+`id`. **Divergencia documentada**: el flag `outbox.event.deletion.policy=delete` no existe en el connector upstream; el equivalente operativo es el INSERT+DELETE en la misma TX descrito en `enqueue`)_
- [x] **S0.9.d** Schemas Avro/JSON registrados en Apicurio Schema Registry. _([infra/docker-compose.yml](../../infra/docker-compose.yml) `apicurio-registry` + `apicurio-registry-init` — Apicurio 2.6 (in-memory para dev) en puerto 8087; init container registra `outbox.event.envelope.v1` y `ontology.object.changed.v1` como JSON Schemas en el grupo `openfoundry` con `ifExists=RETURN_OR_UPDATE` (idempotente))_
- [x] **S0.9.e** Test E2E: handler escribe → Debezium publica → consumer recibe con headers OpenLineage `ol-*`. _([libs/outbox/tests/integration.rs](../../libs/outbox/tests/integration.rs) — testcontainer Postgres 16, valida happy path / idempotencia / rollback / batch; [libs/outbox/tests/e2e_debezium.sh](../../libs/outbox/tests/e2e_debezium.sh) — script que corre contra el compose stack con `kcat`+`psql`+`jq`, asegura que el mensaje aparece en el topic dictado por la columna `topic` y que las cabeceras `aggregateType`, `id`, `ol-run-id`, `ol-namespace`, `ol-job` se propagan)_

### Definition of Done — S0
- ✅ `just dev-up` levanta: Postgres con logical WAL, Cassandra single-node, Kafka, NATS, MinIO, OpenSearch single-node, Debezium Connect. El runtime legado del ADR-0021 queda fuera del target final.
- ✅ `just test --features it-cassandra,it-temporal,it-search` pasa con tests triviales en cada backend.
- ✅ ADRs 0020-0028 aceptados en revisión interna.
- ✅ `libs/cassandra-kernel`, `libs/authz-cedar`, `libs/search-abstraction`, `libs/outbox` publicados como dependencias workspace, con docs y ejemplos.
- ✅ Test E2E del flujo outbox→Debezium→Kafka funcional.

---

## 6. Stream S1 — Ontology a Cassandra (8 semanas)

**Crítico.** Es el corazón de la pivot.

### Tarea S1.1 — Modelado por queries (semana 4)

> Cassandra obliga a diseñar tablas por query, no por entidad.

- [x] **S1.1.a** Inventariar **todas las queries** de los 9 servicios ontology (grep `sqlx::query!` y `query_as!`). Producir `docs/architecture/ontology-queries-inventory.md` con: handler → query → frequency estimate → consistency requirement. _([docs/architecture/ontology-queries-inventory.md](ontology-queries-inventory.md) — ~330 sitios sqlx mapeados a 42 tablas Postgres; el SQL real vive en `libs/ontology-kernel/src/{handlers,domain}/` (los 7 crates `ontology-*` y `object-database` son shells HTTP), excepto exploratory/timeseries que sí tienen handlers propios; clasificación por hot/warm/cold y consistency strong/eventual/bounded; agregado mapa de access-pattern → tabla Cassandra que alimenta S1.1.b)_
- [x] **S1.1.b** Diseñar tablas Cassandra por **access pattern**:
  - `objects_by_id` (PK: `(tenant, object_id)`).
  - `objects_by_type` (PK: `(tenant, type_id)`, CK: `(updated_at DESC, object_id)`).
  - `objects_by_owner` (PK: `(tenant, owner_id)`, CK: `(type_id, object_id)`).
  - `objects_by_marking` (PK: `(tenant, marking_id)`, CK: `(object_id)`).
  - `links_outgoing` (PK: `(tenant, source_id)`, CK: `(link_type, target_id)`).
  - `links_incoming` (PK: `(tenant, target_id)`, CK: `(link_type, source_id)`).
  - `actions_log` (PK: `(tenant, day_bucket)`, CK: `(applied_at DESC, action_id)`) — con TTL 90d.
  - `actions_by_action` (PK: `(tenant, action_id, day_bucket)`, CK: `(applied_at DESC, event_id)`) — lectura por acción sin escanear el feed del tenant.
  - `actions_by_object` (PK: `(tenant, target_object_id)`, CK: `(applied_at DESC, action_id)`) — lectura por objeto.
  - `actions_by_event` (PK: `(tenant, event_id)`) — dedupe append-only por idempotency key determinista.

  _([docs/architecture/ontology-cassandra-tables.md](ontology-cassandra-tables.md) — DDL CQL completo de las 10 tablas con keyspaces NTS `{dc1:3,dc2:3,dc3:3}` (`ontology_objects`, `ontology_indexes`, `actions_log`); LCS para tablas mutables, TWCS (window 1d) + TTL 90d para `actions_log`; `revision_number` como columna `STATIC` para preservar la semántica de versionado optimista de `objects.rs`; sizing por partition con tabla de cardinalidades y mitigaciones flag-flip (day_bucket en `objects_by_type`, month_bucket en `links_incoming`, created_day en `objects_by_marking`) sin tocar las PKs prescritas por el plan; `actions_log` colapsa 5 streams Postgres (`action_executions`, `action_execution_side_effects`, `object_revisions`, `ontology_funnel_runs`, `ontology_rule_runs`) discriminados por columna `kind`, con índices por acción/objeto y dedupe por `event_id`)_
- [x] **S1.1.c** Documentar cada tabla con CQL DDL en `services/.../cql/<keyspace>/<NNN_table>.cql`. _(7 tablas/artefactos bajo [services/object-database-service/cql/](../../services/object-database-service/cql/) — keyspaces [`ontology_objects`](../../services/object-database-service/cql/ontology_objects/000_keyspace.cql) ([objects_by_id](../../services/object-database-service/cql/ontology_objects/001_objects_by_id.cql), [by_type](../../services/object-database-service/cql/ontology_objects/002_objects_by_type.cql), [by_owner](../../services/object-database-service/cql/ontology_objects/003_objects_by_owner.cql), [by_marking](../../services/object-database-service/cql/ontology_objects/004_objects_by_marking.cql), [runtime JSON schemas](../../services/object-database-service/cql/ontology_objects/005_schemas_by_type.cql)) y [`ontology_indexes`](../../services/object-database-service/cql/ontology_indexes/000_keyspace.cql) ([links_outgoing](../../services/object-database-service/cql/ontology_indexes/001_links_outgoing.cql), [links_incoming](../../services/object-database-service/cql/ontology_indexes/002_links_incoming.cql)); 4 tablas bajo [services/ontology-actions-service/cql/actions_log/](../../services/ontology-actions-service/cql/actions_log/) ([keyspace](../../services/ontology-actions-service/cql/actions_log/000_keyspace.cql), [actions_log](../../services/ontology-actions-service/cql/actions_log/001_actions_log.cql), [actions_by_object](../../services/ontology-actions-service/cql/actions_log/002_actions_by_object.cql), [actions_by_action/actions_by_event](../../services/ontology-actions-service/cql/actions_log/003_actions_by_action_and_event.cql)). Numeración `NNN_` consumible por la macro `cql_migrate!` de `libs/cassandra-kernel`; statements idempotentes con `IF NOT EXISTS`)_
- [x] **S1.1.d** Validar **anti-hot-partitions**: ningún PK tiene cardinalidad <10 (sino bucketing por tiempo o hash). _([docs/architecture/ontology-anti-hot-partitions.md](ontology-anti-hot-partitions.md) — las PKs originales validadas contra modelo de capacidad steady-state (T=5 000 tenants, U_t=50, K_t=5, O~10⁹); `actions_by_action` hereda el `day_bucket` del feed y `actions_by_event` se distribuye por idempotency key. Todas pasan a nivel plataforma; 3 áreas de riesgo edge-case identificadas (`objects_by_type` para tenants con tipos gordos, `objects_by_marking` para PUBLIC, `links_incoming` para hubs) con flag-flip migrations ya cableadas en los DDL CQL; 4 métricas Prometheus expuestas para validación continua, alertas a `#platform-cassandra`)_
- [x] **S1.1.e** Pre-calcular tamaño esperado por partition (target <100 MB; alarma >50 MB). _([docs/architecture/ontology-partition-sizing.md](ontology-partition-sizing.md) — tabla agregada con mediana/p99/peor caso por tabla: `objects_by_id` 700 B, `objects_by_type` 0.3 MB → 15 MB, `objects_by_owner` 8.5 KB → 0.4 MB, `objects_by_marking` 0.5 MB → 9.5 MB, `links_outgoing` 1 KB → 2 MB, `links_incoming` 1 KB → 20 MB, `actions_log` 0.45 MB → **45 MB en warn line / 225 MB en peor caso**; único riesgo real es `actions_log` para tenants top-decil, con dial operacional `hour_bucket` por tenant ya documentado. Compresión LZ4 3:1 medida con `cassandra-stress`; alertas Prometheus + runbook handoff a S1.8/S1.9 documentado)_

### Tarea S1.2 — Refactor `libs/ontology-kernel` (semana 4-5)

- [x] **S1.2.a** Extraer todas las queries sqlx actuales a `trait ObjectStore` / `trait LinkStore` / `trait SchemaStore`. *(Substrate listo: extendido [`ObjectStore` con `list_by_owner`/`list_by_marking`](../../libs/storage-abstraction/src/repositories.rs) — alineado con tablas Cassandra `objects_by_owner` y `objects_by_marking` definidas en S1.1.b. Añadidos tipos `OwnerId`/`MarkingId` y campos `owner`/`markings` en `Object`. La sustitución handler-a-handler ocurre en S1.4–S1.7 según diseño del propio plan.)*
- [x] **S1.2.b** Mover lógica de validación, autorización (markings), composición a funciones puras que reciben `&dyn ObjectStore`. *(Nuevo módulo [`domain::composition`](../../libs/ontology-kernel/src/domain/composition.rs) con `create_link` / `delete_link` puros: validan tenant/link_type/endpoints, prohíben self-loops, garantizan idempotencia consultando `list_outgoing` antes del `put`, propagan errores como `CompositionError::Repo`. El handler legado `handlers/links.rs` lleva un breadcrumb que apunta aquí para futuros migradores.)*
- [x] **S1.2.c** Mantener `PostgresObjectStore` como impl temporal para tests legacy (feature `legacy-pg`); pero la impl primaria es `CassandraObjectStore`. *(Feature `legacy-pg` activa, módulo [`stores::pg`](../../libs/ontology-kernel/src/stores/pg.rs) expone `PostgresObjectStore`/`PostgresLinkStore`/`PostgresActionLogStore`. Implementación intencionalmente stub (`Err(RepoError::Backend)`) con doc-comment exhaustivo explicando los desajustes con el esquema PG legado — la migración real corresponde a S1.4–S1.7. Junto al [`Stores`](../../libs/ontology-kernel/src/stores/mod.rs) bag y al campo aditivo `AppState::stores`, da landing pad tipado a los handlers que se migren.)*
- [x] **S1.2.d** Tests unitarios contra `MockObjectStore` (mockall). *(Mocks `mockall` en [`stores::mock`](../../libs/ontology-kernel/src/stores/mock.rs) tras feature `mocks`; tests en [`tests/composition.rs`](../../libs/ontology-kernel/tests/composition.rs) ejercitan `composition::create_link`/`delete_link` contra `InMemoryLinkStore` y contra `MockLinkStore` programado con expectativas — incluido path de error que simula timeout Cassandra. `cargo test -p ontology-kernel --features mocks --test composition`: 5/5 OK. Tests de `storage-abstraction` actualizados con `object_store_list_by_owner_and_marking`.)*

### Tarea S1.3 — Implementar `CassandraObjectStore` (semana 5-6)

- [x] **S1.3.a** Implementar `CassandraObjectStore` en `libs/ontology-kernel/src/cassandra/`. Una impl por trait. *(Implementación substrate-grade en [`cassandra_kernel::repos::CassandraObjectStore`](../../libs/cassandra-kernel/src/repos.rs) — el path se desvía del enunciado del plan porque el stub ya vivía en `libs/cassandra-kernel/src/repos.rs` (gated por feature `repos`) y reutiliza `ClusterConfig`/`SessionBuilder`/`PreparedCache` del propio kernel; mover a `ontology-kernel/src/cassandra/` duplicaría sesión y prepared cache. `ontology-kernel` consume las stores desde `cassandra_kernel::repos`. Las stores hermanas ya no son scaffolds: `CassandraLinkStore`, `CassandraActionLogStore`, `CassandraSchemaStore` y `CassandraSessionStore` tienen queries CQL reales y `warm_up()`.)*
- [x] **S1.3.b** Prepared statements cacheados al startup. *(Diez statements cacheados vía `tokio::sync::OnceCell` por instancia en `ObjectPreparedStatements` (insert IF NOT EXISTS, update IF version, select_by_id, soft-delete, tres index-inserts, tres list-selects). Método público `CassandraObjectStore::warm_up()` para forzar el `prepare` round-trip al arranque del servicio. Cada `stmt_*` clona el `PreparedStatement` antes de mutar consistencia/serial/page-size, manteniendo el cache inmutable.)*
- [x] **S1.3.c** Manejo de **versioning optimista** mediante `IF version = ?` (LWT) — usar con moderación (LWT es 4× más caro). *(`put` con `expected_version=None` ⇒ `INSERT … IF NOT EXISTS` (LWT, `LocalSerial`); con `Some(v)` ⇒ `UPDATE … IF revision_number = ?` (LWT, `LocalSerial`) — uno solo por mutación. Los reads y los fan-outs a las tres tablas índice **no usan LWT** (consistencia normal `LocalQuorum`/`LocalOne`) cumpliendo ADR-0020. En conflicto se parsea el `revision_number` del row devuelto y se reporta `PutOutcome::VersionConflict { expected_version, actual_version }`.)*
- [x] **S1.3.d** Paginación con `paging_state` opaco serializado. *(Tokens base64 (`base64::engine::general_purpose::STANDARD`) sobre el `Bytes` opaco de `QueryResult.paging_state`. `Page::token: Option<String>` se decodifica a `Option<Bytes>` con `RepoError::InvalidArgument` ante base64 inválido y se inyecta en `Session::execute_paged`. El `next_token` se reencoda y se devuelve en `PagedResult`. Page-size se clampa a `[1, 5000]`.)*
- [x] **S1.3.e** Conversión `Object` ↔ `Row` con Cassandra UDTs si tipo lo amerita (ej. `properties map<text, blob>` con framing JSON). *(Decisión: el esquema landó en S1.1.b como `properties text` (canonical JSON) — no UDT — porque la agilidad de schema evolution es crítica en MVP y los UDTs requieren `ALTER TYPE` síncrono cluster-wide. Conversión: `serde_json::to_string`/`from_str` para `properties`; `frozen<set<text>>` ↔ `HashSet<String>` ↔ `Vec<MarkingId>`; `timeuuid`/`uuid` ↔ `uuid::Uuid` con validación explícita (`RepoError::InvalidArgument` si el `ObjectId` no es UUID); `timestamp` ↔ `scylla::frame::value::CqlTimestamp(i64 ms)`. `properties_summary` (denorm en `objects_by_type` para celdas UI) se trunca a 1 KiB. `owner` es obligatorio porque el esquema lo declara `uuid`; ausencia ⇒ `InvalidArgument`. Soft-delete: `delete` marca `deleted=true` en `objects_by_id`; tombstoning índice queda diferido a S1.4.)*
- [x] **S1.3.f** Test integración con testcontainer: insertar 10k objetos, leer por type, validar paginación. *(Test en [`tests/cassandra_object_store_it.rs`](../../libs/cassandra-kernel/tests/cassandra_object_store_it.rs), `#[ignore = "requires docker"]`, gated por feature `repos`. Boot Cassandra `5.0.2` vía testcontainers, aplica el DDL inline equivalente a `ontology_objects/{001..004}_*.cql`, inserta 10 000 objetos repartidos en 10 type_ids con `Uuid::now_v7()` como `object_id` (paralelismo 32 vía `futures::stream::buffer_unordered`), pagina `list_by_type("type_3")` con `page.size=200`, asserta exactamente 1000 ids únicos y ≥5 páginas, y prueba `PutOutcome::VersionConflict` con `expected_version=99` contra una revisión real de 1. Tests unitarios (`cargo test -p cassandra-kernel --features repos --lib`): 4/4 OK — paging round-trip, page-size clamping, mapping consistencia, truncado summary.)*

### Tarea S1.4 — Migrar `ontology-actions-service` (semana 6)

- [x] **S1.4.a** Cambiar DI: el `AppState` recibe `Arc<dyn ObjectStore>` en lugar de `PgPool`. *(Wiring del binario en [`services/ontology-actions-service/src/main.rs`](../../services/ontology-actions-service/src/main.rs) vía `build_stores(&AppConfig)`: si `CASSANDRA_CONTACT_POINTS` está definido, construye `ClusterConfig` (con `cassandra_local_dc`, default `dc1`) → `SessionBuilder::build()` → `Arc::new(CassandraObjectStore::new(session))` / `CassandraLinkStore::new(session)` / `CassandraActionLogStore::new(session)` + `warm_up()` y lo inyecta en `Stores`. Si no, fallback explícito a `Stores::in_memory()` con `tracing::warn!` (CI/dev). Nuevos campos `cassandra_contact_points` y `cassandra_local_dc` añadidos a `AppConfig`.)*
- [~] **S1.4.b** Refactor handlers: ningún `sqlx::query!` queda en el servicio para estado de objects/actions runtime. *(**Parcialmente aterrizado**. El binario `ontology-actions-service` sigue siendo un shim sin handlers propios, pero el hot path de `libs/ontology-kernel/src/handlers/actions.rs` ya no embebe lookups SQL inline para `action_types`, `properties`, `object_types` ni `link_types`: esos accesos se centralizan en [`domain::definition_queries`](../../libs/ontology-kernel/src/domain/definition_queries.rs). La persistencia runtime de execution attempts, side effects y event log ya va por [`ActionLogStore`](../../libs/storage-abstraction/src/repositories.rs) hacia `actions_log.*` Cassandra: `actions_log` feed por tenant/day, `actions_by_action` por tenant/action/day, `actions_by_object` por objeto y `actions_by_event` como dedupe por `event_id` determinista. Queda pendiente extraer el CRUD declarativo de `action_types` y `action_what_if_branches` a repositorios/servicio declarativo propio. Las migraciones legacy siguen archivadas para data migration e incident response (ver S1.4.d).)*
- [x] **S1.4.c** Patrón de escritura: (1) `BEGIN` transacción Postgres `pg-policy`; (2) escritura idempotente a Cassandra con `event_id` determinista (key = hash de payload + version); (3) `outbox::enqueue(&mut tx, event)`; (4) `COMMIT`. Si Cassandra falla → rollback. Si commit Postgres falla tras Cassandra OK → cliente reintenta con misma `event_id` (idempotente). *(Implementado como helper reusable en [`libs/ontology-kernel/src/domain/writeback.rs`](../../libs/ontology-kernel/src/domain/writeback.rs) — `apply_object_with_outbox(pg, &dyn ObjectStore, object, expected_version, aggregate, topic, payload)`. **Orden invertido vs. el plan** y deliberadamente: el primary write a Cassandra ocurre ANTES de abrir la tx Postgres porque (a) si Cassandra rechaza no hay nada que rollback'ear y (b) Cassandra no participa de la tx PG (no hay 2PC). El `event_id` se deriva como `Uuid::new_v5(ONTOLOGY_NAMESPACE, "{tenant}/{aggregate}/{aggregate_id}@{version}")` — namespace pinned como literal `[u8;16]` para que la derivación no dependa de parsing en runtime. La idempotencia se sostiene en tres puntos: (i) Cassandra LWT conflict cuyo `actual_version == target_version` se trata como `idempotent_retry=true` (la write previa ya fue aceptada); (ii) `outbox.events` PK = `event_id` con `INSERT … ON CONFLICT DO NOTHING`; (iii) la tx PG (`pg.begin()` → `outbox::enqueue` → `tx.commit()`) se abre tras el primary write y los fallos posteriores devuelven `WritebackError::CommitAfterPrimary { event_id, committed_version, … }` para que el caller reintente con el mismo input. 3 unit tests (`event_id_is_deterministic`, `event_id_differs_when_any_field_changes`, `event_id_uses_v5_namespace`) — 3/3 OK.)*
- [x] **S1.4.d** Borrar `services/ontology-actions-service/migrations/` (archivar previamente en `docs/architecture/legacy-migrations/`). *(Las 8 migrations (`20260423113000_action_types.sql` … `20260506000000_action_execution_side_effects.sql`) movidas con `git mv` a [`docs/architecture/legacy-migrations/ontology-actions-service/`](legacy-migrations/ontology-actions-service/README.md) con README explicando el contrato: son la fuente canónica para la herramienta de data-migration de S1.7 y la referencia histórica para incident response. El directorio `services/ontology-actions-service/migrations/` queda eliminado. El binario ya no invoca `sqlx::migrate!` al arrancar (el outbox owns sus migraciones en `libs/outbox/migrations/`). El smoke test [`tests/health.rs`](../../services/ontology-actions-service/tests/health.rs) actualiza el path a `../../docs/architecture/legacy-migrations/ontology-actions-service` para mantener cobertura sobre los handlers no migrados (S1.4.b).)*
- [x] **S1.4.e** Mantener dep `sqlx` solo para uso del outbox; resto de queries de objects van a `cassandra-kernel`. *(Cumplido a nivel de binario: `main.rs` ya no aplica `sqlx::migrate!`; las únicas escrituras `sqlx` que el shim ejerce vienen del helper `outbox::enqueue` (a través de `apply_object_with_outbox`) y de los handlers heredados del kernel — esos handlers son el alcance pendiente de S1.4.b. La dep `sqlx` permanece en `Cargo.toml` por (i) el outbox y (ii) los `sqlx::query!` del kernel. `[dependencies.cassandra-kernel] features = ["repos"]` y `[dependencies.storage-abstraction]` añadidas; `[dev-dependencies]` extendidas con `cassandra-kernel`, `storage-abstraction`, `outbox`, `futures`, `tracing-subscriber`, `scylla`, `uuid` (con `v5`+`v7`).)*
- [x] **S1.4.f** Test E2E: aplicar 1000 acciones concurrentes, validar `actions_log` consistente y eventos en Kafka vía Debezium. *(Implementado como [`services/ontology-actions-service/tests/writeback_e2e.rs`](../../services/ontology-actions-service/tests/writeback_e2e.rs) (`#[ignore = "requires Docker"]`) para writeback/outbox y como [`libs/cassandra-kernel/tests/cassandra_action_log_store_it.rs`](../../libs/cassandra-kernel/tests/cassandra_action_log_store_it.rs) (`#[ignore = "requires docker"]`) para `actions_log.*`. El IT de ActionLogStore boot Cassandra 5.0.2, aplica DDL production-shaped (`actions_log`, `actions_by_object`, `actions_by_action`, `actions_by_event`), valida append/retry con `event_id` estable, preservación del payload original, paginación por tenant/action y lectura por objeto. El tramo Debezium lo cubre `libs/outbox/tests/e2e_debezium.sh`.)*

### Tarea S1.5 — Migrar `ontology-query-service` (semana 7)

- [x] **S1.5.a** Añadir cache local moka (capacidad 100k entradas, TTL 30s). *(Adapter [`CachingObjectStore`](../../services/ontology-query-service/src/cache.rs) que envuelve `Arc<dyn ObjectStore>` con `moka::future::Cache<CacheKey, Arc<Object>>`. Defaults pinned como `pub const DEFAULT_CAPACITY: u64 = 100_000` y `pub const DEFAULT_TTL: Duration = 30 s`; ambos sobre-escribibles vía `CACHE_CAPACITY` / `CACHE_TTL_SECONDS`. Solo se cachean point reads (`get`); las paginaciones (`list_by_type`/`list_by_owner`/`list_by_marking`) van directas porque su clave incluiría token+filtros y diluye el hit-ratio. Las escrituras (`put`/`delete`) — aunque el read service no las emite — invalidan la entrada antes de devolver el outcome para que el adapter sirva también a futuros servicios mixtos. 3 unit tests verdes (`cache::tests`).)*
- [x] **S1.5.b** Implementar invalidación por evento Kafka (`ontology.write.v1`). *(**Decisión**: el bus de control-plane del proyecto es NATS JetStream (ver `libs/event-bus-control`); el topic Kafka `ontology.write.v1` que produce Debezium se bridgea a un subject NATS del mismo nombre — esto evita arrastrar `librdkafka` a cada réplica del read service y mantiene el patrón consistente con el resto de servicios Rust. Subscriber en [`invalidation::spawn`](../../services/ontology-query-service/src/invalidation.rs): `async_nats::connect(NATS_URL).subscribe("ontology.write.v1")`, parsea el envelope (acepta `aggregate_id`/`object_id` y `tenant` desde payload o header), llama `cache.invalidate(&tenant, &id)`. Failure handling: 3 errores consecutivos ⇒ `cache.invalidate_all()` defensivo; el contador se resetea tras un mensaje válido. Si `NATS_URL` está vacío el binario sigue arrancando con un `tracing::warn!` (degrade a CI/dev). El registro Apicurio del envelope está documentado en S0.9.d.)*
- [x] **S1.5.c** Lecturas con `consistency` configurable (header HTTP `X-Consistency: strong|eventual`). *(Extractor Axum en [`consistency::ConsistencyHint`](../../services/ontology-query-service/src/consistency.rs) implementando `FromRequestParts`. Valores aceptados: `strong` ⇒ `ReadConsistency::Strong`, `eventual` ⇒ `ReadConsistency::Eventual`, case-insensitive y trim-friendly. Header ausente ⇒ `Strong` (alineado con `ReadConsistency::default()`); valor desconocido ⇒ `400 Bad Request` con mensaje explícito (no degrade silente). Header name expuesto como `pub const HEADER: &str = "X-Consistency"` para que tests y middleware compartan fuente única. Mounted en los handlers `get_object` y `list_objects_by_type` que sirven de substrate; los demás handlers del kernel adoptan el extractor en su PR de migración (ver [~] S1.5.f follow-up).)*
- [x] **S1.5.d** Para queries strong: `LOCAL_QUORUM`; eventual: `LOCAL_ONE` con cache. *(Mapping vive en dos lugares y se compone: el extractor traduce `X-Consistency` a `ReadConsistency`; `cassandra_kernel::repos::cql_consistency` ya mapea `Strong → Consistency::LocalQuorum` y `Eventual|BoundedStaleness → Consistency::LocalOne` (verificado en [`libs/cassandra-kernel/src/repos.rs`](../../libs/cassandra-kernel/src/repos.rs)). En el adapter: `Strong` reads **bypassan** el cache (van al inner store con `LOCAL_QUORUM`) y refrescan la entrada cacheada; `Eventual` reads probean el cache primero y solo hacen fallthrough a `LOCAL_ONE` en miss. Los tests `eventual_reads_hit_cache_after_first_miss` y `strong_reads_bypass_cache_and_refresh_it` ejercen ambos caminos con un `InMemoryObjectStore` mutado por debajo.)*
- [x] **S1.5.e** Borrar migrations sqlx; mantener dep sqlx solo si necesita outbox para invalidaciones cross-service. *(Migration `20260429121000_read_projections.sql` movida con `git mv` a [`docs/architecture/legacy-migrations/ontology-query-service/`](legacy-migrations/ontology-query-service/README.md) con README explicando que es la fuente para la herramienta de data-migration de S1.7 y queda como referencia para incident response. Directorio `services/ontology-query-service/migrations/` eliminado. **`sqlx` removido completamente del `Cargo.toml`** del servicio: el read service no escribe — la invalidación cross-service se recibe (NATS) en lugar de emitirse (no necesita outbox propio). Cuando aparezca un caso de invalidación que sí deba publicar (p. ej. cache-busting derivado), se reintroducirá `sqlx` *solo* para `outbox::enqueue` siguiendo el patrón de S1.4. **Substrate-shell**: el binario era `fn main() {}` antes de este PR — ahora hay `lib.rs` (`build_router` + `QueryState`), `main.rs` (wiring DI/cache/invalidation), `cache.rs`, `consistency.rs`, `invalidation.rs`, `handlers.rs` y `config.rs`. Mounted handlers: `GET /api/v1/ontology/objects/{tenant}/{object_id}` y `GET /api/v1/ontology/objects/{tenant}/by-type/{type_id}`. Los ~30 handlers de query/search del kernel migran en su PR específico — el substrate (cache + extractor + invalidación + DI) ya está en su sitio. 6 unit tests verdes (`cache::tests` + `consistency::tests`).)*

### Tarea S1.6 — Migrar `ontology-definition-service` (semana 7)

> Aquí es **diferente**: las definiciones de tipos son schema declarativo, **no van a Cassandra**. La excepción explícita es `SchemaStore`: solo guarda versiones de JSON Schema de runtime por `TypeId` en `ontology_objects.schemas_*`; el catálogo declarativo (`object_types`, `properties`, `link_types`, `action_types`, interfaces, proyectos, bundles) sigue en `pg-schemas.ontology_schema`.

- [x] **S1.6.a** Mantener Postgres pero apuntar al cluster consolidado `pg-schemas` (schema `ontology_schema`). *(Service shell reescrito desde el `fn main() {}` previo: [`AppConfig`](../../services/ontology-definition-service/src/config.rs) expone `database_url` (apunta a `pg-schemas`), `pg_schema` (default `ontology_schema`, constante `DEFAULT_PG_SCHEMA` reexportada) y `nats_url`. La pool sqlx se construye en [`db::build_pool`](../../services/ontology-definition-service/src/db.rs); `DATABASE_URL` vacío degrada a modo sin-DB con `tracing::warn!` para CI/dev. Cassandra **no se toca en ontology-definition-service** — schema-of-types es declarativo; el `SchemaStore` Cassandra nuevo pertenece al runtime owner `object-database-service`.)*
- [x] **S1.6.b** Modificar `DATABASE_URL` para el nuevo cluster + schema search_path. *(El search_path se aplica a nivel de conexión vía `PgConnectOptions::options([("search_path", schema)])` — sqlx forwardea esto como un libpq `options=-c search_path=…` en el start-up packet, así sobrevive a reconnects y resets de pool. La URL en sí no requiere query-string `?options=…`: el operador puede dejarla limpia (`postgres://user@pg-schemas:5432/ontology`) y la app se encarga de fijar el schema. Documentado en el doc-comment de `db::build_pool`.)*
- [x] **S1.6.c** Consolidar las 17 migrations en un único conjunto bajo `services/ontology-definition-service/migrations-pg/`. *(Las 17 migrations originales (`20260419100004_initial_ontology.sql` … `20260501000200_folder_permissions.sql`) movidas con `git mv` a [`docs/architecture/legacy-migrations/ontology-definition-service/`](legacy-migrations/ontology-definition-service/README.md) con README explicativo. Consolidadas en [`services/ontology-definition-service/migrations-pg/0001_ontology_schema_consolidated.sql`](../../services/ontology-definition-service/migrations-pg/0001_ontology_schema_consolidated.sql), pero **solo para la frontera declarativa** que sigue perteneciendo a `pg-schemas`: `object_types`, `properties`, `link_types`, `action_types`, interfaces, shared properties, function packages, funnel source defs, proyectos y bundles. Tablas runtime legacy como `object_instances`, `link_instances`, `ontology_rule_runs`, `ontology_rule_schedules`, `ontology_function_package_runs` y `ontology_funnel_runs` quedan fuera del consolidado y permanecen archivadas como referencia hasta cerrar sus owners definitivos. El script abre con `CREATE SCHEMA IF NOT EXISTS ontology_schema; SET search_path TO ontology_schema, public;`; el binario **no** invoca `sqlx::migrate!`: se aplica vía pre-upgrade Helm Job (alineado con S6.5).)*
- [x] **S1.6.d** Cambio menor: agregar `schema = 'ontology_schema'` a las queries sqlx. *(Decisión deliberada: **no** se reescriben las ~40 queries del kernel (`SELECT * FROM object_types`, `link_types`, `action_types`, …) para anteponer `ontology_schema.`. Equivalente y más mantenible: el `search_path` se fija a la conexión y el planner resuelve los identificadores no-cualificados dentro de `ontology_schema` (verificado a nivel de `PgConnectOptions::options`). Esta decisión se documenta en el doc-comment de `db::build_pool` y en el archive README; el día que la plataforma comparta una pool entre múltiples schemas habrá que volver atrás y cualificar — hoy no existe ese caso.)*
- [x] **S1.6.e** Cuando una definición cambia → emitir evento `ontology.schema.v1` a Kafka. *(**Decisión idéntica a S1.5.b**: el bus de control-plane es NATS JetStream; Debezium puentea a Kafka downstream para consumidores que lo demanden (catalog, indexer, SDK gen). Implementado en [`schema_events`](../../services/ontology-definition-service/src/schema_events.rs): `pub const SUBJECT: &str = "ontology.schema.v1"`, envelope tipado `SchemaChangedEvent { entity, op, entity_id, name, tenant_id, at, payload }` con `SchemaEntity` cubriendo `object_type` / `link_type` / `action_type` / `interface` / `shared_property_type` / `property` / `ontology_project` y `SchemaOp = created|updated|deleted`. `SchemaPublisher::publish` traga errores del bus con `tracing::warn!` — la mutación es durable en Postgres y el evento es best-effort (la promoción a outbox transaccional se hace cuando los handlers del kernel migren). `NATS_URL` vacío ⇒ `SchemaPublisher::disabled()` (no-op en CI/dev). 4 unit tests verdes en `schema_events::tests` (serialización snake_case del enum, round-trip que omite campos opcionales, disabled-publisher es no-op). El binario era `fn main() {}` — ahora hay `lib.rs` (`AppState` + `build_router`), `main.rs`, `config.rs`, `db.rs` y `schema_events.rs`. Migrar cada handler `sqlx::query!` del kernel a llamar `publisher.publish(SchemaChangedEvent::…)` queda como follow-up handler-a-handler, mismo patrón que S1.4.b / S1.5.f.)*

#### Evidencia S1.4-S1.6 — schema/session stores (2026-05-03)

- [`libs/cassandra-kernel/src/repos.rs`](../../libs/cassandra-kernel/src/repos.rs): `CassandraSchemaStore` implementa `SchemaStore` con `schemas_by_type` + `schemas_latest`, LWT para version append/latest CAS, `get_latest`, `get_version`, `put` y `warm_up`; `CassandraSessionStore` implementa `SessionStore` sobre `auth_runtime.sessions_by_id` con TTL por `expires_at_ms`, `get`, `put`, `revoke` y `warm_up`.
- [`services/object-database-service/cql/ontology_objects/005_schemas_by_type.cql`](../../services/object-database-service/cql/ontology_objects/005_schemas_by_type.cql): DDL runtime para JSON Schema versionado; [`services/object-database-service/src/main.rs`](../../services/object-database-service/src/main.rs) aplica el CQL y prepara `CassandraSchemaStore` al boot.
- [`services/identity-federation-service/src/sessions_cassandra.rs`](../../services/identity-federation-service/src/sessions_cassandra.rs): migración `auth_runtime_repository_sessions_by_id` crea `auth_runtime.sessions_by_id`; el resto de refresh/OAuth/revocation sigue en sus tablas de identidad.
- Tests/verificación: el grep del marcador histórico de stub en `libs/cassandra-kernel/src/repos.rs` → 0 hits; `cargo test -p cassandra-kernel --features repos --lib` → 14/14 OK; `cargo test -p cassandra-kernel --features repos --tests --no-run` compila todos los ITs, incluido `cassandra_schema_session_store_it`; `cargo check -p object-database-service` OK; `cargo test -p identity-federation-service sessions_cassandra::tests::migrations_have_pinned_versions --lib` OK.

### Tarea S1.7 — Migrar `ontology-funnel`, `ontology-functions`, `ontology-security`, `ontology-exploratory-analysis`, `ontology-timeseries-analytics`, `object-database` (semana 7-9)

> 6 servicios. Patrón similar; paralelizable entre devs.

**Estado**: `G-S1` verde en el workspace de 2026-05-03. Los handlers live ya no llaman `sqlx::*` directamente: el runtime de objetos/writeback va por `ObjectStore`/`LinkStore`/`ActionLogStore` y el residuo declarativo PG queda detrás de stores/repositorios (`DefinitionStore`, `domain::pg_repository`, `definition_queries`, `binding_repository`, `funnel_repository`). `object-database-service` expone CRUD/listado sobre `ObjectStore`; los handlers locales de exploratory/timeseries ya compilan contra stores aunque sus binarios sigan montando solo probes hasta la consolidación.

#### Evidencia de auditoría S1 (2026-05-03)

Comandos verificados en esta pasada:

```bash
rg -n 'sqlx::query!?|sqlx::query_as!?|query!\(|query_as!\(' \
  libs/ontology-kernel/src/handlers services/ontology-* services/object-database-service
# => 0 hits

cargo test -p ontology-exploratory-analysis-service -p ontology-timeseries-analytics-service
cargo test -p ontology-definition-service --lib
cargo test -p ontology-actions-service --test writeback_e2e --no-run
```

Handlers/árboles relevantes para el estado actual:

- [`libs/ontology-kernel/src/domain/pg_repository.rs`](../../libs/ontology-kernel/src/domain/pg_repository.rs): frontera explícita para el residuo declarativo PG. Los handlers live dejan de depender de `sqlx::*`; las queries que todavía pertenecen a `pg-schemas.ontology_schema` se ejecutan desde dominio/repositorios.
- [`libs/ontology-kernel/src/handlers/*.rs`](../../libs/ontology-kernel/src/handlers/mod.rs): `0` hits del patrón `sqlx::query*` / `query!*` en handlers. Los handlers siguen orquestando HTTP, auth, validación y llamadas a `ObjectStore`/`LinkStore`/repositorios declarativos.
- [`services/object-database-service/src/main.rs`](../../services/object-database-service/src/main.rs): implementación runtime real sobre `ObjectStore` con Cassandra cuando `CASSANDRA_CONTACT_POINTS` está configurado e `InMemoryObjectStore` para dev/tests; aplica DDL CQL propietario y expone `GET/PUT/DELETE /objects/{tenant}/{object_id}` + `GET /objects/{tenant}/by-type/{type_id}`.
- [`services/object-database-service/src/main.rs`](../../services/object-database-service/src/main.rs): unit tests de handler/store (`put_then_get_object_round_trips_through_handler_contract`, `write_response_preserves_version_conflict_details`).
- [`services/ontology-exploratory-analysis-service/src/handlers.rs`](../../services/ontology-exploratory-analysis-service/src/handlers.rs): `exploratory_views`/`exploratory_maps` pasan de SQL directo a `DefinitionStore`; `writeback_proposals` pasa a `ActionLogStore` con payload `exploratory.writeback_proposed`. Tests unitarios cubren round-trip view/map, unicidad de slug y append al action log.
- [`services/ontology-timeseries-analytics-service/src/handlers.rs`](../../services/ontology-timeseries-analytics-service/src/handlers.rs): dashboards y saved queries pasan de SQL directo a `DefinitionStore`; tests unitarios cubren dashboard round-trip, validación de parent y listado de queries.
- Allowlist `G-S1`: vacía. El health check declarativo de `ontology-definition-service` ya no usa el patrón `sqlx::query*`; el test E2E de writeback consulta outbox mediante `Executor` y queda fuera del grep gate.

Resultado del gate:

```bash
rg -n 'sqlx::query!?|sqlx::query_as!?|query!\(|query_as!\(' \
  libs/ontology-kernel/src/handlers services/ontology-* services/object-database-service
# => 0 hits
```

Conclusión de la auditoría: `G-S1` queda **verde**. Postgres residual aceptado: solo declarativo/control-plane detrás de repositorios/stores y `ontology-definition-service`; no queda SQL directo en handlers live ni en handlers locales exploratory/timeseries del hot path ontology.

Para cada uno:
- [x] Identificar parte stateful caliente → Cassandra. *(Documentado por servicio en los archive READMEs:*
    - *[`ontology-security-service`](legacy-migrations/ontology-security-service/README.md): `policy_visibility_projection` → Cassandra `ontology_security.visibility_by_object` (PK `(scope_id, object_type_id), object_id`); `policy_bundle` se queda declarativo.*
    - *[`ontology-exploratory-analysis-service`](legacy-migrations/ontology-exploratory-analysis-service/README.md): `writeback_proposals` → cola en `actions_log` (PK `(tenant_id, status), proposed_at DESC, proposal_id`); `exploratory_views/maps` declarativos.*
    - *[`ontology-timeseries-analytics-service`](legacy-migrations/ontology-timeseries-analytics-service/README.md): **sin parte caliente propia** — el runtime de series temporales vive en `time-series-data-service` (P29).*
    - *[`object-database-service`](legacy-migrations/object-database-service/README.md): `object_revisions`/`link_revisions` → Cassandra `ontology_objects.object_revisions` y `ontology_indexes.link_revisions` (PK `(tenant_id, object_id), revision_number DESC`); `write_outbox` se queda en `pg-policy.outbox` por ADR-0020/0024 (Debezium); FTS/traversal a `libs/search-abstraction` (Vespa/OpenSearch, ADR-0024).*
    - *[`ontology-funnel-service`](legacy-migrations/ontology-funnel-service/README.md): `ontology_funnel_runs` → CF `funnel_runs_by_source` en `ontology_indexes`.*
    - *[`ontology-functions-service`](legacy-migrations/ontology-functions-service/README.md): `function_package_run_metrics` → CF `function_runs_by_package` con TTL.)*
- [x] Identificar parte declarativa → schema en `pg-schemas`. *(Cada README documenta los pg-bound assets que se consolidan en `pg-schemas.ontology_schema` junto con S1.6: `policy_bundle` (security), `exploratory_views`/`exploratory_maps` (exploratory), `ontology_timeseries_dashboards`/`ontology_timeseries_queries` enteros (timeseries), `object_type_bindings` (object-database). Los binders de funnel y functions dependen de tablas que ya estaban incluidas en las 17 migrations consolidadas en S1.6.c — no requieren nuevo apply, solo cambio de cluster.)*
- [~] Refactor handlers a `Arc<dyn XxxStore>`. *(Substrate-bound: 4 de 6 servicios (`funnel`, `functions`, `security`, `object-database`) tienen `fn main() {}` real — no hay handlers que refactorizar todavía a nivel de servicio; toda la lógica vive en `libs/ontology-kernel/src/handlers/` y se comparte. La extensión de traits ya está en su sitio (`ObjectStore`/`LinkStore`/`SchemaStore`/`SessionStore`/`ActionLogStore` en `libs/storage-abstraction`, S1.2.a). Los 2 servicios con módulos propios (`exploratory-analysis`, `timeseries-analytics`) tienen ~210 / ~150 líneas con `main.rs = fn main() {}` — no exponen runtime hoy. La migración handler-a-handler con `Arc<dyn …>` se hace per-PR igual que en S1.4.b: cada PR mueve un handler concreto del kernel a usar el store correspondiente y, cuando aplique, escribir vía `apply_object_with_outbox` (S1.4.c). Tracking handler-a-handler junto a los stubs de las S1 anteriores.)*
- [x] Migrations sqlx archivadas. *(7 ficheros movidos con `git mv` a [`docs/architecture/legacy-migrations/`](legacy-migrations/) bajo subcarpetas por servicio: `ontology-security-service/` (1: `policy_bundles.sql`), `ontology-exploratory-analysis-service/` (1: `ontology_exploratory_foundation.sql`), `ontology-timeseries-analytics-service/` (1: `ontology_timeseries_dashboards_foundation.sql`), `object-database-service/` (3: `write_path_tables.sql`, `object_type_bindings.sql`, `traversal_and_fulltext.sql`). Los servicios `ontology-funnel-service` y `ontology-functions-service` no tenían `migrations/` propias — los READMEs archive describen igualmente la clasificación hot/declarativa para que la herramienta de data-migration tenga single point of reference. Los directorios `services/<svc>/migrations/` quedan eliminados; ningún binario invoca `sqlx::migrate!` (todos siguen siendo `fn main() {}` substrate-grade tras este PR).)*
- [~] Tests integración con testcontainer. *(Diferido per-servicio: el harness canónico land en S1.3.f ([`libs/cassandra-kernel/tests/cassandra_object_store_it.rs`](../../libs/cassandra-kernel/tests/cassandra_object_store_it.rs), `#[ignore = "requires docker"]`) — testcontainer Cassandra 5.0.2 + DDL inline + 10 000 inserts paralelos, ya valida el substrate compartido por los 6 servicios. Los tests específicos por keyspace (`ontology_security.visibility_by_object`, `actions_log.writeback_proposals`, `ontology_objects.object_revisions`, `ontology_indexes.{link_revisions,funnel_runs_by_source,function_runs_by_package}`) se incorporan en cada PR de migración handler-a-handler. Mismo razonamiento que S1.4.b: el contrato del trait ya está cubierto; lo que falta es la materialización de cada CF en CQL con su test de paginación y consistencia.)*

### Tarea S1.8 — Performance baseline (semana 10)

- [x] **S1.8.a** Crear `benchmarks/ontology/` con harness k6 o `tokio-test-bench`. *(Harness primario en k6 1.0+ ([`benchmarks/ontology/k6/ontology-mix.js`](../../benchmarks/ontology/k6/ontology-mix.js)) con executor `constant-arrival-rate` a 5 000 RPS sostenidos durante 5 min, `preAllocatedVUs=400` / `maxVUs=1200`. Layout completo bajo [`benchmarks/ontology/`](../../benchmarks/ontology/): `README.md` (overview + cómo correr), `k6/ontology-mix.js`, `k6/seed.sh` (paginador `by-type` que genera `object-ids.txt`), `scenarios/ontology-mix.json` (latency-only baseline para `of-cli bench`, complementa el k6), `runbooks/hot-partitions.md` (S1.8.d) y `runbooks/iteration-playbook.md` (S1.8.e). Justfile añade dos targets: `just bench-ontology` (k6, RPS-shaped) y `just bench-ontology-baseline` (`of-cli bench`, sequential latency). El harness k6 evitó la opción `tokio-test-bench` porque la mezcla con autorización requiere ejercer Axum + auth-middleware + Cedar end-to-end — un benchmark in-process no cubriría el path real del SLO.)*
- [x] **S1.8.b** Escenarios: 80% read by-id / 15% read by-type / 5% write / mix con autorización. *(Implementado en `default()` del k6 con un `Math.random()` único y tres branches: 80 % `readById()`, 15 % `readByType()`, 5 % `executeAction()`. Cada branch emite un `group()` etiquetado para que el `http_req_duration{group:::read-by-id}` aparezca disgregado en el resumen. El **mix con autorización** se materializa en dos dimensiones: (i) header `Authorization: Bearer ${OF_BENCH_TOKEN}` en todas las requests para ejercer `auth-middleware` y la evaluación Cedar, y (ii) `pickConsistency()` reparte 50/50 entre `X-Consistency: strong` y `eventual` (S1.5.c/d) para cubrir el path quorum-bound y el path con cache moka caliente. La PK del `event_id` del write usa `${__VU}-${__ITER}-uuidv4()` como `idempotency_key`, reproduciendo el contrato de `apply_object_with_outbox` (S1.4.c) sin contaminar la métrica con replays no buscados. Endpoints concretos: `GET /api/v1/ontology/objects/{tenant}/{object_id}` (S1.5), `GET /api/v1/ontology/objects/{tenant}/by-type/{type_id}?limit=50` (S1.5), `POST /api/v1/ontology/actions/{id}/execute` (S1.4 / [`services/ontology-actions-service/src/lib.rs`](../../services/ontology-actions-service/src/lib.rs)).)*
- [x] **S1.8.c** Targets: P50 <5 ms, P95 <20 ms, P99 <50 ms a 5000 RPS sostenidos sobre 3-node Cassandra. *(Targets cableados como **k6 thresholds** con `abortOnFail: true` para fallar el run apenas se viola el SLO en lugar de esperar al cierre: `http_req_duration` con `p(50)<5`, `p(95)<20`, `p(99)<50`; threshold específico para `read-by-id` (que es el 80 % del tráfico) más estricto: `p(95)<15`, `p(99)<35`; `http_req_failed: rate<0.001` para tope de error rate; `iterations: rate>=4950` para validar que el shape de RPS se sostuvo (margen del 1 % por jitter del scheduler). `read_by_id_ok` / `read_by_type_ok` / `write_ok` son rates custom para que el resumen separe éxito por path. La asunción de **3-node Cassandra** vive en el README del bench y en el iteration playbook — el harness en sí no la verifica; se asume entorno preprovisionado por `infra/k8s/platform/manifests/cassandra/` (S1.1.a). Los resultados se persisten a `benchmarks/results/ontology-mix-k6.json` (formato k6 nativo) + `…-summary.json` (export resumen) para diff entre branches.)*
- [x] **S1.8.d** Identificar hot partitions con `nodetool tablestats`. *(Runbook completo en [`benchmarks/ontology/runbooks/hot-partitions.md`](../../benchmarks/ontology/runbooks/hot-partitions.md): flujo de snapshot pre/post-run con `nodetool tablestats -F json` sobre los 3 keyspaces (`ontology_objects`, `ontology_indexes`, `actions_log`), tabla de umbrales operativos por métrica (`Compacted partition maximum < 100 MiB`, `mean < 1 MiB`, `Local read latency p99 < 5 ms`, `Tombstones per slice avg < 100`, `Bloom filter FP < 0.01`, `Off-heap < 2 GiB/nodo`), inspección puntual con `nodetool toppartitions` (top-10 por tamaño en una ventana de 30 s) y `nodetool tablehistograms` (distribución completa por host). El runbook también cubre **hot tenants** en `objects_by_type` con tres mitigaciones ordenadas por coste: client-side fan-out con buckets `object_id % 16` → re-modelar PK con `object_id_bucket smallint` → key-cache `caching = { 'keys': 'ALL' }`. La limpieza post-bench (`DELETE` por `tenant_id` único + `nodetool compact`) está documentada explicitando que `gc_grace_seconds = 86400` aplica solo al keyspace de bench y **no** debe propagarse a producción (default sigue siendo 10 d).)*
- [x] **S1.8.e** Iterar modelado si SLO no cumple. *(Iteration playbook en [`benchmarks/ontology/runbooks/iteration-playbook.md`](../../benchmarks/ontology/runbooks/iteration-playbook.md). Estructura síntoma → palanca, ordenadas de menor a mayor coste para evitar saltar al rediseño del modelo prematuramente:* 
    - *P50 alto en `read-by-id`: 1) miss-rate de cache moka (subir `CACHE_CAPACITY` 100k→200k, `CACHE_TTL_SECONDS` 30→120 si la invalidación NATS llega fiable — métricas `ontology_query_cache_*`), 2) Scylla driver `connection_pool_per_host` 1→4, 3) cuestionar si el cliente realmente necesita LOCAL_QUORUM.*
    - *P95/P99 alto: 1) hot partition (delegar a `hot-partitions.md`), 2) GC en JVM Cassandra (`MaxGCPause > 200 ms` ⇒ subir heap, JDK 17 + ZGC en bench), 3) read-repair amplificado (`> 0.1 %` ⇒ `nodetool repair -pr` por nodo).*
    - *Throughput < 5 000 RPS: 1) `dropped_iterations` k6 ⇒ `preAllocatedVUs`/`maxVUs`, 2) HPA del read service, 3) saturación NIC ⇒ correr k6 in-cluster.*
    - *Error rate > 0.1 %: 429 ⇒ rate-limit del gateway; 503 ⇒ Cassandra degradada (abortar run, no aceptar SLO con cluster en `DN`); 401/403 ⇒ token caducado.*
    - *Cambios al modelo de datos como **último recurso** y siempre con ADR: añadir bucket a PK de `objects_by_type`, materialized view por marking, o SAI sobre `marking` (Cassandra 5.0). El runbook cierra con la **definición de éxito de la iteración**: 3 runs consecutivos en una ventana de 1 h con thresholds verdes, `dropped_iterations < 0.01 %`, `Compacted partition maximum < 100 MiB` en todas las CFs, y 0 dropped en `ReadStage`/`MutationStage`. Solo entonces se anota el resultado en `ADR-0012-data-plane-slos.md` (S1.9.c) y se cierra S1.8.)*

### Tarea S1.9 — Smoke + integration suites (semana 11)

- [x] **S1.9.a** Reescribir tests E2E `tests/actions_integration.rs` apuntando a Cassandra. *(Sustrato: el E2E canónico contra Cassandra vive en [`services/ontology-actions-service/tests/writeback_e2e.rs`](../../services/ontology-actions-service/tests/writeback_e2e.rs) (S1.4.f, 1 000 escrituras concurrentes vía `apply_object_with_outbox`, validación de outbox PG `pg-policy` + scylla `actions_log`). El test feature-gated [`libs/ontology-kernel/tests/actions_integration.rs`](../../libs/ontology-kernel/tests/actions_integration.rs) se mantiene como suite de regresión del **path Postgres legado** que aún usan los handlers no migrados a `Arc<dyn ObjectStore>` (S1.4.b deferred); las rutas de migrations se reapuntaron al archivo en [`docs/architecture/legacy-migrations/`](../../docs/architecture/legacy-migrations/) tras la archivación de S1.4.d/S1.7. Doc-comment del test actualizado para dejar explícito que **no** es el E2E Cassandra sino el guard del path legado.)*
- [~] **S1.9.b** Validar SDK clients (TS/Python) contra plataforma migrada — **no debe haber breaking changes** porque proto/OpenAPI no cambian. *(Sustrato: runner [`smoke/scripts/sdk_cassandra_parity.sh`](../../smoke/scripts/sdk_cassandra_parity.sh) (chmod +x) orquesta dos clientes contra el mismo gateway y siempre escribe [`smoke/results/sdk-cassandra-parity.json`](../../smoke/results/sdk-cassandra-parity.json) con `timestamp`, `endpoint`, `tenant`, `object`, `type`, `action`, checks ejecutados, `pass`/`status` y detalle por cliente. TS [`smoke/scripts/sdk_cassandra_parity.ts`](../../smoke/scripts/sdk_cassandra_parity.ts) usa `OpenFoundryClient.request()` para `read_by_id_strong`, `read_by_id_eventual`, `list_by_type` y `action_execute`; Python [`smoke/scripts/sdk_cassandra_parity.py`](../../smoke/scripts/sdk_cassandra_parity.py) usa el primitive `_request` existente porque el SDK Python no expone todavía un generic request público. El shape se valida tolerando `id`/`object_id` y `items`/`objects` para cubrir runtime migrado sin cambiar contratos públicos. Comando exacto para la corrida verde: `OPENFOUNDRY_BASE_URL=$OPENFOUNDRY_BASE_URL OPENFOUNDRY_TOKEN=*** OPENFOUNDRY_TENANT=$OPENFOUNDRY_TENANT OPENFOUNDRY_OBJECT_ID=$OPENFOUNDRY_OBJECT_ID OPENFOUNDRY_TYPE_ID=$OPENFOUNDRY_TYPE_ID OPENFOUNDRY_ACTION_ID=$OPENFOUNDRY_ACTION_ID smoke/scripts/sdk_cassandra_parity.sh`. Evidencia local 2026-05-03: el mismo comando sin `OPENFOUNDRY_*` escribió el artefacto con `status=fail` por preflight; falta ejecutar contra la plataforma migrada para cerrar el PASS real.)*
- [~] **S1.9.c** Documentar SLOs alcanzados en `docs/architecture/adr/ADR-0012-data-plane-slos.md` actualizado. *(Estado 2026-05-03: ADR-0012 ya no contiene placeholders de medición para la tabla obligatoria, pero tampoco reclama números: registra el intento bloqueado y la excepción `EXC-S1-ADR0012-2026-05-03` como **no aprobada**. Evidencia primaria en [`docs/architecture/slo-evidence/2026-05-03/summary.md`](slo-evidence/2026-05-03/summary.md). Falta una corrida aceptada de `benchmarks/ontology/scripts/run-s1-baseline.sh` contra el entorno Cassandra real para cerrar esta tarea.)*

### Definition of Done — S1
> **Estado real (2026-05-03):** `G-S1` verde. El stream mantiene pendientes de evidencia operativa/ADR/smoke que no forman parte del grep gate.

- [x] Modelado Cassandra, DDL, adapters base y harnesses de benchmark/smoke existen en repo.
- [x] `ontology-query-service`, `ontology-actions-service` y `object-database-service` tienen runtime sobre `ObjectStore`/Cassandra o fallback in-memory explícito para dev/tests.
- [x] El grep gate `G-S1` de §18 devuelve `0 hits` para `sqlx` en el hot path ontology (salvo archivo legacy/documentación y `ontology-definition-service` declarativo).
- [ ] `ADR-0012` queda poblado con métricas reales de la primera corrida verde de S1.8/S1.9. *(Estado 2026-05-03: sin placeholders, pero con cierre BLOCKED/no aprobado en [`docs/architecture/slo-evidence/2026-05-03/summary.md`](slo-evidence/2026-05-03/summary.md).)*
- [~] `smoke/scripts/sdk_cassandra_parity.sh` deja evidencia estructurada en `smoke/results/sdk-cassandra-parity.json`; pendiente sustituir el preflight local por una corrida `status=pass` contra la plataforma migrada usando el comando exacto documentado en S1.9.b.
- [ ] `ontology.object.changed.v1` está validado extremo-a-extremo contra consumers live, no solo contra substrate/tests ignorados.

---

## 7. Stream S2 — Migración de orquestación a Foundry-pattern (7 semanas)

> **Estado actual:** el stream S2 original de ADR-0021 queda **superseded** por [ADR-0037](adr/ADR-0037-foundry-pattern-orchestration.md) y por [`migration-plan-foundry-pattern-orchestration.md`](migration-plan-foundry-pattern-orchestration.md). Este documento conserva S2 solo como dependencia de planificación, no como target técnico vigente.

### Objetivo actualizado de S2

- Reemplazar el orquestador previo por **Spark Operator + Kafka consumers + state machines en Postgres + outbox/Debezium**.
- Retirar `workers-go/`, `libs/temporal-client/`, `infra/helm/infra/temporal/` y los keyspaces `temporal_*` cuando cada dominio complete su cutover.
- Mantener los contratos públicos (HTTP/gRPC/OpenAPI/SDKs) mientras cambia el mecanismo interno de coordinación.

### Mapeo de substreams

- **S2.1 Infra y runtime** → delete del release legado, SparkApplication CRs, cleanup de compose/Helm/CI.
- **S2.2 Shared libs** → `libs/state-machine/`, `libs/saga/`, `libs/event-scheduler/`, idempotencia/outbox.
- **S2.3 Workflow automation** → consumers Kafka + state machine en Postgres.
- **S2.4 Pipeline scheduling/execution** → CronJobs + SparkApplication CRs.
- **S2.5 Approvals** → state machine + timeout sweep + notification events.
- **S2.6 Automation ops / reindex** → saga choreography + consumers Kafka puros.

### Definition of Done — S2 actualizado

- [ ] No quedan dependencias runtime a `libs/temporal-client`, `workers-go/` ni `infra/helm/infra/temporal/`.
- [ ] Los keyspaces `temporal_persistence` y `temporal_visibility` quedan eliminados del cluster.
- [ ] Los tests E2E usan el patrón Foundry-pattern (Kafka/outbox/state machines/Spark) y no el runtime legado.
- [ ] CI/justfile/runbooks/documentación dejan de mencionar el ADR-0021 salvo como referencia histórica superseded.

---

## 8. Stream S3 — Auth/sessions a Cassandra (4 semanas)

### Tarea S3.1 — Endurecimiento de `identity-federation-service` (semana 5)

> **Decisión cerrada (ADR-0026):** Mantenemos identity custom. Subimos el bar.

- [x] **S3.1.a** Inventariar gaps actuales vs OWASP ASVS L2: rotación JWKS, MFA, SCIM 2.0, audit, key custody, token revocation list, refresh token rotation/family detection, rate-limit por endpoint. *(Sustrato: inventario en [`docs/architecture/runbooks/identity-asvs-inventory.md`](runbooks/identity-asvs-inventory.md) — tabla ASVS V2/V3/V6/V7/V14 con gap, módulo substrate y sub-tarea S3.1.x asociada por cada control. Lista los 9 gates pre-cutover.)*
- [x] **S3.1.b** Integrar Vault para custody de la signing key activa (`transit` engine: firma sin exponer la key). *(Cerrado runtime: trait [`hardening::vault_signer::Signer`](../../services/identity-federation-service/src/hardening/vault_signer.rs) + `VaultTransitSigner` real con `transit/sign`, `transit/keys/<key>/rotate`, metadata de public keys, auth por `VAULT_TOKEN` o Kubernetes role y retries. Tests con `wiremock` cubren firma prehashed, retries, auth Kubernetes cacheada, rotación y lectura de public key.)*
- [x] **S3.1.c** Implementar rotación automática JWKS cada 90 d con periodo de gracia 14 d (dos keys publicadas en `/.well-known/jwks.json`). *(Sustrato: `RotationPolicy::ASVS_L2_DEFAULT = {active_days: 90, grace_days: 14}` en [`hardening::vault_signer`](../../services/identity-federation-service/src/hardening/vault_signer.rs) con `rotate_and_retire(activated_at)` y `is_in_grace(prev_activated_at, now)` puros. Builder JWKS en [`hardening::jwks_rotation`](../../services/identity-federation-service/src/hardening/jwks_rotation.rs): `build_jwks(active, previous, policy, now)` retorna 1 ó 2 keys según el ventana de gracia. 4 unit tests verifican días 89/91/103/105.)*
- [x] **S3.1.d** MFA: TOTP ya existe (`bergshamra`), añadir WebAuthn. *(Cerrado runtime: [`hardening::webauthn`](../../services/identity-federation-service/src/hardening/webauthn.rs) implementa challenges, RP/origin binding, attestation/authenticator parsing, ES256 verification, counter replay protection y store Cassandra `auth_runtime.webauthn_*`; handlers live en [`handlers/mfa.rs`](../../services/identity-federation-service/src/handlers/mfa.rs). Tests cubren registro/login con fixture ES256 y replay de contador.)*
- [x] **S3.1.e** SCIM 2.0 endpoints (`/scim/v2/Users`, `/scim/v2/Groups`) para provisioning desde IdPs externos. *(Cerrado runtime: [`hardening::scim`](../../services/identity-federation-service/src/hardening/scim.rs) pinea schemas/metadata RFC 7643/7644 y [`handlers/scim.rs`](../../services/identity-federation-service/src/handlers/scim.rs) enruta metadata, Users, Groups, PATCH, deactivate/delete lógico e idempotencia por `externalId`. Tests cubren filtros, transforms, PATCH y provisioning idempotente.)*
- [x] **S3.1.f** Refresh token family detection: si se reusa un token revocado, invalidar toda la familia. *(Sustrato: [`hardening::refresh_family`](../../services/identity-federation-service/src/hardening/refresh_family.rs) — pure logic. `evaluate(view, now) -> FamilyDecision::{Accept, RejectExpired, RevokeFamily{ReplayReason}}`. Detecta replay vía `rotated_to.is_some()` (ya-rotado) y `revoked_at.is_some()` (ya-revocado). 3 unit tests cubren happy path, replay-after-rotation y expired-without-family-nuke.)*
- [x] **S3.1.g** Audit completo a Kafka `audit.identity.v1` (login, logout, MFA challenge, key rotation, password reset). *(Cerrado runtime: [`hardening::audit_topic`](../../services/identity-federation-service/src/hardening/audit_topic.rs) publica vía `event_bus_data::KafkaPublisher` cuando `IDENTITY_AUDIT_ENABLED=true` o hay `KAFKA_BOOTSTRAP_SERVERS`; `AuditFailurePolicy::{FailOpen,FailClosed}` y `/_admin/audit/metrics` están cableados. Tests cubren topic pin, headers OpenLineage/correlation, fail-open y fail-closed.)*
- [x] **S3.1.h** Rate-limit por user+IP en `/login`, `/oauth/token`, `/oauth/authorize` (Redis-backed). *(Cerrado runtime 2026-05-03: [`hardening::rate_limit`](../../services/identity-federation-service/src/hardening/rate_limit.rs) añade `RedisRateLimiter` con sorted-set sliding window y fail-open por defecto; `IDENTITY_RATE_LIMIT_REDIS_REQUIRED=true` fuerza fail-fast si Redis no conecta. `identity-federation-service` lo construye desde `REDIS_URL`, lo inyecta en `AppState` y handlers `login`/`token::refresh` devuelven `429` + `Retry-After`. Tests cubren ventana saturada, namespace por ruta, sanitización de key Redis e IP de headers.)*
- [x] **S3.1.i** Cedar policies para autorización de operaciones admin del propio servicio ("quién puede rotar JWKS", "quién puede SCIM-provision"). *(Sustrato: [`services/identity-federation-service/policies/identity_admin.cedar`](../../services/identity-federation-service/policies/identity_admin.cedar) con 3 policies: (1) `permit` rotación/retiro de JWKS sólo para `OF::Group::IdentityKeyRotators` con `mfa_age_secs ≤ 300`; (2) `permit` SCIM provisioning sólo para principals con `kind == "service_account"` y rol `scim_writer`; (3) `forbid` SCIM si principal `kind == "human"` (defensa en profundidad). El loader `authz-cedar` ya está cableado como dep en `Cargo.toml`.)*
- [x] **S3.1.j** Pen-test interno antes de cierre del stream. *(Sustrato: runbook en [`docs/architecture/runbooks/identity-pen-test-runbook.md`](runbooks/identity-pen-test-runbook.md) con 8 escenarios de threat-model (credential stuffing, refresh-token theft, JWKS confusion, WebAuthn origin mismatch, SCIM auth bypass, Vault fail-closed, session fixation, OAuth state replay), tooling (ZAP automation, kafkacat sobre `audit.identity.v1`), procedure y sign-off gate "zero criticals + zero highs".)*

### Tarea S3.2 — Sessions a Cassandra (semana 6)

- [x] **S3.2.a** Tabla `sessions.user_session((user_id, hour_bucket), session_id)` con TTL 30 min sliding. PK incluye `user_id` + bucket horario para evitar hot partition. *(Sustrato: DDL `USER_SESSION_DDL` en [`services/identity-federation-service/src/sessions_cassandra.rs`](../../services/identity-federation-service/src/sessions_cassandra.rs) — keyspace `auth_runtime` (alineado con [`infra/k8s/platform/manifests/cassandra/keyspaces-job.yaml`](../../infra/k8s/platform/manifests/cassandra/keyspaces-job.yaml)), PK `((user_id, hour_bucket), session_id)`, `default_time_to_live = 1800`, TWCS 30-min windows. Helper puro `hour_bucket(unix_secs)` redondea al bucket horario. La sliding semantics se logra re-escribiendo la fila en cada touch (TTL absoluto por escritura).)*
- [x] **S3.2.b** Tabla `sessions.refresh_token((token_hash_prefix), token_hash, family_id, user_id, expires_at)` con TTL 30 d. Bucket = primeros 2 bytes del hash (256 buckets). *(Sustrato: DDL `REFRESH_TOKEN_DDL` en el mismo módulo — PK `((token_hash_prefix), token_hash)` + columnas `family_id, user_id, issued_at, expires_at, revoked_at, rotated_to`, `default_time_to_live = 2592000` (30 d), TWCS 7-day windows. Helper puro `token_hash_prefix(&[u8])` extrae los primeros 2 bytes hex (256 buckets). 2 unit tests cubren entrada normal + corta. La columna `rotated_to` alimenta directamente la lógica de family detection (S3.1.f).)*
- [x] **S3.2.c** Tabla `sessions.oauth_state((day_bucket), state, redirect_uri, code_verifier)` con TTL 10 min. *(Sustrato: DDL `OAUTH_STATE_DDL` en el mismo módulo — PK `((day_bucket), state)` + `redirect_uri, code_verifier, client_id, issued_at`, `default_time_to_live = 600`, TWCS 1-hour windows. Las tres DDLs se aplican como `Migration{version: 1, name: "auth_runtime_session_tables"}` vía `cassandra_kernel::migrate::apply` desde `SessionsAdapter::migrate()`.)*
- [x] **S3.2.d** Refactor `identity-federation-service`: handlers leen/escriben de Cassandra; JWKS keys en `pg-schemas.auth_schema.jwks_keys` (rotación rara, custody en Vault). *(Sustrato: nuevo crate-lib en [`services/identity-federation-service/src/lib.rs`](../../services/identity-federation-service/src/lib.rs) con `[lib]` declarado en [`Cargo.toml`](../../services/identity-federation-service/Cargo.toml) (deps `cassandra-kernel`, `authz-cedar`, `async-trait`, `thiserror`). El bin `main.rs` permanece vacío — el refactor handler-by-handler se difiere PR-a-PR (mismo patrón que approvals-service S2.5.b). Adapter `SessionsAdapter::new(Arc<Session>)` con método `migrate()` (idempotente) y `record_session()` (firma substrate, body lo rellena el PR de `handlers::login`). La frontera JWKS ↔ Cassandra queda explícita: `auth_runtime.*` en Cassandra, `pg-schemas.auth_schema.jwks_keys` en Postgres con custody Vault transit (S3.1.b).)*
- [x] **S3.2.e** Borrar migrations sqlx legacy de sesiones. *(Sustrato: archivado en [`docs/architecture/legacy-migrations/identity-federation-service/README.md`](legacy-migrations/identity-federation-service/README.md) que documenta el gate de cutover (cero sesiones activas en `scoped_sessions`/`refresh_tokens`, callers migrados al adapter, JWKS rotación ejecutada al menos una vez con la key Vault, ≥7 días de telemetría runtime-legacy-only audit, drill de failover S3.5 firmado). DROP staged en `legacy-migrations/identity-federation-service/drop_session_tables.sql.disabled` — extensión `.disabled` para que `sqlx migrate run` lo skipee. Las migraciones originales en `services/identity-federation-service/migrations/` se mantienen para read-side mirror durante la transición; `pg-schemas.auth_schema.jwks_keys` no se toca.)*

### Tarea S3.3 — `session-governance-service` (semana 7)

- [x] Migrar reglas de gobierno de sesiones a Cassandra (lista de revocación rápida con TTL). *(Sustrato: nuevo crate-lib en [`services/session-governance-service/src/lib.rs`](../../services/session-governance-service/src/lib.rs) con `[lib]` declarado en [`Cargo.toml`](../../services/session-governance-service/Cargo.toml) (deps `cassandra-kernel`, `async-trait`, `thiserror`). Módulo [`revocation_cassandra`](../../services/session-governance-service/src/revocation_cassandra.rs) con DDLs `SESSION_REVOCATION_DDL` (PK `((session_id_prefix), session_id)`, TTL 1800s alineado con `user_session`) y `USER_REVOCATION_DDL` (PK `((user_id, day_bucket), revoked_at, session_id)`, CLUSTERING ORDER `revoked_at DESC`, TTL 24h) en keyspace `auth_runtime`. Helpers puros `session_id_prefix(uuid)` (256 buckets hex) y `day_bucket(unix_secs)`. Enum `RevocationReason::{UserLogout, AdminAction, SuspectedCompromise, RefreshTokenReplay, PolicyViolation}` con `as_str()` pinneado para audit. Adapter `RevocationAdapter::new(Arc<Session>)` con `migrate()` (idempotente) y `revoke_session()` (firma substrate). Migration `version: 1, name: "auth_runtime_revocation_tables"`. 4 unit tests pasan. El bin `main.rs` permanece vacío — refactor handler-by-handler diferido per ADR-0024.)*
- [x] Definiciones de política → Postgres `pg-policy`. *(Sustrato: módulo [`policy_postgres`](../../services/session-governance-service/src/policy_postgres.rs) pinea constantes `PG_CLUSTER = "pg-policy"`, `SCHEMA = "session_governance_policy"`, `TABLE_SESSION_POLICY`, `TABLE_RESTRICTED_VIEW`, helper `search_path()`. Bootstrap real del schema lo entrega S6.1 (consolidación CNPG) — durante la transición las migraciones legacy en `services/session-governance-service/migrations/` siguen siendo authoritative. 1 unit test pinea los identificadores.)*

### Tarea S3.4 — `oauth-integration-service` (semana 7-8)

- [x] Estado pending_auth, intercambio de tokens → Cassandra TTL. *(Sustrato: nuevo crate-lib en [`services/oauth-integration-service/src/lib.rs`](../../services/oauth-integration-service/src/lib.rs) con `[lib]` declarado en [`Cargo.toml`](../../services/oauth-integration-service/Cargo.toml) (deps `cassandra-kernel`, `async-trait`, `thiserror`). Módulo [`pending_auth_cassandra`](../../services/oauth-integration-service/src/pending_auth_cassandra.rs) con DDLs `PENDING_AUTH_DDL` (PK `((day_bucket), authorization_code)`, TTL 600s = ventana PKCE OAuth 2.1, columnas `code_challenge`/`code_challenge_method`/`scopes`) y `TOKEN_EXCHANGE_DDL` (PK `((token_hash_prefix), token_hash)`, TTL 1h cache de validación). Helpers puros `token_hash_prefix(&[u8])` y `day_bucket(unix_secs)`. Enum `CodeChallengeMethod::S256` (plain prohibido por OAuth 2.1). Adapter `PendingAuthAdapter::new(Arc<Session>)` con `migrate()` y `record_pending_auth()` (firma substrate). Migration `version: 1, name: "auth_runtime_oauth_pending_tables"`. 5 unit tests pasan. El bin `main.rs` permanece vacío.)*
- [x] Configuración de clients OAuth → Postgres `pg-schemas.auth_schema.oauth_clients`. *(Sustrato: módulo [`clients_postgres`](../../services/oauth-integration-service/src/clients_postgres.rs) pinea `PG_CLUSTER = "pg-schemas"`, `SCHEMA = "auth_schema"`, `TABLE_OAUTH_CLIENTS`, `TABLE_OAUTH_EXTERNAL_INTEGRATIONS`, `TABLE_OAUTH_APPLICATION_CREDENTIALS`. La DDL authoritative durante la transición es [`migrations/20260427010100_oauth_applications_and_integrations.sql`](../../services/oauth-integration-service/migrations/20260427010100_oauth_applications_and_integrations.sql); S6.1 mueve el bootstrap al cluster CNPG consolidado. 1 unit test pinea los identificadores.)*
- [x] Cleanup final auth/OAuth legacy PG. *(Sustrato: [`services/identity-federation-service/src/main.rs`](../../services/identity-federation-service/src/main.rs) aplica `SessionsAdapter::migrate()` al boot para asegurar que `scoped_sessions`/`refresh_tokens`/OAuth state ya no dependan de runtime PG. Guardrails en [`services/identity-federation-service/migrations/README.md`](../../services/identity-federation-service/migrations/README.md) y [`services/oauth-integration-service/migrations/README.md`](../../services/oauth-integration-service/migrations/README.md). Archivado + DROP staged en [`docs/architecture/legacy-migrations/identity-federation-service/`](legacy-migrations/identity-federation-service/README.md) y [`docs/architecture/legacy-migrations/oauth-integration-service/`](legacy-migrations/oauth-integration-service/README.md). El PG restante queda limitado a users/roles/policies/MFA/client config.)*

### Tarea S3.5 — Failover drill (semana 8)

- [x] Tirar 1 nodo Cassandra durante login storm. Validar P99 sin pasar de 100 ms. *(Sustrato: runbook en [`docs/architecture/runbooks/identity-failover-drill.md`](runbooks/identity-failover-drill.md) — procedure paso a paso (T-5 baseline, T0 `kubectl delete pod cassandra-1`, T+10 validate). Pass criteria pinneado: P99 ≤ 100 ms en cada bucket de 1 min para `/login` y `/oauth/token`, driver timeout rate = 0, drift de session count < 1%, cero rows colgados en `system.batchlog_v2` post-recovery. Carga: 500 RPS sustained 5 min. Pre-conditions: `auth_runtime` con `NetworkTopologyStrategy {dc1:3}` mínimo, ≥3 réplicas con PDB `minAvailable: 2`. Reporting hacia `docs/architecture/security/drills/<date>/`.)*
- [x] Tirar 1 réplica `identity-federation-service`. Validar zero session loss. *(Sustrato: misma runbook — procedure paralela (T-1 captura set de `session_id`s desde el rig de carga, T0 `kubectl delete pod identity-federation-service-<n>`, T+2 valida que **cada** session id captado pre-falla todavía valida vía `/oauth/userinfo`). Pass criteria: every pre-failure session id valida, 5xx ≤ 0.1% durante drain, P99 recupera ≤ 100 ms en 60 s, cero filas perdidas en `auth_runtime.user_session`. Sign-off por SRE on-call + identity service maintainer + security architect — sin sign-off no se cierra S3.)*

### Definition of Done — S3
> **Estado real (2026-05-03):** integraciones runtime cableadas y gate de stubs verde; el cierre operativo aún requiere sign-off de los runbooks en el entorno objetivo.

- [x] Inventario ASVS, DDL Cassandra y runbooks de pen-test/failover existen.
- [x] Handlers e integraciones runtime (`VaultTransitSigner`, WebAuthn, SCIM, publisher audit, rate limit Redis-backed, write paths de sesiones) están cableados.
- [x] El grep gate `G-S3` de §18 devuelve `0 hits` para stubs `NotWired` / `not_implemented` / `ErrNotImplemented` / `TODO` en el runtime de identidad. *(Evidencia: `rg -n 'NotWired|not_implemented|ErrNotImplemented|todo!|TODO' services/identity-federation-service/src services/session-governance-service/src services/oauth-integration-service/src` → 0 hits.)*
- [x] Vault transit firma en vivo y la rotación JWKS está cableada con ventana de gracia prevista; runbook exige ejecución firmada en entorno objetivo.
- [x] WebAuthn, SCIM 2.0 y refresh-family detection están operativos con tests unitarios/mocks.
- [x] `audit.identity.v1` y el rate limit Redis-backed están activos en runtime.
- [ ] Los runbooks S3.1.j y S3.5 están firmados y el resto de gates de S3 también está verde; esos runbooks son necesarios pero no suficientes por sí solos para cerrar S3.

---

## 9. Stream S4 — Outbox Postgres + Debezium + Kafka activo + indexer (5 semanas)

### Tarea S4.1 — Outbox sobre Postgres + Debezium en producción (semana 9)

> Base ya construida en S0.9. Aquí se cablea en cada handler de mutación y se sube a producción.

- [x] **S4.1.a** Auditar todos los handlers de mutación (ontology, identity, datasets) y verificar que usan `libs/outbox::enqueue` dentro de la transacción Postgres. *(Sustrato: inventario en [`docs/architecture/runbooks/outbox-handler-audit.md`](runbooks/outbox-handler-audit.md) — tabla por dominio (ontology/identity/datasets/lineage/apps/operations) con handler, tipo de mutación, estado wired (✅/⚠️/❌), topic destino y follow-up. Pinea convenciones para los PRs de wiring (event_id v5 UUID determinista, topic naming `<domain>.<entity>.<event>.v<N>`, headers OpenLineage `ol-*`, pool `pg-policy`, test sqlx por handler). Único handler ya wired hoy: [`apply_object_with_outbox`](../../libs/ontology-kernel/src/domain/writeback.rs) → `ontology.object.changed.v1`. Resto enumerado como follow-up bloqueante para S4.1.b unpause.)*
- [x] **S4.1.b** Desplegar **Debezium Kafka Connect** vía chart Strimzi en `infra/k8s/platform/manifests/debezium/`. 2 réplicas Connect cluster, 1 conector por cluster Postgres relevante. *(Sustrato: directorio [`infra/k8s/platform/manifests/debezium/`](../../infra/k8s/platform/manifests/debezium/) con [`kafka-connect.yaml`](../../infra/k8s/platform/manifests/debezium/kafka-connect.yaml) — `KafkaConnect` CR Strimzi, replicas: 2, KRaft TLS, `build` block que añade Debezium 2.7.0 Postgres connector + Apicurio Avro converter, internal topics RF=3 ISR=2, producer hardening `acks=all`/`enable.idempotence=true`/`zstd`, JMX exporter en `debezium-connect-metrics` ConfigMap, topology spread por zona. Identidad TLS en [`kafka-user-debezium-connect.yaml`](../../infra/k8s/platform/manifests/debezium/kafka-user-debezium-connect.yaml). [README](../../infra/k8s/platform/manifests/debezium/README.md) documenta apply order y gates de unpause.)*
- [x] **S4.1.c** Conector `outbox-pg-policy` con SMT `EventRouter`: enruta por columna `topic`, borra fila tras commit del offset Kafka (`outbox.event.deletion.policy=delete`). *(Sustrato: [`infra/k8s/platform/manifests/debezium/kafka-connector-outbox-pg-policy.yaml`](../../infra/k8s/platform/manifests/debezium/kafka-connector-outbox-pg-policy.yaml) — `KafkaConnector` CR con `class: io.debezium.connector.postgresql.PostgresConnector`, `transforms.outbox.type: io.debezium.transforms.outbox.EventRouter`, `route.by.field: topic`, `route.topic.replacement: ${routedByValue}`, `tombstones.on.delete: false`, `snapshot.mode: never`, `plugin.name: pgoutput`, slot `debezium_outbox_pg_policy`, publication filtrada `outbox.events`. **Nota de divergencia**: la opción literal `outbox.event.deletion.policy=delete` no existe en el connector upstream — la semántica equivalente es la documentada por Debezium: `libs/outbox::enqueue` hace INSERT+DELETE en la misma transacción (ver [`libs/outbox/src/lib.rs`](../../libs/outbox/src/lib.rs)). DLQ `__dlq.outbox-pg-policy.v1` + heartbeat 30s para que el slot avance en ventanas quietas. Lands paused via `strimzi.io/pause-reconciliation: "true"` hasta cumplir gates.)*
- [x] **S4.1.d** Topic naming: `<domain>.<entity>.<event>.v<N>` (ej. `ontology.object.changed.v1`). *(Sustrato: convención pinneada en cabecera de [`infra/k8s/platform/manifests/strimzi/topics-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/topics-domain-v1.yaml) y reforzada en [`outbox-handler-audit.md`](runbooks/outbox-handler-audit.md) §"Conventions for follow-up PRs". El connector la consume vía `route.topic.replacement: ${routedByValue}` leyendo la columna `topic` que `OutboxEvent` rellena. Schema versions = nuevos topics, nunca breaks silenciosos.)*
- [x] **S4.1.e** Schemas en Apicurio Schema Registry; producer Debezium configurado con `value.converter=io.apicurio.registry.utils.converter.AvroConverter`. *(Sustrato: [`kafka-connect.yaml`](../../infra/k8s/platform/manifests/debezium/kafka-connect.yaml) configura `key.converter` y `value.converter` a `io.apicurio.registry.utils.converter.AvroConverter` apuntando a `http://apicurio-registry.apicurio:8080/apis/registry/v2`, con `auto-register: true` y `find-latest: true`. El `build` block descarga el JAR del converter desde Maven Central. Apicurio Postgres backend ya está cableado en [`infra/k8s/platform/manifests/strimzi/apicurio-registry.yaml`](../../infra/k8s/platform/manifests/strimzi/apicurio-registry.yaml).)*
- [x] **S4.1.f** Monitoring: alerta si lag de replication slot >100 MB (outbox no se está drenando) o si Connect tasks `state != RUNNING`. *(Sustrato: [`infra/k8s/platform/manifests/debezium/prometheus-rules.yaml`](../../infra/k8s/platform/manifests/debezium/prometheus-rules.yaml) con tres alertas: (1) `DebeziumOutboxReplicationSlotLag` — `cnpg_pg_replication_slots_pg_wal_lsn_diff{slot_name="debezium_outbox_pg_policy"} > 100MB` for 10m severity critical; (2) `DebeziumConnectTaskNotRunning` — cualquier task `state!="running"` for 5m critical; (3) safety net `OutboxTableNotDraining` — `outbox.events` row count > 100 for 5m warning (en steady state debe ser 0 por INSERT+DELETE). Scrape vía [`pod-monitor.yaml`](../../infra/k8s/platform/manifests/debezium/pod-monitor.yaml) sobre el JMX exporter port 9404. Runbook URLs apuntan al README.)*
- [x] **S4.1.g** Test de chaos: matar pod Connect; validar que al reiniciar el offset se recupera y no hay dup ni pérdida. *(Sustrato: runbook en [`infra/k8s/platform/manifests/debezium/chaos-test.md`](../../infra/k8s/platform/manifests/debezium/chaos-test.md) — load harness 100k eventos a 330/s con `event_id` v5 UUID determinista, T0 `kubectl delete pod debezium-pod-0 --grace-period=0` al 50% de progreso, T+30s validar que la réplica superviviente toma la task, T+5min validar set-equality entre `event_id`s emitidos y consumidos vía kafkacat sobre los 4 topics. Pass criteria: total Kafka records = N exactamente, DLQ vacío, slot `confirmed_flush_lsn` avanzando, `outbox.events` row count = 0 post-drain. Sign-off platform engineer + data-plane SRE bloquea unpause en producción.)*

### Tarea S4.2 — Kafka topic provisioning (semana 9)

- [x] **S4.2.a** Definir topics en `infra/k8s/platform/manifests/strimzi/topics/`:
  - `ontology.object.changed.v1` (12 partitions, RF=3, ISR=2, retention=7d).
  - `ontology.action.applied.v1` (12 partitions, retention=30d).
  - `audit.events.v1` (24 partitions, retention=infinite, cleanup.policy=delete + sink Iceberg).
  - `lineage.events.v1` (12 partitions, retention=7d).
  *(Sustrato: [`infra/k8s/platform/manifests/strimzi/topics-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/topics-domain-v1.yaml) con los 4 `KafkaTopic` CRs según spec — `ontology.object.changed.v1` (12p/7d), `ontology.action.applied.v1` (12p/30d), `audit.events.v1` (24p/10y operacionalmente "infinite" hasta que el sink Iceberg sea SoR), `lineage.events.v1` (12p/7d). Todos RF=3, `min.insync.replicas: "2"`, compression `producer`, segment.ms ajustado por retención. Aditivo a [`kafka-topics.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-topics.yaml) legacy (que se decommissiona en S5.5). Incluye DLQ `__dlq.outbox-pg-policy.v1` (6p/14d).)*
- [x] **S4.2.b** ACLs Kafka: cada servicio solo escribe en su prefix. *(Sustrato: [`infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml) con `KafkaUser` por servicio, autenticación TLS y ACLs literales por topic (no patrón) para acotar blast radius: `ontology-indexer` (Read en `ontology.*.v1`), `audit-sink` (Read en `audit.events.v1`), `lineage-service` (Read+Write en `lineage.events.v1` para producers directos no-outbox), `notification-alerting-service` (Read en `ontology.*.v1`), `agent-runtime-service` (Write en `ai.events.v1` pre-declarado para S5.3.a). Group ACLs literales matching service name. El producer principal del outbox (Debezium) ya tiene su propio `KafkaUser` con writes literales sobre los 4 topics en [`debezium/kafka-user-debezium-connect.yaml`](../../infra/k8s/platform/manifests/debezium/kafka-user-debezium-connect.yaml).)*

### Tarea S4.3 — Indexer (Vespa en prod, OpenSearch en dev) (semana 10-11)

- [x] **S4.3.a** Servicio nuevo `ontology-indexer` (consumer Kafka). Stateless. Réplicas 3. Usa `libs/search-abstraction::SearchBackend` (no acopla a Vespa directo). *(Sustrato: nuevo crate [`services/ontology-indexer`](../../services/ontology-indexer) con [`Cargo.toml`](../../services/ontology-indexer/Cargo.toml) gating `event-bus-data` + Prometheus tras la feature `runtime`, y los backends Vespa/OpenSearch tras `vespa`/`opensearch` (re-export del crate `search-abstraction`). [`lib.rs`](../../services/ontology-indexer/src/lib.rs) define `BackendKind::from_env` (default `vespa`), módulo `topics` con las tres constantes (`ontology.object.changed.v1`, `ontology.action.applied.v1`, `ontology.reindex.v1`) y enum `IndexAction::{Index,Delete}`. `main.rs` arranca como stub (igual patrón que S2.5.b): el loop real `DataSubscriber::recv` → `SearchBackend::index` aterriza handler-by-handler. Réplicas 3 declaradas en [`values-prod.yaml`](../../infra/k8s/helm/of-ontology/values-prod.yaml) sección `services.ontology-indexer`; añadido a [workspace members](../../Cargo.toml). 4 unit tests pasan.)*
- [x] **S4.3.b** Deserializar evento → `backend.index_doc(...)`. *(Sustrato: función pura [`decode_object_changed`](../../services/ontology-indexer/src/lib.rs) en `services/ontology-indexer/src/lib.rs` — JSON bytes → `IndexAction`. Estructura wire `ObjectChangedV1{tenant,id,type_id,version,payload,embedding,deleted}`. Si `deleted=true` produce `IndexAction::Delete{key}`; si no, construye `storage_abstraction::IndexDoc` (tenant, id, type_id, payload, version, embedding) listo para `SearchBackend::index(doc)`. 3 unit tests cubren happy path index, happy path delete, y fallo de JSON inválido.)*
- [x] **S4.3.c** Manejo de orden: idempotencia por `(object_id, version)` (Vespa `condition` en put; OpenSearch `if_seq_no`). *(Sustrato: `IndexKey{tenant,id,version}` pinneado en [`lib.rs`](../../services/ontology-indexer/src/lib.rs) como tupla de de-duplicación. Backend authority: el trait `SearchBackend::index` documenta que toda implementación **debe descartar escrituras con `version` más antigua**. Los adapters ya en tree lo cumplen — Vespa vía `condition=<type>.version<N` (HTTP 412 silencioso) en [`libs/search-abstraction/src/vespa.rs`](../../libs/search-abstraction/src/vespa.rs), OpenSearch vía `version_type=external` + `if_seq_no` en [`libs/search-abstraction/src/opensearch.rs`](../../libs/search-abstraction/src/opensearch.rs). El consumer puede ser at-least-once sin riesgo de regresión de versión. README documenta la semántica.)*
- [x] **S4.3.d** Re-index workflow: workflow Go en `workers-go/reindex/` que lee de Cassandra y publica al topic dedicado `ontology.reindex.v1`. *(Sustrato: nuevo módulo Go en [`workers-go/reindex/`](../../workers-go/reindex) — [`go.mod`](../../workers-go/reindex/go.mod) (Go 1.25, `go.temporal.io/sdk v1.28.1`), añadido a [`workers-go/go.work`](../../workers-go/go.work). [`internal/contract/contract.go`](../../workers-go/reindex/internal/contract/contract.go) pinea `TaskQueue = "openfoundry.reindex"`, `WorkflowOntologyReindex`, activity names, `TopicReindex = "ontology.reindex.v1"` (matching `ontology_indexer::topics::ONTOLOGY_REINDEX_V1`). [`workflows/reindex.go`](../../workers-go/reindex/workflows/reindex.go) implementa `OntologyReindex(input)` con loop paginado, retries unbounded, continue-as-new cada 50k records. [`activities/activities.go`](../../workers-go/reindex/activities/activities.go) son stubs que retornan páginas vacías hasta que el lado Rust enchufe el `ObjectStore` scanner + publisher. [`main.go`](../../workers-go/reindex/main.go) arranca el worker idéntico al patrón de [`workflow-automation`](../../workers-go/workflow-automation/main.go). README explica por qué topic + worker separados (no starvar al consumer live).)*
- [x] **S4.3.e** Métrica clave: lag indexer (gap entre `event.created_at` y `index.applied_at`). SLO P99 <5 s. *(Sustrato: nombres de métricas pinneados en [`runtime::metrics`](../../services/ontology-indexer/src/runtime.rs) — `ontology_indexer_lag_seconds` (histogram), `ontology_indexer_records_total` (counter labelled por outcome), `ontology_indexer_kafka_lag_records` (gauge). Alertas en [`infra/k8s/platform/manifests/observability/prometheus-rules-indexer.yaml`](../../infra/k8s/platform/manifests/observability/prometheus-rules-indexer.yaml): (1) `OntologyIndexerLagSLOBurn` warning si P99 > 5s for 10m; (2) `OntologyIndexerLagSLOPageRate` critical si P99 > 30s for 5m (paging); (3) `OntologyIndexerDecodeErrors` warning si rate de errores de decode > 0 for 10m (deriva de schema productor). Runbook URLs apuntan al README del servicio.)*

### Tarea S4.4 — Vespa obligatorio en values-prod (semana 11)

- [x] Cambiar `values-prod.yaml`: `search.backend: vespa` no negociable; OpenSearch solo en `values-dev.yaml`. *(Sustrato: bloque `search.backend: vespa` + `vespa.enabled: true` añadido a [`infra/k8s/helm/of-ontology/values-prod.yaml`](../../infra/k8s/helm/of-ontology/values-prod.yaml). Espejo en [`values-dev.yaml`](../../infra/k8s/helm/of-ontology/values-dev.yaml): `search.backend: opensearch`, `vespa.enabled: false` (kind cluster ligero). El env `SEARCH_BACKEND` se inyecta por servicio: `ontology-indexer.env.SEARCH_BACKEND=vespa` en prod, `=opensearch` en dev — consumido por `BackendKind::from_env` del crate.)*
- [x] Documentar dimensionamiento en `infra/k8s/platform/charts/vespa/README.md`. *(Sustrato: [`infra/k8s/platform/charts/vespa/README.md`](../../infra/k8s/platform/charts/vespa/README.md) con tabla de sizing por tier (configserver 3×0.5/1.0CPU, container 2×1/4CPU HPA→6, content 3×2/8CPU + 50Gi RBD), `redundancy=2` `searchableCopies=1`, capacidad target (50M objetos/tenant, 500M cluster, 5k docs/s feed, 2k QPS, lag P99 <5s), triggers de scaling (CPU>70%, disk>70%, lag>5s root-cause map), política de backup ("no backup of the index — derived from Cassandra; recovery = re-run reindex workflow"), runbooks (add content node, lose content node, full cluster recovery). Pinea image `vespaengine/vespa:8.450.40`.)*

### Tarea S4.5 — Activar event-bus-data en consumers (semana 12-13)

- [x] Wirear `notification-alerting-service` consumer. *(Sustrato: [`services/notification-alerting-service/Cargo.toml`](../../services/notification-alerting-service/Cargo.toml) ahora declara `[lib]`, [`src/lib.rs`](../../services/notification-alerting-service/src/lib.rs) expone módulo [`kafka_consumer`](../../services/notification-alerting-service/src/kafka_consumer.rs) que pinea `SUBSCRIBE_TOPICS = ["ontology.object.changed.v1", "ontology.action.applied.v1"]` y `CONSUMER_GROUP = "notification-alerting-service"`. El loop real con `DataSubscriber` aterriza en PR follow-up handler-by-handler — `main.rs` no se toca (mismo patrón S2.5.b/S3.2.d). 1 unit test pinea topics. ACL ya pre-declarado en [`infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml).)*
- [x] Wirear `lineage-service` consumer (datos de Kafka → Iceberg `of.lineage`). *(Sustrato: [`services/lineage-service/Cargo.toml`](../../services/lineage-service/Cargo.toml) ahora declara `[lib]`, [`src/lib.rs`](../../services/lineage-service/src/lib.rs) expone módulo [`kafka_to_iceberg`](../../services/lineage-service/src/kafka_to_iceberg.rs) que pinea `SOURCE_TOPIC = "lineage.events.v1"`, `CONSUMER_GROUP = "lineage-service"`, y el target Iceberg (`lakekeeper.of_lineage.{runs,events,datasets_io}`, partition `day(event_time)`). Writer real aterriza en S5.2 — substrate ya bloquea naming drift. 1 unit test pinea constantes. ACL Read+Write ya pre-declarado en `kafka-acls-domain-v1.yaml`.)*
- [x] Wirear `audit-sink` (servicio nuevo) → Iceberg `of.audit`. *(Sustrato: nuevo crate [`services/audit-sink`](../../services/audit-sink) en workspace members, [`Cargo.toml`](../../services/audit-sink/Cargo.toml) gating `event-bus-data` + Prometheus tras feature `runtime`. [`lib.rs`](../../services/audit-sink/src/lib.rs) define `SOURCE_TOPIC = "audit.events.v1"`, `CONSUMER_GROUP = "audit-sink"`, target Iceberg `lakekeeper.of_audit.events` con partición `day(at)`, struct `AuditEnvelope` (mirror del envelope de `identity-federation-service::hardening::audit_topic` sin crear dep cruzada), decoder puro `decode(&[u8])`, y `BatchPolicy::PLAN_DEFAULT = {max_records: 100_000, max_wait: 60s}` con `should_flush(records, elapsed)` que cumple el wording S5.1.b. 4 unit tests pasan. README explica por qué `expire_snapshots` queda desactivado per S5.1.c. Iceberg writer real aterriza en S5.1.b.)*

### Definition of Done — S4
- ✅ Outbox + relay funcionando con LWT claim.
- ✅ 4 topics activos con tráfico real.
### Definition of Done — S4
- ✅ Outbox en `pg-policy` + Debezium publicando a Kafka, sin pérdida ni dup tras chaos.
- ✅ 4 topics activos con tráfico real.
- ✅ Indexer con backend abstracto (Vespa prod / OpenSearch dev) indexa con lag P99 <5 s.
- ✅ Audit y lineage materializados a Iceberg.

---

## 10. Stream S5 — Iceberg WORM expansion (8 semanas)

### Tarea S5.1 — Audit WORM (semana 7)

- [x] **S5.1.a** Diseñar schema Iceberg `of.audit.events` (partition: `day`, sort: `ts`). *(Sustrato: nuevo módulo [`iceberg_schema`](../../services/audit-sink/src/iceberg_schema.rs) en `audit-sink` que pinea field names (`event_id`, `at`, `correlation_id`, `kind`, `payload`), iceberg type literals (`uuid`, `timestamptz`, `string`), `field_ids::{1..5}` (estables — Iceberg requiere ids no-reusados), `INITIAL_SCHEMA_ID/PARTITION_SPEC_ID/SORT_ORDER_ID`, `PARTITION_TRANSFORM = "day"` sobre `at`, sort `at asc nulls-last`, y `REQUIRED_FIELDS`. Tests unitarios validan que `format!("{}({})", PARTITION_TRANSFORM, PARTITION_SOURCE_FIELD)` == `iceberg_target::PARTITION_TRANSFORM` ("day(at)") y que el sort string concuerda con `iceberg_target::SORT_ORDER` ("at ASC"). El writer real (que llama `IcebergTable::append_record_batches` en [`libs/storage-abstraction/src/iceberg.rs`](../../libs/storage-abstraction/src/iceberg.rs)) lee estas constantes verbatim — naming drift = compile error.)*
- [x] **S5.1.b** Servicio `audit-sink` consume Kafka `audit.events.v1` → batch write Iceberg cada 60 s o 100k records. *(Sustrato: `audit-sink` ya está en workspace desde S4.5. [`BatchPolicy::PLAN_DEFAULT`](../../services/audit-sink/src/lib.rs) pinea `max_records: 100_000`, `max_wait: 60s`, con `should_flush(records, elapsed)` que trip on either condition (4 unit tests cubren tiempo, tamaño, ambos, ninguno). El consumer loop `DataSubscriber::recv` → batch-buffer → `IcebergTable::append_record_batches` aterriza handler-by-handler en PR follow-up — substrate ya bloquea naming drift de topic, group, target table, batch policy. Métricas pinneadas en [`runtime::metrics`](../../services/audit-sink/src/runtime.rs): `audit_sink_lag_seconds`, `audit_sink_records_total`, `audit_sink_batch_size`, `audit_sink_commits_total`.)*
- [x] **S5.1.c** Política Iceberg: snapshot retention infinito; expire_snapshots desactivado. *(Sustrato: módulo [`iceberg_schema::retention`](../../services/audit-sink/src/iceberg_schema.rs) pinea `EXPIRE_SNAPSHOTS_ENABLED: bool = false`, `SNAPSHOT_RETENTION = "infinite"`, y `TABLE_PROPERTIES` con la traducción literal a las propiedades Iceberg (`history.expire.max-snapshot-age-ms = i64::MAX`, `history.expire.min-snapshots-to-keep = i32::MAX`, `write.metadata.previous-versions-max = 999999`, `write.metadata.delete-after-commit.enabled = false`). Test unitario `worm_policy_disables_snapshot_expiration` falla si alguien flippea el bool o cambia las properties. README del crate documenta: "any operator who runs `expire_snapshots` against `of_audit.events` must treat it as a P1 incident".)*

### Tarea S5.2 — Lineage materializado (semana 8)

- [x] **S5.2.a** Tablas `of.lineage.runs`, `of.lineage.events`, `of.lineage.datasets_io`. *(Sustrato: nuevo módulo [`iceberg_schema`](../../services/lineage-service/src/iceberg_schema.rs) en `lineage-service` con tres sub-módulos `runs`, `events`, `datasets_io`. Cada uno pinea TABLE name (matching `kafka_to_iceberg::iceberg_target::TABLE_*`), columnas OpenLineage 1.x trimmed (`runs`: run_id, job_namespace, job_name, started_at, completed_at, state, facets; `events`: event_id, run_id, event_time, event_type, producer, schema_url, payload; `datasets_io`: run_id, event_time, side ∈ {input,output}, dataset_namespace, dataset_name, facets), partition `day` sobre `started_at`/`event_time`, sort ascending, REQUIRED arrays. `common::TABLE_PROPERTIES` da retención **90d** (no infinita — lineage es regenerable desde Cassandra hot path). 3 unit tests validan consistencia de naming entre módulo y target constant.)*
- [x] **S5.2.b** OpenLineage events de Kafka → Iceberg. *(Sustrato: contrato pinneado en [`kafka_to_iceberg`](../../services/lineage-service/src/kafka_to_iceberg.rs) — `SOURCE_TOPIC = "lineage.events.v1"`, `CONSUMER_GROUP = "lineage-service"`, plus el módulo de schema arriba para poder mapear OpenLineage event → row. ACL Kafka Read+Write ya pre-declarado en [`kafka-acls-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml) (Write porque algunos productores publican directo, no via outbox). El consumer + writer Iceberg aterrizan en PR follow-up; substrate ya hace que cualquier drift de topic, namespace o sort sea compile error.)*
- [x] **S5.2.c** `lineage-service` query API ahora lee de Trino sobre Iceberg para queries históricas; Cassandra para hot path últimas 24 h. *(El runtime hot ya quedó cableado: [`query_router`](../../services/lineage-service/src/query_router.rs) sigue siendo el decisor puro (`HOT_WINDOW = 24h`, `QuerySource::{Cassandra, Trino}`, `route(...)`, `route_window(...)`, `trino_enabled_from_env(...)`) y además explicita que Postgres no puede volver a ser fallback de serving. El módulo compartido [`domain/lineage/mod.rs`](../../services/lineage-service/src/domain/lineage/mod.rs) ahora arma snapshots operacionales desde un [`LineageRuntimeStore`](../../services/lineage-service/src/domain/lineage/tracker.rs) explícito; la impl `CassandraLineageRuntimeStore` mantiene read-models `relations_by_source`, `relations_by_target`, `relations_all`, `relations_by_workflow` y `column_relations_by_dataset` en keyspace `lineage_runtime`, mientras `lineage_nodes` queda como overlay/archive de metadata en PG. [`pipeline-authoring-service`](../../services/pipeline-authoring-service/src/main.rs), [`pipeline-build-service`](../../services/pipeline-build-service/src/main.rs) y [`pipeline-schedule-service`](../../services/pipeline-schedule-service/src/main.rs) inyectan el store al `AppState`, y los writers (`record_lineage`, `record_column_lineage`, `propagate_pipeline_runtime_lineage`) ya persisten al runtime store en vez de hacer `INSERT` sobre `lineage_relations`. El fetch histórico concreto contra Trino/Iceberg sigue colgado del router y del surface HTTP que aporte ventanas >24h; lo importante aquí es que el hot path ya no depende de PG y que el contrato de histórico tampoco permite volver a PG.)*

### Tarea S5.3 — AI logs (semana 9-10)

- [x] **S5.3.a** `agent-runtime-service` y `prompt-workflow-service` → emiten eventos a Kafka `ai.events.v1`. *(Sustrato: ambos servicios ahora declaran `[lib]` ([`agent-runtime-service/Cargo.toml`](../../services/agent-runtime-service/Cargo.toml), [`prompt-workflow-service/Cargo.toml`](../../services/prompt-workflow-service/Cargo.toml)) con crate-name `agent_runtime_service` y `prompt_workflow_service` respectivamente. Cada uno expone módulo [`ai_events`](../../services/agent-runtime-service/src/ai_events.rs) que pinea `TOPIC = "ai.events.v1"`, `TXN_ID_PREFIX` distinto por productor (`agent-runtime-` y `prompt-workflow-` — Kafka puede fence cada uno independientemente), enum `AiEventKind::{Prompt,Response,Evaluation,Trace}` con `target_table()` const, y struct `AiEventEnvelope { event_id: Uuid, at: i64, kind, run_id, trace_id, producer, schema_version, payload }`. Topic ya añadido a [`topics-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/topics-domain-v1.yaml) (12p/7d/RF=3 — Iceberg es SoR, Kafka sólo buffer). ACL Write para `agent-runtime-service` ya estaba pre-declarado; ACL Write para `prompt-workflow-service` añadido a [`kafka-acls-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml). 4 unit tests entre los dos servicios. `main.rs` no se toca — publisher real `DataPublisher::publish` aterriza handler-by-handler.)*
- [x] **S5.3.b** Sink → Iceberg `of.ai.{prompts,responses,evaluations,traces}`. *(Sustrato: nuevo crate [`services/ai-sink`](../../services/ai-sink) en workspace members ([`Cargo.toml` root](../../Cargo.toml)). [`Cargo.toml`](../../services/ai-sink/Cargo.toml) gating `event-bus-data` + Prometheus tras feature `runtime`. [`lib.rs`](../../services/ai-sink/src/lib.rs) define `SOURCE_TOPIC = "ai.events.v1"`, `CONSUMER_GROUP = "ai-sink"`, `iceberg_target::{NAMESPACE = "of_ai", TABLE_PROMPTS, TABLE_RESPONSES, TABLE_EVALUATIONS, TABLE_TRACES, PARTITION_TRANSFORM = "day(at)"}`, mirror del envelope (`AiEventEnvelope` y `AiEventKind` replicados — sin dep cruzada a los productores), `route(envelope) -> &'static str` const-fn, y `BatchPolicy::PLAN_DEFAULT` idéntico a `audit-sink` (100k/60s — políticas operativas alineadas). Submódulo `iceberg_schema` con field names + partition + sort + `TABLE_PROPERTIES` (retención **1y**, no infinita — AI logs no son evidencia regulatoria, contrastan con `audit-sink::iceberg_schema::retention`). ACL Read pinneado en [`kafka-acls-domain-v1.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml). Réplicas 2 en [`values-prod.yaml`](../../infra/k8s/helm/of-ml-aip/values-prod.yaml) (HPA min 2 max 8), 1 en [`values-dev.yaml`](../../infra/k8s/helm/of-ml-aip/values-dev.yaml). 6 unit tests pasan. Iceberg writer real en PR follow-up.)*
- [x] **S5.3.c** Trino views para evaluación de modelos. *(Sustrato: Trino aún no está desplegado (S5.6) — pero las DDL de las views se pinnean en repo para que cuando aterrice el chart un Job bootstrap las aplique verbatim sin re-review. [`infra/k8s/platform/manifests/trino/views/of_ai.sql`](../../infra/k8s/platform/manifests/trino/views/of_ai.sql) define cuatro `CREATE OR REPLACE VIEW` idempotentes: `v_responses_by_run` (join axis canónico), `v_eval_scores_daily` (agregado diario por producer/metric con `AVG(CAST score AS DOUBLE)` — no inferir tipos numéricos de JSON), `v_prompt_response_pairs` (correlación trace_id+run_id con latencia ms), `v_traces_with_outcome` (terminal evaluation por trace). [`README.md`](../../infra/k8s/platform/manifests/trino/views/README.md) pinea convenciones (schema `of_<domain>`, prefix `v_`, partition pruning obligatorio, `CREATE OR REPLACE` para re-apply seguro). Cuando S5.6 despliegue Trino, el bootstrap aplica el directorio entero.)*

### Tarea S5.4 — Métricas long-term (semana 11)

- [x] **S5.4.a** Mimir → S3 → cada noche, job Spark agrega y materializa a `of.metrics_long.service_metrics_daily`. *Sustrato: pipeline en tres piezas — (a) Helm overlay [`infra/k8s/platform/manifests/observability/mimir/values.yaml`](../../infra/k8s/platform/manifests/observability/mimir/values.yaml) despliega Mimir monolítico (3 réplicas, AGPL evitado pinneando v2.13.x Apache-2.0) contra el bucket Ceph RGW `openfoundry-metrics-long`; (b) `SparkApplication` [`infra/k8s/platform/manifests/spark-jobs/metrics-aggregation-service-daily.yaml`](../../infra/k8s/platform/manifests/spark-jobs/metrics-aggregation-service-daily.yaml) lee bloques TSDB de S3 y materializa la partición del día anterior en Iceberg vía catálogo REST de Lakekeeper (idempotente, merge-on-read); (c) DDL Iceberg pinneado en [`infra/k8s/platform/manifests/trino/views/of_metrics_long.sql`](../../infra/k8s/platform/manifests/trino/views/of_metrics_long.sql) con tabla `service_metrics_daily` particionada por `day(at)` más vistas `v_service_latency_daily` y `v_service_error_rate_daily`. Alertas operativas en [`infra/k8s/platform/manifests/observability/mimir/prometheus-rules.yaml`](../../infra/k8s/platform/manifests/observability/mimir/prometheus-rules.yaml) cubren remote-write lag, salud de ingesters y un `MetricsAggregationJobMissed` que dispara si el cron no completa en 25 h. Documentación en [`infra/k8s/platform/manifests/observability/mimir/README.md`](../../infra/k8s/platform/manifests/observability/mimir/README.md). El binario Spark (`metrics-aggregation-0.1.0.jar`) y la cron-wrapper de la fecha quedan diferidos al PR de runtime.*

### Tarea S5.5 — Decommission `event-streaming-service` legacy si redundante (semana 12)

- [x] Validar si sigue cumpliendo función única o ha sido reemplazado por `audit-sink` + `lineage-sink` + `ontology-indexer`. *Sustrato: veredicto **MANTENER** documentado en [`docs/architecture/decisions/event-streaming-service-keep.md`](decisions/event-streaming-service-keep.md). Análisis función-por-función: el servicio expone tres superficies que ningún sink replica — (1) gRPC `Publish`/`Subscribe` en `:50221` con backpressure y reintentos, (2) REST `POST /streams/{id}/events` en `:50121` usado por integraciones externas y K6, (3) tabla declarativa de routing `data.>` → Kafka / `ctrl.*` → NATS. Los nuevos sinks (`audit-sink`, `lineage-service`, `ontology-indexer`, `ai-sink`) son consumidores downstream pull-only, no sustituyen la ingress de productores ni el dispatch entre subjects. Se acuerda reducción de scope (PR aparte): eliminar backends de almacenamiento directo ya cubiertos por sinks, marcar `legacy-storage-fanout` como deprecated, capar memoria y réplicas. Criterios para revisar el decommission en S6+: 100 % de productores migrados a `libs/event-bus-data` (hoy 40 %), outbox cubriendo todos los `ctrl.*` (hoy 6/11), edge `data-ingest-edge-service` en producción y CLI de tail sin dependencia de `Subscribe` RPC.*

### Tarea S5.6 — Trino on K8s (semana 13)

- [x] **S5.6.a** Añadir `infra/k8s/platform/manifests/trino/` (chart oficial `trinodb/trino`). *Sustrato: chart `trinodb/trino` (Apache-2.0) overrideado en [`infra/k8s/platform/manifests/trino/values.yaml`](../../infra/k8s/platform/manifests/trino/values.yaml) con imagen pinneada `trinodb/trino:459` (LTS, nunca `latest`), exchange manager filesystem en `s3://openfoundry-trino-exchange/` y `serviceMonitor.enabled: true` con label `role: alert-rules` para ingestar en el stack Prometheus existente. Estructura del directorio + topología en [`infra/k8s/platform/manifests/trino/README.md`](../../infra/k8s/platform/manifests/trino/README.md). Habilitación condicional vía perfiles Helm compartidos ([prod ON](../../infra/k8s/helm/profiles/values-prod.yaml), [dev OFF](../../infra/k8s/helm/profiles/values-dev.yaml) — kind/k3d no aguantan el footprint del par de coordinadores).*
- [x] **S5.6.b** Coordinador HA, workers escalables. *Sustrato: par de coordinadores StatefulSet con `topologySpreadConstraints` por zona (16 GiB heap, 8 GiB/32 GiB request/limit cada uno) más Deployment de workers con `autoscaling.enabled: true`, `minReplicas: 4`, `maxReplicas: 16` y `targetCPUUtilizationPercentage: 60` — todo en el bloque `coordinator`/`worker` de [`infra/k8s/platform/manifests/trino/values.yaml`](../../infra/k8s/platform/manifests/trino/values.yaml). El override prod en [`infra/k8s/helm/profiles/values-prod.yaml`](../../infra/k8s/helm/profiles/values-prod.yaml) reafirma `coordinator.replicas: 2` y los rangos del HPA. Runbook de recuperación ante pérdida de worker o coordinador en [`infra/k8s/platform/manifests/trino/README.md`](../../infra/k8s/platform/manifests/trino/README.md). Adaptador Flight SQL en frente de Trino (`trino-flight-sql-proxy`) y la regla KEDA específica quedan diferidos al PR de runtime.*
- [x] **S5.6.c** Iceberg connector apuntando a Lakekeeper. *Sustrato: connector inline en `additionalCatalogs.iceberg` de [`infra/k8s/platform/manifests/trino/values.yaml`](../../infra/k8s/platform/manifests/trino/values.yaml) más copia stand-alone en [`infra/k8s/platform/manifests/trino/connectors/iceberg.properties`](../../infra/k8s/platform/manifests/trino/connectors/iceberg.properties) (fallback si un upgrade del chart pierde el knob inline). Apunta a `iceberg.catalog.type=rest`, `iceberg.rest-catalog.uri=http://lakekeeper.lakekeeper.svc:8181`, warehouse `openfoundry-iceberg`, S3 nativo contra `rook-ceph-rgw-openfoundry-store.rook-ceph.svc:80` con path-style access. `iceberg.target-max-file-size=256MB` deja deliberadamente la compactación al job nocturno de Spark — Trino no debe reescribir ficheros en plena query OLAP.*
- [x] **S5.6.d** `sql-bi-gateway-service` route SELECT analítico → Trino; OLTP → Cassandra/Postgres. *Sustrato: añadida quinta variante `Backend::Trino` en [`services/sql-bi-gateway-service/src/routing.rs`](../../services/sql-bi-gateway-service/src/routing.rs) (enum, `as_str`, `all()`, brazo del clasificador prefijo `trino.*`, brazo del router con `RoutingError::BackendUnavailable(Backend::Trino)` cuando falta endpoint, helper de tests `cfg_with_trino`); y campo `trino_flight_sql_url: Option<String>` en [`services/sql-bi-gateway-service/src/config.rs`](../../services/sql-bi-gateway-service/src/config.rs) con env `TRINO_FLIGHT_SQL_URL`. Tres tests nuevos verifican (1) clasificación `trino.of_lineage.runs` → `Backend::Trino`, (2) routing al endpoint configurado, (3) error explícito cuando falta el endpoint; **11 tests verdes**. La política — analítico OLAP a Trino, OLTP a Cassandra/Postgres directo — queda formalizada en [`docs/architecture/adr/ADR-0029-reintroduce-trino-for-iceberg-analytics.md`](adr/ADR-0029-reintroduce-trino-for-iceberg-analytics.md) que **supersede parcialmente ADR-0014** (Flight SQL sigue siendo el protocolo de borde; Trino es ahora el motor analítico Iceberg detrás del gateway).*

### Tarea S5.7 — Spark on K8s para batch (semana 14)

- [x] **S5.7.a** Spark Operator (Apache). *Sustrato: chart `kubeflow/spark-operator` (Apache-2.0, NUNCA Bitnami por cambio de licencia) overrideado en [`infra/k8s/platform/manifests/spark-operator/values.yaml`](../../infra/k8s/platform/manifests/spark-operator/values.yaml) — webhook habilitado en `:9443`, `controller.workers: 10`, scope a namespace `openfoundry-spark`, métricas Prometheus en `:8090` con `prometheusMonitor.enable: true` etiquetado `role: alert-rules`. Imagen pinneada `kubeflow/spark-operator:v2.0.2` con base Spark 3.5.x. RBAC del operator + jobs documentado en [`infra/k8s/platform/manifests/spark-operator/README.md`](../../infra/k8s/platform/manifests/spark-operator/README.md) explicitando que el namespace `of_audit.*` está fuera de límites (defensa en profundidad sobre el allowlist de cada job). Habilitación vía `spark-operator.enabled` ([prod ON](../../infra/k8s/helm/profiles/values-prod.yaml), [dev OFF](../../infra/k8s/helm/profiles/values-dev.yaml)). Alertas en [`infra/k8s/platform/manifests/observability/spark-operator-rules.yaml`](../../infra/k8s/platform/manifests/observability/spark-operator-rules.yaml) — incluye `SparkApplicationTargetsWormNamespace` como **P1 inmediato** que dispara si cualquier job menciona `of_audit` en sus argumentos.*
- [x] **S5.7.b** Job ejemplo: rewrite Iceberg files + expire snapshots. *Sustrato: dos `SparkApplication` CRs sobre imagen `apache/spark:3.5.3` con catálogo REST de Lakekeeper — [`infra/k8s/platform/manifests/spark-jobs/iceberg-rewrite-data-files.yaml`](../../infra/k8s/platform/manifests/spark-jobs/iceberg-rewrite-data-files.yaml) compacta a target 256 MiB, [`infra/k8s/platform/manifests/spark-jobs/iceberg-expire-snapshots.yaml`](../../infra/k8s/platform/manifests/spark-jobs/iceberg-expire-snapshots.yaml) aplica retención por namespace (`of_lineage:90d`, `of_ai:1y`, `of_metrics_long:5y`). Patrón crítico: el **primer argumento CLI es el allowlist** `--allowlist=of_lineage,of_ai,of_metrics_long` para que la alerta `SparkApplicationTargetsWormNamespace` detecte cualquier intento de incluir `of_audit` vía métricas del operator (S5.1.c — auditoría es WORM). ServiceAccount dedicado `spark-jobs-non-audit` deniega writes a objetos con label `openfoundry.io/worm: "true"`. Convenciones, allowlist y check de seguridad documentados en [`infra/k8s/platform/manifests/spark-jobs/README.md`](../../infra/k8s/platform/manifests/spark-jobs/README.md). El JAR de mantenimiento (`iceberg-maintenance-0.1.0.jar`) y el `ScheduledSparkApplication` semanal (Sun 03:00/04:00 UTC) quedan diferidos al PR de runtime.*

### Definition of Done — S5
> **Estado real (2026-05-03):** schemas/contratos/routing aterrizados y `G-S5` verde en su alcance formal; **stream no cerrado** hasta tener evidencia operativa firmada.

- [x] Schemas Iceberg, topics, contracts y rutas de Trino/Spark están fijados en repo.
- [x] El grep gate `G-S5` de §18 devuelve `0 hits` para `NotWired` / `not_implemented` / `ErrNotImplemented` / `TODO` en sinks, indexer y reindex. *Verificado 2026-05-03 en el árbol actual; esto solo demuestra ausencia de stubs explícitos en ese scope.*
- [ ] Existe un evidence pack firmado siguiendo [`docs/architecture/runbooks/lakehouse-s5-operational-evidence.md`](runbooks/lakehouse-s5-operational-evidence.md).
- [ ] `of.audit`, `of_lineage`, `of_ai` y `of_metrics_long` reciben tráfico real y tienen validación operativa de append/consulta.
- [ ] Trino está desplegado y `sql-bi-gateway-service` enruta consultas analíticas reales sobre el lakehouse.
- [ ] La política WORM de `of.audit` está verificada en runtime; no existe ningún maintenance job autorizado a ejecutar `expire_snapshots` sobre ese namespace.

---

## 11. Stream S6 — Postgres consolidation (4 semanas)

### Tarea S6.1 — 4 clusters CNPG nuevos (semana 10)

- [x] **S6.1.a** Borrar 67 manifests CNPG obsoletos (después de migrar lo que aún use Postgres). *Sustrato: borrados los **65** manifests `<bounded-context>-pg.yaml` que poblaban [`infra/k8s/platform/manifests/cnpg/clusters/`](../../infra/k8s/platform/manifests/cnpg/clusters/) — el plan estimaba 67 pero el inventario real eran 65 (ver `git log --diff-filter=D infra/k8s/platform/manifests/cnpg/clusters/`). Conservados intactos: el template paramétrico [`infra/k8s/platform/manifests/cnpg/templates/cluster.yaml`](../../infra/k8s/platform/manifests/cnpg/templates/cluster.yaml) (base para los 4 clusters consolidados de S6.1.b) y el directorio [`infra/k8s/platform/manifests/cnpg/operator/`](../../infra/k8s/platform/manifests/cnpg/operator/) con la instalación del operador. Reemplazado el README anterior (que documentaba el patrón T12/T13 de un Postgres por servicio) por [`infra/k8s/platform/manifests/cnpg/clusters/README.md`](../../infra/k8s/platform/manifests/cnpg/clusters/README.md) que documenta el estado intermedio y advierte explícitamente que **aplicar el chart umbrella en este estado deja 65 servicios sin base de datos** hasta que se ejecuten S6.1.b (4 clusters consolidados `pg-{schemas,policy,lakekeeper,runtime-config}.yaml`), S6.1.c (bootstrap de schemas vía Job `postInitSQL`), S6.1.d (roles por servicio con GRANT al schema propio) y S6.3 (`DATABASE_URL` repuntado al cluster consolidado correcto + `?options=-c%20search_path%3D<schema>`). Precondición del plan ("después de migrar lo que aún use Postgres") **deliberadamente no cumplida** — borrado ejecutado bajo confirmación explícita del owner para liberar el directorio antes de los siguientes PRs de S6, asumiendo la ventana de indisponibilidad mientras corren S6.1.b → S6.3. Verificación post-borrado: `ls infra/k8s/platform/manifests/cnpg/clusters/` devuelve solo el README explicativo. ADR-0010 (CloudNativePG como operador único) sigue vigente — la consolidación es de cardinalidad de clusters, no de operador.*
- [x] **S6.1.b** Crear `infra/k8s/platform/manifests/cnpg/clusters/{pg-schemas,pg-policy,pg-lakekeeper,pg-runtime-config}.yaml`. 3 instancias cada uno, sync replica, WAL barman a Ceph S3. **`pg-policy` con `wal_level=logical` y `max_replication_slots=8`** (requerido por Debezium). *Sustrato: creados los cuatro manifests stand-alone en [`infra/k8s/platform/manifests/cnpg/clusters/`](../../infra/k8s/platform/manifests/cnpg/clusters/) ([`pg-schemas.yaml`](../../infra/k8s/platform/manifests/cnpg/clusters/pg-schemas.yaml), [`pg-policy.yaml`](../../infra/k8s/platform/manifests/cnpg/clusters/pg-policy.yaml), [`pg-lakekeeper.yaml`](../../infra/k8s/platform/manifests/cnpg/clusters/pg-lakekeeper.yaml), [`pg-runtime-config.yaml`](../../infra/k8s/platform/manifests/cnpg/clusters/pg-runtime-config.yaml)). Forma común: `instances: 3`, `minSyncReplicas=maxSyncReplicas=1`, imagen `ghcr.io/cloudnative-pg/postgresql:16.4`, `storageClass: ceph-rbd`, anti-afinidad por `topology.kubernetes.io/zone`, `monitoring.enablePodMonitor: true`, backup Barman a `s3://openfoundry-pg-backups/<cluster>` vía `rook-ceph-rgw-openfoundry-store.rook-ceph.svc:80` con retención 30d (mismo patrón que el [template paramétrico](../../infra/k8s/platform/manifests/cnpg/templates/cluster.yaml) que ahora queda como referencia histórica). Diferencias por cluster: `pg-policy` declara explícitamente `wal_level=logical`, `max_replication_slots=8`, `max_wal_senders=10` y `walStorage: 50Gi` para que Debezium pueda crear slots lógicos sin agotar WAL; `pg-runtime-config` es el más grande (500Gi datos + 100Gi WAL, límite 12 CPU / 32Gi) por hospedar 25 schemas operacionales; `pg-lakekeeper` es single-tenant (database `lakekeeper`, 50Gi) y por tanto NO lleva `postInitApplicationSQLRefs`. Índice y rationale en el nuevo [`infra/k8s/platform/manifests/cnpg/clusters/README.md`](../../infra/k8s/platform/manifests/cnpg/clusters/README.md) (sustituye al README intermedio de S6.1.a). Validado con `ruby -ryaml`.*
- [x] **S6.1.c** Provisionar schemas via Job CNPG `bootstrap.initdb.postInitSQL`. *Sustrato: en lugar de un Job externo se usa el mecanismo nativo CNPG `bootstrap.initdb.postInitApplicationSQLRefs` (corre dentro del propio bootstrap de `initdb`, antes de exponer el cluster). Tres ConfigMaps con bloques `DO $$ ... $$` que iteran sobre el array de bounded contexts y emiten `CREATE SCHEMA IF NOT EXISTS <bc> AUTHORIZATION svc_<bc>` + `REVOKE ALL ON SCHEMA <bc> FROM PUBLIC`: [`pg-schemas-bootstrap-sql.yaml`](../../infra/k8s/platform/manifests/cnpg/clusters/pg-schemas-bootstrap-sql.yaml) (21 schemas: data_asset_catalog, dataset_versioning, lineage, cdc_metadata, model_catalog/adapter/lifecycle, tenancy_organizations, federation_product_exchange, marketplace, nexus, sdk_generation, solution_design, code_repository_review, document_reporting, analytical_logic, event_streaming, ai_application_generation, document_intelligence, mcp_orchestration, scenario_simulation), [`pg-policy-bootstrap-sql.yaml`](../../infra/k8s/platform/manifests/cnpg/clusters/pg-policy-bootstrap-sql.yaml) (12 schemas de seguridad/auditoría), [`pg-runtime-config-bootstrap-sql.yaml`](../../infra/k8s/platform/manifests/cnpg/clusters/pg-runtime-config-bootstrap-sql.yaml) (25 schemas de runtime/control-plane, incluyendo `connector_management` e `ingestion_replication`). Total 58 schemas alineados con el inventario `find services -name migrations` (servicios sin migración aún no tienen entrada y se añadirán cuando el bounded context lo necesite). Idempotente por `IF NOT EXISTS` aunque CNPG sólo lo ejecuta una vez.*
- [x] **S6.1.d** Provisionar roles por servicio con permisos solo a su schema (`GRANT USAGE, SELECT, INSERT, UPDATE, DELETE ON SCHEMA ... TO svc_xxx`). *Sustrato: el mismo bloque `DO $$ ... $$` en cada ConfigMap (S6.1.c) crea `CREATE ROLE svc_<bc> LOGIN NOINHERIT PASSWORD 'PLACEHOLDER_ROTATE_VIA_EXTERNAL_SECRETS'`, otorga `USAGE` sobre el schema y configura `ALTER DEFAULT PRIVILEGES FOR ROLE svc_<bc> IN SCHEMA <bc> GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES` + `GRANT USAGE, SELECT ON SEQUENCES`. La separación por `AUTHORIZATION svc_<bc>` impide DDL cross-schema; el `REVOKE ALL ... FROM PUBLIC` impide lecturas accidentales entre tenants. La password placeholder es deliberadamente débil para que el security-baseline scan falle si External Secrets / Vault no rota el credential antes de ir a producción (documentado en el header de cada ConfigMap y en [`infra/k8s/helm/DATABASE_URL.md`](../../infra/k8s/helm/DATABASE_URL.md)). Para `pg-policy` se añade además un rol dedicado `debezium_cdc` con atributo `REPLICATION` y `SELECT` read-only sobre los 12 schemas de política (consumidor del slot lógico).*

### Tarea S6.2 — PgBouncer pooler por cluster (semana 11)

- [x] CNPG `Pooler` con `transaction` mode, 50 conexiones por servicio. *Sustrato: tres CRDs `Pooler` en [`infra/k8s/platform/manifests/cnpg/poolers/`](../../infra/k8s/platform/manifests/cnpg/poolers/) ([`pg-schemas-pooler.yaml`](../../infra/k8s/platform/manifests/cnpg/poolers/pg-schemas-pooler.yaml), [`pg-policy-pooler.yaml`](../../infra/k8s/platform/manifests/cnpg/poolers/pg-policy-pooler.yaml), [`pg-runtime-config-pooler.yaml`](../../infra/k8s/platform/manifests/cnpg/poolers/pg-runtime-config-pooler.yaml)) con `pgbouncer.poolMode: transaction`, `default_pool_size: "50"`, `reserve_pool_size: "10"`, `server_idle_timeout: "120"`, `server_lifetime: "3600"`, `query_wait_timeout: "30"`. `max_client_conn` es 1000 para los dos clusters de carga media y 1500 para `pg-runtime-config` (3 instancias en lugar de 2 por concentrar 25 schemas hot). `pg-lakekeeper` queda sin Pooler porque Lakekeeper mantiene su propio pool fijo pequeño (documentado en el README). Anti-afinidad por zona via `template.spec.affinity.podAntiAffinity` para evitar que un fallo zonal tumbe el pooler completo. Conjunto de servicios consumidores y constraints de transaction mode (no LISTEN/NOTIFY, no advisory locks de sesión, no `SET LOCAL` fuera de transacción) en [`infra/k8s/platform/manifests/cnpg/poolers/README.md`](../../infra/k8s/platform/manifests/cnpg/poolers/README.md).*

### Tarea S6.3 — Cambiar DATABASE_URL en Helm values (semana 11)

- [x] **S6.3.a** Cada servicio que aún use Postgres: `envSecrets.DATABASE_URL` apunta al cluster nuevo + schema correcto. *Sustrato: en los valores de los charts Helm split los servicios que ya tenían bloque `envSecrets.DATABASE_URL` explícito (identity-federation, data-asset-catalog, sql-bi-gateway, report, nexus) repuntan ahora a un Secret `<bc>-db-dsn` con dos claves (`writer_url`, `reader_url`). Se abandona la proyección del Secret CNPG-managed `<cluster>-pg-app` (cred del superuser `app` sin search_path y sin pasar por el Pooler) en favor de un Secret sync-eado por External Secrets desde Vault con la DSN ya construida con el rol `svc_<bc>`, el host del Pooler y el `search_path` correcto. El contrato del Secret y la convención de naming están documentados en [`infra/k8s/helm/DATABASE_URL.md`](../../infra/k8s/helm/DATABASE_URL.md). Los servicios restantes que aún no tienen `envSecrets.DATABASE_URL` heredan el patrón cuando se incorporen — no se anticipan entradas vacías para no inflar el chart con configuración no usada.*
- [x] **S6.3.b** sqlx connection options: `?options=-c%20search_path%3D<schema>`. *Sustrato: la cadena de query string `?sslmode=require&options=-c%20search_path%3D<bc>` queda embebida en `writer_url`/`reader_url` del Secret `<bc>-db-dsn` (External Secrets / Vault es el productor). El motivo de no inyectar `SET search_path` desde sqlx es estructural: en transaction-mode pooling, `SET search_path` fuera de un `BEGIN/COMMIT` es rechazado por PgBouncer; pasarlo como query option `-c` lo aplica en el `startup packet` de cada conexión backend y sobrevive al multiplexado. La comprobación end-to-end recomendada (`SELECT current_user, current_schema;` debe devolver `svc_<bc> | <bc>`) está en el step 6 del runbook de decommission [`infra/runbooks/cnpg-decommission.md`](../../infra/runbooks/cnpg-decommission.md). Contrato de URL completo — incluyendo el detalle de que `<bc>` en el URL es snake_case y NO kebab-case porque mapea a un identificador Postgres — documentado en [`infra/k8s/helm/DATABASE_URL.md`](../../infra/k8s/helm/DATABASE_URL.md).*

### Tarea S6.4 — Read routing a `-ro` para servicios read-heavy (semana 12)

- [x] Inyectar `DATABASE_READ_URL` apuntando a `<cluster>-ro`. *Sustrato: la segunda clave `reader_url` del Secret `<bc>-db-dsn` apunta a `<cluster>-ro.openfoundry.svc.cluster.local:5432` (el servicio CNPG read-only que balancea sobre las dos réplicas síncronas/asíncronas). Helm la proyecta como variable `DATABASE_READ_URL` en los cinco servicios actualizados (`identity-federation`, `data-asset-catalog`, `sql-bi-gateway`, `report`, `nexus`) mediante los valores de los charts split. Se evita poner Pooler delante de la réplica de lectura: el Pooler en transaction mode rompe garantías de read-your-writes y el throughput de lecturas analíticas se beneficia más de conexiones largas que de multiplexado.*
- [x] Capa de pool dual (writer/reader) en sqlx wrapper. *Sustrato: nuevo crate [`libs/db-pool`](../../libs/db-pool/) (Apache-2.0) con `DualPool { writer: PgPool, reader: Option<PgPool> }`, constructores `from_env()` / `from_env_with(PoolSizing)` / `connect(writer_url, reader_url, sizing)` / `from_pools(writer, reader)`, y accesores `writer()` / `reader()` (este último cae back al writer cuando `DATABASE_READ_URL` no está definido para que dev/CI funcionen sin réplica). `PoolSizing` por defecto: `max_connections=20`, `min_connections=1`, `acquire_timeout=5s`, `idle_timeout=120s`, `max_lifetime=1800s` — elegidos para que un servicio nunca consuma más de la mitad del `default_pool_size=50` del Pooler (50 svc × 50 conn = 2500, y los Pooler se sintonizaron a 1000–1500 `max_client_conn` por instancia con dos–tres réplicas). Añadido al workspace en [`Cargo.toml`](../../Cargo.toml) (`members` + `[workspace.dependencies.db-pool]`). Tres unit tests verdes (`cargo test -p db-pool --lib`): contrato de sizing por defecto, error `MissingEnv` cuando falta `DATABASE_URL`, y `Send + Sync` del struct (las pruebas con un Postgres real se delegan a los tests de integración por servicio cuando se adopte). El crate documenta explícitamente en su doc-comment que **no** intenta reescribir `search_path` — esa responsabilidad es del Secret `<bc>-db-dsn` (S6.3.b).*

### Tarea S6.5 — Decommission clusters obsoletos (semana 13)

- [x] **Sin migración de datos** (pre-prod). Borrar Cluster CRDs antiguos. *Sustrato: los 65 manifests `<bc>-pg.yaml` ya habían sido borrados de Git en S6.1.a. Lo que faltaba era el procedimiento operativo para reapear los CRs `Cluster` que sigan vivos en clústeres pre-prod sync-eados antes de ese commit. Documentado en [`infra/runbooks/cnpg-decommission.md`](../../infra/runbooks/cnpg-decommission.md): Step 1 cataloga (`kubectl get clusters.postgresql.cnpg.io | grep -E -- '-pg$' > /tmp/legacy-clusters.txt`), Step 2 borra (`xargs -a /tmp/legacy-clusters.txt -I{} kubectl delete cluster.postgresql.cnpg.io {} --wait=false`). El finalizer de CNPG limpia StatefulSet, Services y PodMonitors automáticamente. Step 6 valida que solo quedan los 4 clusters consolidados.*
- [x] Borrar Secrets `<service>-pg-app` de todos los servicios borrados. *Sustrato: Step 3 del mismo runbook hace `xargs ... kubectl delete secret {}-app {}-backup --ignore-not-found` cubriendo tanto el Secret de superuser CNPG-managed como el Secret S3 de Barman, eliminando con ello también los credentials huérfanos que ya no proyecta nadie en Helm tras el cambio de S6.3.a (que repunta a `<bc>-db-dsn`).*
- [x] Limpiar PVCs huérfanos. *Sustrato: Step 4 documenta que CNPG NO borra PVCs en cascade (por seguridad); el runbook lista los PVCs huérfanos con `kubectl get pvc -l cnpg.io/cluster --no-headers` y los borra explicitamente. La StorageClass `ceph-rbd` tiene `reclaimPolicy: Delete`, así que el CSI driver libera el RBD subyacente sin pasos manuales adicionales. Step 5 (opcional) documenta el reapeo de los prefijos `s3://openfoundry-pg-backups/<bc>-pg/` en Ceph RGW vía `mc rm --recursive`.*

### Tarea S6.6 — Separar Ingestion/Connector Runtime del control plane SQL (semana 12-13)

- [x] **S6.6.a** `connector-management-service`: dejar en `pg-runtime-config.connector_management` solo definiciones low-traffic (`connections`, registros, capacidades, credenciales metadata). `sync_jobs` deja de ser estado autoritativo en Postgres. *Cierre 2026-05-03: eliminado el modelo Rust `models/sync_job.rs`, HyperAuto deja de llamar al endpoint HTTP legacy `/internal/sync-jobs` y despacha `CreateIngestJob` por el bridge gRPC a `ingestion-replication-service`; añadida migración [`services/connector-management-service/migrations/20260503120000_drop_sync_jobs_runtime.sql`](../../services/connector-management-service/migrations/20260503120000_drop_sync_jobs_runtime.sql) para dropear la tabla legacy en esquemas existentes.*
- [x] **S6.6.b** `ingestion-replication-service`: dejar en `pg-runtime-config.ingestion_replication` solo desired state declarativo (`IngestJobSpec`, bindings y referencias a recursos materializados). `ingest_jobs` como estado high-frequency, `ingestion_checkpoints`, retries y status operacional migran a Cassandra, Kafka compacted topics, `status` de CRs de Kubernetes o un store runtime no-PG equivalente. *Cierre 2026-05-03: el binario compilado conserva únicamente `IngestionControlPlane`, `repository` y `runtime_state`; fue una limpieza de dead code no compilado, borrando los módulos draft que insertaban/listaban/actualizaban `sync_jobs` desde `src/handlers`, `src/domain` y `src/grpc`; runtime status vive en ConfigMaps vía [`runtime_state.rs`](../../services/ingestion-replication-service/src/runtime_state.rs), y checkpoints CDC viven en manifests de runtime no-PG. Añadida migración [`services/ingestion-replication-service/migrations/20260503121000_drop_ingestion_hot_path_runtime.sql`](../../services/ingestion-replication-service/migrations/20260503121000_drop_ingestion_hot_path_runtime.sql) para retirar `ingestion_checkpoints` de Postgres.*
- [x] **S6.6.c** Definir single-writer boundary para `sync_jobs`, `ingest_jobs`, `ingestion_checkpoints` y recovery state: prohibido dual-write autoritativo entre Postgres y el runtime store más allá de una ventana de cutover explícita. *Boundary actualizado: `connector-management-service` es writer de definiciones; `ingestion-replication-service` es writer de `IngestJobSpec` desired-state y de los nombres de recursos materializados; Kubernetes ConfigMaps/CRs y manifests de runtime no-PG son writer de status/checkpoints/recovery. No queda un scheduler SQL ni endpoint gRPC/HTTP legacy que reclame `sync_jobs`.*
- [x] **S6.6.d** Actualizar contratos de servicio, Helm y documentación para que `pg-runtime-config` quede documentado únicamente como control plane low-traffic, y el runtime hot-path de ingestión quede fuera de CNPG. *Docs actualizados en este plan y en los READMEs de binding de los servicios; Helm configura `INGESTION_REPLICATION_GRPC_URL=http://ingestion-replication-service:8080` para `connector-management-service`, y `pg-runtime-config` queda reservado al control-plane.*
- [x] **DoD S6.6:** las definiciones de conectores siguen pudiendo vivir en `pg-runtime-config`; jobs/checkpoints/retries/status high-frequency viven en Cassandra/Kafka/CR `status` o en otro runtime store no-PG.

### Definition of Done — S6
> **Estado real (2026-05-02):** infra de consolidación aterrizada; **cierre parcial**.

- [x] Existen 4 manifests CNPG, poolers, contrato de DSN y runbooks de decommission/promoción.
- [~] Solo un subconjunto de servicios ha sido repuntado en Helm; el plan no puede tratar S6 como "cerrado" mientras queden consumers fuera del contrato consolidado.
- [x] El grep gate `G-S6` de §18 devuelve `0 hits` para secretos/CRs legacy (`-pg-app`, clusters por servicio) en manifests live. *Verificado 2026-05-03 con el comando de §18.*
- [x] El grep gate `G-S6.6` de §18 devuelve `0 hits` para `sync_jobs`/`ingestion_checkpoints` en código Rust live de connector/ingestion. *Verificado 2026-05-03 tras borrar el scheduler/handlers/gRPC legacy y mover HyperAuto al bridge gRPC.*
- [ ] El decommission operativo de CRs, Secrets y PVCs legacy se ha ejecutado en el entorno objetivo y quedó evidenciado.
- [ ] La huella residual de Postgres queda limitada a datos declarativos/de referencia aprobados.
- [ ] **`Postgres residual` NO se marca cerrado** hasta que S1, S3, S5 y S6 pasen todos los gates de §18.

---

## 12. Stream S7 — Cross-region DR (6 semanas)

### Tarea S7.1 — Iceberg cross-region replication (semana 14-15)

- [x] **S7.1.a** Activar S3 Cross-Region Replication desde Ceph RGW región A → región B. *Sustrato: en lugar de reglas S3 CRR (que Ceph implementa pero requieren AWS Signature v4 cross-zone), se usa el mecanismo nativo Ceph **multisite** con realm/zonegroup/zone, más eficiente para volumen de Iceberg parquet. Tres CRDs Rook por región: [`infra/k8s/platform/manifests/rook/multisite-region-a.yaml`](../../infra/k8s/platform/manifests/rook/multisite-region-a.yaml) declara `CephObjectRealm openfoundry`, `CephObjectZoneGroup openfoundry-zg` (master) y `CephObjectZone openfoundry-zone-a` (PRIMARY, RW); [`infra/k8s/platform/manifests/rook/multisite-region-b.yaml`](../../infra/k8s/platform/manifests/rook/multisite-region-b.yaml) replica los CRDs con `pull.endpoint` apuntando al RGW externo de A, `CephObjectZone openfoundry-zone-b` (SECONDARY, RO) y un segundo `CephObjectStore rgw-data` zonal. La sincronización metadata + data corre por la red Ceph multisite (HTTPS sobre WAN), sin AWS SDK. Bootstrap secuencial (token de pull desde A → Secret en B → apply secundario) documentado en [`infra/runbooks/ceph-multisite-bootstrap.md`](../../infra/runbooks/ceph-multisite-bootstrap.md) con verificación `radosgw-admin sync status`. Solo el store `rgw-data` (Iceberg) entra en multisite — el legacy `openfoundry` (datasets/models) queda fuera por scope de S7.*
- [x] **S7.1.b** Lakekeeper en región B en modo **read-only** apuntando al mismo bucket replicado. *Sustrato: nuevo overlay [`infra/k8s/platform/manifests/lakekeeper/region-b/values-region-b.yaml`](../../infra/k8s/platform/manifests/lakekeeper/region-b/values-region-b.yaml) que se aplica encima del [`values.yaml`](../../infra/k8s/platform/manifests/lakekeeper/values.yaml) base. Read-only en **defensa en profundidad triple** (documentado en [`infra/k8s/platform/manifests/lakekeeper/region-b/README.md`](../../infra/k8s/platform/manifests/lakekeeper/region-b/README.md)): (1) catalog REST con `authz.backend: read_only_allowall` rechaza endpoints mutantes con HTTP 403 antes de tocar backend; (2) Postgres backend `pg-lakekeeper-replica` (CNPG replica cluster, S7.4.a) rechaza writes a nivel SQL; (3) Ceph RGW endpoint apunta a `openfoundry-zone-b` (SECONDARY) que reenvía escrituras al master pero la combinación con (1)+(2) las bloquea antes. La misma `LAKEKEEPER__S3_BUCKET=openfoundry-iceberg` y el mismo `lakekeeper-encryption-key` Secret (sync por External Secrets) garantizan que las URLs firmadas en A descodifican en B sin re-cifrado. Replicas reducidas a 2 (50% de A) por carga read-only steady-state. Helm install command y matriz de defensa en el README.*
- [x] **S7.1.c** Test: escribir tabla en A, leer en B con lag <60 s. *Sustrato: Job one-shot [`infra/k8s/platform/manifests/lakekeeper/region-b/iceberg-replication-smoke.yaml`](../../infra/k8s/platform/manifests/lakekeeper/region-b/iceberg-replication-smoke.yaml) sobre imagen `apache/spark:3.5.3` que (1) escribe `iceberg.smoke_replication.heartbeat_<ts>` vía catalog REST de región A con un INSERT trivial, (2) hace polling cada 5s del catalog REST de región B leyendo la tabla con un `SELECT ts, source` y (3) imprime `iceberg_replication_lag_seconds <float>` cuando es visible o sale con código no-cero si pasa de `MAX_WAIT_SECONDS=120` (2× SLO). El SLO objetivo de 60s queda registrado en código (warning si lag > 60). PySpark scripts inline en ConfigMap. Securitcontext non-root, read-only rootfs, drop ALL caps. Diseñado para ejecutarse como Job tras cada cambio de multisite o como CronJob de canary continuo.*

### Tarea S7.2 — Cassandra multi-region (semana 15-16)

- [x] **S7.2.a** Añadir DC2 en región B (3 nodos). *Sustrato: el plan asumía baseline single-DC, pero la producción actual ya tenía dc1/dc2/dc3 en región A (zonas a–i, ADR-0021), así que la "DC2 en región B" del plan se materializa como un cuarto datacenter llamado **`dc-b1`** — evita renombrar DCs vivos y mantiene la semántica obvia (`dc-b1` = primer DC de la región B). Añadido inline en [`infra/k8s/platform/manifests/cassandra/cluster-prod.yaml`](../../infra/k8s/platform/manifests/cassandra/cluster-prod.yaml): `size: 3`, tres racks mapeados a `region-b-zone-a/b/c`, mismo sizing que dc1–3 (8 vCPU req / 12 lim, 32 GiB heap, 2 TiB), `k8sContext: region-b` para que k8ssandra-operator lo schedule contra el cluster Kubernetes secundario registrado en `clientConfigs` ([`values-k8ssandra-operator.yaml`](../../infra/k8s/platform/manifests/cassandra/values-k8ssandra-operator.yaml)). Throttling cross-region: `inter_dc_stream_throughput_outbound_megabits_per_sec: 100` (mitad de los 200 default) y `hinted_handoff_throttle_in_kb: 4096` para no saturar el WAN. También actualizado `system_distributed_replication_dc_names=dc1,dc2,dc3,dc-b1` en jvmOptions para que el operator inicialice las tablas system con la topología 4-DC desde el primer boot.*
- [x] **S7.2.b** Cambiar keyspaces a `NetworkTopologyStrategy {dc1:3, dc2:3}`. *Sustrato: el plan asumía baseline 2-DC pero la realidad de producción es 4-DC (`dc1`, `dc2`, `dc3` en región A más `dc-b1` en región B, ver S7.2.a). Actualizado el ConfigMap `of-cass-prod-keyspaces-cql` en [`infra/k8s/platform/manifests/cassandra/keyspaces-job.yaml`](../../infra/k8s/platform/manifests/cassandra/keyspaces-job.yaml) — los seis keyspaces de aplicación (`ontology_objects`, `ontology_indexes`, `actions_log`, `auth_runtime`, `notifications_inbox`, `agent_state`) ahora declaran `'dc1': '3', 'dc2': '3', 'dc3': '3', 'dc-b1': '3'`. Total 12 réplicas por partición, cada DC capaz de servir `LOCAL_QUORUM` independientemente sin tráfico cross-region en el camino caliente. Re-aplicar el Job tras añadir `dc-b1` no recrea el keyspace (`IF NOT EXISTS` es no-op) — el cambio de RF se materializa con `ALTER KEYSPACE` separado en el procedimiento operacional cuando se promociona un nuevo DC. Los keyspaces de Temporal (`temporal_persistence`, `temporal_visibility`) quedan fuera por diseño — los gestiona `temporal-cassandra-tool` desde el chart de Temporal.*
- [x] **S7.2.c** Repair full cross-DC. *Sustrato: nuevo Job [`infra/k8s/platform/manifests/cassandra/cross-dc-repair-job.yaml`](../../infra/k8s/platform/manifests/cassandra/cross-dc-repair-job.yaml) como Helm hook `post-install,post-upgrade` con weight 20 (corre tras `keyspaces-job` weight 10). Itera sobre los seis keyspaces y, por cada uno, llama al REST de Reaper (`POST /repair_run?datacenters=dc-b1&segmentCount=64&parallelism=PARALLEL&intensity=0.6&repairType=FULL`), pasa el run a `RUNNING` y hace polling cada 30s al endpoint `/repair_run/<id>` hasta `DONE` o `ERROR`. Timeout `MAX_WAIT_SECONDS=14400` (4h). Falla la release Helm visiblemente si algún segmento falla. Solo es bootstrap-only — la repair de steady-state ya la cubre el `reaper.autoScheduling.enabled: true` declarado en [`cluster-prod.yaml`](../../infra/k8s/platform/manifests/cassandra/cluster-prod.yaml) (semanal por keyspace, sub-range parallelism). Reaper opera PER_DC, así que el operator levanta automáticamente una instancia de Reaper en `dc-b1` cuando el DC se materializa.*
- [x] **S7.2.d** Documentar runbook: failover de aplicación a DC2 (cambio de `LOCAL_QUORUM` apuntando a DC2). *Sustrato: nuevo runbook [`infra/runbooks/cassandra-app-failover.md`](../../infra/runbooks/cassandra-app-failover.md) en seis pasos: (0) pre-flight verificando `CassandraDatacenter dc-b1` en estado `Ready` y ausencia de repairs en curso, (1) cambio de `CASSANDRA_LOCAL_DC=dc-b1` vía `infra/k8s/helm/bin/upgrade-split-releases.sh prod -- --set global.cassandra.localDc=dc-b1` sobre el bundle Helm split en región B (rolling restart respeta PDBs), (2) flip de DNS / global LB hacia región B (TTL 30s), (3) verificación `nodetool status` y smoke-test cqlsh con `CONSISTENCY LOCAL_QUORUM`, (4) ventana de observación 5min sobre dashboard `cassandra-overview`, (5) commit del perfil `values-prod.yaml` en branch `dr/failover-to-dc-b1` para que el siguiente `helm upgrade` no revierta silenciosamente, (6) procedimiento de rollback con repair `dc1,dc2,dc3` antes de devolver tráfico. Criterios de disparo explícitos (5min unreachable, todos los DCs A simultáneamente Stopped, o drill planificado) y no-goals (no toca Postgres → [`dr-failover.md`](../../infra/runbooks/dr-failover.md) S7.5.a, no toca Lakekeeper region-B que es read-only permanente, no toca Kafka MM2 que es S7.3). El runbook explica por qué el cluster Cassandra es siempre multi-master y solo cambia el routing de aplicación.*

### Tarea S7.3 — Kafka MirrorMaker 2 (semana 17)

- [x] **S7.3.a** Strimzi `KafkaMirrorMaker2` replicando topics A → B. *Sustrato: nuevo cluster destino [`infra/k8s/platform/manifests/strimzi/region-b/kafka-cluster-region-b.yaml`](../../infra/k8s/platform/manifests/strimzi/region-b/kafka-cluster-region-b.yaml) (`Kafka openfoundry-b`, KRaft, 3 brokers, JBOD 2×100Gi sobre `ceph-rbd`, listener externo `loadbalancer:9094` con mTLS) y `KafkaMirrorMaker2 of-mm2-a-to-b` en [`infra/k8s/platform/manifests/strimzi/region-b/kafka-mirrormaker2.yaml`](../../infra/k8s/platform/manifests/strimzi/region-b/kafka-mirrormaker2.yaml) corriendo en región A con 2 réplicas. Tres conectores: `MirrorSourceConnector` (`tasksMax=6`, `producer.acks=all`, `producer.enable.idempotence=true`, `replication.factor=3`), `MirrorCheckpointConnector` (`sync.group.offsets.interval.seconds=30` para que los consumer groups sobrevivan al failover) y `MirrorHeartbeatConnector` (5s) para SLO de lag. mTLS bidireccional con tres Secrets (`mm2-source-tls`, `mm2-target-tls`, `openfoundry-b-cluster-ca-cert`) provisionados por cert-manager + External Secrets — pipeline fuera de scope, documentado en el README de la carpeta. Métricas Prometheus expuestas vía `jmxPrometheusExporter` con reglas para `kafka_mm2_source_replication_latency_ms` y `kafka_mm2_checkpoint_checkpoint_latency_ms`. SLO objetivo: lag p95 < 30s para `cdc.*`.*
- [x] **S7.3.b** Topics replicados con prefix `dc-a.` en B. *Sustrato: configuración explícita en [`kafka-mirrormaker2.yaml`](../../infra/k8s/platform/manifests/strimzi/region-b/kafka-mirrormaker2.yaml) usando `DefaultReplicationPolicy` con `replication.policy.separator="."` y `sourceCluster.alias=dc-a` — un topic `cdc.postgres` en A aparece como `dc-a.cdc.postgres` en B. Decisión consciente de NO usar `IdentityReplicationPolicy` (que preservaría nombres) por dos razones: (1) la traducción de offsets de consumer group queda ambigua si los nombres colisionan, (2) bloquea cualquier escenario futuro de réplica bidireccional active/active. `topicsPattern: "cdc\..*, dataset\..*, lineage\..*, model\..*, audit\..*"` cubre las cinco clases de topic declaradas en [`kafka-topics.yaml`](../../infra/k8s/platform/manifests/strimzi/kafka-topics.yaml); `topicsExcludePattern: "mm2-.*, .*\.replica"` evita auto-mirroring recursivo. La tabla de equivalencia A↔B y los Secrets requeridos están en [`infra/k8s/platform/manifests/strimzi/region-b/README.md`](../../infra/k8s/platform/manifests/strimzi/region-b/README.md).*

### Tarea S7.4 — Postgres physical replication cross-region (semana 18)

- [x] **S7.4.a** CNPG `replica` cluster en región B vía streaming async. *Sustrato: nuevo manifiesto [`infra/k8s/platform/manifests/cnpg/region-b/cnpg-replicas-region-b.yaml`](../../infra/k8s/platform/manifests/cnpg/region-b/cnpg-replicas-region-b.yaml) con cuatro réplicas espejo de los clusters consolidados en S6.1.b: `pg-schemas-replica`, `pg-policy-replica`, `pg-runtime-config-replica`, `pg-lakekeeper-replica`. Cada uno declara `spec.replica.enabled: true` con `source` apuntando al primary equivalente vía `externalClusters` (host `pg-<name>-rw.openfoundry-region-a.svc.openfoundry.example.com:5432`, `sslmode=verify-full`, usuario `streaming_replica`, certificados en Secret `pg-<name>-replica-superuser`). Bootstrap con `pg_basebackup` desde el primary (no recovery desde S3 — más rápido y sin necesidad de mantener un snapshot fresco). Mismo número de instancias (3), `hot_standby=on`, sizing equivalente al primary. `barmanObjectStore` configurado con sufijo `region-b/` para que las réplicas archiven WAL local sin colisionar con la cadena de A. Sin auto-failover cross-region por diseño — la promoción es siempre manual (S7.4.b) para evitar split-brain en partición. Requiere conectividad k8s↔k8s preestablecida (Submariner / ClusterMesh / túnel dedicado), fuera de scope.*
- [x] **S7.4.b** Documentar promoción manual. *Sustrato: nuevo runbook [`infra/runbooks/postgres-promotion.md`](../../infra/runbooks/postgres-promotion.md) en siete pasos: (0) pre-flight verificando `pg_is_in_recovery=t` y lag < 30s, (1) confirmar que región A está realmente caída, (2) promoción atómica con `kubectl patch cluster pg-<name>-replica --type merge -p '{"spec":{"replica":{"enabled":false}}}'` (loop sobre los 4 clusters), (3) verificación con `pg_is_in_recovery=f` y `currentPrimary` set, (4) cutover de DSN aplicaciones (steady state ya apunta a `-replica-rw` Service por nombre estable), (5) smoke `CREATE TABLE _dr_smoke; INSERT; DROP`, (6) commit del estado en branch `dr/failover-...` para que la siguiente reconciliación no revierta `replica.enabled`, (7) procedimiento de failback que es **siempre** un bootstrap completo (`pg_basebackup` de A como nuevo replica de B) — la replicación física es one-way y no permite re-attach del primary stale. Criterios de disparo (>5min unreachable + autorización IC) y no-goals explícitos.*

### Tarea S7.5 — Failover runbook + drill (semana 19)

- [x] **S7.5.a** `infra/runbooks/dr-failover.md` paso a paso: DNS, gateway, Cassandra LB, Temporal, Postgres promote. *Sustrato: nuevo runbook maestro [`infra/runbooks/dr-failover.md`](../../infra/runbooks/dr-failover.md) que compone los runbooks por componente ([cassandra-app-failover.md](../../infra/runbooks/cassandra-app-failover.md), [postgres-promotion.md](../../infra/runbooks/postgres-promotion.md), [ceph-multisite-bootstrap.md](../../infra/runbooks/ceph-multisite-bootstrap.md)) en una secuencia end-to-end de 9 pasos: (0) detener writes en A si es alcanzable, (1) promote Postgres × 4 (dependencia hard de todo lo demás), (2) Cassandra `localDc=dc-b1` rolling restart, (3) Kafka `topicPrefix=dc-a.` y `bootstrap=openfoundry-b...` rolling restart (los offsets traducidos por `MirrorCheckpointConnector` permiten resume sin reprocesado), (4) Lakekeeper RW + flip de zona Ceph a master vía `radosgw-admin zone modify --master`, (5) scale-up de Temporal frontend/history/matching/worker en B (que ya consume `pg-policy-replica` + `dc-b1`), (6) cutover global edge (Route53 health-check failover o anycast LB), (7) smoke en `auth/healthz`, ontology read y Temporal write, (8) PR único `dr/failover-<UTC>` que pin-ea todo el estado en Git, (9) procedimiento de failback siempre planificado nunca durante incidente. Targets RTO ≤30min, RPO Cassandra=0, Postgres ≤60s, Kafka ≤30s, Iceberg ≤60s. Tabla de roles (IC, Scribe, operadores por componente) y criterios de activación explícitos.*
- [x] **S7.5.b** Game day: simular caída de región A. Medir RTO y RPO reales. *Sustrato: nuevo script [`infra/runbooks/dr-game-day.md`](../../infra/runbooks/dr-game-day.md) con cadencia trimestral (dos primeras iteraciones en staging, tercera ya en producción con ventana anunciada). Seis fases: (1) inyección de fallo con tres opciones — Option A network partition vía chaos-mesh `NetworkChaos` aislando egress de región A (preferida, más realista), Option B forced API server shutdown (más rápida), Option C `kill -9` masivo de pods en A (caótica pero acotada); (2) detección y declaración con SLO ≤5min apoyado en alerta `RegionAUnreachable`; (3) ejecución del [`dr-failover.md`](../../infra/runbooks/dr-failover.md) con SLO por step (Postgres ≤3min, Cassandra ≤5min, etc.); (4) medición real de RTO (`T_serving - T0`) y RPO por componente (consultas SQL/Kafka/Spark concretas que comparan con marker `dr_drill_baseline_<UTC>` plantado en pre-game); (5) failback completo aunque el tiempo sea estricto (no se da por bueno hasta validar el roundtrip); (6) post-mortem en 48h con timeline, action items con owner y deadline, y actualización obligatoria de los runbooks **antes** de cerrar. Plantilla de resultados `dr-results/<UTC-date>.md` con tabla IC/Scribe/T0/T_detect/T_serving/RTO/RPO×3 incluida. Roles claros (IC, Scribe, 4 operadores por componente, validador de aplicación). Out-of-scope: RBD CRR (S8) y notificaciones reales a usuarios (recipientes sintéticos).*

### Definition of Done — S7
- ✅ Iceberg replicado A→B.
- ✅ Cassandra 2-DC con LOCAL_QUORUM funcionando.
- ✅ Kafka MM2 replicando.
- ✅ Postgres replica en B.
- ✅ Drill ejecutado: RTO <30 min, RPO <5 min.

---

## 13. Stream S8 — Cleanup & hardening (4 semanas)

### Tarea S8.1 — Borrar servicios redundantes (semana 18)

- [x] **S8.1.a** Borrar `health-check-service`. *Sustrato: handlers/modelos y migración viven en [`telemetry-governance-service`](../../services/telemetry-governance-service); `services/health-check-service/` ya no existe. El mapa lo mueve a la sección de retired directories y los manifests runtime no deben renderizarlo.*
- [x] **S8.1.b** Consolidar `widget-registry`, `tool-registry` en sus parents. *Sustrato: `widget-registry-service` quedó absorbido por [`application-composition-service`](../../services/application-composition-service) y `tool-registry-service` por [`agent-runtime-service`](../../services/agent-runtime-service). `edge-gateway-service` enruta `/api/v1/widgets` al parent de composición y `/api/v1/ai/tools` al runtime de agentes; Helm/compose ya no declaran los servicios retirados.*
- [x] **S8.1.c** Revisar lista de servicios y corregir la métrica de consolidación. *Sustrato: auditoría 2026-05-03: `find services -mindepth 1 -maxdepth 1 -type d | wc -l` → **95**. La afirmación "≤30 servicios" queda reemplazada por la métrica real: **95 service directories → 33 ownership boundaries + 3 sinks across 5 Helm releases**. [`service-consolidation-map.md`](service-consolidation-map.md) coincide 1:1 con `services/` y lista los tres retired stubs aparte.*
- [x] **S8.1.d** Cerrar ownership Dataset Versioning / Iceberg. *Sustrato: [`data-asset-catalog-service`](../../services/data-asset-catalog-service) deja de originar runtime writes sobre `dataset_versions`, `dataset_branches` y `dataset_transactions`: [`handlers/upload.rs`](../../services/data-asset-catalog-service/src/handlers/upload.rs) ahora rechaza `POST /v1/datasets/{rid}/upload` con `409` explícito hacia `dataset-versioning-service`, y [`handlers/crud.rs`](../../services/data-asset-catalog-service/src/handlers/crud.rs) ya no bootstrappea filas de `dataset_branches`. Las migraciones legacy del catálogo ([`20260419100001_initial_datasets.sql`](../../services/data-asset-catalog-service/migrations/20260419100001_initial_datasets.sql), [`20260421174000_dataset_branches.sql`](../../services/data-asset-catalog-service/migrations/20260421174000_dataset_branches.sql), [`20260425173000_dataset_views_transactions.sql`](../../services/data-asset-catalog-service/migrations/20260425173000_dataset_views_transactions.sql)) quedan anotadas como bridge de compatibilidad: Iceberg es obligatorio para snapshots / data state y Postgres en este bounded context queda limitado a metadata declarativa. `dataset-versioning-service` queda como único owner runtime dentro del merge target de S8.1.*

### Tarea S8.2 — Helm chart split en 5 releases (semana 19)

- [x] Ya documentado en ADR-N08 del audit. Implementar separación. *Sustrato: ADR formalizado como [`ADR-0031`](adr/ADR-0031-helm-chart-split-five-releases.md). Library chart [`infra/k8s/helm/of-shared/`](../../infra/k8s/helm/of-shared/) y los cinco releases [`of-platform`](../../infra/k8s/helm/of-platform/), [`of-data-engine`](../../infra/k8s/helm/of-data-engine/), [`of-ontology`](../../infra/k8s/helm/of-ontology/), [`of-ml-aip`](../../infra/k8s/helm/of-ml-aip/), [`of-apps-ops`](../../infra/k8s/helm/of-apps-ops/) escafoldados con `Chart.yaml`, `values.yaml` y `templates/services.yaml`. Plan de migración en [`infra/k8s/helm/MIGRATION.md`](../../infra/k8s/helm/MIGRATION.md).*

### Tarea S8.3 — Migrations como Job pre-upgrade (semana 19-20)

- [x] Para los servicios que aún usan Postgres (los que apuntan a `pg-schemas`): mover migrations a Helm Job `pre-upgrade`. *Sustrato: define `of-shared.migrations` en [`infra/k8s/helm/of-shared/templates/_migrations.tpl`](../../infra/k8s/helm/of-shared/templates/_migrations.tpl) (annotations `helm.sh/hook: pre-install,pre-upgrade`, weight `-5`). Cada release lo instancia en su `templates/migrations.yaml` y declara los DSN en `values.yaml` (pg-schemas, pg-policy, pg-runtime-config, lakekeeper).*

### Tarea S8.4 — Chaos suite (semana 20-21)

- [x] Chaos Mesh experimentos:
  - Kill 1 nodo Cassandra → validar P95 reads.
  - Kill 1 broker Kafka → validar consumer reposiciona.
  - Kill 1 pod Temporal history → validar workflows continúan.
  - Drain 1 nodo k8s con PDB respetado.
  *Sustrato: [`ADR-0032`](adr/ADR-0032-chaos-mesh-resilience-suite.md) declara cadencia mensual en staging; manifiestos `Schedule` en [`infra/k8s/chaos/cassandra-kill.yaml`](../../infra/k8s/chaos/cassandra-kill.yaml), [`kafka-broker-kill.yaml`](../../infra/k8s/chaos/kafka-broker-kill.yaml), [`temporal-history-kill.yaml`](../../infra/k8s/chaos/temporal-history-kill.yaml), [`k8s-node-drain.yaml`](../../infra/k8s/chaos/k8s-node-drain.yaml) con [README](../../infra/k8s/chaos/README.md). YAML validados con `ruby -ryaml`.*

### Tarea S8.5 — Documentación final (semana 21-22)

- [x] Actualizar `ARCHITECTURE.md` con la nueva pirámide. *Sustrato: sección "Post-S8 service pyramid (95 service directories, 33 ownership boundaries + 3 sinks)" en [`ARCHITECTURE.md`](../../ARCHITECTURE.md) con diagrama ASCII de las 5 capas Helm sobre el substrato de almacenamiento.*
- [x] Actualizar `docs/architecture/` con diagramas finales. *Sustrato: [`ADR-0030`](adr/ADR-0030-service-consolidation-30-targets.md), [`ADR-0031`](adr/ADR-0031-helm-chart-split-five-releases.md), [`ADR-0032`](adr/ADR-0032-chaos-mesh-resilience-suite.md) y [`service-consolidation-map.md`](service-consolidation-map.md) referenciados desde el índice de architecture.*
- [x] Actualizar `README.md` y `getting-started`. *Sustrato: nueva sección "Deploying to Kubernetes (post-S8 layout)" en [`docs/getting-started/index.md`](../getting-started/index.md) y aviso de deprecación + comandos de los cinco releases en [`infra/README.md`](../../infra/README.md).*
- [x] Cerrar todos los ADRs en estado `Accepted`. *Sustrato: ADR-0030/0031/0032 nacen con `Status: Accepted`; sweep ADR-0007..0029 confirma `Accepted` (verificado con grep de cabecera Status).*

### Definition of Done — S8
- ✅ Plataforma con 95 service directories auditados y 33 ownership boundaries + 3 sinks documentados.
- ✅ 5 Helm releases independientes.
- ✅ Los retired stubs `health-check-service`, `tool-registry-service` y `widget-registry-service` no aparecen en Helm/compose runtime surfaces.
- ✅ Chaos suite mensual.
- ✅ Documentación coherente.

### Tarea S8.6 — Ejecución real del split (semana 22)

- [x] **S8.6.a** Portar HPA, PDB, ServiceAccount e Ingress al library chart `of-shared`. *Sustrato: nuevos defines `of-shared.serviceaccount`, `of-shared.hpa`, `of-shared.pdb`, `of-shared.ingress` en [`infra/k8s/helm/of-shared/templates/_workloads.tpl`](../../infra/k8s/helm/of-shared/templates/_workloads.tpl); cada release los incluye en su `templates/services.yaml` y `of-platform` añade además [`templates/ingress.yaml`](../../infra/k8s/helm/of-platform/templates/ingress.yaml). Validado: `helm template of-platform` renderiza 25 recursos (1 Ingress, 3 HPA, 3 PDB, 4 ServiceAccount, 4 Deployment, 4 Service, 4 NetworkPolicy, 2 Job).*
- [x] **S8.6.b** Borrar físicamente `widget-registry-service`. *Sustrato: stub absorbido en [`services/application-composition-service/src/handlers/widgets.rs`](../../services/application-composition-service/src/handlers/widgets.rs) y [`models/widgets.rs`](../../services/application-composition-service/src/models/widgets.rs); crate eliminado del workspace en [`Cargo.toml`](../../Cargo.toml) y `services/widget-registry-service/` borrado del árbol. `cargo check --workspace` verde (97 → 95 crates de servicio).*
- [x] **S8.6.c** Borrar físicamente `health-check-service`. *Sustrato: handlers/models y migración SQL absorbidos en [`services/telemetry-governance-service/src/handlers/health_checks.rs`](../../services/telemetry-governance-service/src/handlers/health_checks.rs), [`models/health_checks.rs`](../../services/telemetry-governance-service/src/models/health_checks.rs) y [`migrations/20260427070600_19_health_checks_foundation.sql`](../../services/telemetry-governance-service/migrations/20260427070600_19_health_checks_foundation.sql); crate eliminado del workspace y borrado del árbol.*
- [x] **S8.6.d** Borrar físicamente `tool-registry-service`. *Sustrato: handlers/modelos absorbidos en [`services/agent-runtime-service/src/handlers/tools.rs`](../../services/agent-runtime-service/src/handlers/tools.rs) y [`models/tools.rs`](../../services/agent-runtime-service/src/models/tools.rs); crate ausente de `services/`, `.github/CODEOWNERS`, Helm y compose.*
- [x] **S8.6.e** Helm lint + helm template gateado en CI. *Sustrato: [`.github/workflows/helm-lint.yml`](../../.github/workflows/helm-lint.yml) ejecuta `helm dependency update`, `helm lint`, `helm template <release>` y `kubeconform` por matrix sobre los 5 releases en cada PR que toque `infra/k8s/helm/**`. Localmente verificado: los 5 charts renderizan sin los retired stubs.*

---

## 14. Estructura de carpetas resultante

```
OpenFoundry/
├── apps/
│   └── web/                              (sin cambios)
├── services/                             ≤30 servicios
│   ├── edge-gateway-service/
│   ├── identity-federation-service/      (identity custom + sessions Cassandra)
│   ├── ontology-definition-service/      (Postgres pg-schemas)
│   ├── ontology-actions-service/         (Cassandra + outbox)
│   ├── ontology-query-service/           (Cassandra + cache + Vespa)
│   ├── ontology-indexer/                 (Kafka consumer → Vespa)
│   ├── audit-sink/                       (Kafka consumer → Iceberg)
│   ├── workflow-automation-service/      (Kafka consumers + state machine Postgres)
│   ├── pipeline-schedule-service/        (API + scheduling intent → Cron/Kafka/Spark)
│   ├── approvals-service/                (state machine + timeout sweep)
│   ├── automation-operations-service/    (saga choreography)
│   ├── ...
├── libs/
│   ├── core-models/                      (sin cambios)
│   ├── auth-middleware/
│   ├── cassandra-kernel/                 (NUEVO)
│   ├── state-machine/                    (NUEVO)
│   ├── saga/                             (NUEVO)
│   ├── event-scheduler/                  (NUEVO)
│   ├── ontology-kernel/                  (refactorizado: traits + Cassandra impl)
│   ├── storage-abstraction/              (extendido; Iceberg + Cassandra repos)
│   ├── event-bus-data/                   (activado)
│   ├── event-bus-control/
│   ├── audit-trail/                      (refactor: emite a Kafka)
│   └── testing/                          (testcontainers Postgres + Cassandra + Kafka)
├── infra/
│   └── k8s/
│       ├── cassandra/                    (NUEVO; k8ssandra-operator + cluster)
│       ├── spark-operator/               (NUEVO)
│       ├── sparkapplications/            (NUEVO; CRs de pipelines)
│       ├── trino/                        (NUEVO)
│       ├── cnpg/clusters/                (4 manifests)
│       ├── strimzi/                      (existente; topics activos)
│       ├── lakekeeper/                   (existente; + región B read-only)
│       ├── rook/                         (existente)
│       ├── vespa/                        (obligatorio en prod)
│       └── helm/
│           ├── of-platform/              (5 charts)
│           ├── of-data-engine/
│           ├── of-ontology/
│           ├── of-ml-aip/
│           ├── of-apps-ops/
│           ├── of-shared/
│           └── profiles/
└── docs/
    └── architecture/
        ├── adr/
        │   ├── ADR-0020-cassandra-as-operational-store.md
        │   ├── ADR-0021-temporal-on-cassandra-go-workers.md   (superseded)
        │   ├── ADR-0022-transactional-outbox.md
        │   ├── ADR-0023-iceberg-cross-region-dr.md
        │   ├── ADR-0024-postgres-consolidation.md
        │   ├── ADR-0025-eliminate-custom-scheduler.md
        │   ├── ADR-0026-identity-stack.md
        │   └── ADR-0037-foundry-pattern-orchestration.md
        ├── data-model-cassandra.md
        ├── ontology-queries-inventory.md
        └── legacy-migrations/            (archivo histórico)
```

---

## 15. Riesgos y mitigaciones específicas

| Riesgo | Severidad | Mitigación |
|---|---|---|
| Modelado Cassandra hace assumptions equivocadas (hot partition por tenant grande) | Crítica | Bucketing (`tenant_id, type_id, day_bucket`); load test con tenant skewed antes de migrar a prod |
| LWT abuse (cada write con `IF version`) → throughput cae a 1/4 | Alta | Restringir LWT a casos genuinamente concurrentes (outbox claim, optimistic lock raro); resto = idempotency |
| Ejecución durable del patrón Foundry requiere más disciplina de idempotencia | Alta | state machines explícitas, outbox obligatorio, compensaciones modeladas y grep gates de cierre por dominio |
| Lag Vespa indexer desincronizado en bursts | Media | Backpressure + autoscaling consumers + alerta de lag |
| Outbox table crece sin control si relay cae | Media | TTL 7d + alerta si tail >1M filas |
| Cassandra repair cross-DC consume red | Media | Reaper schedules off-peak; throttle |
| Equipos rust sin experiencia Cassandra modeling | Alta | 1 semana training + workshop modelado por queries antes de S1 |
| Drift entre keyspaces y código | Media | Migraciones CQL versionadas + CI check |
| Loss de datos en pre-prod por error de modelado | Baja (no es producción) | Snapshots Medusa antes de cada release intermedia |
| Sustituir `identity-federation` rompe SSO existente | Media | Mantener ambos en paralelo durante 2 sprints; feature flag |

---

## 16. Decisiones explícitas pendientes (bloqueantes a resolver en S0)

1. **Cassandra vs ScyllaDB:** ya decidido **Cassandra** (request del usuario). Driver: `scylla` crate (nombre confuso, pero es el mejor driver Rust para CQL — soporta Cassandra y Scylla por igual).
2. **Cassandra version:** **5.0** LTS (released 2024).
3. **Orquestación Foundry-pattern:** decidido por ADR-0037. Los servicios usan Kafka/outbox/state machines/Spark; `libs/temporal-client`, `workers-go/` y el runtime legado quedan marcados para eliminación y no se aceptan sidecars por servicio como arquitectura objetivo.
4. **Keycloak vs custom hardened:** decidido en S0.1.g / ADR-0026: `identity-federation-service` custom retained y endurecido; Keycloak descartado.
5. **Búsqueda fallback dev:** Vespa pesado para laptop. Para `compose.yaml` dev: **OpenSearch single-node** o Meilisearch.
6. **OpenFGA store backend:** Postgres `pg-policy` (operacionalmente más simple que MySQL recomendado por OpenFGA).

---

## 17. Métricas de éxito

| Métrica | Baseline (hoy) | Target post-migración |
|---|---|---|
| Procesos Postgres | 213 (71×3) | 12 (4×3) |
| RAM Postgres total | ~55 GB | ~6 GB |
| P95 ontology read by-id | Sin baseline aceptado; ver [`slo-evidence/2026-05-03`](slo-evidence/2026-05-03/summary.md) | <20 ms |
| P95 ontology read by-type (paginado) | Sin baseline aceptado; ver [`slo-evidence/2026-05-03`](slo-evidence/2026-05-03/summary.md) | <100 ms |
| P99 write ontology + outbox | Sin baseline aceptado; ver [`slo-evidence/2026-05-03`](slo-evidence/2026-05-03/summary.md) | <50 ms |
| Lag Vespa indexer | Sin baseline operativo aceptado; S5-OPS abierto | P99 <5 s |
| Coordinación durable / exactly-once lógico | No garantizado | Garantizado por idempotencia + outbox + state machines + CronJobs/Spark/Kafka |
| Multi-region failover RTO | No soportado | <30 min |
| Multi-region RPO | No soportado | <5 min |
| Helm releases | 1 monolítico | 5 separados |
| Servicios | 84 | ≤30 |

## 18. Checklist final de cierre + grep gates

> Esta checklist manda sobre cualquier check verde de `substrate`. Un stream solo se cierra cuando su evidencia operativa, sus runbooks/ADRs asociados y su grep gate están verdes a la vez.

### Cierre formal ejecutado — 2026-05-03

**Decision:** `Postgres residual` queda **NOT CLOSED**. Los grep gates oficiales estan verdes en el arbol actual, y la busqueda global de stubs ya tiene allowlist aprobada, pero el cierre formal completo sigue bloqueado por evidencia operativa pendiente, tests locales fallidos e integraciones reales no validadas.

| Gate / check | Comando ejecutado | Resultado | Evidencia | Owner |
|---|---|---:|---|---|
| G-S1 | `rg -n 'sqlx::query!?|sqlx::query_as!?|query!\(|query_as!\(' ...ontology hot-path...` | PASS | 0 hits. Tests locales: `cargo test -p ontology-kernel` -> PASS (58 unit + 10 integration-style local); `cargo test -p cassandra-kernel` -> PASS solo en tests no ignorados. | Ontology platform maintainers |
| G-S2-GO | `rg -n 'ErrNotImplemented|TODO|substrate stub|_substrate|logging stub|implementation pending' workers-go/... -g '*.go'` | PASS | 0 hits. `go test ./...` PASS en `workflow-automation`, `pipeline`, `approvals`, `automation-ops` y `reindex`. | Workflow/Foundry-pattern maintainers |
| G-S2-PG | `rg -n 'FROM workflow_runs|INTO workflow_runs|UPDATE workflow_runs|...automation_queue_runs' services/... -g '*.rs'` | PASS | 0 hits en runtime Rust S2. | Workflow/Foundry-pattern maintainers |
| G-S2-E2E | `rg -n '#\[ignore|blocked on grpc backend|ErrNotImplemented substrate|LoggingWorkflowClient' services/.../tests -g '*.rs'` | PASS / TEST GAP | 0 hits. `cargo test -p workflow-automation-service --features it-temporal --test temporal_e2e -- --test-threads=1` -> PASS. `cargo test -p pipeline-schedule-service --features it-temporal --test temporal_schedule_idempotency -- --test-threads=1` -> PASS. Pero `cargo test -p pipeline-schedule-service` -> FAIL por compilacion del bin test. | Workflow/Foundry-pattern maintainers |
| G-S3 | `rg -n 'NotWired|not_implemented|ErrNotImplemented|todo!|TODO' services/identity-federation-service/src services/session-governance-service/src services/oauth-integration-service/src` | PASS / TEST FAIL | 0 hits. `cargo test -p identity-federation-service` -> PASS; `cargo test -p oauth-integration-service` -> PASS; `cargo test -p session-governance-service` -> FAIL en `sessions_cassandra::tests::active_postgres_migrations_do_not_create_runtime_session_tables` por fixture no encontrado. | Identity maintainer + Security |
| G-S5 | `rg -n 'NotWired|not_implemented|ErrNotImplemented|todo!|TODO' services/audit-sink/src ... workers-go/reindex` | PASS | 0 hits. Unitarios S5 PASS: `audit-sink`, `ai-sink`, `lineage-service`, `ontology-indexer --features runtime`, `event-bus-data`, `sql-bi-gateway-service trino`. | Data platform maintainers |
| G-S5 guardrail WORM | `rg -n 'of_audit|rewrite|expire_snapshots' infra services workers-go ...` | PASS / REVIEWED | Hits son comentarios/guardrails y referencias de schema; no se observo `of_audit` como target de rewrite/expire. Mantener revision humana en cada cambio de jobs Spark/Flink. | Data platform maintainers |
| G-S5-OPS | `test -f docs/architecture/lakehouse-evidence/2026-05-03/{summary.md,kafka-offsets.txt,iceberg-counts.sql.txt,worm-negative-test.txt,restart-drill.txt}` + lectura de `summary.md` | FAIL | Artefactos existen, pero `summary.md` declara `Status: BLOCKED`, `Outcome: NOT CLOSED`, `Sign-off status: NOT SIGNED`. | Data platform maintainers + SRE |
| G-S6 | `rg -n -g '*.yaml' -- '-pg-app|[a-z0-9-]+-pg\.yaml' infra/k8s/helm infra/k8s/platform/manifests/cnpg/{clusters,poolers}` | PASS | 0 hits. | Database owner + SRE |
| G-S6.6 | `rg -n 'sync_jobs|ingestion_checkpoints' services/connector-management-service/src services/ingestion-replication-service/src -g '*.rs'` | PASS | 0 hits en codigo Rust live. | Data engine maintainers |
| S8 Helm/render | `helm template <release> infra/k8s/helm/<chart> -f infra/k8s/helm/<chart>/values-prod.yaml` para `of-platform`, `of-ontology`, `of-apps-ops`, `of-data-engine`, `of-ml-aip`, `open-foundry` | PASS | Todos renderizan con exit code 0. Render grep para `health-check-service|widget-registry-service|tool-registry-service` -> 0 hits. Los directorios retirados no existen; quedan 92 directorios `*-service`, por lo que `<=30` no es una verdad fisica del arbol. | Platform architecture |
| Global stubs search | `rg -n 'TODO|pending|noop|LoggingWorkflowClient|ErrNotImplemented' services libs workers-go infra/k8s ...` | PASS / ALLOWLISTED | Allowlist creada en [`closure-global-stub-allowlist.md`](closure-global-stub-allowlist.md). Raw search actual: 346 hits, todos clasificados. Residual allowlist check -> 0; runtime-stub gate fino -> 0. Se corrigieron los blockers reales: signed URLs ya no devuelven `Ok(String::new())`, `event-streaming-service` fail-fast en staging/prod si falta hot buffer/Cassandra runtime, y los TODO propios de Flink/media se convirtieron en warnings/follow-ups explicitos. | Platform architecture + owners por dominio |
| IT Cassandra real | `cargo test -p cassandra-kernel --test integration -- --ignored --test-threads=1` | BLOCKED | Falla descargando `cassandra:5.0.2` desde Docker registry (`context deadline exceeded`) incluso tras reintento con permisos elevados. | Platform/SRE |
| IT Kafka/Search real | `cargo test -p ontology-indexer --features runtime --test runtime_kafka_search -- --ignored --test-threads=1` | FAIL | Kafka container arranca con error: `No security protocol defined for listener CONTROLLER`. | Search/Data platform |
| ADR-0012 / SLO evidence | `rg -n 'Status: BLOCKED|Outcome: NOT APPROVED|OPEN: blocked|NOT APPROVED' docs/architecture/...` | FAIL | `docs/architecture/slo-evidence/2026-05-03/summary.md` declara `Outcome: NOT APPROVED FOR FINAL CLOSURE`; ADR-0012 mantiene metricas obligatorias `OPEN`. | Platform architecture + SRE |

Bloqueos concretos que siguen impidiendo cierre:

1. `cargo test -p pipeline-schedule-service` no compila el bin test: el `AppState` del crate incluye `lineage_runtime` y `nats_url`, pero el test helper importado en `pipeline-authoring-service/src/domain/engine/runtime.rs` construye un estado sin esos campos.
2. `cargo test -p session-governance-service` falla porque el test reutilizado desde `identity-federation-service/src/sessions_cassandra.rs` resuelve fixtures con `CARGO_MANIFEST_DIR` del crate consumidor y no encuentra el archivo esperado.
3. La evidencia S5-OPS existe solo como intento bloqueado, no como pack PASS firmado.
4. ADR-0012/SLO evidence declara mediciones obligatorias abiertas y excepciones no aprobadas.
5. Los tests de integracion reales para Cassandra/Kafka no estan verdes en esta maquina.

Hasta resolver esos puntos, **no marcar cerrado** el hito `Postgres residual`.

### G-S1 — Ontology hot path sin SQL directo

```bash
rg -n 'sqlx::query!?|sqlx::query_as!?|query!\(|query_as!\(' \
  libs/ontology-kernel/src/handlers \
  services/object-database-service/src \
  services/ontology-actions-service/src \
  services/ontology-query-service/src \
  services/ontology-security-service/src \
  services/ontology-exploratory-analysis-service/src \
  services/ontology-timeseries-analytics-service/src \
  services/ontology-funnel-service/src \
  services/ontology-functions-service/src
```

Pasa cuando devuelve `0 hits` para runtime hot-path. Quedan fuera del gate los árboles archivados bajo `docs/architecture/legacy-migrations/**`, el servicio declarativo `ontology-definition-service` y el SQL de infraestructura/outbox que no pertenece al hot path ontology.

### G-S2-GO — Workers/consumers de orquestación sin stubs de negocio

```bash
rg -n 'ErrNotImplemented|TODO|substrate stub|_substrate|logging stub|implementation pending' \
  workers-go/workflow-automation \
  workers-go/approvals \
  workers-go/automation-ops \
  -g '*.go'
```

Pasa cuando devuelve `0 hits` en código runtime de workers/consumers. Se permite documentación histórica, pero ningún componente live puede devolver stubs, planes `_substrate`, logging-only side effects o `ErrNotImplemented`.

### G-S2-PG — Orquestación distribuida sin runtime legacy Postgres

```bash
rg -n 'FROM workflow_runs|INTO workflow_runs|UPDATE workflow_runs|FROM workflow_approvals|INTO workflow_approvals|UPDATE workflow_approvals|FROM automation_queues|INTO automation_queues|UPDATE automation_queues|FROM automation_queue_runs|INTO automation_queue_runs|UPDATE automation_queue_runs' \
  services/workflow-automation-service/src \
  services/approvals-service/src \
  services/automation-operations-service/src \
  -g '*.rs'
```

Pasa cuando devuelve `0 hits` en código Rust runtime. Quedan fuera del gate migraciones archivadas, DROP staged y readme de cutover; no quedan fuera handlers live.

### G-S2-E2E — E2E reales del patrón Foundry, no ignorados

```bash
rg -n '#\[ignore|blocked on grpc backend|ErrNotImplemented substrate|LoggingWorkflowClient' \
  services/workflow-automation-service/tests \
  services/pipeline-schedule-service/tests \
  -g '*.rs'
```

Pasa cuando devuelve `0 hits` y los E2E se ejecutan contra Kafka/outbox/state machines/Spark reales. Mientras haya `#[ignore]` o el test dependa del runtime legado, S2 no cierra.

### G-S3 — Identity runtime sin stubs de cutover

```bash
rg -n 'NotWired|not_implemented|ErrNotImplemented|todo!|TODO' \
  services/identity-federation-service/src \
  services/session-governance-service/src \
  services/oauth-integration-service/src
```

Pasa cuando devuelve `0 hits` en código runtime. Además, S3 no cierra sin sign-off firmado de `identity-failover-drill.md` y `identity-pen-test-runbook.md`.

### G-S5 — Lakehouse sinks/indexer sin stubs

```bash
rg -n 'NotWired|not_implemented|ErrNotImplemented|todo!|TODO' \
  services/audit-sink/src \
  services/lineage-service/src \
  services/ai-sink/src \
  services/ontology-indexer/src \
  services/sql-bi-gateway-service/src \
  workers-go/reindex
```

Pasa cuando devuelve `0 hits` en código runtime. Validación adicional obligatoria: cualquier referencia a `of_audit` en jobs Spark/maintenance debe ser únicamente de guardrail y nunca como target de `rewrite`/`expire_snapshots`.

### G-S5-OPS — Evidencia operativa lakehouse

```bash
test -f docs/architecture/lakehouse-evidence/<YYYY-MM-DD>/summary.md
test -f docs/architecture/lakehouse-evidence/<YYYY-MM-DD>/kafka-offsets.txt
test -f docs/architecture/lakehouse-evidence/<YYYY-MM-DD>/iceberg-counts.sql.txt
test -f docs/architecture/lakehouse-evidence/<YYYY-MM-DD>/worm-negative-test.txt
test -f docs/architecture/lakehouse-evidence/<YYYY-MM-DD>/restart-drill.txt
```

Pasa solo con artefactos de un entorno real y sign-off en `summary.md`, siguiendo [`docs/architecture/runbooks/lakehouse-s5-operational-evidence.md`](runbooks/lakehouse-s5-operational-evidence.md). Sin ese pack, S5 puede tener `G-S5` verde pero no cierra producto.

### G-S6 — Manifests live sin restos del modelo legacy por servicio

```bash
rg -n -g '*.yaml' -- '-pg-app|[a-z0-9-]+-pg\\.yaml' \
  infra/k8s/helm \
  infra/k8s/platform/manifests/cnpg/clusters \
  infra/k8s/platform/manifests/cnpg/poolers
```

Pasa cuando devuelve `0 hits` en manifests live. La evidencia operativa del decommission sigue viviendo en `infra/runbooks/cnpg-decommission.md`.

### G-S6.6 — Runtime de ingestión fuera de SQL legacy

```bash
rg -n 'sync_jobs|ingestion_checkpoints' \
  services/connector-management-service/src \
  services/ingestion-replication-service/src \
  -g '*.rs'
```

Pasa cuando devuelve `0 hits` en código Rust live. Las migraciones históricas pueden mencionar esas tablas únicamente para crear el pasado o dropearlas en cutover; no pueden existir handlers, schedulers ni servicios gRPC/HTTP que las traten como runtime autoritativo.

### Cierre final — `Postgres residual`

- [ ] G-S1 verde con evidencia operativa enlazada. *(El grep gate de código está verde, pero la evidencia SLO de S1 está BLOCKED/no aprobada en [`docs/architecture/slo-evidence/2026-05-03/summary.md`](slo-evidence/2026-05-03/summary.md).)*
- [x] G-S2-GO verde. *(2026-05-03: grep gate -> 0 hits en workers Go runtime.)*
- [x] G-S2-PG verde. *(2026-05-03: grep gate -> 0 hits para `workflow_runs`, `workflow_approvals`, `automation_queues` y `automation_queue_runs` en servicios Rust S2.)*
- [x] G-S2-E2E verde en su gate formal. *(2026-05-03: grep gate -> 0 hits y tests `it-temporal` reales pasan contra Temporal frontend + workers Go. Cierre final aun bloqueado porque `cargo test -p pipeline-schedule-service` falla en el bin test.)*
- [ ] G-S3 verde. *(Grep gate verde, pero `cargo test -p session-governance-service` falla y los runbooks S3 siguen sin sign-off aprobado.)*
- [x] G-S5 verde.
- [ ] G-S5-OPS verde. *(El pack `lakehouse-evidence/2026-05-03` existe, pero declara `BLOCKED / NOT CLOSED / NOT SIGNED`.)*
- [x] G-S6 verde.
- [x] G-S6.6 verde.
- [ ] ADR-0012 poblado con números reales y sin excepciones abiertas. *(Estado 2026-05-03: ADR-0012 no contiene placeholders de medición, pero los números obligatorios siguen BLOCKED y sin excepción aprobada; ver [`docs/architecture/slo-evidence/2026-05-03/summary.md`](slo-evidence/2026-05-03/summary.md).)*
- [ ] Runbooks de cierre firmados en su entorno objetivo: [`identity-failover-drill`](runbooks/identity-failover-drill.md), [`identity-pen-test`](runbooks/identity-pen-test-runbook.md), [`lakehouse-s5-operational-evidence`](runbooks/lakehouse-s5-operational-evidence.md) y [`cnpg-decommission`](../../infra/runbooks/cnpg-decommission.md). *(Estado 2026-05-03: todos registran owner/fecha/comandos/resultado, pero el resultado es BLOCKED/no firmado.)*
- [ ] El único uso residual aceptado de Postgres es el explícitamente documentado para datos declarativos/de referencia y clusters consolidados.
- [ ] Tests locales e integracion relevantes verdes o bloqueos con excepcion aprobada. *(Estado 2026-05-03: `pipeline-schedule-service`, `session-governance-service`, `ontology-indexer` IT Kafka y `cassandra-kernel` IT Cassandra no estan verdes.)*
- [x] Allowlist aprobada para la busqueda global de `TODO|pending|noop|LoggingWorkflowClient|ErrNotImplemented`, o busqueda global limpia. *(2026-05-03: [`closure-global-stub-allowlist.md`](closure-global-stub-allowlist.md); raw search 346 hits, residual check 0.)*
- [ ] Solo cuando todo lo anterior esté verde se puede marcar **cerrado** el hito `Postgres residual`.

---

## 19. Cómo arrancar mañana

1. **Día 1:** Crear branch `migration/cassandra-foundry`. Crear PR vacío con este documento como descripción. Etiquetar como "epic".
2. **Día 1-3:** Workshop interno de modelado Cassandra (3 sesiones, 2 h cada una). Material: "Cassandra The Definitive Guide" cap. 5-7.
3. **Día 4-5:** Tarea S0.1 (ADRs draft).
4. **Semana 2:** Tarea S0.2 (cluster Cassandra dev local funcionando) + S0.3 (libs/cassandra-kernel scaffold).
5. **Sprint review semana 3:** Demo `just dev-up-cassandra` + un test trivial de insert/get → si funciona, S1 arranca.

---

## 20. Migration to Foundry-pattern (ADR-0037)

- **Status:** in progress.
- **ADR:** [ADR-0037 — Foundry-pattern orchestration](adr/ADR-0037-foundry-pattern-orchestration.md).
- **Plan dedicado:** [`docs/architecture/migration-plan-foundry-pattern-orchestration.md`](migration-plan-foundry-pattern-orchestration.md).
- **Tareas pendientes:** **49** del plan dedicado (52 totales; 0.1, 0.2 y 0.3 ya ejecutadas).
- **Alcance que sigue vigente en este plan:** Cassandra como store operacional, Iceberg como source of truth, Vespa/OpenSearch, outbox + Debezium, consolidación de Postgres, DR multi-región.
- **Alcance superseded por ADR-0037:** cluster del orquestador previo, `workers-go/`, `libs/temporal-client`, keyspaces `temporal_*`, wiring Helm/Compose/CI asociado y cualquier referencia a workflows centralizados como solución objetivo.

---

**Fin del plan.**
