import { isValidLogicInputApiName, type LogicInputDefinition, type LogicInputType } from './inputs';

export type LogicStructuredOutputKind = 'text' | 'json_schema' | 'object' | 'object_list' | 'ontology_edit_bundle';
export type LogicToolKind = 'query_objects' | 'apply_action' | 'execute_function' | 'calculator';
export type LogicSeverity = 'error' | 'warning';
export type LogicFunctionKind = 'typescript' | 'python' | 'existing_logic' | 'function_on_objects';
export type LogicValueType = LogicInputType | 'json' | 'ontology_edit_bundle';

export interface LogicIssue {
  severity: LogicSeverity;
  field: string;
  message: string;
}

export interface LogicStructuredOutputConfig {
  kind: LogicStructuredOutputKind;
  schemaJson?: string;
}

export interface LogicQueryObjectsToolConfig {
  kind: 'query_objects';
  name: string;
  objectTypeId: string;
  selectedProperties: string[];
  readableObjectTypeIds: string[];
  readablePropertiesByObjectType: Record<string, string[]>;
  maxObjects: number;
}

export interface LogicActionToolConfig {
  kind: 'apply_action';
  name: string;
  actionTypeId: string;
  allowedActionTypeIds: string[];
  expectedParameters: Record<string, LogicValueType>;
  parameterMappings: Record<string, string>;
  invocationMode: 'preview' | 'commit';
  invocationSurface: 'draft_preview' | 'published_action' | 'automation';
  logicPublished: boolean;
}

export interface LogicExecuteFunctionToolConfig {
  kind: 'execute_function';
  name: string;
  functionRid: string;
  functionKind: LogicFunctionKind;
  allowedFunctionRids: string[];
  signature: {
    parameters: Record<string, LogicValueType>;
    returnType: LogicValueType;
  };
  parameterMappings: Record<string, string>;
  expectedOutputType: LogicValueType;
}

export interface LogicCalculatorToolConfig {
  kind: 'calculator';
  name: string;
  expression: string;
  parameterRefs: string[];
  outputType: 'integer' | 'double';
}

export type LogicToolConfig =
  | LogicQueryObjectsToolConfig
  | LogicActionToolConfig
  | LogicExecuteFunctionToolConfig
  | LogicCalculatorToolConfig;

export interface LogicLlmBlockConfig {
  id: string;
  name: string;
  modelBinding: {
    mode: 'fixed' | 'model_variable';
    providerId?: string;
    modelVariableApiName?: string;
  };
  systemPrompt: string;
  taskPrompt: string;
  promptVariableRefs: string[];
  toolAccess: LogicToolConfig[];
  structuredOutput: LogicStructuredOutputConfig;
  maxOutputTokens: number;
}

export interface LogicDebuggerTraceMetadata {
  blockId: string;
  renderedPrompt: {
    system: string;
    task: string;
    variables: string[];
  };
  toolCalls: Array<{
    toolName: string;
    kind: LogicToolKind;
    objectTypeId?: string;
    actionTypeId?: string;
    functionRid?: string;
    expression?: string;
    selectedProperties?: string[];
    parameterMappings?: Record<string, string>;
    maxObjects?: number;
    status: 'not_run' | 'preview_only' | 'ok' | 'error';
  }>;
  proposedOntologyEdits: Array<{
    actionTypeId: string;
    parameters: Record<string, string>;
    applyState: 'preview_only' | 'ready_for_commit';
  }>;
  output: {
    structuredOutputKind: LogicStructuredOutputKind;
    preview: string;
  };
  tokenUsage: {
    promptTokensEstimate: number;
    maxOutputTokens: number;
    computeUnitsEstimate: number;
  };
  errors: LogicIssue[];
}

const TOKEN_CHARS = 4;
const PROPERTY_WARNING_THRESHOLD = 12;
const TOOL_OBJECT_WARNING_THRESHOLD = 3;
const CALCULATOR_EXPRESSION_PATTERN = /^[\d\s+\-*/()._%A-Za-z]+$/;

export function estimatePromptTokens(text: string): number {
  return Math.ceil(text.length / TOKEN_CHARS);
}

function knownInputApiNames(inputs: LogicInputDefinition[]): Set<string> {
  return new Set(inputs.map((input) => input.apiName));
}

function inputTypeByApiName(inputs: LogicInputDefinition[]): Map<string, LogicInputType> {
  return new Map(inputs.map((input) => [input.apiName, input.type]));
}

function valueTypeCompatible(inputType: LogicInputType | undefined, expected: LogicValueType): boolean {
  if (!inputType) return false;
  if (expected === 'json') return true;
  if (expected === 'double') return ['double', 'float', 'integer', 'long', 'short'].includes(inputType);
  if (expected === 'float') return ['float', 'integer', 'long', 'short'].includes(inputType);
  if (expected === 'long') return ['long', 'integer', 'short'].includes(inputType);
  if (expected === 'integer') return ['integer', 'short'].includes(inputType);
  if (expected === 'array') return inputType === 'array' || inputType === 'list';
  return inputType === expected;
}

