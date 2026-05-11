import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';

import { getApp, updateApp, type AppDefinition, type AppPage, type AppWidget, type AppSettings, type WorkshopHeaderSettings } from '@/lib/api/apps';
import { getActionType, listActionTypes, listObjectTypes, listObjects, listProperties, queryObjects, updateObject, type ActionInputField, type ActionType, type ObjectInstance, type ObjectType, type Property } from '@/lib/api/ontology';
import { Glyph, type GlyphName } from '@/lib/components/ui/Glyph';
import { EChartCanvas } from '@/lib/components/EChartCanvas';
import { WorkshopMapWidget } from '@/lib/components/apps/widgets/WorkshopMapWidget';
import { readMapLayerConfigs, readMapOverlayConfigs, type WorkshopMapFeatureCollection, type WorkshopMapLayerConfig, type WorkshopMapOverlayLayerConfig } from '@/lib/components/apps/widgets/workshopMap';
import {
  createWorkshopVariableEngine,
  EMPTY_WORKSHOP_VARIABLE_ENGINE,
  variableFiltersForObjectSet,
  type WorkshopRuntimeFilterMetadata,
  type WorkshopVariableEngineResult,
} from '@/lib/components/apps/widgets/workshopVariables';

type LeftTab = 'layout' | 'outline' | 'variables' | 'settings';

interface FilterRuntimeValue {
  values?: string[];
  search?: string;
  range_min?: string;
  range_max?: string;
}

interface RuntimeApi {
  preview: boolean;
  activeObjects: Record<string, ObjectInstance | null>;
  selectedObjectSets: Record<string, ObjectInstance[]>;
  shapeOutputs: Record<string, WorkshopMapFeatureCollection | null>;
  filterValues: Record<string, FilterRuntimeValue>;
  filterMetadata: Record<string, WorkshopRuntimeFilterMetadata>;
  primitiveValues: Record<string, unknown>;
  runtimeParameters: Record<string, string>;
  variableEngine: WorkshopVariableEngineResult;
  refreshKey: number;
  setActiveObject: (variableId: string, object: ObjectInstance | null) => void;
  setSelectedObjectSet: (variableId: string, objects: ObjectInstance[]) => void;
  setShapeOutput: (variableId: string, shape: WorkshopMapFeatureCollection | null) => void;
  setFilterValue: (filterId: string, value: FilterRuntimeValue, metadata?: WorkshopRuntimeFilterMetadata) => void;
  setPrimitiveValue: (variableId: string, value: unknown) => void;
  setRuntimeParameters: (parameters: Record<string, string>) => void;
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
  onButtonClick: () => undefined,
};

const EMPTY_SELECTED_OBJECTS: ObjectInstance[] = [];

export const WorkshopRuntimeContext = createContext<RuntimeApi>(NO_OP_RUNTIME);
export type { RuntimeApi };

function useRuntime(): RuntimeApi {
  return useContext(WorkshopRuntimeContext);
}

interface SelectionState {
  kind: 'header' | 'page' | 'section' | 'widget';
  id: string;
}

interface HeaderUiState {
  enable_module_header: boolean;
  custom_color: boolean;
  enable_app_logo: boolean;
  logo_kind: 'icon' | 'image';
  enable_favoriting: boolean;
  image_url: string;
}

interface ColorOption {
  id: string;
  label: string;
  hex: string;
}

const HEADER_COLORS: ColorOption[] = [
  { id: 'blue-1', label: 'Blue 1', hex: '#cfe1ff' },
  { id: 'blue-2', label: 'Blue 2', hex: '#9ec3ff' },
  { id: 'blue-3', label: 'Blue 3', hex: '#5b9bff' },
  { id: 'blue-4', label: 'Blue 4', hex: '#2d72d2' },
  { id: 'blue-5', label: 'Blue 5', hex: '#1f4ea0' },
  { id: 'green', label: 'Green', hex: '#15803d' },
  { id: 'orange', label: 'Orange', hex: '#cf923f' },
  { id: 'red', label: 'Red', hex: '#b42318' },
  { id: 'purple', label: 'Purple', hex: '#7c5dd6' },
  { id: 'gray', label: 'Gray', hex: '#5c7080' },
];

const HEADER_ICON_OPTIONS: Array<{ id: GlyphName; label: string }> = [
  { id: 'cube', label: 'Cube' },
  { id: 'object', label: 'Application' },
  { id: 'database', label: 'Dataset' },
  { id: 'folder', label: 'Folder' },
  { id: 'document', label: 'Document' },
  { id: 'graph', label: 'Graph' },
  { id: 'list', label: 'List' },
  { id: 'home', label: 'Home' },
  { id: 'pie-chart', label: 'Pie chart' },
  { id: 'shield', label: 'Shield' },
  { id: 'sparkles', label: 'Sparkles' },
  { id: 'badge-check', label: 'Badge' },
  { id: 'view-grid', label: 'Grid' },
  { id: 'star', label: 'Star' },
  { id: 'tag', label: 'Tag' },
];

const DEFAULT_HEADER_UI: HeaderUiState = {
  enable_module_header: true,
  custom_color: false,
  enable_app_logo: true,
  logo_kind: 'icon',
  enable_favoriting: false,
  image_url: '',
};

function readHeaderUi(settings: AppSettings | null | undefined): HeaderUiState {
  const raw = (settings as unknown as { workshop_header_ui?: Partial<HeaderUiState> } | null | undefined)?.workshop_header_ui;
  return { ...DEFAULT_HEADER_UI, ...(raw ?? {}) };
}

function colorByHex(hex: string | null | undefined): ColorOption | null {
  if (!hex) return null;
  return HEADER_COLORS.find((option) => option.hex.toLowerCase() === hex.toLowerCase()) ?? null;
}

const DEFAULT_PAGE_ID = 'page';

function defaultPage(): AppPage {
  const sectionA = makeSection();
  const sectionB = makeSection();
  return {
    id: DEFAULT_PAGE_ID,
    name: 'Page',
    path: '/',
    description: '',
    layout: { kind: 'flex', columns: 2, gap: '12px', max_width: '100%' },
    widgets: [sectionA, sectionB],
    visible: true,
  };
}

function makeId(prefix: string) {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return `${prefix}_${crypto.randomUUID()}`;
  return `${prefix}_${Date.now().toString(36)}_${Math.floor(Math.random() * 1e6)}`;
}

function makeSection(): AppWidget {
  return {
    id: makeId('section'),
    widget_type: 'section',
    title: 'Section',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: { column_width_kind: 'flex', column_width: 1 },
    binding: null,
    events: [],
    children: [],
  };
}

function makeObjectTableWidget(): AppWidget {
  return {
    id: makeId('object_table'),
    widget_type: 'object_table',
    title: 'Object table 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: { object_type_id: '', columns: [], default_sort_property: '', default_sort_direction: 'asc', source_variable_id: '' },
    binding: null,
    events: [],
    children: [],
  };
}

function makeObjectSetTitleWidget(): AppWidget {
  return {
    id: makeId('object_set_title'),
    widget_type: 'object_set_title',
    title: 'Object set title 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: { source_variable_id: '' },
    binding: null,
    events: [],
    children: [],
  };
}

type ButtonOnClickKind = 'none' | 'action' | 'event' | 'export' | 'url';
type ParameterDefaultKind = 'none' | 'variable' | 'static' | 'active_object';

interface ButtonParameterDefault {
  kind: ParameterDefaultKind;
  variable_id?: string;
  static_value?: string;
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

function makeButton(label: string): ButtonGroupButton {
  return {
    id: makeId('btn'),
    label,
    on_click_kind: 'none',
    action_type_id: '',
    parameter_defaults: {},
    default_layout: 'form',
    switch_layout: false,
    conditional_visibility: false,
  };
}

function makeButtonGroupWidget(): AppWidget {
  return {
    id: makeId('button_group'),
    widget_type: 'button_group',
    title: 'Button group 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: {
      button_type: 'inline',
      buttons: [makeButton('Button 1')] as ButtonGroupButton[],
      orientation: 'horizontal',
      fill_horizontal: false,
      row_height_kind: 'auto',
      row_height_value: 600,
    },
    binding: null,
    events: [],
    children: [],
  };
}

interface PropertyListItem {
  id: string;
  property_names: string[];
}

function makePropertyListWidget(): AppWidget {
  return {
    id: makeId('property_list'),
    widget_type: 'property_list',
    title: 'Property list 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: {
      source_variable_id: '',
      items: [{ id: makeId('item'), property_names: [] }] as PropertyListItem[],
      number_of_columns: 2,
      enable_value_wrapping: false,
      row_height_kind: 'auto',
      row_height_value: 600,
    },
    binding: null,
    events: [],
    children: [],
  };
}

function makeMapWidget(): AppWidget {
  return {
    id: makeId('map'),
    widget_type: 'map',
    title: 'Map 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 3 },
    props: {
      base_layer_kind: 'blank',
      zoom: 11,
      output_variable_id: '',
      output_object_set_variable_id: '',
      output_shape_variable_id: '',
      shape_search_output_variable_id: '',
      enable_shape_drawing: true,
      layers: [
        {
          id: makeId('map_layer'),
          title: 'Objects',
          source: 'object_set',
          loading_mode: 'eager',
          source_variable_id: '',
          object_type_id: '',
          tile_layer_id: '',
          tile_page_size: 500,
          tile_simplify_tolerance: 0,
          geometry_type: 'point',
          latitude_field: 'lat',
          longitude_field: 'lon',
          geometry_field: '',
          label_field: 'label',
          color: '#2d72d2',
          visible: true,
          locked: false,
          cluster_enabled: false,
          cluster_radius: 64,
          cluster_max_zoom: 10,
          cluster_color: '#2d72d2',
        },
      ],
      overlay_layers: [],
    },
    binding: null,
    events: [],
    children: [],
  };
}

interface ChartXyLayer {
  id: string;
  title: string;
  data_input: 'object_set' | 'function' | 'time_series';
  source_variable_id: string;
  object_type_id: string;
  layer_type: 'bar' | 'line' | 'scatter';
  show_labels: boolean;
  x_property: string;
  x_bucketing: 'exact' | 'range';
  x_limit: string;
  series_metric: 'count' | 'sum' | 'avg' | 'min' | 'max';
  series_property: string;
  cumulative_sum: boolean;
  segment_by: string;
}

function makeChartXyLayer(): ChartXyLayer {
  return {
    id: makeId('layer'),
    title: 'Layer (bar)',
    data_input: 'object_set',
    source_variable_id: '',
    object_type_id: '',
    layer_type: 'bar',
    show_labels: true,
    x_property: '',
    x_bucketing: 'exact',
    x_limit: '',
    series_metric: 'count',
    series_property: '',
    cumulative_sum: false,
    segment_by: '',
  };
}

function makeChartXyWidget(): AppWidget {
  return {
    id: makeId('chart_xy'),
    widget_type: 'chart_xy',
    title: 'Chart: XY 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: {
      layers: [makeChartXyLayer()] as ChartXyLayer[],
      annotations: [] as Array<{ id: string }>,
      y_axis_kind: 'categorical',
      show_title: false,
      show_color_markers: true,
      enable_numerical_formatting: false,
      sort_by: 'key_asc',
      enable_ontology_colors: true,
      show_legend: false,
      show_tooltips: true,
      allow_exports: true,
      bar_orientation: 'horizontal',
      row_height_kind: 'auto',
      row_height_value: 600,
    },
    binding: null,
    events: [],
    children: [],
  };
}

function makeChartPieWidget(): AppWidget {
  return {
    id: makeId('chart_pie'),
    widget_type: 'chart_pie',
    title: 'Chart: Pie 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: {
      source_variable_id: '',
      object_type_id: '',
      group_by_property: '',
      enable_ontology_colors: true,
      aggregation_metric: 'count',
      aggregation_property: '',
      enable_numeric_formatting: false,
      radius: 0,
      padding: 'large',
      show_legend: true,
      legend_position: 'next-to',
      legend_anchor: 'right',
      row_height_kind: 'auto',
      row_height_value: 600,
    },
    binding: null,
    events: [],
    children: [],
  };
}

function makeFilterListWidget(): AppWidget {
  return {
    id: makeId('filter_list'),
    widget_type: 'filter_list',
    title: 'Filter list 1',
    description: '',
    position: { x: 0, y: 0, width: 1, height: 1 },
    props: {
      object_type_id: '',
      source_variable_id: '',
      filters: [] as FilterEntry[],
      allow_add_remove: false,
      layout: 'vertical',
      output_variable_id: '',
      background_color: 'white',
    },
    binding: null,
    events: [],
    children: [],
  };
}

type FilterComponent = 'multi_select' | 'search' | 'range_numeric' | 'range_date';

interface FilterEntry {
  id: string;
  property_name: string;
  display_name: string;
  component: FilterComponent;
  values: string[];
  range_min: string;
  range_max: string;
}

type VariableKind =
  | 'primitive'
  | 'string'
  | 'numeric'
  | 'boolean'
  | 'array'
  | 'struct'
  | 'date'
  | 'timestamp'
  | 'url_parameter'
  | 'runtime_parameter'
  | 'object_set'
  | 'object_set_definition'
  | 'object_set_filter'
  | 'filter_output'
  | 'object_set_active_object'
  | 'object_set_selection'
  | 'aggregation'
  | 'shape_output';

interface VariableStaticFilter {
  property_name: string;
  operator?: 'equals' | 'contains';
  value: unknown;
}

export interface WorkshopVariable {
  id: string;
  kind: VariableKind;
  name: string;
  object_type_id: string;
  source_widget_id?: string;
  source_variable_id?: string;
  filter_variable_id?: string;
  static_filter?: VariableStaticFilter;
  static_filters?: VariableStaticFilter[];
  default_value?: unknown;
  metadata?: Record<string, unknown>;
}

const VARIABLE_KIND_LABEL: Record<string, string> = {
  primitive: 'Primitive',
  string: 'String',
  numeric: 'Numeric',
  boolean: 'Boolean',
  array: 'Array',
  struct: 'Struct',
  date: 'Date',
  timestamp: 'Timestamp',
  url_parameter: 'URL parameter',
  runtime_parameter: 'Runtime parameter',
  object_set: 'Object set',
  object_set_definition: 'Object set definition',
  object_set_filter: 'Object set filter',
  filter_output: 'Filter output',
  object_set_active_object: 'Active object',
  object_set_selection: 'Selected object set',
  aggregation: 'Aggregation',
  shape_output: 'Shape output',
};

const SECTION_BG_COLORS: Array<{ id: string; label: string; hex: string }> = [
  { id: 'white', label: 'White', hex: '#ffffff' },
  { id: 'light-gray-1', label: 'Light gray 1', hex: '#f7f9fa' },
  { id: 'light-gray-2', label: 'Light gray 2', hex: '#eef1f4' },
  { id: 'light-gray-3', label: 'Light gray 3', hex: '#e3e8ed' },
  { id: 'light-gray-4', label: 'Light gray 4', hex: '#d6dde3' },
  { id: 'light-gray-5', label: 'Light gray 5', hex: '#aab4c0' },
];

const SECTION_HEADER_FORMATS: Array<{ id: string; label: string }> = [
  { id: 'title', label: 'Title' },
  { id: 'contained', label: 'Contained' },
  { id: 'underline', label: 'Underline' },
];

const FILTER_COMPONENT_LABEL: Record<FilterComponent, string> = {
  multi_select: 'Multi-select dropdown',
  search: 'Search',
  range_numeric: 'Numeric range',
  range_date: 'Date range',
};

export function readWorkshopVariables(settings: AppSettings | null | undefined): WorkshopVariable[] {
  const raw = (settings as unknown as { workshop_variables?: WorkshopVariable[] } | null | undefined)?.workshop_variables;
  return Array.isArray(raw) ? raw : [];
}

// fetchObjectsForVariable picks the cheapest path: when the variable carries
// a `static_filter[s]`, push the filter to the bridge via /objects/query so
// the wire payload only contains matching rows. Otherwise, fall back to a
// plain listObjects.
//
// Both legs return { data, total }; the data is already filter-applied by
// the time it returns (server-side when there's a filter, server-side trivially
// when there isn't).
async function fetchObjectsForVariable(
  objectTypeId: string,
  variable: WorkshopVariable | null,
  perPage = 5000,
  variableEngine: WorkshopVariableEngineResult | null = null,
): Promise<{ data: ObjectInstance[]; total: number }> {
  const filters = variableFiltersForObjectSet(variable, variableEngine);
  if (filters.length === 0) {
    const list = await listObjects(objectTypeId, { per_page: perPage });
    return { data: list.data, total: list.total };
  }
  const result = await queryObjects(objectTypeId, {
    filters: filters.map((filter) => ({
      property_name: filter.property_name,
      operator: filter.operator === 'contains' ? 'contains' : 'equals',
      value: filter.value,
    })),
    per_page: perPage,
  });
  return { data: result.data, total: result.total };
}

