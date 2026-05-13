export const LOGIC_INPUT_TYPES = [
  'array',
  'list',
  'boolean',
  'date',
  'double',
  'float',
  'integer',
  'long',
  'media_reference',
  'model',
  'object',
  'object_list',
  'object_set',
  'short',
  'string',
  'struct',
  'timestamp',
] as const;

export type LogicInputType = (typeof LOGIC_INPUT_TYPES)[number];

export interface LogicInputDefinition {
  id: string;
  name: string;
  apiName: string;
  type: LogicInputType;
  required: boolean;
  defaultValue?: string;
  description?: string;
  objectTypeId?: string;
  objectSetObjectTypeId?: string;
  modelVariableKind?: 'llm' | 'embedding' | 'vision' | 'reranker';
  compatibleModelKinds?: Array<'llm' | 'embedding' | 'vision' | 'reranker'>;
  mediaSetRid?: string;
}

export interface LogicInputValidationIssue {
  field: string;
  message: string;
}

const API_NAME_PATTERN = /^[A-Za-z][A-Za-z0-9_]*$/;

export function isValidLogicInputApiName(apiName: string): boolean {
  return API_NAME_PATTERN.test(apiName);
}

export function inputTypeRequiresObjectType(type: LogicInputType): boolean {
  return type === 'object' || type === 'object_list' || type === 'object_set';
}

function parseJson(value: string): unknown {
  return JSON.parse(value);
}

function validateDefaultValue(input: LogicInputDefinition, issues: LogicInputValidationIssue[]) {
  const raw = input.defaultValue?.trim();
  if (!raw) return;

  switch (input.type) {
    case 'boolean':
      if (raw !== 'true' && raw !== 'false') {
        issues.push({ field: 'defaultValue', message: 'Boolean defaults must be true or false.' });
      }
      break;
    case 'integer':
    case 'long':
      if (!/^-?\d+$/.test(raw)) {
        issues.push({ field: 'defaultValue', message: `${input.type} defaults must be whole numbers.` });
      }
      break;
    case 'short': {
      const parsed = Number(raw);
      if (!/^-?\d+$/.test(raw) || parsed < -32768 || parsed > 32767) {
        issues.push({ field: 'defaultValue', message: 'Short defaults must be whole numbers between -32768 and 32767.' });
      }
      break;
    }
    case 'double':
    case 'float':
      if (!Number.isFinite(Number(raw))) {
        issues.push({ field: 'defaultValue', message: `${input.type} defaults must be numeric.` });
      }
      break;
    case 'date':
      if (!/^\d{4}-\d{2}-\d{2}$/.test(raw) || Number.isNaN(Date.parse(`${raw}T00:00:00Z`))) {
        issues.push({ field: 'defaultValue', message: 'Date defaults must use YYYY-MM-DD.' });
      }
      break;
    case 'timestamp':
      if (!raw.includes('T') || Number.isNaN(Date.parse(raw))) {
        issues.push({ field: 'defaultValue', message: 'Timestamp defaults must be ISO-8601 date-times.' });
      }
      break;
    case 'array':
    case 'list':
      try {
        if (!Array.isArray(parseJson(raw))) {
          issues.push({ field: 'defaultValue', message: `${input.type} defaults must be JSON arrays.` });
        }
      } catch {
        issues.push({ field: 'defaultValue', message: `${input.type} defaults must be valid JSON arrays.` });
      }
      break;
    case 'struct':
      try {
        const parsed = parseJson(raw);
        if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
          issues.push({ field: 'defaultValue', message: 'Struct defaults must be JSON objects.' });
        }
      } catch {
        issues.push({ field: 'defaultValue', message: 'Struct defaults must be valid JSON objects.' });
      }
      break;
    case 'object':
    case 'object_list':
    case 'object_set':
    case 'media_reference':
    case 'model':
    case 'string':
      break;
  }
}

export function validateLogicInputDefinition(input: LogicInputDefinition): LogicInputValidationIssue[] {
  const issues: LogicInputValidationIssue[] = [];
  if (!input.name.trim()) {
    issues.push({ field: 'name', message: 'Display name is required.' });
  }
  if (!isValidLogicInputApiName(input.apiName)) {
    issues.push({ field: 'apiName', message: 'API name must start with a letter and contain only letters, numbers, and underscores.' });
  }
  if (!LOGIC_INPUT_TYPES.includes(input.type)) {
    issues.push({ field: 'type', message: 'Input type is not supported by the Logic input board.' });
  }
  if (inputTypeRequiresObjectType(input.type) && !input.objectTypeId?.trim()) {
    issues.push({ field: 'objectTypeId', message: `${input.type} inputs require an object type selection.` });
  }
  if (input.type === 'object_set' && input.objectSetObjectTypeId && input.objectSetObjectTypeId !== input.objectTypeId) {
    issues.push({ field: 'objectSetObjectTypeId', message: 'Object set backing type must match the selected object type.' });
  }
  if (input.type === 'model') {
    if (!input.modelVariableKind) {
      issues.push({ field: 'modelVariableKind', message: 'Model inputs require a model variable kind.' });
    } else if (input.compatibleModelKinds?.length && !input.compatibleModelKinds.includes(input.modelVariableKind)) {
      issues.push({ field: 'modelVariableKind', message: 'Model variable kind is not compatible with the selected model slot.' });
    }
  }
  validateDefaultValue(input, issues);
  return issues;
}

export function validateLogicInputBoard(inputs: LogicInputDefinition[]): LogicInputValidationIssue[] {
  const issues = inputs.flatMap((input) =>
    validateLogicInputDefinition(input).map((issue) => ({
      ...issue,
      field: `${input.apiName || input.id}.${issue.field}`,
    })),
  );
  const seen = new Set<string>();
  for (const input of inputs) {
    const normalized = input.apiName.trim().toLowerCase();
    if (!normalized) continue;
    if (seen.has(normalized)) {
      issues.push({ field: input.apiName, message: 'Input API names must be unique within a Logic file.' });
    }
    seen.add(normalized);
  }
  return issues;
}
