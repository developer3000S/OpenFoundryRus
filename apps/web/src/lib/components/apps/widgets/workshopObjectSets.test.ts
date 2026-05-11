import { describe, expect, it, vi } from 'vitest';

import type { ObjectInstance, ObjectQueryBody, ObjectSetEvaluationResponse } from '@/lib/api/ontology';

import { createWorkshopVariableEngine, type WorkshopVariableLike } from './workshopVariables';
import { executeWorkshopObjectSet, type WorkshopObjectSetExecutorDependencies } from './workshopObjectSets';

function object(id: string, properties: Record<string, unknown>, objectTypeId = 'Trail'): ObjectInstance {
  return {
    id,
    object_type_id: objectTypeId,
    properties,
    created_by: 'test',
    created_at: '2026-05-11T00:00:00Z',
    updated_at: '2026-05-11T00:00:00Z',
  };
}

function deps(objectsByType: Record<string, ObjectInstance[]>): WorkshopObjectSetExecutorDependencies {
  return {
    listObjects: vi.fn(async (typeId: string, params?: { per_page?: number }) => {
      const rows = objectsByType[typeId] ?? [];
      const limit = params?.per_page ?? rows.length;
      return { data: rows.slice(0, limit), total: rows.length };
    }),
    queryObjects: vi.fn(async (typeId: string, body: ObjectQueryBody) => {
      const rows = objectsByType[typeId] ?? [];
      const limit = body.per_page ?? rows.length;
      return { data: rows.slice(0, limit), total: rows.length };
    }),
    evaluateObjectSet: vi.fn(async (id: string): Promise<ObjectSetEvaluationResponse> => ({
      object_set: {
        id,
        name: 'Saved trails',
        description: '',
        base_object_type_id: 'Trail',
        filters: [],
        traversals: [],
        join: null,
        projections: [],
        what_if_label: null,
        policy: {
          allowed_markings: [],
          minimum_clearance: null,
          deny_guest_sessions: false,
          required_restricted_view_id: null,
        },
        materialized_snapshot: null,
        materialized_at: null,
        materialized_row_count: 0,
        owner_id: 'test',
        created_at: '2026-05-11T00:00:00Z',
        updated_at: '2026-05-11T00:00:00Z',
      },
      total_base_matches: 2,
      total_rows: 2,
      traversal_neighbor_count: 0,
      rows: [
        { id: 'trail-1', name: 'Mesa Trail', gain_ft: 800 },
        { id: 'trail-2', name: 'Valley Trail', gain_ft: 200 },
      ],
      generated_at: '2026-05-11T00:00:00Z',
      materialized: false,
    })),
  };
}

