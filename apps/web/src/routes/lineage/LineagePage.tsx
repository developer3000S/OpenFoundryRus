import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { Core, ElementDefinition, EventObject, StylesheetStyle } from 'cytoscape';

import { CytoscapeCanvas } from '@components/CytoscapeCanvas';
import { ScheduleSidebar } from '@/lib/components/lineage/ScheduleSidebar';
import { loadJobSpecStatus, previewDataset, type DatasetPreviewResponse } from '@/lib/api/datasets';
import {
  getDatasetLineageImpact,
  getFullLineage,
  triggerLineageBuilds,
  type LineageBuildResult,
  type LineageGraph,
  type LineageImpactAnalysis,
  type LineageNode,
} from '@/lib/api/pipelines';
import { notifications } from '@stores/notifications';

// =============================================================================
// Coloring schemes — Foundry exposes "resource type", "marking" and (here)
// "build status" so node fill conveys the most useful axis at a glance.
// =============================================================================

type ColoringMode = 'resource_type' | 'marking' | 'build_status';

const COLORING_OPTIONS: { value: ColoringMode; label: string }[] = [
  { value: 'resource_type', label: 'Resource type' },
  { value: 'marking', label: 'Marking' },
  { value: 'build_status', label: 'Build status' },
];

// Pastel palette tuned to match Foundry Data Lineage screenshots.
const RESOURCE_PALETTE: Record<string, string> = {
  dataset: '#d4ec97',
  pipeline: '#b3def0',
  workflow: '#f7c79b',
  object_type: '#c1cfee',
  virtual_table: '#cfe7c4',
  artifact: '#e6dfb6',
};

const MARKING_PALETTE: Record<string, string> = {
  public: '#cbd5e1',
  confidential: '#f4a366',
  pii: '#f08585',
};

const BUILD_STATUS_PALETTE = {
  hasMaster: '#94d39e',
  noMaster: '#cdd5dd',
  notDataset: '#dde2e8',
};

const FALLBACK_COLOR = '#cdd5dd';

function nodeBorderColor(marking: string) {
  return MARKING_PALETTE[marking] ?? '#9aa3ad';
}

function resourceColor(kind: string) {
  return RESOURCE_PALETTE[kind] ?? FALLBACK_COLOR;
}

function buildStatusColor(node: LineageNode, jobSpecMap: Record<string, boolean>) {
  if (node.kind !== 'dataset') return BUILD_STATUS_PALETTE.notDataset;
  return jobSpecMap[node.id] ? BUILD_STATUS_PALETTE.hasMaster : BUILD_STATUS_PALETTE.noMaster;
}

function colorForNode(node: LineageNode, mode: ColoringMode, jobSpecMap: Record<string, boolean>) {
  if (mode === 'marking') return MARKING_PALETTE[node.marking] ?? FALLBACK_COLOR;
  if (mode === 'build_status') return buildStatusColor(node, jobSpecMap);
  return resourceColor(node.kind);
}

// Cytoscape stylesheet — round-rectangle pill with embedded chevrons in the
// label, mimicking Foundry's "‹  name  ›" node visual.
const STYLESHEET: StylesheetStyle[] = [
  {
    selector: 'node',
    style: {
      'background-color': 'data(color)',
      shape: 'round-rectangle',
      label: 'data(displayLabel)',
      color: '#1f252d',
      'text-valign': 'center',
      'text-halign': 'center',
      'text-wrap': 'wrap',
      'text-max-width': '220',
      'font-size': 11,
      'font-weight': 600,
      'font-family':
        'Arial, "Helvetica Neue", Helvetica, "Segoe UI", system-ui, -apple-system, sans-serif',
      width: 'label',
      height: 26,
      'padding-left': '14',
      'padding-right': '14',
      'padding-top': '4',
      'padding-bottom': '4',
      'border-width': 1,
      'border-color': 'data(borderColor)',
    },
  },
  {
    selector: 'edge',
    style: {
      width: 1.5,
      'line-color': '#9aa3ad',
      'target-arrow-color': '#9aa3ad',
      'target-arrow-shape': 'triangle',
      'curve-style': 'bezier',
      'arrow-scale': 0.9,
    },
  },
  {
    selector: 'node:selected',
    style: {
      'border-width': 3,
      'border-color': '#f08c3a',
      'overlay-opacity': 0,
    },
  },
  {
    selector: 'node.dim',
    style: {
      opacity: 0.25,
    },
  },
  {
    selector: 'node.match',
    style: {
      'border-width': 2,
      'border-color': '#2d72d2',
    },
  },
  {
    selector: 'edge.highlight',
    style: {
      'line-color': '#f08c3a',
      'target-arrow-color': '#f08c3a',
      width: 2.5,
    },
  },
];

const LAYOUT_BREADTHFIRST = {
  name: 'breadthfirst',
  directed: true,
  spacingFactor: 1.4,
  padding: 24,
} as const;
const LAYOUT_FCOSE = { name: 'fcose', animate: false, padding: 30 } as const;

// =============================================================================
// Inline SVG icon set for the lineage ribbon. Strokes use currentColor so each
// button can hover-tint without recreating the icon.
// =============================================================================

interface IconProps {
  size?: number;
}
function Icon({ size = 18, children }: IconProps & { children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {children}
    </svg>
  );
}
const IconTools = () => (
  <Icon>
    <path d="M5 5l4 4M5 19l4-4M19 5l-4 4M19 19l-4-4" />
    <circle cx="12" cy="12" r="3" />
  </Icon>
);
const IconLayout = () => (
  <Icon>
    <rect x="3.5" y="4" width="7" height="6" rx="1.2" />
    <rect x="13.5" y="4" width="7" height="9" rx="1.2" />
    <rect x="3.5" y="13" width="7" height="7" rx="1.2" />
    <rect x="13.5" y="16" width="7" height="4" rx="1.2" />
  </Icon>
);
const IconUndo = () => (
  <Icon>
    <path d="M9 14L4 9l5-5" />
    <path d="M4 9h9a6 6 0 0 1 0 12h-3" />
  </Icon>
);
const IconRedo = () => (
  <Icon>
    <path d="M15 14l5-5-5-5" />
    <path d="M20 9h-9a6 6 0 0 0 0 12h3" />
  </Icon>
);
const IconClean = () => (
  <Icon>
    <path d="M3 5h18" />
    <path d="M6 5l3 14h6l3-14" />
    <path d="M9 9l6 6M15 9l-6 6" />
  </Icon>
);
const IconSelect = () => (
  <Icon>
    <circle cx="12" cy="12" r="6" />
    <circle cx="12" cy="12" r="2" />
  </Icon>
);
const IconExpand = () => (
  <Icon>
    <circle cx="6" cy="12" r="2" />
    <circle cx="18" cy="6" r="2" />
    <circle cx="18" cy="18" r="2" />
    <path d="M8 11l8-4M8 13l8 4" />
  </Icon>
);
const IconColor = () => (
  <Icon>
    <path d="M12 4c-3 4-5 7-5 10a5 5 0 0 0 10 0c0-3-2-6-5-10z" />
  </Icon>
);
const IconFind = () => (
  <Icon>
    <circle cx="11" cy="11" r="6" />
    <path d="M16 16l4 4" />
    <path d="M9 11h4M11 9v4" />
  </Icon>
);
const IconRemove = () => (
  <Icon>
    <circle cx="12" cy="12" r="8" />
    <path d="M8 12h8" />
  </Icon>
);
const IconAlign = () => (
  <Icon>
    <rect x="3.5" y="5" width="6" height="6" rx="1" />
    <rect x="14.5" y="5" width="6" height="6" rx="1" />
    <rect x="3.5" y="13" width="6" height="6" rx="1" />
    <rect x="14.5" y="13" width="6" height="6" rx="1" />
  </Icon>
);
const IconLayoutColor = () => (
  <Icon>
    <circle cx="6.5" cy="6.5" r="1.6" />
    <circle cx="12" cy="6.5" r="1.6" />
    <circle cx="17.5" cy="6.5" r="1.6" />
    <circle cx="6.5" cy="12" r="1.6" />
    <circle cx="12" cy="12" r="1.6" />
    <circle cx="17.5" cy="12" r="1.6" />
    <circle cx="6.5" cy="17.5" r="1.6" />
    <circle cx="12" cy="17.5" r="1.6" />
    <circle cx="17.5" cy="17.5" r="1.6" />
  </Icon>
);
const IconGroupColor = () => (
  <Icon>
    <ellipse cx="9" cy="12" rx="5" ry="3.5" />
    <ellipse cx="15" cy="12" rx="5" ry="3.5" />
  </Icon>
);
const IconLegendEye = () => (
  <Icon>
    <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z" />
    <circle cx="12" cy="12" r="3" />
    <path d="M4 4l16 16" />
  </Icon>
);
const IconLegendEyeOpen = () => (
  <Icon>
    <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z" />
    <circle cx="12" cy="12" r="3" />
  </Icon>
);
const IconResourceTable = () => (
  <Icon>
    <rect x="3.5" y="5" width="17" height="14" rx="1.2" />
    <path d="M3.5 10h17M9 5v14" />
  </Icon>
);
const IconBranch = () => (
  <Icon>
    <circle cx="6" cy="6" r="2.2" />
    <circle cx="6" cy="18" r="2.2" />
    <circle cx="18" cy="9" r="2.2" />
    <path d="M6 8.2v7.6M6 12c0-3 2-3 4-3h6" />
  </Icon>
);
const IconCmd = () => (
  <Icon>
    <rect x="4" y="4" width="6" height="6" rx="1.6" />
    <rect x="14" y="4" width="6" height="6" rx="1.6" />
    <rect x="4" y="14" width="6" height="6" rx="1.6" />
    <rect x="14" y="14" width="6" height="6" rx="1.6" />
  </Icon>
);
const IconSettings = () => (
  <Icon>
    <circle cx="12" cy="12" r="3" />
    <path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M5.6 18.4l1.4-1.4M17 7l1.4-1.4" />
  </Icon>
);
const IconChevronDown = () => (
  <Icon size={14}>
    <path d="M6 9l6 6 6-6" />
  </Icon>
);
const IconZoomCenter = () => (
  <Icon>
    <circle cx="12" cy="12" r="3" />
    <path d="M12 4v3M12 17v3M4 12h3M17 12h3" />
  </Icon>
);
const IconZoomIn = () => (
  <Icon>
    <circle cx="11" cy="11" r="6" />
    <path d="M11 8v6M8 11h6M16 16l4 4" />
  </Icon>
);
const IconZoomOut = () => (
  <Icon>
    <circle cx="11" cy="11" r="6" />
    <path d="M8 11h6M16 16l4 4" />
  </Icon>
);
const IconFit = () => (
  <Icon>
    <path d="M4 9V5h4M16 5h4v4M4 15v4h4M16 19h4v-4" />
  </Icon>
);
const IconSearch = () => (
  <Icon>
    <circle cx="11" cy="11" r="6" />
    <path d="M16 16l4 4" />
  </Icon>
);
const IconCalendar = () => (
  <Icon>
    <rect x="3.5" y="5" width="17" height="15" rx="1.2" />
    <path d="M3.5 9h17M8 3v4M16 3v4" />
  </Icon>
);
const IconClipboard = () => (
  <Icon>
    <rect x="6" y="4" width="12" height="17" rx="1.4" />
    <path d="M9 4h6v3H9z" />
  </Icon>
);
const IconWrench = () => (
  <Icon>
    <path d="M14.5 4a4.5 4.5 0 0 1 5 5.5L7 22 2 17 14.5 4z" />
    <path d="M14 9l1 1" />
  </Icon>
);
const IconChevronsLeft = () => (
  <Icon size={16}>
    <path d="M11 7l-5 5 5 5M18 7l-5 5 5 5" />
  </Icon>
);
const IconChevronsDown = () => (
  <Icon size={16}>
    <path d="M7 7l5 5 5-5M7 13l5 5 5-5" />
  </Icon>
);
const IconFullscreen = () => (
  <Icon size={14}>
    <path d="M4 9V5h4M16 5h4v4M4 15v4h4M16 19h4v-4" />
  </Icon>
);
const IconRefresh = () => (
  <Icon>
    <path d="M20 11A8 8 0 1 0 12 20" />
    <path d="M20 5v6h-6" />
  </Icon>
);