function outputTypeCompatible(actual: LogicValueType | undefined, expected: LogicValueType): boolean {
  if (!actual) return false;
  if (expected === 'json') return true;
  if (expected === 'double') return ['double', 'float', 'integer', 'long', 'short'].includes(actual);
  if (expected === 'array') return actual === 'array' || actual === 'list';
  return actual === expected;
}

function validateStructuredOutput(output: LogicStructuredOutputConfig): LogicIssue[] {
  if (output.kind !== 'json_schema' || !output.schemaJson?.trim()) return [];
  try {
    const parsed = JSON.parse(output.schemaJson) as unknown;
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      return [{ severity: 'error', field: 'structuredOutput.schemaJson', message: 'Structured JSON schema must be a JSON object.' }];
    }
  } catch {
    return [{ severity: 'error', field: 'structuredOutput.schemaJson', message: 'Structured JSON schema must be valid JSON.' }];
  }
  return [];
}

export function validateQueryObjectsTool(tool: LogicQueryObjectsToolConfig): LogicIssue[] {
  const issues: LogicIssue[] = [];
  if (!tool.name.trim()) {
    issues.push({ severity: 'error', field: 'tool.name', message: 'Query objects tool name is required.' });
  }
  if (!tool.objectTypeId.trim()) {
    issues.push({ severity: 'error', field: 'tool.objectTypeId', message: 'Query objects requires an object type.' });
    return issues;
  }
  if (!tool.readableObjectTypeIds.includes(tool.objectTypeId)) {
    issues.push({ severity: 'error', field: 'tool.objectTypeId', message: 'Selected object type is not readable by this Logic function or user.' });
  }
  const readableProperties = new Set(tool.readablePropertiesByObjectType[tool.objectTypeId] ?? []);
  if (tool.selectedProperties.length === 0) {
    issues.push({ severity: 'error', field: 'tool.selectedProperties', message: 'Select at least one readable property for query results.' });
  }
  for (const property of tool.selectedProperties) {
    if (!readableProperties.has(property)) {
      issues.push({ severity: 'error', field: `tool.selectedProperties.${property}`, message: `Property ${property} is not readable on ${tool.objectTypeId}.` });
    }
  }
  if (tool.selectedProperties.length > PROPERTY_WARNING_THRESHOLD) {
    issues.push({ severity: 'warning', field: 'tool.selectedProperties', message: 'Exposing many properties can inflate prompts; select only fields the LLM needs.' });
  }
  if (tool.maxObjects > 50) {
    issues.push({ severity: 'warning', field: 'tool.maxObjects', message: 'Large object result limits can be token-expensive; consider lowering max objects or adding filters.' });
  }
  return issues;
}

export function validateApplyActionTool(tool: LogicActionToolConfig, inputs: LogicInputDefinition[]): LogicIssue[] {
  const issues: LogicIssue[] = [];
  const inputTypes = inputTypeByApiName(inputs);
  if (!tool.name.trim()) {
    issues.push({ severity: 'error', field: 'tool.name', message: 'Apply action tool name is required.' });
  }
  if (!tool.actionTypeId.trim()) {
    issues.push({ severity: 'error', field: 'tool.actionTypeId', message: 'Select an action type before exposing Apply action to the LLM.' });
  } else if (!tool.allowedActionTypeIds.includes(tool.actionTypeId)) {
    issues.push({ severity: 'error', field: 'tool.actionTypeId', message: 'Selected action type is not permitted for this Logic function or user.' });
  }
  for (const [parameter, expectedType] of Object.entries(tool.expectedParameters)) {
    const mappedInput = tool.parameterMappings[parameter];
    if (!mappedInput) {
      issues.push({ severity: 'error', field: `tool.parameterMappings.${parameter}`, message: `Action parameter ${parameter} must be mapped.` });
      continue;
    }
    if (!valueTypeCompatible(inputTypes.get(mappedInput), expectedType)) {
      issues.push({ severity: 'error', field: `tool.parameterMappings.${parameter}`, message: `Action parameter ${parameter} mapping is not type-compatible with ${expectedType}.` });
    }
  }
  if (tool.invocationMode === 'commit') {
    if (!tool.logicPublished) {
      issues.push({ severity: 'error', field: 'tool.invocationMode', message: 'Real Ontology edits require published Logic.' });
    }
    if (tool.invocationSurface !== 'published_action' && tool.invocationSurface !== 'automation') {
      issues.push({ severity: 'error', field: 'tool.invocationSurface', message: 'Real Ontology edits require action or automation invocation.' });
    }
  }
  return issues;
}

