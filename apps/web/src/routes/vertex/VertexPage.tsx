import { useCallback, useEffect, useMemo, useState } from 'react';
import type {
  Core,
  ElementDefinition,
  EventObject,
  StylesheetStyle,
} from 'cytoscape';

import { CytoscapeCanvas } from '@components/CytoscapeCanvas';
import { EChartView } from '@/lib/components/analytics/EChartView';
import {
  getOntologyGraph,
  listNeighbors,
  listObjects,
  listObjectTypes,
  listQuiverVisualFunctions,
  searchOntology,
  simulateObjectScenarios,
  type GraphEdge,
  type GraphNode,
  type GraphResponse,
  type NeighborLink,
  type ObjectInstance,
  type ObjectScenarioSimulationResponse,
  type ObjectType,
  type QuiverVisualFunction,
  type ScenarioSimulationCandidate,
  type SearchResult,
} from '@/lib/api/ontology';

type LayoutMode = 'cose' | 'breadthfirst' | 'grid' | 'circle' | 'concentric';
type NodeDisplayMode = 'compact' | 'card';
type SidebarTab = 'selection' | 'events' | 'series' | 'layers' | 'media' | 'scenarios';

interface VertexTemplate {
  id: string;
  name: string;
  description: string;
  rootTypeId: string;
  rootObjectId: string;
  depth: number;
  layout: LayoutMode;
  nodeDisplayMode: NodeDisplayMode;
  subtitleField: string;
  extendedLabelField: string;
  colorByField: string;
  timeField: string;
  eventStartField: string;
  eventEndField: string;
  mediaField: string;
  annotationField: string;
  sharedLensId: string;
  createdAt: string;
  updatedAt: string;
}

interface VertexAnnotation {
  id: string;
  label: string;
  x: number;
  y: number;
  width: number;
  height: number;
  color: string;
  note: string;
}

interface ScenarioDraft {
  name: string;
  description: string;
  propertyName: string;
  propertyValue: string;
}

const LAYOUT_OPTIONS: Array<{ id: LayoutMode; label: string }> = [
  { id: 'cose', label: 'Auto' },
  { id: 'breadthfirst', label: 'Hierarchy' },
  { id: 'grid', label: 'Grid' },
  { id: 'circle', label: 'Circular' },
  { id: 'concentric', label: 'Cluster' },
];

const SIDEBAR_TABS: Array<{ id: SidebarTab; label: string }> = [
  { id: 'selection', label: 'Selection' },
  { id: 'events', label: 'Events' },
  { id: 'series', label: 'Series' },
  { id: 'layers', label: 'Layers' },
  { id: 'media', label: 'Media' },
  { id: 'scenarios', label: 'Scenarios' },
];

const STORAGE_KEYS = {
  templates: 'of.vertex.templates',
  annotations: 'of.vertex.annotations',
};

function createId() {
  return crypto.randomUUID?.() ?? Math.random().toString(36).slice(2, 10);
}

function parseObjectId(node: GraphNode | null) {
  if (!node || !node.id.startsWith('object:')) return '';
  return node.id.slice('object:'.length);
}

function selectedTypeIdFromNode(node: GraphNode | null) {
  const value = node?.metadata?.['object_type_id'];
  return typeof value === 'string' ? value : '';
}

function parseRecord(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  return value as Record<string, unknown>;
}

function stringifyValue(value: unknown) {
  if (value === null || value === undefined) return '—';
  if (Array.isArray(value)) return value.length ? value.join(', ') : '—';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function numericValue(value: unknown) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value.replace(/,/g, ''));
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function objectLabelFromProperties(properties: Record<string, unknown>) {
  for (const key of ['name', 'title', 'display_name', 'label', 'code', 'identifier', 'id']) {
    const value = properties[key];
    if (typeof value === 'string' && value.trim()) return value;
  }
  return 'Object';
}

function looksLikeIso(value: unknown) {
  return typeof value === 'string' && /\d{4}-\d{2}-\d{2}/.test(value);
}

function detectDateField(properties: Record<string, unknown>) {
  const keys = Object.keys(properties);
  return (
    keys.find((key) => /date|time|timestamp|day|week|month/i.test(key) && looksLikeIso(properties[key])) ??
    keys.find((key) => looksLikeIso(properties[key])) ??
    ''
  );
}

function detectMetricField(properties: Record<string, unknown>) {
  const keys = Object.keys(properties);
  return (
    keys.find(
      (key) =>
        /score|value|count|duration|delay|cost|risk|load|temperature|pressure/i.test(key) &&
        numericValue(properties[key]) !== null,
    ) ??
    keys.find((key) => numericValue(properties[key]) !== null) ??
    ''
  );
}

function detectTemporalFields(properties: Record<string, unknown>) {
  const keys = Object.keys(properties);
  const start =
    keys.find((key) => /start|opened|begin|from|scheduled_start/i.test(key) && looksLikeIso(properties[key])) ??
    '';
  const end =
    keys.find((key) => /end|closed|finish|to|scheduled_end/i.test(key) && looksLikeIso(properties[key])) ??
    '';
  return { start, end };
}

function detectMediaField(properties: Record<string, unknown>) {
  const keys = Object.keys(properties);
  return (
    keys.find((key) => /image|media|photo|thumbnail|diagram|url/i.test(key) && typeof properties[key] === 'string') ??
    ''
  );
}

function detectAnnotationField(properties: Record<string, unknown>) {
  return Object.keys(properties).find((key) => /coordinate|bbox|bound|annotation|polygon|box/i.test(key)) ?? '';
}

function coerceScenarioValue(raw: string, original: unknown) {
  if (typeof original === 'number') {
    const parsed = Number(raw);
    return Number.isFinite(parsed) ? parsed : original;
  }
  if (typeof original === 'boolean') return raw.trim().toLowerCase() === 'true';
  if (typeof original === 'object' && raw.trim().startsWith('{')) {
    try {
      return JSON.parse(raw);
    } catch {
      return original;
    }
  }
  return raw;
}

function normalizeAnnotationFrame(x: number, y: number, width: number, height: number) {
  const scale = Math.max(x, y, width, height) > 100 ? 10 : 1;
  return {
    x: Math.max(0, Math.min(90, x / scale)),
    y: Math.max(0, Math.min(90, y / scale)),
    width: Math.max(4, Math.min(70, width / scale)),
    height: Math.max(4, Math.min(70, height / scale)),
  };
}

function parseCoordinates(value: unknown) {
  if (Array.isArray(value) && value.length >= 4) {
    const [x, y, width, height] = value.map((item) => Number(item));
    if ([x, y, width, height].every((item) => Number.isFinite(item))) {
      return normalizeAnnotationFrame(x, y, width, height);
    }
  }
  if (typeof value === 'string' && value.trim()) {
    try {
      return parseCoordinates(JSON.parse(value));
    } catch {
      const parts = value.split(/[,\s]+/).map((item) => Number(item));
      if (parts.length >= 4 && parts.every((item) => Number.isFinite(item))) {
        return normalizeAnnotationFrame(parts[0], parts[1], parts[2], parts[3]);
      }
    }
  }
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>;
    const x = Number(record.x ?? record.left ?? 0);
    const y = Number(record.y ?? record.top ?? 0);
    const width = Number(record.width ?? record.w ?? 0);
    const height = Number(record.height ?? record.h ?? 0);
    if ([x, y, width, height].every((item) => Number.isFinite(item))) {
      return normalizeAnnotationFrame(x, y, width, height);
    }
  }
  return null;
}