export function WorkshopEditorPage() {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const mode: 'preview' | 'edit' = searchParams.get('mode') === 'preview' ? 'preview' : 'edit';
  const [app, setApp] = useState<AppDefinition | null>(null);
  const [pages, setPages] = useState<AppPage[]>([]);
  const [selection, setSelection] = useState<SelectionState>({ kind: 'page', id: DEFAULT_PAGE_ID });
  const [leftTab, setLeftTab] = useState<LeftTab>('layout');
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [pickerOpen, setPickerOpen] = useState<{ widgetId: string } | null>(null);
  const [widgetMenuSection, setWidgetMenuSection] = useState<string | null>(null);
  const [layoutMenuSection, setLayoutMenuSection] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<Date | null>(null);
  const [error, setError] = useState('');
  const [headerSettings, setHeaderSettings] = useState<WorkshopHeaderSettings>({ title: null, icon: null, color: null });
  const [headerUi, setHeaderUi] = useState<HeaderUiState>(DEFAULT_HEADER_UI);
  const [variables, setVariables] = useState<WorkshopVariable[]>([]);
  const [editingVariableId, setEditingVariableId] = useState<string | null>(null);
  const [varAddMenuOpen, setVarAddMenuOpen] = useState(false);
  const [previewPageId, setPreviewPageId] = useState('');

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    void (async () => {
      try {
        const [definition, types] = await Promise.all([
          getApp(id),
          listObjectTypes({ per_page: 200 }).then((response) => response.data).catch(() => [] as ObjectType[]),
        ]);
        if (cancelled) return;
        setApp(definition);
        setObjectTypes(types);
        const initialPages = definition.pages.length > 0 ? definition.pages : [defaultPage()];
        setPages(initialPages);
        setSelection({ kind: 'page', id: initialPages[0].id });
        const existingHeader = definition.settings?.workshop_header ?? { title: null, icon: null, color: null };
        setHeaderSettings({
          title: existingHeader.title ?? definition.name,
          icon: existingHeader.icon ?? 'cube',
          color: existingHeader.color ?? '#2d72d2',
        });
        setHeaderUi(readHeaderUi(definition.settings));
        setVariables(readWorkshopVariables(definition.settings));
      } catch (cause) {
        if (!cancelled) setError(cause instanceof Error ? cause.message : 'Failed to load app');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id]);

  useEffect(() => {
    if (pages.length === 0) {
      setPreviewPageId('');
      return;
    }
    if (!previewPageId || !pages.some((page) => page.id === previewPageId)) {
      setPreviewPageId(pages[0].id);
    }
  }, [pages, previewPageId]);

  const activePage = mode === 'preview'
    ? pages.find((page) => page.id === previewPageId) ?? pages[0] ?? null
    : pages[0] ?? null;

  function patchPage(patch: Partial<AppPage>) {
    setPages((current) => current.map((page, index) => (index === 0 ? { ...page, ...patch } : page)));
  }

  function patchSection(sectionId: string, patcher: (section: AppWidget) => AppWidget) {
    if (!activePage) return;
    const updated: AppWidget[] = activePage.widgets.map((section) => (section.id === sectionId ? patcher(section) : section));
    patchPage({ widgets: updated });
  }

  function patchWidget(sectionId: string, widgetId: string, patcher: (widget: AppWidget) => AppWidget) {
    patchSection(sectionId, (section) => ({
      ...section,
      children: section.children.map((widget) => (widget.id === widgetId ? patcher(widget) : widget)),
    }));
  }

  function removeWidget(sectionId: string, widgetId: string) {
    patchSection(sectionId, (section) => ({
      ...section,
      children: section.children.filter((widget) => widget.id !== widgetId),
    }));
  }

  function addSection() {
    if (!activePage) return;
    const next = [...activePage.widgets, makeSection()];
    patchPage({ widgets: next });
  }

  async function save() {
    if (!app) return;
    setSaving(true);
    setError('');
    try {
      const baseSettings = app.settings ?? ({} as AppSettings);
      const nextSettings = {
        ...baseSettings,
        workshop_header: { ...headerSettings },
        workshop_header_ui: { ...headerUi },
        workshop_variables: variables,
      } as AppSettings;
      const updated = await updateApp(app.id, { pages, settings: nextSettings, status: 'published' });
      setApp(updated);
      setSavedAt(new Date());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    if (mode === 'preview') return;
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'i') {
        event.preventDefault();
        navigate(`/workflow-lineage?app=${encodeURIComponent(id)}`);
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [mode, id, navigate]);

  if (!app || !activePage) {
    return (
      <div style={{ padding: 32 }}>
        <p className="of-text-muted">{error || 'Loading editor…'}</p>
        <Link to="/apps" className="of-link">Back to Workshop</Link>
      </div>
    );
  }

  const selectedSection = selection.kind === 'section' ? activePage.widgets.find((s) => s.id === selection.id) ?? null : null;
  const selectedWidget = selection.kind === 'widget'
    ? activePage.widgets.flatMap((section) => section.children.map((widget) => ({ section, widget }))).find((entry) => entry.widget.id === selection.id) ?? null
    : null;

  if (mode === 'preview') {
    return (
      <PreviewRuntime
        app={app}
        pages={pages}
        activePage={activePage}
        variables={variables}
        objectTypes={objectTypes}
        headerSettings={headerSettings}
        headerUi={headerUi}
        onEdit={() => navigate(`/apps/${app.id}/workshop`)}
        onOpenLineage={() => navigate(`/workflow-lineage?app=${encodeURIComponent(app.id)}`)}
      >
        <main style={{ overflow: 'auto', padding: 18 }}>
          {pages.length > 1 ? (
            <nav role="tablist" aria-label="Workshop pages" style={{ display: 'flex', gap: 6, marginBottom: 12 }}>
              {pages.map((page) => {
                const isActive = page.id === activePage.id;
                return (
                  <button
                    key={page.id}
                    type="button"
                    role="tab"
                    aria-selected={isActive}
                    onClick={() => setPreviewPageId(page.id)}
                    style={{
                      padding: '7px 12px',
                      border: `1px solid ${isActive ? 'var(--status-info)' : 'var(--border-subtle)'}`,
                      borderRadius: 4,
                      background: isActive ? 'rgba(45, 114, 210, 0.08)' : '#fff',
                      color: isActive ? 'var(--status-info)' : 'var(--text-strong)',
                      cursor: 'pointer',
                      fontSize: 13,
                      fontWeight: isActive ? 600 : 500,
                    }}
                  >
                    {page.name}
                  </button>
                );
              })}
            </nav>
          ) : null}
          <div style={{ background: '#fff', border: '1px solid var(--border-subtle)', borderRadius: 6, padding: 14, display: 'grid', gridTemplateColumns: activePage.widgets.map((section) => `${flexValue(section)}fr`).join(' '), gap: 14, minHeight: 320 }}>
            {activePage.widgets.map((section) => {
              const paddingControls = ((section.props as { padding_controls?: string })?.padding_controls) ?? 'no-padding';
              const layoutDirection = ((section.props as { layout_direction?: string })?.layout_direction) ?? 'columns';
              const padPx = paddingControls === 'regular' ? 14 : paddingControls === 'custom' ? 14 : 6;
              const backgroundColorId = ((section.props as { background_color?: string })?.background_color) ?? 'white';
              const backgroundHex = SECTION_BG_COLORS.find((option) => option.id === backgroundColorId)?.hex ?? '#ffffff';
              return (
                <section key={section.id} style={{ display: 'grid', gap: 10, padding: padPx, border: '1px solid var(--border-subtle)', borderRadius: 6, alignContent: 'start', background: backgroundHex }}>
                  <SectionHeaderRender section={section} />
                  <div style={{ display: layoutDirection === 'rows' ? 'grid' : 'grid', gridTemplateColumns: layoutDirection === 'rows' ? '1fr' : `repeat(${Math.min(Math.max(section.children.length, 1), 3)}, minmax(0, 1fr))`, gap: 10 }}>
                    {section.children.map((widget) => (
                      <div key={widget.id} style={{ border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff' }}>
                        {widget.widget_type === 'object_table' ? (
                          <ObjectTableWidgetView widget={widget} variables={variables} />
                        ) : widget.widget_type === 'filter_list' ? (
                          <FilterListWidgetView widget={widget} />
                        ) : widget.widget_type === 'object_set_title' ? (
                          <ObjectSetTitleWidgetView widget={widget} variables={variables} objectTypes={objectTypes} />
                        ) : widget.widget_type === 'button_group' ? (
                          <ButtonGroupWidgetView widget={widget} />
                        ) : widget.widget_type === 'property_list' ? (
                          <PropertyListWidgetView widget={widget} variables={variables} />
                        ) : widget.widget_type === 'chart_pie' ? (
                          <ChartPieWidgetView widget={widget} variables={variables} />
                        ) : widget.widget_type === 'chart_xy' ? (
                          <ChartXyWidgetView widget={widget} variables={variables} />
                        ) : widget.widget_type === 'map' ? (
                          <MapWidgetView widget={widget} variables={variables} />
                        ) : null}
                      </div>
                    ))}
                  </div>
                </section>
              );
            })}
          </div>
        </main>
      </PreviewRuntime>
    );
  }

  return (
    <div style={{ position: 'fixed', inset: 0, zIndex: 75, display: 'grid', gridTemplateRows: 'auto 1fr', background: '#fff' }}>
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '8px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <button type="button" className="of-button of-button--ghost" onClick={() => navigate(`/apps?selected=${encodeURIComponent(app.id)}`)}>
            <Glyph name="chevron-left" size={12} /> Back
          </button>
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, fontSize: 13, color: 'var(--text-muted)' }}>
            <Glyph name="folder" size={12} /> Workshop · <strong style={{ color: 'var(--text-strong)' }}>{app.name}</strong>
          </span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span className="of-text-muted" style={{ fontSize: 11 }}>{savedAt ? `Saved at ${savedAt.toLocaleTimeString()}` : 'Not saved'}</span>
          <button type="button" className="of-button" onClick={() => window.open(`/apps/${app.id}/workshop?mode=preview`, '_blank')}>
            <Glyph name="eye" size={12} /> View
          </button>
          <button
            type="button"
            onClick={() => void save()}
            disabled={saving}
            style={{ padding: '8px 14px', border: 0, borderRadius: 4, background: '#2d72d2', color: '#fff', fontSize: 13, fontWeight: 600, cursor: saving ? 'not-allowed' : 'pointer' }}
          >
            {saving ? 'Saving…' : 'Save and publish'}
          </button>
        </div>
      </header>

      <div style={{ display: 'grid', gridTemplateColumns: '56px 280px 1fr 320px', minHeight: 0 }}>
        <aside style={{ borderRight: '1px solid var(--border-subtle)', padding: '12px 4px', display: 'grid', gap: 4, alignContent: 'start', justifyContent: 'center' }}>
          {(['layout', 'outline', 'variables', 'settings'] as LeftTab[]).map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => setLeftTab(tab)}
              aria-label={tab}
              style={{
                width: 36, height: 36, border: 0, background: leftTab === tab ? 'rgba(45, 114, 210, 0.08)' : 'transparent',
                color: leftTab === tab ? 'var(--status-info)' : 'var(--text-muted)',
                borderRadius: 4, cursor: 'pointer',
              }}
            >
              <Glyph name={tab === 'layout' ? 'cube' : tab === 'outline' ? 'list' : tab === 'variables' ? 'tag' : 'settings'} size={16} />
            </button>
          ))}
        </aside>

        <aside style={{ borderRight: '1px solid var(--border-subtle)', overflowY: 'auto', padding: 14 }}>
          {leftTab === 'layout' ? (
            <LayoutOutline page={activePage} selection={selection} onSelect={setSelection} />
          ) : leftTab === 'outline' ? (
            <p className="of-text-muted" style={{ fontSize: 12 }}>Outline of the page DOM.</p>
          ) : leftTab === 'variables' ? (
            <VariablesPanel
              variables={variables}
              widgets={activePage.widgets}
              addMenuOpen={varAddMenuOpen}
              onToggleAdd={() => setVarAddMenuOpen((open) => !open)}
              onAdd={(variable) => {
                setVariables((current) => [...current, variable]);
                setEditingVariableId(variable.id);
                setVarAddMenuOpen(false);
              }}
              onRename={(variableId, name) => {
                setVariables((current) => current.map((v) => (v.id === variableId ? { ...v, name } : v)));
              }}
              onSelect={(variableId) => setEditingVariableId(variableId)}
              onDelete={(variableId) => {
                setVariables((current) => current.filter((v) => v.id !== variableId));
                if (editingVariableId === variableId) setEditingVariableId(null);
              }}
            />
          ) : (
            <p className="of-text-muted" style={{ fontSize: 12 }}>App settings panel.</p>
          )}
        </aside>

        <main style={{ overflow: 'auto', padding: 18, background: '#f4f6f9' }}>
          {headerUi.enable_module_header ? (
            <div
              onClick={(event) => {
                event.stopPropagation();
                setSelection({ kind: 'header', id: 'header' });
              }}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '10px 14px',
                background: '#fff',
                border: selection.kind === 'header' ? '2px solid var(--status-info)' : '1px solid var(--border-subtle)',
                borderRadius: 4,
                marginBottom: 12,
                cursor: 'pointer',
              }}
            >
              {headerUi.enable_app_logo ? (
                <span
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    width: 28,
                    height: 28,
                    borderRadius: 4,
                    background: `${headerSettings.color ?? '#2d72d2'}1a`,
                    color: headerSettings.color ?? '#2d72d2',
                  }}
                >
                  <Glyph name={(headerSettings.icon ?? 'cube') as GlyphName} size={16} tone={headerSettings.color ?? '#2d72d2'} />
                </span>
              ) : null}
              <span style={{ fontSize: 15, fontWeight: 600, color: 'var(--text-strong)', flex: 1 }}>
                {headerSettings.title || app.name}
              </span>
              {headerUi.enable_favoriting ? <Glyph name="star" size={14} tone="#cf923f" /> : null}
            </div>
          ) : null}
          <SectionToolbar
            label={selection.kind === 'section' || selection.kind === 'widget' ? 'OBJECT TABLE' : 'PAGE'}
            onAddSection={addSection}
            onSplit={(direction) => {
              const sectionId = selection.kind === 'section' ? selection.id : selection.kind === 'widget' ? activePage.widgets.find((s) => s.children.some((w) => w.id === selection.id))?.id ?? null : null;
              if (!sectionId) {
                addSection();
                return;
              }
              const newSection = makeSection();
              const index = activePage.widgets.findIndex((s) => s.id === sectionId);
              if (index < 0) return;
              const insertIndex = direction === 'right' || direction === 'below' ? index + 1 : index;
              const next = [...activePage.widgets];
              next.splice(insertIndex, 0, newSection);
              patchPage({ widgets: next });
              setSelection({ kind: 'section', id: newSection.id });
            }}
          />

          <div style={{ background: '#fff', border: '1px solid var(--border-subtle)', borderRadius: 6, padding: 14, display: 'grid', gridTemplateColumns: activePage.widgets.map((section) => `${flexValue(section)}fr`).join(' '), gap: 14, minHeight: 320 }}>
            {activePage.widgets.map((section) => (
              <div
                key={section.id}
                onClick={(event) => {
                  event.stopPropagation();
                  setSelection({ kind: 'section', id: section.id });
                }}
                style={{
                  display: 'grid',
                  gap: 10,
                  padding: 10,
                  border: selection.kind === 'section' && selection.id === section.id ? '2px solid var(--status-info)' : '1px solid var(--border-subtle)',
                  borderRadius: 6,
                  cursor: 'pointer',
                  alignContent: 'start',
                }}
              >
                <SectionHeaderRender section={section} />
                {section.children.map((widget) => (
                  <div
                    key={widget.id}
                    onClick={(event) => {
                      event.stopPropagation();
                      setSelection({ kind: 'widget', id: widget.id });
                    }}
                    style={{
                      border: selection.kind === 'widget' && selection.id === widget.id ? '2px solid var(--status-info)' : '1px solid var(--border-default)',
                      borderRadius: 4,
                      background: '#fff',
                      cursor: 'pointer',
                    }}
                  >
                    {widget.widget_type === 'object_table' ? (
                      <ObjectTableWidgetView widget={widget} variables={variables} />
                    ) : widget.widget_type === 'filter_list' ? (
                      <FilterListWidgetView widget={widget} />
                    ) : widget.widget_type === 'object_set_title' ? (
                      <ObjectSetTitleWidgetView widget={widget} variables={variables} objectTypes={objectTypes} />
                    ) : widget.widget_type === 'button_group' ? (
                      <ButtonGroupWidgetView widget={widget} />
                    ) : widget.widget_type === 'property_list' ? (
                      <PropertyListWidgetView widget={widget} variables={variables} />
                    ) : widget.widget_type === 'chart_pie' ? (
                      <ChartPieWidgetView widget={widget} variables={variables} />
                    ) : widget.widget_type === 'chart_xy' ? (
                      <ChartXyWidgetView widget={widget} variables={variables} />
                    ) : widget.widget_type === 'map' ? (
                      <MapWidgetView widget={widget} variables={variables} />
                    ) : (
                      <p className="of-text-muted" style={{ padding: 12, margin: 0, fontSize: 12 }}>{widget.widget_type}</p>
                    )}
                  </div>
                ))}
                <div style={{ position: 'relative' }}>
                  <button
                    type="button"
                    className="of-button"
                    onClick={() => setWidgetMenuSection(widgetMenuSection === section.id ? null : section.id)}
                    style={{ width: '100%', justifyContent: 'center', fontSize: 13 }}
                  >
                    <Glyph name="plus" size={13} /> Add widget
                  </button>
                  {widgetMenuSection === section.id ? (
                    <div role="menu" style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 4, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.12)', padding: 4, zIndex: 5 }}>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makeObjectTableWidget();
                          patchSection(section.id, (s) => ({ ...s, children: [...s.children, widget] }));
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                          setPickerOpen({ widgetId: widget.id });
                          const activeId = makeId('var');
                          setVariables((current) => [
                            ...current,
                            {
                              id: activeId,
                              kind: 'object_set_active_object',
                              name: `${widget.title} Active object`,
                              object_type_id: '',
                              source_widget_id: widget.id,
                            },
                          ]);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <Glyph name="list" size={13} tone="#2d72d2" /> Object table
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makeObjectSetTitleWidget();
                          patchSection(section.id, (s) => ({ ...s, children: [...s.children, widget] }));
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <Glyph name="cube" size={13} tone="#2d72d2" /> Object Set Title
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makeButtonGroupWidget();
                          patchSection(section.id, (s) => ({ ...s, children: [...s.children, widget] }));
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <Glyph name="run" size={13} tone="#15803d" /> Button group
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makePropertyListWidget();
                          patchSection(section.id, (s) => ({ ...s, children: [...s.children, widget] }));
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <Glyph name="list" size={13} tone="#cf923f" /> Property list
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makeChartPieWidget();
                          patchSection(section.id, (s) => ({ ...s, children: [...s.children, widget] }));
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <Glyph name="pie-chart" size={13} tone="#cf923f" /> Chart: Pie
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makeChartXyWidget();
                          patchSection(section.id, (s) => ({ ...s, children: [...s.children, widget] }));
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <span style={{ display: 'inline-flex' }}>
                          <ChartXyGlyph />
                        </span>
                        Chart: XY
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makeMapWidget();
                          const activeId = makeId('var');
                          const selectedSetId = makeId('var');
                          const shapeId = makeId('var');
                          const shapeSearchId = makeId('var');
                          patchSection(section.id, (s) => ({
                            ...s,
                            children: [...s.children, { ...widget, props: { ...widget.props, output_variable_id: activeId, output_object_set_variable_id: selectedSetId, output_shape_variable_id: shapeId, shape_search_output_variable_id: shapeSearchId } }],
                          }));
                          setVariables((current) => [
                            ...current,
                            {
                              id: activeId,
                              kind: 'object_set_active_object',
                              name: `${widget.title} Selected object`,
                              object_type_id: '',
                              source_widget_id: widget.id,
                            },
                            {
                              id: selectedSetId,
                              kind: 'object_set_selection',
                              name: `${widget.title} Selected object set`,
                              object_type_id: '',
                              source_widget_id: widget.id,
                            },
                            {
                              id: shapeId,
                              kind: 'shape_output',
                              name: `${widget.title} Drawn shape`,
                              object_type_id: '',
                              source_widget_id: widget.id,
                            },
                            {
                              id: shapeSearchId,
                              kind: 'object_set_selection',
                              name: `${widget.title} Shape search results`,
                              object_type_id: '',
                              source_widget_id: widget.id,
                            },
                          ]);
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <Glyph name="graph" size={13} tone="#15803d" /> Map
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          const widget = makeFilterListWidget();
                          patchSection(section.id, (s) => ({ ...s, children: [...s.children, widget] }));
                          setSelection({ kind: 'widget', id: widget.id });
                          setWidgetMenuSection(null);
                          const variableId = makeId('var');
                          setVariables((current) => [
                            ...current,
                            {
                              id: variableId,
                              kind: 'filter_output',
                              name: `${widget.title} Filter output`,
                              object_type_id: '',
                              source_widget_id: widget.id,
                            },
                          ]);
                          patchSection(section.id, (s) => ({
                            ...s,
                            children: s.children.map((c) => (c.id === widget.id ? { ...c, props: { ...c.props, output_variable_id: variableId } } : c)),
                          }));
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <span style={{ display: 'inline-flex' }}>
                          <FilterListGlyph />
                        </span>
                        Filter list
                      </button>
                    </div>
                  ) : null}
                </div>
                <div style={{ position: 'relative' }}>
                  <button
                    type="button"
                    className="of-button"
                    onClick={() => setLayoutMenuSection(layoutMenuSection === section.id ? null : section.id)}
                    style={{ width: '100%', justifyContent: 'center', fontSize: 13 }}
                  >
                    <Glyph name="view-grid" size={13} /> Set layout
                  </button>
                  {layoutMenuSection === section.id ? (
                    <div role="menu" style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 6, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.12)', padding: 12, zIndex: 6 }}>
                      <p style={{ margin: '0 0 4px', fontSize: 12, fontWeight: 600 }}>Layout</p>
                      <p className="of-text-muted" style={{ margin: '0 0 10px', fontSize: 11 }}>Determines how components will be arranged in this section</p>
                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 6 }}>
                        {([
                          { id: 'columns', label: 'Columns' },
                          { id: 'rows', label: 'Rows' },
                          { id: 'tabs', label: 'Tabs' },
                          { id: 'flow', label: 'Flow' },
                          { id: 'toolbar', label: 'Toolbar' },
                          { id: 'loop', label: 'Loop' },
                        ] as const).map((option) => {
                          const current = ((section.props as { layout_kind?: string })?.layout_kind) ?? 'columns';
                          return (
                            <button
                              key={option.id}
                              type="button"
                              onClick={() => {
                                patchSection(section.id, (s) => ({ ...s, props: { ...s.props, layout_kind: option.id, layout_direction: option.id === 'rows' ? 'rows' : 'columns' } }));
                                setLayoutMenuSection(null);
                              }}
                              style={{ display: 'grid', gap: 4, padding: '10px 6px', border: current === option.id ? '2px solid var(--status-info)' : '1px solid var(--border-subtle)', background: current === option.id ? 'rgba(45, 114, 210, 0.04)' : '#fff', borderRadius: 4, cursor: 'pointer', fontSize: 11 }}
                            >
                              <LayoutPreviewGlyph kind={option.id} />
                              {option.label}
                            </button>
                          );
                        })}
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        </main>

        <aside style={{ borderLeft: '1px solid var(--border-subtle)', overflowY: 'auto' }}>
          {selection.kind === 'header' ? (
            <HeaderInspector
              header={headerSettings}
              ui={headerUi}
              onHeaderChange={setHeaderSettings}
              onUiChange={setHeaderUi}
            />
          ) : selectedWidget && (selectedWidget.widget.widget_type === 'object_set_title' || selectedWidget.widget.widget_type === 'button_group' || selectedWidget.widget.widget_type === 'property_list') ? (
            <DetailWidgetInspector
              widget={selectedWidget.widget}
              variables={variables}
              onChange={(next) => patchWidget(selectedWidget.section.id, selectedWidget.widget.id, () => next)}
              onDelete={() => {
                removeWidget(selectedWidget.section.id, selectedWidget.widget.id);
                setSelection({ kind: 'page', id: activePage.id });
              }}
            />
          ) : selectedWidget && selectedWidget.widget.widget_type === 'chart_xy' ? (
            <ChartXyInspector
              widget={selectedWidget.widget}
              variables={variables}
              objectTypes={objectTypes}
              onChange={(next) => patchWidget(selectedWidget.section.id, selectedWidget.widget.id, () => next)}
              onDelete={() => {
                removeWidget(selectedWidget.section.id, selectedWidget.widget.id);
                setSelection({ kind: 'page', id: activePage.id });
              }}
            />
          ) : selectedWidget && selectedWidget.widget.widget_type === 'chart_pie' ? (
            <ChartPieInspector
              widget={selectedWidget.widget}
              variables={variables}
              objectTypes={objectTypes}
              onChange={(next) => patchWidget(selectedWidget.section.id, selectedWidget.widget.id, () => next)}
              onDelete={() => {
                removeWidget(selectedWidget.section.id, selectedWidget.widget.id);
                setSelection({ kind: 'page', id: activePage.id });
              }}
            />
          ) : selectedWidget && selectedWidget.widget.widget_type === 'map' ? (
            <MapWidgetInspector
              widget={selectedWidget.widget}
              variables={variables}
              objectTypes={objectTypes}
              onChange={(next) => patchWidget(selectedWidget.section.id, selectedWidget.widget.id, () => next)}
              onRenameOutput={(name, objectTypeId) => {
                const outputId = (selectedWidget.widget.props as { output_variable_id?: string })?.output_variable_id;
                const outputSetId = (selectedWidget.widget.props as { output_object_set_variable_id?: string })?.output_object_set_variable_id;
                const shapeSearchOutputId = (selectedWidget.widget.props as { shape_search_output_variable_id?: string })?.shape_search_output_variable_id;
                if (outputId || outputSetId || shapeSearchOutputId) {
                  setVariables((current) => current.map((v) => {
                    if (v.id === outputId) return { ...v, name, object_type_id: objectTypeId || v.object_type_id };
                    if (v.id === outputSetId) return { ...v, name: `${selectedWidget.widget.title} Selected object set`, object_type_id: objectTypeId || v.object_type_id };
                    if (v.id === shapeSearchOutputId) return { ...v, name: `${selectedWidget.widget.title} Shape search results`, object_type_id: objectTypeId || v.object_type_id };
                    return v;
                  }));
                }
              }}
              onDelete={() => {
                removeWidget(selectedWidget.section.id, selectedWidget.widget.id);
                setSelection({ kind: 'page', id: activePage.id });
              }}
            />
          ) : selectedWidget && selectedWidget.widget.widget_type === 'filter_list' ? (
            <FilterListInspector
              widget={selectedWidget.widget}
              objectTypes={objectTypes}
              variables={variables}
              onChange={(next) => patchWidget(selectedWidget.section.id, selectedWidget.widget.id, () => next)}
              onRenameOutput={(name) => {
                const outputId = (selectedWidget.widget.props as { output_variable_id?: string })?.output_variable_id;
                if (outputId) {
                  setVariables((current) => current.map((v) => (v.id === outputId ? { ...v, name } : v)));
                }
              }}
              outputName={
                variables.find((v) => v.id === ((selectedWidget.widget.props as { output_variable_id?: string })?.output_variable_id))?.name ??
                `${selectedWidget.widget.title} Filter output`
              }
              onDelete={() => {
                removeWidget(selectedWidget.section.id, selectedWidget.widget.id);
                setSelection({ kind: 'page', id: activePage.id });
              }}
            />
          ) : selectedWidget ? (
            <WidgetInspector
              widget={selectedWidget.widget}
              section={selectedWidget.section}
              objectTypes={objectTypes}
              variables={variables}
              onChange={(next) => patchWidget(selectedWidget.section.id, selectedWidget.widget.id, () => next)}
              onDelete={() => {
                removeWidget(selectedWidget.section.id, selectedWidget.widget.id);
                setSelection({ kind: 'page', id: activePage.id });
              }}
            />
          ) : selectedSection ? (
            <SectionInspector
              section={selectedSection}
              onChange={(next) => patchSection(selectedSection.id, () => next)}
            />
          ) : (
            <PageInspector page={activePage} onChange={(next) => patchPage(next)} />
          )}
        </aside>
      </div>

      {editingVariableId ? (
        <ObjectSetDefinitionEditor
          variables={variables}
          objectTypes={objectTypes}
          variable={variables.find((v) => v.id === editingVariableId) ?? null}
          onClose={() => setEditingVariableId(null)}
          onChange={(next) => setVariables((current) => current.map((v) => (v.id === next.id ? next : v)))}
        />
      ) : null}

      {pickerOpen ? (
        <ObjectSetPicker
          objectTypes={objectTypes}
          onClose={() => setPickerOpen(null)}
          onSelect={(typeId) => {
            const target = activePage.widgets.flatMap((s) => s.children.map((w) => ({ s, w }))).find((x) => x.w.id === pickerOpen.widgetId);
            if (target) {
              patchWidget(target.s.id, target.w.id, (widget) => ({
                ...widget,
                title: `Object table 1`,
                props: { ...widget.props, object_type_id: typeId },
                binding: { source_type: 'ontology_object_type', source_id: typeId, fields: [], parameters: {} },
              }));
            }
            setPickerOpen(null);
          }}
        />
      ) : null}

      {error ? (
        <div role="alert" style={{ position: 'absolute', bottom: 12, left: '50%', transform: 'translateX(-50%)', padding: '8px 14px', background: 'rgba(180, 35, 24, 0.92)', color: '#fff', borderRadius: 4, fontSize: 12 }}>
          {error}
        </div>
      ) : null}
    </div>
  );
}

function flexValue(section: AppWidget): number {
  const width = (section.props as { column_width?: number })?.column_width;
  if (typeof width === 'number') return Math.max(1, width);
  return 1;
}

function LayoutOutline({
  page,
  selection,
  onSelect,
}: {
  page: AppPage;
  selection: SelectionState;
  onSelect: (selection: SelectionState) => void;
}) {
  return (
    <div style={{ display: 'grid', gap: 6 }}>
      <p style={{ margin: 0, fontSize: 11, fontWeight: 700, letterSpacing: '0.06em', color: 'var(--text-muted)' }}>LAYOUT</p>
      <button
        type="button"
        onClick={() => onSelect({ kind: 'header', id: 'header' })}
        style={outlineRow(selection.kind === 'header')}
      >
        <Glyph name="object" size={12} tone="#5c7080" />
        Header
      </button>
      <button
        type="button"
        onClick={() => onSelect({ kind: 'page', id: page.id })}
        style={outlineRow(selection.kind === 'page' && selection.id === page.id)}
      >
        <Glyph name="document" size={12} tone="#5c7080" />
        Page <span className="of-text-muted" style={{ fontSize: 11 }}>(DEFAULT)</span>
      </button>
      {page.widgets.map((section) => (
        <div key={section.id}>
          <button
            type="button"
            onClick={() => onSelect({ kind: 'section', id: section.id })}
            style={{ ...outlineRow(selection.kind === 'section' && selection.id === section.id), paddingLeft: 22 }}
          >
            <Glyph name="chevron-down" size={11} />
            <Glyph name="cube" size={12} tone="#5c7080" />
            {section.title}
            <span className="of-text-muted" style={{ marginLeft: 'auto', fontSize: 10 }}>ROWS</span>
          </button>
          {section.children.map((widget) => (
            <button
              key={widget.id}
              type="button"
              onClick={() => onSelect({ kind: 'widget', id: widget.id })}
              style={{ ...outlineRow(selection.kind === 'widget' && selection.id === widget.id), paddingLeft: 42 }}
            >
              <Glyph name="list" size={12} tone="#2d72d2" />
              {widget.title}
            </button>
          ))}
        </div>
      ))}
    </div>
  );
}

function outlineRow(active: boolean): React.CSSProperties {
  return {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    width: '100%',
    padding: '6px 10px',
    border: 0,
    background: active ? 'rgba(45, 114, 210, 0.08)' : 'transparent',
    color: active ? 'var(--status-info)' : 'var(--text-strong)',
    fontWeight: active ? 600 : 500,
    fontSize: 13,
    borderRadius: 4,
    cursor: 'pointer',
    textAlign: 'left',
  };
}

function HeaderInspector({
  header,
  ui,
  onHeaderChange,
  onUiChange,
}: {
  header: WorkshopHeaderSettings;
  ui: HeaderUiState;
  onHeaderChange: (next: WorkshopHeaderSettings) => void;
  onUiChange: (next: HeaderUiState) => void;
}) {
  const [iconQuery, setIconQuery] = useState('');
  const filteredIcons = HEADER_ICON_OPTIONS.filter((option) =>
    `${option.label} ${option.id}`.toLowerCase().includes(iconQuery.toLowerCase()),
  );
  const selectedColor = colorByHex(header.color);
  return (
    <div style={inspectorStyle()}>
      <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>Header</span>
      </div>
      <div style={{ padding: 14, display: 'grid', gap: 14 }}>
        <Toggle
          label="Enable module header"
          value={ui.enable_module_header}
          onChange={(checked) => onUiChange({ ...ui, enable_module_header: checked })}
        />

        <Section title="Header configuration" />
        <Field label="Title">
          <input
            value={header.title ?? ''}
            onChange={(event) => onHeaderChange({ ...header, title: event.target.value })}
            placeholder="Workshop title"
            style={inputStyle()}
            disabled={!ui.enable_module_header}
          />
        </Field>

        <Toggle
          label="Custom color"
          value={ui.custom_color}
          onChange={(checked) => onUiChange({ ...ui, custom_color: checked })}
        />

        <Toggle
          label="Enable app logo"
          value={ui.enable_app_logo}
          onChange={(checked) => onUiChange({ ...ui, enable_app_logo: checked })}
        />

        {ui.enable_app_logo ? (
          <>
            <div style={{ display: 'inline-flex', borderRadius: 4, overflow: 'hidden', border: '1px solid var(--border-default)' }}>
              {(['icon', 'image'] as const).map((kind) => (
                <button
                  key={kind}
                  type="button"
                  onClick={() => onUiChange({ ...ui, logo_kind: kind })}
                  style={{
                    flex: 1,
                    padding: '6px 14px',
                    border: 0,
                    background: ui.logo_kind === kind ? '#1c2127' : '#fff',
                    color: ui.logo_kind === kind ? '#fff' : 'var(--text-strong)',
                    cursor: 'pointer',
                    fontSize: 12,
                  }}
                >
                  {kind === 'icon' ? 'Icon' : 'Image'}
                </button>
              ))}
            </div>

            {ui.logo_kind === 'icon' ? (
              <Field label="Icon">
                <div style={{ display: 'grid', gap: 6 }}>
                  <span
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      padding: '6px 10px',
                      border: '1px solid var(--border-default)',
                      borderRadius: 4,
                      background: '#fff',
                    }}
                  >
                    <Glyph name={(header.icon ?? 'cube') as GlyphName} size={14} tone={header.color ?? '#2d72d2'} />
                    <input
                      value={iconQuery}
                      onChange={(event) => setIconQuery(event.target.value)}
                      placeholder={HEADER_ICON_OPTIONS.find((option) => option.id === (header.icon as GlyphName))?.label ?? 'Cube'}
                      style={{ flex: 1, border: 0, outline: 'none', fontSize: 13 }}
                    />
                    {header.icon ? (
                      <button
                        type="button"
                        aria-label="Clear icon"
                        onClick={() => onHeaderChange({ ...header, icon: null })}
                        style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--text-muted)', padding: 2 }}
                      >
                        <Glyph name="x" size={11} />
                      </button>
                    ) : null}
                  </span>
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: 'repeat(5, 1fr)',
                      gap: 4,
                      maxHeight: 160,
                      overflowY: 'auto',
                      border: '1px solid var(--border-subtle)',
                      borderRadius: 4,
                      padding: 6,
                    }}
                  >
                    {filteredIcons.length === 0 ? (
                      <p className="of-text-muted" style={{ gridColumn: '1 / -1', fontSize: 12, padding: 8, textAlign: 'center', margin: 0 }}>
                        No icons match "{iconQuery}".
                      </p>
                    ) : (
                      filteredIcons.map((option) => {
                        const active = header.icon === option.id;
                        return (
                          <button
                            key={option.id}
                            type="button"
                            title={option.label}
                            aria-label={option.label}
                            onClick={() => onHeaderChange({ ...header, icon: option.id })}
                            style={{
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              padding: 8,
                              border: active ? '2px solid var(--status-info)' : '1px solid var(--border-subtle)',
                              borderRadius: 4,
                              background: active ? 'rgba(45, 114, 210, 0.06)' : '#fff',
                              cursor: 'pointer',
                            }}
                          >
                            <Glyph name={option.id} size={14} />
                          </button>
                        );
                      })
                    )}
                  </div>
                </div>
              </Field>
            ) : (
              <Field label="Image URL">
                <input
                  value={ui.image_url}
                  onChange={(event) => onUiChange({ ...ui, image_url: event.target.value })}
                  placeholder="https://example.com/logo.png"
                  style={inputStyle()}
                />
              </Field>
            )}
          </>
        ) : null}

        <Toggle
          label="Color"
          value={Boolean(header.color)}
          onChange={(checked) => onHeaderChange({ ...header, color: checked ? (header.color ?? '#2d72d2') : null })}
        />

        {header.color ? (
          <Field label="">
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 4 }}>
              {HEADER_COLORS.map((option) => {
                const active = selectedColor?.id === option.id;
                return (
                  <button
                    key={option.id}
                    type="button"
                    title={option.label}
                    onClick={() => onHeaderChange({ ...header, color: option.hex })}
                    style={{
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      padding: 8,
                      border: active ? '2px solid var(--status-info)' : '1px solid var(--border-subtle)',
                      borderRadius: 4,
                      background: '#fff',
                      cursor: 'pointer',
                    }}
                  >
                    <span style={{ width: 18, height: 18, borderRadius: 4, background: option.hex, border: '1px solid rgba(0,0,0,0.08)' }} />
                  </button>
                );
              })}
            </div>
            <span className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>
              {selectedColor ? selectedColor.label : header.color}
            </span>
          </Field>
        ) : null}

        <Toggle
          label="Enable favoriting in view mode"
          value={ui.enable_favoriting}
          onChange={(checked) => onUiChange({ ...ui, enable_favoriting: checked })}
        />
      </div>
    </div>
  );
}