export function validateExecuteFunctionTool(tool: LogicExecuteFunctionToolConfig, inputs: LogicInputDefinition[]): LogicIssue[] {
  const issues: LogicIssue[] = [];
  const inputTypes = inputTypeByApiName(inputs);
  if (!tool.name.trim()) {
    issues.push({ severity: 'error', field: 'tool.name', message: 'Execute function tool name is required.' });
  }
  if (!tool.functionRid.trim()) {
    issues.push({ severity: 'error', field: 'tool.functionRid', message: 'Select a TypeScript, Python, existing Logic, or function-on-objects function.' });
  } else if (!tool.allowedFunctionRids.includes(tool.functionRid)) {
    issues.push({ severity: 'error', field: 'tool.functionRid', message: 'Selected function is not executable by this Logic function or user.' });
  }
  for (const [parameter, expectedType] of Object.entries(tool.signature.parameters)) {
    const mappedInput = tool.parameterMappings[parameter];
    if (!mappedInput) {
      issues.push({ severity: 'error', field: `tool.parameterMappings.${parameter}`, message: `Function parameter ${parameter} must be mapped.` });
      continue;
    }
    if (!valueTypeCompatible(inputTypes.get(mappedInput), expectedType)) {
      issues.push({ severity: 'error', field: `tool.parameterMappings.${parameter}`, message: `Function parameter ${parameter} mapping is not type-compatible with ${expectedType}.` });
    }
  }
  if (!outputTypeCompatible(tool.signature.returnType, tool.expectedOutputType)) {
    issues.push({ severity: 'error', field: 'tool.expectedOutputType', message: 'Function return type is not compatible with the configured tool output.' });
  }
  return issues;
}

function calculatorRefs(expression: string): string[] {
  return Array.from(new Set(expression.match(/[A-Za-z_][A-Za-z0-9_]*/g) ?? []));
}

export function validateCalculatorTool(tool: LogicCalculatorToolConfig, inputs: LogicInputDefinition[]): LogicIssue[] {
  const issues: LogicIssue[] = [];
  const inputTypes = inputTypeByApiName(inputs);
  if (!tool.name.trim()) {
    issues.push({ severity: 'error', field: 'tool.name', message: 'Calculator tool name is required.' });
  }
  if (!tool.expression.trim()) {
    issues.push({ severity: 'error', field: 'tool.expression', message: 'Calculator expression is required.' });
    return issues;
  }
  if (!CALCULATOR_EXPRESSION_PATTERN.test(tool.expression)) {
    issues.push({ severity: 'error', field: 'tool.expression', message: 'Calculator supports numbers, input variables, parentheses, and + - * / % only.' });
  }
  const refs = calculatorRefs(tool.expression);
  for (const ref of refs) {
    const inputType = inputTypes.get(ref);
    if (!inputType) {
      issues.push({ severity: 'error', field: `tool.expression.${ref}`, message: `Calculator variable ${ref} does not match a Logic input.` });
    } else if (!['double', 'float', 'integer', 'long', 'short'].includes(inputType)) {
      issues.push({ severity: 'error', field: `tool.expression.${ref}`, message: `Calculator variable ${ref} must be numeric.` });
    }
  }
  for (const ref of tool.parameterRefs) {
    if (!refs.includes(ref)) {
      issues.push({ severity: 'warning', field: `tool.parameterRefs.${ref}`, message: `Calculator parameter ${ref} is configured but not used in the expression.` });
    }
  }
  return issues;
}

export function evaluateCalculatorExpression(expression: string, values: Record<string, number>): number {
  const tokens = expression.match(/\d+(?:\.\d+)?|[A-Za-z_][A-Za-z0-9_]*|[()+\-*/%]/g) ?? [];
  const output: string[] = [];
  const operators: string[] = [];
  const precedence: Record<string, number> = { '+': 1, '-': 1, '*': 2, '/': 2, '%': 2 };
  for (const token of tokens) {
    if (/^\d/.test(token)) {
      output.push(token);
    } else if (/^[A-Za-z_]/.test(token)) {
      const value = values[token];
      if (typeof value !== 'number') throw new Error(`Missing calculator value for ${token}`);
      output.push(String(value));
    } else if (token === '(') {
      operators.push(token);
    } else if (token === ')') {
      while (operators.length && operators[operators.length - 1] !== '(') output.push(operators.pop() ?? '');
      if (operators.pop() !== '(') throw new Error('Mismatched parentheses');
    } else {
      while (operators.length && precedence[operators[operators.length - 1]] >= precedence[token]) output.push(operators.pop() ?? '');
      operators.push(token);
    }
  }
  while (operators.length) {
    const operator = operators.pop() ?? '';
    if (operator === '(') throw new Error('Mismatched parentheses');
    output.push(operator);
  }
  const stack: number[] = [];
  for (const token of output) {
    if (!Number.isNaN(Number(token))) {
      stack.push(Number(token));
      continue;
    }
    const b = stack.pop();
    const a = stack.pop();
    if (a === undefined || b === undefined) throw new Error('Invalid calculator expression');
    if (token === '+') stack.push(a + b);
    else if (token === '-') stack.push(a - b);
    else if (token === '*') stack.push(a * b);
    else if (token === '/') stack.push(a / b);
    else if (token === '%') stack.push(a % b);
  }
  if (stack.length !== 1) throw new Error('Invalid calculator expression');
  return stack[0];
}

