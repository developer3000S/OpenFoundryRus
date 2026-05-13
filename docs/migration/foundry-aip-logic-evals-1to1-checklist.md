# Foundry AIP Logic and Evals 1:1 parity checklist

Date: 2026-05-11
Scope: public-docs-based parity plan for OpenFoundry's AIP Logic and AIP Evals
surfaces: no-code Logic functions, Logic files, input/block/output boards,
Use LLM blocks, prompts, tools, Query objects, Apply action, Execute function,
Calculator, conditionals, loops, create variables, typed inputs, media/model
inputs, block outputs, final outputs, Ontology edit outputs, debugger, run
panel, run history, unit tests, publishing, versions, comparison view, usage
surfaces, command-line/API invocation, execution modes, project-scoped run
history datasets, Automate integration, branch-aware Logic editing, Logic
metrics, compute usage, evaluation suites, target functions, test cases,
object-set-backed test cases, evaluators, custom evaluation functions, built-in
evaluators, Marketplace evaluator handoffs, multi-target runs, single-test-case
runs, run configurations, iterations, parallelization, run metadata,
experiments, intermediate parameters, Ontology-edit simulations, results
analysis, results datasets, metrics dashboards, trace/debug views, and
production-readiness guardrails for LLM-backed functions.

This document is intentionally implementation-oriented. It does not attempt to
clone Palantir branding, private source code, proprietary assets, screenshots,
or any non-public behavior. The target is **functional parity based on public
Palantir Foundry documentation**: the same product concepts, comparable
Logic/Evals authoring and operational workflows, compatible resource models
where useful, and OpenFoundry-native implementation details that can be tested
locally.

## Parity scope boundary

All checklist work is governed by the
[Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).
OpenFoundry may implement behavior described in public Palantir documentation,
but contributors must not copy private source, decompile bundles, import
tenant-specific exports, use Palantir branding, or reuse proprietary assets.
The product target is functional parity in an OpenFoundry-native implementation,
not a pixel-perfect clone.

This checklist covers AIP Logic as a no-code function-builder surface and AIP
Evals as a test/evaluation surface for Logic, agent-like functions, and
code-authored functions. It should integrate with the Ontology/Object Views
checklist for object, object set, action, and permission semantics; with the
Functions checklist for published function invocation and versioning; with the
Automate/Rules checklist for Logic effects and staged human review; with the
Media Sets checklist for media-reference inputs; with the Global Branching
checklist for branch-aware Logic resources; with Data Foundation for results
and run-history datasets; and with AIP/model governance for supported LLMs,
capacity, token usage, and security. It should not duplicate those underlying
surfaces.

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
| `P0` | Required for credible demo workflows that build, run, debug, publish, and evaluate a Logic function over Ontology objects and text. |
| `P1` | Required for Foundry-style AIP Logic and AIP Evals parity beyond simple prompt execution. |
| `P2` | Advanced, governance-heavy, branching, experiment, scale, observability, or marketplace-oriented parity. |

## Official Palantir documentation library

These public docs should be treated as the external behavioral contract while
implementing this checklist.

### AIP overview and Logic

