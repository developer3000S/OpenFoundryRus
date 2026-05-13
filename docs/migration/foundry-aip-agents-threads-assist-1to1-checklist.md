# Foundry AIP Agents, Threads, and Assist 1:1 parity checklist

Date: 2026-05-13
Scope: public-docs-based parity plan for OpenFoundry's AIP agentic assistance
surfaces: AIP Agent Studio / AIP Chatbot Studio-style no-code agent files,
agent versions, instructions, model and temperature settings, retrieval context,
Ontology context, document context, citations, application state, action-taking
tools, object query tools, function tools, command tools, clarification tools,
agent sessions, streaming and blocking agent APIs, agents as Functions,
Workshop AIP Agent widget integration, OSDK and Developer Console handoffs,
AIP Threads ad-hoc conversations, document upload and document modes, thread
export, thread-to-agent upgrade, default agents, AIP Assist platform sidebar,
custom content source registration, custom source-backed Assist agents,
feedback, monitoring, usage, Marketplace packaging, branch/security handoffs,
and production-readiness guardrails for governed LLM assistants.

This document is intentionally implementation-oriented. It does not attempt to
clone Palantir branding, private source code, proprietary assets, screenshots,
or any non-public behavior. The target is **functional parity based on public
Palantir Foundry documentation**: the same product concepts, comparable agent,
thread, and assist workflows, compatible resource models where useful, and
OpenFoundry-native implementation details that can be tested locally.

## Parity scope boundary

All checklist work is governed by the
[Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).
OpenFoundry may implement behavior described in public Palantir documentation,
but contributors must not copy private source, decompile bundles, import
tenant-specific exports, use Palantir branding, or reuse proprietary assets.
The product target is functional parity in an OpenFoundry-native implementation,
not a pixel-perfect clone.

This checklist covers agentic AIP surfaces that are **not** covered by the
[AIP Logic and Evals checklist](./foundry-aip-logic-evals-1to1-checklist.md):
interactive AIP Agents, AIP Threads, and AIP Assist. It should integrate with
AIP Logic/Evals for agents-as-Functions and evaluation suites; with the
Ontology/Object Views checklist for object, object set, action, interface, and
permission semantics; with Workshop for the AIP Agent widget and application
variables; with Developer Console/OSDK surfaces for external application
embedding; with Notepad, Media Sets, and Code Repositories for document/custom
content source ingestion; with DevOps/Marketplace for agent packaging; and with
Security/Governance for AIP enablement, model access, markings, audit, feedback,
retention, and safe logging. It should not duplicate those underlying services.

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
| `P0` | Required for credible demo workflows that create, publish, chat with, and embed an agent using Ontology/document context and safe tools. |
| `P1` | Required for Foundry-style agent, thread, and Assist parity beyond simple chat completion. |
| `P2` | Advanced governance, marketplace, branching, observability, scale, external app, or enterprise rollout parity. |

## Official Palantir documentation library

These public docs should be treated as the external behavioral contract while
implementing this checklist.

### AIP overview and administration

