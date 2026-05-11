// WorkshopRuntimeProvider — runtime-side counterpart of `PreviewRuntime`
// from WorkshopEditorPage.tsx. Wraps a subtree with:
//
//   - WorkshopRuntimeContext   : active object selection, filter values,
//                                refreshKey, button-click → action modal.
//   - WorkshopDataContext      : the variables declared in app.settings
//                                and the object types loaded from the
//                                ontology service.
//
// AppRuntimePage uses this around <AppRenderer> so that any Workshop
// widget rendered through the registry receives the context it needs.
//
// We intentionally do NOT reuse PreviewRuntime as-is: that one renders
// editor chrome (header, "Edit" button, lineage shortcut) and accepts a
// page list. The published runtime owns its own chrome via AppRenderer.

import { useCallback, useEffect, useMemo, useState } from 'react';

import type { AppDefinition } from '@/lib/api/apps';
import { listObjectTypes, type ObjectInstance, type ObjectType } from '@/lib/api/ontology';
import {
  ActionFormModal,
  readWorkshopVariables,
  WorkshopRuntimeContext,
  type ButtonGroupButton,
  type RuntimeApi,
  type WorkshopVariable,
} from '@/routes/apps/WorkshopEditorPage';
import type { WorkshopMapFeatureCollection } from './workshopMap';
import { createWorkshopVariableEngine, type WorkshopRuntimeFilterMetadata } from './workshopVariables';

import { WorkshopDataContext, type WorkshopDataContextValue } from './workshop-context';

interface FilterRuntimeValue {
  values?: string[];
  search?: string;
  range_min?: string;
  range_max?: string;
}

