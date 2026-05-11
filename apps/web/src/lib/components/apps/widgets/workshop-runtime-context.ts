import { createContext, useContext } from 'react';

import type { AppWidget } from '@/lib/api/apps';
import type { ObjectInstance } from '@/lib/api/ontology';
import type { WorkshopMapFeatureCollection } from './workshopMap';
import {
  EMPTY_WORKSHOP_VARIABLE_ENGINE,
  type WorkshopRuntimeFilterMetadata,
  type WorkshopVariableEngineResult,
} from './workshopVariables';
import type { WorkshopObjectSetExecutionOptions, WorkshopObjectSetExecutionResult } from './workshopObjectSets';
import type { WorkshopEventExecution, WorkshopEventHandlers } from './workshopEvents';

export type ButtonOnClickKind = 'none' | 'action' | 'event' | 'export' | 'url';
export type ParameterDefaultKind = 'none' | 'variable' | 'static' | 'active_object';
export type ParameterDefaultVisibility = 'visible' | 'disabled' | 'hidden';

export interface ButtonParameterDefault {
  kind: ParameterDefaultKind;
  variable_id?: string;
  static_value?: unknown;
  visibility?: ParameterDefaultVisibility;
}

export interface ButtonGroupButton {
  id: string;
  label: string;
  on_click_kind: ButtonOnClickKind;
  action_type_id: string;
  parameter_defaults: Record<string, ButtonParameterDefault>;
  default_layout: 'form' | 'table';
  switch_layout: boolean;
  conditional_visibility: boolean;
}

export interface WorkshopFilterRuntimeValue {
  values?: string[];
  search?: string;
  range_min?: string;
  range_max?: string;
}

export interface RuntimeApi {
  preview: boolean;
  activeObjects: Record<string, ObjectInstance | null>;
  selectedObjectSets: Record<string, ObjectInstance[]>;
  shapeOutputs: Record<string, WorkshopMapFeatureCollection | null>;
  filterValues: Record<string, WorkshopFilterRuntimeValue>;
  filterMetadata: Record<string, WorkshopRuntimeFilterMetadata>;
  primitiveValues: Record<string, unknown>;
  runtimeParameters: Record<string, string>;
  variableEngine: WorkshopVariableEngineResult;
  refreshKey: number;
  setActiveObject: (variableId: string, object: ObjectInstance | null) => void;
  setSelectedObjectSet: (variableId: string, objects: ObjectInstance[]) => void;
  setShapeOutput: (variableId: string, shape: WorkshopMapFeatureCollection | null) => void;
  setFilterValue: (filterId: string, value: WorkshopFilterRuntimeValue, metadata?: WorkshopRuntimeFilterMetadata) => void;
  setPrimitiveValue: (variableId: string, value: unknown) => void;
  setRuntimeParameters: (parameters: Record<string, string>) => void;
  executeObjectSet: (variableId: string, options?: WorkshopObjectSetExecutionOptions) => Promise<WorkshopObjectSetExecutionResult>;
  dispatchEvents: (widget: Pick<AppWidget, 'id' | 'events'>, trigger: string, payload?: Record<string, unknown>) => Promise<WorkshopEventExecution[]>;
  setEventHandlers: (handlers: WorkshopEventHandlers) => () => void;
  onButtonClick: (button: ButtonGroupButton) => void;
}

const NO_OP_RUNTIME: RuntimeApi = {
  preview: false,
  activeObjects: {},
  selectedObjectSets: {},
  shapeOutputs: {},
  filterValues: {},
  filterMetadata: {},
  primitiveValues: {},
  runtimeParameters: {},
  variableEngine: EMPTY_WORKSHOP_VARIABLE_ENGINE,
  refreshKey: 0,
  setActiveObject: () => undefined,
  setSelectedObjectSet: () => undefined,
  setShapeOutput: () => undefined,
  setFilterValue: () => undefined,
  setPrimitiveValue: () => undefined,
  setRuntimeParameters: () => undefined,
  executeObjectSet: async (_variableId, options) => ({
    data: [],
    total: 0,
    count: 0,
    objectTypeId: options?.objectTypeId ?? '',
    source: 'object_type',
    filters: [],
    sort: options?.sort ?? [],
    aggregations: [],
    linkedEdges: [],
    knnResults: [],
    contract: {
      object_type_id: options?.objectTypeId ?? '',
      filters: [],
      sort: options?.sort ?? [],
      limit: options?.limit ?? 5000,
      include_count: options?.includeCount ?? true,
      aggregations: options?.aggregations ?? [],
    },
  }),
  dispatchEvents: async () => [],
  setEventHandlers: () => () => undefined,
  onButtonClick: () => undefined,
};

export const WorkshopRuntimeContext = createContext<RuntimeApi>(NO_OP_RUNTIME);

export function useRuntime(): RuntimeApi {
  return useContext(WorkshopRuntimeContext);
}
