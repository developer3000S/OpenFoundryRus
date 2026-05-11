# Contracts and SDK generation

Contracts are generated assets in OpenFoundry, not secondary documentation.

## Pipeline shape

The generation path is:

```text
proto/*
  -> of-cli docs generate / validate commands
  -> checked-in OpenAPI output
  -> generated TypeScript, Python, and Java SDKs
  -> frontend and integration consumers
```

## Repository signals

The CLI surface lives in `tools/of-cli`, where the `docs` command group exposes:

- `generate-openapi`
- `validate-openapi`
- `generate-sdk-typescript`
- `validate-sdk-typescript`
- `generate-sdk-python`
- `validate-sdk-python`
- `generate-sdk-java`
- `validate-sdk-java`

## Artifact destinations

- `apps/web/public/generated/openapi/openfoundry.json`
- `sdks/typescript/openfoundry-sdk`
- `sdks/python/openfoundry-sdk`
- `sdks/java/openfoundry-sdk`

## Why this matters

This flow is one of the cleanest anti-drift mechanisms in the repo. It makes contract changes visible in code review and allows CI to reject mismatches early.

## Local commands

Run from the repository root:

```sh
make openapi-check
make sdk-check
make contracts-check
make contracts-gen
```
