# Foundry Compute Modules 1:1 parity checklist

Date: 2026-05-13
Scope: public-docs-based parity plan for OpenFoundry's Compute Modules
surfaces: project/folder-managed compute module resources, Docker image and
container configuration, function execution mode, pipeline execution mode,
custom client protocol, SDK-backed function schema inference, OpenAPI-backed
server function detection, Functions CLI-compatible build metadata, artifact
repository image publishing, replicas, multi-container replicas, resource
configuration, horizontal autoscaling, min/max replicas, concurrency limits,
zero-scale behavior, logs, deployment history, source/egress configuration,
secrets and tokens, Workshop and Slate function consumption, data integration
pipelines for streams/datasets/media sets, custom model hosting, usage metering,
resource queues, audit, permissions, and production-readiness guardrails for
containerized code running inside OpenFoundry.

This document is intentionally implementation-oriented. It does not attempt to
clone Palantir branding, private source code, proprietary assets, screenshots,
or any non-public behavior. The target is **functional parity based on public
Palantir Foundry documentation**: the same product concepts, comparable compute
module authoring and deployment workflows, compatible resource models where
useful, and OpenFoundry-native implementation details that can be tested
locally.

## Parity scope boundary

All checklist work is governed by the
[Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).
OpenFoundry may implement behavior described in public Palantir documentation,
but contributors must not copy private source, decompile bundles, import
tenant-specific exports, use Palantir branding, or reuse proprietary assets.
The product target is functional parity in an OpenFoundry-native implementation,
not a pixel-perfect clone.

This checklist covers Compute Modules as a cross-platform container runtime
surface. It should integrate with the Functions checklist for container-backed
functions; with Workshop and Slate for interactive application calls; with
Pipeline Builder, Data Connection, Streams, Data Foundation, and Media Sets for
pipeline-mode ingestion and outputs; with Model Integration/Model Studio for
custom model hosting; with Code Workspaces/Code Repositories or external CI for
image builds; with Artifacts/container registry storage; with Security/Governance
for access, sources/egress, tokens, secrets, audit, and retention; and with
Resource Management for compute usage attribution. It should not replace native
OpenFoundry runtimes when a simpler native feature is already the intended path.

## Status vocabulary

| Status | Meaning |
| --- | --- |
| `todo` | Not implemented or not yet verified in OpenFoundry. |
| `partial` | Some surface exists, but behavior is incomplete or not wired end-to-end. |
| `blocked` | Requires a platform dependency, public documentation, or product decision. |
| `done` | Implemented, tested, documented, and verified through UI or API smoke tests. |

## Priority vocabulary

| Priority | Meaning |
| --- | --- |
| `P0` | Required for credible demo workflows that deploy a container-backed function and call it from an OpenFoundry app/API. |
| `P1` | Required for Foundry-style Compute Modules parity across functions, pipelines, scaling, logs, history, and sources. |
| `P2` | Advanced scale, governance, multi-container, custom-client, server/OpenAPI, usage, Marketplace, or production hardening parity. |

## Official Palantir documentation library

These public docs should be treated as the external behavioral contract while
implementing this checklist.

### Compute Modules overview and getting started