// =============================================================================
// Page
// =============================================================================

export function LineagePage() {
  const navigate = useNavigate();
  const [graph, setGraph] = useState<LineageGraph | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selectedNode, setSelectedNode] = useState<LineageNode | null>(null);
  const [impact, setImpact] = useState<LineageImpactAnalysis | null>(null);
  const [impactLoading, setImpactLoading] = useState(false);
  const [building, setBuilding] = useState(false);
  const [buildResult, setBuildResult] = useState<LineageBuildResult | null>(null);
  const [acknowledgeSensitiveLineage, setAcknowledgeSensitiveLineage] = useState(false);
  const [jobSpecByDatasetId, setJobSpecByDatasetId] = useState<Record<string, boolean>>({});

  // Lineage-app specific UI state.
  const [coloringMode, setColoringMode] = useState<ColoringMode>('resource_type');
  const [legendOpen, setLegendOpen] = useState(true);
  const [coloringMenuOpen, setColoringMenuOpen] = useState(false);
  const [expandPopoverOpen, setExpandPopoverOpen] = useState(false);
  const [expandParents, setExpandParents] = useState(5);
  const [expandChildren, setExpandChildren] = useState(0);
  const [findOpen, setFindOpen] = useState(false);
  const [findQuery, setFindQuery] = useState('');
  const [branch, setBranch] = useState('master');
  const [branchMenuOpen, setBranchMenuOpen] = useState(false);
  const [saveMenuOpen, setSaveMenuOpen] = useState(false);
  const [layoutByColor, setLayoutByColor] = useState(false);
  const [groupByColor, setGroupByColor] = useState(false);
  const [bottomTab, setBottomTab] = useState<'preview' | 'history' | 'code' | 'data_health' | 'build_timeline'>('preview');
  const [bottomCollapsed, setBottomCollapsed] = useState(false);
  const [bottomFullscreen, setBottomFullscreen] = useState(false);
  const [activeRightTool, setActiveRightTool] = useState<'search' | 'list' | 'tools' | 'calendar' | 'clipboard' | null>(
    null,
  );
  const [datasetPreview, setDatasetPreview] = useState<DatasetPreviewResponse | null>(null);
  const [datasetPreviewLoading, setDatasetPreviewLoading] = useState(false);

  const cyRef = useRef<Core | null>(null);
  const graphRef = useRef<LineageGraph | null>(null);
  useEffect(() => {
    graphRef.current = graph;
  }, [graph]);

  // ---------------------------------------------------------------------------
  // Data loading
  // ---------------------------------------------------------------------------

  const loadGraph = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const fresh = await getFullLineage();
      const datasetIds = fresh.nodes.filter((n) => n.kind === 'dataset').map((n) => n.id);
      const jobSpecResults = await Promise.allSettled(
        datasetIds.map(async (id) => [id, await loadJobSpecStatus(id)] as const),
      );
      const nextJobSpec: Record<string, boolean> = {};
      for (const r of jobSpecResults) {
        if (r.status === 'fulfilled') {
          const [id, status] = r.value;
          nextJobSpec[id] = status.has_master_jobspec;
        }
      }
      setJobSpecByDatasetId(nextJobSpec);
      setGraph(fresh);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load lineage');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadGraph();
  }, [loadGraph]);

  // ---------------------------------------------------------------------------
  // Cytoscape elements & interaction
  // ---------------------------------------------------------------------------

  const elements = useMemo<ElementDefinition[]>(() => {
    if (!graph) return [];
    return [
      ...graph.nodes.map((node) => ({
        data: {
          id: node.id,
          // Foundry visual: chevron-tagged pill. The chevrons are baked into
          // the label so we keep a single round-rectangle node primitive.
          displayLabel: `‹  ${node.label}  ›`,
          kind: node.kind,
          marking: node.marking,
          color: colorForNode(node, coloringMode, jobSpecByDatasetId),
          borderColor: nodeBorderColor(node.marking),
        },
      })),
      ...graph.edges.map((edge) => ({
        data: {
          id: edge.id,
          source: edge.source,
          target: edge.target,
          relation: edge.relation_kind,
        },
      })),
    ];
  }, [graph, coloringMode, jobSpecByDatasetId]);

  const layout = useMemo(() => {
    return layoutByColor
      ? (LAYOUT_FCOSE as unknown as Parameters<typeof CytoscapeCanvas>[0]['layout'])
      : (LAYOUT_BREADTHFIRST as unknown as Parameters<typeof CytoscapeCanvas>[0]['layout']);
  }, [layoutByColor]);

  const handleCytoscapeReady = useCallback((cy: Core) => {
    cyRef.current = cy;
    cy.removeListener('tap');
    cy.on('tap', 'node', (event: EventObject) => {
      const nodeId = String(event.target.id());
      const node = graphRef.current?.nodes.find((entry) => entry.id === nodeId) ?? null;
      setSelectedNode(node);
      setImpact(null);
      setBuildResult(null);
      setDatasetPreview(null);
      if (node?.kind === 'dataset') {
        void loadImpact(node.id);
        void loadPreview(node.id);
      }
    });
    cy.on('tap', (event) => {
      if (event.target === cy) {
        setSelectedNode(null);
        setImpact(null);
        setBuildResult(null);
        setDatasetPreview(null);
      }
    });
  }, []);

  // Apply find-query highlight (re-runs when the term or graph changes).
  useEffect(() => {
    const cy = cyRef.current;
    if (!cy) return;
    const q = findQuery.trim().toLowerCase();
    cy.nodes().removeClass('match dim');
    if (!q) return;
    const matches = cy.nodes().filter((n) => String(n.data('displayLabel') ?? '').toLowerCase().includes(q));
    if (matches.length === 0) return;
    cy.nodes().not(matches).addClass('dim');
    matches.addClass('match');
  }, [findQuery, elements]);

  async function loadImpact(datasetId: string) {
    setImpactLoading(true);
    setBuildResult(null);
    setAcknowledgeSensitiveLineage(false);
    try {
      const next = await getDatasetLineageImpact(datasetId);
      setImpact(next);
    } catch (cause) {
      setImpact(null);
      notifications.error(cause instanceof Error ? cause.message : 'Failed to load impact analysis');
    } finally {
      setImpactLoading(false);
    }
  }

  async function loadPreview(datasetId: string) {
    setDatasetPreviewLoading(true);
    try {
      const next = await previewDataset(datasetId, { limit: 25 });
      setDatasetPreview(next);
    } catch {
      setDatasetPreview(null);
    } finally {
      setDatasetPreviewLoading(false);
    }
  }

  async function triggerBuilds() {
    if (!selectedNode || selectedNode.kind !== 'dataset') return;
    setBuilding(true);
    try {
      const next = await triggerLineageBuilds(selectedNode.id, {
        include_workflows: true,
        dry_run: false,
        acknowledge_sensitive_lineage: acknowledgeSensitiveLineage,
        context: { initiated_from: 'lineage-explorer' },
      });
      setBuildResult(next);
      notifications.success(`Triggered ${next.triggered.length} downstream build(s)`);
      await loadImpact(selectedNode.id);
      setBottomTab('build_timeline');
      setBottomCollapsed(false);
    } catch (cause) {
      notifications.error(cause instanceof Error ? cause.message : 'Failed to trigger builds');
    } finally {
      setBuilding(false);
    }
  }

  // ---------------------------------------------------------------------------
  // Zoom / fit controls
  // ---------------------------------------------------------------------------

  function zoomBy(factor: number) {
    const cy = cyRef.current;
    if (!cy) return;
    cy.zoom({
      level: cy.zoom() * factor,
      renderedPosition: { x: cy.width() / 2, y: cy.height() / 2 },
    });
  }
  function fitGraph() {
    cyRef.current?.fit(undefined, 40);
  }
  function recenterGraph() {
    cyRef.current?.center();
  }

  // ---------------------------------------------------------------------------
  // Derived counts for ribbon + legend
  // ---------------------------------------------------------------------------

  const sensitiveCandidateCount =
    impact?.build_candidates.filter((c) => c.requires_acknowledgement).length ?? 0;

  const legendEntries = useMemo(() => {
    if (!graph) return [];
    if (coloringMode === 'marking') {
      const seen = new Set<string>();
      const entries: { label: string; color: string; count: number }[] = [];
      for (const n of graph.nodes) {
        if (seen.has(n.marking)) continue;
        seen.add(n.marking);
        entries.push({
          label: n.marking.charAt(0).toUpperCase() + n.marking.slice(1),
          color: MARKING_PALETTE[n.marking] ?? FALLBACK_COLOR,
          count: graph.nodes.filter((m) => m.marking === n.marking).length,
        });
      }
      return entries;
    }
    if (coloringMode === 'build_status') {
      const datasets = graph.nodes.filter((n) => n.kind === 'dataset');
      const has = datasets.filter((n) => jobSpecByDatasetId[n.id]).length;
      const lacks = datasets.length - has;
      return [
        { label: 'JobSpec on master', color: BUILD_STATUS_PALETTE.hasMaster, count: has },
        { label: 'No master JobSpec', color: BUILD_STATUS_PALETTE.noMaster, count: lacks },
      ];
    }
    const seen = new Set<string>();
    const entries: { label: string; color: string; count: number }[] = [];
    for (const n of graph.nodes) {
      if (seen.has(n.kind)) continue;
      seen.add(n.kind);
      entries.push({
        label: n.kind.charAt(0).toUpperCase() + n.kind.slice(1).replace(/_/g, ' '),
        color: resourceColor(n.kind),
        count: graph.nodes.filter((m) => m.kind === n.kind).length,
      });
    }
    return entries;
  }, [graph, coloringMode, jobSpecByDatasetId]);

  const inBetweenSelectionDisabled = selectedNode === null;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <section className="lineage-app" style={pageRoot}>
      <LineageHeader
        branch={branch}
        branchMenuOpen={branchMenuOpen}
        onBranchClick={() => setBranchMenuOpen((v) => !v)}
        onBranchSelect={(b) => {
          setBranch(b);
          setBranchMenuOpen(false);
        }}
        saveMenuOpen={saveMenuOpen}
        onSaveClick={() => notifications.success('Graph saved (local snapshot)')}
        onSaveCaretClick={() => setSaveMenuOpen((v) => !v)}
        onSaveAs={() => {
          setSaveMenuOpen(false);
          notifications.success('Save as… coming soon');
        }}
        onOpenGraph={() => {
          setSaveMenuOpen(false);
          notifications.success('Open graph… coming soon');
        }}
      />

      <Ribbon
        selectedNode={selectedNode}
        onClean={() => {
          setSelectedNode(null);
          setImpact(null);
          setBuildResult(null);
          setFindQuery('');
          setFindOpen(false);
        }}
        onSelectFocus={() => {
          if (selectedNode && cyRef.current) {
            cyRef.current.center(cyRef.current.$id(selectedNode.id));
          }
        }}
        expandPopoverOpen={expandPopoverOpen}
        onExpandClick={() => setExpandPopoverOpen((v) => !v)}
        expandParents={expandParents}
        expandChildren={expandChildren}
        onExpandParentsChange={setExpandParents}
        onExpandChildrenChange={setExpandChildren}
        onExpandApply={() => {
          setExpandPopoverOpen(false);
          notifications.success(`Expanded ${expandParents + expandChildren} nodes`);
        }}
        inBetweenDisabled={inBetweenSelectionDisabled}
        onColorClick={() => setColoringMenuOpen((v) => !v)}
        coloringMenuOpen={coloringMenuOpen}
        coloringMode={coloringMode}
        onColoringChange={(mode) => {
          setColoringMode(mode);
          setColoringMenuOpen(false);
        }}
        findOpen={findOpen}
        onFindClick={() => setFindOpen((v) => !v)}
        findQuery={findQuery}
        onFindQueryChange={setFindQuery}
        onRemoveClick={() => {
          if (!selectedNode || !cyRef.current) return;
          cyRef.current.$id(selectedNode.id).remove();
          setSelectedNode(null);
          notifications.success('Removed from graph');
        }}
        onAlignClick={() => fitGraph()}
        layoutByColor={layoutByColor}
        onLayoutByColorClick={() => setLayoutByColor((v) => !v)}
        groupByColor={groupByColor}
        onGroupByColorClick={() => setGroupByColor((v) => !v)}
        legendOpen={legendOpen}
        onLegendToggle={() => setLegendOpen((v) => !v)}
        onRefresh={() => void loadGraph()}
      />

      <div style={canvasShell}>
        <div style={canvasArea}>
          {error && (
            <div className="of-status-danger" style={errorBanner}>
              {error}
            </div>
          )}

          {loading ? (
            <div style={emptyState}>Loading lineage graph…</div>
          ) : !graph || graph.nodes.length === 0 ? (
            <div style={emptyState}>No lineage data yet. Run a pipeline or workflow to populate the graph.</div>
          ) : (
            <CytoscapeCanvas
              elements={elements}
              stylesheet={STYLESHEET}
              layout={layout}
              height="100%"
              onReady={handleCytoscapeReady}
              className="lineage-canvas"
            />
          )}

          {legendOpen && legendEntries.length > 0 && (
            <div style={legendCard} className="of-panel">
              {legendEntries.map((entry) => (
                <div key={entry.label} style={legendItem}>
                  <span style={{ ...legendSwatch, background: entry.color }} />
                  <span style={{ fontSize: 12, color: '#1f252d', fontWeight: 500 }}>{entry.label}</span>
                  <span style={{ fontSize: 11, color: '#5f6b7a' }}>({entry.count})</span>
                </div>
              ))}
            </div>
          )}

          {/* Floating zoom / fit controls */}
          <div style={zoomStack}>
            <ZoomButton title="Recenter" onClick={recenterGraph}>
              <IconZoomCenter />
            </ZoomButton>
            <ZoomButton title="Zoom in" onClick={() => zoomBy(1.25)}>
              <IconZoomIn />
            </ZoomButton>
            <ZoomButton title="Zoom out" onClick={() => zoomBy(0.8)}>
              <IconZoomOut />
            </ZoomButton>
            <ZoomButton title="Fit to screen" onClick={fitGraph}>
              <IconFit />
            </ZoomButton>
          </div>

          {/* Right rail */}
          <aside style={rightRail}>
            <RightRailButton
              active={activeRightTool === 'search'}
              title="Search"
              onClick={() => setActiveRightTool((v) => (v === 'search' ? null : 'search'))}
            >
              <IconSearch />
            </RightRailButton>
            <RightRailButton
              active={activeRightTool === 'list'}
              title="Resource list"
              onClick={() => setActiveRightTool((v) => (v === 'list' ? null : 'list'))}
            >
              <IconResourceTable />
            </RightRailButton>
            <RightRailButton
              active={activeRightTool === 'tools'}
              title="Tools"
              onClick={() => setActiveRightTool((v) => (v === 'tools' ? null : 'tools'))}
            >
              <IconWrench />
            </RightRailButton>
            <RightRailButton
              active={activeRightTool === 'calendar'}
              title="Schedule"
              onClick={() => setActiveRightTool((v) => (v === 'calendar' ? null : 'calendar'))}
            >
              <IconCalendar />
            </RightRailButton>
            <RightRailButton
              active={activeRightTool === 'clipboard'}
              title="Clipboard"
              onClick={() => setActiveRightTool((v) => (v === 'clipboard' ? null : 'clipboard'))}
            >
              <IconClipboard />
            </RightRailButton>
            <button type="button" style={rightRailCollapse} title="Collapse">
              <IconChevronsLeft />
            </button>
          </aside>

          {activeRightTool && (
            <div style={rightRailDrawer} className="of-panel">
              <RightRailDrawerContent
                tool={activeRightTool}
                graph={graph}
                selectedNode={selectedNode}
                onClose={() => setActiveRightTool(null)}
                onCreateForDataset={(datasetRid) => navigate(`/schedules/new?event_target=${encodeURIComponent(datasetRid)}`)}
                onPick={(id) => {
                  const node = graph?.nodes.find((n) => n.id === id) ?? null;
                  setSelectedNode(node);
                  if (node?.kind === 'dataset') {
                    void loadImpact(node.id);
                    void loadPreview(node.id);
                  }
                  if (cyRef.current) cyRef.current.center(cyRef.current.$id(id));
                }}
              />
            </div>
          )}
        </div>

        {/* Bottom panel — Preview / History / Code / Data health / Build timeline */}
        <BottomPanel
          collapsed={bottomCollapsed}
          fullscreen={bottomFullscreen}
          onCollapse={() => setBottomCollapsed((v) => !v)}
          onFullscreen={() => setBottomFullscreen((v) => !v)}
          tab={bottomTab}
          onTabChange={setBottomTab}
          selectedNode={selectedNode}
          impact={impact}
          impactLoading={impactLoading}
          building={building}
          buildResult={buildResult}
          datasetPreview={datasetPreview}
          datasetPreviewLoading={datasetPreviewLoading}
          acknowledgeSensitiveLineage={acknowledgeSensitiveLineage}
          onAckChange={setAcknowledgeSensitiveLineage}
          sensitiveCandidateCount={sensitiveCandidateCount}
          onTriggerBuilds={() => void triggerBuilds()}
          onReloadImpact={() => selectedNode?.id && void loadImpact(selectedNode.id)}
        />
      </div>
    </section>
  );
}

