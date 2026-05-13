import { useMemo, useState, type ReactNode } from 'react';

import {
  buildDebuggerBlockTraces,
  executeDraftLogicPreview,
  validateLlmBlock,
  validateApplyActionTool,
  validateCalculatorTool,
  validateConditionalBlock,
  validateCreateVariableBlock,
  validateExecuteFunctionTool,
  validateLogicOutputs,
  validateLoopBlock,
  validateQueryObjectsTool,
  type LogicActionToolConfig,
  type LogicDebuggerBlockTrace,
  type LogicPreviewRunResult,
  type LogicCalculatorToolConfig,
  type LogicExecuteFunctionToolConfig,
  type LogicConditionalBlockConfig,
  type LogicLlmBlockConfig,
  type LogicLoopBlockConfig,
  type LogicOutputDefinition,
  type LogicValueType,
  type LogicVariableBlockConfig,
} from '@/lib/logic/blocks';
import {
  LOGIC_INPUT_TYPES,
  validateLogicInputBoard,
  validateLogicInputDefinition,
  type LogicInputDefinition,
  type LogicInputType,
} from '@/lib/logic/inputs';

const RIGHT_RAIL = [
  'Uses',
  'Automations',
  'Evaluations',
  'Run history',
  'Version history',
  'Metrics',
  'Execution settings',
] as const;

const QUERY_OBJECT_TYPES = ['Customer', 'Order', 'Shipment'] as const;
const QUERY_PROPERTIES: Record<string, string[]> = {
  Customer: ['name', 'tier', 'status', 'openCases', 'region'],
  Order: ['orderId', 'status', 'value', 'createdAt'],
  Shipment: ['shipmentId', 'carrier', 'eta', 'riskScore'],
};

const BLOCK_OUTPUT_TYPES: Record<string, LogicValueType> = {
  'llm.text': 'string',
  'llm.structured': 'json',
  'loop.recommendations': 'list',
  'action.preview': 'ontology_edit_bundle',
};

const SAMPLE_INPUTS: LogicInputDefinition[] = [
  {
    id: 'input-1',
    name: 'Customer record',
    apiName: 'customerRecord',
    type: 'object',
    required: true,
    objectTypeId: 'Customer',
    description: 'Ontology object selected by the caller.',
  },
  {
    id: 'input-2',
    name: 'Complaint text',
    apiName: 'complaintText',
    type: 'string',
    required: true,
    defaultValue: 'Late shipment reported by customer.',
    description: 'Free-text prompt context.',
  },
  {
    id: 'input-3',
    name: 'Reference media',
    apiName: 'referenceMedia',
    type: 'media_reference',
    required: false,
    mediaSetRid: 'media.set.demo',
    description: 'Optional image/audio/video evidence from Media Sets.',
  },
  {
    id: 'input-4',
    name: 'Response model',
    apiName: 'responseModel',
    type: 'model',
    required: true,
    modelVariableKind: 'llm',
    compatibleModelKinds: ['llm', 'vision'],
    description: 'Model variable passed into the Use LLM block.',
  },
  {
    id: 'input-5',
    name: 'Base risk',
    apiName: 'baseRisk',
    type: 'integer',
    required: true,
    defaultValue: '35',
    description: 'Numeric signal for calculator tooling.',
  },
  {
    id: 'input-6',
    name: 'Delay hours',
    apiName: 'delayHours',
    type: 'double',
    required: true,
    defaultValue: '6',
    description: 'Exact computation input for LLM workflows.',
  },
  {
    id: 'input-7',
    name: 'Related shipments',
    apiName: 'relatedShipments',
    type: 'object_list',
    required: false,
    objectTypeId: 'Shipment',
    description: 'List input used by loop blocks.',
  },
];

const SAMPLE_LLM_BLOCK: LogicLlmBlockConfig = {
  id: 'llm-risk-summary',
  name: 'Summarize customer risk',
  modelBinding: { mode: 'model_variable', modelVariableApiName: 'responseModel' },
  systemPrompt: 'You are an operations copilot that explains customer risk with concise evidence.',
  taskPrompt: 'Use {{customerRecord}} and {{complaintText}} to recommend the next best action.',
  promptVariableRefs: ['customerRecord', 'complaintText'],
  structuredOutput: {
    kind: 'json_schema',
    schemaJson: '{"type":"object","properties":{"risk":{"type":"string"},"nextAction":{"type":"string"}}}',
  },
  maxOutputTokens: 768,
  toolAccess: [
    {
      kind: 'query_objects',
      name: 'Customer facts',
      objectTypeId: 'Customer',
      selectedProperties: ['name', 'tier', 'status'],
      readableObjectTypeIds: ['Customer', 'Order', 'Shipment'],
      readablePropertiesByObjectType: QUERY_PROPERTIES,
      maxObjects: 8,
    },
    {
      kind: 'apply_action',
      name: 'Open service recovery action',
      actionTypeId: 'create-service-case',
      allowedActionTypeIds: ['create-service-case', 'assign-account-owner'],
      expectedParameters: { customer: 'object', summary: 'string' },
      parameterMappings: { customer: 'customerRecord', summary: 'complaintText' },
      invocationMode: 'preview',
      invocationSurface: 'draft_preview',
      logicPublished: false,
    },
    {
      kind: 'execute_function',
      name: 'Calculate SLA impact',
      functionRid: 'fn.slaImpact.ts',
      functionKind: 'typescript',
      allowedFunctionRids: ['fn.slaImpact.ts', 'fn.route.py', 'logic.existingRisk'],
      signature: { parameters: { complaint: 'string' }, returnType: 'json' },
      parameterMappings: { complaint: 'complaintText' },
      expectedOutputType: 'json',
    },
    {
      kind: 'calculator',
      name: 'Exact risk score',
      expression: '(baseRisk + delayHours * 2) / 100',
      parameterRefs: ['baseRisk', 'delayHours'],
      outputType: 'double',
    },
  ],
};