- [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview)
- [Compute Modules getting started](https://www.palantir.com/docs/foundry/compute-modules/get-started)
- [Compute Modules execution modes](https://www.palantir.com/docs/foundry/compute-modules/execution-modes/)
- [Compute Modules usage and pricing](https://www.palantir.com/docs/foundry/compute-modules/usage)
- [Compute Modules history](https://www.palantir.com/docs/foundry/compute-modules/history/)

### Configuration, containers, functions, and scaling

- [Containers](https://www.palantir.com/docs/foundry/compute-modules/containers/)
- [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/)
- [Scaling](https://www.palantir.com/docs/foundry/compute-modules/scaling)
- [Sources](https://www.palantir.com/docs/foundry/compute-modules/sources)

### Integrated Foundry surfaces

- [Functions overview](https://www.palantir.com/docs/foundry/functions/overview)
- [Workshop overview](https://www.palantir.com/docs/foundry/workshop/overview)
- [Slate overview](https://www.palantir.com/docs/foundry/slate/overview)
- [Pipeline Builder overview](https://www.palantir.com/docs/foundry/pipeline-builder/overview)
- [Streams core concepts](https://www.palantir.com/docs/foundry/data-integration/streams/)
- [Data Connection overview](https://www.palantir.com/docs/foundry/data-connection/overview)
- [Media sets overview](https://www.palantir.com/docs/foundry/media-sets-advanced-formats)
- [Model connectivity and development overview](https://www.palantir.com/docs/foundry/model-integration/overview/)
- [Artifacts overview](https://www.palantir.com/docs/foundry/artifacts/overview)
- [Dev toolchain overview](https://www.palantir.com/docs/foundry/dev-toolchain/overview)
- [Configure egress](https://www.palantir.com/docs/foundry/administration/configure-egress/)
- [Resource Management usage types](https://www.palantir.com/docs/foundry/resource-management/usage-types)

## Target OpenFoundry resource model

The implementation should define stable OpenFoundry-owned resources that can map
to public Foundry concepts without requiring Palantir RID formats. Compatibility
aliases may be accepted at service boundaries, but persisted state should use
OpenFoundry canonical IDs.

| Public Foundry concept | OpenFoundry resource target | Required notes |
| --- | --- | --- |
| Compute Module | `compute_module` | Project/folder-managed container runtime resource with execution mode, image/container config, replicas, scaling, functions/pipelines, logs, history, permissions, and usage. |
| Compute Module version | `compute_module_version` | Immutable or restorable configuration snapshot with image digests/tags, function specs, pipeline specs, resources, sources, scaling, and changelog. |
| Execution mode | `compute_module_execution_mode` | Function or pipeline mode with mode-specific validation, invocation semantics, and unsupported-operation messages. |
| Container image | `compute_module_image` | Artifact/container registry image reference with digest, tag, platform, labels, build provenance, compatibility findings, and vulnerability/security metadata where available. |
| Container configuration | `compute_module_container` | Per-container image, command/args, env vars, ports, resources, log capture, health, secret mounts, and entrypoint/client role. |
| Replica | `compute_module_replica` | Runtime instance containing one or more isolated containers with state, session ID, start time, resource allocation, logs, and termination reason. |
| Scaling policy | `compute_module_scaling_policy` | Min/max replicas, concurrency limit, zero-scale behavior, predictive scaling flags, cooldowns, and quota ceilings. |
| Resource profile | `compute_module_resource_profile` | CPU, memory, GPU, storage, resource queue, and per-container allocation rules used for usage and scheduling. |
| Function spec | `compute_module_function_spec` | Function name, API name, inputs, output, schema source, version behavior, and publication state. |
| Function invocation | `compute_module_function_invocation` | Request/response record for function-mode calls with caller, payload metadata, latency, errors, logs, and usage attribution. |
| OpenAPI server spec | `compute_module_openapi_spec` | Server-backed function detection manifest with local URL, paths, operation IDs, supported verbs, schema validation, and imported functions. |
| Pipeline spec | `compute_module_pipeline_spec` | Pipeline-mode input/output contract for streams, datasets, media sets, and autonomous container processing. |
| Pipeline run | `compute_module_pipeline_run` | Pipeline-mode execution record with input transactions/checkpoints, output resources, logs, status, duration, and retries. |
| Custom client spec | `compute_module_client_spec` | Protocol compatibility descriptor for non-SDK clients, polling behavior, job serialization, and heartbeat/error handling. |
| Source binding | `compute_module_source_binding` | External or in-platform connection/source allowed for a module, including egress policy, source resource, credentials, and audit. |
| Secret binding | `compute_module_secret_binding` | Runtime secret reference exposed to containers without leaking raw values into UI, logs, or exports. |
| Runtime log | `compute_module_runtime_log` | Captured stdout/stderr/file log stream with container, replica, retention, redaction, download, and search metadata. |
| Deployment history entry | `compute_module_history_entry` | Past deployment/configuration record with image, resource, scaling, function/pipeline, source, actor, and rollback metadata. |
| Usage record | `compute_module_usage_record` | Foundry compute-seconds-like usage attributed to module, version, replica, container, queue, project, function/pipeline, and caller. |
| Build artifact | `compute_module_build_artifact` | CLI/CI-produced build plan, function schema inference output, Docker build metadata, publish result, and registry coordinates. |
| Health signal | `compute_module_health_signal` | Readiness/liveness, queue depth, heartbeat, replica health, error rate, and autoscaling signal. |
| Audit event | `compute_module_audit_event` | Create, edit, build, publish, deploy, scale, invoke, run, source/secret update, log read, rollback, and delete governance event. |

## Milestone A: minimum viable Compute Modules parity

### Compute Module resource and image basics

- [ ] `CM.1` Compute Module CRUD and project placement (`P0`, `todo`)
  - Create, list, get, update metadata, move, duplicate, archive/delete, and restore Compute Module resources.
  - Support creation from a folder/project with function-mode or pipeline-mode selection.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Compute Modules getting started](https://www.palantir.com/docs/foundry/compute-modules/get-started).

- [ ] `CM.2` Execution mode model (`P0`, `todo`)
  - Persist function execution mode and pipeline execution mode with mode-specific validation and UI affordances.
  - Block querying pipeline-mode modules as functions and block pipeline input/output config for function-only modules.
  - Docs: [Compute Modules execution modes](https://www.palantir.com/docs/foundry/compute-modules/execution-modes/).

- [ ] `CM.3` Container image reference and compatibility validation (`P0`, `todo`)
  - Store image registry, repository, tag, digest, provenance, and compatibility findings.
  - Validate non-root numeric user, linux/amd64 platform, non-`latest` tags or digest use, and exposed port constraints where locally enforceable.
  - Docs: [Containers](https://www.palantir.com/docs/foundry/compute-modules/containers/).

- [ ] `CM.4` Single-container runtime configuration (`P0`, `todo`)
  - Configure command/args, environment variables, ports, resource profile, logging configuration, health metadata, and entrypoint/client role.
  - Redact secret-like environment values and route secrets through secret bindings.
  - Docs: [Containers](https://www.palantir.com/docs/foundry/compute-modules/containers/).

- [ ] `CM.5` Build and publish metadata ingest (`P0`, `todo`)
  - Accept CLI/CI-produced build plans and published image metadata, including function spec inference output where available.
  - Link builds to Code Repositories, Code Workspaces, external CI, or manual artifact uploads without requiring a specific toolchain.
  - Docs: [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/), [Artifacts overview](https://www.palantir.com/docs/foundry/artifacts/overview).

### Function execution mode MVP

- [ ] `CM.6` Function spec registration (`P0`, `todo`)
  - Register functions by UI/API with function name, API name, input schema, output schema, and description.
  - Support automatic schema inference metadata from SDK/CLI paths and manual JSON schema registration.
  - Docs: [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/).

- [ ] `CM.7` Function type mapping (`P0`, `todo`)
  - Support primitive, date/time, decimal, binary, array/list, map/dict, struct, optional, object reference, object set, media reference, and JSON-compatible payloads where OpenFoundry supports them.
  - Validate serialization/deserialization contracts between Foundry-like function types and container JSON payloads.
  - Docs: [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/), [Functions overview](https://www.palantir.com/docs/foundry/functions/overview).

- [ ] `CM.8` Function invocation dispatcher (`P0`, `todo`)
  - Dispatch requests to an active replica, package inputs as JSON, wait for response, handle errors/timeouts, and return typed outputs.
  - Capture request metadata, response metadata, latency, logs, and usage while redacting payloads by policy.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/).

- [ ] `CM.9` Workshop and Slate consumption path (`P0`, `todo`)
  - Expose compute module-backed functions to Workshop function-backed widgets/actions and Slate custom logic where locally supported.
  - Provide clear errors when a module is stopped, scaling from zero, unavailable, or missing permissions.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Workshop overview](https://www.palantir.com/docs/foundry/workshop/overview), [Slate overview](https://www.palantir.com/docs/foundry/slate/overview).

- [ ] `CM.10` Function testing UI/API (`P0`, `todo`)
  - Provide a Test tab/API for invoking registered functions with sample inputs, inspecting typed outputs, errors, and per-invocation logs.
  - Persist recent test runs with builder-visible retention controls.
  - Docs: [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/).

### Runtime, logs, and first deployment

- [ ] `CM.11` Replica lifecycle and job polling runtime (`P0`, `todo`)
  - Start/stop replicas, track startup/running/draining/failed states, expose heartbeat, and support a polling client protocol for jobs.
  - Ensure statelessness assumptions are explicit and no correctness relies on cross-replica local state.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview).

- [ ] `CM.12` Logs MVP (`P0`, `todo`)
  - Capture stdout/stderr and configured file logs per module/version/replica/container/invocation.
  - Provide log search, tail, download, retention, and redaction policy hooks.
  - Docs: [Containers](https://www.palantir.com/docs/foundry/compute-modules/containers/).

- [ ] `CM.13` Deployment history MVP (`P0`, `todo`)
  - Record every saved/deployed configuration snapshot with image, resources, functions, scaling, sources, actor, timestamp, and status.
  - Allow viewing prior deployments and comparing configuration changes.
  - Docs: [Compute Modules history](https://www.palantir.com/docs/foundry/compute-modules/history/).

- [ ] `CM.14` Permission and application access gates (`P0`, `todo`)
  - Gate module creation/deployment/invocation by app access, project roles, resource permissions, function permissions, and service user rules.
  - Provide explicit blocked states for missing application access or queue/source permissions.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Security overview](https://www.palantir.com/docs/foundry/security/overview/).

- [ ] `CM.15` Usage metering MVP (`P0`, `todo`)
  - Meter active/starting replica time and attribute usage to module, project, version, function, caller, resource queue, container resources, and run.
  - Show current running cost indicators and recent usage history.
  - Docs: [Compute Modules usage and pricing](https://www.palantir.com/docs/foundry/compute-modules/usage), [Resource Management usage types](https://www.palantir.com/docs/foundry/resource-management/usage-types).

## Milestone B: credible Foundry-style Compute Modules parity

### Pipeline execution mode and data integration

- [ ] `CM.16` Pipeline-mode input/output contract (`P1`, `todo`)
  - Define pipeline-mode inputs and outputs for streams, datasets, and media sets with schema, branch, transaction, and checkpoint metadata.
  - Support autonomous container processing where the module reads inputs and writes outputs according to the pipeline spec.
  - Docs: [Compute Modules getting started](https://www.palantir.com/docs/foundry/compute-modules/get-started), [Streams core concepts](https://www.palantir.com/docs/foundry/data-integration/streams/).

- [ ] `CM.17` Pipeline run lifecycle (`P1`, `todo`)
  - Trigger, monitor, retry, cancel, and inspect pipeline-mode runs with status, input transactions/checkpoints, output commits, logs, and errors.
  - Integrate with OpenFoundry build/schedule orchestration where appropriate.
  - Docs: [Compute Modules getting started](https://www.palantir.com/docs/foundry/compute-modules/get-started), [Builds core concepts](https://www.palantir.com/docs/foundry/data-integration/builds/).

- [ ] `CM.18` Stream ingestion and streaming output integration (`P1`, `todo`)
  - Let compute modules consume from and produce to streams with checkpointing, restart behavior, and backpressure controls.
  - Provide consistent delivery/error semantics with the Streaming/Data Connection checklist.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Streams core concepts](https://www.palantir.com/docs/foundry/data-integration/streams/).

- [ ] `CM.19` Dataset and media set output integration (`P1`, `todo`)
  - Commit tabular dataset outputs and media set outputs from pipeline-mode modules with schema/media metadata and lineage.
  - Validate output permissions, transaction semantics, and rollback behavior.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Media sets overview](https://www.palantir.com/docs/foundry/media-sets-advanced-formats).

- [ ] `CM.20` Data Connection source-backed custom connectors (`P1`, `todo`)
  - Use compute modules as custom data integration connectors for arbitrary sources, with source config, credentials, egress, and sync history.
  - Integrate with existing Data Connection source, sync, webhook, and export models rather than inventing parallel connector resources.
  - Docs: [Data Connection overview](https://www.palantir.com/docs/foundry/data-connection/overview), [Sources](https://www.palantir.com/docs/foundry/compute-modules/sources).

### Scaling, resources, and multi-container modules

- [ ] `CM.21` Horizontal autoscaling policy (`P1`, `todo`)
  - Configure minimum replicas, maximum replicas, concurrency limits, scale-to-zero behavior, cooldowns, and predictive/pre-warm hints.
  - Surface current desired/actual replica counts and why scaling decisions occurred.
  - Docs: [Scaling](https://www.palantir.com/docs/foundry/compute-modules/scaling).

- [ ] `CM.22` Resource profile and queue selection (`P1`, `todo`)
  - Configure CPU, memory, GPU, storage, and resource queue per module/container with validation against quotas and queue capabilities.
  - Recalculate usage estimates when resources or replica limits change.
  - Docs: [Compute Modules usage and pricing](https://www.palantir.com/docs/foundry/compute-modules/usage), [Resource Management usage types](https://www.palantir.com/docs/foundry/resource-management/usage-types).

- [ ] `CM.23` Multi-container replicas (`P1`, `todo`)
  - Support replicas that contain multiple isolated containers with one entrypoint/client container and optional sidecars.
  - Define intra-replica networking/shared-volume constraints and prohibit cross-replica state assumptions.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Containers](https://www.palantir.com/docs/foundry/compute-modules/containers/).

- [ ] `CM.24` Health and readiness signals (`P1`, `todo`)
  - Track readiness, liveness, heartbeat, startup failures, queue depth, error rate, and container exit reasons.
  - Feed health into routing, scaling, UI warnings, and deployment history.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Scaling](https://www.palantir.com/docs/foundry/compute-modules/scaling).

- [ ] `CM.25` Zero-scale and cold-start behavior (`P1`, `todo`)
  - Support min replicas of zero where configured, route first request through scale-up, and show cold-start status to callers.
  - Add timeout budgets and retry semantics for scale-up failures.
  - Docs: [Scaling](https://www.palantir.com/docs/foundry/compute-modules/scaling), [Compute Modules usage and pricing](https://www.palantir.com/docs/foundry/compute-modules/usage).

### Developer workflow and server/custom clients

- [ ] `CM.26` Functions CLI-compatible local workflow (`P1`, `todo`)
  - Provide a local/CI CLI or compatible API for static function inference, build-plan validation, image publishing, and deployment metadata upload.
  - Keep implementation OpenFoundry-native while preserving familiar build/publish concepts.
  - Docs: [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/), [Dev toolchain overview](https://www.palantir.com/docs/foundry/dev-toolchain/overview).

- [ ] `CM.27` SDK-backed function schema inference (`P1`, `todo`)
  - Support generated function specs from Python and TypeScript SDK annotations or manifests where locally implemented.
  - Validate inferred schemas against registered function types and publish them without manual editing.
  - Docs: [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/).

- [ ] `CM.28` OpenAPI server-backed functions (`P1`, `todo`)
  - Detect/import functions from an image-provided OpenAPI spec with operation IDs, supported verbs, parameter schemas, and single JSON response schemas.
  - Prevent manual divergence between imported function definitions and the image OpenAPI spec.
  - Docs: [Functions](https://www.palantir.com/docs/foundry/compute-modules/functions/).

- [ ] `CM.29` Custom client protocol compatibility (`P1`, `todo`)
  - Document and implement a stable polling/job protocol for unsupported languages and custom clients.
  - Include job lease, heartbeat, retry, cancellation, error, and result serialization semantics.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Compute Modules getting started](https://www.palantir.com/docs/foundry/compute-modules/get-started).

- [ ] `CM.30` Local development and emulator (`P1`, `todo`)
  - Provide a local harness that simulates function jobs, pipeline jobs, environment variables, tokens, source bindings, and log capture.
  - Make it usable in Code Workspaces, Code Repositories, and external developer environments.
  - Docs: [Compute Modules getting started](https://www.palantir.com/docs/foundry/compute-modules/get-started), [Dev toolchain overview](https://www.palantir.com/docs/foundry/dev-toolchain/overview).

## Milestone C: governance, custom model hosting, Marketplace, and production readiness

### Security, sources, and secrets

- [ ] `CM.31` Zero-trust source and egress policy integration (`P2`, `todo`)
  - Require explicit source/egress configuration before modules connect to external systems.
  - Bind sources from Data Connection and record policy decisions in audit/log metadata.
  - Docs: [Sources](https://www.palantir.com/docs/foundry/compute-modules/sources), [Configure egress](https://www.palantir.com/docs/foundry/administration/configure-egress/).

- [ ] `CM.32` Secret and token delivery (`P2`, `todo`)
  - Deliver credentials and service-user tokens to containers only at runtime through secret bindings or scoped tokens.
  - Prevent raw secrets from appearing in environment exports, logs, build metadata, or deployment history.
  - Docs: [Sources](https://www.palantir.com/docs/foundry/compute-modules/sources), [API authentication](https://www.palantir.com/docs/foundry/api/v2/general/overview/authentication/).

- [ ] `CM.33` Runtime identity and authorization (`P2`, `todo`)
  - Define whether modules run as project, service, or user-scoped identities for functions and pipelines.
  - Enforce downstream data, Ontology, media, stream, function, and model permissions for each request/run.
  - Docs: [Security overview](https://www.palantir.com/docs/foundry/security/overview/), [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview).

- [ ] `CM.34` Image security and supply-chain findings (`P2`, `todo`)
  - Track image digest pinning, provenance, signing status, vulnerability scan results, base-image policy, and unsupported image findings.
  - Gate deployment by policy and provide remediation guidance.
  - Docs: [Containers](https://www.palantir.com/docs/foundry/compute-modules/containers/), [Security overview](https://www.palantir.com/docs/foundry/security/overview/).

- [ ] `CM.35` Compute Module audit log (`P2`, `todo`)
  - Audit create, update, build, image publish, deploy, scale, invoke, run, log read, source/secret change, rollback, and delete events.
  - Include actor, service user, source IP where available, module/version, image digest, policy decisions, and affected resources.
  - Docs: [Audit logs overview](https://www.palantir.com/docs/foundry/security/audit-logs-overview), [Compute Modules history](https://www.palantir.com/docs/foundry/compute-modules/history/).

### Model hosting and advanced platform integration

- [ ] `CM.36` Custom model hosting (`P2`, `todo`)
  - Host custom/open-source models as compute module-backed functions with model metadata, health, inference history, and payload logging policy.
  - Integrate with Model Integration/Model Studio deployments rather than creating a separate model registry.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Model connectivity and development overview](https://www.palantir.com/docs/foundry/model-integration/overview/).

- [ ] `CM.37` Application-side latency and availability controls (`P2`, `todo`)
  - Provide client-visible timeout, retry, fallback, and degraded-mode behavior for Workshop, Slate, OSDK, and API callers.
  - Surface cold starts, queueing, replica exhaustion, and autoscaling events to application builders.
  - Docs: [Scaling](https://www.palantir.com/docs/foundry/compute-modules/scaling), [Workshop overview](https://www.palantir.com/docs/foundry/workshop/overview).

- [ ] `CM.38` Marketplace and DevOps packaging (`P2`, `todo`)
  - Package compute modules, images, functions, sources, pipeline specs, and resource profiles into Marketplace/DevOps products.
  - Validate destination registry, queues, egress policies, secrets, and input mappings during install/upgrade.
  - Docs: [Foundry DevOps overview](https://www.palantir.com/docs/foundry/foundry-devops/overview/), [Marketplace overview](https://www.palantir.com/docs/foundry/marketplace/overview).

- [ ] `CM.39` Branch-aware compute modules (`P2`, `todo`)
  - Let module configs, functions, pipeline specs, and consuming apps participate in branch proposals and previews.
  - Prevent branch previews from mutating production external systems unless side effects are explicitly allowed.
  - Docs: [Global Branching overview](https://www.palantir.com/docs/foundry/foundry-branching/overview/), [Managing side effects via actions on branches](https://www.palantir.com/docs/foundry/global-branching/side-effects-on-branches/).

- [ ] `CM.40` Disaster recovery and rollback (`P2`, `todo`)
  - Roll back to previous known-good image/config/function specs, drain bad replicas, and preserve deployment history/logs.
  - Support forced stop/delete cleanup for stuck replicas or failed pipeline runs.
  - Docs: [Compute Modules history](https://www.palantir.com/docs/foundry/compute-modules/history/).

### Production operations

- [ ] `CM.41` SLOs, alerts, and health dashboards (`P2`, `todo`)
  - Define and monitor latency, error rate, availability, replica health, queue depth, cold starts, and resource saturation.
  - Integrate alerts with Automate/Rules or OpenFoundry-native notification channels.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview), [Data Health](https://www.palantir.com/docs/foundry/observability/data-health/).

- [ ] `CM.42` Quotas and cost guardrails (`P2`, `todo`)
  - Enforce max replicas, max resource profile, max daily usage, per-project budgets, per-caller rate limits, and deployment approval thresholds.
  - Show cost previews when scaling policy or resource profile changes.
  - Docs: [Compute Modules usage and pricing](https://www.palantir.com/docs/foundry/compute-modules/usage), [Resource Management usage types](https://www.palantir.com/docs/foundry/resource-management/usage-types).

- [ ] `CM.43` Log retention and redaction (`P2`, `todo`)
  - Apply retention windows and redaction policies to stdout/stderr, file logs, request metadata, pipeline run logs, and downloaded exports.
  - Provide safe support bundles without secrets or protected payloads.
  - Docs: [Containers](https://www.palantir.com/docs/foundry/compute-modules/containers/), [Retention overview](https://www.palantir.com/docs/foundry/retention/overview/).

- [ ] `CM.44` Backpressure, retry, and dead-letter handling (`P2`, `todo`)
  - Define queue behavior for overloaded replicas, retriable failures, non-retriable failures, cancellation, and dead-letter payload metadata.
  - Make behavior consistent across function and pipeline execution modes.
  - Docs: [Scaling](https://www.palantir.com/docs/foundry/compute-modules/scaling), [Compute Modules execution modes](https://www.palantir.com/docs/foundry/compute-modules/execution-modes/).

- [ ] `CM.45` Unsupported-use guidance and native-feature alternatives (`P2`, `todo`)
  - Warn when users attempt dynamic vertical scaling, stateful cross-replica workflows, or replacements for simpler native OpenFoundry features.
  - Link to native Functions, Pipeline Builder, Data Connection, Model Integration, or AIP Logic alternatives.
  - Docs: [Compute Modules overview](https://www.palantir.com/docs/foundry/compute-modules/overview).

## Implementation inventory to collect before coding

- [ ] `INV.1` Identify existing OpenFoundry container/image registry, artifact repository, Docker build, and CI metadata components.
- [ ] `INV.2` Identify existing function package, function runtime, action/function invocation, Workshop/Slate function consumption, and OSDK/API call surfaces.
- [ ] `INV.3` Identify existing pipeline executor, stream service, dataset transaction service, media set service, Data Connection source/sync, and schedule/build orchestration components.
- [ ] `INV.4` Identify existing sidecar runtimes, Python runner, model serving adapters, LLM/model deployment, and external model connection components that can share compute-module infrastructure.
- [ ] `INV.5` Identify existing service-user, OAuth token, secret storage, egress/source policy, marking, restricted-view, audit, and retention components.
- [ ] `INV.6` Identify existing logs, metrics, health, tracing, usage metering, resource queues, and alerting components.
- [ ] `INV.7` Identify existing DevOps/Marketplace packaging, branching/proposal, project/folder resource, and permission inheritance components.
- [ ] `INV.8` Identify local development constraints for running Docker/containers in tests and decide which tests require integration tags.

## Suggested service boundaries

| Service or package | Responsibility |
| --- | --- |
| `compute-module-service` | Compute Module CRUD, versions, deployment history, function/pipeline specs, scaling config, source/secret bindings, and UI/API orchestration. |
| `compute-runtime-controller` | Replica lifecycle, scheduling, container startup/shutdown, health, job routing, backpressure, and autoscaling decisions. |
| `compute-function-gateway` | Function-mode invocation API, typed payload serialization, request routing, timeouts, retries, and per-invocation logs/metrics. |
| `compute-pipeline-runner` | Pipeline-mode run lifecycle, stream/dataset/media input/output adapters, checkpoints, output commits, and build/schedule integration. |
| `artifact-registry-service` | Image metadata, digest/tag validation, build provenance, registry credentials, and artifact repository integration. |
| `security-governance-service` | Application access, runtime identity, source/egress policy, secrets, markings, audit logs, retention, and policy enforcement. |
| `observability-service` | Logs, metrics, traces, health dashboards, alerts, usage records, and support bundle export. |
| `marketplace-service` | Compute module packaging, install/upgrade validation, destination queue/source/secret mapping, and product lifecycle. |
| `developer-toolchain` | CLI/build-plan compatibility, schema inference, local emulator, Code Repository/Workspace integration, and CI hooks. |

## Acceptance criteria for first complete Compute Modules milestone

- A builder can create a Compute Module resource in a project folder, select function execution mode, attach a compatible container image, configure resources, and deploy it.
- The module can register at least one typed function, invoke it from a test tab/API, and consume it from an OpenFoundry application or function gateway.
- The runtime starts at least one replica, routes invocations, captures logs, tracks health, and records deployment history.
- Usage is attributed to the module/version/replica/function/project with a Foundry compute-seconds-like model.
- Function invocation, logs, deployment changes, and source/secret changes are permission-aware and audited.
- A pipeline-mode proof of concept can read from an input stream or dataset and write to an output stream/dataset/media set with run history.
- Scaling configuration supports min/max replicas and concurrency limits with clear current-state and blocked/error explanations.

## Test plan expectations

- Unit tests for Compute Module resource validation, execution-mode validation, image compatibility findings, container config parsing, scaling policy validation, function schema validation, OpenAPI import validation, and usage calculation.
- API tests for module CRUD, version/history readback, deploy/rollback, function registration, function invocation, test runs, log reads, source binding, secret binding, and usage endpoints.
- Runtime tests for replica lifecycle, job polling, request routing, timeout/error handling, retries, health signals, cold-start behavior, and backpressure.
- Integration tests for Workshop/Slate function consumption, pipeline-mode stream/dataset/media adapters, Data Connection source-backed connectors, and custom model hosting wrappers.
- Security tests for project permissions, function invocation authorization, source/egress policy enforcement, secret redaction, log redaction, audit event completeness, and payload retention.
- Load tests for concurrent function invocations, autoscaling thresholds, replica churn, log volume, stream backpressure, and quota enforcement.
