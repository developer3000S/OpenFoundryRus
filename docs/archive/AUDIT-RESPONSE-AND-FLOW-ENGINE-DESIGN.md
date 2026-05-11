# Respuesta a la auditoría arquitectónica + diseño de motor de workflows propio (of-flow)

> **ARCHIVED — superseded for the of-flow design by**
> [`docs/architecture/adr/ADR-0043-of-flow-user-facing-workflow-engine.md`](../architecture/adr/ADR-0043-of-flow-user-facing-workflow-engine.md).
> The audit-response matrix in Part I is a point-in-time snapshot kept
> for historical context. Any future revisions amend the ADR, not this
> file.
>
> Fecha: 2026‑05‑08
> Autor: Comité técnico (auditoría) + decisiones de adopción
> Branch: `main`
> Documentos relacionados: `ARCHITECTURE.md`, `docs/archive/STUB-AUDIT.md`, `docs/archive/MIGRATION-LOOP-STATUS.md`, `docs/architecture/adr/ADR-0037-foundry-pattern-orchestration.md`

Este documento responde a dos preguntas concretas que se plantearon tras la auditoría profunda del repositorio:

1. **De los hallazgos de la auditoría, qué adoptar y qué dejar tal y como está, y por qué.** No queremos aplicar mecánicamente todas las recomendaciones, sino hacer una elección de ingeniería razonada, distinguiendo entre lo que es deuda técnica real, lo que es preferencia estilística del auditor, y lo que es una decisión arquitectónica ya tomada con un ADR sólido que conviene mantener.

2. **Diseño de un motor de workflows propio inspirado en Conductor OSS, sin Temporal bajo ningún concepto.** Aquí no se trata de sustituir lo que ADR‑0037 acaba de retirar (Temporal), sino de añadir una capa de orquestación declarativa visible para los usuarios finales — analistas, investigadores, power users — que sí necesitan diseñar y monitorizar workflows como si fuese una primitiva de producto, no como infraestructura interna.

El documento es deliberadamente extenso porque las dos preguntas tienen consecuencias arquitectónicas a 12–24 meses, y porque cualquier recorte aquí se paga a triple en producción.

---

## Índice