const SAMPLE_VARIABLE_BLOCK: LogicVariableBlockConfig = {
  id: 'var-escalation-note',
  apiName: 'escalationNote',
  valueType: 'string',
  source: 'literal',
  literalValue: 'Escalate if SLA risk is high.',
};

const SAMPLE_CONDITIONAL_BLOCK: LogicConditionalBlockConfig = {
  id: 'cond-risk-threshold',
  conditionExpression: 'baseRisk > 50 || delayHours > 4',
  trueOutputType: 'string',
  falseOutputType: 'string',
};

const SAMPLE_LOOP_BLOCK: LogicLoopBlockConfig = {
  id: 'loop-related-shipments',
  inputApiName: 'relatedShipments',
  elementVariableApiName: 'shipment',
  indexVariableApiName: 'shipmentIndex',
  bodyOutputType: 'string',
  outputAggregation: 'list',
  finalOutputType: 'list',
  containsActionTool: false,
  parallel: true,
};

const SAMPLE_LOGIC_OUTPUTS: LogicOutputDefinition[] = [
  {
    id: 'out-final-answer',
    name: 'Final answer',
    apiName: 'finalAnswer',
    outputType: 'string',
    source: 'block_output',
    sourceId: 'llm.text',
    final: true,
    workshopUsage: 'markdown_display',
  },
  {
    id: 'out-action-edits',
    name: 'Action edit preview',
    apiName: 'actionEditPreview',
    outputType: 'ontology_edit_bundle',
    source: 'ontology_edit_bundle',
    sourceId: 'action.preview',
    final: false,
    workshopUsage: 'none',
  },
];

const DEFAULT_RUN_INPUTS: Record<string, string> = {
  customerRecord: 'Customer: Acme Logistics / tier: Gold / open cases: 2',
  complaintText: 'Shipment 4421 missed its SLA. Explain the likely risk and recommended next action.',
  referenceMedia: 'media.set.demo/image-4421',
  responseModel: 'gpt-4.1-mini',
  baseRisk: '35',
  delayHours: '6',
  relatedShipments: '[{"shipmentId":"4421","carrier":"Northwind","riskScore":0.47}]',
};

type ConfigTab = 'inputs' | 'blocks' | 'outputs';

function StatusPill({ children, tone = 'info' }: { children: ReactNode; tone?: 'info' | 'success' | 'warning' }) {
  const color = tone === 'success' ? 'var(--status-success)' : tone === 'warning' ? 'var(--status-warning)' : 'var(--status-info)';
  const bg = tone === 'success' ? 'var(--status-success-bg)' : tone === 'warning' ? 'var(--status-warning-bg)' : 'var(--status-info-bg)';
  return (
    <span style={{ borderRadius: 999, background: bg, color, padding: '2px 8px', fontSize: 12, fontWeight: 600 }}>
      {children}
    </span>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label style={{ display: 'grid', gap: 4 }}>
      <span className="of-eyebrow">{label}</span>
      {children}
    </label>
  );
}

