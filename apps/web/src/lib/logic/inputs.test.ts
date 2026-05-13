import { describe, expect, it } from 'vitest';

import { LOGIC_INPUT_TYPES, validateLogicInputBoard, validateLogicInputDefinition, type LogicInputDefinition } from './inputs';

const baseInput: LogicInputDefinition = {
  id: 'input-1',
  name: 'Order',
  apiName: 'orderInput',
  type: 'string',
  required: true,
};

describe('Logic input board validation', () => {
  it('enumerates the documented AIPLE.3 input types', () => {
    expect(LOGIC_INPUT_TYPES).toEqual([
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
    ]);
  });

  it('validates API names and duplicate names', () => {
    expect(validateLogicInputDefinition({ ...baseInput, apiName: '1bad-name' })).toContainEqual({
      field: 'apiName',
      message: 'API name must start with a letter and contain only letters, numbers, and underscores.',
    });

    expect(validateLogicInputBoard([
      baseInput,
      { ...baseInput, id: 'input-2', name: 'Duplicate', apiName: 'ORDERINPUT' },
    ])).toContainEqual({ field: 'ORDERINPUT', message: 'Input API names must be unique within a Logic file.' });
  });

  it('requires object type selections and object set backing compatibility', () => {
    expect(validateLogicInputDefinition({ ...baseInput, type: 'object', objectTypeId: undefined })).toContainEqual({
      field: 'objectTypeId',
      message: 'object inputs require an object type selection.',
    });

    expect(validateLogicInputDefinition({
      ...baseInput,
      type: 'object_set',
      objectTypeId: 'Customer',
      objectSetObjectTypeId: 'Order',
    })).toContainEqual({
      field: 'objectSetObjectTypeId',
      message: 'Object set backing type must match the selected object type.',
    });
  });

  it('validates typed default values', () => {
    expect(validateLogicInputDefinition({ ...baseInput, type: 'array', defaultValue: '{bad}' })).toContainEqual({
      field: 'defaultValue',
      message: 'array defaults must be valid JSON arrays.',
    });
    expect(validateLogicInputDefinition({ ...baseInput, type: 'short', defaultValue: '40000' })).toContainEqual({
      field: 'defaultValue',
      message: 'Short defaults must be whole numbers between -32768 and 32767.',
    });
    expect(validateLogicInputDefinition({ ...baseInput, type: 'timestamp', defaultValue: '2026-05-13T12:00:00Z' })).toHaveLength(0);
  });

  it('validates model variable compatibility', () => {
    expect(validateLogicInputDefinition({
      ...baseInput,
      type: 'model',
      modelVariableKind: 'embedding',
      compatibleModelKinds: ['llm'],
    })).toContainEqual({
      field: 'modelVariableKind',
      message: 'Model variable kind is not compatible with the selected model slot.',
    });
  });
});