export function validateLlmBlock(block: LogicLlmBlockConfig, inputs: LogicInputDefinition[]): LogicIssue[] {
  const issues: LogicIssue[] = [];
  const inputNames = knownInputApiNames(inputs);
  if (!block.name.trim()) {
    issues.push({ severity: 'error', field: 'name', message: 'Use LLM block name is required.' });
  }
  if (block.modelBinding.mode === 'fixed') {
    if (!block.modelBinding.providerId?.trim()) {
      issues.push({ severity: 'error', field: 'modelBinding.providerId', message: 'Select a fixed model provider or switch to a model variable.' });
    }
  } else {
    const apiName = block.modelBinding.modelVariableApiName ?? '';
    const modelInput = inputs.find((input) => input.apiName === apiName);
    if (!isValidLogicInputApiName(apiName)) {
      issues.push({ severity: 'error', field: 'modelBinding.modelVariableApiName', message: 'Model variable reference must be a valid input API name.' });
    } else if (!modelInput || modelInput.type !== 'model') {
      issues.push({ severity: 'error', field: 'modelBinding.modelVariableApiName', message: 'Model variable reference must point to a model input.' });
    }
  }
  if (!block.taskPrompt.trim()) {
    issues.push({ severity: 'error', field: 'taskPrompt', message: 'Task prompt is required for a Use LLM block.' });
  }
  for (const ref of block.promptVariableRefs) {
    if (!inputNames.has(ref)) {
      issues.push({ severity: 'error', field: `promptVariableRefs.${ref}`, message: `Prompt variable ${ref} does not match a Logic input.` });
    }
  }
  issues.push(...validateStructuredOutput(block.structuredOutput));
  for (const tool of block.toolAccess) {
    if (tool.kind === 'query_objects') issues.push(...validateQueryObjectsTool(tool));
    if (tool.kind === 'apply_action') issues.push(...validateApplyActionTool(tool, inputs));
    if (tool.kind === 'execute_function') issues.push(...validateExecuteFunctionTool(tool, inputs));
    if (tool.kind === 'calculator') issues.push(...validateCalculatorTool(tool, inputs));
  }
  if (block.toolAccess.filter((tool) => tool.kind === 'query_objects').length > TOOL_OBJECT_WARNING_THRESHOLD) {
    issues.push({ severity: 'warning', field: 'toolAccess', message: 'Too many query tools can expose more object context than the prompt needs.' });
  }
  const tokenEstimate = estimatePromptTokens(`${block.systemPrompt}\n${block.taskPrompt}`);
  if (tokenEstimate + block.maxOutputTokens > 8000) {
    issues.push({ severity: 'warning', field: 'maxOutputTokens', message: 'Prompt plus output budget may exceed common interactive model context windows.' });
  }
  return issues;
}

function toolHasError(tool: LogicToolConfig, issues: LogicIssue[]): boolean {
  const errorFields = issues.filter((issue) => issue.severity === 'error').map((issue) => issue.field);
  if (tool.kind === 'query_objects') {
    return errorFields.some((field) => field === 'tool.objectTypeId' || field === 'tool.selectedProperties' || field.startsWith('tool.selectedProperties.'));
  }
  if (tool.kind === 'apply_action') {
    return errorFields.some((field) => field === 'tool.actionTypeId' || field.startsWith('tool.parameterMappings.') || field.startsWith('tool.invocation'));
  }
  if (tool.kind === 'execute_function') {
    return errorFields.some((field) => field === 'tool.functionRid' || field.startsWith('tool.parameterMappings.') || field === 'tool.expectedOutputType');
  }
  if (tool.kind === 'calculator') return errorFields.some((field) => field.startsWith('tool.expression'));
  return false;
}