- [AIP overview](https://www.palantir.com/docs/foundry/aip/overview/)
- [AIP features](https://www.palantir.com/docs/foundry/aip/aip-features)
- [Platform overview: AIP capabilities](https://www.palantir.com/docs/foundry/platform-overview/aip-capabilities)
- [Get started with AIP](https://www.palantir.com/docs/foundry/aip/getting-started)
- [Supported LLMs](https://www.palantir.com/docs/foundry/aip/supported-llms)
- [LLM-provider compatible APIs](https://www.palantir.com/docs/foundry/aip/llm-provider-compatible-apis)
- [AIP security and privacy](https://www.palantir.com/docs/foundry/aip/security-and-privacy)
- [AIP compute usage](https://www.palantir.com/docs/foundry/aip/compute-usage)
- [AIP observability](https://www.palantir.com/docs/foundry/aip/observability)
- [Enable AIP features](https://www.palantir.com/docs/foundry/aip/enable-aip-features)
- [LLM capacity management](https://www.palantir.com/docs/foundry/aip/llm-capacity-management)

### AIP Agent Studio / AIP Chatbot Studio

- [AIP Agent Studio overview](https://www.palantir.com/docs/foundry/agent-studio)
- [AIP Agent Studio core concepts](https://www.palantir.com/docs/foundry/agent-studio/core-concepts)
- [AIP Agent Studio getting started](https://www.palantir.com/docs/foundry/agent-studio/getting-started/)
- [Application state](https://www.palantir.com/docs/foundry/agent-studio/application-state/)
- [Retrieval context](https://www.palantir.com/docs/foundry/agent-studio/retrieval-context/)
- [Retrieval context types](https://www.palantir.com/docs/foundry/agent-studio/context-types/)
- [Citations](https://www.palantir.com/docs/foundry/agent-studio/citations/)
- [Tools overview](https://www.palantir.com/docs/foundry/agent-studio/tools/)
- [Use commands as tools in AIP Agent Studio](https://www.palantir.com/docs/foundry/agent-studio/commands-as-tools/)
- [Agents as Functions](https://www.palantir.com/docs/foundry/agent-studio/agents-as-functions)
- [Use AIP Agents through Foundry APIs](https://www.palantir.com/docs/foundry/agent-studio/foundry-apis)
- [Distribute AIP Agents using Marketplace](https://www.palantir.com/docs/foundry/agent-studio/marketplace)

### AIP Threads

- [AIP Threads overview](https://www.palantir.com/docs/foundry/threads/overview/)
- [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/)

### AIP Assist

- [AIP Assist overview](https://www.palantir.com/docs/foundry/assist/overview/)
- [AIP Assist best practices](https://www.palantir.com/docs/foundry/assist/best-practices/)
- [Custom content sources in AIP Assist](https://www.palantir.com/docs/foundry/assist/aip-assist-custom-docs-overview)
- [Register custom content sources with AIP Assist](https://www.palantir.com/docs/foundry/assist/aip-assist-registering-content/)
- [Serve custom content sources to users](https://www.palantir.com/docs/foundry/assist/serving-custom-sources/)
- [Deploy AIP Agents to AIP Assist](https://www.palantir.com/docs/foundry/assist/agents-in-aip-assist/)
- [Custom content source best practices](https://www.palantir.com/docs/foundry/assist/custom-content-best-practices/)
- [AIP Assist application integrations](https://www.palantir.com/docs/foundry/assist/application-integrations/)
- [Suggested actions in AIP Assist](https://www.palantir.com/docs/foundry/assist/suggested-actions/)

### Integrated Foundry surfaces

- [Workshop AIP Agent widget](https://www.palantir.com/docs/foundry/workshop/widgets-aip-agent)
- [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/)
- [Automate overview](https://www.palantir.com/docs/foundry/automate/overview/)
- [Functions overview](https://www.palantir.com/docs/foundry/functions/overview)
- [Ontology overview](https://www.palantir.com/docs/foundry/ontology/overview/)
- [Notepad overview](https://www.palantir.com/docs/foundry/notepad/overview/)
- [Media sets overview](https://www.palantir.com/docs/foundry/media-sets-advanced-formats)
- [Developer Console overview](https://www.palantir.com/docs/foundry/developer-console/overview/)
- [Ontology SDK overview](https://www.palantir.com/docs/foundry/ontology-sdk/overview/)
- [Marketplace overview](https://www.palantir.com/docs/foundry/marketplace/overview)

## Target OpenFoundry resource model

The implementation should define stable OpenFoundry-owned resources that can map
to public Foundry concepts without requiring Palantir RID formats. Compatibility
aliases may be accepted at service boundaries, but persisted state should use
OpenFoundry canonical IDs.

| Public Foundry concept | OpenFoundry resource target | Required notes |
| --- | --- | --- |
| AIP Agent file | `aip_agent` | Project/folder-managed assistant resource with name, description, avatar, model settings, instructions, retrieval context, application state, tool bindings, versions, permissions, and branch metadata. |
| Agent version | `aip_agent_version` | Saved/published immutable snapshot with changelog, prompt hash, model/provider reference, tool manifests, retrieval context manifests, and release metadata. |
| Agent deployment | `aip_agent_deployment` | Published availability target for Threads, Workshop, Assist, OSDK/API, or Function invocation, with environment, status, owner, and rollback metadata. |
| Agent session | `aip_agent_session` | Conversation instance with agent/version, caller surface, user/service identity, application state, message history pointer, usage, feedback, and retention policy. |
| Agent message | `aip_agent_message` | User, assistant, tool, command, clarification, or system message with redacted payload, citations, token usage, trace pointer, and safety metadata. |
| Agent instruction | `aip_agent_instruction` | System prompt/instruction block plus descriptions compiled into the model prompt with provenance and validation status. |
| Agent retrieval context | `aip_agent_retrieval_context` | Configured document, Ontology, media, dataset, Notepad, custom documentation, or application-specific context source. |
| Retrieval result | `aip_retrieval_result` | Per-message retrieved chunk/object/document result with rank, citation, source permissions, token budget, and redaction decision. |
| Agent application state | `aip_agent_application_state` | Read/write application variable contract for Workshop/OSDK embedding, including default values, object sets, variable descriptions, and update permissions. |
| Agent tool | `aip_agent_tool` | Tool descriptor for action, object query, function, command, update application variable, request clarification, or OpenFoundry-native extension. |
| Agent tool invocation | `aip_agent_tool_invocation` | Planned and executed tool call with generated inputs, confirmation state, outputs, errors, audit, and rollback/side-effect metadata where applicable. |
| Agent command binding | `aip_agent_command_binding` | Cross-application command operation exposed as an agent tool with input/output schema and host-application constraints. |
| Agent as Function binding | `aip_agent_function_binding` | Generated function wrapper with `userInput`, optional `sessionId`, application-state inputs/outputs, markdown response, and version publication policy. |
| Thread | `aip_thread` | User-owned or project-visible ad-hoc conversation with model settings, selected documents, selected agent, thread metadata, export options, and deletion state. |
| Thread document | `aip_thread_document` | Uploaded or selected PDF/document/media reference with storage location, extraction mode, permissions, parsed text/chunks, and citation metadata. |
| Thread configuration | `aip_thread_configuration` | Model, temperature, system prompt, document mode, selected documents, selected/default agent, and upgrade-to-agent metadata. |
| Thread export | `aip_thread_export` | JSON/PDF/Markdown export request and artifact with redaction, citation inclusion, and retention metadata. |
| Assist panel configuration | `aip_assist_configuration` | Enrollment/org/project-level Assist enablement, model access, global versus resource-scoped custom sources, and user-facing mode settings. |
| Assist custom content source | `aip_assist_content_source` | Registered Notepad or documentation-repository source with title, description, owner, visibility, indexing status, and serving rules. |
| Assist agent | `aip_assist_agent` | Agent deployment made discoverable inside AIP Assist, with custom source constraints and enrollment availability state. |
| Assist suggested action | `aip_assist_suggested_action` | Contextual action recommendation shown by Assist with triggering context, destination, and audit metadata. |
| Agent feedback | `aip_agent_feedback` | Thumbs up/down, free-text feedback, source surface, message/session reference, moderation state, and analysis status. |
| Agent metric | `aip_agent_metric` | Usage, latency, token, retrieval, tool-call, error, feedback, and adoption metric attributed to agent/version/surface. |
| Agent audit event | `aip_agent_audit_event` | Governance event for create, edit, save, publish, invoke, tool call, content source registration, Assist serving, export, and deletion. |

## Milestone A: minimum viable AIP Agents, Threads, and Assist parity

### Agent Studio shell and authoring basics

- [ ] `AIPAG.1` Agent resource CRUD and project placement (`P0`, `todo`)
  - Create, list, get, update metadata, duplicate, archive/delete, and restore AIP Agent files.
  - Store agents as project/folder-managed resources with canonical OpenFoundry IDs and optional Foundry-style aliases.
  - Docs: [AIP Agent Studio overview](https://www.palantir.com/docs/foundry/agent-studio), [AIP Agent Studio getting started](https://www.palantir.com/docs/foundry/agent-studio/getting-started/).

- [ ] `AIPAG.2` Agent authoring UI shell (`P0`, `todo`)
  - Add an Agent Studio-like editor with landing page, create flow, edit/view modes, save, publish, and preview/chat panels.
  - Include name, description, avatar/icon metadata, location selector, and unsaved-change state.
  - Docs: [AIP Agent Studio getting started](https://www.palantir.com/docs/foundry/agent-studio/getting-started/).

- [ ] `AIPAG.3` Model, temperature, and instruction configuration (`P0`, `todo`)
  - Configure supported LLM, temperature, system instructions, tool descriptions, variable descriptions, and prompt compilation preview.
  - Validate model availability through existing model/AIP governance surfaces.
  - Docs: [AIP Agent Studio core concepts](https://www.palantir.com/docs/foundry/agent-studio/core-concepts), [Supported LLMs](https://www.palantir.com/docs/foundry/aip/supported-llms).

- [ ] `AIPAG.4` Conversation starters and input placeholder (`P0`, `todo`)
  - Store suggested prompts and chat input placeholder per agent version.
  - Render them in view mode, Threads mode, and Workshop widget mode.
  - Docs: [AIP Agent Studio getting started](https://www.palantir.com/docs/foundry/agent-studio/getting-started/).

- [ ] `AIPAG.5` Save, version history, and publish lifecycle (`P0`, `todo`)
  - Support draft saves, named save descriptions, immutable published versions, publish status, rollback, and version comparison metadata.
  - Make published versions selectable from Workshop/API integrations.
  - Docs: [AIP Agent Studio getting started](https://www.palantir.com/docs/foundry/agent-studio/getting-started/).

### Retrieval context, application state, and tools

- [ ] `AIPAG.6` Retrieval context resource model (`P0`, `todo`)
  - Model document, Notepad, media, custom documentation, Ontology, object set, dataset, and function-backed retrieval sources.
  - Capture indexing status, citation policy, permission handoffs, source freshness, token budget, and branch context.
  - Docs: [Retrieval context](https://www.palantir.com/docs/foundry/agent-studio/retrieval-context/), [Retrieval context types](https://www.palantir.com/docs/foundry/agent-studio/context-types/).

- [ ] `AIPAG.7` Citations and retrieval traces (`P0`, `todo`)
  - Return citations for document/Ontology retrieval results and expose them in chat, Threads, Workshop, API, and exported conversations.
  - Store retrieval traces with rank, source, text span, object reference, policy decision, and redaction metadata.
  - Docs: [Citations](https://www.palantir.com/docs/foundry/agent-studio/citations/), [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/).

- [ ] `AIPAG.8` Application state variables (`P0`, `todo`)
  - Define readable/writable application variables with type, default, description, object set support, and Workshop/OSDK mapping rules.
  - Let prompts reference variables and let tools consume variables as inputs.
  - Docs: [Application state](https://www.palantir.com/docs/foundry/agent-studio/application-state/), [Workshop AIP Agent widget](https://www.palantir.com/docs/foundry/workshop/widgets-aip-agent).

- [ ] `AIPAG.9` Tool registry and tool mode (`P0`, `todo`)
  - Support action, object query, function, command, update application variable, request clarification, and legacy semantic-search-compatible tools.
  - Persist tool descriptions, input/output schemas, confirmation requirements, version pinning, and allowed model/tool-mode compatibility.
  - Docs: [Tools overview](https://www.palantir.com/docs/foundry/agent-studio/tools/), [Use commands as tools in AIP Agent Studio](https://www.palantir.com/docs/foundry/agent-studio/commands-as-tools/).

- [ ] `AIPAG.10` Safe action-taking and confirmation flow (`P0`, `todo`)
  - Require explicit confirmation for configured write/action tools unless automatic execution is allowed by policy.
  - Audit proposed inputs, user confirmation, side effects, failures, and resulting Ontology edits.
  - Docs: [Tools overview](https://www.palantir.com/docs/foundry/agent-studio/tools/), [Action types overview](https://www.palantir.com/docs/foundry/action-types/overview).

### Sessions, APIs, and first integrations

- [ ] `AIPAG.11` Agent session and message runtime (`P0`, `todo`)
  - Create resumable sessions with message history, application state, retrieval traces, tool calls, token usage, and retention policy.
  - Support user-scoped and project/application-scoped visibility where locally supported.
  - Docs: [Use AIP Agents through Foundry APIs](https://www.palantir.com/docs/foundry/agent-studio/foundry-apis).

- [ ] `AIPAG.12` Blocking and streaming agent APIs (`P0`, `todo`)
  - Provide API endpoints to create/get sessions, list/get content, send messages, stream responses, and include parameter inputs/updates.
  - Include SDK-friendly request/response shapes for future OSDK parity.
  - Docs: [Use AIP Agents through Foundry APIs](https://www.palantir.com/docs/foundry/agent-studio/foundry-apis).

- [ ] `AIPAG.13` Agents as Functions (`P0`, `todo`)
  - Publish an agent as a Function with `userInput`, optional session ID, application-state inputs/outputs, markdown response, and session output.
  - Support publish-on-save or publish-on-publish policy, version selection, and AIP Evals/Automate handoffs.
  - Docs: [Agents as Functions](https://www.palantir.com/docs/foundry/agent-studio/agents-as-functions), [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/).

- [ ] `AIPAG.14` Workshop AIP Agent widget integration (`P0`, `todo`)
  - Add an AIP Agent-backed widget configuration path that chooses agent/version, maps application state to Workshop variables, shows/hides reasoning where supported, and supports auto-send.
  - Keep legacy Workshop-local agent configuration as a migration-only mode if present.
  - Docs: [Workshop AIP Agent widget](https://www.palantir.com/docs/foundry/workshop/widgets-aip-agent).

- [ ] `AIPAG.15` Monitoring, usage, and feedback MVP (`P0`, `todo`)
  - Capture agent invocations, latency, token/compute usage, retrieval/tool counts, errors, and user feedback.
  - Show per-agent/per-version Monitoring and Usage tabs with exportable metrics.
  - Docs: [AIP Agent Studio getting started](https://www.palantir.com/docs/foundry/agent-studio/getting-started/), [AIP observability](https://www.palantir.com/docs/foundry/aip/observability).

## Milestone B: credible Foundry-style AIP Agents, Threads, and Assist parity

### AIP Threads

- [ ] `AIPAG.16` Threads application shell (`P1`, `todo`)
  - Implement threads navigation, create/select/delete thread, left-panel collapse, dark-mode preference, and conversation interface.
  - Persist thread metadata, title, owner, selected documents, selected agent, and last activity.
  - Docs: [AIP Threads overview](https://www.palantir.com/docs/foundry/threads/overview/), [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/).

- [ ] `AIPAG.17` Thread document upload and storage handoff (`P1`, `todo`)
  - Upload native PDFs or supported documents into a selected storage/media set location and register them with the thread.
  - Store parse status, extraction errors, document metadata, and source permissions.
  - Docs: [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/), [Media sets overview](https://www.palantir.com/docs/foundry/media-sets-advanced-formats).

- [ ] `AIPAG.18` Thread document modes (`P1`, `todo`)
  - Support full document text and relevant chunk retrieval modes with token-budget-aware fallback.
  - Show citations and source snippets when documents answer a question.
  - Docs: [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/).

- [ ] `AIPAG.19` Thread model mode and AIP Agent mode (`P1`, `todo`)
  - Let users choose model, temperature, system prompt, selected documents, selected AIP Agent, and default agent.
  - Ensure model-mode threads only access explicitly selected documents, while agent-mode threads use the published agent configuration.
  - Docs: [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/).

- [ ] `AIPAG.20` Upgrade thread configuration to agent (`P1`, `todo`)
  - Convert a valuable thread configuration into an AIP Agent draft, preserving model settings, prompt, document context, and metadata.
  - Link the source thread to the generated agent for provenance.
  - Docs: [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/), [AIP Agent Studio overview](https://www.palantir.com/docs/foundry/agent-studio).

- [ ] `AIPAG.21` Thread export (`P1`, `todo`)
  - Export conversation contents as JSON and PDF or OpenFoundry-supported equivalents with citations and redaction controls.
  - Audit exports and apply content-source permission checks.
  - Docs: [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/).

### AIP Assist

- [ ] `AIPAG.22` Assist sidebar shell (`P1`, `todo`)
  - Add a platform-wide Assist panel entry point with keyboard shortcut metadata, natural-language query field, response panel, feedback, and mode/agent selector.
  - Gate availability by AIP/application access configuration.
  - Docs: [AIP Assist overview](https://www.palantir.com/docs/foundry/assist/overview/), [Enable AIP features](https://www.palantir.com/docs/foundry/aip/enable-aip-features).

- [ ] `AIPAG.23` Custom content source registration (`P1`, `todo`)
  - Register Notepad documents and documentation repository Markdown as Assist content sources with title, description, indexing status, and owner.
  - Validate heading/content metadata, source permissions, and discoverability constraints.
  - Docs: [Custom content sources in AIP Assist](https://www.palantir.com/docs/foundry/assist/aip-assist-custom-docs-overview), [Register custom content sources with AIP Assist](https://www.palantir.com/docs/foundry/assist/aip-assist-registering-content/).

- [ ] `AIPAG.24` Serve custom sources to users (`P1`, `todo`)
  - Configure whether custom sources are globally available, resource-scoped, project-scoped, or agent-only.
  - Enforce Control Panel-like governance for who can register, approve, serve, and consume sources.
  - Docs: [Serve custom content sources to users](https://www.palantir.com/docs/foundry/assist/serving-custom-sources/), [AIP Assist overview](https://www.palantir.com/docs/foundry/assist/overview/).

- [ ] `AIPAG.25` Deploy custom source-backed AIP Assist agents (`P1`, `todo`)
  - Make selected AIP Agents discoverable in Assist with custom source bindings and source-specific answer constraints.
  - Respect feature availability, source permissions, and agent read permissions.
  - Docs: [Deploy AIP Agents to AIP Assist](https://www.palantir.com/docs/foundry/assist/agents-in-aip-assist/), [AIP Agent Studio overview](https://www.palantir.com/docs/foundry/agent-studio).

- [ ] `AIPAG.26` Suggested actions in Assist (`P1`, `todo`)
  - Surface contextual suggested actions with destination resource/action metadata and audit trail.
  - Allow resource owners/admins to enable, suppress, or configure suggestions.
  - Docs: [Suggested actions in AIP Assist](https://www.palantir.com/docs/foundry/assist/suggested-actions/).

### Distribution and cross-surface parity

- [ ] `AIPAG.27` OSDK and Developer Console handoff (`P1`, `todo`)
  - Expose published agents through SDK/API client shapes compatible with OpenFoundry Developer Console and generated clients.
  - Include session metadata APIs, content APIs, streaming support, and application-state parameter updates.
  - Docs: [Use AIP Agents through Foundry APIs](https://www.palantir.com/docs/foundry/agent-studio/foundry-apis), [Ontology SDK overview](https://www.palantir.com/docs/foundry/ontology-sdk/overview/).

- [ ] `AIPAG.28` Marketplace packaging for agents (`P1`, `todo`)
  - Package AIP Agents as product outputs; include required document/media context where supported; block unsupported Assist-agent packaging or document it as unsupported.
  - Handle input mapping for tools, functions, actions, object types, media sets, and content sources.
  - Docs: [Distribute AIP Agents using Marketplace](https://www.palantir.com/docs/foundry/agent-studio/marketplace), [Marketplace overview](https://www.palantir.com/docs/foundry/marketplace/overview).

- [ ] `AIPAG.29` Agent migration from legacy Workshop agents (`P1`, `todo`)
  - Detect legacy Workshop AIP Agent widget configurations and provide a migration flow to a first-class AIP Agent resource.
  - Preserve prompts, tools, variables, semantic search settings, and visible behavior where possible.
  - Docs: [Workshop AIP Agent widget](https://www.palantir.com/docs/foundry/workshop/widgets-aip-agent), [AIP Agent Studio overview](https://www.palantir.com/docs/foundry/agent-studio).

- [ ] `AIPAG.30` Evaluation-suite integration for agents (`P1`, `todo`)
  - Create and run AIP Evals against agents published as Functions, including session reset, object set variable handling, and markdown response evaluation.
  - Record evaluator traces alongside agent traces.
  - Docs: [Agents as Functions](https://www.palantir.com/docs/foundry/agent-studio/agents-as-functions), [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/).

## Milestone C: governance, scale, and production readiness

### Governance and security

- [ ] `AIPAG.31` AIP enablement and model/capacity controls (`P2`, `todo`)
  - Gate Agent Studio, Threads, and Assist by AIP enablement, model allowlists, capacity pools, project settings, and app access controls.
  - Provide actionable blocked states when a model, tool, or source is unavailable.
  - Docs: [Enable AIP features](https://www.palantir.com/docs/foundry/aip/enable-aip-features), [LLM capacity management](https://www.palantir.com/docs/foundry/aip/llm-capacity-management).

- [ ] `AIPAG.32` Permission-aware retrieval and tool calls (`P2`, `todo`)
  - Enforce source, object, marking, restricted-view, action, function, and project permissions before retrieving context or executing tools.
  - Include policy decisions in debug traces without exposing protected data.
  - Docs: [AIP security and privacy](https://www.palantir.com/docs/foundry/aip/security-and-privacy), [Object permissioning: managing object security](https://www.palantir.com/docs/foundry/object-permissioning/managing-object-security/).

- [ ] `AIPAG.33` Prompt, payload, and feedback redaction (`P2`, `todo`)
  - Redact sensitive prompts, retrieved chunks, tool inputs/outputs, conversation exports, and feedback based on markings and retention policy.
  - Support opt-in debug payload capture with expiry and elevated permissions.
  - Docs: [AIP security and privacy](https://www.palantir.com/docs/foundry/aip/security-and-privacy), [AIP observability](https://www.palantir.com/docs/foundry/aip/observability).

- [ ] `AIPAG.34` Agent audit log (`P2`, `todo`)
  - Audit create, edit, save, publish, view, invoke, stream, export, feedback, tool-call, content-source registration, Assist serving, and delete events.
  - Include actor, source surface, session, agent/version, policy decisions, and affected resources.
  - Docs: [Audit logs overview](https://www.palantir.com/docs/foundry/security/audit-logs-overview), [AIP observability](https://www.palantir.com/docs/foundry/aip/observability).

- [ ] `AIPAG.35` Retention and deletion policy (`P2`, `todo`)
  - Configure retention for sessions, messages, retrieval traces, tool traces, exports, feedback, and content-source indexes.
  - Honor thread deletion, agent deletion, source unregistration, and legal hold-like policies where locally supported.
  - Docs: [Retention overview](https://www.palantir.com/docs/foundry/retention/overview/), [AIP Threads getting started](https://www.palantir.com/docs/foundry/threads/getting-started/).

### Reliability and scale

- [ ] `AIPAG.36` Token budgeting and context-window management (`P2`, `todo`)
  - Estimate token usage for instructions, conversation history, retrieval context, application state, and tool schemas before invocation.
  - Provide deterministic truncation, summarization, and user-facing context-window errors.
  - Docs: [AIP Agent Studio core concepts](https://www.palantir.com/docs/foundry/agent-studio/core-concepts), [AIP compute usage](https://www.palantir.com/docs/foundry/aip/compute-usage).

- [ ] `AIPAG.37` Agent run trace and debugger (`P2`, `todo`)
  - Store an inspectable trace for model calls, retrieval, tool selection, tool execution, commands, clarification turns, and final response.
  - Provide security-aware debug access to builders and admins.
  - Docs: [AIP observability](https://www.palantir.com/docs/foundry/aip/observability), [Tools overview](https://www.palantir.com/docs/foundry/agent-studio/tools/).

- [ ] `AIPAG.38` Usage metering and quotas (`P2`, `todo`)
  - Meter by agent, version, thread, Assist mode, Workshop module, external app, user/service, model, tool, and content source.
  - Enforce quotas and show cost previews or warnings for expensive retrieval/model/tool patterns.
  - Docs: [AIP compute usage](https://www.palantir.com/docs/foundry/aip/compute-usage), [Resource Management usage types](https://www.palantir.com/docs/foundry/resource-management/usage-types).

- [ ] `AIPAG.39` Branch-aware agents and content sources (`P2`, `todo`)
  - Let agents, tool bindings, object types, functions, Workshop modules, and content sources participate in Global Branching proposals.
  - Preview branch-specific agents without leaking side effects to production.
  - Docs: [Global Branching overview](https://www.palantir.com/docs/foundry/foundry-branching/overview/), [Branching functions](https://www.palantir.com/docs/foundry/global-branching/branching-functions/).

- [ ] `AIPAG.40` Production rollout and adoption analytics (`P2`, `todo`)
  - Track active users, unanswered questions, thumbs-down clusters, tool failure categories, retrieval misses, Assist content gaps, and version adoption.
  - Provide builder/admin dashboards and exportable datasets for continuous improvement.
  - Docs: [AIP Agent Studio getting started](https://www.palantir.com/docs/foundry/agent-studio/getting-started/), [AIP Assist best practices](https://www.palantir.com/docs/foundry/assist/best-practices/).

## Implementation inventory to collect before coding

- [ ] `INV.1` Identify existing OpenFoundry chat, LLM provider, model routing, prompt, token metering, and streaming response components.
- [ ] `INV.2` Identify existing Ontology object set, action, function, interface, restricted view, and audit APIs that can back agent tools.
- [ ] `INV.3` Identify existing Workshop variable, command, widget, event, and publish/version APIs that can host the AIP Agent widget.
- [ ] `INV.4` Identify existing Media Set, Notepad, Markdown/documentation repository, PDF parsing, OCR, chunking, embedding, and citation components.
- [ ] `INV.5` Identify existing AIP Logic/Evals function invocation, run history, evaluator, trace, and results dataset components.
- [ ] `INV.6` Identify existing Developer Console/OSDK/API token surfaces for external agent application usage.
- [ ] `INV.7` Identify existing Marketplace product packaging and input mapping for agents, tools, functions, actions, and document/media context.
- [ ] `INV.8` Identify existing security controls for markings, scoped sessions, restricted views, OAuth scopes, service users, retention, and audit.

## Suggested service boundaries

| Service or package | Responsibility |
| --- | --- |
| `aip-agent-service` | Agent CRUD, versions, publish lifecycle, authoring metadata, session/message APIs, agent runtime orchestration, tracing, feedback, and metrics. |
| `aip-retrieval-service` | Document/Ontology/custom-source indexing, retrieval, citations, token budgeting, permission checks, and retrieval traces. |
| `aip-assist-service` | Assist sidebar configuration, custom content registration, source serving rules, Assist agents, suggested actions, and Assist feedback. |
| `aip-threads-service` | Thread CRUD, document upload/selection, model mode, agent mode, exports, thread-to-agent upgrade, and thread retention. |
| `ontology-actions-service` | Tool execution for actions, functions, object queries, object sets, and interface-aware edits. |
| `application-composition-service` | Workshop AIP Agent widget configuration, variable mapping, command integration, and auto-send event wiring. |
| `model-integration-service` | LLM/model registry, provider routing, capacity, model availability, usage attribution, and external model guardrails. |
| `security-governance-service` | AIP enablement, markings, restricted views, scoped sessions, audit logs, retention, and policy decisions. |
| `marketplace-service` | Agent packaging, input mapping, document/media context inclusion, install validation, and unsupported Assist-agent enforcement. |

## Acceptance criteria for first complete AIP Agents milestone

- A builder can create an AIP Agent in a project folder, configure model, temperature, instructions, conversation starters, retrieval context, application state, and at least one safe tool.
- The builder can save and publish immutable agent versions, view/run a published version, and inspect Monitoring/Usage/Feedback basics.
- A user can chat with the agent through an API and through a Workshop AIP Agent widget with mapped application variables.
- A user can create an AIP Thread, upload/select a document, ask questions with citations, export the thread, and upgrade a thread configuration into an agent draft.
- An admin/builder can register a custom Notepad or Markdown documentation source for Assist and serve it either through the default Assist experience or a source-backed Assist agent.
- Agents can be published as Functions and invoked from AIP Evals or Automate-compatible surfaces with session reset semantics.
- Retrieval, tool calls, action confirmations, exports, and feedback are audited and permission-aware.

## Test plan expectations

- Unit tests for agent resource validation, prompt compilation, model settings, version snapshots, retrieval context manifests, application state mapping, tool schema validation, and token budgeting.
- Unit tests for permission decisions over document chunks, object queries, actions, functions, content sources, and Assist source serving rules.
- API tests for agent CRUD, save/publish/version readback, session create/get/content, blocking send, streaming send, feedback, metrics, thread CRUD, thread export, content-source registration, and Assist agent discovery.
- Integration tests for Workshop AIP Agent widget variable mapping, action confirmation flows, agents-as-Functions, AIP Evals invocation, Assist source-backed answers, and thread-to-agent upgrade.
- Security tests for marking-aware retrieval redaction, restricted-view object access, service/user-scoped invocation, payload logging redaction, export authorization, and audit event completeness.
- Load tests for concurrent streaming sessions, retrieval fan-out, tool invocation retries, Assist sidebar usage, and token/cost quota enforcement.