- [Parte I — Adopciones de la auditoría](#parte-i--adopciones-de-la-auditoría)
  - [1. Marco de decisión](#1-marco-de-decisión)
  - [2. Adopciones inmediatas (P0)](#2-adopciones-inmediatas-p0)
  - [3. Adopciones antes de producción (P1)](#3-adopciones-antes-de-producción-p1)
  - [4. Adopciones para escalar (P2)](#4-adopciones-para-escalar-p2)
  - [5. Decisiones que mantenemos tal y como están](#5-decisiones-que-mantenemos-tal-y-como-están)
  - [6. Decisiones que matizamos (adoptamos parcialmente)](#6-decisiones-que-matizamos-adoptamos-parcialmente)
  - [7. Tabla resumen de adopción](#7-tabla-resumen-de-adopción)
- [Parte II — Diseño del motor de workflows propio (of-flow)](#parte-ii--diseño-del-motor-de-workflows-propio-of-flow)
  - [8. Por qué un motor propio y no Temporal](#8-por-qué-un-motor-propio-y-no-temporal)
  - [9. Por qué Conductor OSS como inspiración](#9-por-qué-conductor-oss-como-inspiración)
  - [10. Distinción crítica: of-flow vs Foundry-pattern (ADR-0037)](#10-distinción-crítica-of-flow-vs-foundry-pattern-adr-0037)
  - [11. Principios de diseño de of-flow](#11-principios-de-diseño-de-of-flow)
  - [12. Arquitectura de alto nivel](#12-arquitectura-de-alto-nivel)
  - [13. Modelo de datos (PostgreSQL)](#13-modelo-de-datos-postgresql)
  - [14. Lenguaje declarativo de workflows (DSL)](#14-lenguaje-declarativo-de-workflows-dsl)
  - [15. Tipos de tarea soportados](#15-tipos-de-tarea-soportados)
  - [16. Modelo de ejecución y semántica](#16-modelo-de-ejecución-y-semántica)
  - [17. Worker SDKs (Go, Python, TypeScript)](#17-worker-sdks-go-python-typescript)
  - [18. Versionado de definiciones](#18-versionado-de-definiciones)
  - [19. Idempotencia, retries, compensaciones, sagas](#19-idempotencia-retries-compensaciones-sagas)
  - [20. Human-in-the-loop](#20-human-in-the-loop)
  - [21. Observabilidad y auditoría](#21-observabilidad-y-auditoría)
  - [22. Seguridad y políticas (Cedar)](#22-seguridad-y-políticas-cedar)
  - [23. Despliegue en OpenShift](#23-despliegue-en-openshift)
  - [24. Diferencias frente a Conductor OSS](#24-diferencias-frente-a-conductor-oss)
- [Parte III — ADR propuesto y roadmap](#parte-iii--adr-propuesto-y-roadmap)
  - [25. Borrador de ADR-0043: of-flow user-facing workflow engine](#25-borrador-de-adr-0043-of-flow-user-facing-workflow-engine)
  - [26. Roadmap por fases (12 meses)](#26-roadmap-por-fases-12-meses)
  - [27. Riesgos y mitigaciones](#27-riesgos-y-mitigaciones)
  - [28. Checklist final](#28-checklist-final)

---

## Parte I — Adopciones de la auditoría

### 1. Marco de decisión

Antes de adoptar cualquier recomendación hace falta un marco para decidir. El que aplicamos aquí es:

1. **¿Es un riesgo real, demostrable, en producción?** Si la respuesta es sí, se adopta. Ejemplos: secretos en placeholders, Tempo no desplegado mientras el código exporta OTLP, Ceph sin reglas de placement.
2. **¿Es una preferencia del auditor o una decisión ya tomada con un ADR?** Si hay ADR razonado, **no se cambia**. La auditoría es un input, no un mandato. Ejemplos: Vespa vs OpenSearch (ADR‑0007), Trino reintroducido (ADR‑0029), Foundry‑pattern (ADR‑0037), Lakekeeper (ADR‑0008).
3. **¿La adopción cuesta más de lo que ahorra a 12 meses?** Si sí, se pospone o se descarta. Ejemplo: dividir el monorepo Go en múltiples módulos hoy, cuando el equipo es pequeño y la coherencia de tipos cross‑servicio importa más que el aislamiento.
4. **¿Cierra un hueco que el propio repo declara abierto?** Si la respuesta es sí, se prioriza alto. Ejemplos: los 9 stubs P0 listados en `STUB-AUDIT.md`, el SLO S1 abierto en ADR‑0012.
5. **¿Cumple con el objetivo declarado de comportamiento "tipo Foundry/Gotham"?** Foundry no es sólo un data lake. Es ontología, casos, evidencias, permisos por entidad/atributo, time travel, replay, colaboración. Toda recomendación que acerque al sistema a esa forma se prioriza incluso si no es estrictamente necesaria para la primera release.

Con este marco aplicado de forma honesta, la auditoría tiene tres tipos de hallazgo:

- **Deuda técnica real** que hay que cerrar — adoptamos.
- **Preferencias o decisiones de stack que ya están tomadas con ADR** — mantenemos.
- **Recomendaciones cuya adopción depende de SLOs reales** que aún no tenemos medidos — pospuestas hasta tener evidencia.

### 2. Adopciones inmediatas (P0)

Estos son los hallazgos que adoptamos de la auditoría sin matizar. Cada uno tiene una justificación clara y un coste asumible. El criterio P0 significa: **bloqueante para go‑live de cualquier entorno productivo**.

#### 2.1 Secretos vía External Secrets Operator + Vault

**Adopto.** El hallazgo señala que `infra/helm/infra/postgres-clusters/values.yaml:111` contiene literalmente `password: "PLACEHOLDER_ROTATE_VIA_EXTERNAL_SECRETS"` y que el patrón `existingSecret: open-foundry-env` requiere creación manual del secreto en cada entorno. Esto no es opinión — es un anti‑patrón crítico.

**Por qué es no negociable:**

- Cualquier rotación de credenciales es manual, lo que en un cluster de 20 nodos con 40 servicios significa decenas de minutos de downtime planificado o, peor, rotación que nunca ocurre.
- Auditoría de cumplimiento (PCI/HIPAA/ISO27001) exige rotación periódica probada y trazada.
- El servicio `identity-federation-service` ya tiene escrito el adaptador `vault_signer.go` con autenticación por Kubernetes role. **Está construido y no se está usando.** Es un caso de "el código existe, pero no está enchufado".
- ESO + Vault permite rotación atómica con `Reloader` para restart de pods sin perder en‑flight requests.

**Cómo se adopta:**

1. Helm chart `external-secrets/external-secrets` en namespace `external-secrets`.
2. Vault HA (3 réplicas Raft, sealed con `auto‑unseal` por KMS interno o transit secret engine).
3. `ClusterSecretStore` apuntando a Vault con auth Kubernetes.
4. Por cada secreto hoy literal: `ExternalSecret` que sincroniza desde Vault a un Secret K8s consumido por los servicios.
5. Reemplazar el `vault_signer.go` mock por la integración real para JWKS rotation; esto activa el ciclo 90/14 días que ya está implementado en `internal/jwksrotation/`.
6. Sealed‑secrets como segunda línea para secretos de bootstrap (pre‑Vault), aceptando la deuda residual.

Coste estimado: 1‑2 sprints de un ENG senior. Beneficio: cierre de un agujero de cumplimiento estructural.

#### 2.2 Tempo + Loki + Alertmanager routing

**Adopto.** La auditoría detectó que `libs/observability/tracing.go` exporta vía OTLP/gRPC pero **no hay backend Tempo desplegado**. Es decir, las trazas se mandan al vacío. Mimir está desplegado para métricas, pero Loki para logs y Tempo para trazas no. Y aunque las reglas Prometheus están commiteadas (Kafka, Vespa, CNPG, Flink, NATS, Lakekeeper, Mimir), **Alertmanager no tiene routing configurado**, así que las alertas dispararían a un canal vacío.

**Por qué es no negociable:**

- Sin Tempo, no se puede investigar latencia tail. Los SLOs declarados en ADR‑0012 (p99 Vespa < 80 ms, p99 Kafka producer ack < 25 ms, p99 NATS control < 5 ms) son inverificables sin trazas.
- Sin Loki, post‑mortem de incidentes es imposible. Los logs van a stdout y la única forma de leerlos es `kubectl logs` por pod, lo que se pierde cuando el pod se restartea.
- Sin routing de Alertmanager, todo el trabajo de definir alertas en `infra/observability/prometheus-rules/` no llega a ningún humano. Es decoración.
- Hay runbooks (`infra/runbooks/`) bien escritos atados a alertas que no se reciben.

**Cómo se adopta:**

1. Helm chart `grafana/tempo-distributed` con backend Ceph S3 (mismo bucket pattern que Mimir).
2. Helm chart `grafana/loki-distributed` con Ceph S3 y promtail/Alloy como agente DaemonSet.
3. Ruta Alertmanager con árbol mínimo: `severity=page → PagerDuty`, `severity=warning → Slack #alerts-warning`, `team=platform/data/security → canales específicos`. Receivers configurados via `ExternalSecret`.
4. Inyección de W3C `traceparent` y `tracestate` en cabeceras de mensajes Kafka y NATS — es trivial extender el publisher actual; el código ya tiene `trace.SpanContextFromContext(ctx)`.
5. Bridge slog → OTel para que cada log line lleve trace_id y span_id correlacionables.

Coste estimado: 2‑3 sprints. Beneficio: observabilidad realmente operativa.

#### 2.3 Ceph CRUSH map por chassis Synergy

**Adopto.** El hallazgo más alarmante: `infra/helm/infra/ceph-cluster/values.yaml:2` está vacío y delega a defaults de Rook. En un cluster de 20 servidores Dell Synergy organizados en chassis (típicamente 12 blades por chassis), Rook defaults colocan OSDs por hostname, no por chassis. Pérdida de un chassis ⇒ pérdida potencial de varios OSDs simultáneamente, lo que con erasure coding 8+3 está dentro de tolerancia, pero con replicación 3x puede ser fatal si los tres replicas viven en el mismo chassis.

**Por qué es no negociable:**

- 20 nodos en 2‑3 chassis significa que sin reglas de placement explícitas el sistema **no es resiliente a fallo de chassis**.
- Erasure coding 8+3 sin failure‑domain definido a nivel rack/chassis es un EC superficial.
- Synergy tiene labels físicos disponibles vía cMM (Composable Management Module) que se pueden propagar a nodes K8s.

**Cómo se adopta:**

1. Etiquetar nodos con `openfoundry.io/chassis={frame_id}` y `openfoundry.io/rack={rack}` en bootstrap.
2. CRUSH map declarado en values.yaml con regla `step chooseleaf firstn 0 type chassis`.
3. EC 8+3 con `failureDomain: chassis` para pools de datos generales; EC 4+2 con `failureDomain: rack` para pools Iceberg críticos.
4. Mon=5 con `topologySpreadConstraints` por chassis.
5. RGW × 3 con anti‑affinity por chassis.
6. Test de drill: shutdown completo de un chassis y verificar que el sistema sigue write‑available.

Coste estimado: 1 sprint si los labels físicos están claros. Beneficio: el sistema sobrevive a pérdida de chassis sin intervención humana.

#### 2.4 Cierre de los 9 stubs P0/P1

**Adopto.** El propio `STUB-AUDIT.md` lista nueve placeholders productivos cuyo despliegue significaría que rutas de runtime devuelven 501 o respuestas vacías. Son:

- P0: `agent-runtime-service.AskCopilot` y chat completion
- P0: `pipeline-build-service` handlers + dispatch
- P0: `ontology-kernel` action/function execute (501)
- P0: `ontology-actions-service` substrate fallback
- P1: `ontology-indexer` Kafka runtime
- P1: `audit-sink` Iceberg writer
- P1: `ai-sink` Iceberg writer
- P1: `media-transform-runtime-service` catalog NotImplemented
- P1: `ontology-actions-service` media transform passthrough

**Por qué es no negociable:**

- Sin `ontology-indexer` Kafka loop, Vespa nunca se rellena, y por tanto la búsqueda no funciona en producción.
- Sin Iceberg writers en `audit-sink` y `ai-sink`, la auditoría histórica y la trazabilidad de prompts/respuestas IA no persisten más allá de la retención Kafka. **Pérdida estructural de datos con coste regulatorio.**
- Sin pipeline‑build dispatch real, no hay ETL operativo.
- Sin ontology actions execute, las primitivas declarativas del producto son cáscaras.

**Orden de prioridad para cerrarlos:**

1. `ontology-indexer` Kafka loop — habilita búsqueda
2. `audit-sink` Iceberg writer — cierra audit gap
3. `ai-sink` Iceberg writer — cierra IA observability gap
4. `pipeline-build-service` dispatch — habilita pipelines
5. `ontology-kernel` actions/functions — habilita primitivas declarativas
6. `agent-runtime-service` — desbloquea IA de producto
7. `media-transform-runtime-service` — desbloquea media

Coste estimado: 4‑6 sprints distribuidos entre 3‑4 ingenieros según ownership. Beneficio: el sistema deja de ser "demo" y empieza a ser plataforma.

#### 2.5 Mantenimiento Iceberg como CronJobs

**Adopto.** El hallazgo es claro: ADR‑0041 marcó P4 ("compaction worker") como deferred, lo que en la práctica significa que el lakehouse va a degradar inevitablemente. Iceberg sin mantenimiento acumula:

- Pequeños ficheros de commit (Flink CDC commit cada 60s genera muchos ficheros pequeños).
- Snapshots viejos que mantienen ficheros referenciados (ningún GC).
- Manifests sin reescribir que crecen sin límite.
- Orphan files de fallos parciales de jobs Spark.

**Por qué es no negociable:**

- Trino/Spark queries se degradan exponencialmente con número de ficheros.
- Coste S3 (Ceph RGW pool space) crece linealmente con snapshots no expirados.
- Recovery tras un job fallido deja huérfanos que jamás se limpian.

**Cómo se adopta:**

1. CronJob diario por warehouse: `RewriteDataFiles` con `target‑file‑size 512 MB` y `partial‑progress=true`.
2. CronJob semanal: `ExpireSnapshots` mantiene los últimos 7 días + último por hora del último mes.
3. CronJob mensual: `RemoveOrphanFiles` con dry‑run antes de borrado real.
4. CronJob diario: `RewriteManifests` para mantener manifest count razonable.
5. Métricas exportadas a Prometheus: `iceberg_table_file_count`, `iceberg_table_size_bytes`, `iceberg_snapshots_count`, alertas si crecen monotónicamente sin compactar.

Para implementarlo, no hace falta esperar a que `apache/iceberg-go` estabilice su API de write — los jobs de mantenimiento son procedimientos Spark estándar via `CALL system.rewrite_data_files(...)`.

Coste estimado: 1‑2 sprints. Beneficio: el lakehouse se mantiene operacional indefinidamente.

#### 2.6 Cedar enforcement en runtime, no solo en admin

**Adopto.** El hallazgo identifica que `libs/authz-cedar-go` tiene un schema rico (User, Group, Role, Marking, Dataset, MediaSet, IcebergTable, JwksKey, ScimUser…) y políticas escritas, pero **el evaluador solo se llama desde rutas administrativas** (JWKS rotation, SCIM). En las rutas de runtime de `object-database-service`, los markings se almacenan pero no se enforced en el read path. Esto significa que un caller con scope incorrecto puede leer objetos clasificados si la lógica del handler no comprueba explícitamente.

**Por qué es no negociable:**

- Un sistema "tipo Foundry" sin permisos por entidad/atributo/marking enforced en la capa de almacenamiento es un sistema en el que el control de acceso vive en el handler — y por tanto es violable por cualquier nuevo handler que olvide la comprobación.
- ADR‑0027 promete Cedar como motor de políticas, pero la promesa solo se cumple si Cedar evalúa cada read y write a entidades clasificadas.

**Cómo se adopta:**

1. En `object-database-service`, envolver `ObjectStore.Get/List/Write` con un `cedar.Permit(...)` que recibe el principal del contexto y el `obj.AsCedarEntity()`.
2. Política Cedar por defecto: `permit (principal, action, resource) when principal.clearances.containsAll(resource.markings)`.
3. Hot reload de políticas vía NATS subject `authz.policy.changed` (ya implementado).
4. Decisiones auditadas en `audit.authz.v1` (ya implementado).
5. Tests negativos exhaustivos: usuario A no puede leer objeto de usuario B; usuario sin clearance `pii` no puede listar objetos con marking `pii`; tenant cross‑access denegado.
6. Mismo patrón en `lineage-service`, `media-sets-service`, `notification-alerting-service`.

Coste estimado: 2‑3 sprints más tests. Beneficio: el sistema gana defensa en profundidad real.

#### 2.7 Bitemporalidad en la ontología

**Adopto.** El hallazgo es estructural: `Object` solo tiene `UpdatedAtMs` (transaction time). No hay `valid_from` / `valid_to`. La propiedad `time_dependent` en `PropertyType` existe como bandera pero sin semántica de intervalo. Sin bitemporalidad, no hay "as‑of" queries, no hay replay correcto, no hay anomaly detection temporal — pieza clave de Foundry.

**Por qué es no negociable:**

- Un caso de investigación necesita responder "¿qué sabíamos sobre X el 12 de marzo a las 18:00?". Sin transaction time + valid time, eso no se puede responder.
- Replay desde el lakehouse para reconstruir un read model debe poder pivotar por valid_time, no solo por orden de inserción.
- Foundry/Gotham hacen exactly this como característica diferencial.

**Cómo se adopta:**

1. Añadir `valid_from timestamp` y `valid_to timestamp` al `Object`.
2. Schema migration en Cassandra: `ALTER TABLE objects_by_id ADD valid_from timestamp, valid_to timestamp;`.
3. Nueva tabla `objects_by_type_valid_time` con clustering por `valid_from DESC`.
4. Soporte explícito de "current" vs "as-of T" en `ObjectStore.Get(ctx, id, asOf *time.Time)`.
5. Replay: el indexer Kafka loop puede reconstruir Vespa proyectando solo registros con `valid_from <= asOf < valid_to OR valid_to IS NULL`.
6. UI: time slider que reescribe queries con `as_of`.
7. Versionado de propiedades: cambios in‑place crean una nueva versión del Object con el `valid_from` apuntando al instante del cambio y cierran el `valid_to` de la versión anterior.

Coste estimado: 4‑6 sprints — es una pieza estructural. Beneficio: el sistema deja de ser "current state CRUD" y pasa a ser "memoria histórica navegable".

#### 2.8 ArgoCD GitOps real

**Adopto.** Hoy los despliegues son `helmfile apply` manual. No hay drift detection, no hay rollback automático, no hay revisión PR de cambios de infra como código. Helmfile en Git es una buena base, pero le falta el motor.

**Por qué es no negociable:**

- 20 nodos en 5 releases Helm más infra y operadores significa decenas de Application potenciales. Manual no escala.
- Auditoría de cambios de infra (quién, cuándo, qué) se pierde sin un sistema declarativo con git como fuente.
- Recuperación tras desastre: con ArgoCD, "reinstalar todo" es `argocd app sync` por bootstrap; sin él, es un runbook frágil.

**Cómo se adopta:**

1. ArgoCD instalado en namespace `argocd` (ironic: con helmfile primero, luego self‑managed).
2. `AppProject openfoundry` con whitelists de namespace, repositorio y clusters destino.
3. App‑of‑apps: una `Application` raíz que apunta a `infra/argocd/apps/` donde viven Application por release.
4. Sync waves: operators (wave 0) → infra (wave 1) → apps (wave 2).
5. `syncPolicy.automated.prune=true, selfHeal=true` para prod después de validar en staging.
6. Hook PreSync para validaciones (`kubeval`, `polaris`, `kyverno`).
7. Notifications a Slack en sync failures.

Coste estimado: 2 sprints. Beneficio: cierre operacional total del ciclo entrega.

### 3. Adopciones antes de producción (P1)

Estos son hallazgos que adoptamos pero que pueden esperar 1‑3 meses sin riesgo material.

#### 3.1 libs/resilience (timeouts + circuit breakers)

**Adopto.** El hallazgo: clientes HTTP en `identity-federation-service` (Vault, SAML metadata, token revocation) y en `edge-gateway-service` no tienen timeouts uniformes ni circuit breakers. Bajo carga o degradación de un upstream, esto provoca cascada.

**Cómo se adopta:**

```go
package resilience

import (
    "net/http"
    "time"
    "github.com/sony/gobreaker/v2"
)

type Options struct {
    Name           string
    Timeout        time.Duration  // total request budget
    DialTimeout    time.Duration
    Backoff        BackoffPolicy
    MaxConcurrent  int            // bulkhead
    BreakerSettings gobreaker.Settings
}

func NewClient(o Options) *http.Client { /* ... */ }
```

Política por defecto:
- Timeout total 5s salvo override.
- Dial timeout 1s.
- Bulkhead semaphore por 100 in‑flight por cliente.
- Circuit breaker abre tras 5 fallos consecutivos, half‑open tras 10s.
- Retries exponenciales con jitter para 5xx y errores de red, no para 4xx.

Aplicación: refactor de los ~12 puntos donde hoy hay `http.Client{}` desnudo. Coste 1 sprint.

#### 3.2 Apicurio Registry como gate de schemas

**Adopto.** Hoy hay validación local en `libs/event-bus-control/schema_registry.go` (Avro/JSON/Protobuf) más una integración Avro‑only en `ingestion-replication-service`. Apicurio está declarado en values pero no es la fuente de verdad.

**Cómo se adopta:**

1. Apicurio Registry en namespace `of-platform` con backend Postgres (CNPG cluster ya existente).
2. Cada productor Kafka registra el schema antes de publicar; si no está registrado o la versión rompe compatibilidad, falla.
3. CI pipeline corre `apicurio-cli check-compatibility` por cada cambio en `proto/` o schema declarado.
4. ConfigMap por servicio con la URL del registry.
5. Política por topic: domain events `BACKWARD`, audit events `FULL`, CDC `NONE`.

Coste 2 sprints. Beneficio: imposible publicar un evento que rompe consumidores.

#### 3.3 NetworkPolicies completas

**Adopto.** El template default ya hace deny‑by‑default + allow same‑release. Falta allow explícito a infra (Kafka brokers, Cassandra contact points, NATS, Postgres, Vespa, Lakekeeper) y egress controlado a IDPs externos.

**Cómo se adopta:** generar NetworkPolicies por servicio a partir de un manifest declarativo `service.allows: [kafka, cassandra, nats]` que el chart traduce. Coste 1 sprint.

#### 3.4 golang-migrate o atlas para migrations

**Adopto.** Hoy cada servicio tiene su mini‑runner basado en `embed.FS` sin tabla de versión. Riesgo de re‑ejecución, no hay rollback.

**Cómo se adopta:** `golang-migrate` con backend Postgres y tabla `schema_migrations` por base de datos lógica. Mantener el embed.FS pero ejecutarlo a través de la lib, no a mano. Coste 1‑2 sprints distribuidos por servicio.

#### 3.5 SBOM, gosec, trivy, cosign

**Adopto.** Supply chain hoy es ciega. Gosec no en CI, no SBOM, no firmas de imagen, no admisión que verifique firma.

**Cómo se adopta:**

1. `golangci-lint` con `gosec` enabled como check.
2. `trivy fs` y `trivy image` en CI.
3. `syft sbom` exportado como artifact por imagen.
4. `cosign sign` en pipeline release.
5. `cosign verify` en admission controller (Kyverno verifyImages).

Coste 1‑2 sprints. Beneficio: defensa contra supply chain attacks no triviales.

#### 3.6 W3C traceparent en headers Kafka/NATS

**Adopto.** Hoy `correlation_id` viaja en payload, no en header. Sin `traceparent` no hay continuity OTel cross‑bus.

**Cómo se adopta:** modificar `libs/event-bus-data.Publisher.Publish` y `libs/event-bus-control.Publisher.Publish` para inyectar `traceparent` y `tracestate` desde span context. Modificar `Subscriber` para extraer y propagar. Coste 1 sprint.

#### 3.7 Tests IDOR y privilege escalation

**Adopto.** Auth tests existen pero no hay suite explícita de tests negativos cross‑tenant.

**Cómo se adopta:** suite en `services/edge-gateway-service/internal/proxy/security_test.go` y `services/object-database-service/internal/server/security_test.go` que cree dos tenants, dos usuarios por tenant, y verifique que cada combinación cruzada falla con 403. Generadores de fixtures en `libs/testing/`. Coste 1 sprint.

#### 3.8 PodAntiAffinity duro en componentes críticos

**Adopto.** Hoy hay `topologySpreadConstraints` con `ScheduleAnyway`. Bajo presión de scheduler, dos réplicas pueden caer en el mismo nodo.

**Cómo se adopta:** para `edge-gateway-service`, `identity-federation-service`, `object-database-service`, `iceberg-catalog-service`: `podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution` por `kubernetes.io/hostname` y `preferredDuringScheduling…` por `openfoundry.io/chassis`. Coste 1 sprint.

#### 3.9 readOnlyRootFilesystem: true por defecto

**Adopto.** Helpers ya tienen `emptyDir /tmp`. Cambiar el default a `true` y dejar overrides explícitos.

Coste: 1 día. Beneficio: hardening adicional sin coste runtime.

#### 3.10 Activación región B Kafka MM2 + DR drill

**Adopto.** `kafka-cluster/values.yaml:218` tiene `regionB.enabled: false`. ADR‑0023 documenta intención de DR cross‑region. No probado.

**Cómo se adopta:**

1. Activar en staging con un cluster pequeño (3 brokers en otra zona).
2. MirrorMaker2 configurado con prefijo `b.` para topics.
3. Game‑day semestral siguiendo `infra/runbooks/dr-game-day.md`.
4. Métricas de lag MM2 con alerta si > 1000 mensajes durante 5 min.

Coste 2 sprints más operación recurrente. Beneficio: RPO/RTO declarables.

### 4. Adopciones para escalar (P2)

Estos hallazgos los adoptamos cuando el sistema lo requiera, no antes. Son inversiones, no urgencias.

#### 4.1 Decisión formal Vespa vs OpenSearch

**Adopto, pero la conclusión esperada es mantener Vespa.** La auditoría señala que el prompt declaraba OpenSearch como objetivo y el repo usa Vespa. Hace falta un ADR formal de cierre. Mi posición: **mantener Vespa**. Razones técnicas en §5.1.

#### 4.2 Decisión formal Ceph vs Ozone

**Adopto, pero la conclusión esperada es mantener Ceph.** Ver §5.2.

#### 4.3 Evaluación StarRocks vs Trino+DataFusion

**Adopto cuando haya benchmark.** No introducir StarRocks por adelantado. Razones en §5.3.

#### 4.4 HPA con KEDA por Kafka lag

**Adopto cuando haya consumidores con lag medible.** Ahora mismo el volumen no lo justifica. KEDA es una pieza más a operar.

#### 4.5 Service mesh (Istio/Linkerd o Cilium WireGuard)

**No adopto en horizonte 12 meses.** Es una pieza pesada. mTLS interno via SPIFFE/SPIRE light o Cilium con WireGuard es alternativa mejor cuando el requisito sea real. Hoy las NetworkPolicies bien hechas + Vault para certificados internos + cert‑manager es suficiente.

#### 4.6 Múltiples Go modules

**No adopto en horizonte 12 meses.** Single module está bien para un equipo pequeño. Splitar es una migración cara con beneficios marginales hasta que el build tarde > 5 min o el equipo supere ~10 ENG.

#### 4.7 Servicio case‑management

**Adopto en fase 5 de roadmap.** Es Foundry parity real. Necesario para investigar/colaborar como Gotham. Diseño aparte.

#### 4.8 Modelo de propiedad enriquecido

**Adopto en fase 3/4.** `{value, source, confidence, asserted_at, evidence_ref}` por propiedad, no solo por entidad. Coste alto pero diferencial claro. Hace falta tras bitemporalidad.

#### 4.9 Geospatial activation

**Adopto solo si el caso de uso lo exige.** Hoy `libs/geospatial-core` es placeholder. No invertir hasta que un cliente lo pida explícitamente.

### 5. Decisiones que mantenemos tal y como están

Aquí están las decisiones que la auditoría señaló como divergentes del prompt o como "antipatrones potenciales", pero que tras analizar son decisiones arquitectónicas válidas con justificación técnica fuerte. **No se cambian.**

#### 5.1 Vespa en lugar de OpenSearch

**Mantengo Vespa.** Justificación:

- **Vespa es superior técnicamente para el caso "search híbrido + vector denso"** que un sistema tipo Foundry necesita. BM25 + ANN HNSW + ranking learned‑to‑rank son nativos en un solo motor. OpenSearch tiene k‑NN plugin con HNSW pero el ranking híbrido es menos maduro.
- **Vespa escala horizontalmente con menos overhead operacional** que OpenSearch para indices con vector denso a la escala que necesitamos. OpenSearch cluster con vectores denso a millones de docs requiere tuning agresivo.
- **Vespa tiene tensor framework integrado** que permite expressing ranking complejos sin pre‑cálculo externo.
- **Ya está integrado** (`libs/search-abstraction/vespa/`), `ontology-indexer` lo soporta, hay reglas Prometheus, runbooks, dashboards de Grafana commiteados.
- **Cambiar a OpenSearch sería rebuild de read models, retraining de embeddings y operación de un cluster que el equipo no domina hoy.** Coste alto, beneficio marginal.

ADR‑0007 ya razonó esto. La auditoría señala la divergencia con el prompt; respondemos con ADR formal de cierre que ratifica la decisión. **No es un antipatrón — es una decisión técnica sólida que el prompt no había considerado.**

Lo único que adopto es escribir el ADR de cierre formal.

#### 5.2 Ceph en lugar de Apache Ozone

**Mantengo Ceph.** Justificación:

- **Ceph es mucho más maduro en Kubernetes** vía Rook que Ozone vía Hadoop YARN. Para una plataforma cloud‑native on‑prem, esto es decisivo.
- **Rook tiene operadores estables** desde hace años; Ozone Kubernetes operator es relativamente nuevo y menos battle‑tested.
- **Ceph soporta block (RBD) + object (RGW) + filesystem (CephFS)** en un solo cluster, lo que simplifica la operación. Ozone es solo object.
- **El equipo de operación tiene mucha más experiencia con Ceph** que con Ozone.
- **Ceph encaja mejor con el modelo Synergy** (failure domain by chassis/rack) usando CRUSH rules. Ozone tiene rack awareness pero la integración con CRUSH no aplica.
- **Ozone ofrece mejor integración con Hadoop ecosystem** (HDFS API, native YARN), lo cual es ventaja **solo si el stack es Hadoop‑native**, que no es el caso.

La divergencia con el prompt se cierra con un ADR formal explicando criterios. Y con Ceph CRUSH bien diseñado (§2.3), la objeción "Ozone tiene mejor failure domain" desaparece.

#### 5.3 Sin StarRocks; Trino + DataFusion + materializaciones

**Mantengo. No introducir StarRocks por adelantado.** Justificación:

- **StarRocks soluciona un problema real solo si el SLO de dashboards interactivos es menor que 200ms p99 con cardinalidades altas (mas de 100M filas).** Si ese SLO no está medido y validado contra Trino, introducir StarRocks es over‑engineering.
- **ADR‑0014 retiró Trino, ADR‑0029 lo reintrodujo.** Esto refleja maduración: el equipo entendió por qué hacía falta. Trino es el mejor federated SQL open source, y para "analítica histórica con federación" cumple.
- **DataFusion local + Iceberg via Flight SQL** cubre el caso de "consulta cacheable o de dataset acotado" con latencias sub‑segundo. Ya está integrado en `sql-bi-gateway-service`.
- **Materializaciones en Cassandra + Vespa** cubren los read models hot. Si un dashboard concreto necesita < 200 ms p99, se materializa específicamente — no se introduce todo un OLAP store por cubrir un caso.
- **StarRocks añade un nuevo backend a operar**, con sus propios FE/BE/CN nodes, su propia HA, su propia ingesta desde Kafka, su propio integración con Iceberg. Es un coste operacional grande.

**Cuándo se reconsideraría:** cuando un benchmark con datos reales muestre que más del 30% de los dashboards tiene p99 > 1s con Trino tuneado. Hasta entonces, mantener stack actual y materializar selectivamente.

#### 5.4 Sin Valkey/Redis distribuida; Redis solo para rate-limit

**Mantengo. No introducir caché distribuida operacional general.** Justificación:

- **Caché operacional distribuida añade un punto de invalidación que es famosamente difícil de mantener consistente.** "There are only two hard things in computer science: cache invalidation and naming things." — el adagio aplica.
- **Cassandra LOCAL_QUORUM con read repair + JVM page cache** ya da latencias p99 < 15 ms para reads por id. Una caché L1 en el servicio (in‑process LRU) cubre el 80/20 sin red.
- **Sessions ya viven en Cassandra** con replicación natural cross‑DC.
- **Redis está limitado a rate‑limit del gateway** porque ahí sí necesita estado distribuido con expiración fina. Eso es uso correcto.
- **Introducir Valkey/Redis general** abriría debates de "qué se cachea, dónde se invalida, qué TTL" que distraen del trabajo arquitectónico real.

**Cuándo se reconsideraría:** si un endpoint específico muestra p99 > SLO por culpa de Cassandra hot read y la cardinalidad de cache hit es alta (>80%). Solución localizada, no plataforma.

#### 5.5 NATS JetStream + Kafka (split control/data)

**Mantengo el split.** ADR‑0011 lo justifica. Razones:

- **NATS para control plane** (RPC durables, fan‑out, eventos transitorios) tiene latencias µs–ms que Kafka no alcanza por su naturaleza partitioned‑log.
- **Kafka para data plane** (CDC, ingestión, lineage, audit) es la única opción con la durabilidad y ordenación parcial que el sistema requiere.
- **El split es coherente con la lectura del sistema**: Cassandra/Postgres/Iceberg/Spark/Trino piensan en "registros y filas" → Kafka. Los servicios piensan en "eventos de coordinación que disparan handlers" → NATS.
- **`tools/bus-lint` aplica reglas** de qué tipo de evento puede ir a qué bus. Esto previene drift.
- **Foundry‑pattern (ADR‑0037) explícitamente usa Kafka para eventos durables y NATS para hot reload de políticas y notifications fan‑out.** Es coherente.

La auditoría sugería "matriz documentada evento↔bus". Esa parte sí la adopto. La decisión de tener dos buses es correcta.

#### 5.6 Foundry-pattern orchestration (ADR-0037) sin volver a Temporal

**Mantengo. No volver a Temporal jamás.** ADR‑0037 lo dice y la decisión es sólida. Razones reforzadas:

- **Temporal-on-Cassandra fue retirado por SPOF (cluster dedicado), determinismo footgun, y duplicación de coordinación** que outbox+Debezium ya proveía.
- **Cualquier "vuelta a Temporal"** introduciría exactamente los problemas que se eliminaron: cluster dedicado, requirement de determinismo en el código de workflow, operación de Cadence/Temporal cluster.
- **Foundry‑pattern actual** (Postgres state machine + outbox + Kafka choreography + idempotency record‑before‑process + saga LIFO compensation + CronJobs SKIP LOCKED) es **operacionalmente más ligero**, **más testeable**, y **alineado con el resto del stack**.
- Para los workflows que sí necesitan ser orquestados centralmente y visibles para usuarios — caso de uso distinto al de Foundry‑pattern — proponemos un motor propio (Parte II), inspirado en Conductor OSS, **no en Temporal**.

#### 5.7 Lakekeeper como catálogo Iceberg externo + iceberg-catalog-service propio

**Mantengo ambos.** ADR‑0008 introdujo Lakekeeper, ADR‑0041 introdujo el catálogo propio. La razón de tener dos no es duplicación:

- **Lakekeeper** sirve como REST Catalog adapter para clientes externos (Spark jobs, Trino, PyIceberg). Es Apache 2.0, K8s‑native, soporta credentials vending y signed URLs.
- **iceberg-catalog-service** propio implementa la semántica Foundry específica: multi‑table transactions atómicas, master/main aliasing, schema strict mode con `/alter-schema` explícito, integración Cedar nativa para policies por tabla/namespace.

Esta distinción es **deliberada**: Lakekeeper es "catalog para herramientas estándar", iceberg-catalog-service es "catalog para el producto". Es un patrón de adapter de fronteras: cada audiencia con su catálogo, ambos sobre el mismo storage.

#### 5.8 Trino reintroducido (ADR-0029) tras retiro (ADR-0014)

**Mantengo.** El "ping-pong" no es indecisión sino aprendizaje:

- ADR‑0014 retiró Trino para apostar por DataFusion local en `sql-bi-gateway-service` con Flight SQL.
- ADR‑0029 reintrodujo Trino al confirmar que DataFusion local no escala para queries que tocan datasets > 10GB ni federación entre catálogos heterogéneos.

Ambos son ADRs cerrados. La conclusión es **DataFusion para fast path acotado, Trino para queries pesadas/federadas**. Es la división correcta.

#### 5.9 Cedar como motor de políticas

**Mantengo.** ADR‑0027 lo justifica. Cedar tiene:

- Sintaxis declarativa diseñada para autorización (no es lógica imperativa).
- Análisis estático posible (verificación formal de políticas).
- Schema rico que ya cubre User/Group/Role/Marking/Dataset/MediaSet/IcebergTable/etc.
- Performance suficiente (~µs por evaluación con políticas no triviales).
- Integración cedar‑go con tests de conformancia.

Alternativas (OPA/Rego, Ranger, Casbin): cada una tiene tradeoffs peores para este caso. OPA/Rego es Turing‑complete y más difícil de razonar. Ranger es Hadoop‑oriented. Casbin es más ligero pero menos expresivo.

Lo único que adopto (§2.6) es **enforced en runtime, no solo en admin**. La elección del motor es correcta.

#### 5.10 CloudNativePG como operador Postgres

**Mantengo.** ADR‑0010. CNPG:

- Es K8s‑native con CRDs limpios.
- Tiene backup integrado (Barman a S3).
- Maneja failover automático con quorum.
- Tiene PgBouncer integrado vía `pooler` CRD.
- Es mantenido por EnterpriseDB con releases estables.

Patroni y Zalando operator son alternativas válidas pero menos integradas con K8s. Cambiar de operador es trabajo grande sin beneficio claro.

#### 5.11 K8ssandra para Cassandra

**Mantengo.** Mejor operador multi‑DC, incluye Reaper (anti‑entropy repair) y Medusa (backup). DataStax‑backed pero open source Apache 2.0.

#### 5.12 Strimzi para Kafka KRaft

**Mantengo.** ADR‑0013. Strimzi es el operador Kafka standard en K8s, soporta KRaft sin ZooKeeper, hace rolling upgrades correctos, integra MM2.

#### 5.13 Single Helm chart con _shared/templates/helpers.tpl

**Mantengo.** El patrón DRY de helpers compartidos entre las 5 releases es correcto. Permite cambios de defaults globales sin tocar 40 charts. Solo adopto los endurecimientos de §3.8 y §3.9.

#### 5.14 Single Go module

**Mantengo.** Por las razones de §4.6.

#### 5.15 Ontology indexing via Kafka (no graph DB primario)

**Mantengo.** El user prompt mencionaba JanusGraph como opcional. La elección actual — proyectar a Vespa + Cassandra + materialized read models — es correcta para el 95% de queries. Un graph DB se justifica solo para traversals profundos (>3 hops) con cardinalidad alta, lo que no es la query típica de Foundry. Si aparece, se introduce localmente para ese caso de uso, sin desplazar la fuente de verdad.

#### 5.16 Audit trail con UUIDv5 deterministic + outbox + Iceberg

**Mantengo.** Patrón ejemplar. El single único cambio es reducir retención Kafka de `audit.events.v1` a 30‑90 días dado que Iceberg `of_audit.events` es ya la verdad histórica. Ahorra disco Kafka sin perder nada.

#### 5.17 Distroless + nonroot UID 10001 + caps drop ALL

**Mantengo.** Hardening top tier. Solo adopto pin de imagen base por digest (§3.5).

### 6. Decisiones que matizamos (adoptamos parcialmente)

#### 6.1 ADR formal Vespa vs OpenSearch

Adopto el **acto de escribir el ADR**, no la conclusión que la auditoría sugería. La conclusión queda "mantener Vespa".

#### 6.2 ADR formal Ceph vs Ozone

Idem. Adopto el ADR; la conclusión es mantener Ceph con CRUSH map adecuado.

#### 6.3 Reducción retención Kafka audit.events.v1

Adopto reducción a 30 días. Iceberg es la verdad histórica; mantener 10 años en Kafka es desperdicio de almacenamiento.

#### 6.4 Tests de contrato (Pact)

Adopto **selectivamente**: solo en boundaries críticos gateway↔identity, gateway↔ontology, ontology↔object-database, sinks↔Kafka. No exhaustivo en todos los pares servicio↔servicio porque el coste de mantener N² contratos no compensa.

#### 6.5 Chaos Mesh

Adopto suite mínima en CI nightly: kill broker Kafka, kill PG primary, kill RGW, kill ontology-indexer pod. No adopto un programa extensivo de chaos hasta que el sistema esté estable y la operación tenga ancho de banda para responder.

### 7. Tabla resumen de adopción

| Hallazgo de auditoría | Decisión | Prioridad | Coste estimado | Justificación corta |
|---|---|---|---|---|
| Secretos vía ESO + Vault | **Adopto** | P0 | 1‑2 sprints | Cumplimiento, rotación |
| Tempo + Loki + Alertmanager routing | **Adopto** | P0 | 2‑3 sprints | Observabilidad real |
| Ceph CRUSH por chassis Synergy | **Adopto** | P0 | 1 sprint | Sobrevivir pérdida de chassis |
| Cierre 9 stubs P0/P1 | **Adopto** | P0 | 4‑6 sprints | Producto operativo |
| Mantenimiento Iceberg | **Adopto** | P0 | 1‑2 sprints | Lakehouse operacional |
| Cedar enforcement runtime | **Adopto** | P0 | 2‑3 sprints | Seguridad real |
| Bitemporalidad ontología | **Adopto** | P0 | 4‑6 sprints | Foundry parity |
| ArgoCD GitOps | **Adopto** | P0 | 2 sprints | Operación |
| libs/resilience | **Adopto** | P1 | 1 sprint | Higiene |
| Apicurio gate | **Adopto** | P1 | 2 sprints | Schema governance |
| NetworkPolicies completas | **Adopto** | P1 | 1 sprint | Seguridad |
| golang-migrate | **Adopto** | P1 | 1‑2 sprints | Schema versioning |
| SBOM/gosec/cosign | **Adopto** | P1 | 1‑2 sprints | Supply chain |
| W3C traceparent en headers | **Adopto** | P1 | 1 sprint | Tracing |
| Tests IDOR/escalada | **Adopto** | P1 | 1 sprint | Seguridad |
| podAntiAffinity duro | **Adopto** | P1 | 1 sprint | HA |
| readOnlyRootFilesystem | **Adopto** | P1 | 1 día | Hardening |
| Activación región B Kafka MM2 | **Adopto** | P1 | 2 sprints | DR |
| ADR formal Vespa vs OpenSearch | **Adopto el ADR** | P2 | 1 día | Cierre decisión |
| ADR formal Ceph vs Ozone | **Adopto el ADR** | P2 | 1 día | Cierre decisión |
| Cambiar Vespa → OpenSearch | **NO adopto** | — | — | Vespa es superior técnicamente |
| Cambiar Ceph → Ozone | **NO adopto** | — | — | Ceph más maduro en K8s |
| Introducir StarRocks | **NO adopto ahora** | P2 condicional | — | Sin SLO que lo justifique |
| Introducir Valkey | **NO adopto ahora** | P2 condicional | — | Cassandra + L1 in-process basta |
| Volver a Temporal | **NO adopto jamás** | — | — | ADR-0037 cerrado |
| Service mesh | **NO adopto en 12m** | P2 | — | NPs + cert-manager basta |
| Multi Go module | **NO adopto en 12m** | P2 | — | Single module funciona |
| Reducir retention Kafka audit | **Adopto** | P1 | 1 día | Iceberg es la verdad |
| Tests Pact selectivos | **Adopto** | P1 | 1 sprint | Boundaries críticos |
| Chaos Mesh suite mínima | **Adopto** | P1 | 1 sprint | Resiliencia probada |
| Servicio case-management | **Adopto en fase 5** | P2 | 4‑6 sprints | Foundry parity real |
| Modelo propiedad enriquecido | **Adopto en fase 4** | P2 | 4 sprints | Confidence/provenance |
| Geospatial activation | **Adopto si caso lo exige** | P3 | TBD | Sin caso real |

---

## Parte II — Diseño del motor de workflows propio (of-flow)

Esta parte plantea el diseño técnico de un motor de workflows propio inspirado en Conductor OSS. Se llama **of-flow** (OpenFoundry Flow) en este documento. El nombre puede cambiar; la arquitectura no.

### 8. Por qué un motor propio y no Temporal

Esto se ha discutido a fondo en ADR‑0037 y la conclusión sigue siendo válida. Resumido:

- **Temporal exige determinismo en el código de workflow** (replay seguro depende de que el workflow code sea puro). Esto es un footgun: cualquier llamada no determinista (time.Now, rand, lookup en mapa con orden de iteración) rompe el replay y se diagnostica solo en producción.
- **Temporal cluster es un SPOF‑class workload** (frontend/history/matching/worker, todo en un cluster que tiene su propio Cassandra/Postgres/MySQL como backend). Operar Temporal en producción es operar un sub‑sistema completo más.
- **Temporal duplica funcionalidad** que outbox+Debezium+Postgres state machines ya proveen en este sistema. ADR‑0037 cuantifica el ahorro.
- **El equipo no quiere Temporal**, lo dice explícitamente. Esa es razón suficiente.

Hay otras alternativas (Cadence, que es la versión Uber pre‑Temporal; Conductor; Argo Workflows). Conductor encaja por las razones del siguiente punto.

### 9. Por qué Conductor OSS como inspiración

**Conductor OSS** (originalmente Netflix, ahora Orkes) tiene un modelo radicalmente distinto a Temporal:

- **Workflows declarativos en JSON/YAML**, no en código.
- **Workers polling**: cada worker es un proceso aparte que hace long‑poll al servidor para reclamar tareas pendientes de su `task_type`.
- **Servidor central** que mantiene el estado del workflow y orquesta el avance.
- **Tipos de tarea built‑in**: HTTP, decision, switch, fork, join, dynamic_fork, do_while, sub_workflow, wait, terminate, kafka_publish, json_jq_transform, set_variable, inline, …
- **Backend** de persistencia: Redis original, ahora Cassandra/Postgres/MySQL/ScyllaDB en versiones recientes.
- **UI built‑in** para diseñar y monitorizar.

**Por qué inspirar en Conductor y no en Argo Workflows:**

- Argo Workflows está orientado a **batch ML/ETL en K8s**, donde cada paso es un Pod nuevo. Para un workflow user‑facing con tareas ligeras (HTTP, decisión, transformación) crear un Pod por tarea es overhead enorme.
- Conductor está orientado a **flujos largos con tareas heterogéneas**: HTTP a microservicios, decisiones, esperas, fork/join. Es exactamente el patrón de un sistema tipo Foundry.
- Argo CRD‑native acopla más a K8s; Conductor es agnóstico al runtime (workers pueden ser cualquier proceso).

**Lo que SÍ tomamos de Conductor:**

1. Modelo declarativo JSON/YAML.
2. Worker polling (con long‑poll para reducir latencia).
3. Tipos de tarea built‑in para los casos comunes.
4. Servidor central con UI de diseño y monitorización.
5. Versionado de definiciones.

**Lo que NO tomamos de Conductor:**

1. Redis como backend primario. Usaremos **Postgres** para persistencia (ya hay 4 clusters CNPG + libs como `state-machine`, `outbox`, `idempotency`).
2. Lógica de orquestación monolítica. La nuestra reusará las primitivas existentes (state-machine, saga, outbox, scheduler).
3. SDK propio. El nuestro se publica como `libs/flow-sdk-go`, `sdks/python/openfoundry-flow`, `sdks/typescript/openfoundry-flow`.
4. Decoupling completo de auth. **Cedar evalúa cada inicio de workflow y cada acción humana.**
5. UI separada. La integramos en `apps/web` como módulo, no como app aparte.

### 10. Distinción crítica: of-flow vs Foundry-pattern (ADR-0037)

**Esta es la decisión arquitectónica más importante de Parte II:**

- **Foundry-pattern (ADR-0037)** es la columna vertebral de **orquestación interna** del sistema. Saga interservicio (cleanup_workspace, retention_sweep), approvals con timeout sweep, automation runs, dataset lifecycle. **No la ven los usuarios finales.** Es plumbing del sistema.

- **of-flow** es **orquestación user‑facing**. Workflows que un analista o investigador define en una UI declarativa para automatizar **su propio proceso**: "cuando llegue una alerta de tipo X, busca documentos relacionados, envíalos al modelo Y, espera revisión humana, si aprobado actualiza el caso, si no abre ticket en sistema externo".

**Las dos cosas coexisten. No se sustituyen mutuamente.**

| Dimensión | Foundry‑pattern | of-flow |
|---|---|---|
| Audiencia | Servicios internos | Usuarios finales (analistas, devs de apps) |
| Definido por | Ingenieros en Go | Usuarios en UI / YAML |
| Versionado | Código + migrations | Workflow definitions JSON con `(name, version)` |
| Coordinación | Choreography (eventos Kafka) | Orchestration (servidor central) |
| Modelo | State machine + saga + outbox | Workers polling + DAG executor |
| Visibilidad | Logs + audit + Grafana | UI built‑in + audit + Grafana |
| Cambio en runtime | Deploy + migration | Publicar nueva versión, runs viejos siguen con la suya |
| Tareas típicas | "consume evento, escribe outbox, aplica state machine" | "llama HTTP, decide, espera humano, publica Kafka" |
| Errores | Compensación saga LIFO | Reintentos por tarea + compensaciones declarativas |

**Analogía:** Foundry‑pattern es como las pipelines internas del kernel de un OS. of-flow es como `cron` + `systemd timers` + `ifttt` + `zapier` que el usuario configura.

### 11. Principios de diseño de of-flow

1. **Reusar lo existente.** No reinventar nada que ya viva en `libs/`. Específicamente:
   - `libs/state-machine` para estado de workflow runs y task runs.
   - `libs/saga` para compensaciones cuando un workflow falla parcialmente.
   - `libs/outbox` para emitir eventos de cada transición.
   - `libs/idempotency` para record‑before‑process en workers.
   - `libs/event-scheduler` para `wait_for_time` tasks (SKIP LOCKED).
   - `libs/scheduling-cron` para schedules recurrentes.
   - `libs/event-bus-control` (NATS) para distribuir tareas a workers.
   - `libs/event-bus-data` (Kafka) para emitir audit events.
   - `libs/audit-trail` para event envelopes.
   - `libs/authz-cedar-go` para evaluar políticas en workflow start y human tasks.
   - `libs/observability` para tracing y métricas.

2. **Postgres como single source of truth de runs y tasks.** No Redis. No memoria. La razón: replay y debugging dependen de un store ACID.

3. **NATS JetStream para distribución de tareas a workers.** Cada `task_type` tiene un subject; workers se suscriben con AckExplicit + durable consumer. Long‑poll efímero del lado del worker.

4. **Workers son procesos cualquier‑lenguaje** que importan un SDK y registran handlers por `task_type`. Son completamente stateless. Pueden escalar como Deployment normal.

5. **Engine es un servicio K8s** (`flow-orchestrator-service`), tres réplicas, líder elect via Postgres advisory lock — el líder corre los timers y schedules, los seguidores sirven la API.

6. **DSL declarativo en YAML/JSON**, validado por JSON Schema. Versionado, almacenado en Postgres.

7. **Cada workflow run es una entidad de la ontología** (opcional pero recomendable) — esto hace que workflows sean primera‑clase y aparecen en búsqueda, lineage, etc.

8. **Audit trail es no negociable.** Cada start, claim, complete, fail, compensate, human action emite un evento `flow.events.v1` que va a Iceberg via audit-sink.

9. **Cedar enforcement.** Cada start de workflow se autoriza (puede usuario X ejecutar workflow Y con input Z?). Cada human task se asigna a un principal y la respuesta se autoriza.

10. **Determinismo NO se exige al código de los workers.** Workers pueden hacer time.Now, llamadas externas, lo que sea. El engine reconcilia con idempotency keys, no con replay de código. Esta es la diferencia capital con Temporal.

### 12. Arquitectura de alto nivel

```
                                 apps/web (UI: designer + monitor)
                                              │
                  REST + WebSocket │ POST /api/v1/flow/definitions
                                   │ POST /api/v1/flow/runs
                                   │ POST /api/v1/flow/tasks/poll
                                   │ POST /api/v1/flow/tasks/{id}/complete
                                   │ POST /api/v1/flow/tasks/{id}/fail
                                   │ POST /api/v1/flow/human/{id}/respond
                                   │ GET  /api/v1/flow/runs/{id} (SSE for live)
                                              │
                                              ▼
        ┌────────────────────────────────────────────────────────────────┐
        │                  flow-orchestrator-service                       │
        │  ┌─────────────────────────────────────────────────────────┐   │
        │  │  HTTP API (chi)                                          │   │
        │  │  ├─ DefinitionRegistry (CRUD + version + validate)      │   │
        │  │  ├─ RunController (start/cancel/inspect/replay)         │   │
        │  │  ├─ TaskController (poll/complete/fail/heartbeat)       │   │
        │  │  └─ HumanController (assign/respond/escalate)           │   │
        │  └─────────────────────────────────────────────────────────┘   │
        │  ┌─────────────────────────────────────────────────────────┐   │
        │  │  Engine (leader-elected, Postgres advisory lock)         │   │
        │  │  ├─ DAGExecutor: avanza runs, dispatcha siguientes      │   │
        │  │  ├─ TimerWheel: dispara wait_for_time, timeouts         │   │
        │  │  ├─ ScheduleTicker: cron schedules → run starts         │   │
        │  │  ├─ Reaper: detecta workers caídos vía heartbeat        │   │
        │  │  └─ Compensator: ejecuta saga LIFO en fallos            │   │
        │  └─────────────────────────────────────────────────────────┘   │
        │  ┌─────────────────────────────────────────────────────────┐   │
        │  │  Repositories (Postgres via libs/db-pool)                │   │
        │  └─────────────────────────────────────────────────────────┘   │
        └────────────────────────────────────────────────────────────────┘
                                   │                          │
                  NATS subjects   │                         │  Kafka topics
                  flow.tasks.{type} │                        │  flow.events.v1
                                   ▼                          ▼
                  ┌──────────────────────────┐     audit-sink → Iceberg
                  │  Workers (Deployments)   │
                  │  Go / Python / TS / Java │
                  │  poll → execute → ack    │
                  └──────────────────────────┘
                                   │
                  ┌────────────────┴───────────────┐
                  ▼                                ▼
        ontology-actions-service        otros servicios HTTP/gRPC
        connector-management              (vía task type http/grpc)
```

Notas:
- **Tres réplicas** del orchestrator. Solo una es leader (timers, schedule ticker). Las demás sirven API.
- **Postgres advisory lock** para leader election (cero infra extra; ya hay PG).
- **NATS JetStream** distribuye tasks a workers con AckExplicit. Workers no fallan silenciosamente: si no acked en `task.deadline`, el reaper devuelve la tarea a la cola.

### 13. Modelo de datos (PostgreSQL)

Diseñado para reutilizar el schema isolation pattern actual (`pgPolicy` o nuevo cluster `pgFlow`). Por simplicidad asumo namespace `flow.*` en `pg-runtime-config` cluster CNPG existente.

```sql
-- ---------------------------------------------------------------
-- Definiciones de workflows (declarativas, versionadas, immutables)
-- ---------------------------------------------------------------
CREATE TABLE flow.definitions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL,
    name          text NOT NULL,
    version       integer NOT NULL,
    spec          jsonb NOT NULL,                 -- DSL completo
    spec_hash     text NOT NULL,                  -- sha256(spec) para dedup
    status        text NOT NULL DEFAULT 'active', -- active | deprecated | archived
    description   text,
    owner_id      uuid NOT NULL,
    tags          text[] NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    deprecated_at timestamptz,
    UNIQUE (tenant_id, name, version)
);

CREATE INDEX idx_definitions_active ON flow.definitions(tenant_id, name)
    WHERE status = 'active';

-- ---------------------------------------------------------------
-- Runs (instancias de workflows en ejecución)
-- ---------------------------------------------------------------
CREATE TABLE flow.runs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL,
    definition_id   uuid NOT NULL REFERENCES flow.definitions(id),
    correlation_id  uuid,                          -- para trace
    parent_run_id   uuid,                          -- sub_workflow
    state           text NOT NULL,                 -- pending | running | succeeded | failed | cancelled | compensating
    state_data      jsonb NOT NULL DEFAULT '{}',   -- variables del workflow
    input           jsonb NOT NULL DEFAULT '{}',
    output          jsonb,
    error           jsonb,
    version         bigint NOT NULL DEFAULT 1,     -- optimistic locking
    started_by      uuid NOT NULL,                 -- principal que inició
    started_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz,
    expires_at      timestamptz,                   -- TTL global del run
    priority        integer NOT NULL DEFAULT 0     -- mayor = más prioritario
);

CREATE INDEX idx_runs_active ON flow.runs(tenant_id, state)
    WHERE state IN ('pending', 'running', 'compensating');
CREATE INDEX idx_runs_expires ON flow.runs(expires_at)
    WHERE state IN ('pending', 'running');

-- ---------------------------------------------------------------
-- Task runs (cada nodo del DAG ejecutado)
-- ---------------------------------------------------------------
CREATE TABLE flow.task_runs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id          uuid NOT NULL REFERENCES flow.runs(id) ON DELETE CASCADE,
    task_id         text NOT NULL,                 -- id en el spec
    task_type       text NOT NULL,                 -- http, decision, fork, …
    state           text NOT NULL,                 -- scheduled | claimed | running | succeeded | failed | cancelled | skipped | compensated
    input           jsonb NOT NULL DEFAULT '{}',
    output          jsonb,
    error           jsonb,
    attempt         integer NOT NULL DEFAULT 1,
    max_attempts    integer NOT NULL DEFAULT 3,
    backoff         jsonb NOT NULL DEFAULT '{"type":"exponential","base_ms":500,"cap_ms":30000,"jitter":true}',
    claimed_by      text,                          -- worker id
    claimed_at      timestamptz,
    deadline_at     timestamptz,                   -- si no acked, vuelve a cola
    scheduled_at    timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz,
    succeeded_at    timestamptz,
    failed_at       timestamptz,
    idempotency_key text,                          -- inyectado al worker
    correlation_id  uuid,                          -- trace
    span_id         text,                          -- OTel
    UNIQUE (run_id, task_id, attempt)
);

-- Índice para el polling de workers
CREATE INDEX idx_task_runs_pollable
    ON flow.task_runs(task_type, state, scheduled_at)
    WHERE state = 'scheduled';

-- Índice para el reaper de workers caídos
CREATE INDEX idx_task_runs_deadline
    ON flow.task_runs(deadline_at)
    WHERE state = 'claimed';

-- ---------------------------------------------------------------
-- Eventos (audit trail interno; también va al outbox externo)
-- ---------------------------------------------------------------
CREATE TABLE flow.events (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       uuid NOT NULL REFERENCES flow.runs(id) ON DELETE CASCADE,
    task_run_id  uuid REFERENCES flow.task_runs(id) ON DELETE CASCADE,
    kind         text NOT NULL,                    -- run.started, task.scheduled, …
    payload      jsonb,
    actor        jsonb,                            -- {id, type: user|service|system}
    occurred_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_run ON flow.events(run_id, occurred_at);

-- ---------------------------------------------------------------
-- Human tasks (subset especial de task_runs con asignación humana)
-- ---------------------------------------------------------------
CREATE TABLE flow.human_tasks (
    task_run_id   uuid PRIMARY KEY REFERENCES flow.task_runs(id) ON DELETE CASCADE,
    assignee_id   uuid,                            -- principal específico, NULL si rol
    assignee_role text,                            -- rol o grupo
    form_id       text,                            -- referencia a form en form-registry
    form_data     jsonb,                           -- pre-fill data
    response      jsonb,                           -- respuesta del humano
    responded_by  uuid,
    responded_at  timestamptz,
    escalation_chain uuid[]                        -- next assignees on timeout
);

CREATE INDEX idx_human_tasks_assignee ON flow.human_tasks(assignee_id);

-- ---------------------------------------------------------------
-- Schedules (workflows recurrentes via libs/scheduling-cron)
-- ---------------------------------------------------------------
CREATE TABLE flow.schedules (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL,
    name           text NOT NULL,
    definition_id  uuid NOT NULL REFERENCES flow.definitions(id),
    cron           text NOT NULL,                  -- 5/6 fields
    timezone       text NOT NULL DEFAULT 'UTC',
    input_template jsonb,                          -- jq-style template
    enabled        boolean NOT NULL DEFAULT true,
    last_run_at    timestamptz,
    next_run_at    timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX idx_schedules_due ON flow.schedules(next_run_at)
    WHERE enabled = true;

-- ---------------------------------------------------------------
-- Outbox para eventos hacia Kafka (Debezium SMT)
-- ---------------------------------------------------------------
CREATE TABLE flow.outbox_events (
    event_id     uuid PRIMARY KEY,
    aggregate    text NOT NULL,
    aggregate_id text NOT NULL,
    payload      jsonb NOT NULL,
    headers      jsonb NOT NULL DEFAULT '{}',
    topic        text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE flow.outbox_events REPLICA IDENTITY FULL;
```

### 14. Lenguaje declarativo de workflows (DSL)

Diseñado en YAML para escritura humana, JSON para wire format. Validado por JSON Schema. Inspirado en Conductor con simplificaciones y extensiones específicas de OpenFoundry.

#### 14.1 Estructura general

```yaml
name: investigation_review
version: 1
description: Review an investigation case and trigger downstream actions
inputs:
  - name: case_id
    type: string
    required: true
  - name: priority
    type: string
    enum: [low, normal, high, critical]
    default: normal
outputs:
  - name: result
    type: object
defaults:
  retry:
    max_attempts: 3
    backoff: { type: exponential, base_ms: 500, cap_ms: 30000, jitter: true }
  timeout: 30s
permissions:
  required: [investigation:read, investigation:write]
tasks:
  - id: load_case
    type: ontology_action
    spec:
      action: case.read
      input:
        id: ${input.case_id}
    out: case
  ...
```

#### 14.2 Convenciones

- `${input.X}` resuelve a inputs del workflow.
- `${tasks.{id}.output.X}` resuelve a output de una tarea anterior.
- `${var.X}` resuelve a variable definida con `set_variable`.
- `${secrets.X}` resuelve a un secret referenciado por nombre (no embebido).
- Expressions: subset de jq (`length`, `keys`, `select`, `map`, comparaciones).
- `out: {name}` asigna el output de la tarea a `tasks.{id}.output` y opcionalmente a una variable.

#### 14.3 Control flow

```yaml
- id: classification_branch
  type: decision
  spec:
    on: ${case.classification}
    cases:
      pii:        { next: pii_handling }
      confidential:{ next: confidential_handling }
    default:     { next: standard_handling }

- id: extract_in_parallel
  type: fork
  spec:
    branches:
      - [extract_documents, classify_documents]
      - [extract_entities]
      - [score_risk]

- id: join_extraction
  type: join
  spec:
    join_on: extract_in_parallel
    output:
      docs: ${tasks.extract_documents.output}
      entities: ${tasks.extract_entities.output}
      risk: ${tasks.score_risk.output}
```

### 15. Tipos de tarea soportados

#### 15.1 Tareas que ejecuta el engine (sin worker)

- **decision** — switch/case por expresión.
- **fork** — abre N branches paralelas.
- **join** — espera todas o N de M branches.
- **wait_for_time** — duerme hasta `at` o `after`. Implementado con TimerWheel + libs/event-scheduler.
- **wait_for_event** — espera evento Kafka con filtro. Engine subscribe.
- **set_variable** — asigna a `state_data`.
- **expression** — evalúa jq y guarda en out.
- **terminate** — termina el run con state succeeded/failed/cancelled.
- **sub_workflow** — invoca otro workflow; el run se marca como suspended hasta que el child termine.

#### 15.2 Tareas que requieren worker

- **http** — request HTTP arbitrario (con políticas de retry y timeout).
- **grpc** — request gRPC contra un servicio interno.
- **kafka_publish** — publica un mensaje a un topic.
- **ontology_action** — ejecuta una acción de la ontología (vía ontology-actions-service).
- **connector** — invoca un conector (data ingestion, webhook, etc.).
- **custom** — `task_type: custom.{name}`, lo recoge un worker registrado.

#### 15.3 Tareas humanas

- **human** — bloquea hasta respuesta humana. Asignado por user o role. Form opcional.

#### 15.4 Tareas saga / compensación

- **compensate** — invoca explícitamente compensation de tareas previas.

#### 15.5 Ejemplo de workflow completo

```yaml
name: pii_alert_review
version: 1
description: |
  Cuando llega una alerta de PII, recoge contexto, clasifica documentos,
  pide revisión a un analista senior, y publica resultado.
inputs:
  - { name: alert_id, type: string, required: true }
permissions:
  required: [alerts:read, cases:write]
tasks:
  - id: load_alert
    type: ontology_action
    spec:
      action: alert.read
      input: { id: ${input.alert_id} }
    out: alert

  - id: load_related_documents
    type: ontology_action
    spec:
      action: document.search
      input:
        filter:
          related_to: ${alert.entity_id}
          classification: ${alert.classification}
        limit: 50
    out: documents

  - id: extract_entities
    type: http
    spec:
      url: http://entity-resolution-service.of-data.svc:8080/api/v1/extract
      method: POST
      body:
        document_ids: ${documents.ids}
      headers:
        Authorization: "Bearer ${secrets.svc_token}"
    retry: { max_attempts: 5, backoff: { type: exponential, base_ms: 1000 } }
    timeout: 60s
    out: entities

  - id: score_risk
    type: http
    spec:
      url: http://risk-scoring-service.of-ml.svc:8080/api/v1/score
      method: POST
      body:
        alert_id: ${input.alert_id}
        entities: ${entities}
    timeout: 30s
    out: risk

  - id: high_risk_decision
    type: decision
    spec:
      on: ${risk.level}
      cases:
        critical: { next: senior_review }
        high:     { next: senior_review }
      default:    { next: auto_close }

  - id: senior_review
    type: human
    spec:
      assignee_role: senior_analyst
      form: pii_alert_review_v1
      form_data:
        alert: ${alert}
        documents: ${documents}
        entities: ${entities}
        risk: ${risk}
      timeout: 4h
      escalation:
        - role: manager
        - role: head_of_security
    out: review

  - id: review_decision
    type: decision
    spec:
      on: ${review.outcome}
      cases:
        confirmed: { next: open_case }
        false_positive: { next: dismiss_alert }
        needs_more_info: { next: extract_entities }   # loop
      default: { next: dismiss_alert }

  - id: open_case
    type: ontology_action
    spec:
      action: case.create
      input:
        title: "PII alert: ${alert.title}"
        priority: high
        entities: ${entities.ids}
        evidence:
          alert_id: ${input.alert_id}
          review_id: ${review.id}
    out: case

  - id: notify
    type: kafka_publish
    spec:
      topic: case.opened.v1
      key: ${case.id}
      payload:
        case_id: ${case.id}
        alert_id: ${input.alert_id}
        opened_by: senior_review
      headers:
        x-correlation-id: ${run.correlation_id}

  - id: dismiss_alert
    type: ontology_action
    spec:
      action: alert.dismiss
      input:
        id: ${input.alert_id}
        reason: ${review.outcome}
        dismissed_by: ${review.responded_by}

  - id: auto_close
    type: ontology_action
    spec:
      action: alert.dismiss
      input:
        id: ${input.alert_id}
        reason: low_risk_auto

on_failure:
  - id: rollback_case
    type: compensate
    spec:
      tasks: [open_case]                          # LIFO compensation
  - id: notify_failure
    type: kafka_publish
    spec:
      topic: workflow.failed.v1
      payload: { run_id: ${run.id}, error: ${run.error} }
```

### 16. Modelo de ejecución y semántica

#### 16.1 Lifecycle de un run

1. `POST /api/v1/flow/runs` con `{ name, version, input }`.
2. **Authorization**: Cedar evalúa si el principal puede ejecutar este workflow con este input. Si no, 403.
3. Se inserta en `flow.runs` (state=pending, version=1).
4. Engine DAGExecutor encuentra el primer task; lo inserta en `flow.task_runs` (state=scheduled).
5. Se publica un evento `flow.events.v1` (`run.started`) vía outbox.
6. Si la tarea es de engine (decision, fork…), engine la ejecuta inline y avanza.
7. Si la tarea es de worker, se publica a NATS subject `flow.tasks.{task_type}`.
8. Worker hace poll, reclama (UPDATE state=claimed, claimed_by={id}, deadline_at=now()+timeout), ejecuta, y POST `/complete` o `/fail`.
9. Engine procesa la respuesta, avanza el DAG, repite.
10. Cuando no quedan tareas pendientes, run pasa a state=succeeded.
11. Se publica `run.completed` vía outbox.

#### 16.2 Semántica de errores

- Cada task tiene `max_attempts` (default 3). Si falla, attempt incrementa y se re‑schedule con backoff.
- Si agota intentos: task pasa a state=failed.
- Engine evalúa `on_failure` del workflow. Si hay compensaciones, se ejecutan en LIFO.
- Run pasa a state=failed con `error`.

#### 16.3 Replay y resume

- **Replay** de un run desde la última task succeeded: `POST /api/v1/flow/runs/{id}/resume`. Útil cuando un fallo era transitorio.
- **Time travel** (consultar estado en T pasado): la tabla `flow.events` tiene timestamp; se puede reconstruir.
- **No replay determinista de código** (vs Temporal): el código de los workers no necesita ser puro. La reconciliación es por idempotency key + state machine.

#### 16.4 Cancelación

- `POST /api/v1/flow/runs/{id}/cancel` con razón.
- Engine marca state=cancelled, marca todas las tasks pendientes como cancelled, dispara compensaciones según `on_cancel` (similar a `on_failure`).

### 17. Worker SDKs (Go, Python, TypeScript)

#### 17.1 Go SDK (libs/flow-sdk-go)

```go
package flow

type Worker struct {
    name        string
    apiURL      string
    authToken   string
    pollInterval time.Duration
    concurrency int
    handlers    map[string]Handler
}

type Handler func(ctx context.Context, in Input) (Output, error)

type Input struct {
    RunID         string
    TaskID        string
    Attempt       int
    Payload       []byte
    CorrelationID string
}

func NewWorker(name string, opts Options) *Worker { /* ... */ }

func (w *Worker) Register(taskType string, h Handler) { /* ... */ }

func (w *Worker) Run(ctx context.Context) error {
    // 1. Connect to NATS
    // 2. Subscribe to flow.tasks.{taskType} for each registered handler
    // 3. On message: respect bulkhead semaphore, claim via API,
    //    execute handler, complete/fail via API
    // 4. Heartbeat each task every 5s
    // 5. Graceful shutdown on ctx.Done()
}

// Uso:
func main() {
    w := flow.NewWorker("classify-documents-worker", flow.Options{
        APIURL:      "http://flow-orchestrator-service.of-platform.svc:8080",
        Concurrency: 16,
    })

    w.Register("classify_documents", func(ctx context.Context, in flow.Input) (flow.Output, error) {
        var args struct{ DocumentIDs []string `json:"document_ids"` }
        in.Bind(&args)

        // Trabajo real…
        result := classify(ctx, args.DocumentIDs)

        return flow.Output{"classifications": result}, nil
    })

    if err := w.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

#### 17.2 Python SDK (sdks/python/openfoundry-flow)

```python
from openfoundry_flow import Worker, Input, Output
import asyncio

w = Worker(
    name="classify-documents-worker",
    api_url="http://flow-orchestrator-service.of-platform.svc:8080",
    concurrency=16,
)

@w.task("classify_documents")
async def classify(input: Input) -> Output:
    args = input.bind()
    result = await classify_async(args["document_ids"])
    return Output(classifications=result)

if __name__ == "__main__":
    asyncio.run(w.run())
```

#### 17.3 TypeScript SDK (sdks/typescript/@openfoundry/flow)

```typescript
import { Worker, Input, Output } from '@openfoundry/flow';

const w = new Worker({
  name: 'classify-documents-worker',
  apiUrl: 'http://flow-orchestrator-service.of-platform.svc:8080',
  concurrency: 16,
});

w.register(
  'classify_documents',
  async (input) => {
    const args = input.payload;
    const classifications = await classify(args.document_ids);
    return { classifications };
  }
);

await w.run();
```

#### 17.4 Características comunes de los SDKs

- **Long-poll**: el SDK conecta a NATS JetStream con un consumer durable y AckExplicit. No es polling HTTP en bucle.
- **Bulkhead semaphore**: configurable concurrency limit.
- **Heartbeat**: cada 5s mientras la task corre, para que el reaper no la tome.
- **Idempotency key**: el SDK lo expone al handler; el handler usa `libs/idempotency` para record‑before‑process.
- **OTel tracing**: el SDK extrae `traceparent` del header NATS y crea un span hijo.
- **Graceful shutdown**: en ctx.Done(), termina las tasks en curso y cierra la subscripción.
- **Error categorization**: el handler puede lanzar `flow.RetryableError` (retry) o `flow.FatalError` (no retry).

### 18. Versionado de definiciones

Critical para Foundry parity. Los workflows en producción no se pueden romper cuando un usuario edita la definición.

**Modelo:**

- Cada definition tiene `(name, version)`. Version es un integer monotónico.
- Una nueva edición de un workflow no muta la fila — crea una nueva fila con `version+1`.
- Los runs activos guardan `definition_id` (referencia inmutable a la versión específica).
- Status `active`/`deprecated`/`archived`:
  - `active`: nuevos runs usan esta versión por defecto.
  - `deprecated`: nuevos runs requieren flag explícito.
  - `archived`: nuevos runs prohibidos; runs activos siguen.
- API para promover una versión: `POST /api/v1/flow/definitions/{name}/versions/{n}/activate`.

**Migración entre versiones (cambios breaking):**

- Si una nueva versión añade un input requerido sin default, runs en curso de la versión anterior **no se migran**. Siguen con su versión.
- Si se quiere migrar, hay un endpoint explícito `POST /api/v1/flow/runs/{id}/migrate?to_version=2` que valida compatibilidad y puede fallar.

### 19. Idempotencia, retries, compensaciones, sagas

#### 19.1 Idempotencia

- Cada task run tiene `idempotency_key = sha256(run_id || task_id || attempt)`.
- El SDK la pasa al handler.
- El handler puede usar `libs/idempotency` para asegurar que side effects se aplican exactamente una vez incluso si el ack al engine se pierde y la task se re‑schedule.

#### 19.2 Retries

- Configurable per task con backoff exponencial + jitter (defaults razonables).
- `max_attempts` finito.
- Errores `RetryableError` cuentan; `FatalError` mata la task inmediatamente.

#### 19.3 Compensaciones

- Cada task tipo HTTP/gRPC/etc. puede declarar un `compensate` block en el spec.
- Si el run falla después de que esta task succeeded, el engine ejecuta la compensación.

```yaml
- id: charge_card
  type: http
  spec:
    url: http://payment-service/charge
    body: { amount: 100, card: ${card} }
  compensate:
    type: http
    spec:
      url: http://payment-service/refund
      body: { transaction_id: ${tasks.charge_card.output.tx_id} }
```

#### 19.4 Sagas

- El nodo `compensate` global en `on_failure` ejecuta saga LIFO usando `libs/saga`.
- Cada compensation se persiste como un task_run con `task_type=compensation` para trazabilidad.

### 20. Human-in-the-loop

#### 20.1 Asignación

- Por usuario específico (`assignee_id`).
- Por rol (`assignee_role`); cualquier usuario con ese rol puede tomarla.
- Por queue (`assignee_queue`); funciona como un pool donde múltiples usuarios pueden tomar.

#### 20.2 Forms

- Referencia a un form_id en `form-registry-service` (servicio nuevo o función de application-composition-service).
- Forms son JSON Schema validados.
- form_data permite pre‑fill.

#### 20.3 Timeouts y escalación

- Cada human task tiene `timeout` (default 24h).
- Si no hay respuesta en ese tiempo, se ejecuta `escalation`:
  - Reasignación al siguiente en `escalation_chain`.
  - Notificación vía `notification-alerting-service`.
  - Si todos los niveles agotan, task falla.

#### 20.4 Endpoint de respuesta

```http
POST /api/v1/flow/human/{task_id}/respond
Authorization: Bearer …
Content-Type: application/json

{
  "outcome": "confirmed",
  "comment": "PII verificado y caso abierto",
  "form_data": { ... }
}
```

Engine valida que el principal puede responder (Cedar), aplica la respuesta como output, y avanza el DAG.

### 21. Observabilidad y auditoría

#### 21.1 Métricas Prometheus

```
of_flow_runs_total{tenant, definition, version, state}
of_flow_run_duration_seconds{tenant, definition, version}  # histogram
of_flow_tasks_total{tenant, task_type, state}
of_flow_task_duration_seconds{tenant, task_type}            # histogram
of_flow_task_retries_total{tenant, task_type}
of_flow_human_tasks_pending{tenant, role}                   # gauge
of_flow_human_tasks_overdue{tenant, role}                   # gauge
of_flow_workers_connected{worker_name}                      # gauge
of_flow_outbox_lag_seconds                                  # gauge
of_flow_engine_leader{instance}                             # gauge (1 leader, 0 follower)
of_flow_schedule_drift_seconds{tenant, schedule}            # histogram
```

#### 21.2 Tracing

- Cada run inicia un trace span.
- Cada task es un span hijo.
- `traceparent` se propaga a workers via NATS headers.
- HTTP / gRPC tasks propagan a través de cabeceras estándar.
- Visibilidad end‑to‑end en Tempo.

#### 21.3 Audit

Cada transición publica un evento `flow.events.v1` vía outbox:

```json
{
  "event_id": "{deterministic uuid v5}",
  "kind": "task.completed",
  "run_id": "...",
  "task_run_id": "...",
  "task_type": "http",
  "tenant": "...",
  "actor": { "id": "...", "type": "service" },
  "occurred_at": "...",
  "trace_id": "...",
  "span_id": "..."
}
```

audit-sink consume y materializa a Iceberg `of_audit.flow_events` con retention 10 años. Investigación post‑hoc completa.

### 22. Seguridad y políticas (Cedar)

Tres puntos de evaluación:

1. **Run start.** `Permit (principal, action: "Flow::Run", resource: Flow::"{name}:{version}") when …`
   - Política puede restringir qué workflows un user puede ejecutar.
   - Política puede restringir input (e.g., solo tu tenant).

2. **Human task respond.** `Permit (principal, action: "FlowHumanTask::Respond", resource: HumanTask::"{id}")`
   - Verifica que el principal está en el rol/usuario asignado.
   - Verifica que el principal tiene clearance para ver el form_data.

3. **Worker claim.** El worker se autentica vía service account JWT. Cedar valida que ese service account puede ejecutar ese task_type.

Schema Cedar adicional:

```cedar
entity Flow in [Tenant] {
    name: String,
    version: Long,
    permissions_required: Set[String]
};

entity FlowRun in [Flow, Tenant] {
    state: String,
    started_by: User,
    classification: String
};

entity HumanTask in [FlowRun] {
    assignee_id: User,
    assignee_role: Role,
    classification: String
};

action "Flow::Run" appliesTo {
    principal: [User, ServicePrincipal],
    resource: [Flow]
};

action "FlowHumanTask::Respond" appliesTo {
    principal: [User],
    resource: [HumanTask]
};
```

### 23. Despliegue en OpenShift

#### 23.1 Topología

- **flow-orchestrator-service**: 3 réplicas, podAntiAffinity por hostname, leader via Postgres advisory lock. CNPG cluster compartido (`pg-runtime-config` con namespace `flow.*`).
- **Workers**: cada uno es un Deployment normal, escalable con HPA por CPU o KEDA por queue depth en NATS JetStream consumer pending.
- **NATS**: cluster ya existente. Subjects nuevos `flow.tasks.>`, `flow.events.>` con stream `OF_FLOW_TASKS` (JetStream, 7d retention).

#### 23.2 Helm chart

Sigue patrón de `_shared/templates/helpers.tpl`:

```yaml
# infra/helm/apps/of-platform/values.yaml (delta)
services:
  flow-orchestrator-service:
    image: { repository: openfoundry/flow-orchestrator, tag: 0.1.0 }
    replicas: 3
    podDisruptionBudget: { enabled: true, minAvailable: 2 }
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app.kubernetes.io/name: flow-orchestrator-service
            topologyKey: kubernetes.io/hostname
    resources:
      requests: { cpu: 500m, memory: 512Mi }
      limits:   { cpu: 2,    memory: 2Gi }
    env:
      - { name: PG_DSN_SECRET, valueFrom: { secretKeyRef: { name: of-flow-pg, key: dsn } } }
      - { name: NATS_URL, value: nats://nats-cluster.of-infra.svc:4222 }
      - { name: KAFKA_BROKERS, value: kafka-cluster-kafka-bootstrap.of-infra.svc:9092 }
```

Workers ejemplo:

```yaml
services:
  flow-worker-classify-documents:
    image: { repository: openfoundry/worker-classify-documents, tag: 0.1.0 }
    replicas: 4
    autoscaling:
      enabled: true
      minReplicas: 2
      maxReplicas: 16
      keda:
        triggers:
          - type: nats-jetstream
            metadata:
              natsServerMonitoringEndpoint: nats-cluster:8222
              account: $G
              stream: OF_FLOW_TASKS
              consumer: flow-tasks-classify-documents
              lagThreshold: "10"
```

### 24. Diferencias frente a Conductor OSS

| Dimensión | Conductor OSS | of-flow |
|---|---|---|
| Backend de persistencia | Redis (original), Cassandra/Postgres (modern) | Postgres (CNPG) único |
| Distribución a workers | HTTP polling | NATS JetStream pull con AckExplicit |
| DSL | JSON | YAML/JSON con DSL extensible |
| UI | Built-in (Angular) | Integrada en apps/web (SvelteKit) |
| Authn/Authz | Plugin / external | Cedar nativo |
| Audit | Logs + metrics | outbox → Kafka → Iceberg (10y) |
| Time travel queries | No | Sí, via flow.events table |
| OpenTelemetry | Plugin | Nativo, traceparent en NATS headers |
| Idempotency primitive | A cargo del worker | Built-in, idempotency_key inyectado |
| Compensación / saga | manual | Declarativa en DSL + integración libs/saga |
| Schedule recurrente | Conductor Schedules | Postgres + libs/scheduling-cron + libs/event-scheduler |
| Versionado de defs | Sí, integer | Sí, integer + status (active/deprecated/archived) |
| Sub-workflows | Sí | Sí |
| Dynamic fork | Sí | Sí (siguiente versión) |
| Human tasks | Limited | First-class, con forms y escalación |
| Multi-tenant | Sí | Sí, con isolation por tenant_id |
| Métricas | Prometheus exportable | Prometheus nativo + USE/RED |

---

## Parte III — ADR propuesto y roadmap

### 25. Borrador de ADR-0043: of-flow user-facing workflow engine

**Status:** Proposed
**Date:** 2026-05-08
**Supersedes:** N/A
**Superseded by:** N/A
**Related:** ADR-0037 (Foundry-pattern orchestration), ADR-0011 (control vs data bus), ADR-0027 (Cedar policy engine)

#### Context

ADR-0037 retiró Temporal y consolidó la orquestación interna del sistema sobre Postgres state machines + outbox + Kafka choreography ("Foundry-pattern"). Esa decisión es sólida y se mantiene. Sin embargo, hay un caso de uso que Foundry-pattern no cubre por diseño: **workflows definidos por usuarios finales** (analistas, investigadores, power users) que necesitan diseñar y monitorizar procesos automatizados en una UI, equivalente a la capacidad de Foundry "Automate" o de Conductor OSS / Camunda.

Estos workflows tienen requirements distintos:

- Definición declarativa (no en código).
- Versionado y promoción entre entornos.
- Visibilidad en UI con monitoring en vivo.
- Tareas heterogéneas: HTTP/gRPC a microservicios, decisiones, esperas, fork/join, llamadas a la ontología, acciones humanas.
- Compensaciones declarativas.

Las opciones consideradas:

1. **Reusar Foundry-pattern**. Rechazado: no tiene UI, no tiene DSL, exige código Go por workflow.
2. **Adoptar Conductor OSS as-is**. Considerado y descartado por: backend Redis no encaja con CNPG already in use; auth no integra Cedar; audit no integra outbox+Iceberg pattern.
3. **Adoptar Argo Workflows**. Rechazado: orientado a batch ML/ETL en K8s con Pod por step, overhead enorme para tareas ligeras user-facing.
4. **Volver a Temporal**. Rechazado por las razones de ADR-0037.
5. **Construir of-flow propio inspirado en Conductor**. Aceptado.

#### Decision

Construir **of-flow**, un motor de workflows propio inspirado en Conductor OSS pero integrado nativamente con la arquitectura existente:

- Backend Postgres (CNPG cluster compartido).
- Distribución a workers via NATS JetStream.
- DSL declarativo YAML/JSON validado por JSON Schema.
- Autorización vía Cedar.
- Auditoría vía outbox → Kafka → Iceberg.
- SDKs en Go, Python, TypeScript.
- UI integrada en apps/web.
- Reuso de libs/state-machine, libs/saga, libs/outbox, libs/idempotency, libs/event-scheduler, libs/scheduling-cron.

of-flow **coexiste con** Foundry-pattern. No lo sustituye.

#### Consequences

**Positivas:**

- Capacidad clave de Foundry/Gotham (Automate) cubierta.
- Reuso máximo de primitivas existentes — coste de desarrollo mucho menor que un motor desde cero.
- Cero introducción de runtime nuevo: Postgres, NATS, Kafka, todos ya en cluster.
- Auditoría y trazabilidad consistentes con resto del sistema.
- DSL versionado permite "deploy de proceso" como artefacto de primera clase, paralelo a "deploy de código".

**Negativas:**

- Construir un motor de workflows es trabajo no trivial. Estimación: 12-18 sprints distribuidos entre 2-3 ingenieros.
- Mantener un motor propio es coste recurrente. Mitigación: el surface API es estable (estilo Conductor) y la lógica reusa primitivas que ya operamos.
- UI de diseñador es inversión separada. Mitigación: empezar con DSL YAML versionado en Git (GitOps friendly), UI como fase 2.
- Riesgo de divergencia con expectativas de mercado (Conductor, Camunda, Temporal). Mitigación: documentar diferencias y mantener interop por exportación a JSON estándar.

**Neutras:**

- Add otro componente operacional (`flow-orchestrator-service` + workers). Compensado por que no introduce nuevo runtime.

#### Implementation plan

Ver §26 (roadmap por fases).

### 26. Roadmap por fases (12 meses)

Roadmap para of-flow + adopciones P0/P1 de la auditoría. Los frentes corren en paralelo donde el equipo lo permita.

#### Fase A — Plataforma operativa (meses 1-2)

Adopciones de auditoría P0. Sin esto no hay producción.

- A1. ESO + Vault (1-2 sprints).
- A2. Tempo + Loki + Alertmanager routing (2-3 sprints).
- A3. Ceph CRUSH map (1 sprint).
- A4. ArgoCD + app-of-apps (2 sprints).
- A5. Iceberg maintenance CronJobs (1-2 sprints).
- A6. NetworkPolicies completas (1 sprint).
- A7. SBOM + cosign + gosec en CI (1-2 sprints).
- A8. golang-migrate (1-2 sprints).
- A9. libs/resilience (1 sprint).
- A10. W3C traceparent en headers Kafka/NATS (1 sprint).
- A11. podAntiAffinity duro + readOnlyRootFilesystem true (1 sprint).
- A12. Tests IDOR/escalada (1 sprint).

#### Fase B — Cierre de stubs y producto (meses 2-4)

- B1. Cerrar stubs P0 listados en STUB-AUDIT.md (4-6 sprints).
- B2. Cedar enforcement runtime en object-database-service (2-3 sprints).
- B3. Apicurio Registry como gate (2 sprints).
- B4. Reducción retention Kafka audit + ADR formal (1 sprint).

#### Fase C — Ontología avanzada (meses 4-7)

- C1. Bitemporalidad (valid_from/valid_to + history projection) (4-6 sprints).
- C2. Modelo de propiedad enriquecido (source/confidence/evidence) (4 sprints).
- C3. ER merge/split histórico integrado en Object base (2 sprints).
- C4. Versionado de la ontología + migrations seguras (2 sprints).

#### Fase D — of-flow MVP (meses 6-9, paralelo con C)

- D1. Schema Postgres + repositorios Go (1 sprint).
- D2. flow-orchestrator-service (HTTP API + DAGExecutor + TimerWheel) (3 sprints).
- D3. Worker SDK Go (1 sprint).
- D4. Tipos de tarea engine (decision, fork, join, wait, expression, set_variable) (1 sprint).
- D5. Tipos de tarea worker (http, grpc, kafka_publish, ontology_action) (2 sprints).
- D6. Schedule ticker + libs/scheduling-cron integration (1 sprint).
- D7. Cedar guards en run start + human respond (1 sprint).
- D8. Audit outbox + observabilidad (1 sprint).
- D9. Tests E2E con todos los tipos de tarea (1 sprint).
- D10. ADR-0043 finalizado y mergeado.

#### Fase E — of-flow con humanos y UI (meses 9-12)

- E1. Human tasks + escalation chain + form-registry-service (3 sprints).
- E2. UI designer en apps/web (drag-and-drop + YAML preview) (4-6 sprints).
- E3. UI monitor con SSE en vivo (2 sprints).
- E4. Worker SDK Python (1 sprint).
- E5. Worker SDK TypeScript (1 sprint).
- E6. Sub-workflows + dynamic fork (2 sprints).
- E7. Catálogo de workflows reutilizables (1 sprint).

#### Fase F — Endurecimiento operacional (mes 12+)

- F1. Activación región B Kafka MM2 + DR drill (2 sprints).
- F2. Chaos Mesh suite mínima nightly (1 sprint).
- F3. Restore drills calendarizados (recurrente).
- F4. ADRs formales: Vespa vs OpenSearch, Ceph vs Ozone (1 sprint cada).

#### Fase G — Foundry parity (12-18 meses)

- G1. case-management-service (4-6 sprints).
- G2. Time travel UI + replay ontológico (3 sprints).
- G3. Explicabilidad de inferencias y RAG con permisos (4 sprints).
- G4. App composition + SDK declarativo (4 sprints).

### 27. Riesgos y mitigaciones

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| of-flow MVP se alarga > 9 meses | Media | Alto | Scope estricto: tipos de tarea built-in mínimos en MVP; UI designer es fase 2 |
| Postgres se convierte en bottleneck para of-flow | Baja | Medio | Cluster CNPG separado si > 500 runs/s; particionado de tablas por tenant |
| NATS JetStream pierde tareas en partitioning | Baja | Alto | AckExplicit + reaper + idempotency garantizan at-least-once; tests caos |
| DSL diverge de necesidades reales tras E2 | Media | Medio | Iterar con 3-5 workflows piloto antes de UI fase E2 |
| Adopción de of-flow es lenta porque users prefieren código | Media | Medio | SDK Go/Py/TS para "code as workflow"; DSL es opcional |
| Foundry-pattern y of-flow se confunden en discusiones | Alta | Bajo | Documentación clara desde día 1; este documento es el canon |
| Coste de mantenimiento del motor propio crece | Media | Alto | Surface API estable; reuso de libs reduce blast radius; tests E2E exhaustivos |
| Bitemporalidad rompe queries existentes | Media | Alto | Default: queries actuales = "as-of NOW()"; nuevas queries opt-in |
| Cedar enforcement runtime degrada latencia | Baja | Alto | Cache de decisiones por (principal, action, resource_type) con TTL 30s |

### 28. Checklist final

#### Adopciones inmediatas (P0)

- [ ] External Secrets Operator + Vault Transit en producción.
- [ ] Tempo + Loki desplegados; Alertmanager con rutas a PagerDuty/Slack.
- [ ] Ceph CRUSH map con failureDomain por chassis Synergy.
- [ ] 9 stubs P0/P1 cerrados (orden indicado en §2.4).
- [ ] CronJobs Iceberg (compaction, expire, orphan, manifests).
- [ ] Cedar evaluado en runtime de object-database-service y similares.
- [ ] valid_from/valid_to + history projection en ontología.
- [ ] ArgoCD app-of-apps con sync waves operations→infra→apps.

#### Adopciones antes de producción (P1)

- [ ] libs/resilience con timeouts + circuit breakers + bulkheads.
- [ ] Apicurio Registry como gate de schemas en CI y runtime.
- [ ] NetworkPolicies completas con allowFrom/allowTo por servicio.
- [ ] golang-migrate en cada servicio con tabla schema_migrations.
- [ ] SBOM + cosign + gosec + trivy en CI; admission verifyImages.
- [ ] W3C traceparent + tracestate en cabeceras Kafka/NATS.
- [ ] Tests IDOR / privilege escalation explícitos.
- [ ] podAntiAffinity duro en componentes críticos.
- [ ] readOnlyRootFilesystem: true por defecto.
- [ ] Activación región B Kafka MM2 (staging) + DR drill semestral.
- [ ] Reducción retention Kafka audit.events.v1 a 30 días.

#### of-flow

- [ ] ADR-0043 mergeado.
- [ ] Schema Postgres flow.* deployed.
- [ ] flow-orchestrator-service v0.1 con tipos de tarea engine.
- [ ] Worker SDK Go publicado en libs/flow-sdk-go.
- [ ] Tipos de tarea worker: http, grpc, kafka_publish, ontology_action.
- [ ] Cedar guards en run start y human respond.
- [ ] Audit a flow.events.v1 → Iceberg.
- [ ] 3-5 workflows piloto en producción.
- [ ] UI designer (fase E).
- [ ] SDKs Python y TypeScript.

#### Decisiones formalizadas (mantenidas)

- [ ] ADR formal Vespa vs OpenSearch — conclusión: Vespa.
- [ ] ADR formal Ceph vs Ozone — conclusión: Ceph con CRUSH.
- [ ] ADR formal No StarRocks — conclusión: Trino + DataFusion + materializaciones, reevaluar si SLO menor que 200ms p99 con mas de 100M filas.
- [ ] ADR formal No Valkey — conclusión: Redis solo rate-limit; Cassandra + L1 in-process basta.

---

## Cierre

Esta auditoría tiene tres lecturas honestas, según el rol del lector:

- **Para un comité técnico**: el sistema está mucho mejor diseñado de lo que un revisor superficial diría. Los 42 ADRs, el split control/data bus, Foundry-pattern, Cedar, outbox+Debezium, son decisiones de calidad. Lo que falta es operar en producción lo que se ha diseñado.

- **Para el equipo de ingeniería**: hay 8 frentes P0 que cerrar antes de cualquier go‑live. La mayor parte del trabajo es **enchufar piezas que ya están construidas** (Vault signer, Cedar, JWKS rotation, sinks de Iceberg, of-flow primitives existentes en libs). No es trabajo desde cero.

- **Para el negocio**: el sistema está a 6-9 meses de un MVP comparable a un Foundry/Gotham reducido. Lo que diferencia entre "alternativa OSS válida" y "decoración de slides" es: bitemporalidad real, Cedar enforced en cada read, of-flow operativo, lakehouse con mantenimiento, y un DR probado.

Las decisiones que **no cambiamos** son tan importantes como las que sí. Vespa, Ceph, no-StarRocks, no-Valkey, no-Temporal, single Go module, NATS+Kafka split, Lakekeeper+iceberg-catalog-service, Trino reintroducido, Cedar — todas son decisiones tomadas con razón técnica y soportarlas frente a un auditor externo es parte del trabajo.

El motor of-flow no compite con Foundry-pattern. Lo complementa. La distinción entre "orquestación interna del sistema" (Foundry-pattern) y "orquestación user‑facing" (of-flow) es la clave para que ambas piezas convivan sin redundancia.

Lo que se construye es una plataforma open source que, ejecutada con rigor, puede sostener comparación con sistemas comerciales de inteligencia operacional. Lo que se evita es construir el doble de lo que hace falta porque sí.
