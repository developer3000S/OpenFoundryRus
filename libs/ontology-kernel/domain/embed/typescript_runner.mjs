import fs from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

function normalizeBaseUrl(value) {
  if (!value) throw new Error('Base URL is required');
  return value.endsWith('/') ? value : `${value}/`;
}

function toSearchParams(query) {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value === undefined || value === null || value === '') continue;
    params.set(key, String(value));
  }
  const serialized = params.toString();
  return serialized ? `?${serialized}` : '';
}

function parseJson(text) {
  if (!text || !text.trim()) return null;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function errorMessage(method, path, status, payload) {
  if (payload && typeof payload === 'object' && payload.error) {
    return `${method} ${path} failed with ${status}: ${payload.error}`;
  }
  if (typeof payload === 'string' && payload.trim()) {
    return `${method} ${path} failed with ${status}: ${payload}`;
  }
  return `${method} ${path} failed with ${status}`;
}

function renderLogValue(value) {
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

async function main() {
  const [, , userFilePath, inputFilePath] = process.argv;
  const input = JSON.parse(await fs.readFile(inputFilePath, 'utf8'));
  const stdout = [];
  const stderr = [];
  const originalFetch = globalThis.fetch.bind(globalThis);
  const blockedCapability = (name) => async () => {
    throw new Error(`${name} capability is disabled for this function package`);
  };

  console.log = (...args) => stdout.push(args.map(renderLogValue).join(' '));
  console.error = (...args) => stderr.push(args.map(renderLogValue).join(' '));

  function toUrl(resource, baseUrl) {
    if (typeof resource === 'string' || resource instanceof URL) {
      return new URL(resource, normalizeBaseUrl(baseUrl));
    }
    if (resource && typeof resource.url === 'string') {
      return new URL(resource.url, normalizeBaseUrl(baseUrl));
    }
    throw new Error('Unsupported fetch resource');
  }

  async function guardedFetch(resource, init) {
    const resolvedUrl = toUrl(resource, input.ontologyServiceUrl);
    const allowedOrigins = new Set(
      [input.ontologyServiceUrl, input.aiServiceUrl, input.connectorManagementServiceUrl]
        .filter(Boolean)
        .map((baseUrl) => new URL(baseUrl).origin),
    );
    if (!input.policy?.allowNetwork && !allowedOrigins.has(resolvedUrl.origin)) {
      throw new Error(`Network access is disabled for ${resolvedUrl.origin}`);
    }
    return originalFetch(resource, init);
  }

  globalThis.fetch = guardedFetch;

  async function request(baseUrl, method, path, body, query) {
    const url = new URL(path.replace(/^\//, ''), normalizeBaseUrl(baseUrl));
    const suffix = toSearchParams(query);
    if (suffix) {
      url.search = suffix.slice(1);
    }

    const headers = {
      authorization: input.serviceToken,
    };
    if (body !== undefined) {
      headers['content-type'] = 'application/json';
    }

    const response = await guardedFetch(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    const text = await response.text();
    const payload = parseJson(text);
    if (!response.ok) {
      throw new Error(errorMessage(method, path, response.status, payload));
    }
    return payload;
  }

  const allowOntologyRead = input.policy?.allowOntologyRead !== false;
  const allowOntologyWrite = input.policy?.allowOntologyWrite !== false;
  const allowAi = input.policy?.allowAi !== false;

  async function invokeDataConnectionWebhook({ sourceId, sourceRid, webhookId, inputs = {} } = {}) {
    const id = sourceId ?? sourceRid ?? webhookId;
    if (!id || typeof id !== 'string') {
      throw new Error('dataConnection.invokeWebhook requires sourceId');
    }
    if (!input.connectorManagementServiceUrl) {
      throw new Error('Data Connection webhook invocation is not configured for this function runtime');
    }
    return request(
      input.connectorManagementServiceUrl,
      'POST',
      `/api/v1/webhooks/${encodeURIComponent(id)}/invoke`,
      { inputs },
    );
  }

  const sdk = {
    ontology: {
      getObject: allowOntologyRead
        ? ({ typeId, objectId }) =>
            request(input.ontologyServiceUrl, 'GET', `/api/v1/ontology/types/${typeId}/objects/${objectId}`)
        : blockedCapability('ontology.read'),
      updateObject: allowOntologyWrite
        ? ({ typeId, objectId, properties, replace = false, marking }) =>
            request(input.ontologyServiceUrl, 'PATCH', `/api/v1/ontology/types/${typeId}/objects/${objectId}`, {
              properties,
              replace,
              marking,
            })
        : blockedCapability('ontology.write'),
      queryObjects: allowOntologyRead
        ? ({ typeId, equals = {}, limit }) =>
            request(input.ontologyServiceUrl, 'POST', `/api/v1/ontology/types/${typeId}/objects/query`, {
              equals,
              limit,
            })
        : blockedCapability('ontology.read'),
      knnObjects: allowOntologyRead
        ? ({ typeId, propertyName, anchorObjectId, queryVector, limit, metric, excludeAnchor }) =>
            request(input.ontologyServiceUrl, 'POST', `/api/v1/ontology/types/${typeId}/objects/knn`, {
              property_name: propertyName,
              anchor_object_id: anchorObjectId,
              query_vector: queryVector,
              limit,
              metric,
              exclude_anchor: excludeAnchor,
            })
        : blockedCapability('ontology.read'),
      listNeighbors: allowOntologyRead
        ? ({ typeId, objectId }) =>
            request(input.ontologyServiceUrl, 'GET', `/api/v1/ontology/types/${typeId}/objects/${objectId}/neighbors`)
        : blockedCapability('ontology.read'),
      createLink: allowOntologyWrite
        ? ({ linkTypeId, sourceObjectId, targetObjectId, properties }) =>
            request(input.ontologyServiceUrl, 'POST', `/api/v1/ontology/links/${linkTypeId}/instances`, {
              source_object_id: sourceObjectId,
              target_object_id: targetObjectId,
              properties,
            })
        : blockedCapability('ontology.write'),
      search: allowOntologyRead
        ? ({ query, kind, objectTypeId, limit, semantic = true }) =>
            request(input.ontologyServiceUrl, 'POST', '/api/v1/ontology/search', {
              query,
              kind,
              object_type_id: objectTypeId,
              limit,
              semantic,
            })
        : blockedCapability('ontology.read'),
      graph: allowOntologyRead
        ? ({ rootObjectId, rootTypeId, depth, limit } = {}) =>
            request(input.ontologyServiceUrl, 'GET', '/api/v1/ontology/graph', undefined, {
              root_object_id: rootObjectId,
              root_type_id: rootTypeId,
              depth,
              limit,
            })
        : blockedCapability('ontology.read'),
    },
    ai: {
      complete: allowAi
        ? ({
            userMessage,
            systemPrompt,
            preferredProviderId,
            knowledgeBaseId,
            temperature = 0.2,
            maxTokens = 512,
          }) =>
            request(input.aiServiceUrl, 'POST', '/api/v1/ai/chat/completions', {
              user_message: userMessage,
              system_prompt: systemPrompt,
              preferred_provider_id: preferredProviderId,
              knowledge_base_id: knowledgeBaseId,
              fallback_enabled: true,
              temperature,
              max_tokens: maxTokens,
            })
        : blockedCapability('ai.complete'),
    },
    dataConnection: {
      invokeWebhook: invokeDataConnectionWebhook,
      webhooks: {
        invoke: invokeDataConnectionWebhook,
      },
    },
  };

  const llm = {
    complete: sdk.ai.complete,
  };

  try {
    const userModule = await import(pathToFileURL(userFilePath).href);
    const preferredEntrypoint = input.functionPackage?.entrypoint;
    const handler =
      (preferredEntrypoint === 'default' ? userModule.default : undefined) ??
      (preferredEntrypoint && preferredEntrypoint !== 'default' ? userModule[preferredEntrypoint] : undefined) ??
      userModule.default ??
      userModule.handler;
    if (typeof handler !== 'function') {
      throw new Error(
        'TypeScript function must export a default async function or a named handler(context)',
      );
    }

    const context = {
      ...input.context,
      sdk,
      llm,
      functionPackage: input.functionPackage ?? null,
      capabilities: input.policy ?? {},
    };

    const result = await handler(context);
    process.stdout.write(JSON.stringify({ result, stdout, stderr }));
  } catch (error) {
    process.stdout.write(
      JSON.stringify({
        result: null,
        stdout,
        stderr,
        error: {
          message: error?.stack ?? String(error),
        },
      }),
    );
    process.exitCode = 1;
  }
}

await main();