- [AIP overview](https://www.palantir.com/docs/foundry/aip/overview/)
- [AIP features](https://www.palantir.com/docs/foundry/aip/aip-features/)
- [AIP Logic overview](https://www.palantir.com/docs/foundry/logic)
- [AIP Logic core concepts](https://www.palantir.com/docs/foundry/logic/core-concepts/)
- [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started)
- [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/)
- [AIP Logic compute usage](https://www.palantir.com/docs/foundry/logic/compute-usage)
- [AIP Logic metrics](https://www.palantir.com/docs/foundry/logic/logic-metrics/)
- [AIP Logic execution mode settings](https://www.palantir.com/docs/foundry/logic/execution-mode-settings/)
- [AIP Logic integration with Automate](https://www.palantir.com/docs/foundry/logic/aip-logic-integration-automate/)
- [Branching AIP Logic](https://www.palantir.com/docs/foundry/logic/branching-logic)

### AIP Evals

- [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/)
- [Evaluation suites for Logic functions](https://www.palantir.com/docs/foundry/aip-evals/getting-started/)
- [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite)
- [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/)
- [Run experiments](https://www.palantir.com/docs/foundry/aip-evals/experiments/)
- [Analyze run results](https://www.palantir.com/docs/foundry/aip-evals/analyze-run-results)
- [Use intermediate parameters to evaluate block output](https://www.palantir.com/docs/foundry/logic/evaluations-intermediate-parameters/)
- [Evaluate Ontology edits](https://www.palantir.com/docs/foundry/aip-evals/ontology-edits)
- [Write run results to a dataset](https://www.palantir.com/docs/foundry/aip-evals/results-dataset)
- [View results in metrics dashboard](https://www.palantir.com/docs/foundry/logic/evaluations-metrics-dashboard/)

### Integrated Foundry surfaces

- [Automate overview](https://www.palantir.com/docs/foundry/automate/overview/)
- [AIP Logic integration with Automate](https://www.palantir.com/docs/foundry/logic/aip-logic-integration-automate/)
- [Functions getting started](https://www.palantir.com/docs/foundry/functions/getting-started/)
- [Functions in Workshop](https://www.palantir.com/docs/foundry/workshop/functions-use/)
- [Action types overview](https://www.palantir.com/docs/foundry/action-types/overview)
- [Object Explorer overview](https://www.palantir.com/docs/foundry/object-explorer/overview)
- [Media sets overview](https://www.palantir.com/docs/foundry/media-sets-advanced-formats)

## Target OpenFoundry resource model

The implementation should define stable OpenFoundry-owned resources that can map
to public Foundry concepts without requiring Palantir RID formats. Compatibility
aliases may be accepted at service boundaries, but persisted state should use
OpenFoundry canonical IDs.

| Public Foundry concept | OpenFoundry resource target | Required notes |
| --- | --- | --- |
| Logic file | `logic_file` | Project/folder-managed no-code function resource with inputs, blocks, outputs, version history, execution mode, branch metadata, and permissions. |
| Logic function | `logic_function` | Published callable function produced from a Logic file, compatible with Functions, Workshop, Actions, Automate, and API/curl invocation. |
| Logic input | `logic_input` | Typed input definition supporting primitives, arrays/lists, structs, objects, object lists, object sets, media references, models, and timestamps. |
| Logic block | `logic_block` | Node in the Logic execution graph with type, inputs, outputs, prompt/tool configuration, dependency edges, and debug trace identity. |
| Use LLM block | `logic_llm_block` | Prompt + model + tool-enabled LLM interaction with configured output type and trace metadata. |
| Logic tool | `logic_tool` | Tool definition used by an LLM block: query objects, apply action, execute function, calculator, or OpenFoundry-native extension. |
| Intermediate parameter | `logic_intermediate_parameter` | Exposed block output usable by AIP Evals and result datasets. |
| Logic output | `logic_output` | Final value/object/output definition or Ontology-edit output produced by the Logic function. |
| Ontology edit bundle | `logic_ontology_edit_bundle` | Simulated or staged object/action edits produced by Logic, never applied directly except through action/automation flows. |
| Debug trace | `logic_debug_trace` | Per-run block trace, prompts/tool calls, inputs/outputs, errors, and evaluator trace links with security-aware retention. |
| Logic run | `logic_run` | Invocation instance from preview, Workshop, Action, Automate, API, or Evals with execution mode, status, logs, duration, token/compute usage, and outputs. |
| Logic version | `logic_version` | Saved or published version with diff metadata and comparison-view support. |
| Execution mode | `logic_execution_mode` | User-scoped or project-scoped execution behavior controlling permissions, run-history visibility, imports, and results dataset behavior. |
| Logic metric | `logic_metric` | Success/failure counts, P95 duration, run history, failure category, and telemetry surfaced in Ontology Manager/Workflow Lineage-like views. |
| Evaluation suite | `eval_suite` | Test/evaluation resource with target functions, test cases, evaluators, run configuration defaults, results, and permissions. |
| Target function | `eval_target_function` | A Logic, agent-like, or code-authored published function under test, with input/output signature and version selection. |
| Test case | `eval_test_case` | Manual or object-set-backed input/expected-output row with typed columns, expected values, metadata, and generated name hints. |
| Evaluator | `eval_evaluator` | Built-in, custom function, Logic-backed, or Marketplace evaluator that returns Boolean/numeric metrics and optional debug strings. |
| Metric objective | `eval_metric_objective` | Boolean target or numeric maximize/minimize/threshold configuration used to decide pass/fail. |
| Evaluation run | `eval_run` | Full suite, selected target, experiment, or single-test execution with iterations, parallelization, execution mode, metadata, and results. |
| Experiment | `eval_experiment` | Grid-search run group over model/prompt/parameter combinations with grouped aggregate results. |
| Result dataset | `eval_results_dataset` | Dataset written by project-scoped evaluation runs containing function outputs, evaluator results, metadata, and errors. |
| Results analyzer | `eval_results_analyzer` | LLM-assisted failure clustering and prompt suggestion resource with model/max-category/max-test-case configuration. |

## Milestone A: minimum viable AIP Logic and Evals parity

### AIP Logic application shell and authoring basics

- [x] `AIPLE.1` Logic file CRUD and project placement (`P0`, `done`)
  - Create, get, list, update metadata, move, duplicate, archive/delete, and restore Logic files.
  - Require Logic files to be saved in project folders rather than personal-only home folders when mirroring documented behavior.
  - Track name, description, project/folder, owner, created/updated timestamps, current draft version, published version, execution mode, and permissions.
  - Implemented in `agent-runtime-service` with project/folder-required Logic file metadata, owner/permission-aware CRUD, move, duplicate, archive/delete, and restore endpoints under `/api/v1/agent-runtime/logic/files`.
  - Tests cover the project/folder placement guardrail, execution-mode validation, authentication guardrail, and duplicate placement validation.
  - Docs: [AIP Logic overview](https://www.palantir.com/docs/foundry/logic), [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started).

- [x] `AIPLE.2` Logic authoring interface shell (`P0`, `done`)
  - Provide a three-panel authoring UI: inputs/blocks/outputs configuration, debugger, and run panel.
  - Include right sidebar entry points for uses, automations, evaluations, run history, version history, metrics, and execution settings.
  - Preserve OpenFoundry-native UI styling and avoid Palantir screenshots or visual assets.
  - Implemented as the OpenFoundry-native `/logic` authoring route with configuration, debugger, run panel, and right resource rail for uses, automations, evaluations, history, metrics, and execution settings.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [AIP Logic core concepts](https://www.palantir.com/docs/foundry/logic/core-concepts/).

- [x] `AIPLE.3` Logic input board (`P0`, `done`)
  - Define typed inputs for array/list, Boolean, date, double, float, integer, long, media reference, model, object, object list, object set, short, string, struct, and timestamp where local services exist.
  - Validate input API names, required/optional state, default values, object type selections, object set compatibility, and model variable compatibility.
  - Implemented shared input-board validators and UI editing for all supported input types, including typed defaults, object/object-set checks, and model slot compatibility.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/).

- [ ] `AIPLE.4` Logic block graph and dataflow (`P0`, `todo`)
  - Add, remove, reorder, duplicate, and connect blocks.
  - Type-check block inputs/outputs and prevent cycles unless a documented loop/conditional construct owns the flow.
  - Show which block outputs feed subsequent blocks and final outputs.
  - Docs: [AIP Logic core concepts](https://www.palantir.com/docs/foundry/logic/core-concepts/), [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/).

- [x] `AIPLE.5` Use LLM block (`P0`, `done`)
  - Configure model, system/task prompt, tool access, structured output type, and prompt variable references.
  - Support model variables so Evals experiments can swap model values.
  - Record prompt, tool-call, output, token/compute, and error trace metadata in the debugger.
  - Implemented shared Use LLM block config/validation, model-variable binding for Evals, structured output validation, and debugger trace metadata surfaced in the `/logic` authoring shell.
  - Docs: [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/), [AIP Logic compute usage](https://www.palantir.com/docs/foundry/logic/compute-usage).

- [x] `AIPLE.6` Query objects tool (`P0`, `done`)
  - Allow LLM blocks to query configured object types and selected properties.
  - Limit accessible object types/properties to what the Logic function/user can read.
  - Provide token-efficiency warnings when too many object types or properties are exposed.
  - Implemented Query objects tool selection in the Use LLM block editor with readable object/property allowlists and token-efficiency warnings for wide/large query context.
  - Docs: [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/), [AIP Logic overview](https://www.palantir.com/docs/foundry/logic).

- [x] `AIPLE.7` Apply action tool and Ontology edits (`P0`, `done`)
  - Let LLM blocks propose action-backed Ontology edits using selected action types and parameters.
  - Show proposed edits in debugger during preview without applying them to the real Ontology.
  - Require published Logic plus action or automation invocation before real Ontology edits can be applied.
  - Implemented Apply action tool config/validation, preview-only proposed edit trace metadata, and commit guardrails requiring published Logic plus action/automation invocation.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/).

- [x] `AIPLE.8` Execute function and calculator tools (`P0`, `done`)
  - Configure calls to TypeScript, Python, existing Logic, and function-on-objects functions where available.
  - Provide calculator tool support for exact mathematical computation in LLM workflows.
  - Validate function signatures, parameter mapping, permissions, and output type compatibility.
  - Implemented Execute function and calculator tool config/validation, permission allowlists, signature/type checks, deterministic arithmetic evaluation, and debugger tool metadata.
  - Docs: [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/).

- [x] `AIPLE.9` Create variable, conditionals, and loops (`P0`, `done`)
  - Support create variable blocks for primitive/object-compatible values.
  - Support conditionals and loops with list inputs, element/index variables, output aggregation, and parallel-loop behavior when no actions are present.
  - Validate list/array conversion and loop output type compatibility.
  - Implemented create-variable, conditional, and loop config/validation for source typing, branch output compatibility, list/object-list inputs, array-to-list conversion warnings, element/index loop variables, aggregation output compatibility, ontology-edit/no-action branches, and action-aware parallelization.
  - Docs: [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/).

- [x] `AIPLE.10` Logic outputs (`P0`, `done`)
  - Define final outputs as primitive values, objects, object sets/lists, structs, media references, or Ontology edit bundles where locally supported.
  - Support block intermediary outputs and final Logic function output.
  - Enforce that Workshop Markdown usage requires a string output when using Logic as a display function.
  - Implemented final/intermediate output definitions and validators for supported output families, source compatibility, final-output presence, unique API names, Ontology edit bundles, and Workshop Markdown string outputs.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [AIP Logic core concepts](https://www.palantir.com/docs/foundry/logic/core-concepts/).

### Running, debugging, publishing, and usage

- [x] `AIPLE.11` Run panel and preview execution (`P0`, `done`)
  - Execute draft Logic with sample inputs from the run panel.
  - Show latest result, status, duration, run metadata, recent runs, and save-as-test-case shortcuts.
  - Support single-run rerun and input editing without publishing.
  - Implemented local draft preview execution helpers plus an editable run panel with latest result, run status/duration/metadata, recent run selection, rerun, and save-as-test-case shortcut affordances.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started).

- [x] `AIPLE.12` Logic debugger (`P0`, `done`)
  - Display block-by-block trace, prompt/tool-call details, inputs, outputs, errors, and final result.
  - Allow expanding/collapsing block cards and clearing local tool-call display state.
  - Ensure logs/traces are security-filtered and retained according to execution mode.
  - Implemented debugger trace builders and UI cards for input binding, LLM prompt/tool calls, final output mapping, expandable block details, local tool-call clearing, sensitive-key redaction, and draft-vs-policy retention metadata.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [AIP Logic core concepts](https://www.palantir.com/docs/foundry/logic/core-concepts/).

- [ ] `AIPLE.13` Save, publish, and version history (`P0`, `todo`)
  - Save draft Logic versions and publish callable Logic functions.
  - Record version history with author, timestamp, block/input/output changes, prompt changes, model changes, and publish status.
  - Provide comparison view for added, edited, and removed blocks.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [AIP Logic core concepts](https://www.palantir.com/docs/foundry/logic/core-concepts/).

- [ ] `AIPLE.14` Function usage surfaces (`P0`, `todo`)
  - Use published Logic functions in Workshop, action-backed workflows, other Logic functions, function-on-objects style calls, Automate, and API/curl invocation where supported.
  - Block command-line/API invocation for Logic functions that return Ontology edits when mirroring documented limitations.
  - Show usage snippets and links from the Uses sidebar.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [Functions in Workshop](https://www.palantir.com/docs/foundry/workshop/functions-use/).

- [ ] `AIPLE.15` User-scoped execution mode (`P0`, `todo`)
  - Execute Logic using the permissions of the initiating user.
  - Restrict execution logs so users only see their own logs.
  - Apply short-lived retention for user-scoped execution logs.
  - Docs: [AIP Logic execution mode settings](https://www.palantir.com/docs/foundry/logic/execution-mode-settings/).

- [ ] `AIPLE.16` Basic Logic metrics (`P0`, `todo`)
  - Surface success count, failure count, failure categories, recent run history, and P95 duration over recent time windows.
  - Show metrics from Logic detail, Ontology Manager-like resource views, and Workflow Lineage-like execution nodes.
  - Require viewer permission to see metrics.
  - Docs: [AIP Logic metrics](https://www.palantir.com/docs/foundry/logic/logic-metrics/).

### Minimum viable AIP Evals

- [ ] `AIPLE.17` Evaluation suite CRUD (`P0`, `todo`)
  - Create, get, list, update, move, duplicate, archive/delete, and restore evaluation suites.
  - Track suite name, project/folder, owner, target functions, test case columns, evaluators, run history, results dataset, and permissions.
  - Create evaluation suites from Logic preview, Evals sidebar, AIP Evals app, and code-authored published function surfaces where available.
  - Docs: [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/), [Evaluation suites for Logic functions](https://www.palantir.com/docs/foundry/aip-evals/getting-started/), [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite).

- [ ] `AIPLE.18` Target functions (`P0`, `todo`)
  - Add Logic, agent-like, and code-authored functions as target functions.
  - Support multiple target functions in one suite and target-specific evaluator mappings.
  - Validate target function input/output signatures and version availability.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite).

- [ ] `AIPLE.19` Manual test cases and columns (`P0`, `todo`)
  - Add manual test cases with name, typed input columns, expected output columns, metadata, and optional generated name hints.
  - Edit test case parameter columns, reorder columns, and validate column types against target function inputs and evaluator mappings.
  - Support adding a test case from a Logic preview run.
  - Docs: [Evaluation suites for Logic functions](https://www.palantir.com/docs/foundry/aip-evals/getting-started/), [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite).

- [ ] `AIPLE.20` Built-in evaluators and objectives (`P0`, `todo`)
  - Support built-in exact match, regex, distance, length, keyword, object/object-set, integer/numeric/floating-point range, and temporal range evaluators where local type support exists.
  - Configure actual/expected mappings, Boolean objectives, numeric maximize/minimize objectives, and thresholds.
  - Compute metric-level, iteration-level, and test-case-level pass/fail status.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite), [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/).

- [ ] `AIPLE.21` Evaluation run basics (`P0`, `todo`)
  - Run full suites and single test cases from AIP Logic sidebar and AIP Evals app.
  - Support target version selection for last-saved Logic, published Logic, and published non-Logic functions.
  - Display aggregate pass percentage, individual test case results, metric results, errors, and debugger links.
  - Docs: [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/), [Analyze run results](https://www.palantir.com/docs/foundry/aip-evals/analyze-run-results).

## Milestone B: credible Foundry-style AIP Logic and Evals parity

### Advanced Logic execution and integration

- [ ] `AIPLE.22` Project-scoped execution mode (`P1`, `todo`)
  - Execute Logic using project permissions when configured.
  - Require all used resources to be imported into the same project and require users to have marking/security access where applicable.
  - Make project-scoped logs visible to project viewers and preserve configurable run history.
  - Docs: [AIP Logic execution mode settings](https://www.palantir.com/docs/foundry/logic/execution-mode-settings/).

- [ ] `AIPLE.23` Logic run history dataset (`P1`, `todo`)
  - Configure a dataset that records project-scoped Logic execution history.
  - Preserve recent run rows up to a documented or locally configured limit.
  - Include inputs, outputs, status, errors, duration, model, branch/version, user/service context, and trace references subject to permissions.
  - Docs: [AIP Logic execution mode settings](https://www.palantir.com/docs/foundry/logic/execution-mode-settings/).

- [ ] `AIPLE.24` Automate integration (`P1`, `todo`)
  - Create pre-populated automations from the Logic Uses sidebar when Logic outputs Ontology edits.
  - Support automatic application of edits or staging of action proposals for human review.
  - Show automation event chart, proposals tab, agent proposal detail, proposed action preview, and decision log handoff.
  - Docs: [AIP Logic integration with Automate](https://www.palantir.com/docs/foundry/logic/aip-logic-integration-automate/), [Automate overview](https://www.palantir.com/docs/foundry/automate/overview/).

- [ ] `AIPLE.25` Logic-backed actions (`P1`, `todo`)
  - Create function-backed action types that invoke published Logic functions.
  - Support action execution from Workshop and branch-aware preview contexts.
  - Ensure real Ontology edits are only applied through action execution or approved automation flows.
  - Docs: [AIP Logic getting started](https://www.palantir.com/docs/foundry/logic/getting-started), [Action types overview](https://www.palantir.com/docs/foundry/action-types/overview).

- [ ] `AIPLE.26` Logic compute usage metering (`P1`, `todo`)
  - Meter usage per executed block and account for downstream systems invoked by blocks.
  - Attribute usage to Logic file, version, block, user/project, Automate run, action, Workshop widget, or Evals run.
  - Surface cost/usage warnings before expensive run/evaluation/experiment configurations.
  - Docs: [AIP Logic compute usage](https://www.palantir.com/docs/foundry/logic/compute-usage).

- [ ] `AIPLE.27` Branching AIP Logic adapter (`P1`, `todo`)
  - Add, remove, edit, publish, review, rebase, and merge Logic files on Global Branches.
  - Enforce merge requirements such as published state, up-to-date with main, no pending approvals, and publishable state.
  - Keep branched Logic versions isolated from main and other branches.
  - Docs: [Branching AIP Logic](https://www.palantir.com/docs/foundry/logic/branching-logic).

- [ ] `AIPLE.28` Logic permissions and security (`P1`, `todo`)
  - Enforce resource view/edit/manage permissions on Logic files and function invocation permissions on published Logic functions.
  - Enforce user/project permission boundaries for tools, object queries, actions, functions, media references, and result datasets.
  - Ensure LLM-accessible data is limited to explicitly configured and permissioned resources.
  - Docs: [AIP Logic overview](https://www.palantir.com/docs/foundry/logic), [AIP Logic execution mode settings](https://www.palantir.com/docs/foundry/logic/execution-mode-settings/).

### Advanced AIP Evals suite construction

- [ ] `AIPLE.29` Object-set-backed test cases (`P1`, `todo`)
  - Add test cases from object sets and map object, object property, linked object, linked object set, linked property, and static value columns.
  - Support multiple object sets plus manual test cases in one suite.
  - Recompute object-set-backed rows according to local snapshot/refresh semantics.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite).

- [ ] `AIPLE.30` Custom evaluation functions (`P1`, `todo`)
  - Select published TypeScript/Python/Logic functions as evaluators.
  - Require at least one Boolean or numeric metric return value and allow struct returns containing multiple metrics.
  - Store string debug outputs from custom evaluators in debug views without treating them as metrics.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite), [Analyze run results](https://www.palantir.com/docs/foundry/aip-evals/analyze-run-results).

- [ ] `AIPLE.31` Marketplace evaluator handoff (`P1`, `todo`)
  - Support installed evaluator functions such as rubric grader, contains-key-details, and ROUGE-like evaluators when OpenFoundry Marketplace/product packaging exists.
  - Open setup wizard and dependency installation flow when an evaluator product is missing.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite).

- [ ] `AIPLE.32` Intermediate parameters (`P1`, `todo`)
  - Expose selected block outputs as intermediate parameters from Logic authoring UI.
  - Use intermediate parameters as evaluator inputs and include their values in project-scoped results datasets.
  - Support evaluating final outputs and intermediate outputs in the same suite.
  - Docs: [Use intermediate parameters to evaluate block output](https://www.palantir.com/docs/foundry/logic/evaluations-intermediate-parameters/).

- [ ] `AIPLE.33` Ontology-edit evaluations (`P1`, `todo`)
  - Execute Logic functions that create/edit/delete Ontology objects inside an Ontology simulation during evaluation.
  - Support custom evaluation functions and intermediate parameters to verify simulated edits.
  - Ensure simulated edits cannot alter the real Ontology during tests.
  - Docs: [Evaluate Ontology edits](https://www.palantir.com/docs/foundry/aip-evals/ontology-edits).

- [ ] `AIPLE.34` Eval run configuration (`P1`, `todo`)
  - Support target version selection, input mapping, user/project execution mode, iteration count, test parallelization, and run metadata.
  - Recommend multiple iterations for LLM-backed functions and warn about rate limits at high parallelization.
  - Preserve branch, version, model, custom metadata, execution mode, and run initiator.
  - Docs: [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/).

- [ ] `AIPLE.35` Multi-target runs and comparisons (`P1`, `todo`)
  - Run the same suite against multiple target functions and choose included targets per run.
  - Compare results across target functions, versions, and models.
  - Disable incompatible experiment options in multi-target mode when required.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite), [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/), [Analyze run results](https://www.palantir.com/docs/foundry/aip-evals/analyze-run-results).

- [ ] `AIPLE.36` Results dataset integration (`P1`, `todo`)
  - Configure a run results dataset in the same project as the evaluation suite.
  - Write function outputs, evaluator results, user metadata, auto-captured metadata, errors, and intermediate parameters when project-scoped execution is used.
  - Document unsupported output cases such as functions that only return Ontology edits.
  - Docs: [Write run results to a dataset](https://www.palantir.com/docs/foundry/aip-evals/results-dataset).

### Evals result analysis and dashboards

- [ ] `AIPLE.37` Results table and debug view (`P1`, `todo`)
  - Show aggregate metrics, per-test-case results, iterations, inputs, expected values, actual outputs, evaluator outputs, debug strings, and errors.
  - Open debug view for individual test cases, including Logic trace, code function preview, evaluator trace, expected vs actual values, and custom evaluator debug outputs.
  - Docs: [Analyze run results](https://www.palantir.com/docs/foundry/aip-evals/analyze-run-results), [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/).

- [ ] `AIPLE.38` Run-to-run comparison (`P1`, `todo`)
  - Compare two evaluation runs side by side and highlight output differences.
  - Compare aggregate metric changes, per-test-case status changes, model/version metadata, and evaluator output differences.
  - Docs: [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/), [Analyze run results](https://www.palantir.com/docs/foundry/aip-evals/analyze-run-results).

- [ ] `AIPLE.39` Metrics dashboard (`P1`, `todo`)
  - Provide charts/statistics for aggregate evaluator metrics and individual test case metrics.
  - Support drill-down into LLM trace viewer and evaluator trace for individual test cases.
  - Support filtering by suite, run, version, model, metric, status, test case, target, and time window.
  - Docs: [View results in metrics dashboard](https://www.palantir.com/docs/foundry/logic/evaluations-metrics-dashboard/).

- [ ] `AIPLE.40` Results analyzer (`P1`, `todo`)
  - Generate LLM-assisted failure-pattern summaries for failed test cases.
  - Group failures into root-cause categories with examples, affected test case filters, and prompt suggestions.
  - Configure analyzer model, max categories, and max failing test cases.
  - Docs: [Analyze run results](https://www.palantir.com/docs/foundry/aip-evals/analyze-run-results).

## Milestone C: advanced experiments, governance, and production readiness

### Experiments and production quality

- [ ] `AIPLE.41` Eval experiments (`P2`, `todo`)
  - Enable grid-search experiments over Logic/function parameters such as model, prompt context, thresholds, and evaluator settings.
  - Preview total run count and parameter combinations before execution.
  - Group experiment results by parameter and compare up to locally configured run limits.
  - Docs: [Run experiments](https://www.palantir.com/docs/foundry/aip-evals/experiments/).

- [ ] `AIPLE.42` Model/prompt parameterization (`P2`, `todo`)
  - Let Logic authors parameterize model selection and prompt fragments as inputs for experiments.
  - Configure LLM blocks to use model variables and prompt variables safely.
  - Track prompt suggestion application and follow-up evaluation evidence.
  - Docs: [Run experiments](https://www.palantir.com/docs/foundry/aip-evals/experiments/), [AIP Logic blocks](https://www.palantir.com/docs/foundry/logic/blocks/).

- [ ] `AIPLE.43` Production readiness gates (`P2`, `todo`)
  - Allow Logic publish/automation/action rollout gates requiring passing evaluation suites, minimum pass percentages, no critical failures, and fresh run timestamps.
  - Show stale or failing Evals warnings before publishing or automation enablement.
  - Store waiver decisions and reviewer comments when a gate is bypassed.
  - Docs: [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/), [AIP Logic integration with Automate](https://www.palantir.com/docs/foundry/logic/aip-logic-integration-automate/).

- [ ] `AIPLE.44` Evaluation scheduling and regression monitoring (`P2`, `todo`)
  - Schedule evaluation suites against published Logic/function versions.
  - Compare scheduled runs against baselines and alert on metric regression, variance spikes, cost spikes, or new failure categories.
  - Integrate with Data Health and Automate for notification or remediation flows.
  - Docs: [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/), [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/).

### Observability, governance, and security

- [ ] `AIPLE.45` Logic operational health (`P2`, `todo`)
  - Monitor failure rate, P95 duration, token/compute usage, tool failures, action failures, object query failures, model unavailability, run-history dataset failures, and automation proposal backlog.
  - Surface health in Logic detail, Workflow Lineage-like views, Data Health, and project dashboards.
  - Docs: [AIP Logic metrics](https://www.palantir.com/docs/foundry/logic/logic-metrics/), [AIP Logic compute usage](https://www.palantir.com/docs/foundry/logic/compute-usage).

- [ ] `AIPLE.46` AIP security and data minimization guardrails (`P2`, `todo`)
  - Show all resources, object types, properties, functions, actions, and media references that a Logic file exposes to LLM blocks.
  - Warn when prompts/tools expose broad object sets or sensitive properties.
  - Add policy hooks for redaction, prompt review, model allowlists, and export/logging restrictions where local governance exists.
  - Docs: [AIP Logic overview](https://www.palantir.com/docs/foundry/logic), [AIP features](https://www.palantir.com/docs/foundry/aip/aip-features/).

- [ ] `AIPLE.47` Audit event stream (`P2`, `todo`)
  - Emit immutable audit events for Logic creation/edit/publish/delete, tool/resource exposure, execution mode changes, run invocations, action/automation uses, Evals suite changes, evaluator changes, run results, experiments, and result dataset writes.
  - Filter audit by Logic file, suite, target, user, project, model, object type, action type, run, and time window.
  - Docs: [AIP Logic execution mode settings](https://www.palantir.com/docs/foundry/logic/execution-mode-settings/), [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/).

- [ ] `AIPLE.48` Branch-aware Evals and result isolation (`P2`, `todo`)
  - Run evaluation suites against branched Logic resources and branch-scoped Ontology/function/action dependencies.
  - Keep branch result datasets and run histories isolated from main unless explicitly published or exported.
  - Compare branch runs to main baselines before merge.
  - Docs: [Branching AIP Logic](https://www.palantir.com/docs/foundry/logic/branching-logic), [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/).

- [ ] `AIPLE.49` AIP Evals API and SDK surface (`P2`, `todo`)
  - Provide OpenFoundry-native APIs for suite CRUD, target functions, test cases, evaluators, run configuration, run execution, result retrieval, result dataset config, experiments, and analyzer jobs.
  - Generate SDK helpers for creating suites, running regression checks, and comparing metrics in CI-like workflows.
  - Docs: [AIP Evals overview](https://www.palantir.com/docs/foundry/aip-evals/overview/), [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite), [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/).

- [ ] `AIPLE.50` Marketplace and reusable evaluator packages (`P2`, `todo`)
  - Package evaluator functions, test case templates, Logic files, and example suites as OpenFoundry product outputs where DevOps/Marketplace exists.
  - Support installation/remapping of evaluator dependencies and target function placeholders.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite), [AIP features](https://www.palantir.com/docs/foundry/aip/aip-features/).

- [ ] `AIPLE.51` CI/CD and code-authored function parity (`P2`, `todo`)
  - Trigger AIP Evals for code-authored functions from code repository published function pages or CI-like checks.
  - Compare code-authored function versions against Logic and agent-like targets in the same suite.
  - Store results as release evidence before publishing function packages.
  - Docs: [Create an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/create-suite), [Run an evaluation suite](https://www.palantir.com/docs/foundry/aip-evals/run-suite/).

- [ ] `AIPLE.52` Cost-aware experiment and eval planning (`P2`, `todo`)
  - Estimate run count, block executions, target invocations, evaluator invocations, LLM/tool usage, and expected cost before running suites or experiments.
  - Enforce per-project or per-user budgets and require confirmation for high-cost experiment grids.
  - Docs: [AIP Logic compute usage](https://www.palantir.com/docs/foundry/logic/compute-usage), [Run experiments](https://www.palantir.com/docs/foundry/aip-evals/experiments/).

## Implementation inventory to collect before coding

- [ ] `INV.1` Identify existing OpenFoundry Logic-like, function-builder, prompt-builder, or workflow-builder models that can store inputs, blocks, outputs, versions, and published functions.
- [ ] `INV.2` Identify existing LLM runtime, model registry, AI-service, tool-calling, prompt templating, token usage, and model allowlist primitives.
- [ ] `INV.3` Identify existing Ontology query, object set, action execution, function execution, media reference, calculator, conditional, and loop primitives usable as Logic tools/blocks.
- [ ] `INV.4` Identify existing debugger, run history, telemetry, audit, Workflow Lineage, Data Health, and metrics primitives for Logic executions.
- [ ] `INV.5` Identify existing Functions service package/version/callable-function contracts needed to publish Logic functions and use them in Workshop, Actions, Automate, API, and other Logic functions.
- [ ] `INV.6` Identify existing Automate and action proposal primitives needed for Logic edit staging, approval, proposal visibility, and decision logs.
- [ ] `INV.7` Identify existing Global Branching adapters for Logic files and versioned function resources.
- [ ] `INV.8` Identify existing evaluation/test-case/evaluator models, unit test primitives, run orchestration, result storage, and metrics dashboards.
- [ ] `INV.9` Identify existing built-in evaluator implementations, custom function evaluator support, Marketplace evaluator products, and LLM-as-judge capabilities.
- [ ] `INV.10` Identify existing object-set-backed dataset/test generation, object storage v2 linked object/property traversal, and saved object set APIs.
- [ ] `INV.11` Identify existing result dataset creation/write permissions, project-scoped execution, dataset schema, and Data Foundation integration for Evals results.
- [ ] `INV.12` Identify existing security/governance, marking, project import, redaction, audit, checkpoint, and prompt/tool exposure review primitives.
- [ ] `INV.13` Identify existing code repository function publish pages and CI/CD hooks that can launch evaluation suites for code-authored functions.
- [ ] `INV.14` Produce a machine-readable parity matrix sibling JSON after inventory, following the pattern of [foundry-feature-parity-matrix.json](./foundry-feature-parity-matrix.json).

## Suggested service boundaries

| Surface | Responsibilities |
| --- | --- |
| `logic-service` | Logic file CRUD, inputs/blocks/outputs, draft saves, publish/version history, comparison view, execution mode settings, branch adapter metadata. |
| `logic-runtime-service` | Logic execution graph runtime, LLM blocks, tools, prompts, loops/conditionals, Ontology edit simulation, run panel execution, debugger traces. |
| `ai-service` | Model selection, LLM invocation, token/compute usage, model variables, model allowlists, prompt/tool-call trace storage. |
| `ontology-query-service` | Query objects tool, object set inputs, object list inputs, linked object traversal, permission-aware object data access. |
| `ontology-actions-service` | Apply action tool, Logic-backed actions, action proposal staging, action execution, Ontology edit validation. |
| `functions service` | Execute function tool, published Logic function registration, function version resolution, Workshop/function/action/API invocation contracts. |
| `automation service` | Logic-to-Automate integration, Logic effects, staged human review, proposal activity, automation run links. |
| `aip-evals-service` | Evaluation suites, target functions, test cases, evaluators, run configurations, run orchestration, experiments, analyzer jobs. |
| `eval-results service` | Result persistence, result datasets, metrics dashboards, run comparisons, debug views, trace viewer links. |
| `dataset-versioning-service` | Project-scoped run-history datasets, evaluation results datasets, schema/write validation, lineage to Logic/Evals resources. |
| `global-branch-service` | Branch-scoped Logic resources, proposal participation, rebase/merge checks, branch-specific Evals results isolation. |
| `security/governance service` | Execution-mode permission checks, project imports, resource exposure review, redaction, audit, checkpoint and model policy hooks. |
| `data-health service` | Logic health, eval regression health, run-history dataset health, automation proposal backlog, failed metric alerts. |
| `apps/web` | Logic editor, run panel, debugger, uses sidebar, metrics pages, Evals suite editor, result views, analyzer UI, experiment UI. |

## Acceptance criteria for first complete AIP Logic and Evals milestone

- [ ] A user can create a Logic file in a project folder, define typed inputs, add Use LLM / Query objects / Execute function / Apply action / Calculator / variable blocks, and define final outputs.
- [ ] A user can run draft Logic with sample inputs, inspect block-by-block debugger traces, and see recent run history.
- [ ] Logic preview can propose Ontology edits in simulation without applying them to real objects.
- [ ] A user can save, publish, view version history, compare versions, and use the published Logic function in Workshop or an action-backed flow.
- [ ] Logic execution supports user-scoped permissions and records basic metrics: success/failure counts, P95 duration, failure categories, and recent run links.
- [ ] A user can create an evaluation suite from Logic, add manual test cases, configure built-in evaluators, run the full suite, and run a single test case.
- [ ] Evaluation results show aggregate pass percentage, per-test-case metric results, evaluator outputs, errors, and debugger links.
- [ ] A user can add multiple target functions and compare results across versions/models/functions.
- [ ] A user can expose an intermediate block output and evaluate it with an evaluator.
- [ ] Evaluation suites can evaluate Logic functions that create/edit/delete Ontology objects in simulation without mutating real objects.
- [ ] Project-scoped evaluation runs can write results to a configured dataset with outputs, evaluator results, metadata, and errors.
- [ ] Results analyzer can summarize failed test cases into categories and propose prompt improvements for Logic functions.
- [ ] All OpenFoundry runtime UI is OpenFoundry-native and does not use Palantir branding, screenshots, icons, fonts, or proprietary assets.

## Test plan expectations

- Unit tests for Logic input type validation, block graph validation, prompt variable substitution, LLM tool configuration, object query permission filtering, Apply action simulation, loop/conditional semantics, output type validation, version diffing, execution mode permission decisions, and compute usage attribution.
- API tests for Logic file CRUD, save/publish/version history, run preview, debugger traces, usage snippets, execution mode settings, metrics, evaluation suite CRUD, target functions, test case columns, evaluators, run configs, run execution, experiments, result dataset config, and analyzer jobs.
- Integration tests for Logic querying Ontology objects, Logic applying simulated actions, Logic-backed Workshop display values, Logic-backed action execution, Logic-to-Automate proposal staging, user/project execution modes, result datasets, branch-scoped Logic resources, and Data Health metrics.
- Evals integration tests for manual test cases, object-set-backed test cases, built-in evaluators, custom function evaluators, intermediate parameters, Ontology-edit simulations, multi-target comparisons, iterations/parallelization, single-test runs, experiments, results analyzer, and metrics dashboard traces.
- E2E tests for Logic authoring, run/debug, publish, version compare, Uses sidebar, Automate creation, Evals suite creation, Add-as-test-case from preview, suite run, single-case debug, run comparison, experiment run, results dataset setup, and analyzer prompt suggestions.
- Regression tests proving Logic preview cannot mutate the real Ontology, user-scoped logs are not visible to other users, project-scoped execution requires imported resources, unauthorized object properties are not exposed to LLM blocks, result datasets cannot be written outside permitted projects, and branch-only Logic versions cannot leak into main runtime use.