describe('Workshop object set execution', () => {
  it('pushes object-type filters to the object database query endpoint', async () => {
    const variables: WorkshopVariableLike[] = [
      {
        id: 'hard_trails',
        kind: 'object_set_definition',
        name: 'Hard trails',
        object_type_id: 'Trail',
        static_filters: [{ property_name: 'difficulty', operator: 'equals', value: 'hard' }],
      },
    ];
    const engine = createWorkshopVariableEngine(variables);
    const fake = deps({ Trail: [object('trail-1', { difficulty: 'hard' })] });

    await executeWorkshopObjectSet({ variableId: 'hard_trails', variables, engine, limit: 25 }, fake);

    expect(fake.queryObjects).toHaveBeenCalledWith('Trail', {
      filters: [{ property_name: 'difficulty', operator: 'equals', value: 'hard' }],
      sort: [],
      per_page: 25,
      limit: 25,
      include_count: true,
      aggregations: [],
    });
    expect(fake.listObjects).not.toHaveBeenCalled();
  });

  it('uses the object database query contract even without filters and forwards sort/count/aggregations', async () => {
    const variables: WorkshopVariableLike[] = [
      {
        id: 'trail_estimates',
        kind: 'object_set_definition',
        name: 'Trail estimates',
        object_type_id: 'Trail',
        metadata: {
          sort: [{ property_name: 'gain_ft', direction: 'desc' }],
          aggregations: [
            { id: 'trail_count', function: 'count' },
            { id: 'avg_gain', function: 'avg', property_name: 'gain_ft' },
          ],
        },
      },
    ];
    const fake = deps({ Trail: [object('trail-1', { gain_ft: 800 })] });

    const result = await executeWorkshopObjectSet({ variableId: 'trail_estimates', variables, engine: createWorkshopVariableEngine(variables), limit: 10 }, fake);

    expect(fake.queryObjects).toHaveBeenCalledWith('Trail', {
      filters: [],
      sort: [{ property_name: 'gain_ft', direction: 'desc' }],
      per_page: 10,
      limit: 10,
      include_count: true,
      aggregations: [
        { id: 'trail_count', alias: undefined, function: 'count', property_name: undefined },
        { id: 'avg_gain', alias: undefined, function: 'avg', property_name: 'gain_ft' },
      ],
    });
    expect(fake.listObjects).not.toHaveBeenCalled();
    expect(result.contract.object_type_id).toBe('Trail');
    expect(result.count).toBe(1);
  });

  it('executes one object set from another and applies only the child filters client side', async () => {
    const variables: WorkshopVariableLike[] = [
      {
        id: 'selected_trails',
        kind: 'object_set_selection',
        name: 'Selected trails',
        object_type_id: 'Trail',
      },
      {
        id: 'steep_selected',
        kind: 'object_set_definition',
        name: 'Steep selected',
        object_type_id: 'Trail',
        source_variable_id: 'selected_trails',
        static_filters: [{ property_name: 'gain_ft', operator: 'gte', value: 500 }],
      },
    ];
    const engine = createWorkshopVariableEngine(variables, {
      selectedObjectSets: {
        selected_trails: [
          object('trail-1', { gain_ft: 800 }),
          object('trail-2', { gain_ft: 200 }),
        ],
      },
    });

    const result = await executeWorkshopObjectSet({ variableId: 'steep_selected', variables, engine }, deps({}));

    expect(result.source).toBe('selected_objects');
    expect(result.data.map((entry) => entry.id)).toEqual(['trail-1']);
  });

  it('sorts selected object sets and computes aggregations locally', async () => {
    const variables: WorkshopVariableLike[] = [
      {
        id: 'selected_trails',
        kind: 'object_set_selection',
        name: 'Selected trails',
        object_type_id: 'Trail',
        metadata: {
          sort: { property_name: 'gain_ft', direction: 'asc' },
          aggregations: [{ id: 'gain_sum', function: 'sum', property_name: 'gain_ft' }],
        },
      },
    ];
    const engine = createWorkshopVariableEngine(variables, {
      selectedObjectSets: {
        selected_trails: [
          object('trail-2', { gain_ft: 800 }),
          object('trail-1', { gain_ft: 200 }),
        ],
      },
    });
    const result = await executeWorkshopObjectSet({ variableId: 'selected_trails', variables, engine }, deps({}));

    expect(result.data.map((entry) => entry.id)).toEqual(['trail-1', 'trail-2']);
    expect(result.aggregations).toEqual([
      { id: 'gain_sum', alias: undefined, function: 'sum', property_name: 'gain_ft', value: 1000, count: 2 },
    ]);
    expect(result.contract.selected_object_ids).toEqual(['trail-2', 'trail-1']);
  });

  it('loads saved object sets through evaluateObjectSet and maps rows to objects', async () => {
    const variables: WorkshopVariableLike[] = [
      {
        id: 'saved_trails',
        kind: 'object_set_definition',
        name: 'Saved trails',
        object_type_id: 'Trail',
        metadata: { saved_object_set_id: 'saved-1' },
      },
    ];
    const fake = deps({});

    const result = await executeWorkshopObjectSet({ variableId: 'saved_trails', variables, engine: createWorkshopVariableEngine(variables) }, fake);

    expect(fake.evaluateObjectSet).toHaveBeenCalledWith('saved-1', { limit: 5000 });
    expect(result.source).toBe('saved_object_set');
    expect(result.data.map((entry) => entry.properties.name)).toEqual(['Mesa Trail', 'Valley Trail']);
  });

  it('supports search-around object sets over selected anchors', async () => {
    const variables: WorkshopVariableLike[] = [
      { id: 'selected_trail', kind: 'object_set_selection', name: 'Selected trail', object_type_id: 'Trail' },
      {
        id: 'nearby_coffee',
        kind: 'object_set_definition',
        name: 'Nearby coffee',
        object_type_id: 'CoffeeShop',
        metadata: {
          search_around: {
            source_variable_id: 'selected_trail',
            radius_miles: 1,
            latitude_property: 'lat',
            longitude_property: 'lon',
          },
        },
      },
    ];
    const engine = createWorkshopVariableEngine(variables, {
      selectedObjectSets: {
        selected_trail: [object('trailhead', { lat: 40, lon: -105 }, 'Trail')],
      },
    });
    const fake = deps({
      CoffeeShop: [
        object('coffee-near', { lat: 40.005, lon: -105.005 }, 'CoffeeShop'),
        object('coffee-far', { lat: 41, lon: -106 }, 'CoffeeShop'),
      ],
    });

    const result = await executeWorkshopObjectSet({ variableId: 'nearby_coffee', variables, engine }, fake);

    expect(result.source).toBe('search_around');
    expect(result.data.map((entry) => entry.id)).toEqual(['coffee-near']);
  });

  it('pushes link-type search-around traversal to the object database query endpoint', async () => {
    const variables: WorkshopVariableLike[] = [
      { id: 'selected_trail', kind: 'object_set_selection', name: 'Selected trail', object_type_id: 'Trail' },
      {
        id: 'linked_coffee',
        kind: 'object_set_definition',
        name: 'Linked coffee',
        object_type_id: 'CoffeeShop',
        metadata: {
          search_around: {
            source_variable_id: 'selected_trail',
            link_type_ids: ['trail_near_coffee'],
            direction: 'outgoing',
            depth: 1,
          },
        },
      },
    ];
    const engine = createWorkshopVariableEngine(variables, {
      selectedObjectSets: {
        selected_trail: [object('trail-1', { name: 'Mesa Trail' }, 'Trail')],
      },
    });
    const fake = deps({ CoffeeShop: [object('coffee-1', { name: 'Boxcar Coffee' }, 'CoffeeShop')] });
    vi.mocked(fake.queryObjects).mockResolvedValueOnce({
      data: [object('coffee-1', { name: 'Boxcar Coffee' }, 'CoffeeShop')],
      total: 1,
      count: 1,
      linked_edges: [{
        link_id: 'trail_near_coffee:trail-1:coffee-1',
        link_type_id: 'trail_near_coffee',
        source_object_id: 'trail-1',
        target_object_id: 'coffee-1',
        direction: 'outgoing',
        depth: 1,
      }],
    });

    const result = await executeWorkshopObjectSet({ variableId: 'linked_coffee', variables, engine, limit: 25 }, fake);

    expect(fake.queryObjects).toHaveBeenCalledWith('CoffeeShop', {
      filters: [],
      sort: [],
      per_page: 25,
      limit: 25,
      include_count: true,
      aggregations: [],
      search_around: {
        source_object_ids: ['trail-1'],
        link_type_ids: ['trail_near_coffee'],
        direction: 'outgoing',
        depth: 1,
        target_object_type_id: 'CoffeeShop',
      },
    });
    expect(result.source).toBe('search_around');
    expect(result.data.map((entry) => entry.id)).toEqual(['coffee-1']);
    expect(result.linkedEdges).toHaveLength(1);
    expect(result.contract.search_around?.link_type_ids).toEqual(['trail_near_coffee']);
  });

  it('pushes KNN vector retrieval from a selected object to the object database query endpoint', async () => {
    const variables: WorkshopVariableLike[] = [
      { id: 'selected_run', kind: 'object_set_selection', name: 'Selected run', object_type_id: 'Run' },
      {
        id: 'similar_runs',
        kind: 'object_set_definition',
        name: 'Similar runs',
        object_type_id: 'Run',
        static_filters: [{ property_name: 'activity_type', operator: 'equals', value: 'trail' }],
        metadata: {
          knn: {
            source_variable_id: 'selected_run',
            property_name: 'trail_vector',
            k: 2,
            metric: 'euclidean',
          },
        },
      },
    ];
    const engine = createWorkshopVariableEngine(variables, {
      selectedObjectSets: {
        selected_run: [object('run-anchor', { trail_vector: [6, 120], activity_type: 'trail' }, 'Run')],
      },
    });
    const fake = deps({ Run: [object('run-1', { name: 'Mesa effort', trail_vector: [5, 100] }, 'Run')] });
    vi.mocked(fake.queryObjects).mockResolvedValueOnce({
      data: [object('run-1', {
        name: 'Mesa effort',
        trail_vector: [5, 100],
        __of_knn_rank: 1,
        __of_knn_score: 0.04756,
      }, 'Run')],
      total: 1,
      count: 1,
      knn_results: [{
        object_id: 'run-1',
        rank: 1,
        score: 0.04756,
        distance: 20.02498,
        metric: 'euclidean',
        property_name: 'trail_vector',
      }],
    });

    const result = await executeWorkshopObjectSet({ variableId: 'similar_runs', variables, engine, limit: 25 }, fake);

    expect(fake.queryObjects).toHaveBeenCalledWith('Run', {
      filters: [{ property_name: 'activity_type', operator: 'equals', value: 'trail' }],
      sort: [],
      per_page: 25,
      limit: 25,
      include_count: true,
      aggregations: [],
      knn: {
        property_name: 'trail_vector',
        vector: [6, 120],
        k: 2,
        metric: 'euclidean',
      },
    });
    expect(result.source).toBe('knn');
    expect(result.knnResults).toHaveLength(1);
    expect(result.contract.knn?.property_name).toBe('trail_vector');
    expect(result.contract.knn?.vector).toEqual([6, 120]);
  });
});