// =============================================================================
// Header
// =============================================================================

interface LineageHeaderProps {
  branch: string;
  branchMenuOpen: boolean;
  onBranchClick: () => void;
  onBranchSelect: (b: string) => void;
  saveMenuOpen: boolean;
  onSaveClick: () => void;
  onSaveCaretClick: () => void;
  onSaveAs: () => void;
  onOpenGraph: () => void;
}
function LineageHeader({
  branch,
  branchMenuOpen,
  onBranchClick,
  onBranchSelect,
  saveMenuOpen,
  onSaveClick,
  onSaveCaretClick,
  onSaveAs,
  onOpenGraph,
}: LineageHeaderProps) {
  return (
    <header style={headerRow}>
      <div style={headerLeft}>
        <span style={lineageMark}>
          <svg width={22} height={22} viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <rect x="3" y="3" width="6" height="6" rx="1.4" fill="#f08c3a" />
            <rect x="14" y="14" width="7" height="7" rx="1.4" fill="#3b86c4" />
            <path d="M9 6h5a4 4 0 0 1 4 4v4" stroke="#1f252d" strokeWidth="1.5" />
          </svg>
        </span>
        <span style={headerTitle}>Data Lineage</span>
      </div>

      <div style={headerCenter}>
        <div style={branchPicker}>
          <button type="button" style={branchTrigger} onClick={onBranchClick}>
            <IconBranch />
            <span style={{ flex: 1, textAlign: 'left' }}>{branch}</span>
            <IconChevronDown />
          </button>
          {branchMenuOpen && (
            <div style={branchMenu} className="of-panel">
              {['master', 'main', 'develop', 'staging'].map((b) => (
                <button
                  key={b}
                  type="button"
                  style={menuItem(b === branch)}
                  onClick={() => onBranchSelect(b)}
                >
                  <IconBranch />
                  <span>{b}</span>
                </button>
              ))}
            </div>
          )}
        </div>
        <button type="button" style={iconBtn} title="Branch settings">
          <IconBranch />
        </button>
      </div>

      <div style={headerRight}>
        <button type="button" style={iconBtn} title="Command palette">
          <IconCmd />
        </button>
        <button type="button" style={iconBtn} title="Settings">
          <IconSettings />
        </button>
        <div style={{ position: 'relative', display: 'inline-flex' }}>
          <button type="button" style={savePrimary} onClick={onSaveClick}>
            Save
          </button>
          <button type="button" style={saveCaret} onClick={onSaveCaretClick} title="Save options">
            <IconChevronDown />
          </button>
          {saveMenuOpen && (
            <div style={saveMenu} className="of-panel">
              <button type="button" style={menuItem(false)} onClick={onSaveAs}>
                Save as…
              </button>
              <button type="button" style={menuItem(false)} onClick={onOpenGraph}>
                Open graph…
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}

// =============================================================================
// Ribbon
// =============================================================================

interface RibbonProps {
  selectedNode: LineageNode | null;
  onClean: () => void;
  onSelectFocus: () => void;
  expandPopoverOpen: boolean;
  onExpandClick: () => void;
  expandParents: number;
  expandChildren: number;
  onExpandParentsChange: (n: number) => void;
  onExpandChildrenChange: (n: number) => void;
  onExpandApply: () => void;
  inBetweenDisabled: boolean;
  onColorClick: () => void;
  coloringMenuOpen: boolean;
  coloringMode: ColoringMode;
  onColoringChange: (m: ColoringMode) => void;
  findOpen: boolean;
  onFindClick: () => void;
  findQuery: string;
  onFindQueryChange: (s: string) => void;
  onRemoveClick: () => void;
  onAlignClick: () => void;
  layoutByColor: boolean;
  onLayoutByColorClick: () => void;
  groupByColor: boolean;
  onGroupByColorClick: () => void;
  legendOpen: boolean;
  onLegendToggle: () => void;
  onRefresh: () => void;
}
function Ribbon(props: RibbonProps) {
  const {
    onClean,
    onSelectFocus,
    expandPopoverOpen,
    onExpandClick,
    expandParents,
    expandChildren,
    onExpandParentsChange,
    onExpandChildrenChange,
    onExpandApply,
    inBetweenDisabled,
    onColorClick,
    coloringMenuOpen,
    coloringMode,
    onColoringChange,
    findOpen,
    onFindClick,
    findQuery,
    onFindQueryChange,
    onRemoveClick,
    onAlignClick,
    layoutByColor,
    onLayoutByColorClick,
    groupByColor,
    onGroupByColorClick,
    legendOpen,
    onLegendToggle,
    onRefresh,
  } = props;

  return (
    <div style={ribbonRow}>
      <ToolButton label="Tools" onClick={() => undefined}>
        <IconTools />
      </ToolButton>
      <ToolButton label="Layout" onClick={() => undefined}>
        <IconLayout />
      </ToolButton>
      <div style={ribbonGroup}>
        <ToolButton label="Undo" onClick={() => undefined}>
          <IconUndo />
        </ToolButton>
        <ToolButton label="Redo" onClick={() => undefined}>
          <IconRedo />
        </ToolButton>
      </div>
      <ToolButton label="Clean" onClick={onClean}>
        <IconClean />
      </ToolButton>
      <ToolButton label="Select" onClick={onSelectFocus}>
        <IconSelect />
      </ToolButton>
      <div style={{ position: 'relative' }}>
        <ToolButton label="Expand" onClick={onExpandClick} active={expandPopoverOpen}>
          <IconExpand />
        </ToolButton>
        {expandPopoverOpen && (
          <div style={expandPopover} className="of-panel">
            <button type="button" style={expandRow} disabled={inBetweenDisabled}>
              <IconExpand /> Add in-between
              <span style={hotkey}>⌘B</span>
            </button>
            <button type="button" style={expandRow} disabled={inBetweenDisabled}>
              <IconExpand /> Add common ancestors
              <span style={hotkey}>⌘J</span>
            </button>
            <button type="button" style={expandRow} disabled={inBetweenDisabled}>
              <IconExpand /> Add common descendants
              <span style={hotkey}>⌘K</span>
            </button>
            <div style={{ height: 1, background: 'var(--border-subtle)', margin: '8px 0' }} />
            <div style={expandStepperRow}>
              <Stepper
                value={expandParents}
                onChange={onExpandParentsChange}
                hint="Expand parents"
              />
              <Stepper
                value={expandChildren}
                onChange={onExpandChildrenChange}
                hint="Expand children"
              />
            </div>
            <button
              type="button"
              className="of-btn of-btn-primary"
              style={{ marginTop: 12, width: '100%' }}
              onClick={onExpandApply}
            >
              Add {expandParents + expandChildren} nodes
            </button>
          </div>
        )}
      </div>
      <div style={{ position: 'relative' }}>
        <ToolButton label="Color" onClick={onColorClick} active={coloringMenuOpen}>
          <IconColor />
        </ToolButton>
        {coloringMenuOpen && (
          <div style={coloringPopover} className="of-panel">
            {COLORING_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                style={menuItem(opt.value === coloringMode)}
                onClick={() => onColoringChange(opt.value)}
              >
                {opt.label}
              </button>
            ))}
          </div>
        )}
      </div>
      <div style={{ position: 'relative' }}>
        <ToolButton label="Find" onClick={onFindClick} active={findOpen}>
          <IconFind />
        </ToolButton>
        {findOpen && (
          <div style={findPopover} className="of-panel">
            <input
              type="text"
              autoFocus
              placeholder="Find by label…"
              value={findQuery}
              onChange={(e) => onFindQueryChange(e.target.value)}
              style={findInput}
            />
          </div>
        )}
      </div>
      <ToolButton label="Remove" onClick={onRemoveClick}>
        <IconRemove />
      </ToolButton>
      <ToolButton label="Align" onClick={onAlignClick}>
        <IconAlign />
      </ToolButton>

      <div style={{ flex: 1 }} />

      <ToolButton label={`Layout\nby color`} active={layoutByColor} onClick={onLayoutByColorClick}>
        <IconLayoutColor />
      </ToolButton>
      <ToolButton label={`Group\nby color`} active={groupByColor} onClick={onGroupByColorClick}>
        <IconGroupColor />
      </ToolButton>
      <ToolButton label="Legend" active={legendOpen} onClick={onLegendToggle}>
        {legendOpen ? <IconLegendEyeOpen /> : <IconLegendEye />}
      </ToolButton>
      <div style={coloringSelect}>
        <IconResourceTable />
        <span style={{ flex: 1, padding: '0 8px' }}>
          {COLORING_OPTIONS.find((o) => o.value === coloringMode)?.label}
        </span>
        <button type="button" style={iconBtnGhost} onClick={onColorClick}>
          <IconChevronDown />
        </button>
      </div>
      <ToolButton label="Refresh" onClick={onRefresh}>
        <IconRefresh />
      </ToolButton>
    </div>
  );
}

interface ToolButtonProps {
  label: string;
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  children: ReactNode;
}
function ToolButton({ label, onClick, active = false, disabled = false, children }: ToolButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      style={{
        ...toolBtn,
        ...(active ? toolBtnActive : {}),
        ...(disabled ? toolBtnDisabled : {}),
      }}
    >
      <span style={toolBtnIcon}>{children}</span>
      <span style={toolBtnLabel}>{label}</span>
    </button>
  );
}