function Toggle({
  label,
  value,
  onChange,
}: {
  label: string;
  value: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8, fontSize: 13, color: 'var(--text-strong)' }}>
      <span style={{ fontSize: 12, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.04em', fontWeight: 600 }}>{label}</span>
      <span
        onClick={() => onChange(!value)}
        role="switch"
        aria-checked={value}
        tabIndex={0}
        style={{
          display: 'inline-flex',
          width: 32,
          height: 18,
          borderRadius: 999,
          background: value ? 'var(--status-info)' : '#c5cdd9',
          padding: 2,
          cursor: 'pointer',
          transition: 'background 120ms',
        }}
      >
        <span
          style={{
            width: 14,
            height: 14,
            borderRadius: '50%',
            background: '#fff',
            transform: value ? 'translateX(14px)' : 'translateX(0)',
            transition: 'transform 120ms',
            boxShadow: '0 1px 2px rgba(15, 23, 42, 0.16)',
          }}
        />
      </span>
    </label>
  );
}

function PageInspector({ page, onChange }: { page: AppPage; onChange: (patch: Partial<AppPage>) => void }) {
  return (
    <div style={inspectorStyle()}>
      <Section title="Page" />
      <Field label="Page name">
        <input value={page.name} onChange={(event) => onChange({ name: event.target.value })} style={inputStyle()} />
      </Field>
      <Field label="Page id (optional)">
        <input value={page.id} onChange={(event) => onChange({ id: event.target.value })} style={inputStyle()} />
      </Field>
      <Section title="Layout" />
      <Field label="Layout direction">
        <div style={{ display: 'inline-flex', borderRadius: 4, overflow: 'hidden', border: '1px solid var(--border-default)' }}>
          {(['flex', 'grid'] as const).map((kind) => (
            <button key={kind} type="button" onClick={() => onChange({ layout: { ...page.layout, kind } })} style={{ padding: '6px 14px', border: 0, background: page.layout.kind === kind ? '#1c2127' : '#fff', color: page.layout.kind === kind ? '#fff' : 'var(--text-strong)', cursor: 'pointer', fontSize: 12 }}>{kind === 'flex' ? 'Columns' : 'Rows'}</button>
          ))}
        </div>
      </Field>
    </div>
  );
}

function SectionInspector({ section, onChange }: { section: AppWidget; onChange: (next: AppWidget) => void }) {
  const widthKind = ((section.props as { column_width_kind?: string })?.column_width_kind) ?? 'flex';
  const widthValue = ((section.props as { column_width?: number })?.column_width) ?? 1;
  const headerEnabled = (section.props as { header_enabled?: boolean })?.header_enabled !== false;
  const collapsible = Boolean((section.props as { collapsible?: boolean })?.collapsible);
  const initiallyOpen = (section.props as { initially_open?: boolean })?.initially_open !== false;
  const iconExpand = (section.props as { icon_expand?: string })?.icon_expand ?? 'menu-closed';
  const iconCollapse = (section.props as { icon_collapse?: string })?.icon_collapse ?? 'menu-open';
  const iconName = (section.props as { icon?: string })?.icon ?? '';
  const headerFormat = (section.props as { header_format?: string })?.header_format ?? 'title';
  const backgroundColor = (section.props as { background_color?: string })?.background_color ?? 'white';
  const [iconQuery, setIconQuery] = useState('');
  const filteredIcons = HEADER_ICON_OPTIONS.filter((entry) => `${entry.label} ${entry.id}`.toLowerCase().includes(iconQuery.toLowerCase()));

  function patchProps(patch: Record<string, unknown>) {
    onChange({ ...section, props: { ...section.props, ...patch } });
  }

  return (
    <div style={inspectorStyle()}>
      <Section title="Section" />
      <Toggle label="Section header" value={headerEnabled} onChange={(checked) => patchProps({ header_enabled: checked })} />
      {headerEnabled ? (
        <>
          <Field label="Style">
            <select
              value={(section.props as { style?: string })?.style ?? 'subheader'}
              onChange={(event) => patchProps({ style: event.target.value })}
              style={inputStyle()}
            >
              <option value="header">Header</option>
              <option value="title">Title</option>
              <option value="subheader">Subheader</option>
              <option value="caption">Caption</option>
            </select>
          </Field>
          <Field label="Title">
            <input value={section.title} onChange={(event) => onChange({ ...section, title: event.target.value })} style={inputStyle()} />
          </Field>
          <Field label="Icon">
            <span style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff' }}>
              {iconName ? <Glyph name={iconName as GlyphName} size={13} tone="#5c7080" /> : <Glyph name="search" size={12} tone="#aab4c0" />}
              <input value={iconQuery} onChange={(event) => setIconQuery(event.target.value)} placeholder={HEADER_ICON_OPTIONS.find((option) => option.id === iconName)?.label ?? 'Select an icon…'} style={{ flex: 1, border: 0, outline: 'none', fontSize: 13 }} />
              {iconName ? (
                <button type="button" aria-label="Clear icon" onClick={() => patchProps({ icon: '' })} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--text-muted)' }}>
                  <Glyph name="x" size={11} />
                </button>
              ) : null}
            </span>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 4, marginTop: 6, maxHeight: 130, overflowY: 'auto' }}>
              {filteredIcons.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  title={option.label}
                  onClick={() => patchProps({ icon: option.id })}
                  style={{ padding: 6, border: iconName === option.id ? '2px solid var(--status-info)' : '1px solid var(--border-subtle)', background: '#fff', borderRadius: 4, cursor: 'pointer' }}
                >
                  <Glyph name={option.id} size={13} />
                </button>
              ))}
            </div>
          </Field>
        </>
      ) : null}

      <Toggle label="Collapsible" value={collapsible} onChange={(checked) => patchProps({ collapsible: checked })} />
      {collapsible ? (
        <>
          <Field label="Section is initially">
            <select value={initiallyOpen ? 'open' : 'closed'} onChange={(event) => patchProps({ initially_open: event.target.value === 'open' })} style={inputStyle()}>
              <option value="open">open</option>
              <option value="closed">closed</option>
            </select>
          </Field>
          <Field label="Icon to expand">
            <select value={iconExpand} onChange={(event) => patchProps({ icon_expand: event.target.value })} style={inputStyle()}>
              <option value="menu-closed">Menu closed</option>
              <option value="chevron-down">Chevron down</option>
              <option value="plus">Plus</option>
            </select>
          </Field>
          <Field label="Icon to collapse">
            <select value={iconCollapse} onChange={(event) => patchProps({ icon_collapse: event.target.value })} style={inputStyle()}>
              <option value="menu-open">Menu open</option>
              <option value="chevron-down">Chevron down</option>
              <option value="x">X</option>
            </select>
          </Field>
        </>
      ) : null}

      <Section title="Formatting" />
      <Field label="Header format">
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 6 }}>
          {SECTION_HEADER_FORMATS.map((option) => (
            <button
              key={option.id}
              type="button"
              onClick={() => patchProps({ header_format: option.id })}
              style={{ padding: '8px 4px', border: headerFormat === option.id ? '2px solid var(--status-info)' : '1px solid var(--border-default)', background: headerFormat === option.id ? 'rgba(45, 114, 210, 0.06)' : '#fff', borderRadius: 4, cursor: 'pointer', fontSize: 12 }}
            >
              {option.label}
            </button>
          ))}
        </div>
      </Field>
      <Field label="Background color">
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 4 }}>
          {SECTION_BG_COLORS.map((option) => (
            <button
              key={option.id}
              type="button"
              title={option.label}
              onClick={() => patchProps({ background_color: option.id })}
              style={{ padding: 4, border: backgroundColor === option.id ? '2px solid var(--status-info)' : '1px solid var(--border-subtle)', background: '#fff', borderRadius: 4, cursor: 'pointer' }}
            >
              <span style={{ display: 'block', height: 18, background: option.hex, borderRadius: 2, border: '1px solid rgba(0,0,0,0.06)' }} />
            </button>
          ))}
        </div>
      </Field>

      <Section title="Dimensions" />
      <Field label="Column width">
        <div style={{ display: 'flex', gap: 0, alignItems: 'center' }}>
          {(['absolute', 'flex'] as const).map((kind) => (
            <button
              key={kind}
              type="button"
              onClick={() => patchProps({ column_width_kind: kind })}
              style={{ padding: '6px 14px', border: '1px solid var(--border-default)', background: widthKind === kind ? '#1c2127' : '#fff', color: widthKind === kind ? '#fff' : 'var(--text-strong)', cursor: 'pointer', fontSize: 12 }}
            >
              {kind === 'absolute' ? 'Absolute' : 'Flex'}
            </button>
          ))}
          <input
            type="number"
            min={1}
            value={widthValue}
            onChange={(event) => patchProps({ column_width: Number(event.target.value) })}
            style={{ ...inputStyle(), width: 80, marginLeft: 8 }}
          />
        </div>
      </Field>
      <Field label="Row height">
        <div style={{ display: 'flex', gap: 0, alignItems: 'center' }}>
          {(['auto', 'absolute', 'flex'] as const).map((kind) => {
            const current = ((section.props as { row_height_kind?: string })?.row_height_kind) ?? 'auto';
            return (
              <button
                key={kind}
                type="button"
                onClick={() => patchProps({ row_height_kind: kind })}
                style={{ padding: '6px 12px', border: '1px solid var(--border-default)', background: current === kind ? '#1c2127' : '#fff', color: current === kind ? '#fff' : 'var(--text-strong)', cursor: 'pointer', fontSize: 12 }}
              >
                {kind === 'auto' ? 'Auto (max)' : kind === 'absolute' ? 'Absolute' : 'Flex'}
              </button>
            );
          })}
          <input
            type="number"
            min={1}
            value={Number((section.props as { row_height_value?: number })?.row_height_value ?? 1)}
            onChange={(event) => patchProps({ row_height_value: Number(event.target.value) })}
            style={{ ...inputStyle(), width: 80, marginLeft: 8 }}
          />
        </div>
      </Field>

      <Section title="Layout" />
      <Field label="Padding controls">
        <select
          value={((section.props as { padding_controls?: string })?.padding_controls) ?? 'no-padding'}
          onChange={(event) => patchProps({ padding_controls: event.target.value })}
          style={inputStyle()}
        >
          <option value="no-padding">No padding</option>
          <option value="regular">Regular</option>
          <option value="custom">Custom</option>
        </select>
      </Field>
      <Field label="Layout direction">
        <div style={{ display: 'inline-flex', borderRadius: 4, overflow: 'hidden', border: '1px solid var(--border-default)' }}>
          {(['columns', 'rows'] as const).map((kind) => {
            const current = ((section.props as { layout_direction?: string })?.layout_direction) ?? 'columns';
            return (
              <button key={kind} type="button" onClick={() => patchProps({ layout_direction: kind })} style={{ padding: '6px 14px', border: 0, background: current === kind ? '#1c2127' : '#fff', color: current === kind ? '#fff' : 'var(--text-strong)', cursor: 'pointer', fontSize: 12 }}>
                {kind === 'columns' ? 'Columns' : 'Rows'}
              </button>
            );
          })}
        </div>
      </Field>
    </div>
  );
}

function WidgetInspector({
  widget,
  objectTypes,
  variables,
  onChange,
  onDelete,
}: {
  widget: AppWidget;
  section: AppWidget;
  objectTypes: ObjectType[];
  variables: WorkshopVariable[];
  onChange: (next: AppWidget) => void;
  onDelete: () => void;
}) {
  const [tab, setTab] = useState<'setup' | 'metadata' | 'display'>('setup');
  const [properties, setProperties] = useState<Property[]>([]);
  const sourceVariableId = (widget.props as { source_variable_id?: string })?.source_variable_id ?? '';
  const sourceVariable = variables.find((v) => v.id === sourceVariableId) ?? null;
  const objectTypeId = sourceVariable?.object_type_id ?? (widget.props as { object_type_id?: string })?.object_type_id ?? '';
  const columns: string[] = ((widget.props as { columns?: string[] })?.columns) ?? [];
  const sortProperty = (widget.props as { default_sort_property?: string })?.default_sort_property ?? '';

  useEffect(() => {
    if (!objectTypeId) {
      setProperties([]);
      return;
    }
    let cancelled = false;
    void listProperties(objectTypeId)
      .then((response) => {
        if (cancelled) return;
        setProperties(response);
      })
      .catch(() => {
        if (!cancelled) setProperties([]);
      });
    return () => {
      cancelled = true;
    };
  }, [objectTypeId]);

  const objectType = objectTypes.find((entry) => entry.id === objectTypeId);

  function patchProps(patch: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patch } });
  }

  function toggleColumn(name: string) {
    const next = columns.includes(name) ? columns.filter((entry) => entry !== name) : [...columns, name];
    patchProps({ columns: next });
  }

  function addAllProperties() {
    patchProps({ columns: properties.map((property) => property.name) });
  }

  function removeAllProperties() {
    patchProps({ columns: [] });
  }

  function moveColumn(name: string, direction: -1 | 1) {
    const index = columns.indexOf(name);
    if (index === -1) return;
    const next = [...columns];
    const swap = index + direction;
    if (swap < 0 || swap >= next.length) return;
    [next[index], next[swap]] = [next[swap], next[index]];
    patchProps({ columns: next });
  }

  function reorderColumns(from: number, to: number) {
    if (from === to || from < 0 || to < 0 || from >= columns.length || to >= columns.length) return;
    const next = [...columns];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    patchProps({ columns: next });
  }

  return (
    <div style={inspectorStyle()}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>{widget.title}</span>
        <span className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.06em' }}>OBJECT TABLE</span>
      </div>
      <div style={{ display: 'flex', gap: 0, padding: '0 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        {(['setup', 'metadata', 'display'] as const).map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => setTab(value)}
            style={{ padding: '8px 6px', border: 0, background: 'transparent', borderBottom: tab === value ? '2px solid var(--status-info)' : '2px solid transparent', cursor: 'pointer', fontSize: 12, fontWeight: tab === value ? 600 : 500, color: tab === value ? 'var(--text-strong)' : 'var(--text-muted)', marginRight: 14 }}
          >
            {value === 'setup' ? 'Widget setup' : value === 'metadata' ? 'Metadata' : 'Display'}
          </button>
        ))}
      </div>
      {tab === 'setup' ? (
        <div style={{ padding: 14, display: 'grid', gap: 14 }}>
          <Section title="Input data" />
          <Field label="Object set">
            <select
              value={sourceVariableId ? `var:${sourceVariableId}` : objectTypeId ? `type:${objectTypeId}` : ''}
              onChange={(event) => {
                const raw = event.target.value;
                if (raw.startsWith('var:')) {
                  patchProps({ source_variable_id: raw.slice(4), object_type_id: '', columns: [], default_sort_property: '' });
                } else if (raw.startsWith('type:')) {
                  patchProps({ source_variable_id: '', object_type_id: raw.slice(5), columns: [], default_sort_property: '' });
                } else {
                  patchProps({ source_variable_id: '', object_type_id: '', columns: [], default_sort_property: '' });
                }
              }}
              style={inputStyle()}
            >
              <option value="">Select object set variable…</option>
              {variables
                .filter((v) => v.kind === 'object_set' || v.kind === 'object_set_definition' || v.kind === 'object_set_selection')
                .map((variable) => (
                  <option key={variable.id} value={`var:${variable.id}`}>
                    {variable.name} ({VARIABLE_KIND_LABEL[variable.kind]})
                  </option>
                ))}
              {objectTypes.map((type) => (
                <option key={type.id} value={`type:${type.id}`}>{type.display_name || type.name}</option>
              ))}
            </select>
            <span className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>Current value: {sourceVariable ? sourceVariable.name : objectType ? objectType.display_name || objectType.name : 'undefined'}</span>
          </Field>

          <Section title="Column configuration" />
          {properties.length === 0 ? (
            <p className="of-text-muted" style={{ fontSize: 12 }}>Pick an object set to configure columns.</p>
          ) : (
            <ColumnConfiguration
              objectType={objectType ?? null}
              properties={properties}
              columns={columns}
              onToggle={toggleColumn}
              onMove={moveColumn}
              onReorder={reorderColumns}
              onAddAll={addAllProperties}
              onRemoveAll={removeAllProperties}
            />
          )}

          <Section title="Default sort" />
          <Field label="Property">
            <select value={sortProperty} onChange={(event) => patchProps({ default_sort_property: event.target.value })} style={inputStyle()}>
              <option value="">Select a property to sort by</option>
              {properties.map((property) => (
                <option key={property.id} value={property.name}>{property.display_name || property.name}</option>
              ))}
            </select>
          </Field>

          <button type="button" className="of-button" onClick={onDelete} style={{ color: 'var(--status-danger)', borderColor: '#fecaca' }}>
            <Glyph name="trash" size={12} /> Delete widget
          </button>
        </div>
      ) : (
        <div style={{ padding: 14 }}>
          <p className="of-text-muted" style={{ fontSize: 12 }}>{tab === 'metadata' ? 'Widget metadata' : 'Display options'} coming soon.</p>
        </div>
      )}
    </div>
  );
}

function DragHandleGlyph() {
  return (
    <svg width={10} height={14} viewBox="0 0 10 14" aria-hidden="true">
      {[0, 1, 2].map((row) => (
        [0, 1].map((col) => (
          <circle key={`${row}-${col}`} cx={2 + col * 6} cy={2 + row * 5} r={1.2} fill="#aab4c0" />
        ))
      ))}
    </svg>
  );
}

function ColumnConfiguration({
  objectType,
  properties,
  columns,
  onToggle,
  onReorder,
  onAddAll,
  onRemoveAll,
}: {
  objectType: ObjectType | null;
  properties: Property[];
  columns: string[];
  onToggle: (name: string) => void;
  onMove: (name: string, direction: -1 | 1) => void;
  onReorder: (from: number, to: number) => void;
  onAddAll: () => void;
  onRemoveAll: () => void;
}) {
  const [addOpen, setAddOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [dragIndex, setDragIndex] = useState<number | null>(null);
  const [dropIndex, setDropIndex] = useState<number | null>(null);

  const selected = useMemo(() => columns.map((name) => properties.find((p) => p.name === name)).filter((p): p is Property => Boolean(p)), [columns, properties]);
  const unselected = useMemo(() => properties.filter((p) => !columns.includes(p.name)), [properties, columns]);
  const filteredUnselected = unselected.filter((p) => `${p.display_name} ${p.name}`.toLowerCase().includes(search.toLowerCase()));

  function handleDragStart(index: number) {
    return (event: React.DragEvent<HTMLDivElement>) => {
      setDragIndex(index);
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', String(index));
    };
  }
  function handleDragOver(index: number) {
    return (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      event.dataTransfer.dropEffect = 'move';
      if (dropIndex !== index) setDropIndex(index);
    };
  }
  function handleDrop(index: number) {
    return (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();
      const from = dragIndex ?? Number(event.dataTransfer.getData('text/plain'));
      if (Number.isFinite(from)) onReorder(from, index);
      setDragIndex(null);
      setDropIndex(null);
    };
  }

  return (
    <div style={{ display: 'grid', gap: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '4px 4px' }}>
        <Glyph name="cube" size={13} tone="#2d72d2" />
        <span style={{ fontSize: 13, fontWeight: 600 }}>{objectType?.display_name || objectType?.name || 'Object'}</span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>
        Columns <span aria-hidden="true">ⓘ</span>
      </div>
      <div style={{ display: 'grid', gap: 4 }}>
        {selected.length === 0 ? (
          <p className="of-text-muted" style={{ margin: 0, fontSize: 12, padding: '6px 8px' }}>No columns selected.</p>
        ) : selected.map((property, index) => (
          <div
            key={property.id}
            draggable
            onDragStart={handleDragStart(index)}
            onDragOver={handleDragOver(index)}
            onDrop={handleDrop(index)}
            onDragEnd={() => { setDragIndex(null); setDropIndex(null); }}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              padding: '6px 8px',
              border: '1px solid var(--border-subtle)',
              borderRadius: 4,
              background: dropIndex === index ? 'rgba(45, 114, 210, 0.06)' : '#fff',
              opacity: dragIndex === index ? 0.6 : 1,
              cursor: 'grab',
              fontSize: 13,
            }}
            role="listitem"
            aria-label={`Reorder ${property.display_name || property.name}`}
          >
            <span style={{ display: 'inline-flex', cursor: 'grab' }}><DragHandleGlyph /></span>
            <span style={{ flex: 1 }}>{property.display_name || property.name}</span>
            <button
              type="button"
              aria-label="Remove column"
              onClick={() => onToggle(property.name)}
              style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--status-danger)', padding: 2 }}
            >
              <Glyph name="trash" size={11} />
            </button>
            <Glyph name="chevron-down" size={11} tone="#5c7080" />
          </div>
        ))}
      </div>
      <div style={{ position: 'relative' }}>
        <button
          type="button"
          onClick={() => setAddOpen((open) => !open)}
          className="of-button"
          style={{ width: '100%', justifyContent: 'center', fontSize: 12 }}
        >
          <Glyph name="plus" size={11} /> Add column
        </button>
        {addOpen ? (
          <div
            role="menu"
            style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 4, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.12)', padding: 6, zIndex: 5, maxHeight: 280, overflowY: 'auto' }}
          >
            <input
              autoFocus
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search property…"
              style={{ width: '100%', padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 13, marginBottom: 6 }}
            />
            <p className="of-text-muted" style={{ margin: '4px 6px', fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
              Current object ({filteredUnselected.length})
            </p>
            {filteredUnselected.length === 0 ? (
              <p className="of-text-muted" style={{ padding: 8, fontSize: 12, margin: 0 }}>No more properties.</p>
            ) : filteredUnselected.map((property) => (
              <button
                key={property.id}
                type="button"
                onClick={() => { onToggle(property.name); setAddOpen(false); setSearch(''); }}
                style={addWidgetItemStyle()}
              >
                <Glyph name="tag" size={11} tone="#5c7080" /> {property.display_name || property.name}
              </button>
            ))}
          </div>
        ) : null}
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 2 }}>
        <button type="button" className="of-link" onClick={onAddAll} style={linkBtnStyle()}>Add all properties</button>
        <button type="button" className="of-link" onClick={onRemoveAll} style={{ ...linkBtnStyle(), color: 'var(--status-danger)' }}>Remove all properties</button>
      </div>
    </div>
  );
}

