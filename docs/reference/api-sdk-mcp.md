# API, SDK, and MCP

OpenFoundry treats contracts and client surfaces as generated platform assets, not hand-maintained side projects.

## Source of Truth

The repository uses `tools/of-cli` to generate and validate several outputs:

- OpenAPI
- TypeScript SDK
- Python SDK
- Java SDK
- Terraform provider schema

## Generated Artifact Locations

| Artifact | Location |
| --- | --- |
| OpenAPI contract | `apps/web/public/generated/openapi/openfoundry.json` |
| TypeScript SDK | `sdks/typescript/openfoundry-sdk` |
| Python SDK | `sdks/python/openfoundry-sdk` |
| Java SDK | `sdks/java/openfoundry-sdk` |
| Terraform provider schema | `infra/terraform/providers/openfoundry/provider.schema.json` |
| Frontend Terraform schema | `apps/web/public/generated/terraform/openfoundry-provider.json` |

## Core Recipes

The root `Makefile` exposes the main workflows:

```bash
make openapi-gen
make openapi-check
make sdk-typescript-gen
make sdk-typescript-check
make sdk-python-gen
make sdk-python-check
make sdk-java-gen
make sdk-java-check
make sdk-check
make contracts-gen
make contracts-check
```

## MCP Surface

The SDK layer includes MCP-oriented surfaces in both generated client stacks:

- `sdks/typescript/openfoundry-sdk/src/mcp.ts`
- `sdks/python/openfoundry-sdk/openfoundry_sdk/mcp.py`

These surfaces let the repository expose a more agent-friendly operation model on top of the generated contract set.

## Why This Matters

Keeping generated contracts in-repo gives OpenFoundry several advantages:

- frontend and backend evolve in lockstep
- SDK changes are visible in pull requests
- CI can validate that checked-in outputs still match the generator
- external integration surfaces become part of normal platform review