// =============================================================================
// Stepper (Expand popover)
// =============================================================================

interface StepperProps {
  value: number;
  onChange: (n: number) => void;
  hint: string;
}
function Stepper({ value, onChange, hint }: StepperProps) {
  return (
    <div style={stepperWrap}>
      <div style={stepperControls}>
        <button type="button" style={stepperBtn} onClick={() => onChange(Math.max(0, value - 5))}>
          «
        </button>
        <button type="button" style={stepperBtn} onClick={() => onChange(Math.max(0, value - 1))}>
          ‹
        </button>
        <span style={stepperValue}>{value}</span>
        <button type="button" style={stepperBtn} onClick={() => onChange(value + 1)}>
          ›
        </button>
        <button type="button" style={stepperBtn} onClick={() => onChange(value + 5)}>
          »
        </button>
      </div>
      <span style={stepperHint}>{hint}</span>
    </div>
  );
}

// =============================================================================
// Right rail
// =============================================================================

interface RightRailButtonProps {
  active: boolean;
  title: string;
  onClick: () => void;
  children: ReactNode;
}
function RightRailButton({ active, title, onClick, children }: RightRailButtonProps) {
  return (
    <button
      type="button"
      title={title}
      onClick={onClick}
      style={{
        ...rightRailButton,
        ...(active ? rightRailButtonActive : {}),
      }}
    >
      {children}
    </button>
  );
}