export function WorkshopRuntimeProvider({
  app,
  children,
}: {
  app: AppDefinition;
  children: React.ReactNode;
}) {
  const variables: WorkshopVariable[] = useMemo(
    () => readWorkshopVariables(app.settings),
    [app.settings],
  );

  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const response = await listObjectTypes();
        if (cancelled) return;
        const items = Array.isArray(response)
          ? (response as ObjectType[])
          : ((response as { data?: ObjectType[] }).data ?? []);
        setObjectTypes(items);
      } catch {
        if (!cancelled) setObjectTypes([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const [activeObjects, setActiveObjects] = useState<Record<string, ObjectInstance | null>>({});
  const [selectedObjectSets, setSelectedObjectSets] = useState<Record<string, ObjectInstance[]>>({});
  const [shapeOutputs, setShapeOutputs] = useState<Record<string, WorkshopMapFeatureCollection | null>>({});
  const [filterValues, setFilterValues] = useState<Record<string, FilterRuntimeValue>>({});
  const [filterMetadata, setFilterMetadata] = useState<Record<string, WorkshopRuntimeFilterMetadata>>({});
  const [primitiveValues, setPrimitiveValues] = useState<Record<string, unknown>>({});
  const [runtimeParameters, setRuntimeParametersState] = useState<Record<string, string>>({});
  const [refreshKey, setRefreshKey] = useState(0);
  const [actionModal, setActionModal] = useState<{ button: ButtonGroupButton } | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  const setActiveObject = useCallback((variableId: string, object: ObjectInstance | null) => {
    setActiveObjects((current) => ({ ...current, [variableId]: object }));
  }, []);
  const setSelectedObjectSet = useCallback((variableId: string, objects: ObjectInstance[]) => {
    setSelectedObjectSets((current) => {
      const existing = current[variableId] ?? [];
      if (sameObjectSelection(existing, objects)) return current;
      return { ...current, [variableId]: objects };
    });
  }, []);
  const setShapeOutput = useCallback((variableId: string, shape: WorkshopMapFeatureCollection | null) => {
    setShapeOutputs((current) => {
      if (sameShapeOutput(current[variableId] ?? null, shape)) return current;
      return { ...current, [variableId]: shape };
    });
  }, []);
  const setFilterValue = useCallback((filterId: string, value: FilterRuntimeValue, metadata?: WorkshopRuntimeFilterMetadata) => {
    setFilterValues((current) => ({ ...current, [filterId]: value }));
    if (metadata) {
      setFilterMetadata((current) => ({ ...current, [filterId]: { ...(current[filterId] ?? {}), ...metadata } }));
    }
  }, []);
  const setPrimitiveValue = useCallback((variableId: string, value: unknown) => {
    setPrimitiveValues((current) => (Object.is(current[variableId], value) ? current : { ...current, [variableId]: value }));
  }, []);
  const setRuntimeParameters = useCallback((parameters: Record<string, string>) => {
    setRuntimeParametersState((current) => (sameStringRecord(current, parameters) ? current : { ...parameters }));
  }, []);
  const onButtonClick = useCallback((button: ButtonGroupButton) => {
    if (button.on_click_kind === 'action' && button.action_type_id) {
      setActionModal({ button });
    }
  }, []);
  const variableEngine = useMemo(() => createWorkshopVariableEngine(variables, {
    activeObjects,
    selectedObjectSets,
    shapeOutputs,
    filterValues,
    filterMetadata,
    primitiveValues,
    runtimeParameters,
  }), [activeObjects, filterMetadata, filterValues, primitiveValues, runtimeParameters, selectedObjectSets, shapeOutputs, variables]);

  const runtime = useMemo<RuntimeApi>(() => ({
    preview: true,
    activeObjects,
    selectedObjectSets,
    shapeOutputs,
    filterValues,
    filterMetadata,
    primitiveValues,
    runtimeParameters,
    variableEngine,
    refreshKey,
    setActiveObject,
    setSelectedObjectSet,
    setShapeOutput,
    setFilterValue,
    setPrimitiveValue,
    setRuntimeParameters,
    onButtonClick,
  }), [activeObjects, filterMetadata, filterValues, primitiveValues, refreshKey, runtimeParameters, selectedObjectSets, setActiveObject, setFilterValue, setPrimitiveValue, setRuntimeParameters, setSelectedObjectSet, setShapeOutput, shapeOutputs, variableEngine, onButtonClick]);

  const data = useMemo<WorkshopDataContextValue>(
    () => ({ variables, objectTypes }),
    [variables, objectTypes],
  );

  return (
    <WorkshopRuntimeContext.Provider value={runtime}>
      <WorkshopDataContext.Provider value={data}>
        {children}
        {actionModal ? (
          <ActionFormModal
            button={actionModal.button}
            variables={variables}
            activeObjects={activeObjects}
            selectedObjectSets={selectedObjectSets}
            objectTypes={objectTypes}
            onClose={() => setActionModal(null)}
            onSuccess={() => {
              setActionModal(null);
              setToast('Edits successfully applied.');
              setRefreshKey((key) => key + 1);
              window.setTimeout(() => setToast(null), 4000);
            }}
          />
        ) : null}
        {toast ? (
          <div
            role="status"
            style={{
              position: 'fixed',
              top: 16,
              left: '50%',
              transform: 'translateX(-50%)',
              zIndex: 100,
              display: 'inline-flex',
              alignItems: 'center',
              gap: 10,
              padding: '10px 16px',
              borderRadius: 6,
              background: '#15803d',
              color: '#fff',
              fontSize: 13,
              boxShadow: '0 8px 24px rgba(15, 23, 42, 0.18)',
            }}
          >
            <span>{toast}</span>
            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => setToast(null)}
              style={{ border: 0, background: 'transparent', color: '#fff', cursor: 'pointer' }}
            >
              ×
            </button>
          </div>
        ) : null}
      </WorkshopDataContext.Provider>
    </WorkshopRuntimeContext.Provider>
  );
}

function sameObjectSelection(left: ObjectInstance[], right: ObjectInstance[]) {
  if (left.length !== right.length) return false;
  return left.every((entry, index) => entry.id === right[index]?.id);
}

function sameShapeOutput(left: WorkshopMapFeatureCollection | null, right: WorkshopMapFeatureCollection | null) {
  if (left === right) return true;
  if (!left || !right) return false;
  return JSON.stringify(left) === JSON.stringify(right);
}

function sameStringRecord(left: Record<string, string>, right: Record<string, string>) {
  const leftKeys = Object.keys(left);
  const rightKeys = Object.keys(right);
  if (leftKeys.length !== rightKeys.length) return false;
  return leftKeys.every((key) => left[key] === right[key]);
}