export function buildLlmDebuggerTrace(block: LogicLlmBlockConfig, inputs: LogicInputDefinition[]): LogicDebuggerTraceMetadata {
  const issues = validateLlmBlock(block, inputs);
  const promptTokensEstimate = estimatePromptTokens(`${block.systemPrompt}\n${block.taskPrompt}`);
  return {
    blockId: block.id,
    renderedPrompt: {
      system: block.systemPrompt,
      task: block.taskPrompt,
      variables: block.promptVariableRefs,
    },
    toolCalls: block.toolAccess.map((tool) => ({
      toolName: tool.name,
      kind: tool.kind,
      objectTypeId: tool.kind === 'query_objects' ? tool.objectTypeId : undefined,
      actionTypeId: tool.kind === 'apply_action' ? tool.actionTypeId : undefined,
      functionRid: tool.kind === 'execute_function' ? tool.functionRid : undefined,
      expression: tool.kind === 'calculator' ? tool.expression : undefined,
      selectedProperties: tool.kind === 'query_objects' ? tool.selectedProperties : undefined,
      parameterMappings: tool.kind === 'apply_action' || tool.kind === 'execute_function' ? tool.parameterMappings : undefined,
      maxObjects: tool.kind === 'query_objects' ? tool.maxObjects : undefined,
      status: toolHasError(tool, issues) ? 'error' : tool.kind === 'apply_action' && tool.invocationMode === 'preview' ? 'preview_only' : 'not_run',
    })),
    proposedOntologyEdits: block.toolAccess
      .filter((tool): tool is LogicActionToolConfig => tool.kind === 'apply_action')
      .map((tool) => ({
        actionTypeId: tool.actionTypeId,
        parameters: tool.parameterMappings,
        applyState: tool.invocationMode === 'commit' ? 'ready_for_commit' : 'preview_only',
      })),
    output: {
      structuredOutputKind: block.structuredOutput.kind,
      preview: block.structuredOutput.kind === 'text' ? 'LLM text response preview' : 'Structured output will be validated before final output mapping.',
    },
    tokenUsage: {
      promptTokensEstimate,
      maxOutputTokens: block.maxOutputTokens,
      computeUnitsEstimate: Math.max(1, Math.ceil((promptTokensEstimate + block.maxOutputTokens) / 1000)),
    },
    errors: issues,
  };
}

export type LogicVariableSource = 'literal' | 'input' | 'block_output';
export type LogicOutputSource = 'block_output' | 'intermediate' | 'ontology_edit_bundle';
export type WorkshopOutputUsage = 'none' | 'markdown_display' | 'general_display';

export interface LogicVariableBlockConfig {
  id: string;
  apiName: string;
  valueType: LogicValueType;
  source: LogicVariableSource;
  literalValue?: string;
  inputApiName?: string;
  blockOutputId?: string;
}

export interface LogicConditionalBranchConfig {
  id: string;
  conditionExpression?: string;
  outputType?: LogicValueType;
  returnsOntologyEdits?: boolean;
  takeNoAction?: boolean;
}

export interface LogicConditionalBlockConfig {
  id: string;
  conditionExpression: string;
  trueOutputType: LogicValueType;
  falseOutputType: LogicValueType;
  branches?: LogicConditionalBranchConfig[];
}

export interface LogicLoopBlockConfig {
  id: string;
  inputApiName: string;
  elementVariableApiName: string;
  indexVariableApiName: string;
  bodyOutputType: LogicValueType;
  outputAggregation: 'list' | 'first' | 'count' | 'none';
  finalOutputType: LogicValueType;
  containsActionTool: boolean;
  parallel: boolean;
  arrayToListInserted?: boolean;
}

export interface LogicOutputDefinition {
  id: string;
  name: string;
  apiName: string;
  outputType: LogicValueType;
  source: LogicOutputSource;
  sourceId: string;
  final: boolean;
  workshopUsage: WorkshopOutputUsage;
}

const LOGIC_OUTPUT_VALUE_TYPES = new Set<LogicValueType>([
  'array',
  'list',
  'boolean',
  'date',
  'double',
  'float',
  'integer',
  'long',
  'media_reference',
  'object',
  'object_list',
  'object_set',
  'short',
  'string',
  'struct',
  'timestamp',
  'json',
  'ontology_edit_bundle',
]);

const CREATE_VARIABLE_VALUE_TYPES = new Set<LogicValueType>([
  'array',
  'boolean',
  'date',
  'double',
  'float',
  'integer',
  'long',
  'object',
  'short',
  'string',
  'struct',
  'timestamp',
  'json',
]);

function isCollectionType(type: LogicValueType | undefined): boolean {
  return type === 'array' || type === 'list' || type === 'object_list' || type === 'object_set';
}

function isOntologyEditType(type: LogicValueType | undefined): boolean {
  return type === 'ontology_edit_bundle';
}

function literalCompatible(value: string | undefined, type: LogicValueType): boolean {
  const raw = value?.trim() ?? '';
  if (!raw) return false;
  if (type === 'string') return true;
  if (type === 'boolean') return raw === 'true' || raw === 'false';
  if (['short', 'integer', 'long'].includes(type)) return /^-?\d+$/.test(raw);
  if (['float', 'double'].includes(type)) return Number.isFinite(Number(raw));
  if (type === 'date') return /^\d{4}-\d{2}-\d{2}$/.test(raw) && !Number.isNaN(Date.parse(`${raw}T00:00:00Z`));
  if (type === 'timestamp') return raw.includes('T') && !Number.isNaN(Date.parse(raw));
  if (['json', 'struct', 'array', 'object'].includes(type)) {
    try {
      const parsed = JSON.parse(raw) as unknown;
      if (type === 'array') return Array.isArray(parsed);
      if (type === 'struct' || type === 'object') return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed);
      return true;
    } catch {
      return false;
    }
  }
  return false;
}

