import { describe, expect, it } from 'vitest';

import {
  buildDebuggerBlockTraces,
  buildLlmDebuggerTrace,
  evaluateCalculatorExpression,
  executeDraftLogicPreview,
  validateApplyActionTool,
  validateCalculatorTool,
  validateExecuteFunctionTool,
  validateConditionalBlock,
  validateCreateVariableBlock,
  validateLlmBlock,
  validateLogicOutputs,
  validateLoopBlock,
  validateQueryObjectsTool,
  type LogicActionToolConfig,
  type LogicCalculatorToolConfig,
  type LogicExecuteFunctionToolConfig,
  type LogicLlmBlockConfig,
} from './blocks';
import type { LogicInputDefinition } from './inputs';

const inputs: LogicInputDefinition[] = [
  { id: 'i1', name: 'Customer', apiName: 'customer', type: 'object', required: true, objectTypeId: 'Customer' },
  { id: 'i2', name: 'Question', apiName: 'question', type: 'string', required: true },
  { id: 'i3', name: 'Experiment model', apiName: 'experimentModel', type: 'model', required: true, modelVariableKind: 'llm' },
  { id: 'i4', name: 'Delay hours', apiName: 'delayHours', type: 'double', required: true },
  { id: 'i5', name: 'Base risk', apiName: 'baseRisk', type: 'integer', required: true },
  { id: 'i6', name: 'Segments', apiName: 'segments', type: 'array', required: true, defaultValue: '["vip"]' },
];

const actionTool: LogicActionToolConfig = {
  kind: 'apply_action',
  name: 'Create service case',
  actionTypeId: 'create-service-case',
  allowedActionTypeIds: ['create-service-case'],
  expectedParameters: { customer: 'object', summary: 'string' },
  parameterMappings: { customer: 'customer', summary: 'question' },
  invocationMode: 'preview',
  invocationSurface: 'draft_preview',
  logicPublished: false,
};

const functionTool: LogicExecuteFunctionToolConfig = {
  kind: 'execute_function',
  name: 'SLA impact',
  functionRid: 'fn.slaImpact.ts',
  functionKind: 'typescript',
  allowedFunctionRids: ['fn.slaImpact.ts'],
  signature: { parameters: { complaint: 'string', delay: 'double' }, returnType: 'json' },
  parameterMappings: { complaint: 'question', delay: 'delayHours' },
  expectedOutputType: 'json',
};

const calculatorTool: LogicCalculatorToolConfig = {
  kind: 'calculator',
  name: 'Exact score',
  expression: '(baseRisk + delayHours * 2) / 100',
  parameterRefs: ['baseRisk', 'delayHours'],
  outputType: 'double',
};

const block: LogicLlmBlockConfig = {
  id: 'llm-1',
  name: 'Summarize customer risk',
  modelBinding: { mode: 'model_variable', modelVariableApiName: 'experimentModel' },
  systemPrompt: 'You are a customer operations assistant.',
  taskPrompt: 'Answer {{question}} for {{customer}}.',
  promptVariableRefs: ['question', 'customer'],
  structuredOutput: { kind: 'json_schema', schemaJson: '{"type":"object"}' },
  maxOutputTokens: 512,
  toolAccess: [
    {
      kind: 'query_objects',
      name: 'Customer lookup',
      objectTypeId: 'Customer',
      selectedProperties: ['name', 'tier'],
      readableObjectTypeIds: ['Customer'],
      readablePropertiesByObjectType: { Customer: ['name', 'tier', 'status'] },
      maxObjects: 10,
    },
  ],
};

