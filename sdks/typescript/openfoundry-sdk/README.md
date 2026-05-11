# OpenFoundry TypeScript SDK

Generated from `apps/web/public/generated/openapi/openfoundry.json`.

Version: `0.1.0`

## Usage

```ts
import { OpenFoundryClient } from '@open-foundry/sdk';

const client = new OpenFoundryClient({
  baseUrl: 'https://platform.example.com',
  token: '<token>',
  timeoutMs: 15_000,
  retry: { maxAttempts: 2 },
});

const me = await client.auth.getme();
const datasets = await client.dataset.listdatasets({ search: 'sales' });
```

## MCP bridging

```ts
import { OPENFOUNDRY_MCP_TOOLS, callOpenFoundryMcpTool } from '@open-foundry/sdk/mcp';

const result = await callOpenFoundryMcpTool(client, OPENFOUNDRY_MCP_TOOLS[0].name, {
  query: { page: 1, per_page: 20 },
});
```

## React helpers

```ts
import { OpenFoundryProvider, useOpenFoundry, useOpenFoundryQuery } from '@open-foundry/sdk/react';

function DatasetCount() {
  const client = useOpenFoundry();
  const datasets = useOpenFoundryQuery(() => client.dataset.listdatasets(), [client]);
  return <div>{datasets.data?.datasets?.length ?? 0}</div>;
}

function App() {
  return (
    <OpenFoundryProvider options={{ baseUrl: 'https://platform.example.com', token: '<token>' }}>
      <DatasetCount />
    </OpenFoundryProvider>
  );
}
```
