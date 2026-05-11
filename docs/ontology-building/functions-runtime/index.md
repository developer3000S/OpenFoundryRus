# Functions by runtime

Functions are likely to evolve into a multi-runtime capability in OpenFoundry.

## Current runtime signals

The repo already suggests several function execution modes:

- Rust-native control-plane logic
- Node-oriented package simulation and validation via `node_runtime_command`
- Python-enabled semantics through `pyo3` in `ontology-service`
- SDK-backed external consumption paths
- Data Connection external Functions through configured webhooks via
  `context.sdk.dataConnection.invokeWebhook({ sourceId, inputs })`

## Why this matters

Separating function capability by runtime helps document:

- authoring ergonomics
- execution limits
- packaging requirements
- security and permission models
- external egress governance through Data Connection sources instead of
  ad hoc runtime HTTP calls

## Section map

- [Function package lifecycle](/ontology-building/functions-runtime/function-package-lifecycle)
- [Language and runtime comparison](/ontology-building/functions-runtime/language-and-runtime-comparison)
- [Validation and simulation flow](/ontology-building/functions-runtime/validation-and-simulation-flow)