describe('Logic LLM block validation', () => {
  it('accepts model variables for Evals experiments', () => {
    expect(validateLlmBlock(block, inputs).filter((issue) => issue.severity === 'error')).toHaveLength(0);
  });

  it('rejects prompt refs that are not Logic inputs', () => {
    expect(validateLlmBlock({ ...block, promptVariableRefs: ['missingInput'] }, inputs)).toContainEqual({
      severity: 'error',
      field: 'promptVariableRefs.missingInput',
      message: 'Prompt variable missingInput does not match a Logic input.',
    });
  });

  it('rejects unreadable object types and properties on query tools', () => {
    expect(validateQueryObjectsTool({
      kind: 'query_objects',
      name: 'Private lookup',
      objectTypeId: 'SecretCustomer',
      selectedProperties: ['ssn'],
      readableObjectTypeIds: ['Customer'],
      readablePropertiesByObjectType: { SecretCustomer: ['name'] },
      maxObjects: 10,
    })).toEqual(expect.arrayContaining([
      { severity: 'error', field: 'tool.objectTypeId', message: 'Selected object type is not readable by this Logic function or user.' },
      { severity: 'error', field: 'tool.selectedProperties.ssn', message: 'Property ssn is not readable on SecretCustomer.' },
    ]));
  });

  it('warns when query object context is token-expensive', () => {
    expect(validateQueryObjectsTool({
      kind: 'query_objects',
      name: 'Wide customer lookup',
      objectTypeId: 'Customer',
      selectedProperties: Array.from({ length: 13 }, (_, i) => `p${i}`),
      readableObjectTypeIds: ['Customer'],
      readablePropertiesByObjectType: { Customer: Array.from({ length: 13 }, (_, i) => `p${i}`) },
      maxObjects: 100,
    })).toEqual(expect.arrayContaining([
      { severity: 'warning', field: 'tool.selectedProperties', message: 'Exposing many properties can inflate prompts; select only fields the LLM needs.' },
      { severity: 'warning', field: 'tool.maxObjects', message: 'Large object result limits can be token-expensive; consider lowering max objects or adding filters.' },
    ]));
  });

  it('builds debugger trace metadata for prompts, tools, output, token usage, and errors', () => {
    const trace = buildLlmDebuggerTrace(block, inputs);
    expect(trace.renderedPrompt.variables).toEqual(['question', 'customer']);
    expect(trace.toolCalls[0]).toMatchObject({ toolName: 'Customer lookup', kind: 'query_objects', objectTypeId: 'Customer' });
    expect(trace.output.structuredOutputKind).toBe('json_schema');
    expect(trace.tokenUsage.computeUnitsEstimate).toBeGreaterThan(0);
    expect(trace.errors.filter((issue) => issue.severity === 'error')).toHaveLength(0);
  });

  it('keeps Apply action in preview unless Logic is published and invoked by action or automation', () => {
    expect(validateApplyActionTool(actionTool, inputs).filter((issue) => issue.severity === 'error')).toHaveLength(0);
    expect(validateApplyActionTool({ ...actionTool, invocationMode: 'commit' }, inputs)).toEqual(expect.arrayContaining([
      { severity: 'error', field: 'tool.invocationMode', message: 'Real Ontology edits require published Logic.' },
      { severity: 'error', field: 'tool.invocationSurface', message: 'Real Ontology edits require action or automation invocation.' },
    ]));
  });

  it('validates execute-function permissions, parameter mappings, and output compatibility', () => {
    expect(validateExecuteFunctionTool(functionTool, inputs).filter((issue) => issue.severity === 'error')).toHaveLength(0);
    expect(validateExecuteFunctionTool({ ...functionTool, functionRid: 'fn.private.py', expectedOutputType: 'string' }, inputs)).toEqual(expect.arrayContaining([
      { severity: 'error', field: 'tool.functionRid', message: 'Selected function is not executable by this Logic function or user.' },
      { severity: 'error', field: 'tool.expectedOutputType', message: 'Function return type is not compatible with the configured tool output.' },
    ]));
  });

  it('supports deterministic calculator validation and computation', () => {
    expect(validateCalculatorTool(calculatorTool, inputs).filter((issue) => issue.severity === 'error')).toHaveLength(0);
    expect(evaluateCalculatorExpression(calculatorTool.expression, { baseRisk: 35, delayHours: 6 })).toBe(0.47);
    expect(validateCalculatorTool({ ...calculatorTool, expression: 'customer + 1' }, inputs)).toContainEqual({
      severity: 'error',
      field: 'tool.expression.customer',
      message: 'Calculator variable customer must be numeric.',
    });
  });

  it('adds proposed Ontology edits and tool metadata to debugger traces', () => {
    const trace = buildLlmDebuggerTrace({ ...block, toolAccess: [actionTool, functionTool, calculatorTool] }, inputs);
    expect(trace.proposedOntologyEdits).toEqual([{
      actionTypeId: 'create-service-case',
      parameters: { customer: 'customer', summary: 'question' },
      applyState: 'preview_only',
    }]);
    expect(trace.toolCalls).toEqual(expect.arrayContaining([
      expect.objectContaining({ kind: 'apply_action', status: 'preview_only' }),
      expect.objectContaining({ kind: 'execute_function', functionRid: 'fn.slaImpact.ts' }),
      expect.objectContaining({ kind: 'calculator', expression: '(baseRisk + delayHours * 2) / 100' }),
    ]));
  });
});