interface RightRailDrawerContentProps {
  tool: 'search' | 'list' | 'tools' | 'calendar' | 'clipboard';
  graph: LineageGraph | null;
  selectedNode: LineageNode | null;
  onClose: () => void;
  onCreateForDataset: (datasetRid: string) => void;
  onPick: (id: string) => void;
}
function RightRailDrawerContent({ tool, graph, selectedNode, onClose, onCreateForDataset, onPick }: RightRailDrawerContentProps) {
  const [q, setQ] = useState('');
  const filtered = useMemo(() => {
    const list = graph?.nodes ?? [];
    const query = q.trim().toLowerCase();
    if (!query) return list.slice(0, 80);
    return list.filter((n) => n.label.toLowerCase().includes(query) || n.id.toLowerCase().includes(query)).slice(0, 80);
  }, [graph, q]);

  return (
    <div style={{ padding: '12px 14px', display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <strong style={{ fontSize: 13, color: 'var(--text-strong)' }}>
          {tool === 'search' && 'Search'}
          {tool === 'list' && 'Resource list'}
          {tool === 'tools' && 'Tools'}
          {tool === 'calendar' && 'Schedule'}
          {tool === 'clipboard' && 'Clipboard'}
        </strong>
        <button type="button" style={iconBtnGhost} onClick={onClose} title="Close">
          ×
        </button>
      </div>

      {(tool === 'search' || tool === 'list') && (
        <>
          <input
            type="text"
            placeholder="Search resources…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ ...findInput, marginBottom: 10 }}
          />
          <div style={{ flex: 1, overflow: 'auto', display: 'grid', gap: 4 }}>
            {filtered.length === 0 ? (
              <div className="of-text-muted" style={{ fontSize: 12 }}>
                No matches.
              </div>
            ) : (
              filtered.map((n) => (
                <button
                  key={n.id}
                  type="button"
                  onClick={() => onPick(n.id)}
                  style={resourceListRow}
                >
                  <span style={{ ...resourceDot, background: resourceColor(n.kind) }} />
                  <span style={{ flex: 1, textAlign: 'left' }}>{n.label}</span>
                  <span style={{ color: 'var(--text-soft)', fontSize: 11 }}>{n.kind}</span>
                </button>
              ))
            )}
          </div>
        </>
      )}

      {tool === 'tools' && (
        <p className="of-text-muted" style={{ fontSize: 12 }}>
          Advanced graph tools coming soon.
        </p>
      )}
      {tool === 'calendar' && (
        <ScheduleSidebar
          selectedDatasetRids={selectedNode?.kind === 'dataset' ? [metadataString(selectedNode.metadata ?? {}, 'rid') || selectedNode.id] : []}
          onCreateForDataset={onCreateForDataset}
        />
      )}
      {tool === 'clipboard' && (
        <p className="of-text-muted" style={{ fontSize: 12 }}>
          Clipboard pinned graphs coming soon.
        </p>
      )}
    </div>
  );
}

// =============================================================================
// Bottom panel — Preview / History / Code / Data health / Build timeline
// =============================================================================

const BOTTOM_TABS: { id: 'preview' | 'history' | 'code' | 'data_health' | 'build_timeline'; label: string; icon: ReactNode }[] = [
  { id: 'preview', label: 'Preview', icon: <IconResourceTable /> },
  { id: 'history', label: 'History', icon: <IconRefresh /> },
  { id: 'code', label: 'Code', icon: <IconCmd /> },
  { id: 'data_health', label: 'Data health', icon: <IconColor /> },
  { id: 'build_timeline', label: 'Build timeline', icon: <IconCalendar /> },
];

interface BottomPanelProps {
  collapsed: boolean;
  fullscreen: boolean;
  onCollapse: () => void;
  onFullscreen: () => void;
  tab: 'preview' | 'history' | 'code' | 'data_health' | 'build_timeline';
  onTabChange: (t: 'preview' | 'history' | 'code' | 'data_health' | 'build_timeline') => void;
  selectedNode: LineageNode | null;
  impact: LineageImpactAnalysis | null;
  impactLoading: boolean;
  building: boolean;
  buildResult: LineageBuildResult | null;
  datasetPreview: DatasetPreviewResponse | null;
  datasetPreviewLoading: boolean;
  acknowledgeSensitiveLineage: boolean;
  onAckChange: (v: boolean) => void;
  sensitiveCandidateCount: number;
  onTriggerBuilds: () => void;
  onReloadImpact: () => void;
}
function BottomPanel({
  collapsed,
  fullscreen,
  onCollapse,
  onFullscreen,
  tab,
  onTabChange,
  selectedNode,
  impact,
  impactLoading,
  building,
  buildResult,
  datasetPreview,
  datasetPreviewLoading,
  acknowledgeSensitiveLineage,
  onAckChange,
  sensitiveCandidateCount,
  onTriggerBuilds,
  onReloadImpact,
}: BottomPanelProps) {
  const height = collapsed ? 36 : fullscreen ? '60vh' : 240;

  return (
    <div style={{ ...bottomPanel, height, transition: 'height 120ms ease-out' }}>
      <div style={bottomPanelTabs}>
        {BOTTOM_TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            style={{ ...bottomTabBtn, ...(t.id === tab && !collapsed ? bottomTabBtnActive : {}) }}
            onClick={() => {
              onTabChange(t.id);
              if (collapsed) onCollapse();
            }}
          >
            <span style={{ display: 'inline-flex', width: 14, height: 14 }}>{t.icon}</span>
            {t.label}
          </button>
        ))}
        <div style={{ flex: 1 }} />
        <span style={selectedIndicator}>
          {selectedNode ? `${selectedNode.kind} selected` : 'No node selected'}
        </span>
        <button type="button" style={iconBtnGhost} onClick={onCollapse} title={collapsed ? 'Expand panel' : 'Collapse panel'}>
          <IconChevronsDown />
        </button>
        <button type="button" style={iconBtnGhost} onClick={onFullscreen} title="Fullscreen panel">
          <IconFullscreen />
        </button>
      </div>

      {!collapsed && (
        <div style={bottomPanelBody}>
          {tab === 'preview' && (
            <PreviewTab
              selectedNode={selectedNode}
              datasetPreview={datasetPreview}
              datasetPreviewLoading={datasetPreviewLoading}
            />
          )}
          {tab === 'history' && (
            <p className="of-text-muted" style={{ fontSize: 12, padding: 16 }}>
              Resource history will surface dataset/pipeline transactions for the selected node.
            </p>
          )}
          {tab === 'code' && <CodeTab selectedNode={selectedNode} />}
          {tab === 'data_health' && (
            <DataHealthTab
              selectedNode={selectedNode}
              impact={impact}
              impactLoading={impactLoading}
              building={building}
              acknowledgeSensitiveLineage={acknowledgeSensitiveLineage}
              onAckChange={onAckChange}
              sensitiveCandidateCount={sensitiveCandidateCount}
              onTriggerBuilds={onTriggerBuilds}
              onReloadImpact={onReloadImpact}
            />
          )}
          {tab === 'build_timeline' && <BuildTimelineTab buildResult={buildResult} />}
        </div>
      )}
    </div>
  );
}