export function validateCreateVariableBlock(block: LogicVariableBlockConfig, inputs: LogicInputDefinition[], blockOutputTypes: Record<string, LogicValueType> = {}): LogicIssue[] {
  const issues: LogicIssue[] = [];
  const inputTypes = inputTypeByApiName(inputs);
  if (!isValidLogicInputApiName(block.apiName)) {
    issues.push({ severity: 'error', field: 'variable.apiName', message: 'Variable API name must start with a letter and contain only letters, numbers, and underscores.' });
  }
  if (!CREATE_VARIABLE_VALUE_TYPES.has(block.valueType)) {
    issues.push({ severity: 'error', field: 'variable.valueType', message: 'Create variable blocks support primitive, array, object, struct, and JSON-compatible values only.' });
  }
  if (block.source === 'literal' && !literalCompatible(block.literalValue, block.valueType)) {
    issues.push({ severity: 'error', field: 'variable.literalValue', message: `Literal value is not compatible with ${block.valueType}.` });
  }
  if (block.source === 'input') {
    const inputType = inputTypes.get(block.inputApiName ?? '');
    if (!valueTypeCompatible(inputType, block.valueType)) {
      issues.push({ severity: 'error', field: 'variable.inputApiName', message: `Input source is not compatible with ${block.valueType}.` });
    }
  }
  if (block.source === 'block_output') {
    const outputType = blockOutputTypes[block.blockOutputId ?? ''];
    if (!outputTypeCompatible(outputType, block.valueType)) {
      issues.push({ severity: 'error', field: 'variable.blockOutputId', message: `Block output source is not compatible with ${block.valueType}.` });
    }
  }
  return issues;
}

function branchOutputCompatible(expected: LogicValueType, actual: LogicValueType | undefined): boolean {
  return Boolean(actual && (outputTypeCompatible(actual, expected) || outputTypeCompatible(expected, actual)));
}

export function validateConditionalBlock(block: LogicConditionalBlockConfig): LogicIssue[] {
  const issues: LogicIssue[] = [];
  if (!block.conditionExpression.trim()) {
    issues.push({ severity: 'error', field: 'conditional.conditionExpression', message: 'Conditional expression is required.' });
  }

  const branches = block.branches?.length
    ? block.branches
    : [
        { id: 'then', outputType: block.trueOutputType },
        { id: 'else', outputType: block.falseOutputType },
      ];
  const valueBranches = branches.filter((branch) => !branch.takeNoAction && !branch.returnsOntologyEdits);
  const editBranches = branches.filter((branch) => branch.returnsOntologyEdits);
  const noActionBranches = branches.filter((branch) => branch.takeNoAction);

  if (valueBranches.length > 0) {
    const expectedType = valueBranches[0].outputType;
    if (!expectedType || valueBranches.some((branch) => !branchOutputCompatible(expectedType, branch.outputType))) {
      issues.push({ severity: 'error', field: 'conditional.outputType', message: 'Conditional branches must produce compatible output types.' });
    }
    if (editBranches.length > 0 || noActionBranches.length > 0) {
      issues.push({ severity: 'error', field: 'conditional.branches', message: 'Conditionals cannot mix value-returning branches with ontology-edit or no-action branches.' });
    }
  } else if (editBranches.length > 0 && branches.some((branch) => !branch.returnsOntologyEdits && !branch.takeNoAction)) {
    issues.push({ severity: 'error', field: 'conditional.branches', message: 'Ontology edit conditionals require every branch to run an action or explicitly take no action.' });
  }
  return issues;
}

