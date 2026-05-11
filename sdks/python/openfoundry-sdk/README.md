# OpenFoundry Python SDK

Generated from `apps/web/public/generated/openapi/openfoundry.json`.

Version: `0.1.0`

## Usage

```python
from openfoundry_sdk import OpenFoundryClient

client = OpenFoundryClient(
    base_url="https://platform.example.com",
    token="<token>",
    timeout_seconds=15,
    max_retries=2,
)

me = client.auth.auth_get_me()
datasets = client.dataset.listdatasets({"search": "sales"})
```

## MCP bridging

```python
from openfoundry_sdk.mcp import MCP_TOOL_REGISTRY, call_openfoundry_mcp_tool

result = call_openfoundry_mcp_tool(client, MCP_TOOL_REGISTRY[0]["name"], {"query": {"page": 1, "per_page": 20}})
```