interface PreviewTabProps {
  selectedNode: LineageNode | null;
  datasetPreview: DatasetPreviewResponse | null;
  datasetPreviewLoading: boolean;
}
function PreviewTab({ selectedNode, datasetPreview, datasetPreviewLoading }: PreviewTabProps) {
  if (!selectedNode) {
    return <div style={tabHint}>Select a node to inspect its preview.</div>;
  }
  if (selectedNode.kind !== 'dataset') {
    return (
      <div style={tabHint}>
        {selectedNode.label} is a {selectedNode.kind}. Preview is only available for datasets.
      </div>
    );
  }
  const metadata = selectedNode.metadata ?? {};
  const datasetPath = metadataString(metadata, 'path') || metadataString(metadata, 'folder_path');
  const displayName = metadataString(metadata, 'display_name') || selectedNode.label;
  const datasetHref = datasetHrefForNode(selectedNode);
  const previewHeader = (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, padding: '10px 12px', borderBottom: '1px solid var(--border-default)' }}>
      <div style={{ minWidth: 0 }}>
        <strong style={{ display: 'block', fontSize: 12, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{displayName}</strong>
        <span className="of-text-muted" style={{ display: 'block', marginTop: 2, fontFamily: 'var(--font-mono)', fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {datasetPath || selectedNode.id}
        </span>
      </div>
      <Link to={datasetHref} className="of-button" style={{ flex: '0 0 auto', fontSize: 11 }}>
        Open dataset
      </Link>
    </div>
  );
  if (datasetPreviewLoading) {
    return (
      <div style={{ display: 'grid', gridTemplateRows: 'max-content 1fr', height: '100%' }}>
        {previewHeader}
        <div style={tabHint}>Loading dataset preview…</div>
      </div>
    );
  }
  if (!datasetPreview || !datasetPreview.rows || datasetPreview.rows.length === 0) {
    return (
      <div style={{ display: 'grid', gridTemplateRows: 'max-content 1fr', height: '100%' }}>
        {previewHeader}
        <div style={tabHint}>No preview rows available for this dataset.</div>
      </div>
    );
  }
  const columns = datasetPreview.columns ?? [];
  const rows = datasetPreview.rows;

  return (
    <div style={{ display: 'grid', gridTemplateRows: 'max-content 1fr', height: '100%' }}>
      {previewHeader}
      <div style={{ overflow: 'auto', minHeight: 0 }}>
        <table style={previewTable}>
          <thead>
            <tr>
              {columns.map((col) => (
                <th key={col.name} style={previewTh}>
                  {col.name}
                  <div style={{ fontSize: 10, color: 'var(--text-soft)', fontWeight: 400 }}>
                    {col.field_type ?? col.data_type ?? ''}
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, idx) => (
              <tr key={idx}>
                {columns.map((col) => (
                  <td key={col.name} style={previewTd}>
                    {formatPreviewCell(row[col.name])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function formatPreviewCell(value: unknown): string {
  if (value === null || value === undefined) return '∅';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function metadataString(metadata: Record<string, unknown>, key: string) {
  const value = metadata[key];
  return typeof value === 'string' && value.trim() ? value : '';
}

function datasetHrefForNode(node: LineageNode) {
  const links = node.metadata?.links;
  if (links && typeof links === 'object' && !Array.isArray(links)) {
    const linkMap = links as Record<string, unknown>;
    for (const key of ['preview', 'self']) {
      const value = linkMap[key];
      if (typeof value === 'string' && value.trim()) return value;
    }
  }
  return `/datasets/${encodeURIComponent(node.id)}`;
}

interface CodeTabProps {
  selectedNode: LineageNode | null;
}
function CodeTab({ selectedNode }: CodeTabProps) {
  if (!selectedNode) {
    return <div style={tabHint}>Select a dataset to view its job-spec source.</div>;
  }
  const meta = selectedNode.metadata ?? {};
  const code =
    (typeof meta.transform_source === 'string' && meta.transform_source) ||
    (typeof meta.source === 'string' && meta.source) ||
    '';
  if (!code) {
    return (
      <div style={tabHint}>
        No transform source captured for {selectedNode.label}. Code is only available for datasets
        produced by Code Repository or Pipeline Builder transforms.
      </div>
    );
  }
  return (
    <pre style={codePre}>
      <code>{code}</code>
    </pre>
  );
}

interface DataHealthTabProps {
  selectedNode: LineageNode | null;
  impact: LineageImpactAnalysis | null;
  impactLoading: boolean;
  building: boolean;
  acknowledgeSensitiveLineage: boolean;
  onAckChange: (v: boolean) => void;
  sensitiveCandidateCount: number;
  onTriggerBuilds: () => void;
  onReloadImpact: () => void;
}
function DataHealthTab({
  selectedNode,
  impact,
  impactLoading,
  building,
  acknowledgeSensitiveLineage,
  onAckChange,
  sensitiveCandidateCount,
  onTriggerBuilds,
  onReloadImpact,
}: DataHealthTabProps) {
  if (!selectedNode || selectedNode.kind !== 'dataset') {
    return <div style={tabHint}>Select a dataset to inspect upstream / downstream impact.</div>;
  }
  if (impactLoading) return <div style={tabHint}>Loading impact analysis…</div>;
  if (!impact) {
    return (
      <div style={tabHint}>
        Impact data is not available.{' '}
        <button type="button" className="of-btn" onClick={onReloadImpact}>
          Retry
        </button>
      </div>
    );
  }
  return (
    <div style={{ display: 'grid', gap: 12, padding: '12px 16px', overflow: 'auto', height: '100%' }}>
      <div style={metricsGrid}>
        <MetricCard label="Upstream" value={impact.upstream.length} />
        <MetricCard label="Downstream" value={impact.downstream.length} />
        <MetricCard label="Build candidates" value={impact.build_candidates.length} />
        <MetricCard label="Propagated marking" value={impact.propagated_marking} />
      </div>

      {sensitiveCandidateCount > 0 && (
        <label
          className="of-status-warning"
          style={{
            display: 'flex',
            alignItems: 'flex-start',
            gap: 12,
            padding: '10px 14px',
            borderRadius: 'var(--radius-md)',
            fontSize: 13,
          }}
        >
          <input
            type="checkbox"
            checked={acknowledgeSensitiveLineage}
            onChange={(e) => onAckChange(e.target.checked)}
            style={{ marginTop: 2 }}
          />
          <span>
            {sensitiveCandidateCount} downstream build candidate(s) inherit confidential or PII
            lineage. Acknowledge before dispatching.
          </span>
        </label>
      )}

      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" className="of-btn" onClick={onReloadImpact} disabled={impactLoading}>
          {impactLoading ? 'Refreshing…' : 'Refresh impact'}
        </button>
        <button
          type="button"
          className="of-btn of-btn-primary"
          onClick={onTriggerBuilds}
          disabled={building || (sensitiveCandidateCount > 0 && !acknowledgeSensitiveLineage)}
        >
          {building ? 'Triggering…' : 'Build impacted'}
        </button>
      </div>

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
        <ImpactColumn title="Upstream" items={impact.upstream.slice(0, 8)} empty="No upstream dependencies." />
        <ImpactColumn title="Downstream" items={impact.downstream.slice(0, 8)} empty="No downstream dependencies." />
      </div>

      <div className="of-panel-muted" style={{ padding: 12 }}>
        <p className="of-eyebrow">Build candidates</p>
        <div style={{ marginTop: 8, display: 'grid', gap: 8 }}>
          {impact.build_candidates.length === 0 ? (
            <div className="of-text-muted" style={{ fontSize: 12 }}>
              No downstream pipelines or workflows are reachable from this dataset.
            </div>
          ) : (
            impact.build_candidates.map((c) => (
              <div key={c.id} style={candidateRow}>
                <div style={{ flex: 1 }}>
                  <div style={{ fontWeight: 500, fontSize: 13 }}>{c.label}</div>
                  <div className="of-eyebrow" style={{ marginTop: 4 }}>
                    {c.kind} · distance {c.distance} · path marking {c.effective_marking}
                  </div>
                  {c.requires_acknowledgement && (
                    <div style={{ marginTop: 4, fontSize: 11, color: 'var(--status-warning)' }}>
                      Sensitive lineage acknowledgment required
                    </div>
                  )}
                  {c.blocked_reason && (
                    <div style={{ marginTop: 4, fontSize: 11, color: 'var(--status-danger)' }}>
                      {c.blocked_reason}
                    </div>
                  )}
                </div>
                <div style={{ textAlign: 'right' }}>
                  <div
                    style={{
                      fontSize: 12,
                      fontWeight: 500,
                      color: c.triggerable ? 'var(--status-success)' : 'var(--text-muted)',
                    }}
                  >
                    {c.status ?? 'unknown'}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

interface BuildTimelineTabProps {
  buildResult: LineageBuildResult | null;
}
function BuildTimelineTab({ buildResult }: BuildTimelineTabProps) {
  if (!buildResult) {
    return (
      <div style={tabHint}>
        Trigger a downstream build from <strong>Data health</strong> to populate the build timeline.
      </div>
    );
  }
  const items = [...buildResult.triggered, ...buildResult.skipped];
  return (
    <div style={{ padding: '12px 16px', display: 'grid', gap: 8, overflow: 'auto', height: '100%' }}>
      <div className="of-text-muted" style={{ fontSize: 12 }}>
        {buildResult.triggered.length} triggered · {buildResult.skipped.length} skipped
      </div>
      {items.map((item) => (
        <div key={item.id} style={candidateRow}>
          <div style={{ flex: 1 }}>
            <div style={{ fontWeight: 500, fontSize: 13 }}>{item.label}</div>
            <div className="of-eyebrow" style={{ marginTop: 4 }}>
              {item.kind}
            </div>
          </div>
          <div style={{ textAlign: 'right' }}>
            <div style={{ fontWeight: 500, fontSize: 12 }}>{item.status}</div>
            <div style={{ fontSize: 11, color: 'var(--text-soft)' }}>
              {item.run_id ?? item.message ?? '—'}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

interface ImpactColumnProps {
  title: string;
  items: { id: string; label: string; kind: string; distance: number; marking: string; effective_marking: string }[];
  empty: string;
}
function ImpactColumn({ title, items, empty }: ImpactColumnProps) {
  return (
    <div className="of-panel-muted" style={{ padding: 12 }}>
      <p className="of-eyebrow">{title}</p>
      <div style={{ marginTop: 8, display: 'grid', gap: 6 }}>
        {items.length === 0 ? (
          <div className="of-text-muted" style={{ fontSize: 12 }}>
            {empty}
          </div>
        ) : (
          items.map((item) => (
            <div key={item.id} style={impactRow}>
              <div style={{ fontWeight: 500, fontSize: 12 }}>{item.label}</div>
              <div className="of-eyebrow" style={{ marginTop: 2 }}>
                {item.kind} · d{item.distance} · {item.marking}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="of-panel-muted" style={{ padding: 12 }}>
      <p className="of-eyebrow">{label}</p>
      <div style={{ marginTop: 4, fontSize: 22, fontWeight: 600 }}>{value}</div>
    </div>
  );
}

function ZoomButton({ title, onClick, children }: { title: string; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" title={title} onClick={onClick} style={zoomButton}>
      {children}
    </button>
  );
}

// =============================================================================
// Inline styles — keeps the layout self-contained next to the component logic.
// =============================================================================

const pageRoot: CSSProperties = {
  display: 'grid',
  gridTemplateRows: 'auto auto 1fr',
  height: 'calc(100vh - var(--topbar-height, 56px))',
  background: 'var(--bg-canvas)',
  margin: 0,
};

const headerRow: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 12,
  padding: '6px 12px',
  background: 'var(--bg-default)',
  borderBottom: '1px solid var(--border-default)',
};
const headerLeft: CSSProperties = { display: 'flex', alignItems: 'center', gap: 10, minWidth: 220 };
const lineageMark: CSSProperties = { display: 'inline-flex' };
const headerTitle: CSSProperties = { fontWeight: 600, fontSize: 14, color: 'var(--text-strong)' };
const headerCenter: CSSProperties = { flex: 1, display: 'flex', justifyContent: 'center', gap: 6 };
const headerRight: CSSProperties = { display: 'flex', alignItems: 'center', gap: 4 };

const branchPicker: CSSProperties = { position: 'relative' };
const branchTrigger: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 8,
  width: 280,
  height: 30,
  padding: '0 10px',
  border: '1px solid var(--border-default)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--bg-default)',
  color: 'var(--text-default)',
  fontSize: 13,
};
const branchMenu: CSSProperties = {
  position: 'absolute',
  top: 'calc(100% + 4px)',
  left: 0,
  width: 280,
  zIndex: 30,
  padding: 4,
  display: 'grid',
  gap: 2,
};

const iconBtn: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 30,
  height: 30,
  border: '1px solid transparent',
  background: 'transparent',
  color: 'var(--text-muted)',
  borderRadius: 'var(--radius-sm)',
};
const iconBtnGhost: CSSProperties = {
  ...iconBtn,
  width: 22,
  height: 22,
  fontSize: 13,
};

const savePrimary: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  height: 30,
  padding: '0 14px',
  border: '1px solid #18794a',
  background: '#1f9c5b',
  color: '#fff',
  borderRadius: 'var(--radius-sm) 0 0 var(--radius-sm)',
  fontWeight: 600,
  fontSize: 13,
};
const saveCaret: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  height: 30,
  width: 24,
  border: '1px solid #18794a',
  borderLeft: 'none',
  background: '#1f9c5b',
  color: '#fff',
  borderRadius: '0 var(--radius-sm) var(--radius-sm) 0',
};
const saveMenu: CSSProperties = {
  position: 'absolute',
  top: 'calc(100% + 4px)',
  right: 0,
  width: 200,
  zIndex: 30,
  padding: 4,
  display: 'grid',
  gap: 2,
};

const ribbonRow: CSSProperties = {
  display: 'flex',
  alignItems: 'stretch',
  gap: 4,
  padding: '6px 8px',
  background: 'var(--bg-panel-muted)',
  borderBottom: '1px solid var(--border-default)',
  minHeight: 56,
};
const ribbonGroup: CSSProperties = {
  display: 'inline-flex',
  background: 'var(--bg-default)',
  borderRadius: 'var(--radius-sm)',
  border: '1px solid var(--border-subtle)',
};
const toolBtn: CSSProperties = {
  display: 'inline-flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 2,
  minWidth: 56,
  padding: '4px 6px',
  border: '1px solid transparent',
  background: 'transparent',
  color: 'var(--text-default)',
  borderRadius: 'var(--radius-sm)',
};
const toolBtnActive: CSSProperties = {
  background: 'var(--bg-chip-active)',
  border: '1px solid var(--border-focus)',
  color: 'var(--text-link)',
};
const toolBtnDisabled: CSSProperties = { opacity: 0.4, cursor: 'not-allowed' };
const toolBtnIcon: CSSProperties = { display: 'inline-flex', alignItems: 'center', justifyContent: 'center' };
const toolBtnLabel: CSSProperties = {
  fontSize: 10,
  color: 'var(--text-muted)',
  textTransform: 'none',
  whiteSpace: 'pre',
  textAlign: 'center',
  lineHeight: 1.1,
  marginTop: 2,
};

const expandPopover: CSSProperties = {
  position: 'absolute',
  top: 'calc(100% + 4px)',
  left: 0,
  zIndex: 25,
  width: 320,
  padding: 12,
};
const expandRow: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 10,
  width: '100%',
  padding: '6px 8px',
  border: 'none',
  background: 'transparent',
  color: 'var(--text-default)',
  borderRadius: 'var(--radius-sm)',
  textAlign: 'left',
};
const hotkey: CSSProperties = { marginLeft: 'auto', color: 'var(--text-soft)', fontSize: 11 };
const expandStepperRow: CSSProperties = { display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' };

const stepperWrap: CSSProperties = { display: 'grid', gap: 4, justifyItems: 'center' };
const stepperControls: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'stretch',
  border: '1px solid var(--border-default)',
  borderRadius: 'var(--radius-sm)',
};
const stepperBtn: CSSProperties = {
  width: 24,
  height: 28,
  background: 'var(--bg-default)',
  border: 'none',
  borderRight: '1px solid var(--border-subtle)',
  color: 'var(--text-muted)',
};
const stepperValue: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 36,
  height: 28,
  fontWeight: 600,
};
const stepperHint: CSSProperties = { fontSize: 11, color: 'var(--text-muted)' };

const coloringPopover: CSSProperties = {
  position: 'absolute',
  top: 'calc(100% + 4px)',
  left: 0,
  zIndex: 25,
  width: 200,
  padding: 4,
};
const findPopover: CSSProperties = {
  position: 'absolute',
  top: 'calc(100% + 4px)',
  left: 0,
  zIndex: 25,
  width: 240,
  padding: 8,
};
const findInput: CSSProperties = {
  width: '100%',
  height: 28,
  padding: '0 8px',
  border: '1px solid var(--border-default)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--bg-default)',
  color: 'var(--text-default)',
  fontSize: 12,
};

const coloringSelect: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 4,
  height: 36,
  padding: '0 8px',
  border: '1px solid var(--border-default)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--bg-default)',
  fontSize: 12,
  minWidth: 170,
  color: 'var(--text-default)',
};

const canvasShell: CSSProperties = {
  display: 'grid',
  gridTemplateRows: '1fr auto',
  minHeight: 0,
};
const canvasArea: CSSProperties = {
  position: 'relative',
  background: 'var(--bg-canvas)',
  minHeight: 0,
};
const errorBanner: CSSProperties = {
  position: 'absolute',
  top: 12,
  left: 12,
  right: 12,
  zIndex: 5,
  padding: '10px 14px',
  borderRadius: 'var(--radius-md)',
  fontSize: 13,
};
const emptyState: CSSProperties = {
  position: 'absolute',
  inset: 0,
  display: 'grid',
  placeItems: 'center',
  color: 'var(--text-muted)',
  fontSize: 13,
};

const legendCard: CSSProperties = {
  position: 'absolute',
  top: 18,
  right: 80,
  zIndex: 8,
  padding: 12,
  display: 'grid',
  gap: 6,
  minWidth: 180,
};
const legendItem: CSSProperties = { display: 'flex', alignItems: 'center', gap: 8 };
const legendSwatch: CSSProperties = {
  width: 16,
  height: 16,
  borderRadius: 'var(--radius-sm)',
  border: '1px solid rgba(31,37,45,0.18)',
};

const zoomStack: CSSProperties = {
  position: 'absolute',
  bottom: 12,
  left: 12,
  display: 'grid',
  gap: 6,
  zIndex: 6,
};
const zoomButton: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 32,
  height: 32,
  background: 'var(--bg-default)',
  border: '1px solid var(--border-default)',
  borderRadius: 'var(--radius-sm)',
  color: 'var(--text-muted)',
  boxShadow: 'var(--shadow-popover)',
};

const rightRail: CSSProperties = {
  position: 'absolute',
  top: 12,
  right: 12,
  display: 'grid',
  gap: 4,
  background: 'var(--bg-default)',
  border: '1px solid var(--border-default)',
  borderRadius: 'var(--radius-sm)',
  padding: 4,
  zIndex: 6,
};
const rightRailButton: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 30,
  height: 30,
  background: 'transparent',
  border: '1px solid transparent',
  borderRadius: 'var(--radius-sm)',
  color: 'var(--text-muted)',
};
const rightRailButtonActive: CSSProperties = {
  background: 'var(--bg-chip-active)',
  borderColor: 'var(--border-focus)',
  color: 'var(--text-link)',
};
const rightRailCollapse: CSSProperties = {
  ...rightRailButton,
  marginTop: 2,
};
const rightRailDrawer: CSSProperties = {
  position: 'absolute',
  top: 12,
  right: 60,
  width: 320,
  height: 'calc(100% - 32px)',
  zIndex: 5,
  display: 'flex',
  flexDirection: 'column',
};

const resourceListRow: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 8,
  padding: '6px 8px',
  border: '1px solid var(--border-subtle)',
  borderRadius: 'var(--radius-sm)',
  background: 'var(--bg-default)',
  fontSize: 12,
  color: 'var(--text-default)',
};
const resourceDot: CSSProperties = {
  width: 12,
  height: 12,
  borderRadius: 'var(--radius-sm)',
  border: '1px solid rgba(31,37,45,0.2)',
};

const bottomPanel: CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  borderTop: '1px solid var(--border-default)',
  background: 'var(--bg-default)',
  overflow: 'hidden',
};
const bottomPanelTabs: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: 2,
  padding: '0 12px',
  borderBottom: '1px solid var(--border-subtle)',
  height: 36,
};
const bottomTabBtn: CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: 6,
  height: 32,
  padding: '0 10px',
  border: '1px solid transparent',
  borderBottom: 'none',
  background: 'transparent',
  color: 'var(--text-muted)',
  borderRadius: 'var(--radius-sm) var(--radius-sm) 0 0',
  fontSize: 12,
};
const bottomTabBtnActive: CSSProperties = {
  background: 'var(--bg-default)',
  border: '1px solid var(--border-default)',
  borderBottom: '1px solid var(--bg-default)',
  color: 'var(--text-link)',
  fontWeight: 600,
  position: 'relative',
  top: 1,
};
const selectedIndicator: CSSProperties = {
  marginRight: 12,
  fontSize: 12,
  color: 'var(--text-link)',
  fontWeight: 500,
};
const bottomPanelBody: CSSProperties = {
  flex: 1,
  minHeight: 0,
  overflow: 'auto',
  background: 'var(--bg-default)',
};

const tabHint: CSSProperties = {
  padding: 16,
  color: 'var(--text-muted)',
  fontSize: 12,
};

const previewTable: CSSProperties = {
  borderCollapse: 'collapse',
  width: '100%',
  fontSize: 12,
};
const previewTh: CSSProperties = {
  textAlign: 'left',
  padding: '6px 10px',
  borderBottom: '1px solid var(--border-default)',
  background: 'var(--bg-panel-muted)',
  fontWeight: 600,
  color: 'var(--text-strong)',
  position: 'sticky',
  top: 0,
};
const previewTd: CSSProperties = {
  padding: '6px 10px',
  borderBottom: '1px solid var(--border-subtle)',
  color: 'var(--text-default)',
};

const codePre: CSSProperties = {
  margin: 0,
  padding: 16,
  background: '#0f172a',
  color: '#e2e8f0',
  fontFamily: 'var(--font-mono)',
  fontSize: 12,
  whiteSpace: 'pre',
  overflow: 'auto',
  height: '100%',
};

const metricsGrid: CSSProperties = {
  display: 'grid',
  gap: 8,
  gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
};
const candidateRow: CSSProperties = {
  display: 'flex',
  alignItems: 'flex-start',
  gap: 12,
  border: '1px solid var(--border-subtle)',
  borderRadius: 'var(--radius-sm)',
  padding: 10,
  background: 'var(--bg-default)',
};
const impactRow: CSSProperties = {
  border: '1px solid var(--border-subtle)',
  borderRadius: 'var(--radius-sm)',
  padding: 8,
  background: 'var(--bg-default)',
};

function menuItem(active: boolean): CSSProperties {
  return {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    width: '100%',
    padding: '6px 10px',
    border: 'none',
    background: active ? 'var(--bg-chip-active)' : 'transparent',
    color: active ? 'var(--text-link)' : 'var(--text-default)',
    borderRadius: 'var(--radius-sm)',
    fontSize: 12,
    textAlign: 'left',
    fontWeight: active ? 600 : 400,
  };
}
