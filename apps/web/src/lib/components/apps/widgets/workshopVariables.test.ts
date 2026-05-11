import { describe, expect, it } from 'vitest';

import type { ObjectInstance } from '@/lib/api/ontology';

import {
  createWorkshopVariableEngine,
  variableFiltersForObjectSet,
  type WorkshopVariableLike,
} from './workshopVariables';

function object(id: string, properties: Record<string, unknown>): ObjectInstance {
  return {
    id,
    object_type_id: 'Trail',
    properties,
    created_by: 'test',
    created_at: '2026-05-11T00:00:00Z',
    updated_at: '2026-05-11T00:00:00Z',
  };
}

describe('Workshop variable engine', () => {
  it('builds dependency graph and recomputes object sets when filter outputs change', () => {
    const variables: WorkshopVariableLike[] = [
      {
        id: 'trail_filter',
        kind: 'filter_output',
        name: 'Trail filter',
        object_type_id: 'Trail',
        source_widget_id: 'filter_widget',
      },
      {
        id: 'filtered_trails',
        kind: 'object_set_definition',
        name: 'Filtered trails',
        object_type_id: 'Trail',
        filter_variable_id: 'trail_filter',
      },
    ];

    const first = createWorkshopVariableEngine(variables, {
      filterValues: {
        difficulty_filter: { search: 'hard' },
      },
      filterMetadata: {
        difficulty_filter: {
          outputVariableId: 'trail_filter',
          sourceWidgetId: 'filter_widget',
          propertyName: 'difficulty',
          component: 'search',
        },
      },
    });

    expect(first.evaluationOrder).toEqual(['trail_filter', 'filtered_trails']);
    expect(first.graph.trail_filter.dependents).toEqual(['filtered_trails']);
    expect(first.getObjectSetFilter('trail_filter')?.filters).toEqual([
      { property_name: 'difficulty', operator: 'contains', value: 'hard' },
    ]);
    expect(variableFiltersForObjectSet(variables[1], first)).toEqual([
      { property_name: 'difficulty', operator: 'contains', value: 'hard' },
    ]);

    const second = createWorkshopVariableEngine(variables, {
      filterValues: {
        difficulty_filter: { search: 'easy' },
      },
      filterMetadata: {
        difficulty_filter: {
          outputVariableId: 'trail_filter',
          sourceWidgetId: 'filter_widget',
          propertyName: 'difficulty',
          component: 'search',
        },
      },
    }, first, ['trail_filter']);

    expect(second.dirtyVariableIds).toEqual(expect.arrayContaining(['trail_filter', 'filtered_trails']));
    expect(second.getObjectSet('filtered_trails')?.filters).toEqual([
      { property_name: 'difficulty', operator: 'contains', value: 'easy' },
    ]);
  });

  it('resolves selected object sets and aggregation variables', () => {
    const variables: WorkshopVariableLike[] = [
      { id: 'selected_trails', kind: 'object_set_selection', name: 'Selected trails', object_type_id: 'Trail' },
      {
        id: 'selected_count',
        kind: 'aggregation',
        name: 'Selected count',
        source_variable_id: 'selected_trails',
        metadata: { metric: 'count' },
      },
      {
        id: 'selected_gain_sum',
        kind: 'aggregation',
        name: 'Selected gain',
        source_variable_id: 'selected_trails',
        metadata: { metric: 'sum', property_name: 'gain_ft' },
      },
    ];

    const engine = createWorkshopVariableEngine(variables, {
      selectedObjectSets: {
        selected_trails: [
          object('trail-1', { gain_ft: 750 }),
          object('trail-2', { gain_ft: 250 }),
        ],
      },
    });

    expect(engine.getSelectedObjectSet('selected_trails').map((entry) => entry.id)).toEqual(['trail-1', 'trail-2']);
    expect(engine.getPrimitive('selected_count')).toBe(2);
    expect(engine.getPrimitive('selected_gain_sum')).toBe(1000);
  });

  it('initializes primitive values from URL and runtime parameters', () => {
    const variables: WorkshopVariableLike[] = [
      { id: 'trail_id', kind: 'url_parameter', name: 'Trail ID', metadata: { parameter_name: 'trail' } },
      { id: 'temperature_unit', kind: 'string', name: 'unit', default_value: 'fahrenheit' },
    ];

    const engine = createWorkshopVariableEngine(variables, {
      runtimeParameters: { trail: 'mesa', unit: 'celsius' },
    });

    expect(engine.getPrimitive('trail_id')).toBe('mesa');
    expect(engine.getPrimitive('temperature_unit')).toBe('celsius');
  });

  it('reports missing dependencies and cycles', () => {
    const variables: WorkshopVariableLike[] = [
      { id: 'a', kind: 'aggregation', name: 'A', source_variable_id: 'b' },
      { id: 'b', kind: 'aggregation', name: 'B', source_variable_id: 'a' },
      { id: 'c', kind: 'object_set_definition', name: 'C', filter_variable_id: 'missing' },
    ];

    const engine = createWorkshopVariableEngine(variables);

    expect(engine.diagnostics.map((entry) => entry.code)).toEqual(expect.arrayContaining(['cycle', 'missing_dependency']));
    expect(engine.diagnostics.some((entry) => entry.message.includes('missing'))).toBe(true);
  });
});