export function validateLoopBlock(block: LogicLoopBlockConfig, inputs: LogicInputDefinition[]): LogicIssue[] {
  const issues: LogicIssue[] = [];
  const inputTypes = inputTypeByApiName(inputs);
  const listInputType = inputTypes.get(block.inputApiName);
  if (listInputType !== 'array' && listInputType !== 'list' && listInputType !== 'object_list') {
    issues.push({ severity: 'error', field: 'loop.inputApiName', message: 'Loop input must be an array, list, or object list.' });
  }
  if (listInputType === 'array' && !block.arrayToListInserted) {
    issues.push({ severity: 'warning', field: 'loop.arrayToListInserted', message: 'Array loop inputs require an Array to List conversion before iteration.' });
  }
  if (!isValidLogicInputApiName(block.elementVariableApiName)) {
    issues.push({ severity: 'error', field: 'loop.elementVariableApiName', message: 'Loop element variable must be a valid API name.' });
  }
  if (!isValidLogicInputApiName(block.indexVariableApiName)) {
    issues.push({ severity: 'error', field: 'loop.indexVariableApiName', message: 'Loop index variable must be a valid API name.' });
  }
  if (block.elementVariableApiName === block.indexVariableApiName) {
    issues.push({ severity: 'error', field: 'loop.indexVariableApiName', message: 'Loop element and index variables must have distinct API names.' });
  }
  if (block.parallel && block.containsActionTool) {
    issues.push({ severity: 'error', field: 'loop.parallel', message: 'Loops that contain action tools must run sequentially.' });
  }
  if (!block.parallel && !block.containsActionTool) {
    issues.push({ severity: 'warning', field: 'loop.parallel', message: 'Loop can run in parallel because it has no action tools.' });
  }
  if (block.outputAggregation === 'list') {
    if (!isCollectionType(block.finalOutputType)) {
      issues.push({ severity: 'error', field: 'loop.finalOutputType', message: 'List aggregation must produce an array, list, object list, or object set output.' });
    }
    if (isOntologyEditType(block.bodyOutputType)) {
      issues.push({ severity: 'error', field: 'loop.outputAggregation', message: 'Ontology edit loop bodies must aggregate as ontology edits, not lists.' });
    }
  } else if (block.outputAggregation === 'first') {
    if (!outputTypeCompatible(block.bodyOutputType, block.finalOutputType)) {
      issues.push({ severity: 'error', field: 'loop.finalOutputType', message: 'First aggregation output must match the loop body output type.' });
    }
  } else if (block.outputAggregation === 'count') {
    if (block.finalOutputType !== 'integer' && block.finalOutputType !== 'long') {
      issues.push({ severity: 'error', field: 'loop.finalOutputType', message: 'Count aggregation must produce an integer or long output.' });
    }
  } else if (block.outputAggregation === 'none') {
    if (!isOntologyEditType(block.bodyOutputType) || !isOntologyEditType(block.finalOutputType)) {
      issues.push({ severity: 'error', field: 'loop.finalOutputType', message: 'No aggregation is only valid for ontology edit bundle loop outputs.' });
    }
  }
  return issues;
}

export function validateLogicOutputDefinition(output: LogicOutputDefinition, blockOutputTypes: Record<string, LogicValueType>): LogicIssue[] {
  const issues: LogicIssue[] = [];
  if (!output.name.trim()) {
    issues.push({ severity: 'error', field: 'output.name', message: 'Output display name is required.' });
  }
  if (!isValidLogicInputApiName(output.apiName)) {
    issues.push({ severity: 'error', field: 'output.apiName', message: 'Output API name must start with a letter and contain only letters, numbers, and underscores.' });
  }
  if (!LOGIC_OUTPUT_VALUE_TYPES.has(output.outputType)) {
    issues.push({ severity: 'error', field: 'output.outputType', message: 'Logic outputs cannot return model variables or unsupported local value types.' });
  }
  if (output.source === 'ontology_edit_bundle' && output.outputType !== 'ontology_edit_bundle') {
    issues.push({ severity: 'error', field: 'output.outputType', message: 'Ontology edit bundle sources must produce ontology_edit_bundle outputs.' });
  } else if (output.source !== 'ontology_edit_bundle') {
    const sourceType = blockOutputTypes[output.sourceId];
    if (!sourceType) {
      issues.push({ severity: 'error', field: 'output.sourceId', message: 'Output source must reference an existing block or intermediary output.' });
    } else if (!outputTypeCompatible(sourceType, output.outputType)) {
      issues.push({ severity: 'error', field: 'output.sourceId', message: `Output source is not compatible with ${output.outputType}.` });
    }
  }
  if (output.workshopUsage === 'markdown_display' && output.outputType !== 'string') {
    issues.push({ severity: 'error', field: 'output.workshopUsage', message: 'Workshop Markdown display functions require a string output.' });
  }
  return issues;
}

export function validateLogicOutputs(outputs: LogicOutputDefinition[], blockOutputTypes: Record<string, LogicValueType>): LogicIssue[] {
  const issues = outputs.flatMap((output) => validateLogicOutputDefinition(output, blockOutputTypes));
  if (!outputs.some((output) => output.final)) {
    issues.push({ severity: 'error', field: 'outputs.final', message: 'At least one final Logic function output is required.' });
  }
  const seen = new Set<string>();
  for (const output of outputs) {
    const normalized = output.apiName.toLowerCase();
    if (seen.has(normalized)) {
      issues.push({ severity: 'error', field: `outputs.${output.apiName}`, message: 'Output API names must be unique.' });
    }
    seen.add(normalized);
  }
  return issues;
}

export type LogicRunStatus = 'idle' | 'running' | 'succeeded' | 'failed';
export type LogicExecutionMode = 'draft_preview' | 'published' | 'automation';

export interface LogicPreviewRunMetadata {
  runId: string;
  executionMode: LogicExecutionMode;
  startedAtIso: string;
  durationMs: number;
  inputCount: number;
  toolCallCount: number;
  retainedUntil: 'local_session' | 'platform_policy';
  securityFiltered: boolean;
}

export interface LogicPreviewRunResult {
  id: string;
  status: LogicRunStatus;
  result: string;
  durationMs: number;
  metadata: LogicPreviewRunMetadata;
  trace: LogicDebuggerTraceMetadata;
  errors: LogicIssue[];
}