function InputsBoard({ inputs, selectedId, onSelect, onChange }: {
  inputs: LogicInputDefinition[];
  selectedId: string;
  onSelect: (id: string) => void;
  onChange: (input: LogicInputDefinition) => void;
}) {
  const selected = inputs.find((input) => input.id === selectedId) ?? inputs[0];
  const selectedIssues = validateLogicInputDefinition(selected);
  const boardIssues = validateLogicInputBoard(inputs);

  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'minmax(220px, 0.8fr) minmax(280px, 1fr)', gap: 10 }}>
      <div className="of-panel-muted" style={{ padding: 10 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}>
          <p className="of-eyebrow">Logic inputs</p>
          <StatusPill tone={boardIssues.length === 0 ? 'success' : 'warning'}>{boardIssues.length} issues</StatusPill>
        </div>
        <div style={{ display: 'grid', gap: 8, marginTop: 8 }}>
          {inputs.map((input) => {
            const issues = validateLogicInputDefinition(input);
            return (
              <button
                key={input.id}
                type="button"
                onClick={() => onSelect(input.id)}
                className="of-panel"
                style={{
                  padding: 10,
                  textAlign: 'left',
                  borderColor: input.id === selected.id ? 'var(--border-focus)' : 'var(--border-default)',
                  background: input.id === selected.id ? 'var(--status-info-bg)' : 'var(--bg-panel)',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                  <strong>{input.name}</strong>
                  <span className="of-text-muted">{input.type}</span>
                </div>
                <div className="of-text-muted" style={{ marginTop: 4 }}>
                  {input.apiName} · {input.required ? 'required' : 'optional'}
                </div>
                {issues.length > 0 && <div style={{ color: 'var(--status-warning)', marginTop: 4 }}>{issues[0].message}</div>}
              </button>
            );
          })}
        </div>
      </div>

      <div className="of-panel-muted" style={{ padding: 10 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
          <p className="of-eyebrow">Selected input</p>
          <StatusPill tone={selectedIssues.length === 0 ? 'success' : 'warning'}>
            {selectedIssues.length === 0 ? 'valid' : 'needs attention'}
          </StatusPill>
        </div>
        <div style={{ display: 'grid', gap: 10, marginTop: 10 }}>
          <Field label="Display name">
            <input className="of-input" value={selected.name} onChange={(event) => onChange({ ...selected, name: event.target.value })} />
          </Field>
          <Field label="API name">
            <input className="of-input" value={selected.apiName} onChange={(event) => onChange({ ...selected, apiName: event.target.value })} />
          </Field>
          <Field label="Type">
            <select
              className="of-select"
              value={selected.type}
              onChange={(event) => onChange({ ...selected, type: event.target.value as LogicInputType })}
            >
              {LOGIC_INPUT_TYPES.map((type) => <option key={type} value={type}>{type}</option>)}
            </select>
          </Field>
          <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <input type="checkbox" checked={selected.required} onChange={(event) => onChange({ ...selected, required: event.target.checked })} />
            Required input
          </label>
          <Field label="Default value">
            <input
              className="of-input"
              value={selected.defaultValue ?? ''}
              placeholder="Typed default value, JSON for arrays/structs"
              onChange={(event) => onChange({ ...selected, defaultValue: event.target.value })}
            />
          </Field>
          <Field label="Object type / model compatibility">
            <input
              className="of-input"
              value={selected.objectTypeId ?? selected.modelVariableKind ?? selected.mediaSetRid ?? ''}
              placeholder="Customer, object set backing type, llm, or media set RID"
              onChange={(event) => {
                const value = event.target.value;
                if (selected.type === 'model') onChange({ ...selected, modelVariableKind: value as LogicInputDefinition['modelVariableKind'] });
                else if (selected.type === 'media_reference') onChange({ ...selected, mediaSetRid: value });
                else onChange({ ...selected, objectTypeId: value, objectSetObjectTypeId: selected.type === 'object_set' ? value : selected.objectSetObjectTypeId });
              }}
            />
          </Field>
          {selectedIssues.length > 0 && (
            <div className="of-status-warning" style={{ padding: 10, borderRadius: 4 }}>
              <strong>Validation</strong>
              <ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>
                {selectedIssues.map((issue) => <li key={`${issue.field}-${issue.message}`}>{issue.message}</li>)}
              </ul>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function BlocksBoard({ inputs, llmBlock, onChange }: {
  inputs: LogicInputDefinition[];
  llmBlock: LogicLlmBlockConfig;
  onChange: (block: LogicLlmBlockConfig) => void;
}) {
  const issues = validateLlmBlock(llmBlock, inputs);
  const queryTool = llmBlock.toolAccess.find((tool) => tool.kind === 'query_objects');
  const actionTool = llmBlock.toolAccess.find((tool): tool is LogicActionToolConfig => tool.kind === 'apply_action');
  const functionTool = llmBlock.toolAccess.find((tool): tool is LogicExecuteFunctionToolConfig => tool.kind === 'execute_function');
  const calculatorTool = llmBlock.toolAccess.find((tool): tool is LogicCalculatorToolConfig => tool.kind === 'calculator');
  const queryIssues = queryTool?.kind === 'query_objects' ? validateQueryObjectsTool(queryTool) : [];
  const actionIssues = actionTool ? validateApplyActionTool(actionTool, inputs) : [];
  const functionIssues = functionTool ? validateExecuteFunctionTool(functionTool, inputs) : [];
  const calculatorIssues = calculatorTool ? validateCalculatorTool(calculatorTool, inputs) : [];
  const variableIssues = validateCreateVariableBlock(SAMPLE_VARIABLE_BLOCK, inputs, BLOCK_OUTPUT_TYPES);
  const conditionalIssues = validateConditionalBlock(SAMPLE_CONDITIONAL_BLOCK);
  const loopIssues = validateLoopBlock(SAMPLE_LOOP_BLOCK, inputs);
  const modelInputs = inputs.filter((input) => input.type === 'model');
  function replaceTool(nextTool: LogicLlmBlockConfig['toolAccess'][number]) {
    onChange({ ...llmBlock, toolAccess: llmBlock.toolAccess.map((tool) => (tool.kind === nextTool.kind ? nextTool : tool)) });
  }
  return (
    <div style={{ display: 'grid', gap: 10 }}>
      <div className="of-panel-muted" style={{ padding: 12 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}>
          <div>
            <p className="of-eyebrow">Use LLM block</p>
            <strong>{llmBlock.name}</strong>
          </div>
          <StatusPill tone={issues.some((issue) => issue.severity === 'error') ? 'warning' : 'success'}>
            {issues.filter((issue) => issue.severity === 'error').length} errors · {issues.filter((issue) => issue.severity === 'warning').length} warnings
          </StatusPill>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 12 }}>
          <Field label="Model binding">
            <select
              className="of-select"
              value={llmBlock.modelBinding.mode}
              onChange={(event) => onChange({ ...llmBlock, modelBinding: { ...llmBlock.modelBinding, mode: event.target.value as 'fixed' | 'model_variable' } })}
            >
              <option value="fixed">Fixed model</option>
              <option value="model_variable">Model variable for Evals</option>
            </select>
          </Field>
          {llmBlock.modelBinding.mode === 'model_variable' ? (
            <Field label="Model variable input">
              <select
                className="of-select"
                value={llmBlock.modelBinding.modelVariableApiName ?? ''}
                onChange={(event) => onChange({ ...llmBlock, modelBinding: { mode: 'model_variable', modelVariableApiName: event.target.value } })}
              >
                {modelInputs.map((input) => <option key={input.id} value={input.apiName}>{input.apiName}</option>)}
              </select>
            </Field>
          ) : (
            <Field label="Provider model">
              <input
                className="of-input"
                value={llmBlock.modelBinding.providerId ?? 'gpt-4.1-mini'}
                onChange={(event) => onChange({ ...llmBlock, modelBinding: { mode: 'fixed', providerId: event.target.value } })}
              />
            </Field>
          )}
        </div>
        <div style={{ display: 'grid', gap: 10, marginTop: 10 }}>
          <Field label="System prompt">
            <textarea className="of-textarea" style={{ minHeight: 86 }} value={llmBlock.systemPrompt} onChange={(event) => onChange({ ...llmBlock, systemPrompt: event.target.value })} />
          </Field>
          <Field label="Task prompt">
            <textarea className="of-textarea" style={{ minHeight: 96 }} value={llmBlock.taskPrompt} onChange={(event) => onChange({ ...llmBlock, taskPrompt: event.target.value })} />
          </Field>
          <div>
            <p className="of-eyebrow">Prompt variables</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {inputs.map((input) => {
                const checked = llmBlock.promptVariableRefs.includes(input.apiName);
                return (
                  <label key={input.id} className="of-chip" style={{ display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={(event) => {
                        const refs = event.target.checked
                          ? [...llmBlock.promptVariableRefs, input.apiName]
                          : llmBlock.promptVariableRefs.filter((ref) => ref !== input.apiName);
                        onChange({ ...llmBlock, promptVariableRefs: refs });
                      }}
                    />
                    {input.apiName}
                  </label>
                );
              })}
            </div>
          </div>
          <Field label="Structured output type">
            <select
              className="of-select"
              value={llmBlock.structuredOutput.kind}
              onChange={(event) => onChange({ ...llmBlock, structuredOutput: { ...llmBlock.structuredOutput, kind: event.target.value as LogicLlmBlockConfig['structuredOutput']['kind'] } })}
            >
              <option value="text">Text</option>
              <option value="json_schema">JSON schema</option>
              <option value="object">Object</option>
              <option value="object_list">Object list</option>
              <option value="ontology_edit_bundle">Ontology edit bundle</option>
            </select>
          </Field>
          <Field label="JSON schema">
            <textarea className="of-textarea" style={{ minHeight: 74 }} value={llmBlock.structuredOutput.schemaJson ?? ''} onChange={(event) => onChange({ ...llmBlock, structuredOutput: { ...llmBlock.structuredOutput, schemaJson: event.target.value } })} />
          </Field>
        </div>
      </div>

      {queryTool && (
        <div className="of-panel-muted" style={{ padding: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <div>
              <p className="of-eyebrow">Tool access</p>
              <strong>Query objects</strong>
            </div>
            <StatusPill tone={queryIssues.some((issue) => issue.severity === 'error') ? 'warning' : queryIssues.length ? 'warning' : 'success'}>
              {queryIssues.length} access/token notes
            </StatusPill>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
            <Field label="Readable object type">
              <select
                className="of-select"
                value={queryTool.objectTypeId}
                onChange={(event) => replaceTool({ ...queryTool, objectTypeId: event.target.value, selectedProperties: QUERY_PROPERTIES[event.target.value]?.slice(0, 3) ?? [] })}
              >
                {QUERY_OBJECT_TYPES.map((type) => <option key={type} value={type}>{type}</option>)}
              </select>
            </Field>
            <Field label="Max objects">
              <input
                className="of-input"
                type="number"
                min={1}
                value={queryTool.maxObjects}
                onChange={(event) => replaceTool({ ...queryTool, maxObjects: Number(event.target.value) })}
              />
            </Field>
          </div>
          <div style={{ marginTop: 10 }}>
            <p className="of-eyebrow">Selected readable properties</p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {(QUERY_PROPERTIES[queryTool.objectTypeId] ?? []).map((property) => {
                const checked = queryTool.selectedProperties.includes(property);
                return (
                  <label key={property} className="of-chip" style={{ display: 'inline-flex', gap: 5, alignItems: 'center' }}>
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={(event) => {
                        const selectedProperties = event.target.checked
                          ? [...queryTool.selectedProperties, property]
                          : queryTool.selectedProperties.filter((candidate) => candidate !== property);
                        replaceTool({ ...queryTool, selectedProperties });
                      }}
                    />
                    {property}
                  </label>
                );
              })}
            </div>
          </div>
          {queryIssues.length > 0 && (
            <div className="of-status-warning" style={{ padding: 10, borderRadius: 4, marginTop: 10 }}>
              <strong>Tool access validation</strong>
              <ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>
                {queryIssues.map((issue) => <li key={`${issue.field}-${issue.message}`}>{issue.message}</li>)}
              </ul>
            </div>
          )}
        </div>
      )}

      {actionTool && (
        <div className="of-panel-muted" style={{ padding: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <div>
              <p className="of-eyebrow">Ontology edits</p>
              <strong>Apply action</strong>
            </div>
            <StatusPill tone={actionIssues.some((issue) => issue.severity === 'error') ? 'warning' : 'success'}>{actionIssues.length} edit guardrails</StatusPill>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
            <Field label="Action type">
              <select className="of-select" value={actionTool.actionTypeId} onChange={(event) => replaceTool({ ...actionTool, actionTypeId: event.target.value })}>
                {actionTool.allowedActionTypeIds.map((actionType) => <option key={actionType} value={actionType}>{actionType}</option>)}
              </select>
            </Field>
            <Field label="Invocation mode">
              <select className="of-select" value={actionTool.invocationMode} onChange={(event) => replaceTool({ ...actionTool, invocationMode: event.target.value as LogicActionToolConfig['invocationMode'] })}>
                <option value="preview">Preview proposed edits only</option>
                <option value="commit">Commit when published + action/automation invoked</option>
              </select>
            </Field>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
            {Object.entries(actionTool.expectedParameters).map(([parameter]) => (
              <Field key={parameter} label={`Parameter: ${parameter}`}>
                <select className="of-select" value={actionTool.parameterMappings[parameter] ?? ''} onChange={(event) => replaceTool({ ...actionTool, parameterMappings: { ...actionTool.parameterMappings, [parameter]: event.target.value } })}>
                  {inputs.map((input) => <option key={input.id} value={input.apiName}>{input.apiName}</option>)}
                </select>
              </Field>
            ))}
          </div>
          <p className="of-text-muted" style={{ margin: '10px 0 0' }}>Preview records proposed Ontology edits in the debugger; real edits require published Logic plus action or automation invocation.</p>
        </div>
      )}

      {functionTool && (
        <div className="of-panel-muted" style={{ padding: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <div>
              <p className="of-eyebrow">Function tools</p>
              <strong>Execute function</strong>
            </div>
            <StatusPill tone={functionIssues.some((issue) => issue.severity === 'error') ? 'warning' : 'success'}>{functionTool.functionKind} · {functionIssues.length} signature notes</StatusPill>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
            <Field label="Function">
              <select className="of-select" value={functionTool.functionRid} onChange={(event) => replaceTool({ ...functionTool, functionRid: event.target.value })}>
                {functionTool.allowedFunctionRids.map((rid) => <option key={rid} value={rid}>{rid}</option>)}
              </select>
            </Field>
            <Field label="Function kind">
              <select className="of-select" value={functionTool.functionKind} onChange={(event) => replaceTool({ ...functionTool, functionKind: event.target.value as LogicExecuteFunctionToolConfig['functionKind'] })}>
                <option value="typescript">TypeScript</option>
                <option value="python">Python</option>
                <option value="existing_logic">Existing Logic</option>
                <option value="function_on_objects">Function on objects</option>
              </select>
            </Field>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 10 }}>
            {Object.entries(functionTool.signature.parameters).map(([parameter]) => (
              <Field key={parameter} label={`Parameter: ${parameter}`}>
                <select className="of-select" value={functionTool.parameterMappings[parameter] ?? ''} onChange={(event) => replaceTool({ ...functionTool, parameterMappings: { ...functionTool.parameterMappings, [parameter]: event.target.value } })}>
                  {inputs.map((input) => <option key={input.id} value={input.apiName}>{input.apiName}</option>)}
                </select>
              </Field>
            ))}
          </div>
        </div>
      )}

      {calculatorTool && (
        <div className="of-panel-muted" style={{ padding: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <div>
              <p className="of-eyebrow">Exact computation</p>
              <strong>Calculator</strong>
            </div>
            <StatusPill tone={calculatorIssues.some((issue) => issue.severity === 'error') ? 'warning' : 'success'}>{calculatorIssues.length} math notes</StatusPill>
          </div>
          <Field label="Expression">
            <input className="of-input" value={calculatorTool.expression} onChange={(event) => replaceTool({ ...calculatorTool, expression: event.target.value })} />
          </Field>
          <p className="of-text-muted" style={{ margin: '8px 0 0' }}>Calculator uses deterministic arithmetic for values the LLM should not estimate.</p>
        </div>
      )}

      <div className="of-panel-muted" style={{ padding: 12 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
          <div>
            <p className="of-eyebrow">Control flow</p>
            <strong>Create variable, conditional, and loop</strong>
          </div>
          <StatusPill tone={[...variableIssues, ...conditionalIssues, ...loopIssues].some((issue) => issue.severity === 'error') ? 'warning' : 'success'}>
            {[...variableIssues, ...conditionalIssues, ...loopIssues].length} flow notes
          </StatusPill>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 8, marginTop: 10 }}>
          <div className="of-panel" style={{ padding: 10 }}>
            <p className="of-eyebrow">Create variable</p>
            <strong>{SAMPLE_VARIABLE_BLOCK.apiName}</strong>
            <p className="of-text-muted" style={{ margin: '6px 0 0' }}>{SAMPLE_VARIABLE_BLOCK.valueType} from {SAMPLE_VARIABLE_BLOCK.source}</p>
          </div>
          <div className="of-panel" style={{ padding: 10 }}>
            <p className="of-eyebrow">Conditional</p>
            <strong>{SAMPLE_CONDITIONAL_BLOCK.conditionExpression}</strong>
            <p className="of-text-muted" style={{ margin: '6px 0 0' }}>{SAMPLE_CONDITIONAL_BLOCK.trueOutputType} / {SAMPLE_CONDITIONAL_BLOCK.falseOutputType}</p>
          </div>
          <div className="of-panel" style={{ padding: 10 }}>
            <p className="of-eyebrow">Loop</p>
            <strong>{SAMPLE_LOOP_BLOCK.inputApiName}</strong>
            <p className="of-text-muted" style={{ margin: '6px 0 0' }}>{SAMPLE_LOOP_BLOCK.parallel ? 'parallel' : 'sequential'} · {SAMPLE_LOOP_BLOCK.outputAggregation} aggregation</p>
          </div>
        </div>
        <p className="of-text-muted" style={{ margin: '10px 0 0' }}>Loops validate list/object-list inputs, element/index variables, output aggregation, and action-aware parallelization.</p>
      </div>
    </div>
  );
}

function OutputsBoard() {
  const outputIssues = validateLogicOutputs(SAMPLE_LOGIC_OUTPUTS, BLOCK_OUTPUT_TYPES);
  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}>
        <p className="of-eyebrow">Logic outputs</p>
        <StatusPill tone={outputIssues.some((issue) => issue.severity === 'error') ? 'warning' : 'success'}>{outputIssues.length} output notes</StatusPill>
      </div>
      {SAMPLE_LOGIC_OUTPUTS.map((output) => (
        <div key={output.id} className="of-panel-muted" style={{ padding: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <strong>{output.name}</strong>
            <StatusPill>{output.outputType}</StatusPill>
          </div>
          <p className="of-text-muted" style={{ margin: '6px 0 0' }}>
            {output.final ? 'Final Logic function output' : 'Intermediate output'} · source {output.sourceId} · Workshop {output.workshopUsage}
          </p>
        </div>
      ))}
      <div className="of-panel-muted" style={{ padding: 12 }}>
        <strong>Supported output families</strong>
        <p className="of-text-muted" style={{ margin: '6px 0 0' }}>Primitive values, objects, object lists/sets, structs, media references, and Ontology edit bundles where locally supported.</p>
      </div>
    </div>
  );
}


function TraceCard({ block, expanded, onToggle }: {
  block: LogicDebuggerBlockTrace;
  expanded: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="of-panel-muted" style={{ padding: 10 }}>
      <button type="button" className="of-button" onClick={onToggle} style={{ width: '100%', justifyContent: 'space-between' }}>
        <span>{expanded ? '▾' : '▸'} {block.title}</span>
        <span>{block.status} · {block.durationMs} ms</span>
      </button>
      {expanded && (
        <div style={{ display: 'grid', gap: 8, marginTop: 8 }}>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            <StatusPill tone={block.status === 'error' ? 'warning' : 'success'}>{block.status}</StatusPill>
            <StatusPill>{block.retention === 'local_session' ? 'local draft trace' : 'policy retained'}</StatusPill>
            <StatusPill tone="success">security filtered</StatusPill>
          </div>
          {block.prompt && (
            <div className="of-panel" style={{ padding: 8 }}>
              <p className="of-eyebrow">Prompt</p>
              <pre style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: 12 }}>{JSON.stringify(block.prompt, null, 2)}</pre>
            </div>
          )}
          <div className="of-panel" style={{ padding: 8 }}>
            <p className="of-eyebrow">Inputs / outputs</p>
            <pre style={{ margin: 0, overflow: 'auto', fontSize: 12 }}>{JSON.stringify({ inputs: block.inputs, outputs: block.outputs }, null, 2)}</pre>
          </div>
          <div className="of-panel" style={{ padding: 8 }}>
            <p className="of-eyebrow">Tool calls</p>
            <pre style={{ margin: 0, overflow: 'auto', fontSize: 12 }}>{JSON.stringify(block.toolCalls, null, 2)}</pre>
          </div>
          {block.errors.length > 0 && (
            <div className="of-status-warning" style={{ padding: 8, borderRadius: 4 }}>
              <strong>Errors</strong>
              <ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>
                {block.errors.map((issue) => <li key={`${issue.field}-${issue.message}`}>{issue.message}</li>)}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function DebuggerPanel({ run, inputValues }: {
  run?: LogicPreviewRunResult;
  inputValues: Record<string, string>;
}) {
  const [expandedBlockIds, setExpandedBlockIds] = useState<Set<string>>(() => new Set(['input-binding', SAMPLE_LLM_BLOCK.id, 'final-output']));
  const [toolCallsCleared, setToolCallsCleared] = useState(false);
  const blocks = useMemo(() => buildDebuggerBlockTraces(run, inputValues, toolCallsCleared), [inputValues, run, toolCallsCleared]);

  function toggleBlock(id: string) {
    setExpandedBlockIds((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  return (
    <section className="of-panel" style={{ padding: 12, minHeight: 680 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center' }}>
        <div>
          <div className="of-eyebrow">Debugger</div>
          <h2 className="of-heading-md" style={{ margin: 0 }}>Block trace</h2>
        </div>
        <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
          <StatusPill tone={run?.status === 'failed' ? 'warning' : run ? 'success' : 'info'}>{run?.status ?? 'not run'}</StatusPill>
          <button type="button" className="of-button" onClick={() => setToolCallsCleared((current) => !current)} disabled={!run}>
            {toolCallsCleared ? 'Restore tool calls' : 'Clear tool calls'}
          </button>
        </div>
      </div>
      <p className="of-text-muted" style={{ margin: '8px 0 0' }}>
        Draft traces are security-filtered and retained locally for this session; published or automation runs would use platform retention policy.
      </p>
      <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
        {blocks.length === 0 ? (
          <div className="of-panel-muted" style={{ padding: 12 }}>
            Run the draft Logic function to open the debugger with inputs, prompts, tool calls, outputs, errors, and the final result.
          </div>
        ) : blocks.map((block) => (
          <TraceCard key={block.id} block={block} expanded={expandedBlockIds.has(block.id)} onToggle={() => toggleBlock(block.id)} />
        ))}
      </div>
    </section>
  );
}

function RunPanel({ inputs, llmBlock, inputValues, onInputChange, latestRun, recentRuns, onRun, onSelectRun }: {
  inputs: LogicInputDefinition[];
  llmBlock: LogicLlmBlockConfig;
  inputValues: Record<string, string>;
  onInputChange: (apiName: string, value: string) => void;
  latestRun?: LogicPreviewRunResult;
  recentRuns: LogicPreviewRunResult[];
  onRun: () => void;
  onSelectRun: (run: LogicPreviewRunResult) => void;
}) {
  const canRun = validateLogicInputBoard(inputs).length === 0 && validateLlmBlock(llmBlock, inputs).filter((issue) => issue.severity === 'error').length === 0;
  return (
    <section className="of-panel" style={{ padding: 12, minHeight: 680 }}>
      <div className="of-eyebrow">Run panel</div>
      <h2 className="of-heading-md" style={{ margin: 0 }}>Draft preview execution</h2>
      <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
        {inputs.filter((input) => input.required || ['customerRecord', 'complaintText', 'baseRisk', 'delayHours'].includes(input.apiName)).map((input) => (
          <Field key={input.id} label={input.apiName}>
            {input.apiName === 'complaintText' ? (
              <textarea className="of-textarea" value={inputValues[input.apiName] ?? ''} onChange={(event) => onInputChange(input.apiName, event.target.value)} style={{ minHeight: 88 }} />
            ) : (
              <input className="of-input" value={inputValues[input.apiName] ?? ''} onChange={(event) => onInputChange(input.apiName, event.target.value)} />
            )}
          </Field>
        ))}
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" className="of-button of-button--primary" disabled={!canRun} onClick={onRun}>Run draft</button>
          <button type="button" className="of-button" disabled={!latestRun || !canRun} onClick={onRun}>Rerun latest</button>
          <button type="button" className="of-button" disabled={!latestRun}>Save as test case</button>
        </div>
        <div className="of-panel-muted" style={{ padding: 12 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <strong>Latest result</strong>
            <StatusPill tone={latestRun?.status === 'failed' ? 'warning' : latestRun ? 'success' : 'info'}>{latestRun?.status ?? 'idle'}</StatusPill>
          </div>
          <p className="of-text-muted" style={{ margin: '6px 0 0' }}>{latestRun?.result ?? 'No draft run yet. Edit inputs and run without publishing.'}</p>
          {latestRun && (
            <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 8px', margin: '10px 0 0' }}>
              <dt className="of-text-muted">Duration</dt><dd style={{ margin: 0 }}>{latestRun.durationMs} ms</dd>
              <dt className="of-text-muted">Run ID</dt><dd style={{ margin: 0 }}>{latestRun.id}</dd>
              <dt className="of-text-muted">Started</dt><dd style={{ margin: 0 }}>{latestRun.metadata.startedAtIso}</dd>
              <dt className="of-text-muted">Metadata</dt><dd style={{ margin: 0 }}>{latestRun.metadata.toolCallCount} tool calls · {latestRun.metadata.retainedUntil}</dd>
            </dl>
          )}
        </div>
        <div className="of-panel-muted" style={{ padding: 12 }}>
          <strong>Recent runs</strong>
          <div style={{ display: 'grid', gap: 6, marginTop: 8 }}>
            {recentRuns.length === 0 ? <p className="of-text-muted" style={{ margin: 0 }}>Runs from this draft session appear here.</p> : recentRuns.map((run) => (
              <button key={run.id} type="button" className="of-button" onClick={() => onSelectRun(run)} style={{ justifyContent: 'space-between' }}>
                <span>{run.id}</span>
                <span>{run.status} · {run.durationMs} ms</span>
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

export function LogicAuthoringPage() {
  const [activeTab, setActiveTab] = useState<ConfigTab>('inputs');
  const [selectedInputId, setSelectedInputId] = useState(SAMPLE_INPUTS[0].id);
  const [inputs, setInputs] = useState<LogicInputDefinition[]>(SAMPLE_INPUTS);
  const [llmBlock, setLlmBlock] = useState<LogicLlmBlockConfig>(SAMPLE_LLM_BLOCK);
  const [runInputValues, setRunInputValues] = useState<Record<string, string>>(DEFAULT_RUN_INPUTS);
  const [latestRun, setLatestRun] = useState<LogicPreviewRunResult>();
  const [recentRuns, setRecentRuns] = useState<LogicPreviewRunResult[]>([]);
  const boardIssues = useMemo(() => validateLogicInputBoard(inputs), [inputs]);

  function updateInput(next: LogicInputDefinition) {
    setInputs((current) => current.map((input) => (input.id === next.id ? next : input)));
  }

  function updateRunInput(apiName: string, value: string) {
    setRunInputValues((current) => ({ ...current, [apiName]: value }));
  }

  function runDraftPreview() {
    const run = executeDraftLogicPreview(llmBlock, inputs, runInputValues);
    setLatestRun(run);
    setRecentRuns((current) => [run, ...current.filter((candidate) => candidate.id !== run.id)].slice(0, 5));
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 10 }}>
      <div className="of-toolbar" style={{ justifyContent: 'space-between' }}>
        <div>
          <div className="of-eyebrow">AIP Logic</div>
          <h1 className="of-heading-lg" style={{ margin: 0 }}>Customer triage logic</h1>
          <div className="of-text-muted">Project: Customer operations / Folder: AIP demos / Draft v7</div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <StatusPill tone={boardIssues.length === 0 ? 'success' : 'warning'}>{boardIssues.length === 0 ? 'ready to run' : `${boardIssues.length} input issues`}</StatusPill>
          <button className="of-button" type="button">Save draft</button>
          <button className="of-button of-button--primary" type="button">Publish</button>
        </div>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(520px, 1.4fr) minmax(320px, 0.9fr) minmax(300px, 0.8fr) 170px', gap: 10, alignItems: 'stretch' }}>
        <section className="of-panel" style={{ padding: 12, minHeight: 680 }}>
          <div style={{ display: 'flex', gap: 6, borderBottom: '1px solid var(--border-subtle)', paddingBottom: 8, marginBottom: 10 }}>
            {(['inputs', 'blocks', 'outputs'] as const).map((tab) => (
              <button key={tab} type="button" className={`of-tab ${activeTab === tab ? 'of-tab-active' : ''}`} onClick={() => setActiveTab(tab)}>
                {tab[0].toUpperCase() + tab.slice(1)}
              </button>
            ))}
          </div>
          {activeTab === 'inputs' && <InputsBoard inputs={inputs} selectedId={selectedInputId} onSelect={setSelectedInputId} onChange={updateInput} />}
          {activeTab === 'blocks' && <BlocksBoard inputs={inputs} llmBlock={llmBlock} onChange={setLlmBlock} />}
          {activeTab === 'outputs' && <OutputsBoard />}
        </section>

        <DebuggerPanel run={latestRun} inputValues={runInputValues} />

        <RunPanel
          inputs={inputs}
          llmBlock={llmBlock}
          inputValues={runInputValues}
          onInputChange={updateRunInput}
          latestRun={latestRun}
          recentRuns={recentRuns}
          onRun={runDraftPreview}
          onSelectRun={setLatestRun}
        />

        <aside className="of-panel" style={{ padding: 8, minHeight: 680 }} aria-label="Logic resource entry points">
          <div className="of-eyebrow" style={{ padding: '6px 8px' }}>Resource</div>
          <nav style={{ display: 'grid', gap: 6 }}>
            {RIGHT_RAIL.map((entry, index) => (
              <button
                key={entry}
                type="button"
                className="of-button"
                style={{ justifyContent: 'space-between', minHeight: 34, background: index === 6 ? 'var(--status-info-bg)' : 'var(--bg-panel)' }}
              >
                <span>{entry}</span>
                <span className="of-text-soft">›</span>
              </button>
            ))}
          </nav>
        </aside>
      </div>
    </section>
  );
}