describe('Logic control-flow and output validation', () => {
  it('validates create-variable source typing for literals and inputs', () => {
    expect(validateCreateVariableBlock({
      id: 'var-1',
      apiName: 'riskNote',
      valueType: 'string',
      source: 'literal',
      literalValue: 'Escalate customer',
    }, inputs)).toHaveLength(0);
    expect(validateCreateVariableBlock({
      id: 'var-2',
      apiName: 'badModel',
      valueType: 'model',
      source: 'input',
      inputApiName: 'experimentModel',
    }, inputs)).toContainEqual({
      severity: 'error',
      field: 'variable.valueType',
      message: 'Create variable blocks support primitive, array, object, struct, and JSON-compatible values only.',
    });
  });

  it('validates conditionals and loop list conversion/parallel rules', () => {
    expect(validateConditionalBlock({
      id: 'cond-1',
      conditionExpression: 'baseRisk > 50',
      trueOutputType: 'string',
      falseOutputType: 'integer',
    })).toContainEqual({
      severity: 'error',
      field: 'conditional.outputType',
      message: 'Conditional branches must produce compatible output types.',
    });

    expect(validateLoopBlock({
      id: 'loop-1',
      inputApiName: 'question',
      elementVariableApiName: 'item',
      indexVariableApiName: 'index',
      bodyOutputType: 'string',
      outputAggregation: 'list',
      finalOutputType: 'string',
      containsActionTool: true,
      parallel: true,
    }, inputs)).toEqual(expect.arrayContaining([
      { severity: 'error', field: 'loop.inputApiName', message: 'Loop input must be an array, list, or object list.' },
      { severity: 'error', field: 'loop.parallel', message: 'Loops that contain action tools must run sequentially.' },
      { severity: 'error', field: 'loop.finalOutputType', message: 'List aggregation must produce an array, list, object list, or object set output.' },
    ]));
  });

  it('warns about array-to-list loop conversion and validates ontology edit aggregation', () => {
    expect(validateLoopBlock({
      id: 'loop-2',
      inputApiName: 'segments',
      elementVariableApiName: 'segment',
      indexVariableApiName: 'index',
      bodyOutputType: 'string',
      outputAggregation: 'list',
      finalOutputType: 'list',
      containsActionTool: false,
      parallel: true,
    }, inputs)).toContainEqual({
      severity: 'warning',
      field: 'loop.arrayToListInserted',
      message: 'Array loop inputs require an Array to List conversion before iteration.',
    });

    expect(validateLoopBlock({
      id: 'loop-3',
      inputApiName: 'segments',
      elementVariableApiName: 'segment',
      indexVariableApiName: 'index',
      bodyOutputType: 'ontology_edit_bundle',
      outputAggregation: 'none',
      finalOutputType: 'ontology_edit_bundle',
      containsActionTool: true,
      parallel: false,
      arrayToListInserted: true,
    }, inputs).filter((issue) => issue.severity === 'error')).toHaveLength(0);
  });

  it('supports conditional ontology edit branches that explicitly take no action', () => {
    expect(validateConditionalBlock({
      id: 'cond-2',
      conditionExpression: 'baseRisk > 80',
      trueOutputType: 'ontology_edit_bundle',
      falseOutputType: 'ontology_edit_bundle',
      branches: [
        { id: 'then', conditionExpression: 'baseRisk > 80', returnsOntologyEdits: true },
        { id: 'else', takeNoAction: true },
      ],
    }).filter((issue) => issue.severity === 'error')).toHaveLength(0);
  });

  it('validates final/intermediate outputs and Workshop Markdown string requirement', () => {
    expect(validateLogicOutputs([
      {
        id: 'out-1',
        name: 'Markdown panel',
        apiName: 'markdownPanel',
        outputType: 'object',
        source: 'block_output',
        sourceId: 'llm.final',
        final: true,
        workshopUsage: 'markdown_display',
      },
    ], { 'llm.final': 'object' })).toContainEqual({
      severity: 'error',
      field: 'output.workshopUsage',
      message: 'Workshop Markdown display functions require a string output.',
    });

    expect(validateLogicOutputs([
      {
        id: 'out-2',
        name: 'Final answer',
        apiName: 'finalAnswer',
        outputType: 'string',
        source: 'block_output',
        sourceId: 'llm.text',
        final: true,
        workshopUsage: 'markdown_display',
      },
      {
        id: 'out-3',
        name: 'Action edits',
        apiName: 'actionEdits',
        outputType: 'ontology_edit_bundle',
        source: 'ontology_edit_bundle',
        sourceId: 'action.preview',
        final: false,
        workshopUsage: 'none',
      },
    ], { 'llm.text': 'string' }).filter((issue) => issue.severity === 'error')).toHaveLength(0);
  });

  it('rejects unsupported output families and unknown intermediary sources', () => {
    expect(validateLogicOutputs([
      {
        id: 'out-4',
        name: 'Model slot',
        apiName: 'modelSlot',
        outputType: 'model',
        source: 'intermediate',
        sourceId: 'missing.output',
        final: true,
        workshopUsage: 'none',
      },
    ], {})).toEqual(expect.arrayContaining([
      { severity: 'error', field: 'output.outputType', message: 'Logic outputs cannot return model variables or unsupported local value types.' },
      { severity: 'error', field: 'output.sourceId', message: 'Output source must reference an existing block or intermediary output.' },
    ]));
  });
});