export interface LogicDebuggerBlockTrace {
  id: string;
  title: string;
  status: 'not_run' | 'ok' | 'error';
  durationMs: number;
  inputs: Record<string, unknown>;
  outputs: Record<string, unknown>;
  prompt?: LogicDebuggerTraceMetadata['renderedPrompt'];
  toolCalls: LogicDebuggerTraceMetadata['toolCalls'];
  errors: LogicIssue[];
  retention: LogicPreviewRunMetadata['retainedUntil'];
  securityFiltered: boolean;
}

const SENSITIVE_TRACE_KEY_PATTERN = /authorization|password|secret|token|api[_-]?key|credential/i;
const TRACE_STRING_LIMIT = 240;

function sanitizeTraceValue(value: unknown, key = ''): unknown {
  if (SENSITIVE_TRACE_KEY_PATTERN.test(key)) return '[redacted]';
  if (typeof value === 'string') {
    return value.length > TRACE_STRING_LIMIT ? `${value.slice(0, TRACE_STRING_LIMIT)}…` : value;
  }
  if (Array.isArray(value)) return value.map((entry) => sanitizeTraceValue(entry));
  if (typeof value === 'object' && value !== null) {
    return Object.fromEntries(Object.entries(value).map(([entryKey, entryValue]) => [entryKey, sanitizeTraceValue(entryValue, entryKey)]));
  }
  return value;
}

function numericInputValue(inputValues: Record<string, string>, apiName: string, fallback: number): number {
  const parsed = Number(inputValues[apiName]);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function executeDraftLogicPreview(
  block: LogicLlmBlockConfig,
  inputs: LogicInputDefinition[],
  inputValues: Record<string, string>,
  now = new Date(),
): LogicPreviewRunResult {
  const trace = buildLlmDebuggerTrace(block, inputs);
  const errors = trace.errors.filter((issue) => issue.severity === 'error');
  const durationMs = Math.max(80, trace.tokenUsage.computeUnitsEstimate * 145 + trace.toolCalls.length * 38);
  const runId = `draft-${now.getTime().toString(36)}`;
  const complaint = inputValues.complaintText?.trim() || 'No complaint text provided.';
  const baseRisk = numericInputValue(inputValues, 'baseRisk', 0);
  const delayHours = numericInputValue(inputValues, 'delayHours', 0);
  const riskScore = Math.round((baseRisk + delayHours * 2) * 10) / 10;
  const result = errors.length > 0
    ? 'Draft run failed validation before execution.'
    : `Preview result: ${complaint} Risk score ${riskScore}; recommend service recovery follow-up and review proposed Ontology edits before publishing.`;

  return {
    id: runId,
    status: errors.length > 0 ? 'failed' : 'succeeded',
    result,
    durationMs,
    metadata: {
      runId,
      executionMode: 'draft_preview',
      startedAtIso: now.toISOString(),
      durationMs,
      inputCount: inputs.length,
      toolCallCount: trace.toolCalls.length,
      retainedUntil: 'local_session',
      securityFiltered: true,
    },
    trace,
    errors,
  };
}

export function buildDebuggerBlockTraces(
  run: LogicPreviewRunResult | undefined,
  inputValues: Record<string, string>,
  clearToolCalls = false,
): LogicDebuggerBlockTrace[] {
  if (!run) return [];
  const retention = run.metadata.executionMode === 'draft_preview' ? 'local_session' : 'platform_policy';
  const sanitizedInputs = sanitizeTraceValue(inputValues) as Record<string, unknown>;
  const filteredToolCalls = clearToolCalls ? [] : run.trace.toolCalls.map((toolCall) => sanitizeTraceValue(toolCall) as LogicDebuggerTraceMetadata['toolCalls'][number]);

  return [
    {
      id: 'input-binding',
      title: 'Input binding',
      status: 'ok',
      durationMs: 42,
      inputs: sanitizedInputs,
      outputs: { boundInputs: Object.keys(inputValues).length },
      toolCalls: [],
      errors: [],
      retention,
      securityFiltered: true,
    },
    {
      id: run.trace.blockId,
      title: 'Use LLM prompt render',
      status: run.errors.length > 0 ? 'error' : 'ok',
      durationMs: Math.max(80, run.durationMs - 84),
      inputs: { variables: run.trace.renderedPrompt.variables },
      outputs: { structuredOutputKind: run.trace.output.structuredOutputKind, tokenUsage: run.trace.tokenUsage },
      prompt: sanitizeTraceValue(run.trace.renderedPrompt) as LogicDebuggerTraceMetadata['renderedPrompt'],
      toolCalls: filteredToolCalls,
      errors: run.errors,
      retention,
      securityFiltered: true,
    },
    {
      id: 'final-output',
      title: 'Final output mapping',
      status: run.status === 'failed' ? 'error' : 'ok',
      durationMs: 42,
      inputs: { source: 'llm.text' },
      outputs: { finalResult: sanitizeTraceValue(run.result) },
      toolCalls: [],
      errors: run.errors,
      retention,
      securityFiltered: true,
    },
  ];
}