function loadTemplatesFromStorage(): VertexTemplate[] {
  if (typeof localStorage === 'undefined') return [];
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEYS.templates) ?? '[]');
  } catch {
    return [];
  }
}

function loadAnnotationsFromStorage(): Record<string, VertexAnnotation[]> {
  if (typeof localStorage === 'undefined') return {};
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEYS.annotations) ?? '{}');
  } catch {
    return {};
  }
}

export function VertexPage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [visualFunctions, setVisualFunctions] = useState<QuiverVisualFunction[]>([]);
  const [graph, setGraph] = useState<GraphResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [graphLoading, setGraphLoading] = useState(false);
  const [searchLoading, setSearchLoading] = useState(false);
  const [scenarioLoading, setScenarioLoading] = useState(false);
  const [neighborLoading, setNeighborLoading] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [notice, setNotice] = useState('');

  const [rootTypeId, setRootTypeId] = useState('');
  const [rootObjectId, setRootObjectId] = useState('');
  const [depth, setDepth] = useState(2);
  const [layoutMode, setLayoutMode] = useState<LayoutMode>('cose');
  const [nodeDisplayMode, setNodeDisplayMode] = useState<NodeDisplayMode>('compact');
  const [activeTab, setActiveTab] = useState<SidebarTab>('selection');

  const [selectedNodeId, setSelectedNodeId] = useState('');

  const [subtitleField, setSubtitleField] = useState('');
  const [extendedLabelField, setExtendedLabelField] = useState('');
  const [colorByField, setColorByField] = useState('');
  const [timeField, setTimeField] = useState('');
  const [eventStartField, setEventStartField] = useState('');
  const [eventEndField, setEventEndField] = useState('');
  const [mediaField, setMediaField] = useState('');
  const [annotationField, setAnnotationField] = useState('');
  const [selectedLensId, setSelectedLensId] = useState('');

  const [templates, setTemplates] = useState<VertexTemplate[]>(() => loadTemplatesFromStorage());
  const [selectedTemplateId, setSelectedTemplateId] = useState('');
  const [templateName, setTemplateName] = useState('');
  const [templateDescription, setTemplateDescription] = useState('');

  const [neighborResults, setNeighborResults] = useState<NeighborLink[]>([]);
  const [searchAroundFilter, setSearchAroundFilter] = useState('');
  const [globalSearchQuery, setGlobalSearchQuery] = useState('');
  const [globalSearchResults, setGlobalSearchResults] = useState<SearchResult[]>([]);

  const [cachedTypeRows, setCachedTypeRows] = useState<Record<string, ObjectInstance[]>>({});
  const [currentTimeIndex, setCurrentTimeIndex] = useState(0);

  const [customAnnotations, setCustomAnnotations] = useState<Record<string, VertexAnnotation[]>>(() =>
    loadAnnotationsFromStorage(),
  );
  const [annotationLabel, setAnnotationLabel] = useState('');
  const [annotationColor, setAnnotationColor] = useState('#ef4444');
  const [annotationNote, setAnnotationNote] = useState('');
  const [annotationX, setAnnotationX] = useState(18);
  const [annotationY, setAnnotationY] = useState(18);
  const [annotationWidth, setAnnotationWidth] = useState(24);
  const [annotationHeight, setAnnotationHeight] = useState(16);

  const [scenarioDrafts, setScenarioDrafts] = useState<ScenarioDraft[]>([
    {
      name: 'Optimistic case',
      description: 'Improve one modeled input to understand downstream impact.',
      propertyName: '',
      propertyValue: '',
    },
    {
      name: 'Stress case',
      description: 'Apply a second override to compare a more constrained state.',
      propertyName: '',
      propertyValue: '',
    },
  ]);
  const [scenarioResponse, setScenarioResponse] = useState<ObjectScenarioSimulationResponse | null>(null);

  const typeMap = useMemo(() => new Map(objectTypes.map((item) => [item.id, item])), [objectTypes]);
  const selectedNode = useMemo(
    () => graph?.nodes.find((node) => node.id === selectedNodeId) ?? null,
    [graph, selectedNodeId],
  );
  const selectedNodeProperties = useMemo(
    () => (selectedNode ? parseRecord(selectedNode.metadata?.properties) : {}),
    [selectedNode],
  );

  // Persist templates / annotations when they change.
  useEffect(() => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEYS.templates, JSON.stringify(templates));
    }
  }, [templates]);
  useEffect(() => {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEYS.annotations, JSON.stringify(customAnnotations));
    }
  }, [customAnnotations]);

  // Initial catalog load.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoading(true);
      setLoadError('');
      try {
        const [typesResponse, lensResponse] = await Promise.all([
          listObjectTypes({ per_page: 200 }),
          listQuiverVisualFunctions({ per_page: 100, include_shared: true }).catch(() => ({
            data: [] as QuiverVisualFunction[],
            total: 0,
            page: 1,
            per_page: 100,
          })),
        ]);
        if (cancelled) return;
        setObjectTypes(typesResponse.data);
        setVisualFunctions(lensResponse.data);
        if (typesResponse.data[0]) {
          setRootTypeId((current) => current || typesResponse.data[0].id);
        }
      } catch (cause) {
        if (!cancelled) setLoadError(cause instanceof Error ? cause.message : 'Failed to load Vertex');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Auto-select the first matching lens when the root type changes.
  useEffect(() => {
    if (!rootTypeId || selectedLensId) return;
    const matching = visualFunctions.find((lens) => lens.primary_type_id === rootTypeId);
    if (matching) setSelectedLensId(matching.id);
  }, [rootTypeId, visualFunctions, selectedLensId]);

  // Load the graph whenever rootTypeId / rootObjectId / depth change.
  const loadGraph = useCallback(async () => {
    if (!rootTypeId && !rootObjectId) return;
    setGraphLoading(true);
    setLoadError('');
    setScenarioResponse(null);
    try {
      const next = await getOntologyGraph({
        root_object_id: rootObjectId || undefined,
        root_type_id: rootObjectId ? undefined : rootTypeId || undefined,
        depth,
        limit: 120,
      });
      setGraph(next);
      setSelectedNodeId(next.nodes[0]?.id ?? '');
    } catch (cause) {
      setLoadError(cause instanceof Error ? cause.message : 'Failed to load Vertex graph');
      setGraph(null);
    } finally {
      setGraphLoading(false);
    }
  }, [rootTypeId, rootObjectId, depth]);

  useEffect(() => {
    void loadGraph();
  }, [loadGraph]);

  // Cache the rows of the selected node's type.
  useEffect(() => {
    const typeId = selectedTypeIdFromNode(selectedNode);
    if (!typeId || cachedTypeRows[typeId]) return;
    let cancelled = false;
    (async () => {
      const rows: ObjectInstance[] = [];
      let page = 1;
      let total = 0;
      try {
        do {
          const response = await listObjects(typeId, { page, per_page: 100 });
          rows.push(...response.data);
          total = response.total;
          page += 1;
        } while (rows.length < total);
      } catch {
        // ignore — leave cache untouched
      }
      if (!cancelled) setCachedTypeRows((prev) => ({ ...prev, [typeId]: rows }));
    })();
    return () => {
      cancelled = true;
    };
  }, [selectedNode, cachedTypeRows]);

  // Hydrate field defaults whenever the selected node changes.
  useEffect(() => {
    if (!selectedNode) return;
    const properties = parseRecord(selectedNode.metadata?.properties);
    setSubtitleField((current) => current || detectDateField(properties) || Object.keys(properties)[0] || '');
    setExtendedLabelField((current) => current || detectMetricField(properties));
    setColorByField((current) => current || detectMetricField(properties));
    setTimeField((current) => current || detectDateField(properties));
    const temporal = detectTemporalFields(properties);
    setEventStartField((current) => current || temporal.start);
    setEventEndField((current) => current || temporal.end);
    setMediaField((current) => current || detectMediaField(properties));
    setAnnotationField((current) => current || detectAnnotationField(properties));
    setScenarioDrafts((drafts) =>
      drafts[0]?.propertyName
        ? drafts
        : drafts.map((draft) => ({
            ...draft,
            propertyName: detectMetricField(properties) || Object.keys(properties)[0] || '',
          })),
    );
  }, [selectedNode]);

  // ── Derived data ──

  const selectedObjectRows = useMemo(() => {
    const typeId = selectedTypeIdFromNode(selectedNode);
    return typeId ? cachedTypeRows[typeId] ?? [] : [];
  }, [selectedNode, cachedTypeRows]);

  const selectedSeriesRows = useMemo(() => {
    if (!selectedObjectRows.length || !timeField || !extendedLabelField) return [];
    const buckets: Record<string, number> = {};
    for (const row of selectedObjectRows) {
      const bucket = String(row.properties[timeField] ?? '').slice(0, 10);
      if (!bucket) continue;
      const value = numericValue(row.properties[extendedLabelField]);
      if (value === null) continue;
      buckets[bucket] = (buckets[bucket] ?? 0) + value;
    }
    return Object.entries(buckets)
      .map(([date, value]) => ({ date, value: Number(value.toFixed(2)) }))
      .sort((left, right) => left.date.localeCompare(right.date));
  }, [selectedObjectRows, timeField, extendedLabelField]);

  const selectedGroupedRows = useMemo(() => {
    if (!selectedObjectRows.length) return [];
    const key = subtitleField || Object.keys(selectedObjectRows[0].properties)[0] || '';
    const buckets: Record<string, number> = {};
    for (const row of selectedObjectRows) {
      const group = String(row.properties[key] ?? 'Unknown');
      const value = numericValue(row.properties[extendedLabelField]);
      buckets[group] = (buckets[group] ?? 0) + (value ?? 1);
    }
    return Object.entries(buckets)
      .map(([group, value]) => ({ group, value: Number(value.toFixed(2)) }))
      .sort((left, right) => right.value - left.value)
      .slice(0, 12);
  }, [selectedObjectRows, subtitleField, extendedLabelField]);

  const eventRows = useMemo(() => {
    if (!graph || !selectedNode) return [];
    const focus = selectedNode;
    const adjacentIds = graph.edges
      .filter((edge) => edge.source === focus.id || edge.target === focus.id)
      .map((edge) => (edge.source === focus.id ? edge.target : edge.source));
    const currentTime = selectedSeriesRows[currentTimeIndex]?.date;
    return graph.nodes
      .filter((node) => adjacentIds.includes(node.id))
      .map((node) => {
        const properties = parseRecord(node.metadata?.properties);
        const temporal = detectTemporalFields(properties);
        const start = String(properties[eventStartField || temporal.start] ?? '');
        const end = String(properties[eventEndField || temporal.end] ?? '');
        return {
          nodeId: node.id,
          label: node.label,
          start,
          end,
          typeLabel: node.secondary_label ?? node.kind,
          active:
            currentTime != null
              ? start.slice(0, 10) <= currentTime && currentTime <= end.slice(0, 10)
              : false,
        };
      })
      .filter((row) => row.start && row.end);
  }, [graph, selectedNode, selectedSeriesRows, currentTimeIndex, eventStartField, eventEndField]);

  function nodeLabelFor(node: GraphNode) {
    const properties = parseRecord(node.metadata?.properties);
    const parts = [node.label];
    if (subtitleField && properties[subtitleField] != null) parts.push(String(properties[subtitleField]));
    if (extendedLabelField && properties[extendedLabelField] != null) parts.push(String(properties[extendedLabelField]));
    return parts.join('\n');
  }

  function nodeColorFor(node: GraphNode) {
    const properties = parseRecord(node.metadata?.properties);
    if (node.metadata?.scenario_changed === true) return '#f97316';
    if (node.metadata?.scenario_deleted === true) return '#ef4444';
    const eventMatch = eventRows.find((row) => row.nodeId === node.id);
    if (eventMatch?.active) return '#dc2626';
    if (colorByField) {
      const value = numericValue(properties[colorByField]);
      if (value !== null) {
        if (value >= 90) return '#991b1b';
        if (value >= 60) return '#ea580c';
        if (value >= 30) return '#0f766e';
      }
    }
    return node.color || '#2458b8';
  }

  function edgeWidthFor(edge: GraphEdge) {
    if (edge.metadata?.simulated === true) return 3.6;
    if (edge.metadata?.crosses_organization_boundary === true) return 2.8;
    return 1.8;
  }

  // Cytoscape elements + stylesheet, derived from graph + styling fields.
  const cyElements = useMemo<ElementDefinition[]>(() => {
    if (!graph) return [];
    return [
      ...graph.nodes.map((node) => ({
        data: {
          id: node.id,
          label: nodeLabelFor(node),
          color: nodeColorFor(node),
        },
      })),
      ...graph.edges.map((edge) => ({
        data: {
          id: edge.id,
          source: edge.source,
          target: edge.target,
          label: edge.label,
          width: edgeWidthFor(edge),
          lineStyle: edge.metadata?.simulated === true ? 'dashed' : 'solid',
        },
      })),
    ];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [graph, subtitleField, extendedLabelField, colorByField, eventRows]);

  const cyStylesheet = useMemo<StylesheetStyle[]>(() => {
    const fontSize = nodeDisplayMode === 'card' ? 11 : 10;
    const dim = nodeDisplayMode === 'card' ? 60 : 26;
    const padding = nodeDisplayMode === 'card' ? '18px' : '12px';
    const maxWidth = nodeDisplayMode === 'card' ? '150' : '110';
    return [
      {
        selector: 'node',
        style: {
          'background-color': 'data(color)',
          label: 'data(label)',
          color: '#10233f',
          'font-size': fontSize,
          'font-family': 'Georgia, "Times New Roman", serif',
          'text-wrap': 'wrap',
          'text-max-width': maxWidth,
          'text-valign': 'center',
          'text-halign': 'center',
          width: dim,
          height: dim,
          shape: 'round-rectangle',
          padding,
          'border-width': 1.4,
          'border-color': '#d6dfef',
        },
      },
      {
        selector: 'node:selected',
        style: { 'border-color': '#0f172a', 'border-width': 4 },
      },
      {
        selector: 'edge',
        style: {
          label: 'data(label)',
          width: 'data(width)' as unknown as number,
          'line-color': '#94a3b8',
          'target-arrow-color': '#94a3b8',
          'target-arrow-shape': 'triangle',
          'curve-style': 'bezier',
          'line-style': 'data(lineStyle)' as unknown as 'solid',
          'font-size': 9,
          color: '#64748b',
          'text-rotation': 'autorotate',
        },
      },
    ];
  }, [nodeDisplayMode]);

  const cyLayout = useMemo(
    () => ({ name: layoutMode, animate: true, padding: 36 }) as Parameters<typeof CytoscapeCanvas>[0]['layout'],
    [layoutMode],
  );

  const handleCytoscapeReady = useCallback((cy: Core) => {
    cy.on('tap', 'node', (event: EventObject) => {
      setSelectedNodeId(String(event.target.id()));
    });
  }, []);

  // ── Actions ──

  function applyTemplate(template: VertexTemplate) {
    setSelectedTemplateId(template.id);
    setTemplateName(template.name);
    setTemplateDescription(template.description);
    setRootTypeId(template.rootTypeId);
    setRootObjectId(template.rootObjectId);
    setDepth(template.depth);
    setLayoutMode(template.layout);
    setNodeDisplayMode(template.nodeDisplayMode);
    setSubtitleField(template.subtitleField);
    setExtendedLabelField(template.extendedLabelField);
    setColorByField(template.colorByField);
    setTimeField(template.timeField);
    setEventStartField(template.eventStartField);
    setEventEndField(template.eventEndField);
    setMediaField(template.mediaField);
    setAnnotationField(template.annotationField);
    setSelectedLensId(template.sharedLensId);
  }

  function saveTemplate() {
    const next: VertexTemplate = {
      id: selectedTemplateId || createId(),
      name: templateName.trim() || `${typeMap.get(rootTypeId)?.display_name ?? 'Vertex'} template`,
      description: templateDescription.trim(),
      rootTypeId,
      rootObjectId,
      depth,
      layout: layoutMode,
      nodeDisplayMode,
      subtitleField,
      extendedLabelField,
      colorByField,
      timeField,
      eventStartField,
      eventEndField,
      mediaField,
      annotationField,
      sharedLensId: selectedLensId,
      createdAt: selectedTemplateId
        ? templates.find((item) => item.id === selectedTemplateId)?.createdAt ?? new Date().toISOString()
        : new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    setSelectedTemplateId(next.id);
    setTemplates([next, ...templates.filter((item) => item.id !== next.id)]);
    setNotice(`Saved Vertex template "${next.name}".`);
  }

  function deleteTemplate(id: string) {
    setTemplates((prev) => prev.filter((item) => item.id !== id));
    if (selectedTemplateId === id) {
      setSelectedTemplateId('');
      setTemplateName('');
      setTemplateDescription('');
    }
  }

  async function runGlobalSearch() {
    const query = globalSearchQuery.trim();
    if (!query) {
      setGlobalSearchResults([]);
      return;
    }
    setSearchLoading(true);
    try {
      const response = await searchOntology({
        query,
        object_type_id: rootTypeId || undefined,
        semantic: true,
        limit: 10,
      });
      setGlobalSearchResults(response.data);
    } catch (cause) {
      setLoadError(cause instanceof Error ? cause.message : 'Failed to search graph resources');
    } finally {
      setSearchLoading(false);
    }
  }

  async function loadNeighborsForSelection() {
    if (!selectedNode) return;
    const objectId = parseObjectId(selectedNode);
    const typeId = selectedTypeIdFromNode(selectedNode);
    if (!objectId || !typeId) return;
    setNeighborLoading(true);
    try {
      const next = await listNeighbors(typeId, objectId);
      setNeighborResults(next);
    } catch (cause) {
      setLoadError(cause instanceof Error ? cause.message : 'Failed to search around the selected node');
    } finally {
      setNeighborLoading(false);
    }
  }

  function addNeighborToGraph(neighbor: NeighborLink) {
    if (!graph || !selectedNode) return;
    const typeItem = typeMap.get(neighbor.object.object_type_id);
    const nodeId = `object:${neighbor.object.id}`;
    let next = graph;
    if (!next.nodes.some((node) => node.id === nodeId)) {
      const newNode: GraphNode = {
        id: nodeId,
        kind: 'object_instance',
        label: objectLabelFromProperties(neighbor.object.properties),
        secondary_label: typeItem?.display_name ?? neighbor.object.object_type_id,
        color: typeItem?.color ?? null,
        route: `/ontology/${neighbor.object.object_type_id}#object-${neighbor.object.id}`,
        metadata: {
          object_type_id: neighbor.object.object_type_id,
          properties: neighbor.object.properties,
        },
      };
      next = { ...next, nodes: [...next.nodes, newNode], total_nodes: next.total_nodes + 1 };
    }
    const edgeId = `neighbor:${neighbor.link_id}:${selectedNode.id}:${neighbor.object.id}`;
    if (!next.edges.some((edge) => edge.id === edgeId)) {
      const nextEdge: GraphEdge = {
        id: edgeId,
        kind: 'link_instance',
        source: neighbor.direction === 'outbound' ? selectedNode.id : nodeId,
        target: neighbor.direction === 'outbound' ? nodeId : selectedNode.id,
        label: neighbor.link_name,
        metadata: { link_type_id: neighbor.link_type_id, search_around: true },
      };
      next = { ...next, edges: [...next.edges, nextEdge], total_edges: next.total_edges + 1 };
    }
    setGraph(next);
  }

  function addSearchResultToGraph(result: SearchResult) {
    if (!result.object_type_id) return;
    setRootTypeId(result.object_type_id);
    if (result.kind === 'object_instance') setRootObjectId(result.id);
  }

  async function runScenarios() {
    if (!selectedNode) return;
    const objectId = parseObjectId(selectedNode);
    const typeId = selectedTypeIdFromNode(selectedNode);
    if (!objectId || !typeId) return;
    setScenarioLoading(true);
    try {
      const candidates: ScenarioSimulationCandidate[] = scenarioDrafts
        .filter((draft) => draft.name.trim() && draft.propertyName.trim())
        .map((draft) => {
          const original = selectedNodeProperties[draft.propertyName];
          return {
            name: draft.name.trim(),
            description: draft.description.trim(),
            operations: [
              { properties_patch: { [draft.propertyName]: coerceScenarioValue(draft.propertyValue, original) } },
            ],
          };
        });
      const response = await simulateObjectScenarios(typeId, objectId, {
        scenarios: candidates,
        include_baseline: true,
      });
      setScenarioResponse(response);
      setNotice(
        `Simulated ${response.scenarios.length} Vertex scenario${
          response.scenarios.length === 1 ? '' : 's'
        }.`,
      );
    } catch (cause) {
      setLoadError(cause instanceof Error ? cause.message : 'Failed to simulate scenarios');
    } finally {
      setScenarioLoading(false);
    }
  }

  // ── Annotations ──

  function activeMediaUrl() {
    const value = selectedNodeProperties[mediaField];
    if (typeof value === 'string') return value;
    const typeLabel = typeMap.get(selectedTypeIdFromNode(selectedNode))?.display_name ?? 'system';
    const svg = encodeURIComponent(
      `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 900 520"><rect width="900" height="520" fill="#1e3a8a"/><text x="64" y="64" fill="#e2e8f0" font-size="28" font-family="Georgia, serif">Vertex media layer</text><text x="64" y="100" fill="#93c5fd" font-size="18" font-family="Georgia, serif">${typeLabel}</text></svg>`,
    );
    return `data:image/svg+xml;charset=UTF-8,${svg}`;
  }

  const graphAnnotations = useMemo<VertexAnnotation[]>(() => {
    const annotations: VertexAnnotation[] = [];
    const selectedId = parseObjectId(selectedNode);
    annotations.push(...(selectedId ? customAnnotations[selectedId] ?? [] : []));
    if (!graph || !annotationField) return annotations;
    for (const node of graph.nodes) {
      const properties = parseRecord(node.metadata?.properties);
      const parsed = parseCoordinates(properties[annotationField]);
      if (!parsed) continue;
      annotations.push({
        id: `graph-${node.id}`,
        label: node.label,
        x: parsed.x,
        y: parsed.y,
        width: parsed.width,
        height: parsed.height,
        color: '#38bdf8',
        note: node.secondary_label ?? '',
      });
    }
    return annotations;
  }, [selectedNode, customAnnotations, graph, annotationField]);

  function addAnnotation() {
    const selectedId = parseObjectId(selectedNode);
    if (!selectedId || !annotationLabel.trim()) return;
    const next: VertexAnnotation = {
      id: createId(),
      label: annotationLabel.trim(),
      x: annotationX,
      y: annotationY,
      width: annotationWidth,
      height: annotationHeight,
      color: annotationColor,
      note: annotationNote.trim(),
    };
    setCustomAnnotations((prev) => ({
      ...prev,
      [selectedId]: [...(prev[selectedId] ?? []), next],
    }));
    setAnnotationLabel('');
    setAnnotationNote('');
  }

  function removeAnnotation(id: string) {
    const selectedId = parseObjectId(selectedNode);
    if (!selectedId) return;
    setCustomAnnotations((prev) => ({
      ...prev,
      [selectedId]: (prev[selectedId] ?? []).filter((item) => item.id !== id),
    }));
  }

  function currentTimeLabel() {
    return selectedSeriesRows[currentTimeIndex]?.date ?? 'No timeline';
  }

  const selectedLens = useMemo(
    () => visualFunctions.find((item) => item.id === selectedLensId) ?? null,
    [visualFunctions, selectedLensId],
  );

  // ── Render ──

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div
        className="of-panel"
        style={{
          padding: 24,
          background: 'linear-gradient(135deg, #081428 0%, #10284f 52%, #1d4f91 100%)',
          color: '#fff',
        }}
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#bae6fd' }}>
              Vertex
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8, color: '#fff' }}>
              Visualize, simulate, and annotate your digital twin as a dedicated graph product.
            </h1>
            <p style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7, color: '#e0f2fe' }}>
              Graph exploration, graph templates, event badges, time-series sidecars, media layers,
              and what-if scenario simulation in one product.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', minWidth: 360 }}>
            {[
              { label: 'Templates', value: templates.length },
              { label: 'Graph nodes', value: graph?.total_nodes ?? 0 },
              { label: 'Scenarios', value: scenarioResponse?.scenarios.length ?? 0 },
            ].map((stat) => (
              <div
                key={stat.label}
                style={{
                  borderRadius: 'var(--radius-md)',
                  border: '1px solid rgba(255,255,255,0.18)',
                  background: 'rgba(255,255,255,0.08)',
                  padding: 12,
                }}
              >
                <p style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#bae6fd' }}>
                  {stat.label}
                </p>
                <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600, color: '#fff' }}>{stat.value}</p>
              </div>
            ))}
          </div>
        </div>
      </div>

      {loadError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {loadError}
        </div>
      )}
      {notice && (
        <div className="of-status-success" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {notice}
        </div>
      )}

      <div className="of-panel" style={{ padding: 20 }}>
        <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
          <Field label="Object type">
            <select className="of-select" value={rootTypeId} onChange={(e) => setRootTypeId(e.target.value)}>
              {objectTypes.map((typeItem) => (
                <option key={typeItem.id} value={typeItem.id}>
                  {typeItem.display_name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Root object id">
            <input
              className="of-input"
              value={rootObjectId}
              onChange={(e) => setRootObjectId(e.target.value)}
              placeholder="Optional object UUID"
            />
          </Field>
          <Field label="Depth">
            <input
              type="number"
              className="of-input"
              value={depth}
              onChange={(e) => setDepth(Number(e.target.value))}
              min={1}
              max={4}
            />
          </Field>
          <Field label="Layout">
            <select className="of-select" value={layoutMode} onChange={(e) => setLayoutMode(e.target.value as LayoutMode)}>
              {LAYOUT_OPTIONS.map((option) => (
                <option key={option.id} value={option.id}>
                  {option.label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Node mode">
            <select
              className="of-select"
              value={nodeDisplayMode}
              onChange={(e) => setNodeDisplayMode(e.target.value as NodeDisplayMode)}
            >
              <option value="compact">Compact</option>
              <option value="card">Object card</option>
            </select>
          </Field>
          <Field label="Quiver lens">
            <select className="of-select" value={selectedLensId} onChange={(e) => setSelectedLensId(e.target.value)}>
              <option value="">No shared lens</option>
              {visualFunctions.map((lens) => (
                <option key={lens.id} value={lens.id}>
                  {lens.name}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 16, alignItems: 'center' }}>
          <input
            className="of-input"
            value={globalSearchQuery}
            onChange={(e) => setGlobalSearchQuery(e.target.value)}
            placeholder="Find objects or types"
            style={{ flex: 1, minWidth: 240 }}
          />
          <button type="button" className="of-btn" onClick={() => void runGlobalSearch()} disabled={searchLoading}>
            {searchLoading ? '…' : 'Search'}
          </button>
          <button
            type="button"
            className="of-btn of-btn-primary"
            onClick={() => void loadGraph()}
            disabled={graphLoading}
          >
            {graphLoading ? 'Loading…' : 'Load graph'}
          </button>
          <button type="button" className="of-btn" onClick={saveTemplate}>
            Save as template
          </button>
        </div>

        {globalSearchResults.length > 0 && (
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', marginTop: 16 }}>
            {globalSearchResults.map((result) => (
              <button
                key={`${result.kind}-${result.id}`}
                type="button"
                onClick={() => addSearchResultToGraph(result)}
                style={{
                  textAlign: 'left',
                  padding: 16,
                  border: '1px solid var(--border-default)',
                  borderRadius: 'var(--radius-md)',
                  background: 'var(--bg-panel-muted)',
                  cursor: 'pointer',
                }}
              >
                <p className="of-eyebrow">{result.kind.replaceAll('_', ' ')}</p>
                <div style={{ marginTop: 8, fontWeight: 500, color: 'var(--text-strong)' }}>{result.title}</div>
                <div className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                  {result.subtitle ?? result.snippet}
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.42fr) 380px' }}>
        <div style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ padding: 0, overflow: 'hidden' }}>
            <div
              style={{
                display: 'flex',
                flexWrap: 'wrap',
                alignItems: 'center',
                justifyContent: 'space-between',
                gap: 12,
                borderBottom: '1px solid var(--border-subtle)',
                padding: '16px 20px',
              }}
            >
              <div>
                <p className="of-eyebrow">Graph canvas</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Editable system graph
                </h2>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                <span className="of-chip of-status-info">Nodes {graph?.total_nodes ?? 0}</span>
                <span className="of-chip of-status-success">Edges {graph?.total_edges ?? 0}</span>
                <span className="of-chip of-status-warning">Timeline {currentTimeLabel()}</span>
              </div>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 270px' }}>
              <div style={{ position: 'relative', borderRight: '1px solid var(--border-subtle)' }}>
                <CytoscapeCanvas
                  elements={cyElements}
                  stylesheet={cyStylesheet}
                  layout={cyLayout}
                  height={640}
                  onReady={handleCytoscapeReady}
                />
                {graphLoading && (
                  <div
                    style={{
                      position: 'absolute',
                      inset: 0,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      background: 'rgba(255,255,255,0.7)',
                      fontSize: 13,
                      color: 'var(--text-muted)',
                    }}
                  >
                    Loading Vertex graph…
                  </div>
                )}
              </div>
              <div style={{ display: 'grid', gap: 16, padding: 16 }}>
                <div className="of-panel-muted" style={{ padding: 16 }}>
                  <p className="of-eyebrow">Graph template</p>
                  <input
                    className="of-input"
                    value={templateName}
                    onChange={(e) => setTemplateName(e.target.value)}
                    placeholder="Template name"
                    style={{ marginTop: 12, fontSize: 13 }}
                  />
                  <textarea
                    className="of-textarea"
                    value={templateDescription}
                    onChange={(e) => setTemplateDescription(e.target.value)}
                    placeholder="Describe when to reuse this template"
                    style={{ marginTop: 12, fontSize: 13, minHeight: 80 }}
                  />
                  <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
                    {templates.slice(0, 6).map((template) => (
                      <div
                        key={template.id}
                        style={{
                          border: '1px solid var(--border-default)',
                          borderRadius: 'var(--radius-md)',
                          background: '#fff',
                          padding: 12,
                        }}
                      >
                        <button
                          type="button"
                          onClick={() => applyTemplate(template)}
                          style={{
                            width: '100%',
                            textAlign: 'left',
                            background: 'transparent',
                            border: 0,
                            cursor: 'pointer',
                            padding: 0,
                          }}
                        >
                          <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{template.name}</div>
                          <div className="of-text-muted" style={{ fontSize: 12, marginTop: 4 }}>
                            {template.updatedAt.slice(0, 10)} ·{' '}
                            {LAYOUT_OPTIONS.find((item) => item.id === template.layout)?.label}
                          </div>
                        </button>
                        <button
                          type="button"
                          className="of-btn of-btn-danger"
                          onClick={() => deleteTemplate(template.id)}
                          style={{ marginTop: 12, minHeight: 28, fontSize: 11 }}
                        >
                          Delete
                        </button>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="of-panel-muted" style={{ padding: 16 }}>
                  <p className="of-eyebrow">Search around</p>
                  <button
                    type="button"
                    className="of-btn of-btn-primary"
                    onClick={() => void loadNeighborsForSelection()}
                    disabled={neighborLoading || !selectedNode}
                    style={{ marginTop: 12, width: '100%' }}
                  >
                    {neighborLoading ? 'Loading…' : 'Load related objects'}
                  </button>
                  <input
                    className="of-input"
                    value={searchAroundFilter}
                    onChange={(e) => setSearchAroundFilter(e.target.value)}
                    placeholder="Filter neighbors"
                    style={{ marginTop: 12, fontSize: 13 }}
                  />
                  <div style={{ display: 'grid', gap: 8, marginTop: 12, maxHeight: 240, overflowY: 'auto' }}>
                    {neighborResults
                      .filter((neighbor) => {
                        const query = searchAroundFilter.trim().toLowerCase();
                        if (!query) return true;
                        return `${neighbor.link_name} ${objectLabelFromProperties(neighbor.object.properties)}`
                          .toLowerCase()
                          .includes(query);
                      })
                      .map((neighbor) => (
                        <button
                          key={neighbor.link_id}
                          type="button"
                          onClick={() => addNeighborToGraph(neighbor)}
                          style={{
                            textAlign: 'left',
                            padding: 12,
                            border: '1px solid var(--border-default)',
                            background: '#fff',
                            borderRadius: 'var(--radius-md)',
                            cursor: 'pointer',
                          }}
                        >
                          <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>
                            {objectLabelFromProperties(neighbor.object.properties)}
                          </div>
                          <div className="of-text-muted" style={{ fontSize: 12, marginTop: 4 }}>
                            {neighbor.direction} via {neighbor.link_name}
                          </div>
                        </button>
                      ))}
                    {!neighborLoading && neighborResults.length === 0 && (
                      <div
                        style={{
                          border: '1px dashed var(--border-default)',
                          borderRadius: 'var(--radius-md)',
                          padding: '12px 16px',
                          fontSize: 13,
                          color: 'var(--text-muted)',
                        }}
                      >
                        Load neighbors from the current selection to expand the graph.
                      </div>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </section>

          <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr' }}>
            <section className="of-panel" style={{ padding: 24 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                <div>
                  <p className="of-eyebrow">Time series</p>
                  <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                    Series view
                  </h2>
                </div>
                {selectedLens && (
                  <span className="of-chip of-status-info">{selectedLens.name}</span>
                )}
              </div>
              <div style={{ marginTop: 16, height: 320 }}>
                <EChartView
                  rows={selectedSeriesRows.map((row) => ({ date: row.date, value: row.value }))}
                  categoryKey="date"
                  valueKeys={['value']}
                  mode="line"
                  emptyLabel="Select an object-backed node with time and numeric properties to open the series view."
                  onCategoryClick={(value) => {
                    const index = selectedSeriesRows.findIndex((row) => row.date === value);
                    setCurrentTimeIndex(index >= 0 ? index : 0);
                  }}
                />
              </div>
              {selectedSeriesRows.length > 1 && (
                <input
                  type="range"
                  min={0}
                  max={Math.max(0, selectedSeriesRows.length - 1)}
                  value={currentTimeIndex}
                  onChange={(e) => setCurrentTimeIndex(Number(e.target.value))}
                  style={{ marginTop: 16, width: '100%', accentColor: '#2458b8' }}
                />
              )}
            </section>

            <section className="of-panel" style={{ padding: 24 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                <div>
                  <p className="of-eyebrow">Graph readouts</p>
                  <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                    Histogram and extended labels
                  </h2>
                </div>
                <span className="of-text-muted" style={{ fontSize: 12 }}>
                  {subtitleField || 'No group field yet'}
                </span>
              </div>
              <div style={{ marginTop: 16, height: 320 }}>
                <EChartView
                  rows={selectedGroupedRows.map((row) => ({ group: row.group, value: row.value }))}
                  categoryKey="group"
                  valueKeys={['value']}
                  mode="bar"
                  emptyLabel="Select a node to derive grouped readouts from its object type."
                />
              </div>
            </section>
          </div>
        </div>

        <aside style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ padding: 12 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {SIDEBAR_TABS.map((tab) => (
                <button
                  key={tab.id}
                  type="button"
                  className={activeTab === tab.id ? 'of-btn of-btn-primary' : 'of-btn'}
                  onClick={() => setActiveTab(tab.id)}
                  style={{ minHeight: 30, fontSize: 12, padding: '0 10px' }}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          </section>

          {activeTab === 'selection' && (
            <SidebarSection title="Selection" subtitle={selectedNode?.label ?? 'Choose a node'}>
              <p className="of-text-muted" style={{ fontSize: 13 }}>
                {selectedNode?.secondary_label ?? 'No object selected.'}
              </p>
              <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
                {Object.entries(selectedNodeProperties)
                  .slice(0, 12)
                  .map(([key, value]) => (
                    <div key={key} className="of-panel-muted" style={{ padding: '12px 16px' }}>
                      <p className="of-eyebrow">{key}</p>
                      <p style={{ marginTop: 8, fontSize: 13, color: 'var(--text-strong)' }}>
                        {stringifyValue(value)}
                      </p>
                    </div>
                  ))}
              </div>
              {selectedNode?.route && (
                <a
                  href={selectedNode.route}
                  className="of-btn"
                  style={{ display: 'inline-flex', marginTop: 16, fontSize: 13 }}
                >
                  Open source object
                </a>
              )}
            </SidebarSection>
          )}

          {activeTab === 'events' && (
            <SidebarSection title="Associated events" subtitle="Timeline-aware event badges">
              <p className="of-text-muted" style={{ fontSize: 13, marginTop: 8 }}>
                Neighbor objects with start and end timestamps are surfaced as Vertex-style events
                around the current selection.
              </p>
              <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
                {eventRows.map((row) => (
                  <button
                    key={row.nodeId}
                    type="button"
                    onClick={() => setSelectedNodeId(row.nodeId)}
                    style={{
                      width: '100%',
                      textAlign: 'left',
                      padding: 16,
                      border: `1px solid ${row.active ? '#fecaca' : 'var(--border-default)'}`,
                      background: row.active ? '#fff5f5' : 'var(--bg-panel-muted)',
                      borderRadius: 'var(--radius-md)',
                      cursor: 'pointer',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
                      <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{row.label}</div>
                      <span
                        className={`of-chip ${row.active ? 'of-status-danger' : 'of-status-info'}`}
                        style={{ fontSize: 11, fontWeight: 600 }}
                      >
                        {row.active ? 'Active' : 'Scheduled'}
                      </span>
                    </div>
                    <div className="of-text-muted" style={{ fontSize: 12, marginTop: 8 }}>
                      {row.typeLabel} · {row.start.slice(0, 10)} → {row.end.slice(0, 10)}
                    </div>
                  </button>
                ))}
                {eventRows.length === 0 && (
                  <div
                    style={{
                      border: '1px dashed var(--border-default)',
                      borderRadius: 'var(--radius-md)',
                      padding: '12px 16px',
                      fontSize: 13,
                      color: 'var(--text-muted)',
                    }}
                  >
                    No temporal neighbors were detected for the current selection.
                  </div>
                )}
              </div>
            </SidebarSection>
          )}

          {activeTab === 'series' && (
            <SidebarSection title="Series handoff" subtitle="Time-series actions">
              <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
                <div className="of-panel-muted" style={{ padding: 16 }}>
                  <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>Current timeline point</p>
                  <p className="of-text-muted" style={{ fontSize: 13, marginTop: 8 }}>
                    {currentTimeLabel()}
                  </p>
                </div>
                <div className="of-panel-muted" style={{ padding: 16 }}>
                  <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>Series field</p>
                  <p className="of-text-muted" style={{ fontSize: 13, marginTop: 8 }}>
                    {timeField || 'Set a time field in Layers'}
                  </p>
                </div>
                <a href="/quiver" className="of-btn" style={{ fontSize: 13 }}>
                  Open in Quiver
                </a>
              </div>
            </SidebarSection>
          )}

          {activeTab === 'layers' && (
            <SidebarSection title="Layer styling" subtitle="Object and edge display options">
              <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
                <Field label="Subtitle field">
                  <input className="of-input" value={subtitleField} onChange={(e) => setSubtitleField(e.target.value)} />
                </Field>
                <Field label="Extended label field">
                  <input
                    className="of-input"
                    value={extendedLabelField}
                    onChange={(e) => setExtendedLabelField(e.target.value)}
                  />
                </Field>
                <Field label="Color by field">
                  <input className="of-input" value={colorByField} onChange={(e) => setColorByField(e.target.value)} />
                </Field>
                <Field label="Time field">
                  <input className="of-input" value={timeField} onChange={(e) => setTimeField(e.target.value)} />
                </Field>
                <Field label="Event start / end">
                  <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
                    <input
                      className="of-input"
                      value={eventStartField}
                      onChange={(e) => setEventStartField(e.target.value)}
                      placeholder="start"
                    />
                    <input
                      className="of-input"
                      value={eventEndField}
                      onChange={(e) => setEventEndField(e.target.value)}
                      placeholder="end"
                    />
                  </div>
                </Field>
              </div>
            </SidebarSection>
          )}

          {activeTab === 'media' && (
            <SidebarSection title="Media layers" subtitle="Image annotations and overlays">
              <div
                style={{
                  marginTop: 16,
                  overflow: 'hidden',
                  borderRadius: 'var(--radius-md)',
                  border: '1px solid var(--border-default)',
                  background: '#0f172a',
                }}
              >
                <div style={{ position: 'relative', aspectRatio: '16 / 10', width: '100%', overflow: 'hidden' }}>
                  <img
                    src={activeMediaUrl()}
                    alt="Vertex media layer"
                    style={{ height: '100%', width: '100%', objectFit: 'cover' }}
                  />
                  {graphAnnotations.map((annotation) => (
                    <div
                      key={annotation.id}
                      style={{
                        position: 'absolute',
                        left: `${annotation.x}%`,
                        top: `${annotation.y}%`,
                        width: `${annotation.width}%`,
                        height: `${annotation.height}%`,
                        border: `2px solid ${annotation.color}`,
                        background: `${annotation.color}22`,
                        borderRadius: 8,
                      }}
                    >
                      <span
                        style={{
                          position: 'absolute',
                          left: 4,
                          top: 4,
                          padding: '2px 6px',
                          background: 'rgba(0,0,0,0.6)',
                          borderRadius: 4,
                          fontSize: 10,
                          fontWeight: 600,
                          color: '#fff',
                        }}
                      >
                        {annotation.label}
                      </span>
                    </div>
                  ))}
                </div>
              </div>

              <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
                <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr' }}>
                  <input
                    className="of-input"
                    value={mediaField}
                    onChange={(e) => setMediaField(e.target.value)}
                    placeholder="media property"
                  />
                  <input
                    className="of-input"
                    value={annotationField}
                    onChange={(e) => setAnnotationField(e.target.value)}
                    placeholder="annotation property"
                  />
                </div>
                <input
                  className="of-input"
                  value={annotationLabel}
                  onChange={(e) => setAnnotationLabel(e.target.value)}
                  placeholder="annotation label"
                />
                <textarea
                  className="of-textarea"
                  value={annotationNote}
                  onChange={(e) => setAnnotationNote(e.target.value)}
                  placeholder="note"
                  style={{ minHeight: 70 }}
                />
                <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(4, 1fr)' }}>
                  <input
                    type="number"
                    className="of-input"
                    value={annotationX}
                    onChange={(e) => setAnnotationX(Number(e.target.value))}
                    placeholder="x %"
                  />
                  <input
                    type="number"
                    className="of-input"
                    value={annotationY}
                    onChange={(e) => setAnnotationY(Number(e.target.value))}
                    placeholder="y %"
                  />
                  <input
                    type="number"
                    className="of-input"
                    value={annotationWidth}
                    onChange={(e) => setAnnotationWidth(Number(e.target.value))}
                    placeholder="w %"
                  />
                  <input
                    type="number"
                    className="of-input"
                    value={annotationHeight}
                    onChange={(e) => setAnnotationHeight(Number(e.target.value))}
                    placeholder="h %"
                  />
                </div>
                <input
                  type="color"
                  value={annotationColor}
                  onChange={(e) => setAnnotationColor(e.target.value)}
                  style={{ height: 44, width: '100%', border: '1px solid var(--border-default)', borderRadius: 'var(--radius-sm)' }}
                />
                <button type="button" className="of-btn of-btn-primary" onClick={addAnnotation}>
                  Create annotation
                </button>
                <div style={{ display: 'grid', gap: 8 }}>
                  {graphAnnotations
                    .filter((item) => item.id && !item.id.startsWith('graph-'))
                    .map((annotation) => (
                      <div
                        key={annotation.id}
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          gap: 12,
                          border: '1px solid var(--border-default)',
                          borderRadius: 'var(--radius-md)',
                          background: 'var(--bg-panel-muted)',
                          padding: 12,
                        }}
                      >
                        <div>
                          <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{annotation.label}</div>
                          <div className="of-text-muted" style={{ fontSize: 12, marginTop: 4 }}>
                            {annotation.note || 'No note'}
                          </div>
                        </div>
                        <button
                          type="button"
                          className="of-btn of-btn-danger"
                          onClick={() => removeAnnotation(annotation.id)}
                          style={{ minHeight: 28, fontSize: 11 }}
                        >
                          Remove
                        </button>
                      </div>
                    ))}
                </div>
              </div>
            </SidebarSection>
          )}

          {activeTab === 'scenarios' && (
            <SidebarSection title="Scenarios" subtitle="What-if simulation">
              <p className="of-text-muted" style={{ fontSize: 13, marginTop: 8 }}>
                Run multi-case overrides on the selected root object and inspect graph deltas,
                impacted objects, and rule outcomes.
              </p>
              <div style={{ display: 'grid', gap: 16, marginTop: 16 }}>
                {scenarioDrafts.map((draft, index) => (
                  <div key={index} className="of-panel-muted" style={{ padding: 16 }}>
                    <input
                      className="of-input"
                      value={draft.name}
                      onChange={(e) =>
                        setScenarioDrafts((prev) => prev.map((item, i) => (i === index ? { ...item, name: e.target.value } : item)))
                      }
                    />
                    <textarea
                      className="of-textarea"
                      value={draft.description}
                      onChange={(e) =>
                        setScenarioDrafts((prev) =>
                          prev.map((item, i) => (i === index ? { ...item, description: e.target.value } : item)),
                        )
                      }
                      style={{ marginTop: 12, minHeight: 70, fontSize: 13 }}
                    />
                    <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr', marginTop: 12 }}>
                      <input
                        className="of-input"
                        value={draft.propertyName}
                        onChange={(e) =>
                          setScenarioDrafts((prev) =>
                            prev.map((item, i) => (i === index ? { ...item, propertyName: e.target.value } : item)),
                          )
                        }
                        placeholder="property"
                      />
                      <input
                        className="of-input"
                        value={draft.propertyValue}
                        onChange={(e) =>
                          setScenarioDrafts((prev) =>
                            prev.map((item, i) => (i === index ? { ...item, propertyValue: e.target.value } : item)),
                          )
                        }
                        placeholder="override"
                      />
                    </div>
                  </div>
                ))}
                <button
                  type="button"
                  className="of-btn of-btn-primary"
                  onClick={() => void runScenarios()}
                  disabled={scenarioLoading || !selectedNode}
                >
                  {scenarioLoading ? 'Simulating…' : 'Run scenarios'}
                </button>
              </div>

              {scenarioResponse && (
                <div style={{ display: 'grid', gap: 12, marginTop: 20 }}>
                  {scenarioResponse.scenarios.map((result) => (
                    <div key={result.scenario_id} className="of-panel-muted" style={{ padding: 16 }}>
                      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                        <div>
                          <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{result.name}</div>
                          <div className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
                            {result.description ?? 'Scenario result'}
                          </div>
                        </div>
                        <span className="of-chip of-status-info" style={{ fontSize: 11, fontWeight: 600 }}>
                          Goal {result.summary.goal_score}
                        </span>
                      </div>
                      <div style={{ display: 'grid', gap: 8, gridTemplateColumns: '1fr 1fr', marginTop: 12, fontSize: 13 }}>
                        <Stat label="Changed" value={result.summary.changed_object_count} />
                        <Stat label="Schedules" value={result.summary.schedule_count} />
                        <Stat label="Advisory" value={result.summary.advisory_rule_matches} />
                        <Stat label="Boundaries" value={result.summary.boundary_crossings} />
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </SidebarSection>
          )}
        </aside>
      </div>

      {loading && (
        <div className="of-text-muted" style={{ fontSize: 13, textAlign: 'center', padding: 16 }}>
          Loading Vertex…
        </div>
      )}
    </section>
  );
}

interface FieldProps {
  label: string;
  children: React.ReactNode;
}

function Field({ label, children }: FieldProps) {
  return (
    <label style={{ display: 'block', fontSize: 13 }}>
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      {children}
    </label>
  );
}

interface SidebarSectionProps {
  title: string;
  subtitle: string;
  children: React.ReactNode;
}

function SidebarSection({ title, subtitle, children }: SidebarSectionProps) {
  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">{title}</p>
      <h2 className="of-heading-md" style={{ marginTop: 4 }}>
        {subtitle}
      </h2>
      {children}
    </section>
  );
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div
      style={{
        border: '1px solid var(--border-default)',
        borderRadius: 'var(--radius-sm)',
        background: '#fff',
        padding: '8px 12px',
      }}
    >
      <span className="of-text-muted">{label}</span> {value}
    </div>
  );
}