describe('Logic draft run panel and debugger helpers', () => {
  it('executes a draft preview run with metadata, duration, and latest result', () => {
    const run = executeDraftLogicPreview({ ...block, toolAccess: [actionTool, functionTool, calculatorTool] }, inputs, {
      customer: 'Customer: Acme',
      question: 'Shipment 4421 missed SLA.',
      experimentModel: 'gpt-4.1-mini',
      baseRisk: '35',
      delayHours: '6',
    }, new Date('2026-05-13T12:00:00Z'));

    expect(run).toMatchObject({
      id: 'draft-mp40c5c0',
      status: 'succeeded',
      metadata: {
        executionMode: 'draft_preview',
        retainedUntil: 'local_session',
        securityFiltered: true,
        toolCallCount: 3,
      },
    });
    expect(run.durationMs).toBeGreaterThan(0);
    expect(run.result).toContain('Risk score 47');
  });

  it('builds security-filtered expandable debugger block traces and can clear tool calls', () => {
    const run = executeDraftLogicPreview({ ...block, toolAccess: [actionTool] }, inputs, {
      customer: 'Customer: Acme',
      question: 'Use apiToken=secret to inspect shipment.',
      apiToken: 'super-secret-token',
      baseRisk: '20',
      delayHours: '2',
    }, new Date('2026-05-13T12:01:00Z'));

    const traces = buildDebuggerBlockTraces(run, { apiToken: 'super-secret-token', question: 'safe' });
    expect(traces.map((trace) => trace.title)).toEqual(['Input binding', 'Use LLM prompt render', 'Final output mapping']);
    expect(traces[0].inputs.apiToken).toBe('[redacted]');
    expect(traces[1].toolCalls).toHaveLength(1);
    expect(traces.every((trace) => trace.securityFiltered && trace.retention === 'local_session')).toBe(true);

    const cleared = buildDebuggerBlockTraces(run, { apiToken: 'super-secret-token' }, true);
    expect(cleared[1].toolCalls).toHaveLength(0);
  });
});