export function ObjectTableWidgetView({ widget, variables }: { widget: AppWidget; variables: WorkshopVariable[] }) {
  const sourceVariableId = (widget.props as { source_variable_id?: string })?.source_variable_id ?? '';
  const sourceVariable = variables.find((v) => v.id === sourceVariableId) ?? null;
  const objectTypeId = sourceVariable?.object_type_id ?? (widget.props as { object_type_id?: string })?.object_type_id ?? '';
  const columns: string[] = ((widget.props as { columns?: string[] })?.columns) ?? [];
  const sortProperty = (widget.props as { default_sort_property?: string })?.default_sort_property ?? '';
  const [properties, setProperties] = useState<Property[]>([]);
  const [rows, setRows] = useState<ObjectInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const runtime = useRuntime();
  const selectedRows = sourceVariable?.kind === 'object_set_selection' ? runtime.variableEngine.getSelectedObjectSet(sourceVariableId) : null;
  const activeObjectVariable = useMemo(
    () => variables.find((v) => v.kind === 'object_set_active_object' && v.source_widget_id === widget.id) ?? null,
    [variables, widget.id],
  );
  const activeObjectId = activeObjectVariable ? runtime.variableEngine.getActiveObject(activeObjectVariable.id)?.id ?? null : null;

  useEffect(() => {
    if (!objectTypeId) {
      setRows([]);
      setProperties([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    const objectsPromise = selectedRows !== null
      ? Promise.resolve({ data: selectedRows, total: selectedRows.length })
      : fetchObjectsForVariable(objectTypeId, sourceVariable, 5000, runtime.variableEngine);
    void Promise.all([listProperties(objectTypeId), objectsPromise])
      .then(([propResponse, fetchResponse]) => {
        if (cancelled) return;
        setProperties(propResponse);
        setRows(fetchResponse.data);
      })
      .catch(() => {
        if (cancelled) return;
        setRows([]);
        setProperties([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [objectTypeId, runtime.refreshKey, runtime.variableEngine, selectedRows, sourceVariable]);

  const visibleColumns = columns.length > 0 ? columns : properties.map((property) => property.name);

  const filteredRows = useMemo(() => {
    if (!runtime.preview) return rows;
    return rows;
  }, [rows, runtime.preview]);

  const sortedRows = useMemo(() => {
    if (!sortProperty) return filteredRows;
    return [...filteredRows].sort((a, b) => {
      const av = String((a.properties as Record<string, unknown>)?.[sortProperty] ?? '');
      const bv = String((b.properties as Record<string, unknown>)?.[sortProperty] ?? '');
      return av.localeCompare(bv);
    });
  }, [filteredRows, sortProperty]);

  if (!objectTypeId) {
    return (
      <div style={{ padding: '36px 24px', textAlign: 'center' }}>
        <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>Pick an Object set in the inspector to render this table.</p>
      </div>
    );
  }

  return (
    <div style={{ overflow: 'auto', maxHeight: 360 }}>
      <table className="of-table" style={{ width: '100%', fontSize: 12 }}>
        <thead>
          <tr>
            {visibleColumns.map((column) => (
              <th key={column} style={{ padding: '6px 10px', textAlign: 'left', borderBottom: '1px solid var(--border-subtle)' }}>
                {properties.find((property) => property.name === column)?.display_name || column}
                {sortProperty === column ? <span style={{ marginLeft: 6, color: 'var(--status-info)' }}>↕</span> : null}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading ? (
            <tr><td colSpan={visibleColumns.length} style={{ padding: 16, textAlign: 'center' }}><span className="of-text-muted">Loading…</span></td></tr>
          ) : sortedRows.length === 0 ? (
            <tr><td colSpan={visibleColumns.length} style={{ padding: 16, textAlign: 'center' }}><span className="of-text-muted">No objects.</span></td></tr>
          ) : (
            sortedRows.slice(0, 100).map((row) => {
              const isActive = activeObjectId === row.id;
              const interactive = runtime.preview && activeObjectVariable;
              return (
                <tr
                  key={row.id}
                  onClick={() => {
                    if (interactive && activeObjectVariable) {
                      runtime.setActiveObject(activeObjectVariable.id, row);
                    }
                  }}
                  style={{ background: isActive ? 'rgba(45, 114, 210, 0.08)' : undefined, cursor: interactive ? 'pointer' : 'default' }}
                >
                  {visibleColumns.map((column) => (
                    <td key={column} style={{ padding: '6px 10px', borderBottom: '1px solid var(--border-subtle)' }}>
                      {String((row.properties as Record<string, unknown>)?.[column] ?? '')}
                    </td>
                  ))}
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}

export function MapWidgetView({ widget, variables }: { widget: AppWidget; variables: WorkshopVariable[] }) {
  const runtime = useRuntime();
  return (
    <div style={{ padding: 12, minHeight: 320 }}>
      <WorkshopMapWidget
        widget={widget}
        variables={variables}
        variableEngine={runtime.variableEngine}
        onSelectObject={(variableId, object) => runtime.setActiveObject(variableId, object)}
        onSelectObjectSet={(variableId, objects) => runtime.setSelectedObjectSet(variableId, objects)}
        onShapeChange={(variableId, shape) => runtime.setShapeOutput(variableId, shape)}
      />
    </div>
  );
}

function MapWidgetInspector({
  widget,
  variables,
  objectTypes,
  onChange,
  onRenameOutput,
  onDelete,
}: {
  widget: AppWidget;
  variables: WorkshopVariable[];
  objectTypes: ObjectType[];
  onChange: (next: AppWidget) => void;
  onRenameOutput: (name: string, objectTypeId: string) => void;
  onDelete: () => void;
}) {
  const [tab, setTab] = useState<'setup' | 'metadata' | 'display'>('setup');
  const cfgLayers = readMapLayerConfigs(widget.props);
  const cfgOverlays = readMapOverlayConfigs(widget.props);
  const outputVariableId = (widget.props as { output_variable_id?: string })?.output_variable_id ?? '';
  const outputObjectSetVariableId = (widget.props as { output_object_set_variable_id?: string })?.output_object_set_variable_id ?? '';
  const outputShapeVariableId = (widget.props as { output_shape_variable_id?: string })?.output_shape_variable_id ?? '';
  const shapeSearchOutputVariableId = (widget.props as { shape_search_output_variable_id?: string })?.shape_search_output_variable_id ?? '';
  const outputVariable = variables.find((entry) => entry.id === outputVariableId) ?? null;

  function patchProps(patch: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patch } });
  }

  function patchLayers(layers: WorkshopMapLayerConfig[]) {
    patchProps({ layers });
  }

  function patchLayer(layerId: string, patch: Partial<WorkshopMapLayerConfig>) {
    const layers = cfgLayers.map((layer) => (layer.id === layerId ? { ...layer, ...patch } : layer));
    patchLayers(layers);
    const firstObjectTypeId = firstMapLayerObjectType(layers, variables);
    if (firstObjectTypeId) onRenameOutput(outputVariable?.name ?? `${widget.title} Selected object`, firstObjectTypeId);
  }

  function addLayer() {
    patchLayers([
      ...cfgLayers,
      {
        id: makeId('map_layer'),
        title: `Layer ${cfgLayers.length + 1}`,
        source: 'object_set',
        loading_mode: 'eager',
        source_variable_id: '',
        object_type_id: '',
        tile_layer_id: '',
        tile_page_size: 500,
        tile_simplify_tolerance: 0,
        geometry_type: 'point',
        latitude_field: 'lat',
        longitude_field: 'lon',
        geometry_field: '',
        label_field: 'label',
        color: '#15803d',
        visible: true,
        locked: false,
        filter_field: '',
        filter_value: '',
        radius: 6,
        line_width: 3,
        fill_opacity: 0.22,
        cluster_enabled: false,
        cluster_radius: 64,
        cluster_max_zoom: 10,
        cluster_color: '#15803d',
      },
    ]);
  }

  function removeLayer(layerId: string) {
    patchLayers(cfgLayers.filter((layer) => layer.id !== layerId));
  }

  function patchOverlays(overlays: WorkshopMapOverlayLayerConfig[]) {
    patchProps({ overlay_layers: overlays });
  }

  function patchOverlay(overlayId: string, patch: Partial<WorkshopMapOverlayLayerConfig>) {
    patchOverlays(cfgOverlays.map((overlay) => (overlay.id === overlayId ? { ...overlay, ...patch } : overlay)));
  }

  function addOverlay() {
    patchOverlays([
      ...cfgOverlays,
      {
        id: makeId('map_overlay'),
        title: `Overlay ${cfgOverlays.length + 1}`,
        source: 'geojson_url',
        url: '',
        resource_id: '',
        source_layer: '',
        geometry_type: 'auto',
        color: '#64748b',
        visible: true,
        opacity: 0.78,
        radius: 5,
        line_width: 2,
        fill_opacity: 0.14,
        min_zoom: 0,
        max_zoom: 22,
        attribution: '',
      },
    ]);
  }

  function removeOverlay(overlayId: string) {
    patchOverlays(cfgOverlays.filter((overlay) => overlay.id !== overlayId));
  }

  return (
    <div style={inspectorStyle()}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>{widget.title}</span>
        <span className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.06em' }}>MAP</span>
      </div>
      <div style={{ display: 'flex', gap: 0, padding: '0 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        {(['setup', 'metadata', 'display'] as const).map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => setTab(value)}
            style={{ padding: '8px 6px', border: 0, background: 'transparent', borderBottom: tab === value ? '2px solid var(--status-info)' : '2px solid transparent', cursor: 'pointer', fontSize: 12, fontWeight: tab === value ? 600 : 500, color: tab === value ? 'var(--text-strong)' : 'var(--text-muted)', marginRight: 14 }}
          >
            {value === 'setup' ? 'Widget setup' : value === 'metadata' ? 'Metadata' : 'Display'}
          </button>
        ))}
      </div>
      {tab === 'setup' ? (
        <div style={{ padding: 14, display: 'grid', gap: 14 }}>
          <Section title="Base map" />
          <Field label="Base layer">
            <select value={((widget.props as { base_layer_kind?: string })?.base_layer_kind) ?? 'blank'} onChange={(event) => patchProps({ base_layer_kind: event.target.value })} style={inputStyle()}>
              <option value="blank">OpenFoundry light background</option>
              <option value="streets">OpenStreetMap raster tiles</option>
            </select>
          </Field>
          <Field label="Zoom">
            <input type="number" min={1} max={18} value={String((widget.props as { zoom?: number })?.zoom ?? 11)} onChange={(event) => patchProps({ zoom: Number(event.target.value) })} style={inputStyle()} />
          </Field>

          <Section title={`Object layers ${cfgLayers.length}`} />
          <div style={{ display: 'grid', gap: 8 }}>
            {cfgLayers.map((layer) => {
              const variable = variables.find((entry) => entry.id === layer.source_variable_id) ?? null;
              const objectTypeId = variable?.object_type_id || layer.object_type_id;
              const objectType = objectTypes.find((entry) => entry.id === objectTypeId) ?? null;
              return (
                <div key={layer.id} style={{ display: 'grid', gap: 8, padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f7f9fa' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                    <Glyph name="graph" size={13} tone={layer.color} />
                    <input value={layer.title} onChange={(event) => patchLayer(layer.id, { title: event.target.value })} style={{ ...inputStyle(), flex: 1 }} />
                    {cfgLayers.length > 1 ? (
                      <button type="button" aria-label="Remove layer" onClick={() => removeLayer(layer.id)} style={{ border: 0, background: 'transparent', color: 'var(--status-danger)', cursor: 'pointer' }}>
                        <Glyph name="trash" size={11} />
                      </button>
                    ) : null}
                  </div>
                  <Field label="Input object set">
                    <select
                      value={layer.source === 'geospatial_tile' ? 'tile' : layer.source_variable_id ? `var:${layer.source_variable_id}` : layer.object_type_id ? `type:${layer.object_type_id}` : ''}
                      onChange={(event) => {
                        const raw = event.target.value;
                        if (raw === 'tile') patchLayer(layer.id, { source: 'geospatial_tile', loading_mode: 'viewport_tiles', source_variable_id: '', object_type_id: '' });
                        else if (raw.startsWith('var:')) patchLayer(layer.id, { source: 'object_set', loading_mode: 'eager', source_variable_id: raw.slice(4), object_type_id: '', tile_layer_id: '' });
                        else if (raw.startsWith('type:')) patchLayer(layer.id, { source: 'object_type', loading_mode: 'eager', source_variable_id: '', object_type_id: raw.slice(5), tile_layer_id: '' });
                        else patchLayer(layer.id, { source: 'object_set', loading_mode: 'eager', source_variable_id: '', object_type_id: '', tile_layer_id: '' });
                      }}
                      style={inputStyle()}
                    >
                      <option value="">Select object set…</option>
                      <option value="tile">Viewport tile layer</option>
                      {variables
                        .filter((entry) => entry.kind === 'object_set' || entry.kind === 'object_set_definition' || entry.kind === 'filter_output')
                        .map((entry) => (
                          <option key={entry.id} value={`var:${entry.id}`}>{entry.name} ({VARIABLE_KIND_LABEL[entry.kind]})</option>
                        ))}
                      {objectTypes.map((type) => (
                        <option key={type.id} value={`type:${type.id}`}>{type.display_name || type.name}</option>
                      ))}
                    </select>
                    <span className="of-text-muted" style={{ marginTop: 4, fontSize: 11 }}>
                      Current value: {layer.source === 'geospatial_tile' ? (layer.tile_layer_id || 'tile layer not configured') : variable?.name ?? objectType?.display_name ?? objectType?.name ?? 'undefined'}
                    </span>
                  </Field>
                  {layer.source === 'geospatial_tile' ? (
                    <>
                      <Field label="Geospatial layer ID">
                        <input value={layer.tile_layer_id} onChange={(event) => patchLayer(layer.id, { tile_layer_id: event.target.value })} placeholder="geospatial layer UUID" style={inputStyle()} />
                      </Field>
                      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                        <Field label="Page size">
                          <input type="number" min={1} max={5000} value={String(layer.tile_page_size)} onChange={(event) => patchLayer(layer.id, { tile_page_size: Number(event.target.value) })} style={inputStyle()} />
                        </Field>
                        <Field label="Simplify tolerance">
                          <input type="number" min={0} step={0.0001} value={String(layer.tile_simplify_tolerance)} onChange={(event) => patchLayer(layer.id, { tile_simplify_tolerance: Number(event.target.value) })} style={inputStyle()} />
                        </Field>
                      </div>
                    </>
                  ) : null}
                  <Field label="Geometry">
                    <select value={layer.geometry_type} onChange={(event) => patchLayer(layer.id, { geometry_type: event.target.value as WorkshopMapLayerConfig['geometry_type'] })} style={inputStyle()}>
                      <option value="point">Point</option>
                      <option value="line">Line</option>
                      <option value="polygon">Polygon</option>
                      <option value="auto">Auto from GeoJSON</option>
                    </select>
                  </Field>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                    <Field label="Latitude">
                      <input value={layer.latitude_field} onChange={(event) => patchLayer(layer.id, { latitude_field: event.target.value })} placeholder="lat" style={inputStyle()} />
                    </Field>
                    <Field label="Longitude">
                      <input value={layer.longitude_field} onChange={(event) => patchLayer(layer.id, { longitude_field: event.target.value })} placeholder="lon" style={inputStyle()} />
                    </Field>
                  </div>
                  <Field label="GeoJSON / geoshape">
                    <input value={layer.geometry_field} onChange={(event) => patchLayer(layer.id, { geometry_field: event.target.value })} placeholder="geometry" style={inputStyle()} />
                  </Field>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 96px', gap: 8 }}>
                    <Field label="Label">
                      <input value={layer.label_field} onChange={(event) => patchLayer(layer.id, { label_field: event.target.value })} placeholder="name" style={inputStyle()} />
                    </Field>
                    <Field label="Color">
                      <input type="color" value={layer.color} onChange={(event) => patchLayer(layer.id, { color: event.target.value })} style={{ ...inputStyle(), padding: 2, height: 32 }} />
                    </Field>
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                    <Field label="Filter field">
                      <input value={layer.filter_field} onChange={(event) => patchLayer(layer.id, { filter_field: event.target.value })} placeholder="kind" style={inputStyle()} />
                    </Field>
                    <Field label="Filter value">
                      <input value={layer.filter_value} onChange={(event) => patchLayer(layer.id, { filter_value: event.target.value })} placeholder="trail_start" style={inputStyle()} />
                    </Field>
                  </div>
                  <Toggle label="Selectable" value={!layer.locked} onChange={(checked) => patchLayer(layer.id, { locked: !checked })} />
                  <Toggle label="Cluster points" value={layer.cluster_enabled} onChange={(checked) => patchLayer(layer.id, { cluster_enabled: checked })} />
                  {layer.cluster_enabled ? (
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 96px', gap: 8 }}>
                      <Field label="Cluster radius">
                        <input type="number" min={24} max={160} value={String(layer.cluster_radius)} onChange={(event) => patchLayer(layer.id, { cluster_radius: Number(event.target.value) })} style={inputStyle()} />
                      </Field>
                      <Field label="Max cluster zoom">
                        <input type="number" min={1} max={18} value={String(layer.cluster_max_zoom)} onChange={(event) => patchLayer(layer.id, { cluster_max_zoom: Number(event.target.value) })} style={inputStyle()} />
                      </Field>
                      <Field label="Color">
                        <input type="color" value={layer.cluster_color || layer.color} onChange={(event) => patchLayer(layer.id, { cluster_color: event.target.value })} style={{ ...inputStyle(), padding: 2, height: 32 }} />
                      </Field>
                    </div>
                  ) : null}
                </div>
              );
            })}
            <button type="button" onClick={addLayer} className="of-button" style={{ fontSize: 12, justifyContent: 'center' }}>
              <Glyph name="plus" size={11} /> Add object layer
            </button>
          </div>

          <Section title={`Overlay layers ${cfgOverlays.length}`} />
          <div style={{ display: 'grid', gap: 8 }}>
            {cfgOverlays.map((overlay) => (
              <div key={overlay.id} style={{ display: 'grid', gap: 8, padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f7f9fa' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <Glyph name="graph" size={13} tone={overlay.color} />
                  <input value={overlay.title} onChange={(event) => patchOverlay(overlay.id, { title: event.target.value })} style={{ ...inputStyle(), flex: 1 }} />
                  <button type="button" aria-label="Remove overlay" onClick={() => removeOverlay(overlay.id)} style={{ border: 0, background: 'transparent', color: 'var(--status-danger)', cursor: 'pointer' }}>
                    <Glyph name="trash" size={11} />
                  </button>
                </div>
                <Field label="Overlay source">
                  <select value={overlay.source} onChange={(event) => patchOverlay(overlay.id, { source: event.target.value as WorkshopMapOverlayLayerConfig['source'] })} style={inputStyle()}>
                    <option value="geojson_url">GeoJSON URL</option>
                    <option value="mvt_url">MVT URL</option>
                    <option value="saved_map_layer">Saved map layer resource</option>
                    <option value="raster_url">Raster tile URL</option>
                  </select>
                </Field>
                {overlay.source === 'saved_map_layer' ? (
                  <Field label="Saved layer resource ID">
                    <input value={overlay.resource_id} onChange={(event) => patchOverlay(overlay.id, { resource_id: event.target.value })} placeholder="geospatial layer id" style={inputStyle()} />
                  </Field>
                ) : (
                  <Field label={overlay.source === 'mvt_url' || overlay.source === 'raster_url' ? 'Tile URL template' : 'GeoJSON URL'}>
                    <input value={overlay.url} onChange={(event) => patchOverlay(overlay.id, { url: event.target.value })} placeholder={overlay.source === 'geojson_url' ? '/layers/trails.geojson' : 'https://tiles.example/{z}/{x}/{y}.pbf'} style={inputStyle()} />
                  </Field>
                )}
                {overlay.source === 'mvt_url' ? (
                  <Field label="MVT source layer">
                    <input value={overlay.source_layer} onChange={(event) => patchOverlay(overlay.id, { source_layer: event.target.value })} placeholder="layer name inside tile" style={inputStyle()} />
                  </Field>
                ) : null}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 96px', gap: 8 }}>
                  <Field label="Geometry">
                    <select value={overlay.geometry_type} onChange={(event) => patchOverlay(overlay.id, { geometry_type: event.target.value as WorkshopMapOverlayLayerConfig['geometry_type'] })} style={inputStyle()}>
                      <option value="auto">Auto</option>
                      <option value="point">Point</option>
                      <option value="line">Line</option>
                      <option value="polygon">Polygon</option>
                    </select>
                  </Field>
                  <Field label="Color">
                    <input type="color" value={overlay.color} onChange={(event) => patchOverlay(overlay.id, { color: event.target.value })} style={{ ...inputStyle(), padding: 2, height: 32 }} />
                  </Field>
                </div>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                  <Field label="Opacity">
                    <input type="number" min={0} max={1} step={0.05} value={String(overlay.opacity)} onChange={(event) => patchOverlay(overlay.id, { opacity: Number(event.target.value) })} style={inputStyle()} />
                  </Field>
                  <Field label="Line width">
                    <input type="number" min={1} max={16} step={1} value={String(overlay.line_width)} onChange={(event) => patchOverlay(overlay.id, { line_width: Number(event.target.value) })} style={inputStyle()} />
                  </Field>
                </div>
                <Toggle label="Visible by default" value={overlay.visible} onChange={(checked) => patchOverlay(overlay.id, { visible: checked })} />
              </div>
            ))}
            <button type="button" onClick={addOverlay} className="of-button" style={{ fontSize: 12, justifyContent: 'center' }}>
              <Glyph name="plus" size={11} /> Add overlay layer
            </button>
          </div>

          <Section title="Selected objects" />
          <Field label="Active object output">
            <select value={outputVariableId} onChange={(event) => patchProps({ output_variable_id: event.target.value })} style={inputStyle()}>
              <option value="">No selection output</option>
              {variables
                .filter((entry) => entry.kind === 'object_set_active_object')
                .map((entry) => (
                  <option key={entry.id} value={entry.id}>{entry.name}</option>
                ))}
            </select>
          </Field>
          <Field label="Selected object set output">
            <select value={outputObjectSetVariableId} onChange={(event) => patchProps({ output_object_set_variable_id: event.target.value })} style={inputStyle()}>
              <option value="">No object-set output</option>
              {variables
                .filter((entry) => entry.kind === 'object_set_selection')
                .map((entry) => (
                  <option key={entry.id} value={entry.id}>{entry.name}</option>
                ))}
            </select>
          </Field>
          <Toggle label="Shape drawing tools" value={((widget.props as { enable_shape_drawing?: boolean })?.enable_shape_drawing) ?? true} onChange={(checked) => patchProps({ enable_shape_drawing: checked })} />
          <Field label="Drawn shape output">
            <select value={outputShapeVariableId} onChange={(event) => patchProps({ output_shape_variable_id: event.target.value })} style={inputStyle()}>
              <option value="">No shape output</option>
              {variables
                .filter((entry) => entry.kind === 'shape_output')
                .map((entry) => (
                  <option key={entry.id} value={entry.id}>{entry.name}</option>
                ))}
            </select>
          </Field>
          <Field label="Shape search output">
            <select value={shapeSearchOutputVariableId} onChange={(event) => patchProps({ shape_search_output_variable_id: event.target.value })} style={inputStyle()}>
              <option value="">No search output</option>
              {variables
                .filter((entry) => entry.kind === 'object_set_selection')
                .map((entry) => (
                  <option key={entry.id} value={entry.id}>{entry.name}</option>
                ))}
            </select>
          </Field>
          <button type="button" className="of-button" onClick={onDelete} style={{ color: 'var(--status-danger)', borderColor: '#fecaca' }}>
            <Glyph name="trash" size={12} /> Delete widget
          </button>
        </div>
      ) : tab === 'display' ? (
        <DisplayTab widget={widget} onChange={onChange} />
      ) : (
        <div style={{ padding: 14 }}><p className="of-text-muted" style={{ fontSize: 12 }}>Widget metadata coming soon.</p></div>
      )}
    </div>
  );
}

function firstMapLayerObjectType(layers: WorkshopMapLayerConfig[], variables: WorkshopVariable[]) {
  for (const layer of layers) {
    const variable = variables.find((entry) => entry.id === layer.source_variable_id);
    if (variable?.object_type_id) return variable.object_type_id;
    if (layer.object_type_id) return layer.object_type_id;
  }
  return '';
}

function ObjectSetPicker({ objectTypes, onClose, onSelect }: { objectTypes: ObjectType[]; onClose: () => void; onSelect: (typeId: string) => void }) {
  const [search, setSearch] = useState('');
  const filtered = objectTypes.filter((type) => `${type.display_name} ${type.name}`.toLowerCase().includes(search.toLowerCase()));
  return (
    <div role="dialog" aria-modal="true" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }} style={{ position: 'fixed', inset: 0, zIndex: 90, background: 'rgba(17, 24, 39, 0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
      <section style={{ width: '100%', maxWidth: 720, height: 'min(540px, 90vh)', background: '#fff', borderRadius: 6, boxShadow: '0 20px 48px rgba(15, 23, 42, 0.2)', display: 'grid', gridTemplateRows: 'auto 1fr auto' }}>
        <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px 18px', borderBottom: '1px solid var(--border-subtle)' }}>
          <h2 style={{ margin: 0, fontSize: 14, fontWeight: 600 }}>Select starting object set</h2>
          <button type="button" aria-label="Close" onClick={onClose} className="of-button of-button--ghost" style={{ padding: 4 }}><Glyph name="x" size={14} /></button>
        </header>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', minHeight: 0 }}>
          <aside style={{ borderRight: '1px solid var(--border-subtle)', overflowY: 'auto', padding: 8 }}>
            <input
              autoFocus
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search"
              style={{ width: '100%', padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 13, marginBottom: 8 }}
            />
            <p className="of-text-muted" style={{ margin: '4px 6px', fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.05em' }}>Search results</p>
            {filtered.map((type) => (
              <button key={type.id} type="button" onClick={() => onSelect(type.id)} style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '6px 8px', border: 0, background: 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13, borderRadius: 4 }}>
                <Glyph name="cube" size={13} tone="var(--status-info)" />
                {type.display_name || type.name}
              </button>
            ))}
            {filtered.length === 0 ? (<p className="of-text-muted" style={{ padding: 12, fontSize: 12 }}>No results</p>) : null}
          </aside>
          <div style={{ padding: 18, overflowY: 'auto' }}>
            <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>Pick an object type from the left to back the table.</p>
          </div>
        </div>
        <footer style={{ display: 'flex', justifyContent: 'flex-end', padding: 12, borderTop: '1px solid var(--border-subtle)' }}>
          <button type="button" onClick={onClose} className="of-button">Cancel</button>
        </footer>
      </section>
    </div>
  );
}

function Section({ title }: { title: string }) {
  return <p style={{ margin: '6px 0 0', fontSize: 11, fontWeight: 700, letterSpacing: '0.06em', color: 'var(--text-muted)', textTransform: 'uppercase' }}>{title}</p>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label style={{ display: 'grid', gap: 4 }}>
      <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{label}</span>
      {children}
    </label>
  );
}

function inspectorStyle(): React.CSSProperties {
  return { display: 'grid', gap: 0 };
}

function inputStyle(): React.CSSProperties {
  return { padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff', fontSize: 13, color: 'var(--text-strong)', width: '100%' };
}

function linkBtnStyle(): React.CSSProperties {
  return { background: 'none', border: 0, padding: 0, color: 'var(--status-info)', cursor: 'pointer', fontSize: 12 };
}

function addWidgetItemStyle(): React.CSSProperties {
  return { display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '6px 10px', border: 0, background: 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13 };
}

function FilterListGlyph() {
  return (
    <svg width={13} height={13} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M3 5h18l-7 9v6l-4-2v-4z" stroke="#5c7080" strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  );
}

export function FilterListWidgetView({ widget }: { widget: AppWidget }) {
  const filters = ((widget.props as { filters?: FilterEntry[] })?.filters) ?? [];
  const layout = ((widget.props as { layout?: string })?.layout) ?? 'vertical';
  const outputVariableId = (widget.props as { output_variable_id?: string })?.output_variable_id ?? '';
  const runtime = useRuntime();
  if (filters.length === 0) {
    return (
      <div style={{ padding: '36px 20px', textAlign: 'center' }}>
        <FilterListGlyph />
        <p style={{ margin: '8px 0 0', fontSize: 13, fontWeight: 600 }}>Filter list</p>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>Select this widget to edit configuration.</p>
      </div>
    );
  }
  return (
    <div style={{ padding: 12, display: layout === 'pills' ? 'flex' : 'grid', flexWrap: layout === 'pills' ? 'wrap' : undefined, gap: layout === 'pills' ? 6 : 12 }}>
      {filters.map((filter) => {
        const value = runtime.filterValues[filter.id] ?? {};
        const interactive = runtime.preview;
        return (
          <div key={filter.id} style={{ display: 'grid', gap: 4 }}>
            <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--text-muted)', textTransform: 'uppercase' }}>{filter.display_name}</span>
            {filter.component === 'multi_select' || filter.component === 'search' ? (
              <input
                placeholder="Search…"
                value={value.search ?? ''}
                readOnly={!interactive}
                onChange={(event) => runtime.setFilterValue(filter.id, { ...value, search: event.target.value }, {
                  outputVariableId,
                  sourceWidgetId: widget.id,
                  propertyName: filter.property_name,
                  component: filter.component,
                })}
                style={{ padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 12, background: '#fff' }}
              />
            ) : (
              <div style={{ display: 'flex', gap: 6 }}>
                <input
                  placeholder="Min"
                  value={value.range_min ?? ''}
                  readOnly={!interactive}
                  onChange={(event) => runtime.setFilterValue(filter.id, { ...value, range_min: event.target.value }, {
                    outputVariableId,
                    sourceWidgetId: widget.id,
                    propertyName: filter.property_name,
                    component: filter.component,
                  })}
                  style={{ padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 12, background: '#fff', flex: 1 }}
                />
                <input
                  placeholder="Max"
                  value={value.range_max ?? ''}
                  readOnly={!interactive}
                  onChange={(event) => runtime.setFilterValue(filter.id, { ...value, range_max: event.target.value }, {
                    outputVariableId,
                    sourceWidgetId: widget.id,
                    propertyName: filter.property_name,
                    component: filter.component,
                  })}
                  style={{ padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 12, background: '#fff', flex: 1 }}
                />
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function FilterListInspector({
  widget,
  variables,
  outputName,
  onChange,
  onRenameOutput,
  onDelete,
}: {
  widget: AppWidget;
  objectTypes: ObjectType[];
  variables: WorkshopVariable[];
  outputName: string;
  onChange: (next: AppWidget) => void;
  onRenameOutput: (name: string) => void;
  onDelete: () => void;
}) {
  const [tab, setTab] = useState<'setup' | 'metadata' | 'display'>('setup');
  const sourceVariableId = (widget.props as { source_variable_id?: string })?.source_variable_id ?? '';
  const sourceVariable = variables.find((v) => v.id === sourceVariableId) ?? null;
  const objectTypeId = sourceVariable?.object_type_id ?? '';
  const filters: FilterEntry[] = ((widget.props as { filters?: FilterEntry[] })?.filters) ?? [];
  const allowAddRemove = Boolean((widget.props as { allow_add_remove?: boolean })?.allow_add_remove);
  const layout = ((widget.props as { layout?: 'vertical' | 'pills' })?.layout) ?? 'vertical';
  const [properties, setProperties] = useState<Property[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (!objectTypeId) {
      setProperties([]);
      return;
    }
    let cancelled = false;
    void listProperties(objectTypeId)
      .then((response) => { if (!cancelled) setProperties(response); })
      .catch(() => { if (!cancelled) setProperties([]); });
    return () => { cancelled = true; };
  }, [objectTypeId]);

  function patchProps(patch: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patch } });
  }

  function addFilter(propertyName: string) {
    if (filters.some((entry) => entry.property_name === propertyName)) return;
    const property = properties.find((entry) => entry.name === propertyName);
    const next: FilterEntry = {
      id: makeId('filter'),
      property_name: propertyName,
      display_name: property?.display_name ?? propertyName,
      component: 'multi_select',
      values: [],
      range_min: '',
      range_max: '',
    };
    patchProps({ filters: [...filters, next] });
    setAddOpen(false);
    setSearch('');
  }

  function patchFilter(id: string, patch: Partial<FilterEntry>) {
    patchProps({ filters: filters.map((entry) => (entry.id === id ? { ...entry, ...patch } : entry)) });
  }

  function removeFilter(id: string) {
    patchProps({ filters: filters.filter((entry) => entry.id !== id) });
  }

  const filteredProperties = properties.filter((entry) => `${entry.display_name} ${entry.name}`.toLowerCase().includes(search.toLowerCase()));

  return (
    <div style={inspectorStyle()}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>{widget.title}</span>
        <span className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.06em' }}>FILTER LIST</span>
      </div>
      <div style={{ display: 'flex', gap: 0, padding: '0 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        {(['setup', 'metadata', 'display'] as const).map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => setTab(value)}
            style={{ padding: '8px 6px', border: 0, background: 'transparent', borderBottom: tab === value ? '2px solid var(--status-info)' : '2px solid transparent', cursor: 'pointer', fontSize: 12, fontWeight: tab === value ? 600 : 500, color: tab === value ? 'var(--text-strong)' : 'var(--text-muted)', marginRight: 14 }}
          >
            {value === 'setup' ? 'Widget setup' : value === 'metadata' ? 'Metadata' : 'Display'}
          </button>
        ))}
      </div>
      {tab === 'setup' ? (
        <div style={{ padding: 14, display: 'grid', gap: 14 }}>
          <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>Display and update a filter variable that can be used to dynamically filter downstream object set variables.</p>
          <Section title="Input data" />
          <Field label="Object set">
            <select
              value={sourceVariableId}
              onChange={(event) => patchProps({ source_variable_id: event.target.value, filters: [] })}
              style={inputStyle()}
            >
              <option value="">Select object set variable…</option>
              {variables
                .filter((v) => v.kind === 'object_set' || v.kind === 'object_set_definition')
                .map((variable) => (
                  <option key={variable.id} value={variable.id}>{variable.name}</option>
                ))}
            </select>
            {sourceVariable ? (
              <span className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>Current value: {sourceVariable.name}</span>
            ) : null}
          </Field>

          <Section title="Filters configuration" />
          <p style={{ margin: 0, fontSize: 11, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--text-muted)' }}>FILTER LIST</p>
          <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>Add, reorder and rename filters</p>
          {filters.length > 0 ? (
            <div style={{ display: 'grid', gap: 6 }}>
              {filters.map((filter) => (
                <details key={filter.id} style={{ background: '#f4f6f9', border: '1px solid var(--border-subtle)', borderRadius: 4 }}>
                  <summary style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', cursor: 'pointer', listStyle: 'none', fontSize: 13 }}>
                    <Glyph name="move" size={12} tone="#aab4c0" />
                    <span style={{ flex: 1 }}>{filter.display_name}</span>
                    <Glyph name="chevron-down" size={11} />
                  </summary>
                  <div style={{ padding: '8px 12px', display: 'grid', gap: 8, borderTop: '1px solid var(--border-subtle)' }}>
                    <Field label="Filter name">
                      <input value={filter.display_name} onChange={(event) => patchFilter(filter.id, { display_name: event.target.value })} style={inputStyle()} />
                    </Field>
                    <Field label="Filter component">
                      <select value={filter.component} onChange={(event) => patchFilter(filter.id, { component: event.target.value as FilterComponent })} style={inputStyle()}>
                        {(Object.keys(FILTER_COMPONENT_LABEL) as FilterComponent[]).map((kind) => (
                          <option key={kind} value={kind}>{FILTER_COMPONENT_LABEL[kind]}</option>
                        ))}
                      </select>
                    </Field>
                    <button type="button" onClick={() => removeFilter(filter.id)} className="of-button" style={{ fontSize: 12, color: 'var(--status-danger)', borderColor: '#fecaca' }}>
                      <Glyph name="trash" size={11} /> Remove filter
                    </button>
                  </div>
                </details>
              ))}
            </div>
          ) : null}

          <div style={{ position: 'relative' }}>
            <button type="button" onClick={() => setAddOpen((open) => !open)} className="of-button" style={{ width: '100%', justifyContent: 'center', fontSize: 12 }}>
              <Glyph name="plus" size={12} /> Add filter
            </button>
            {addOpen ? (
              <div role="menu" style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 4, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.12)', padding: 6, zIndex: 5 }}>
                <input autoFocus value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search property…" style={{ width: '100%', padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 13, marginBottom: 6 }} />
                <p className="of-text-muted" style={{ margin: '4px 6px', fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.05em' }}>Filter on a single property ({filteredProperties.length})</p>
                {filteredProperties.map((property) => (
                  <button key={property.id} type="button" onClick={() => addFilter(property.name)} style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '6px 8px', border: 0, background: 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13 }}>
                    <Glyph name="tag" size={11} tone="#5c7080" />
                    {property.display_name || property.name}
                  </button>
                ))}
                {filteredProperties.length === 0 ? (<p className="of-text-muted" style={{ padding: 8, fontSize: 12 }}>No properties.</p>) : null}
              </div>
            ) : null}
          </div>

          <Toggle label="Allow users to add and remove filters" value={allowAddRemove} onChange={(checked) => patchProps({ allow_add_remove: checked })} />
          <Field label="Layout">
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
              {(['vertical', 'pills'] as const).map((kind) => (
                <button
                  key={kind}
                  type="button"
                  onClick={() => patchProps({ layout: kind })}
                  style={{ padding: '14px 10px', border: layout === kind ? '2px solid var(--status-info)' : '1px solid var(--border-default)', borderRadius: 6, background: layout === kind ? 'rgba(45, 114, 210, 0.04)' : '#fff', cursor: 'pointer', fontSize: 12, textAlign: 'center' }}
                >
                  <div style={{ display: 'grid', gap: 4, justifyItems: 'center' }}>
                    <span style={{ width: 32, height: 8, background: '#aab4c0', borderRadius: kind === 'pills' ? 999 : 2 }} />
                    <span style={{ width: 32, height: 8, background: '#aab4c0', borderRadius: kind === 'pills' ? 999 : 2 }} />
                    <span style={{ width: 32, height: 8, background: '#aab4c0', borderRadius: kind === 'pills' ? 999 : 2 }} />
                  </div>
                  <p style={{ margin: '6px 0 0', fontWeight: 600 }}>{kind === 'vertical' ? 'Vertical' : 'Pills'}</p>
                </button>
              ))}
            </div>
          </Field>

          <Section title="Output data" />
          <Field label="Filter output">
            <input value={outputName} onChange={(event) => onRenameOutput(event.target.value)} style={inputStyle()} />
          </Field>

          <button type="button" onClick={onDelete} className="of-button" style={{ color: 'var(--status-danger)', borderColor: '#fecaca' }}>
            <Glyph name="trash" size={12} /> Delete widget
          </button>
        </div>
      ) : (
        <div style={{ padding: 14 }}><p className="of-text-muted" style={{ fontSize: 12 }}>{tab === 'metadata' ? 'Widget metadata' : 'Display options'} coming soon.</p></div>
      )}
    </div>
  );
}

function VariablesPanel({
  variables,
  widgets,
  addMenuOpen,
  onToggleAdd,
  onAdd,
  onRename,
  onSelect,
  onDelete,
}: {
  variables: WorkshopVariable[];
  widgets: AppWidget[];
  addMenuOpen: boolean;
  onToggleAdd: () => void;
  onAdd: (variable: WorkshopVariable) => void;
  onRename: (variableId: string, name: string) => void;
  onSelect: (variableId: string) => void;
  onDelete: (variableId: string) => void;
}) {
  function usedInCount(variableId: string) {
    let count = 0;
    for (const section of widgets) {
      for (const widget of section.children) {
        if ((widget.props as { source_variable_id?: string })?.source_variable_id === variableId) count += 1;
      }
    }
    return count;
  }
  return (
    <div style={{ display: 'grid', gap: 6 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', position: 'relative' }}>
        <p style={{ margin: 0, fontSize: 13, fontWeight: 600 }}>Variables ({variables.length})</p>
        <button type="button" onClick={onToggleAdd} className="of-button of-button--ghost" aria-label="Add variable" style={{ padding: 4 }}>
          <Glyph name="plus" size={14} />
        </button>
        {addMenuOpen ? (
          <div role="menu" style={{ position: 'absolute', top: '100%', right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 4, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.16)', padding: 6, zIndex: 5, minWidth: 240 }}>
            <p className="of-text-muted" style={{ margin: '4px 6px', fontSize: 11, fontWeight: 700, letterSpacing: '0.05em' }}>OBJECT SET</p>
            <button
              type="button"
              onClick={() => onAdd({ id: makeId('var'), kind: 'object_set_definition', name: 'New object set', object_type_id: '' })}
              style={addWidgetItemStyle()}
            >
              <Glyph name="cube" size={13} tone="#2d72d2" />
              <span style={{ display: 'grid', gap: 2 }}>
                <strong style={{ fontSize: 13 }}>Object set definition</strong>
                <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>Define an object set with filters and linked object traversals.</span>
              </span>
            </button>
            <button
              type="button"
              onClick={() => onAdd({ id: makeId('var'), kind: 'object_set_filter', name: 'New object set filter', object_type_id: '', static_filters: [] })}
              style={addWidgetItemStyle()}
            >
              <Glyph name="list" size={13} tone="#7c5dd6" />
              <span style={{ display: 'grid', gap: 2 }}>
                <strong style={{ fontSize: 13 }}>Object set filter</strong>
                <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>Store filter state that can be applied to object sets.</span>
              </span>
            </button>
            <p className="of-text-muted" style={{ margin: '8px 6px 4px', fontSize: 11, fontWeight: 700, letterSpacing: '0.05em' }}>VALUE</p>
            <button
              type="button"
              onClick={() => onAdd({ id: makeId('var'), kind: 'string', name: 'New string', object_type_id: '', default_value: '' })}
              style={addWidgetItemStyle()}
            >
              <Glyph name="tag" size={13} tone="#0891b2" />
              <span style={{ display: 'grid', gap: 2 }}>
                <strong style={{ fontSize: 13 }}>Primitive value</strong>
                <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>Text, numeric, boolean, date, array, or struct value.</span>
              </span>
            </button>
            <button
              type="button"
              onClick={() => onAdd({ id: makeId('var'), kind: 'url_parameter', name: 'URL parameter', object_type_id: '', metadata: { parameter_name: 'param' } })}
              style={addWidgetItemStyle()}
            >
              <Glyph name="external-link" size={13} tone="#15803d" />
              <span style={{ display: 'grid', gap: 2 }}>
                <strong style={{ fontSize: 13 }}>URL/runtime parameter</strong>
                <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>Read state from app URL or runtime parameters.</span>
              </span>
            </button>
            <button
              type="button"
              onClick={() => onAdd({ id: makeId('var'), kind: 'aggregation', name: 'New aggregation', object_type_id: '', metadata: { metric: 'count', source_variable_id: '' } })}
              style={addWidgetItemStyle()}
            >
              <Glyph name="graph" size={13} tone="#7c5dd6" />
              <span style={{ display: 'grid', gap: 2 }}>
                <strong style={{ fontSize: 13 }}>Aggregation</strong>
                <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>Count or aggregate an object set variable.</span>
              </span>
            </button>
          </div>
        ) : null}
      </div>
      <input type="search" placeholder="Search…" style={{ padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 13, width: '100%' }} />
      <div style={{ display: 'grid', gap: 4 }}>
        {variables.length === 0 ? (
          <p className="of-text-muted" style={{ fontSize: 12, padding: 8 }}>No variables yet.</p>
        ) : (
          variables.map((variable) => (
            <div
              key={variable.id}
              style={{ display: 'grid', gap: 4, padding: '6px 8px', border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#fff' }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <Glyph name={variable.kind === 'filter_output' ? 'list' : 'cube'} size={13} tone={variable.kind === 'filter_output' ? '#7c5dd6' : '#2d72d2'} />
                <input
                  value={variable.name}
                  onChange={(event) => onRename(variable.id, event.target.value)}
                  style={{ flex: 1, border: 0, background: 'transparent', outline: 'none', fontSize: 13, fontWeight: 600 }}
                />
                {variable.kind === 'object_set_definition' ? (
                  <button type="button" aria-label="Edit definition" onClick={() => onSelect(variable.id)} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--text-muted)' }}>
                    <Glyph name="pencil" size={12} />
                  </button>
                ) : null}
                <button type="button" aria-label="Delete" onClick={() => onDelete(variable.id)} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--status-danger)' }}>
                  <Glyph name="x" size={11} />
                </button>
              </div>
              <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
                {VARIABLE_KIND_LABEL[variable.kind]} · Used in {usedInCount(variable.id)} widget{usedInCount(variable.id) === 1 ? '' : 's'}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function ObjectSetDefinitionEditor({
  variable,
  variables,
  objectTypes,
  onClose,
  onChange,
}: {
  variable: WorkshopVariable | null;
  variables: WorkshopVariable[];
  objectTypes: ObjectType[];
  onClose: () => void;
  onChange: (next: WorkshopVariable) => void;
}) {
  const [filterMenuOpen, setFilterMenuOpen] = useState(false);
  if (!variable) return null;
  return (
    <aside
      style={{
        position: 'fixed',
        top: 56,
        left: 56,
        width: 460,
        maxHeight: 'calc(100vh - 100px)',
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 6,
        boxShadow: '0 12px 32px rgba(15, 23, 42, 0.12)',
        zIndex: 80,
        display: 'grid',
        gridTemplateRows: 'auto 1fr',
      }}
    >
      <header style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <Glyph name="cube" size={14} tone="#2d72d2" />
        <input
          value={variable.name}
          onChange={(event) => onChange({ ...variable, name: event.target.value })}
          style={{ flex: 1, border: 0, outline: 'none', fontSize: 14, fontWeight: 600 }}
        />
        <button type="button" aria-label="Close" onClick={onClose} className="of-button of-button--ghost" style={{ padding: 4 }}>
          <Glyph name="x" size={12} />
        </button>
      </header>
      <div style={{ padding: 14, display: 'grid', gap: 14, overflowY: 'auto' }}>
        <Field label="Starting object set">
          <select
            value={variable.object_type_id}
            onChange={(event) => onChange({ ...variable, object_type_id: event.target.value })}
            style={inputStyle()}
          >
            <option value="">Select object type…</option>
            {objectTypes.map((type) => (
              <option key={type.id} value={type.id}>{type.display_name || type.name}</option>
            ))}
          </select>
        </Field>

        <div style={{ display: 'grid', gridTemplateColumns: '120px 1fr', gap: 10, alignItems: 'center', padding: '10px 12px', border: '1px solid var(--border-subtle)', borderRadius: 6 }}>
          <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>Filter…</span>
          <div style={{ display: 'grid', gap: 4 }}>
            <button type="button" disabled className="of-button" style={{ justifyContent: 'flex-start', fontSize: 12 }}>
              <Glyph name="plus" size={11} /> On a property
            </button>
            <div style={{ position: 'relative' }}>
              <button
                type="button"
                onClick={() => setFilterMenuOpen((open) => !open)}
                className="of-button"
                style={{ justifyContent: 'flex-start', fontSize: 12, width: '100%' }}
              >
                <span style={{ fontFamily: 'serif', fontStyle: 'italic', color: '#7c5dd6' }}>(x)</span> Using a variable
                {variable.filter_variable_id ? (
                  <span style={{ marginLeft: 'auto', fontSize: 11, color: 'var(--status-info)' }}>
                    {variables.find((v) => v.id === variable.filter_variable_id)?.name ?? ''}
                  </span>
                ) : null}
              </button>
              {filterMenuOpen ? (
                <div role="menu" style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 4, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.12)', padding: 4, zIndex: 5 }}>
                  {variables.filter((v) => v.kind === 'filter_output').length === 0 ? (
                    <p className="of-text-muted" style={{ padding: 6, fontSize: 12 }}>No filter outputs available.</p>
                  ) : (
                    variables.filter((v) => v.kind === 'filter_output').map((source) => (
                      <button
                        key={source.id}
                        type="button"
                        onClick={() => {
                          onChange({ ...variable, filter_variable_id: source.id });
                          setFilterMenuOpen(false);
                        }}
                        style={addWidgetItemStyle()}
                      >
                        <Glyph name="list" size={12} tone="#7c5dd6" /> {source.name}
                      </button>
                    ))
                  )}
                </div>
              ) : null}
            </div>
            <button type="button" disabled className="of-button" style={{ justifyContent: 'flex-start', fontSize: 12 }}>
              <Glyph name="link" size={11} /> On a link
            </button>
          </div>
          <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>Traverse to</span>
          <button type="button" disabled className="of-button" style={{ justifyContent: 'flex-start', fontSize: 12 }}>
            <Glyph name="graph" size={11} /> Get linked objects
          </button>
        </div>
        <button type="button" disabled className="of-button" style={{ justifyContent: 'flex-start', fontSize: 12 }}>
          <Glyph name="plus" size={11} /> Combine with another object set
        </button>
      </div>
    </aside>
  );
}


function SectionToolbar({ label, onAddSection, onSplit }: { label: string; onAddSection: () => void; onSplit: (direction: "above" | "below" | "left" | "right") => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 10px", background: "#fff", border: "1px solid var(--border-subtle)", borderRadius: 4, marginBottom: 12, position: "relative" }}>
      <span className="of-text-muted" style={{ fontSize: 11, fontWeight: 700, letterSpacing: "0.06em" }}>{label}</span>
      <button type="button" className="of-button" onClick={onAddSection} style={{ fontSize: 12 }}>
        <Glyph name="plus" size={12} /> Add section inside
      </button>
      <div style={{ width: 1, height: 18, background: "var(--border-subtle)", margin: "0 4px" }} />
      <span className="of-text-muted" style={{ fontSize: 11, fontWeight: 700, letterSpacing: "0.06em" }}>SPLIT CURRENT SECTION</span>
      <button type="button" aria-label="Split above" onClick={() => onSplit("above")} className="of-button of-button--ghost" style={{ padding: 4 }}><SplitGlyph dir="above" /></button>
      <button type="button" aria-label="Split below" onClick={() => onSplit("below")} className="of-button of-button--ghost" style={{ padding: 4 }}><SplitGlyph dir="below" /></button>
      <button type="button" aria-label="Split left" onClick={() => onSplit("left")} className="of-button of-button--ghost" style={{ padding: 4 }}><SplitGlyph dir="left" /></button>
      <button type="button" aria-label="Split right" onClick={() => onSplit("right")} className="of-button of-button--ghost" style={{ padding: 4 }}><SplitGlyph dir="right" /></button>
      <button type="button" className="of-button" onClick={() => setOpen((value) => !value)} style={{ fontSize: 12 }}>
        <Glyph name="move" size={12} /> Split section <Glyph name="chevron-down" size={11} />
      </button>
      {open ? (
        <div role="menu" style={{ position: "absolute", top: "100%", right: 0, background: "#fff", border: "1px solid var(--border-default)", borderRadius: 4, boxShadow: "0 8px 24px rgba(15, 23, 42, 0.12)", padding: 4, marginTop: 4, zIndex: 20, minWidth: 220 }}>
          {(["above", "below", "left", "right"] as const).map((dir) => (
            <button key={dir} type="button" onClick={() => { onSplit(dir); setOpen(false); }} style={addWidgetItemStyle()}>
              <SplitGlyph dir={dir} />
              New section on {dir === "above" ? "top" : dir === "below" ? "bottom" : dir}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function SplitGlyph({ dir }: { dir: "above" | "below" | "left" | "right" }) {
  const map = {
    above: { x: 4, y: 4, w: 16, h: 7 },
    below: { x: 4, y: 13, w: 16, h: 7 },
    left: { x: 4, y: 4, w: 7, h: 16 },
    right: { x: 13, y: 4, w: 7, h: 16 },
  } as const;
  const r = map[dir];
  return (
    <svg width={14} height={14} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="4" width="16" height="16" rx="1.5" stroke="#5c7080" strokeWidth="1.4" />
      <rect x={r.x} y={r.y} width={r.w} height={r.h} fill="#2d72d2" opacity="0.25" />
    </svg>
  );
}

export function ObjectSetTitleWidgetView({ widget, variables = [], objectTypes = [] }: { widget: AppWidget; variables?: WorkshopVariable[]; objectTypes?: ObjectType[] }) {
  const sourceVariableId = (widget.props as { source_variable_id?: string })?.source_variable_id ?? "";
  const variable = variables.find((v) => v.id === sourceVariableId) ?? null;
  const objectTypeId = variable?.object_type_id ?? "";
  const objectType = objectTypes.find((t) => t.id === objectTypeId);
  const runtime = useRuntime();
  const selectedObjects = variable?.kind === "object_set_selection" ? runtime.selectedObjectSets[sourceVariableId] ?? EMPTY_SELECTED_OBJECTS : null;
  const [count, setCount] = useState<number | null>(null);
  useEffect(() => {
    if (!objectTypeId) {
      setCount(null);
      return;
    }
    if (selectedObjects !== null) {
      setCount(selectedObjects.length);
      return;
    }
    let cancelled = false;
    void fetchObjectsForVariable(objectTypeId, variable, 5000, runtime.variableEngine)
      .then((response) => {
        if (!cancelled) setCount(response.total);
      })
      .catch(() => {
        if (!cancelled) setCount(null);
      });
    return () => {
      cancelled = true;
    };
  }, [objectTypeId, runtime.variableEngine, selectedObjects, variable]);
  if (!variable) {
    return <div style={{ padding: 12 }}><p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>Select an object set in the inspector.</p></div>;
  }
  return (
    <div style={{ padding: "10px 14px", display: "flex", alignItems: "center", gap: 8 }}>
      <Glyph name="cube" size={16} tone="#2d72d2" />
      <strong style={{ fontSize: 16 }}>{count === null ? "…" : count.toLocaleString()} {objectType?.display_name || objectType?.name || variable.name}</strong>
    </div>
  );
}

export function ButtonGroupWidgetView({ widget }: { widget: AppWidget }) {
  const buttons: ButtonGroupButton[] = ((widget.props as { buttons?: ButtonGroupButton[] })?.buttons) ?? [];
  const fillHorizontal = Boolean((widget.props as { fill_horizontal?: boolean })?.fill_horizontal);
  const orientation = (widget.props as { orientation?: "horizontal" | "vertical" })?.orientation ?? "horizontal";
  const runtime = useRuntime();
  return (
    <div style={{ padding: 10, display: orientation === "horizontal" ? "flex" : "grid", gap: 6 }}>
      {buttons.map((btn) => (
        <button
          key={btn.id}
          type="button"
          className="of-button"
          onClick={(event) => {
            if (runtime.preview) {
              event.stopPropagation();
              runtime.onButtonClick(btn);
            }
          }}
          style={{ flex: fillHorizontal ? 1 : "0 0 auto", justifyContent: "center", fontSize: 12 }}
        >
          {btn.label}
        </button>
      ))}
    </div>
  );
}

export function PropertyListWidgetView({ widget, variables }: { widget: AppWidget; variables: WorkshopVariable[] }) {
  const sourceVariableId = (widget.props as { source_variable_id?: string })?.source_variable_id ?? "";
  const variable = variables.find((v) => v.id === sourceVariableId) ?? null;
  const items: PropertyListItem[] = ((widget.props as { items?: PropertyListItem[] })?.items) ?? [];
  const numColumns = Number((widget.props as { number_of_columns?: number })?.number_of_columns ?? 2);
  const objectTypeId = variable?.object_type_id ?? "";
  const runtime = useRuntime();
  const [properties, setProperties] = useState<Property[]>([]);
  const [sample, setSample] = useState<ObjectInstance | null>(null);
  const selectedObjects = variable?.kind === "object_set_selection" ? runtime.variableEngine.getSelectedObjectSet(sourceVariableId) : null;
  useEffect(() => {
    if (!objectTypeId) {
      setProperties([]);
      setSample(null);
      return;
    }
    let cancelled = false;
    void Promise.all([listProperties(objectTypeId), fetchObjectsForVariable(objectTypeId, variable, 1, runtime.variableEngine)])
      .then(([propResponse, listResponse]) => {
        if (cancelled) return;
        setProperties(propResponse);
        setSample(listResponse.data[0] ?? null);
      })
      .catch(() => {
        if (cancelled) return;
        setProperties([]);
        setSample(null);
      });
    return () => { cancelled = true; };
  }, [objectTypeId, runtime.variableEngine, variable]);
  if (!variable) {
    return <div style={{ padding: 12 }}><p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>Select an object set in the inspector.</p></div>;
  }
  const allNames = items.flatMap((item) => item.property_names);
  const active = runtime.variableEngine.getActiveObject(sourceVariableId);
  const object = variable.kind === "object_set_active_object" ? active : variable.kind === "object_set_selection" ? selectedObjects?.[0] ?? null : sample;
  return (
    <div style={{ padding: 12 }}>
      <div style={{ display: "grid", gridTemplateColumns: `repeat(${Math.max(1, numColumns)}, minmax(0, 1fr))`, gap: "6px 18px" }}>
        {allNames.length === 0 ? (
          <p className="of-text-muted" style={{ margin: 0, fontSize: 12, gridColumn: "1 / -1" }}>No properties added. Use the inspector to add values.</p>
        ) : allNames.map((name) => {
          const property = properties.find((p) => p.name === name);
          const value = object ? String((object.properties as Record<string, unknown>)?.[name] ?? "") : "";
          return (
            <div key={name} style={{ display: "grid", gridTemplateColumns: "120px 1fr", gap: 8, alignItems: "center", fontSize: 12 }}>
              <span className="of-text-muted">{property?.display_name || name}</span>
              <span style={{ color: "var(--text-strong)" }}>{value || "—"}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function DetailWidgetInspector({
  widget,
  variables,
  onChange,
  onDelete,
}: {
  widget: AppWidget;
  variables: WorkshopVariable[];
  onChange: (next: AppWidget) => void;
  onDelete: () => void;
}) {
  const [tab, setTab] = useState<"setup" | "metadata" | "display">("setup");
  function patchProps(patch: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patch } });
  }
  const widgetTypeLabel = widget.widget_type === "object_set_title" ? "OBJECT SET TITLE" : widget.widget_type === "button_group" ? "BUTTON GROUP" : "PROPERTY LIST";
  return (
    <div style={inspectorStyle()}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "10px 14px", borderBottom: "1px solid var(--border-subtle)" }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>{widget.title}</span>
        <span className="of-text-muted" style={{ fontSize: 11, textTransform: "uppercase", letterSpacing: "0.06em" }}>{widgetTypeLabel}</span>
      </div>
      <div style={{ display: "flex", gap: 0, padding: "0 14px", borderBottom: "1px solid var(--border-subtle)" }}>
        {(["setup", "metadata", "display"] as const).map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => setTab(value)}
            style={{ padding: "8px 6px", border: 0, background: "transparent", borderBottom: tab === value ? "2px solid var(--status-info)" : "2px solid transparent", cursor: "pointer", fontSize: 12, fontWeight: tab === value ? 600 : 500, color: tab === value ? "var(--text-strong)" : "var(--text-muted)", marginRight: 14 }}
          >
            {value === "setup" ? "Widget setup" : value === "metadata" ? "Metadata" : "Display"}
          </button>
        ))}
      </div>
      {tab === "setup" ? (
        <div style={{ padding: 14, display: "grid", gap: 14 }}>
          {(widget.widget_type === "object_set_title" || widget.widget_type === "property_list") ? (
            <Field label="Input object set">
              <select
                value={(widget.props as { source_variable_id?: string })?.source_variable_id ?? ""}
                onChange={(event) => patchProps({ source_variable_id: event.target.value })}
                style={inputStyle()}
              >
                <option value="">Select object set variable…</option>
                {variables.map((v) => (
                  <option key={v.id} value={v.id}>{v.name} ({VARIABLE_KIND_LABEL[v.kind]})</option>
                ))}
              </select>
            </Field>
          ) : null}
          {widget.widget_type === "button_group" ? <ButtonGroupSetup widget={widget} variables={variables} onChange={onChange} /> : null}
          {widget.widget_type === "property_list" ? <PropertyListSetup widget={widget} variables={variables} onChange={onChange} /> : null}
          <button type="button" onClick={onDelete} className="of-button" style={{ color: "var(--status-danger)", borderColor: "#fecaca" }}>
            <Glyph name="trash" size={12} /> Delete widget
          </button>
        </div>
      ) : tab === "display" ? (
        <DisplayTab widget={widget} onChange={onChange} />
      ) : (
        <div style={{ padding: 14 }}><p className="of-text-muted" style={{ fontSize: 12 }}>Widget metadata coming soon.</p></div>
      )}
    </div>
  );
}

function ButtonGroupSetup({ widget, variables, onChange }: { widget: AppWidget; variables: WorkshopVariable[]; onChange: (next: AppWidget) => void }) {
  const buttons: ButtonGroupButton[] = ((widget.props as { buttons?: ButtonGroupButton[] })?.buttons) ?? [];
  const buttonType = ((widget.props as { button_type?: string })?.button_type) ?? "inline";
  const orientation = ((widget.props as { orientation?: "horizontal" | "vertical" })?.orientation) ?? "horizontal";
  const fillHorizontal = Boolean((widget.props as { fill_horizontal?: boolean })?.fill_horizontal);
  const [editingButtonId, setEditingButtonId] = useState<string | null>(null);
  function patch(patchObj: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patchObj } });
  }
  function patchButton(id: string, patchObj: Partial<ButtonGroupButton>) {
    patch({ buttons: buttons.map((b) => (b.id === id ? { ...b, ...patchObj } : b)) });
  }
  const editingButton = editingButtonId ? buttons.find((b) => b.id === editingButtonId) ?? null : null;

  if (editingButton) {
    return (
      <ButtonItemEditor
        button={editingButton}
        variables={variables}
        onBack={() => setEditingButtonId(null)}
        onChange={(next) => patchButton(editingButton.id, next)}
      />
    );
  }

  return (
    <>
      <Section title="Button type" />
      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 6 }}>
        {(["inline", "menu", "two-part"] as const).map((kind) => (
          <button
            key={kind}
            type="button"
            onClick={() => patch({ button_type: kind })}
            style={{ padding: "8px 4px", border: buttonType === kind ? "2px solid var(--status-info)" : "1px solid var(--border-default)", background: buttonType === kind ? "rgba(45, 114, 210, 0.06)" : "#fff", borderRadius: 4, cursor: "pointer", fontSize: 12 }}
          >
            {kind === "inline" ? "Inline" : kind === "menu" ? "Menu" : "Two-part"}
          </button>
        ))}
      </div>
      <Section title="Button configuration" />
      <div style={{ display: "grid", gap: 4 }}>
        {buttons.map((btn) => (
          <button
            key={btn.id}
            type="button"
            onClick={() => setEditingButtonId(btn.id)}
            style={{ display: "flex", gap: 6, alignItems: "center", padding: "8px 10px", background: "#f4f6f9", border: "1px solid var(--border-subtle)", borderRadius: 4, cursor: "pointer", textAlign: "left" }}
          >
            <Glyph name="move" size={12} tone="#aab4c0" />
            <span style={{ flex: 1, fontSize: 13, fontWeight: 500 }}>{btn.label}</span>
            <Glyph name="chevron-right" size={11} tone="#5c7080" />
          </button>
        ))}
        <button type="button" onClick={() => patch({ buttons: [...buttons, makeButton(`Button ${buttons.length + 1}`)] })} className="of-button" style={{ fontSize: 12, justifyContent: "center" }}>
          <Glyph name="plus" size={11} /> Add Button
        </button>
      </div>
      <Section title="Display & formatting" />
      <Field label="Orientation">
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
          {(["horizontal", "vertical"] as const).map((kind) => (
            <button
              key={kind}
              type="button"
              onClick={() => patch({ orientation: kind })}
              style={{ padding: "6px 8px", border: orientation === kind ? "2px solid var(--status-info)" : "1px solid var(--border-default)", background: orientation === kind ? "rgba(45, 114, 210, 0.06)" : "#fff", borderRadius: 4, cursor: "pointer", fontSize: 12 }}
            >
              {kind === "horizontal" ? "Horizontal" : "Vertical"}
            </button>
          ))}
        </div>
      </Field>
      <Toggle label="Fill available horizontal space in row and column layouts" value={fillHorizontal} onChange={(checked) => patch({ fill_horizontal: checked })} />
    </>
  );
}

function ButtonItemEditor({
  button,
  variables,
  onBack,
  onChange,
}: {
  button: ButtonGroupButton;
  variables: WorkshopVariable[];
  onBack: () => void;
  onChange: (next: Partial<ButtonGroupButton>) => void;
}) {
  const [actions, setActions] = useState<ActionType[]>([]);
  const [actionSearch, setActionSearch] = useState('');
  const [actionPickerOpen, setActionPickerOpen] = useState(false);
  const [selectedActionType, setSelectedActionType] = useState<ActionType | null>(null);
  const [selectedActionInputs, setSelectedActionInputs] = useState<ActionInputField[]>([]);
  const [editingParameter, setEditingParameter] = useState<string>('');

  useEffect(() => {
    if (!button.action_type_id) {
      setSelectedActionType(null);
      setSelectedActionInputs([]);
      return;
    }
    let cancelled = false;
    void getActionType(button.action_type_id)
      .then((action) => {
        if (cancelled) return;
        setSelectedActionType(action);
        setSelectedActionInputs(action.input_schema ?? []);
        if (!editingParameter && (action.input_schema?.length ?? 0) > 0) {
          const objectParam = (action.input_schema ?? []).find((field) => field.property_type === 'object_reference' || field.name.toLowerCase().includes('order') || field.name === 'object');
          setEditingParameter(objectParam?.name ?? action.input_schema![0].name);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setSelectedActionType(null);
          setSelectedActionInputs([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [button.action_type_id, editingParameter]);

  useEffect(() => {
    if (!actionPickerOpen) return;
    let cancelled = false;
    void listActionTypes({ per_page: 100, search: actionSearch || undefined }).then((response) => {
      if (!cancelled) setActions(response.data);
    }).catch(() => {
      if (!cancelled) setActions([]);
    });
    return () => {
      cancelled = true;
    };
  }, [actionPickerOpen, actionSearch]);

  function patchParameter(parameterName: string, patch: Partial<ButtonParameterDefault>) {
    const current = button.parameter_defaults[parameterName] ?? { kind: 'none' };
    onChange({
      parameter_defaults: { ...button.parameter_defaults, [parameterName]: { ...current, ...patch } as ButtonParameterDefault },
    });
  }

  const editingParam = selectedActionInputs.find((f) => f.name === editingParameter) ?? null;
  const editingDefault = button.parameter_defaults[editingParameter] ?? { kind: 'none' as const };

  return (
    <>
      <button
        type="button"
        onClick={onBack}
        style={{ display: 'inline-flex', alignItems: 'center', gap: 6, border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--status-info)', fontSize: 13, padding: 0, marginBottom: 6 }}
      >
        <Glyph name="chevron-left" size={11} /> {button.label}
      </button>

      <Field label="Text">
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <input
            value={button.label}
            onChange={(event) => onChange({ label: event.target.value })}
            style={{ ...inputStyle(), flex: 1 }}
          />
        </div>
        <button type="button" className="of-link" style={{ ...linkBtnStyle(), justifySelf: 'end' }}>Use variable</button>
      </Field>

      <Section title="Conditional visibility" />
      <Toggle label="Conditional visibility" value={button.conditional_visibility} onChange={(checked) => onChange({ conditional_visibility: checked })} />

      <Section title="On click" />
      <Field label="Action kind">
        <select
          value={button.on_click_kind}
          onChange={(event) => onChange({ on_click_kind: event.target.value as ButtonOnClickKind })}
          style={inputStyle()}
        >
          <option value="none">No action</option>
          <option value="action">Action</option>
          <option value="event">Event</option>
          <option value="export">Export data</option>
          <option value="url">Open URL</option>
        </select>
      </Field>

      {button.on_click_kind === 'action' ? (
        <>
          <div style={{ position: 'relative' }}>
            {selectedActionType ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f7f9fa' }}>
                <Glyph name="pencil" size={12} tone="#5c7080" />
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 13, fontWeight: 500 }}>{selectedActionType.display_name || selectedActionType.name}</div>
                  <div className="of-text-muted" style={{ fontSize: 11 }}>on {selectedActionType.object_type_id}</div>
                </div>
                <button type="button" aria-label="Info" className="of-button of-button--ghost" style={{ padding: 2 }}>
                  <Glyph name="info" size={11} />
                </button>
                <button type="button" aria-label="Edit" onClick={() => setActionPickerOpen(true)} className="of-button of-button--ghost" style={{ padding: 2 }}>
                  <Glyph name="chevron-down" size={11} />
                </button>
                <button type="button" aria-label="Clear" onClick={() => onChange({ action_type_id: '', parameter_defaults: {} })} className="of-button of-button--ghost" style={{ padding: 2 }}>
                  <Glyph name="x" size={11} />
                </button>
              </div>
            ) : (
              <button type="button" onClick={() => setActionPickerOpen(true)} style={{ width: '100%', display: 'flex', alignItems: 'center', gap: 6, padding: '8px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#fff', cursor: 'pointer', fontSize: 13 }}>
                <Glyph name="search" size={11} />
                <span style={{ flex: 1, color: 'var(--text-muted)', textAlign: 'left' }}>Select an Action…</span>
                <Glyph name="chevron-down" size={11} />
              </button>
            )}
            {actionPickerOpen ? (
              <div role="menu" style={{ position: 'absolute', top: 'calc(100% + 4px)', left: 0, right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 4, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.12)', padding: 6, zIndex: 6, maxHeight: 280, overflowY: 'auto' }}>
                <input
                  autoFocus
                  value={actionSearch}
                  onChange={(event) => setActionSearch(event.target.value)}
                  placeholder="Search actions…"
                  style={{ width: '100%', padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 13, marginBottom: 6 }}
                />
                {actions.length === 0 ? (
                  <p className="of-text-muted" style={{ padding: 8, fontSize: 12, margin: 0 }}>No actions match.</p>
                ) : actions.map((action) => (
                  <button
                    key={action.id}
                    type="button"
                    onClick={() => {
                      onChange({ action_type_id: action.id });
                      setActionPickerOpen(false);
                      setActionSearch('');
                    }}
                    style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '6px 10px', border: 0, background: 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13, borderRadius: 4 }}
                  >
                    <Glyph name="pencil" size={11} tone="#5c7080" />
                    <span style={{ flex: 1 }}>{action.display_name || action.name}</span>
                  </button>
                ))}
              </div>
            ) : null}
          </div>

          {selectedActionType ? (
            <>
              <Field label="Default layout">
                <div style={{ display: 'inline-flex', borderRadius: 4, overflow: 'hidden', border: '1px solid var(--border-default)' }}>
                  {(['form', 'table'] as const).map((kind) => (
                    <button
                      key={kind}
                      type="button"
                      onClick={() => onChange({ default_layout: kind })}
                      style={{ padding: '6px 14px', border: 0, background: button.default_layout === kind ? '#1c2127' : '#fff', color: button.default_layout === kind ? '#fff' : 'var(--text-strong)', cursor: 'pointer', fontSize: 12 }}
                    >
                      {kind === 'form' ? 'Form' : 'Table'}
                    </button>
                  ))}
                </div>
              </Field>
              <Section title="End-user features" />
              <Toggle label="Switch layout" value={button.switch_layout} onChange={(checked) => onChange({ switch_layout: checked })} />

              <Section title="Parameter defaults" />
              <p className="of-text-muted" style={{ margin: 0, fontSize: 11 }}>Local default values for parameters</p>
              <Field label="Select parameter to configure">
                <select
                  value={editingParameter}
                  onChange={(event) => setEditingParameter(event.target.value)}
                  style={inputStyle()}
                >
                  <option value="">Select parameter…</option>
                  {selectedActionInputs.map((input) => (
                    <option key={input.name} value={input.name}>{input.display_name || input.name}</option>
                  ))}
                </select>
              </Field>

              {editingParam ? (
                <>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f7f9fa' }}>
                    <Glyph name={editingParam.property_type === 'object_reference' ? 'cube' : 'tag'} size={12} tone="#2d72d2" />
                    <span style={{ flex: 1, fontSize: 13, fontWeight: 500 }}>{editingParam.display_name || editingParam.name}</span>
                    <button type="button" aria-label="Required indicator" style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--status-danger)' }}>*</button>
                    <button type="button" aria-label="Clear default" onClick={() => patchParameter(editingParam.name, { kind: 'none' })} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--text-muted)' }}><Glyph name="x" size={11} /></button>
                  </div>

                  <Field label="Local default value">
                    <select
                      value={editingDefault.kind === 'variable' ? `var:${editingDefault.variable_id ?? ''}` : editingDefault.kind === 'static' ? 'static' : editingDefault.kind === 'active_object' ? 'active_object' : 'none'}
                      onChange={(event) => {
                        const raw = event.target.value;
                        if (raw.startsWith('var:')) {
                          patchParameter(editingParam.name, { kind: 'variable', variable_id: raw.slice(4) });
                        } else if (raw === 'static') {
                          patchParameter(editingParam.name, { kind: 'static', static_value: '' });
                        } else if (raw === 'active_object') {
                          patchParameter(editingParam.name, { kind: 'active_object' });
                        } else {
                          patchParameter(editingParam.name, { kind: 'none' });
                        }
                      }}
                      style={inputStyle()}
                    >
                      <option value="none">No default</option>
                      <option value="static">Static value</option>
                      {variables.filter((v) => v.kind === 'object_set_active_object').map((v) => (
                        <option key={v.id} value={`var:${v.id}`}>{v.name}</option>
                      ))}
                      {variables.filter((v) => v.kind === 'object_set' || v.kind === 'object_set_definition' || v.kind === 'filter_output' || v.kind === 'object_set_selection').map((v) => (
                        <option key={v.id} value={`var:${v.id}`}>{v.name}</option>
                      ))}
                    </select>
                  </Field>
                  {editingDefault.kind === 'static' ? (
                    <Field label="Static value">
                      <input
                        value={editingDefault.static_value ?? ''}
                        onChange={(event) => patchParameter(editingParam.name, { static_value: event.target.value })}
                        style={inputStyle()}
                      />
                    </Field>
                  ) : null}
                  {editingDefault.kind === 'variable' ? (
                    <p className="of-text-muted" style={{ fontSize: 11, margin: 0 }}>Local override applied</p>
                  ) : null}

                  <Section title="Visibility in form" />
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 0 }}>
                    {(['visible', 'disabled', 'hidden'] as const).map((kind) => (
                      <button
                        key={kind}
                        type="button"
                        style={{ padding: '6px 8px', border: '1px solid var(--border-default)', background: kind === 'visible' ? '#fff' : '#f4f6f9', color: kind === 'visible' ? 'var(--text-strong)' : 'var(--text-muted)', cursor: 'pointer', fontSize: 12, fontWeight: kind === 'visible' ? 600 : 500 }}
                      >
                        {kind === 'visible' ? 'Visible' : kind === 'disabled' ? 'Disabled' : 'Hidden'}
                      </button>
                    ))}
                  </div>
                </>
              ) : null}
            </>
          ) : null}
        </>
      ) : null}
    </>
  );
}

function PropertyListSetup({ widget, variables, onChange }: { widget: AppWidget; variables: WorkshopVariable[]; onChange: (next: AppWidget) => void }) {
  const sourceVariableId = (widget.props as { source_variable_id?: string })?.source_variable_id ?? "";
  const variable = variables.find((v) => v.id === sourceVariableId) ?? null;
  const items: PropertyListItem[] = ((widget.props as { items?: PropertyListItem[] })?.items) ?? [];
  const numColumns = Number((widget.props as { number_of_columns?: number })?.number_of_columns ?? 2);
  const enableWrapping = Boolean((widget.props as { enable_value_wrapping?: boolean })?.enable_value_wrapping);
  const objectTypeId = variable?.object_type_id ?? "";
  const [properties, setProperties] = useState<Property[]>([]);
  const [search, setSearch] = useState("");
  const [addOpen, setAddOpen] = useState<string | null>(null);
  useEffect(() => {
    if (!objectTypeId) { setProperties([]); return; }
    let cancelled = false;
    void listProperties(objectTypeId).then((response) => { if (!cancelled) setProperties(response); }).catch(() => { if (!cancelled) setProperties([]); });
    return () => { cancelled = true; };
  }, [objectTypeId]);

  function patch(patchObj: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patchObj } });
  }

  function patchItem(id: string, names: string[]) {
    patch({ items: items.map((it) => (it.id === id ? { ...it, property_names: names } : it)) });
  }

  function addAllProperties(itemId: string) {
    patchItem(itemId, properties.map((p) => p.name));
  }

  function removeAll(itemId: string) {
    patchItem(itemId, []);
  }

  function addItem() {
    patch({ items: [...items, { id: makeId("item"), property_names: [] }] });
  }

  function removeItem(id: string) {
    patch({ items: items.filter((it) => it.id !== id) });
  }

  const filteredProps = properties.filter((p) => `${p.display_name} ${p.name}`.toLowerCase().includes(search.toLowerCase()));

  return (
    <>
      <Section title="Items" />
      <button type="button" onClick={addItem} className="of-button" style={{ fontSize: 12, justifyContent: "center" }}>
        <Glyph name="plus" size={11} /> Add Item
      </button>
      {items.map((item, index) => (
        <div key={item.id} style={{ border: "1px solid var(--border-subtle)", borderRadius: 4, padding: 10, display: "grid", gap: 6 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <span style={{ fontSize: 12, fontWeight: 600 }}>Item {index + 1}</span>
            {items.length > 1 ? (
              <button type="button" aria-label="Remove item" onClick={() => removeItem(item.id)} style={{ border: 0, background: "transparent", cursor: "pointer", color: "var(--status-danger)" }}><Glyph name="x" size={12} /></button>
            ) : null}
          </div>
          <span className="of-text-muted" style={{ fontSize: 11, textTransform: "uppercase", letterSpacing: "0.04em" }}>Properties</span>
          {item.property_names.map((name) => (
            <div key={name} style={{ display: "flex", alignItems: "center", gap: 6, padding: "4px 8px", border: "1px solid var(--border-subtle)", borderRadius: 4 }}>
              <Glyph name="tag" size={11} tone="#5c7080" />
              <span style={{ flex: 1, fontSize: 12 }}>{properties.find((p) => p.name === name)?.display_name || name}</span>
              <button type="button" aria-label="Remove" onClick={() => patchItem(item.id, item.property_names.filter((n) => n !== name))} style={{ border: 0, background: "transparent", cursor: "pointer", color: "var(--status-danger)" }}><Glyph name="trash" size={11} /></button>
            </div>
          ))}
          <div style={{ position: "relative" }}>
            <button type="button" onClick={() => setAddOpen(addOpen === item.id ? null : item.id)} className="of-button" style={{ fontSize: 12, justifyContent: "center", width: "100%" }}>
              <Glyph name="plus" size={11} /> Add value
            </button>
            {addOpen === item.id ? (
              <div role="menu" style={{ position: "absolute", top: "calc(100% + 4px)", left: 0, right: 0, background: "#fff", border: "1px solid var(--border-default)", borderRadius: 4, boxShadow: "0 8px 24px rgba(15, 23, 42, 0.12)", padding: 6, zIndex: 5 }}>
                <input autoFocus value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search property…" style={{ width: "100%", padding: "6px 10px", border: "1px solid var(--border-default)", borderRadius: 4, fontSize: 13, marginBottom: 6 }} />
                <p className="of-text-muted" style={{ margin: "4px 6px", fontSize: 11, textTransform: "uppercase", letterSpacing: "0.05em" }}>Current object ({filteredProps.length})</p>
                {filteredProps.map((p) => (
                  <button key={p.id} type="button" onClick={() => { if (!item.property_names.includes(p.name)) patchItem(item.id, [...item.property_names, p.name]); setAddOpen(null); setSearch(""); }} style={addWidgetItemStyle()}>
                    <Glyph name="tag" size={11} tone="#5c7080" /> {p.display_name || p.name}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
          <div style={{ display: "flex", justifyContent: "space-between" }}>
            <button type="button" onClick={() => addAllProperties(item.id)} className="of-link" style={linkBtnStyle()}>Add all properties</button>
            <button type="button" onClick={() => removeAll(item.id)} className="of-link" style={{ ...linkBtnStyle(), color: "var(--status-danger)" }}>Remove all properties</button>
          </div>
        </div>
      ))}
      <Section title="Number of columns" />
      <input type="number" min={1} max={6} value={numColumns} onChange={(event) => patch({ number_of_columns: Number(event.target.value) })} style={inputStyle()} />
      <Toggle label="Enable value wrapping" value={enableWrapping} onChange={(checked) => patch({ enable_value_wrapping: checked })} />
    </>
  );
}

function DisplayTab({ widget, onChange }: { widget: AppWidget; onChange: (next: AppWidget) => void }) {
  const heightKind = ((widget.props as { row_height_kind?: string })?.row_height_kind) ?? "auto";
  const heightValue = Number((widget.props as { row_height_value?: number })?.row_height_value ?? 600);
  const overrideWidth = Boolean((widget.props as { override_section_width?: boolean })?.override_section_width);
  function patch(patchObj: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patchObj } });
  }
  return (
    <div style={{ padding: 14, display: "grid", gap: 14 }}>
      <Section title="Dimensions" />
      <Field label="Row height">
        <div style={{ display: "flex", alignItems: "center", gap: 0 }}>
          {(["auto", "absolute", "flex"] as const).map((kind) => (
            <button
              key={kind}
              type="button"
              onClick={() => patch({ row_height_kind: kind })}
              style={{ padding: "6px 12px", border: "1px solid var(--border-default)", background: heightKind === kind ? "#1c2127" : "#fff", color: heightKind === kind ? "#fff" : "var(--text-strong)", cursor: "pointer", fontSize: 12 }}
            >
              {kind === "auto" ? "Auto (max)" : kind === "absolute" ? "Absolute" : "Flex"}
            </button>
          ))}
          <input
            type="number"
            min={1}
            value={heightValue}
            onChange={(event) => patch({ row_height_value: Number(event.target.value) })}
            style={{ ...inputStyle(), width: 100, marginLeft: 8 }}
          />
        </div>
      </Field>
      <Toggle label="Override section width" value={overrideWidth} onChange={(checked) => patch({ override_section_width: checked })} />
    </div>
  );
}



function SectionHeaderRender({ section }: { section: AppWidget }) {
  const headerEnabled = (section.props as { header_enabled?: boolean })?.header_enabled !== false;
  if (!headerEnabled) return null;
  const styleKind = ((section.props as { style?: string })?.style) ?? "subheader";
  const iconName = (section.props as { icon?: string })?.icon ?? "";
  const headerFormat = ((section.props as { header_format?: string })?.header_format) ?? "title";
  const backgroundColorId = ((section.props as { background_color?: string })?.background_color) ?? "white";
  const backgroundHex = SECTION_BG_COLORS.find((option) => option.id === backgroundColorId)?.hex ?? "#ffffff";
  const styleMap: Record<string, { fontSize: number; fontWeight: number; padding?: string }> = {
    header: { fontSize: 18, fontWeight: 700 },
    title: { fontSize: 16, fontWeight: 600 },
    subheader: { fontSize: 13, fontWeight: 600 },
    caption: { fontSize: 12, fontWeight: 500 },
  };
  const sty = styleMap[styleKind] ?? styleMap.subheader;
  const containerStyle: React.CSSProperties =
    headerFormat === "contained"
      ? { display: "flex", alignItems: "center", gap: 8, padding: "8px 10px", background: backgroundHex, border: "1px solid var(--border-subtle)", borderRadius: 4 }
      : headerFormat === "underline"
      ? { display: "flex", alignItems: "center", gap: 8, padding: "0 0 6px", borderBottom: "2px solid var(--border-default)" }
      : { display: "flex", alignItems: "center", gap: 8 };
  return (
    <div style={containerStyle}>
      {iconName ? (
        <span style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", width: 22, height: 22, borderRadius: 4, background: "rgba(45, 114, 210, 0.08)" }}>
          <Glyph name={iconName as GlyphName} size={13} tone="#2d72d2" />
        </span>
      ) : null}
      <span style={{ margin: 0, fontSize: sty.fontSize, fontWeight: sty.fontWeight, color: "var(--text-strong)" }}>{section.title}</span>
    </div>
  );
}



function LayoutPreviewGlyph({ kind }: { kind: "columns" | "rows" | "tabs" | "flow" | "toolbar" | "loop" }) {
  const stroke = "#5c7080";
  return (
    <svg width={28} height={20} viewBox="0 0 36 24" aria-hidden="true">
      <rect x="2" y="2" width="32" height="20" rx="2" fill="none" stroke={stroke} strokeWidth="1.4" />
      {kind === "columns" ? (
        <>
          <rect x="5" y="6" width="11" height="12" rx="1" fill={stroke} opacity="0.25" />
          <rect x="20" y="6" width="11" height="12" rx="1" fill={stroke} opacity="0.25" />
        </>
      ) : null}
      {kind === "rows" ? (
        <>
          <rect x="5" y="5" width="26" height="6" rx="1" fill={stroke} opacity="0.25" />
          <rect x="5" y="13" width="26" height="6" rx="1" fill={stroke} opacity="0.25" />
        </>
      ) : null}
      {kind === "tabs" ? (
        <>
          <rect x="2" y="2" width="10" height="4" fill={stroke} opacity="0.4" />
          <rect x="2" y="6" width="32" height="16" fill="none" stroke={stroke} strokeWidth="1.4" />
        </>
      ) : null}
      {kind === "flow" ? (
        <>
          <rect x="5" y="5" width="6" height="6" rx="1" fill={stroke} opacity="0.25" />
          <rect x="13" y="5" width="6" height="6" rx="1" fill={stroke} opacity="0.25" />
          <rect x="21" y="5" width="6" height="6" rx="1" fill={stroke} opacity="0.25" />
          <rect x="5" y="13" width="6" height="6" rx="1" fill={stroke} opacity="0.25" />
          <rect x="13" y="13" width="6" height="6" rx="1" fill={stroke} opacity="0.25" />
        </>
      ) : null}
      {kind === "toolbar" ? (
        <>
          <rect x="5" y="5" width="6" height="3" rx="1" fill={stroke} opacity="0.4" />
          <rect x="13" y="5" width="6" height="3" rx="1" fill={stroke} opacity="0.4" />
          <rect x="5" y="11" width="26" height="8" rx="1" fill={stroke} opacity="0.18" />
        </>
      ) : null}
      {kind === "loop" ? (
        <>
          <rect x="5" y="5" width="26" height="4" rx="1" fill={stroke} opacity="0.18" />
          <rect x="5" y="11" width="26" height="4" rx="1" fill={stroke} opacity="0.18" />
          <rect x="5" y="17" width="26" height="2" rx="1" fill={stroke} opacity="0.18" />
        </>
      ) : null}
    </svg>
  );
}

const PIE_PADDING_PX: Record<string, number> = { none: 0, compact: 6, normal: 14, large: 24 };
const PIE_PALETTE = ['#2d72d2', '#cf923f', '#15803d', '#b42318', '#7c5dd6', '#5c7080', '#0d9488', '#db2777', '#ca8a04', '#1f4ea0'];

function readPieProps(widget: AppWidget) {
  const p = widget.props as Record<string, unknown>;
  return {
    sourceVariableId: (p.source_variable_id as string) ?? '',
    objectTypeId: (p.object_type_id as string) ?? '',
    groupBy: (p.group_by_property as string) ?? '',
    enableColors: p.enable_ontology_colors !== false,
    metric: ((p.aggregation_metric as string) ?? 'count') as 'count' | 'sum' | 'avg' | 'min' | 'max',
    metricProperty: (p.aggregation_property as string) ?? '',
    enableNumeric: Boolean(p.enable_numeric_formatting),
    radius: Number(p.radius ?? 0),
    padding: ((p.padding as string) ?? 'large') as 'none' | 'compact' | 'normal' | 'large',
    showLegend: p.show_legend !== false,
    legendPosition: ((p.legend_position as string) ?? 'next-to') as 'inside' | 'next-to',
    legendAnchor: ((p.legend_anchor as string) ?? 'right') as 'left' | 'right' | 'top' | 'bottom',
  };
}

export function ChartPieWidgetView({ widget, variables }: { widget: AppWidget; variables: WorkshopVariable[] }) {
  const cfg = readPieProps(widget);
  const sourceVariable = variables.find((v) => v.id === cfg.sourceVariableId) ?? null;
  const objectTypeId = sourceVariable?.object_type_id ?? cfg.objectTypeId ?? '';
  const [rows, setRows] = useState<ObjectInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const runtime = useRuntime();

  useEffect(() => {
    if (!objectTypeId || !cfg.groupBy) {
      setRows([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    void fetchObjectsForVariable(objectTypeId, sourceVariable, 5000, runtime.variableEngine)
      .then((response) => {
        if (cancelled) return;
        setRows(response.data);
      })
      .catch(() => {
        if (cancelled) return;
        setRows([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [objectTypeId, cfg.groupBy, runtime.variableEngine, sourceVariable]);

  const data = useMemo(() => {
    if (!cfg.groupBy) return [] as Array<{ name: string; value: number }>;
    const buckets = new Map<string, number>();
    for (const row of rows) {
      const props = (row.properties as Record<string, unknown>) ?? {};
      const rawKey = props[cfg.groupBy];
      const key = rawKey == null || rawKey === '' ? 'No value' : String(rawKey);
      let increment = 1;
      if (cfg.metric !== 'count' && cfg.metricProperty) {
        const num = Number(props[cfg.metricProperty]);
        if (!Number.isFinite(num)) continue;
        increment = num;
      }
      const previous = buckets.get(key) ?? 0;
      if (cfg.metric === 'count' || cfg.metric === 'sum') {
        buckets.set(key, previous + increment);
      } else if (cfg.metric === 'avg') {
        buckets.set(key, previous + increment);
      } else if (cfg.metric === 'min') {
        buckets.set(key, previous === 0 ? increment : Math.min(previous, increment));
      } else if (cfg.metric === 'max') {
        buckets.set(key, Math.max(previous, increment));
      }
    }
    return Array.from(buckets.entries()).map(([name, value]) => ({ name, value }));
  }, [rows, cfg.groupBy, cfg.metric, cfg.metricProperty]);

  const padPx = PIE_PADDING_PX[cfg.padding] ?? 14;
  const innerRadiusPercent = Math.min(99, Math.max(0, Math.round(cfg.radius)));
  const outerRadius = '70%';
  const innerRadius = `${Math.round(innerRadiusPercent * 0.7)}%`;

  const legendOption = !cfg.showLegend
    ? null
    : cfg.legendPosition === 'inside'
    ? { show: true, orient: 'vertical', left: 'center', top: 'center', textStyle: { fontSize: 11 } }
    : {
        show: true,
        orient: cfg.legendAnchor === 'top' || cfg.legendAnchor === 'bottom' ? 'horizontal' : 'vertical',
        left: cfg.legendAnchor === 'left' ? 8 : cfg.legendAnchor === 'right' ? 'right' : 'center',
        top: cfg.legendAnchor === 'top' ? 8 : cfg.legendAnchor === 'bottom' ? 'bottom' : 'middle',
        textStyle: { fontSize: 11 },
      };

  const echartsOption = useMemo(() => ({
    color: PIE_PALETTE,
    tooltip: { trigger: 'item' },
    legend: legendOption ?? { show: false },
    series: [
      {
        type: 'pie',
        radius: innerRadiusPercent === 0 ? outerRadius : [innerRadius, outerRadius],
        center: ['50%', '50%'],
        avoidLabelOverlap: true,
        label: { show: false },
        labelLine: { show: false },
        data,
      },
    ],
  }), [data, innerRadiusPercent, innerRadius, legendOption]);

  if (!objectTypeId) {
    return (
      <div style={{ padding: '36px 24px', textAlign: 'center' }}>
        <Glyph name="pie-chart" size={32} tone="#cf923f" />
        <p className="of-text-muted" style={{ margin: '8px 0 0', fontSize: 13 }}>Pick an Input Object Set in the inspector to render this chart.</p>
      </div>
    );
  }
  if (!cfg.groupBy) {
    return (
      <div style={{ padding: '36px 24px', textAlign: 'center' }}>
        <Glyph name="pie-chart" size={32} tone="#cf923f" />
        <p className="of-text-muted" style={{ margin: '8px 0 0', fontSize: 13 }}>Choose a property to Group By in the inspector.</p>
      </div>
    );
  }

  return (
    <div style={{ padding: padPx }}>
      {loading ? (
        <p className="of-text-muted" style={{ margin: 0, fontSize: 12, textAlign: 'center', padding: 24 }}>Loading…</p>
      ) : data.length === 0 ? (
        <p className="of-text-muted" style={{ margin: 0, fontSize: 12, textAlign: 'center', padding: 24 }}>No data to display.</p>
      ) : (
        <EChartCanvas options={echartsOption} style={{ height: 280 }} />
      )}
    </div>
  );
}

function ChartPieInspector({
  widget,
  variables,
  objectTypes,
  onChange,
  onDelete,
}: {
  widget: AppWidget;
  variables: WorkshopVariable[];
  objectTypes: ObjectType[];
  onChange: (next: AppWidget) => void;
  onDelete: () => void;
}) {
  const [tab, setTab] = useState<'setup' | 'metadata' | 'display'>('setup');
  const cfg = readPieProps(widget);
  const sourceVariable = variables.find((v) => v.id === cfg.sourceVariableId) ?? null;
  const objectTypeId = sourceVariable?.object_type_id ?? cfg.objectTypeId ?? '';
  const objectType = objectTypes.find((entry) => entry.id === objectTypeId) ?? null;
  const [properties, setProperties] = useState<Property[]>([]);
  const [pickerOpen, setPickerOpen] = useState(false);

  useEffect(() => {
    if (!objectTypeId) {
      setProperties([]);
      return;
    }
    let cancelled = false;
    void listProperties(objectTypeId)
      .then((response) => {
        if (!cancelled) setProperties(response);
      })
      .catch(() => {
        if (!cancelled) setProperties([]);
      });
    return () => {
      cancelled = true;
    };
  }, [objectTypeId]);

  function patch(patchObj: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patchObj } });
  }

  const inputCount = properties.length > 0 ? widget.props : {};
  void inputCount;
  const numericProperties = properties.filter((p) => ['number', 'integer', 'float', 'double', 'decimal'].includes(String(p.property_type).toLowerCase()));

  return (
    <div style={inspectorStyle()}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>{widget.title}</span>
        <span className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.06em' }}>CHART: PIE</span>
      </div>
      <div style={{ display: 'flex', gap: 0, padding: '0 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        {(['setup', 'metadata', 'display'] as const).map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => setTab(value)}
            style={{ padding: '8px 6px', border: 0, background: 'transparent', borderBottom: tab === value ? '2px solid var(--status-info)' : '2px solid transparent', cursor: 'pointer', fontSize: 12, fontWeight: tab === value ? 600 : 500, color: tab === value ? 'var(--text-strong)' : 'var(--text-muted)', marginRight: 14 }}
          >
            {value === 'setup' ? 'Widget setup' : value === 'metadata' ? 'Metadata' : 'Display'}
          </button>
        ))}
      </div>
      {tab === 'setup' ? (
        <div style={{ padding: 14, display: 'grid', gap: 14 }}>
          <Section title="Input object set" />
          <Field label="Source">
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#f7f9fa' }}>
              <Glyph name="cube" size={13} tone="#2d72d2" />
              <span style={{ flex: 1, fontSize: 13 }}>{sourceVariable ? sourceVariable.name : objectType ? objectType.display_name || objectType.name : 'Select object set…'}</span>
              <button type="button" aria-label="Edit" onClick={() => setPickerOpen(true)} className="of-button of-button--ghost" style={{ padding: 2 }}>
                <Glyph name="pencil" size={11} />
              </button>
              {(cfg.sourceVariableId || cfg.objectTypeId) ? (
                <button type="button" aria-label="Clear" onClick={() => patch({ source_variable_id: '', object_type_id: '', group_by_property: '', aggregation_property: '' })} className="of-button of-button--ghost" style={{ padding: 2 }}>
                  <Glyph name="x" size={11} />
                </button>
              ) : null}
            </div>
            <span className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>Current value: {sourceVariable ? sourceVariable.name : objectType ? objectType.display_name || objectType.name : 'undefined'}</span>
          </Field>

          <Section title="Group by" />
          <Field label="Property">
            <select value={cfg.groupBy} onChange={(event) => patch({ group_by_property: event.target.value })} style={inputStyle()}>
              <option value="">Select a property…</option>
              {properties.map((p) => (
                <option key={p.id} value={p.name}>{p.display_name || p.name}</option>
              ))}
            </select>
          </Field>
          <Toggle label="Enable ontology colors" value={cfg.enableColors} onChange={(checked) => patch({ enable_ontology_colors: checked })} />

          <Section title="Aggregation" />
          <Field label="Metric">
            <select value={cfg.metric} onChange={(event) => patch({ aggregation_metric: event.target.value })} style={inputStyle()}>
              <option value="count">Count</option>
              <option value="sum">Sum</option>
              <option value="avg">Average</option>
              <option value="min">Min</option>
              <option value="max">Max</option>
            </select>
          </Field>
          {cfg.metric !== 'count' ? (
            <Field label="Metric property">
              <select value={cfg.metricProperty} onChange={(event) => patch({ aggregation_property: event.target.value })} style={inputStyle()}>
                <option value="">Select a numeric property…</option>
                {numericProperties.map((p) => (
                  <option key={p.id} value={p.name}>{p.display_name || p.name}</option>
                ))}
              </select>
            </Field>
          ) : null}
          <Toggle label="Enable numeric formatting" value={cfg.enableNumeric} onChange={(checked) => patch({ enable_numeric_formatting: checked })} />

          <Section title="Radius" />
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <input type="range" min={0} max={99} value={cfg.radius} onChange={(event) => patch({ radius: Number(event.target.value) })} style={{ flex: 1 }} />
            <span style={{ fontSize: 12, color: 'var(--text-strong)', minWidth: 36, textAlign: 'right' }}>{cfg.radius}%</span>
          </div>

          <Section title="Padding" />
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 0 }}>
            {(['none', 'compact', 'normal', 'large'] as const).map((kind) => (
              <button
                key={kind}
                type="button"
                onClick={() => patch({ padding: kind })}
                style={{ padding: '6px 8px', border: '1px solid var(--border-default)', background: cfg.padding === kind ? '#fff' : '#f4f6f9', color: cfg.padding === kind ? 'var(--text-strong)' : 'var(--text-muted)', cursor: 'pointer', fontSize: 12, fontWeight: cfg.padding === kind ? 600 : 500 }}
              >
                {kind === 'none' ? 'None' : kind === 'compact' ? 'Compact' : kind === 'normal' ? 'Normal' : 'Large'}
              </button>
            ))}
          </div>

          <Section title="Legend" />
          <Toggle label="Show legend" value={cfg.showLegend} onChange={(checked) => patch({ show_legend: checked })} />
          {cfg.showLegend ? (
            <>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 0 }}>
                {(['inside', 'next-to'] as const).map((kind) => (
                  <button
                    key={kind}
                    type="button"
                    onClick={() => patch({ legend_position: kind })}
                    style={{ padding: '6px 8px', border: '1px solid var(--border-default)', background: cfg.legendPosition === kind ? '#fff' : '#f4f6f9', color: cfg.legendPosition === kind ? 'var(--text-strong)' : 'var(--text-muted)', cursor: 'pointer', fontSize: 12, fontWeight: cfg.legendPosition === kind ? 600 : 500 }}
                  >
                    {kind === 'inside' ? 'Inside chart' : 'Next to chart'}
                  </button>
                ))}
              </div>
              {cfg.legendPosition === 'next-to' ? (
                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 0 }}>
                  {(['left', 'right', 'top', 'bottom'] as const).map((kind) => (
                    <button
                      key={kind}
                      type="button"
                      onClick={() => patch({ legend_anchor: kind })}
                      style={{ padding: '6px 8px', border: '1px solid var(--border-default)', background: cfg.legendAnchor === kind ? '#fff' : '#f4f6f9', color: cfg.legendAnchor === kind ? 'var(--text-strong)' : 'var(--text-muted)', cursor: 'pointer', fontSize: 12, fontWeight: cfg.legendAnchor === kind ? 600 : 500 }}
                    >
                      {kind[0].toUpperCase() + kind.slice(1)}
                    </button>
                  ))}
                </div>
              ) : null}
            </>
          ) : null}

          <button type="button" onClick={onDelete} className="of-button" style={{ color: 'var(--status-danger)', borderColor: '#fecaca' }}>
            <Glyph name="trash" size={12} /> Delete widget
          </button>
        </div>
      ) : tab === 'display' ? (
        <DisplayTab widget={widget} onChange={onChange} />
      ) : (
        <div style={{ padding: 14 }}><p className="of-text-muted" style={{ fontSize: 12 }}>Widget metadata coming soon.</p></div>
      )}
      {pickerOpen ? (
        <ObjectSetPicker
          objectTypes={objectTypes}
          onClose={() => setPickerOpen(false)}
          onSelect={(typeId) => {
            patch({ source_variable_id: '', object_type_id: typeId, group_by_property: '', aggregation_property: '' });
            setPickerOpen(false);
          }}
        />
      ) : null}
    </div>
  );
}

function ChartXyGlyph() {
  return (
    <svg width={13} height={13} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <line x1="3" y1="20" x2="21" y2="20" stroke="#5c7080" strokeWidth="1.5" />
      <line x1="3" y1="20" x2="3" y2="4" stroke="#5c7080" strokeWidth="1.5" />
      <rect x="6" y="10" width="3" height="9" fill="#2d72d2" />
      <rect x="11" y="6" width="3" height="13" fill="#cf923f" />
      <rect x="16" y="13" width="3" height="6" fill="#2d72d2" />
    </svg>
  );
}

const XY_PALETTE = ['#2d72d2', '#cf923f', '#15803d', '#b42318', '#7c5dd6', '#5c7080', '#0d9488', '#db2777', '#ca8a04', '#1f4ea0'];

function readXyConfig(widget: AppWidget) {
  const p = widget.props as Record<string, unknown>;
  const layers = (p.layers as ChartXyLayer[] | undefined) ?? [];
  return {
    layers,
    yAxisKind: ((p.y_axis_kind as string) ?? 'categorical') as 'categorical' | 'continuous',
    showTitle: Boolean(p.show_title),
    showColorMarkers: p.show_color_markers !== false,
    enableNumericalFormatting: Boolean(p.enable_numerical_formatting),
    sortBy: ((p.sort_by as string) ?? 'key_asc') as 'key_asc' | 'key_desc' | 'value_asc' | 'value_desc',
    enableOntologyColors: p.enable_ontology_colors !== false,
    showLegend: Boolean(p.show_legend),
    showTooltips: p.show_tooltips !== false,
    allowExports: p.allow_exports !== false,
    barOrientation: ((p.bar_orientation as string) ?? 'horizontal') as 'horizontal' | 'vertical',
  };
}

export function ChartXyWidgetView({ widget, variables }: { widget: AppWidget; variables: WorkshopVariable[] }) {
  const cfg = readXyConfig(widget);
  const layer = cfg.layers[0] ?? null;
  const sourceVariable = layer ? variables.find((v) => v.id === layer.source_variable_id) ?? null : null;
  const objectTypeId = sourceVariable?.object_type_id ?? layer?.object_type_id ?? '';
  const [rows, setRows] = useState<ObjectInstance[]>([]);
  const [loading, setLoading] = useState(false);
  const runtime = useRuntime();

  useEffect(() => {
    if (!objectTypeId || !layer?.x_property) {
      setRows([]);
      return;
    }
    let cancelled = false;
    setLoading(true);
    void fetchObjectsForVariable(objectTypeId, sourceVariable, 5000, runtime.variableEngine)
      .then((response) => {
        if (cancelled) return;
        setRows(response.data);
      })
      .catch(() => {
        if (cancelled) return;
        setRows([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [objectTypeId, layer?.x_property, runtime.variableEngine, sourceVariable]);

  const echartsOption = useMemo(() => {
    if (!layer || !layer.x_property) return null;
    const xKeys = new Set<string>();
    const segmentKeys = new Set<string>();
    const buckets = new Map<string, Map<string, number>>();
    for (const row of rows) {
      const props = (row.properties as Record<string, unknown>) ?? {};
      const xRaw = props[layer.x_property];
      const xKey = xRaw == null || xRaw === '' ? 'No value' : String(xRaw);
      const segRaw = layer.segment_by ? props[layer.segment_by] : null;
      const segKey = layer.segment_by ? (segRaw == null || segRaw === '' ? 'No value' : String(segRaw)) : '__series__';
      xKeys.add(xKey);
      segmentKeys.add(segKey);
      let increment = 1;
      if (layer.series_metric !== 'count' && layer.series_property) {
        const num = Number(props[layer.series_property]);
        if (!Number.isFinite(num)) continue;
        increment = num;
      }
      const segMap = buckets.get(segKey) ?? new Map<string, number>();
      const previous = segMap.get(xKey) ?? 0;
      if (layer.series_metric === 'count' || layer.series_metric === 'sum' || layer.series_metric === 'avg') {
        segMap.set(xKey, previous + increment);
      } else if (layer.series_metric === 'min') {
        segMap.set(xKey, segMap.has(xKey) ? Math.min(previous, increment) : increment);
      } else if (layer.series_metric === 'max') {
        segMap.set(xKey, Math.max(previous, increment));
      }
      buckets.set(segKey, segMap);
    }
    let xAxisKeys = Array.from(xKeys);
    xAxisKeys.sort((a, b) => {
      if (cfg.sortBy === 'key_asc' || cfg.sortBy === 'key_desc') {
        const cmp = a.localeCompare(b, undefined, { numeric: true });
        return cfg.sortBy === 'key_asc' ? cmp : -cmp;
      }
      const sa = Array.from(buckets.values()).reduce((acc, m) => acc + (m.get(a) ?? 0), 0);
      const sb = Array.from(buckets.values()).reduce((acc, m) => acc + (m.get(b) ?? 0), 0);
      return cfg.sortBy === 'value_asc' ? sa - sb : sb - sa;
    });
    if (layer.x_limit) {
      const limit = Number(layer.x_limit);
      if (Number.isFinite(limit) && limit > 0) xAxisKeys = xAxisKeys.slice(0, limit);
    }
    const series = Array.from(segmentKeys).map((segKey) => {
      const segMap = buckets.get(segKey) ?? new Map<string, number>();
      let data = xAxisKeys.map((x) => segMap.get(x) ?? 0);
      if (layer.cumulative_sum) {
        let acc = 0;
        data = data.map((value) => {
          acc += value;
          return acc;
        });
      }
      const seriesType = layer.layer_type === 'line' ? 'line' : layer.layer_type === 'scatter' ? 'scatter' : 'bar';
      return {
        name: layer.segment_by ? segKey : layer.title,
        type: seriesType,
        stack: layer.segment_by && seriesType === 'bar' ? 'total' : undefined,
        data,
        label: { show: layer.show_labels && seriesType !== 'scatter', position: cfg.barOrientation === 'horizontal' ? 'top' : 'right' },
      };
    });
    const valueAxis = { type: 'value', name: cfg.showTitle ? 'Value' : '' };
    const categoryAxis = { type: 'category', data: xAxisKeys, name: cfg.showTitle ? layer.x_property : '' };
    const isHorizontal = cfg.barOrientation === 'horizontal';
    return {
      color: XY_PALETTE,
      tooltip: cfg.showTooltips ? { trigger: 'axis' } : { show: false },
      legend: cfg.showLegend ? { show: true, top: 0 } : { show: false },
      grid: { left: 50, right: 16, top: cfg.showLegend ? 28 : 12, bottom: 36, containLabel: true },
      xAxis: isHorizontal ? categoryAxis : valueAxis,
      yAxis: isHorizontal ? valueAxis : categoryAxis,
      series,
    };
  }, [rows, layer, cfg.barOrientation, cfg.sortBy, cfg.showLegend, cfg.showTitle, cfg.showTooltips]);

  if (!objectTypeId) {
    return (
      <div style={{ padding: '36px 24px', textAlign: 'center' }}>
        <ChartXyGlyph />
        <p className="of-text-muted" style={{ margin: '8px 0 0', fontSize: 13 }}>Pick an Input Object Set in the layer editor.</p>
      </div>
    );
  }
  if (!layer?.x_property) {
    return (
      <div style={{ padding: '36px 24px', textAlign: 'center' }}>
        <ChartXyGlyph />
        <p className="of-text-muted" style={{ margin: '8px 0 0', fontSize: 13 }}>Choose an X axis property in the layer editor.</p>
      </div>
    );
  }
  return (
    <div style={{ padding: 12 }}>
      {loading ? (
        <p className="of-text-muted" style={{ margin: 0, fontSize: 12, textAlign: 'center', padding: 24 }}>Loading…</p>
      ) : !echartsOption ? (
        <p className="of-text-muted" style={{ margin: 0, fontSize: 12, textAlign: 'center', padding: 24 }}>No data to display.</p>
      ) : (
        <EChartCanvas options={echartsOption} style={{ height: 280 }} />
      )}
    </div>
  );
}

function ChartXyInspector({
  widget,
  variables,
  objectTypes,
  onChange,
  onDelete,
}: {
  widget: AppWidget;
  variables: WorkshopVariable[];
  objectTypes: ObjectType[];
  onChange: (next: AppWidget) => void;
  onDelete: () => void;
}) {
  const [tab, setTab] = useState<'setup' | 'metadata' | 'display'>('setup');
  const [editingLayerId, setEditingLayerId] = useState<string | null>(null);
  const [orientationCustomize, setOrientationCustomize] = useState(false);
  const cfg = readXyConfig(widget);

  function patch(patchObj: Record<string, unknown>) {
    onChange({ ...widget, props: { ...widget.props, ...patchObj } });
  }

  function patchLayer(layerId: string, mutator: (layer: ChartXyLayer) => ChartXyLayer) {
    const layers = cfg.layers.map((layer) => (layer.id === layerId ? mutator(layer) : layer));
    patch({ layers });
  }

  function addLayer() {
    patch({ layers: [...cfg.layers, makeChartXyLayer()] });
  }

  function removeLayer(id: string) {
    patch({ layers: cfg.layers.filter((layer) => layer.id !== id) });
  }

  const editingLayer = cfg.layers.find((layer) => layer.id === editingLayerId) ?? null;

  return (
    <div style={inspectorStyle()}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '10px 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        <span style={{ fontSize: 13, fontWeight: 600 }}>{widget.title}</span>
        <span className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.06em' }}>CHART: XY</span>
      </div>
      <div style={{ display: 'flex', gap: 0, padding: '0 14px', borderBottom: '1px solid var(--border-subtle)' }}>
        {(['setup', 'metadata', 'display'] as const).map((value) => (
          <button
            key={value}
            type="button"
            onClick={() => setTab(value)}
            style={{ padding: '8px 6px', border: 0, background: 'transparent', borderBottom: tab === value ? '2px solid var(--status-info)' : '2px solid transparent', cursor: 'pointer', fontSize: 12, fontWeight: tab === value ? 600 : 500, color: tab === value ? 'var(--text-strong)' : 'var(--text-muted)', marginRight: 14 }}
          >
            {value === 'setup' ? 'Widget setup' : value === 'metadata' ? 'Metadata' : 'Display'}
          </button>
        ))}
      </div>
      {tab === 'setup' ? (
        editingLayer ? (
          <ChartXyLayerEditor
            layer={editingLayer}
            variables={variables}
            objectTypes={objectTypes}
            onBack={() => setEditingLayerId(null)}
            onChange={(next) => patchLayer(editingLayer.id, () => next)}
          />
        ) : (
          <div style={{ padding: 14, display: 'grid', gap: 14 }}>
            <p style={{ margin: 0, fontSize: 13, fontWeight: 600, textAlign: 'center' }}>Chart Editor</p>
            <Section title={`Plot Layers ${cfg.layers.length}`} />
            <div style={{ display: 'grid', gap: 4 }}>
              {cfg.layers.map((layer) => (
                <div key={layer.id} style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '8px 10px', border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f7f9fa' }}>
                  <ChartXyGlyph />
                  <button type="button" onClick={() => setEditingLayerId(layer.id)} style={{ flex: 1, border: 0, background: 'transparent', padding: 0, textAlign: 'left', fontSize: 13, cursor: 'pointer' }}>{layer.title}</button>
                  {cfg.layers.length > 1 ? (
                    <button type="button" aria-label="Remove layer" onClick={() => removeLayer(layer.id)} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--status-danger)' }}><Glyph name="trash" size={11} /></button>
                  ) : null}
                  <button type="button" aria-label="Edit layer" onClick={() => setEditingLayerId(layer.id)} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--text-muted)' }}><Glyph name="chevron-right" size={11} /></button>
                </div>
              ))}
              <button type="button" onClick={addLayer} className="of-button" style={{ fontSize: 12, justifyContent: 'center' }}>
                <Glyph name="plus" size={11} /> Add a layer
              </button>
            </div>

            <Section title={`Annotations 0`} />
            <button type="button" className="of-button" style={{ fontSize: 12, justifyContent: 'center' }}>
              <Glyph name="plus" size={11} /> Add annotation
            </button>

            <Section title="Y axis" />
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 0 }}>
              {(['categorical', 'continuous'] as const).map((kind) => (
                <button
                  key={kind}
                  type="button"
                  onClick={() => patch({ y_axis_kind: kind })}
                  style={{ padding: '6px 8px', border: '1px solid var(--border-default)', background: cfg.yAxisKind === kind ? '#fff' : '#f4f6f9', color: cfg.yAxisKind === kind ? 'var(--text-strong)' : 'var(--text-muted)', cursor: 'pointer', fontSize: 12, fontWeight: cfg.yAxisKind === kind ? 600 : 500 }}
                >
                  {kind === 'categorical' ? 'Categorical' : 'Continuous'}
                </button>
              ))}
            </div>
            <Toggle label="Show title" value={cfg.showTitle} onChange={(checked) => patch({ show_title: checked })} />
            <Toggle label="Show color markers" value={cfg.showColorMarkers} onChange={(checked) => patch({ show_color_markers: checked })} />
            <Toggle label="Enable numerical formatting" value={cfg.enableNumericalFormatting} onChange={(checked) => patch({ enable_numerical_formatting: checked })} />
            <Field label="Sort by">
              <select value={cfg.sortBy} onChange={(event) => patch({ sort_by: event.target.value })} style={inputStyle()}>
                <option value="key_asc">Key Ascending</option>
                <option value="key_desc">Key Descending</option>
                <option value="value_asc">Value Ascending</option>
                <option value="value_desc">Value Descending</option>
              </select>
            </Field>

            <Section title="X axis" />
            <div style={{ padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f7f9fa' }}>
              <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>Axes display settings are automatically generated by series data.</p>
              <button type="button" className="of-button" style={{ marginTop: 8, fontSize: 12, justifyContent: 'center', width: '100%' }}>
                <Glyph name="settings" size={11} /> Customize
              </button>
            </div>

            <Section title="Ontology formatting" />
            <Toggle label="Enable ontology colors" value={cfg.enableOntologyColors} onChange={(checked) => patch({ enable_ontology_colors: checked })} />

            <Section title="Legend" />
            <Toggle label="Show legend" value={cfg.showLegend} onChange={(checked) => patch({ show_legend: checked })} />

            <Section title="Tooltips" />
            <Toggle label="Show tooltips" value={cfg.showTooltips} onChange={(checked) => patch({ show_tooltips: checked })} />

            <Section title="Exports" />
            <Toggle label="Allow exports" value={cfg.allowExports} onChange={(checked) => patch({ allow_exports: checked })} />

            <Section title="Bar orientation" />
            {orientationCustomize ? (
              <div style={{ padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 4, display: 'grid', gap: 8 }}>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 0 }}>
                  {(['horizontal', 'vertical'] as const).map((kind) => (
                    <button
                      key={kind}
                      type="button"
                      onClick={() => patch({ bar_orientation: kind })}
                      style={{ padding: '6px 8px', border: '1px solid var(--border-default)', background: cfg.barOrientation === kind ? '#fff' : '#f4f6f9', color: cfg.barOrientation === kind ? 'var(--text-strong)' : 'var(--text-muted)', cursor: 'pointer', fontSize: 12, fontWeight: cfg.barOrientation === kind ? 600 : 500 }}
                    >
                      {kind === 'horizontal' ? 'Horizontal' : 'Vertical'}
                    </button>
                  ))}
                </div>
                <button type="button" className="of-link" onClick={() => setOrientationCustomize(false)} style={linkBtnStyle()}>Close</button>
              </div>
            ) : (
              <div style={{ padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f7f9fa' }}>
                <p className="of-text-muted" style={{ margin: 0, fontSize: 12 }}>Bar Orientation settings are automatically generated by series data.</p>
                <button type="button" className="of-button" onClick={() => setOrientationCustomize(true)} style={{ marginTop: 8, fontSize: 12, justifyContent: 'center', width: '100%' }}>
                  <Glyph name="settings" size={11} /> Customize
                </button>
              </div>
            )}

            <button type="button" onClick={onDelete} className="of-button" style={{ color: 'var(--status-danger)', borderColor: '#fecaca' }}>
              <Glyph name="trash" size={12} /> Delete widget
            </button>
          </div>
        )
      ) : tab === 'display' ? (
        <DisplayTab widget={widget} onChange={onChange} />
      ) : (
        <div style={{ padding: 14 }}><p className="of-text-muted" style={{ fontSize: 12 }}>Widget metadata coming soon.</p></div>
      )}
    </div>
  );
}

function PreviewRuntime({
  app,
  pages,
  activePage,
  variables,
  objectTypes,
  headerSettings,
  headerUi,
  onEdit,
  onOpenLineage,
  children,
}: {
  app: AppDefinition;
  pages: AppPage[];
  activePage: AppPage;
  variables: WorkshopVariable[];
  objectTypes: ObjectType[];
  headerSettings: WorkshopHeaderSettings;
  headerUi: HeaderUiState;
  onEdit: () => void;
  onOpenLineage: () => void;
  children: React.ReactNode;
}) {
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
      if (sameRuntimeObjectSelection(existing, objects)) return current;
      return { ...current, [variableId]: objects };
    });
  }, []);
  const setShapeOutput = useCallback((variableId: string, shape: WorkshopMapFeatureCollection | null) => {
    setShapeOutputs((current) => {
      if (sameRuntimeShapeOutput(current[variableId] ?? null, shape)) return current;
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

  void pages;
  void activePage;

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'i') {
        event.preventDefault();
        onOpenLineage();
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onOpenLineage]);

  return (
    <WorkshopRuntimeContext.Provider value={runtime}>
      <PreviewShell app={app} headerSettings={headerSettings} headerUi={headerUi} onEdit={onEdit}>
        {children}
      </PreviewShell>
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
        <div role="status" style={{ position: 'fixed', top: 16, left: '50%', transform: 'translateX(-50%)', zIndex: 100, display: 'inline-flex', alignItems: 'center', gap: 10, padding: '10px 16px', borderRadius: 6, background: '#15803d', color: '#fff', fontSize: 13, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.18)' }}>
          <Glyph name="check" size={13} tone="#fff" />
          <span>{toast}</span>
          <button type="button" className="of-link" style={{ background: 'none', border: 0, color: '#fff', cursor: 'pointer', fontSize: 13, fontWeight: 600 }}>Revert</button>
          <button type="button" aria-label="Dismiss" onClick={() => setToast(null)} style={{ border: 0, background: 'transparent', color: '#fff', cursor: 'pointer' }}><Glyph name="x" size={11} tone="#fff" /></button>
        </div>
      ) : null}
    </WorkshopRuntimeContext.Provider>
  );
}

function sameRuntimeObjectSelection(left: ObjectInstance[], right: ObjectInstance[]) {
  if (left.length !== right.length) return false;
  return left.every((entry, index) => entry.id === right[index]?.id);
}

function sameRuntimeShapeOutput(left: WorkshopMapFeatureCollection | null, right: WorkshopMapFeatureCollection | null) {
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

function resolveRuntimeObject(
  variableId: string,
  activeObjects: Record<string, ObjectInstance | null>,
  selectedObjectSets: Record<string, ObjectInstance[]>,
) {
  return activeObjects[variableId] ?? selectedObjectSets[variableId]?.[0] ?? null;
}

export function ActionFormModal({
  button,
  variables = [],
  activeObjects = {},
  selectedObjectSets = {},
  objectTypes = [],
  onClose,
  onSuccess,
}: {
  button: ButtonGroupButton;
  variables?: WorkshopVariable[];
  activeObjects?: Record<string, ObjectInstance | null>;
  selectedObjectSets?: Record<string, ObjectInstance[]>;
  objectTypes?: ObjectType[];
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [action, setAction] = useState<ActionType | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [formValues, setFormValues] = useState<Record<string, string>>({});

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    void getActionType(button.action_type_id)
      .then((fetched) => {
        if (cancelled) return;
        setAction(fetched);
        const config = fetched.config as { property_mappings?: Array<{ property_name: string; kind: string; static_value?: string }> } | null | undefined;
        const mappings = config?.property_mappings ?? [];
        const initialValues: Record<string, string> = {};
        for (const field of fetched.input_schema ?? []) {
          const def = button.parameter_defaults?.[field.name];
          if (def?.kind === 'static' && def.static_value !== undefined) {
            initialValues[field.name] = def.static_value;
          } else if (def?.kind === 'active_object' || def?.kind === 'variable') {
            const variable = def.variable_id ? variables.find((v) => v.id === def.variable_id) ?? null : null;
            const obj = variable ? resolveRuntimeObject(variable.id, activeObjects, selectedObjectSets) : null;
            initialValues[field.name] = obj?.id ?? '';
          } else {
            const mapping = mappings.find((m) => m.property_name === field.name);
            if (mapping?.kind === 'static' && typeof mapping.static_value === 'string') {
              initialValues[field.name] = mapping.static_value;
            } else {
              initialValues[field.name] = '';
            }
          }
        }
        const orderField = (fetched.input_schema ?? []).find((field) => field.property_type === 'object_reference' || field.name.toLowerCase().includes('order') || field.name === 'object');
        if (orderField && !initialValues[orderField.name]) {
          const objectVariable = variables.find((v) => v.kind === 'object_set_active_object') ?? variables.find((v) => v.kind === 'object_set_selection') ?? null;
          if (objectVariable) {
            const obj = resolveRuntimeObject(objectVariable.id, activeObjects, selectedObjectSets);
            if (obj) initialValues[orderField.name] = obj.id;
          }
        }
        setFormValues(initialValues);
      })
      .catch((cause) => {
        if (cancelled) return;
        setError(cause instanceof Error ? cause.message : 'Failed to load action');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [button, variables, activeObjects, selectedObjectSets]);

  const orderField = (action?.input_schema ?? []).find((field) => field.property_type === 'object_reference' || field.name.toLowerCase().includes('order') || field.name === 'object');
  const orderId = orderField ? formValues[orderField.name] : '';
  const defaultActiveObject = useMemo(() => {
    for (const def of Object.values(button.parameter_defaults ?? {})) {
      if ((def.kind === 'active_object' || def.kind === 'variable') && def.variable_id) {
        const object = activeObjects[def.variable_id];
        if (object) return object;
        const selected = selectedObjectSets[def.variable_id]?.[0] ?? null;
        if (selected) return selected;
      }
    }
    const objectVariable = variables.find((v) => v.kind === 'object_set_active_object') ?? variables.find((v) => v.kind === 'object_set_selection') ?? null;
    return objectVariable ? resolveRuntimeObject(objectVariable.id, activeObjects, selectedObjectSets) : null;
  }, [activeObjects, button.parameter_defaults, selectedObjectSets, variables]);
  const targetObjectId = orderId || defaultActiveObject?.id || '';
  const objectType = action ? objectTypes.find((entry) => entry.id === action.object_type_id) ?? null : null;

  async function submit() {
    if (!action) return;
    if (!targetObjectId) {
      setError('Pick an order to update.');
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      const properties: Record<string, unknown> = {};
      const config = action.config as { property_mappings?: Array<{ property_name: string; kind: string; static_value?: string }> } | null | undefined;
      const mappings = config?.property_mappings ?? [];
      for (const mapping of mappings) {
        if (mapping.property_name === orderField?.name) continue;
        if (mapping.kind === 'parameter') {
          const value = formValues[mapping.property_name];
          if (value !== undefined && value !== '') {
            properties[mapping.property_name] = value;
          }
        } else if (mapping.kind === 'static' && mapping.static_value !== undefined) {
          properties[mapping.property_name] = mapping.static_value;
        }
      }
      for (const field of action.input_schema ?? []) {
        if (field.name === orderField?.name) continue;
        if (mappings.find((m) => m.property_name === field.name)) continue;
        const value = formValues[field.name];
        if (value !== undefined && value !== '') {
          properties[field.name] = value;
        }
      }
      await updateObject(action.object_type_id, targetObjectId, { properties });
      onSuccess();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Submit failed');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div role="dialog" aria-modal="true" onMouseDown={(event) => { if (event.target === event.currentTarget && !submitting) onClose(); }} style={{ position: 'fixed', inset: 0, zIndex: 95, background: 'rgba(17, 24, 39, 0.42)', display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}>
      <section style={{ width: '100%', maxWidth: 540, background: '#fff', borderRadius: 6, boxShadow: '0 20px 48px rgba(15, 23, 42, 0.2)', overflow: 'hidden' }}>
        <header style={{ padding: '14px 18px', borderBottom: '1px solid var(--border-subtle)', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <h3 style={{ margin: 0, fontSize: 14, fontWeight: 600 }}>{action?.display_name || action?.name || 'Action'}</h3>
          <button type="button" aria-label="Close" onClick={onClose} className="of-button of-button--ghost" style={{ padding: 4 }}><Glyph name="x" size={12} /></button>
        </header>
        <div style={{ padding: 18, display: 'grid', gap: 14 }}>
          {loading ? (
            <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>Loading…</p>
          ) : !action ? (
            <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>{error || 'Action not found.'}</p>
          ) : (
            <>
              {orderField ? (
                <label style={{ display: 'grid', gap: 4 }}>
                  <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--text-muted)', textTransform: 'uppercase' }}>{orderField.display_name || orderField.name} <span style={{ color: 'var(--status-danger)' }}>*</span></span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#f7f9fa' }}>
                    <Glyph name="cube" size={12} tone="#2d72d2" />
                    <span style={{ flex: 1, fontSize: 13 }}>{orderId || objectType?.display_name || objectType?.name || 'Select an object'}</span>
                    {orderId ? <button type="button" aria-label="Clear" onClick={() => setFormValues((current) => ({ ...current, [orderField.name]: '' }))} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--text-muted)' }}><Glyph name="x" size={11} /></button> : null}
                    <Glyph name="chevron-down" size={11} />
                  </span>
                </label>
              ) : (
                <div style={{ display: 'grid', gap: 4 }}>
                  <span style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--text-muted)', textTransform: 'uppercase' }}>Object <span style={{ color: 'var(--status-danger)' }}>*</span></span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '8px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#f7f9fa' }}>
                    <Glyph name="cube" size={12} tone="#2d72d2" />
                    <span style={{ flex: 1, fontSize: 13 }}>{targetObjectId || objectType?.display_name || objectType?.name || 'Select an object'}</span>
                  </span>
                </div>
              )}
              {(action.input_schema ?? []).filter((field) => field !== orderField).map((field) => (
                <label key={field.name} style={{ display: 'grid', gap: 4 }}>
                  <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', fontSize: 11, fontWeight: 700, letterSpacing: '0.04em', color: 'var(--text-muted)', textTransform: 'uppercase' }}>
                    <span>{field.display_name || field.name} <span style={{ color: 'var(--status-danger)' }}>*</span></span>
                    {formValues[field.name] ? <span className="of-chip" style={{ background: '#fef3c7', color: '#b45309', fontSize: 10 }}>Edited</span> : null}
                  </span>
                  <input
                    value={formValues[field.name] ?? ''}
                    onChange={(event) => setFormValues((current) => ({ ...current, [field.name]: event.target.value }))}
                    placeholder={field.display_name || field.name}
                    style={{ padding: '8px 10px', border: '1px solid var(--border-default)', borderRadius: 4, fontSize: 13 }}
                  />
                </label>
              ))}
              {error ? (
                <div role="alert" className="of-status-danger" style={{ padding: '8px 12px', borderRadius: 4, fontSize: 12 }}>
                  {error}
                </div>
              ) : null}
            </>
          )}
        </div>
        <footer style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, padding: 12, borderTop: '1px solid var(--border-subtle)' }}>
          <button type="button" aria-label="Reset" className="of-button of-button--ghost" style={{ padding: 8 }}><Glyph name="undo" size={12} /></button>
          <button type="button" className="of-button" onClick={onClose} disabled={submitting}>Cancel</button>
          <button
            type="button"
            onClick={() => void submit()}
            disabled={loading || submitting || !action}
            style={{ padding: '8px 16px', border: 0, borderRadius: 4, background: '#15803d', color: '#fff', fontSize: 13, fontWeight: 600, cursor: submitting ? 'not-allowed' : 'pointer' }}
          >
            {submitting ? 'Submitting…' : 'Submit'}
          </button>
        </footer>
      </section>
    </div>
  );
}

function PreviewShell({
  app,
  headerSettings,
  headerUi,
  onEdit,
  children,
}: {
  app: AppDefinition;
  headerSettings: WorkshopHeaderSettings;
  headerUi: HeaderUiState;
  onEdit: () => void;
  children: React.ReactNode;
}) {
  const [pillOpen, setPillOpen] = useState(true);
  const [moreOpen, setMoreOpen] = useState(false);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'e') {
        event.preventDefault();
        onEdit();
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onEdit]);

  return (
    <div style={{ position: 'fixed', inset: 0, zIndex: 75, display: 'grid', gridTemplateRows: 'auto 1fr', background: '#f4f6f9' }}>
      <header style={{ position: 'relative', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '8px 14px', borderBottom: '1px solid var(--border-subtle)', background: '#fff' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          {headerUi.enable_app_logo ? (
            <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 28, height: 28, borderRadius: 4, background: `${headerSettings.color ?? '#2d72d2'}1a` }}>
              <Glyph name={(headerSettings.icon ?? 'cube') as GlyphName} size={16} tone={headerSettings.color ?? '#2d72d2'} />
            </span>
          ) : null}
          <span style={{ fontSize: 15, fontWeight: 600 }}>{headerSettings.title || app.name}</span>
          {headerUi.enable_favoriting ? <Glyph name="star" size={14} tone="#cf923f" /> : null}
        </div>
        <div style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 6 }}>
          <button type="button" aria-label="More" onClick={() => setMoreOpen((open) => !open)} className="of-button of-button--ghost" style={{ padding: 6 }}>
            <span style={{ display: 'inline-flex', gap: 2 }}>
              <span style={{ width: 3, height: 3, borderRadius: '50%', background: '#5c7080' }} />
              <span style={{ width: 3, height: 3, borderRadius: '50%', background: '#5c7080' }} />
              <span style={{ width: 3, height: 3, borderRadius: '50%', background: '#5c7080' }} />
            </span>
          </button>
          {moreOpen ? (
            <div role="menu" style={{ position: 'absolute', top: 'calc(100% + 4px)', right: 0, background: '#fff', border: '1px solid var(--border-default)', borderRadius: 4, boxShadow: '0 8px 24px rgba(15, 23, 42, 0.12)', padding: 4, zIndex: 6, minWidth: 180 }}>
              <button type="button" onClick={() => { setMoreOpen(false); onEdit(); }} style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '8px 10px', border: 0, background: 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13, borderRadius: 4 }}>
                <Glyph name="pencil" size={12} tone="#5c7080" /> Edit
                <span style={{ marginLeft: 'auto', color: 'var(--text-muted)', fontSize: 11 }}>⌘E</span>
              </button>
              <button type="button" onClick={() => setMoreOpen(false)} style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '8px 10px', border: 0, background: 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13, borderRadius: 4 }}>
                <Glyph name="duplicate" size={12} tone="#5c7080" /> Copy link
              </button>
              <button type="button" onClick={() => setMoreOpen(false)} style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%', padding: '8px 10px', border: 0, background: 'transparent', cursor: 'pointer', textAlign: 'left', fontSize: 13, borderRadius: 4 }}>
                <Glyph name="external-link" size={12} tone="#5c7080" /> Open in new tab
              </button>
            </div>
          ) : null}
        </div>

        {pillOpen ? (
          <div
            role="toolbar"
            style={{
              position: 'absolute',
              left: '50%',
              top: 'calc(100% + 8px)',
              transform: 'translateX(-50%)',
              display: 'inline-flex',
              alignItems: 'center',
              padding: 4,
              borderRadius: 8,
              background: '#1c2127',
              boxShadow: '0 8px 24px rgba(15, 23, 42, 0.32)',
              gap: 0,
              zIndex: 6,
            }}
          >
            <button type="button" onClick={onEdit} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '6px 12px', border: 0, background: 'transparent', color: '#fff', fontSize: 13, fontWeight: 500, cursor: 'pointer', borderRadius: 4 }}>
              <Glyph name="pencil" size={13} tone="#fff" /> Edit
              <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', padding: '1px 6px', borderRadius: 4, background: 'rgba(255, 255, 255, 0.12)', fontSize: 11, fontWeight: 500 }}>⌘E</span>
            </button>
            <span style={{ width: 1, height: 18, background: 'rgba(255, 255, 255, 0.12)' }} />
            <button type="button" style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '6px 12px', border: 0, background: 'transparent', color: '#fff', fontSize: 13, fontWeight: 500, cursor: 'pointer', borderRadius: 4 }}>
              <Glyph name="object" size={13} tone="#fff" /> Main
              <span style={{ color: '#aab4c0', fontSize: 11 }}>Default</span>
              <Glyph name="chevron-down" size={11} tone="#aab4c0" />
            </button>
            <span style={{ width: 1, height: 18, background: 'rgba(255, 255, 255, 0.12)' }} />
            <button type="button" style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '6px 12px', border: 0, background: 'transparent', color: '#fff', fontSize: 13, fontWeight: 500, cursor: 'pointer', borderRadius: 4 }}>
              <Glyph name="badge-check" size={13} tone="#fff" /> v0.1.0
              <Glyph name="chevron-down" size={11} tone="#aab4c0" />
            </button>
            <button type="button" aria-label="Hide toolbar" onClick={() => setPillOpen(false)} style={{ position: 'absolute', left: '50%', bottom: -10, transform: 'translateX(-50%)', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 22, height: 14, border: 0, borderRadius: '0 0 6px 6px', background: '#1c2127', color: '#aab4c0', cursor: 'pointer' }}>
              <Glyph name="chevron-down" size={9} tone="#aab4c0" />
            </button>
          </div>
        ) : (
          <button
            type="button"
            aria-label="Show toolbar"
            onClick={() => setPillOpen(true)}
            style={{ position: 'absolute', left: '50%', top: 'calc(100% + 4px)', transform: 'translateX(-50%)', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 28, height: 16, border: 0, borderRadius: '0 0 6px 6px', background: '#1c2127', color: '#aab4c0', cursor: 'pointer', zIndex: 6 }}
          >
            <Glyph name="chevron-down" size={11} tone="#aab4c0" />
          </button>
        )}
      </header>
      {children}
    </div>
  );
}

function ChartXyLayerEditor({
  layer,
  variables,
  objectTypes,
  onBack,
  onChange,
}: {
  layer: ChartXyLayer;
  variables: WorkshopVariable[];
  objectTypes: ObjectType[];
  onBack: () => void;
  onChange: (next: ChartXyLayer) => void;
}) {
  const sourceVariable = variables.find((v) => v.id === layer.source_variable_id) ?? null;
  const objectTypeId = sourceVariable?.object_type_id ?? layer.object_type_id ?? '';
  const objectType = objectTypes.find((entry) => entry.id === objectTypeId) ?? null;
  const [properties, setProperties] = useState<Property[]>([]);
  const [pickerOpen, setPickerOpen] = useState(false);

  useEffect(() => {
    if (!objectTypeId) {
      setProperties([]);
      return;
    }
    let cancelled = false;
    void listProperties(objectTypeId)
      .then((response) => {
        if (!cancelled) setProperties(response);
      })
      .catch(() => {
        if (!cancelled) setProperties([]);
      });
    return () => {
      cancelled = true;
    };
  }, [objectTypeId]);

  function patch(patchObj: Partial<ChartXyLayer>) {
    onChange({ ...layer, ...patchObj });
  }

  const numericProperties = properties.filter((p) => ['number', 'integer', 'float', 'double', 'decimal'].includes(String(p.property_type).toLowerCase()));

  return (
    <div style={{ padding: 14, display: 'grid', gap: 14 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <button type="button" onClick={onBack} style={{ border: 0, background: 'transparent', cursor: 'pointer', color: 'var(--status-info)', fontSize: 13, padding: 0, display: 'inline-flex', alignItems: 'center', gap: 4 }}>
          <Glyph name="chevron-right" size={11} /> <span style={{ transform: 'rotate(180deg)', display: 'inline-block' }}></span>Chart Editor
        </button>
        <span style={{ fontSize: 13, fontWeight: 600 }}>{layer.title}</span>
      </div>
      <Section title="Title" />
      <input value={layer.title} onChange={(event) => patch({ title: event.target.value })} style={inputStyle()} />

      <Section title="Data input" />
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 0 }}>
        {(['object_set', 'function', 'time_series'] as const).map((kind) => (
          <button
            key={kind}
            type="button"
            onClick={() => patch({ data_input: kind })}
            style={{ padding: '6px 8px', border: '1px solid var(--border-default)', background: layer.data_input === kind ? '#fff' : '#f4f6f9', color: layer.data_input === kind ? 'var(--text-strong)' : 'var(--text-muted)', cursor: 'pointer', fontSize: 12, fontWeight: layer.data_input === kind ? 600 : 500 }}
          >
            {kind === 'object_set' ? 'Object set' : kind === 'function' ? 'Function' : 'Time series set'}
          </button>
        ))}
      </div>

      <Field label="Source">
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', border: '1px solid var(--border-default)', borderRadius: 4, background: '#f7f9fa' }}>
          <Glyph name="cube" size={13} tone="#2d72d2" />
          <span style={{ flex: 1, fontSize: 13 }}>{sourceVariable ? sourceVariable.name : objectType ? objectType.display_name || objectType.name : 'Select object set…'}</span>
          <button type="button" aria-label="Edit" onClick={() => setPickerOpen(true)} className="of-button of-button--ghost" style={{ padding: 2 }}>
            <Glyph name="pencil" size={11} />
          </button>
          {(layer.source_variable_id || layer.object_type_id) ? (
            <button type="button" aria-label="Clear" onClick={() => patch({ source_variable_id: '', object_type_id: '', x_property: '', segment_by: '', series_property: '' })} className="of-button of-button--ghost" style={{ padding: 2 }}>
              <Glyph name="x" size={11} />
            </button>
          ) : null}
        </div>
        <span className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>Current value: {sourceVariable ? sourceVariable.name : objectType ? objectType.display_name || objectType.name : 'undefined'}</span>
      </Field>
      <Field label="Variable">
        <select
          value={layer.source_variable_id}
          onChange={(event) => patch({ source_variable_id: event.target.value, object_type_id: '', x_property: '', segment_by: '', series_property: '' })}
          style={inputStyle()}
        >
          <option value="">Select object set variable…</option>
          {variables
            .filter((v) => v.kind === 'object_set' || v.kind === 'object_set_definition' || v.kind === 'filter_output')
            .map((v) => (
              <option key={v.id} value={v.id}>{v.name} ({VARIABLE_KIND_LABEL[v.kind]})</option>
            ))}
        </select>
      </Field>

      <Section title="Layer type" />
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 6 }}>
        {(['bar', 'line', 'scatter'] as const).map((kind) => (
          <button
            key={kind}
            type="button"
            onClick={() => patch({ layer_type: kind, title: kind === 'bar' ? 'Layer (bar)' : kind === 'line' ? 'Layer (line)' : 'Layer (scatter)' })}
            style={{ padding: '10px 6px', border: layer.layer_type === kind ? '2px solid var(--status-info)' : '1px solid var(--border-default)', background: layer.layer_type === kind ? 'rgba(45, 114, 210, 0.06)' : '#fff', borderRadius: 4, cursor: 'pointer', fontSize: 12, display: 'grid', justifyItems: 'center', gap: 4 }}
          >
            {kind === 'bar' ? <ChartXyGlyph /> : kind === 'line' ? <Glyph name="graph" size={13} tone="#5c7080" /> : <Glyph name="sparkles" size={13} tone="#5c7080" />}
            {kind === 'bar' ? 'Bar Chart' : kind === 'line' ? 'Line Chart' : 'Scatter Chart'}
          </button>
        ))}
      </div>
      <Toggle label="Show labels" value={layer.show_labels} onChange={(checked) => patch({ show_labels: checked })} />

      <Section title="X axis property" />
      <Field label="Property">
        <select value={layer.x_property} onChange={(event) => patch({ x_property: event.target.value })} style={inputStyle()}>
          <option value="">Select a property…</option>
          {properties.map((p) => (
            <option key={p.id} value={p.name}>{p.display_name || p.name}</option>
          ))}
        </select>
      </Field>
      <Field label="Bucketing">
        <select value={layer.x_bucketing} onChange={(event) => patch({ x_bucketing: event.target.value as ChartXyLayer['x_bucketing'] })} style={inputStyle()}>
          <option value="exact">Exact Value</option>
          <option value="range">Range</option>
        </select>
      </Field>
      <Field label="Limit">
        <input
          type="number"
          min={0}
          value={layer.x_limit}
          placeholder="Set category limit…"
          onChange={(event) => patch({ x_limit: event.target.value })}
          style={inputStyle()}
        />
      </Field>

      <Section title={layer.layer_type === 'bar' ? 'Bar series' : layer.layer_type === 'line' ? 'Line series' : 'Scatter series'} />
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', border: '1px solid var(--border-subtle)', borderRadius: 4, background: '#f4f6f9' }}>
        <Glyph name="run" size={11} tone="#5c7080" />
        <select value={layer.series_metric} onChange={(event) => patch({ series_metric: event.target.value as ChartXyLayer['series_metric'] })} style={{ flex: 1, border: 0, background: 'transparent', outline: 'none', fontSize: 13 }}>
          <option value="count">Count</option>
          <option value="sum">Sum</option>
          <option value="avg">Average</option>
          <option value="min">Min</option>
          <option value="max">Max</option>
        </select>
        {layer.series_metric !== 'count' ? (
          <select value={layer.series_property} onChange={(event) => patch({ series_property: event.target.value })} style={{ border: 0, background: 'transparent', outline: 'none', fontSize: 13 }}>
            <option value="">…</option>
            {numericProperties.map((p) => (
              <option key={p.id} value={p.name}>{p.display_name || p.name}</option>
            ))}
          </select>
        ) : null}
      </div>
      <button type="button" className="of-button" style={{ fontSize: 12, justifyContent: 'center' }}>
        <Glyph name="plus" size={11} /> Add step
      </button>
      <Toggle label="Cumulative sum" value={layer.cumulative_sum} onChange={(checked) => patch({ cumulative_sum: checked })} />

      <Section title="Segment by (optional)" />
      <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
        <select value={layer.segment_by} onChange={(event) => patch({ segment_by: event.target.value })} style={{ ...inputStyle(), borderRadius: '4px 0 0 4px' }}>
          <option value="">Select a property…</option>
          {properties.map((p) => (
            <option key={p.id} value={p.name}>{p.display_name || p.name}</option>
          ))}
        </select>
        {layer.segment_by ? (
          <button type="button" aria-label="Clear" onClick={() => patch({ segment_by: '' })} style={{ padding: '6px 8px', border: '1px solid var(--border-default)', borderLeft: 0, borderRadius: '0 4px 4px 0', background: '#fff', cursor: 'pointer' }}>
            <Glyph name="x" size={11} />
          </button>
        ) : null}
      </div>

      <Section title="Segment display overrides" />
      <button type="button" className="of-button" style={{ fontSize: 12, justifyContent: 'center' }}>
        <Glyph name="plus" size={11} /> Add segment
      </button>

      {pickerOpen ? (
        <ObjectSetPicker
          objectTypes={objectTypes}
          onClose={() => setPickerOpen(false)}
          onSelect={(typeId) => {
            patch({ source_variable_id: '', object_type_id: typeId, x_property: '', segment_by: '', series_property: '' });
            setPickerOpen(false);
          }}
        />
      ) : null}
    </div>
  );
}
