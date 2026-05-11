import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  createPipelineProposal,
  generatePipelineTransformById,
  getPipeline,
  listPipelineVersions,
  listRuns,
  pipelineDAGWithNodes,
  pipelineNodesFromDAG,
  pipelineSchemaGuidanceById,
  publishPipeline,
  retryPipelineRun,
  restorePipelineVersion,
  triggerRun,
  updatePipeline,
  validatePipelineById,
  type Pipeline,
  type PipelineDAG,
  type PipelineJoinSchemaGuidance,
  type PipelineNode,
  type PipelineRun,
  type PipelineVersion,
  type PipelineUnionSchemaGuidance,
  type PipelineValidationResponse,
} from '@/lib/api/pipelines';
import { JsonEditor } from '@/lib/components/JsonEditor';
import { Tabs } from '@/lib/components/Tabs';
import { Glyph } from '@/lib/components/ui/Glyph';
import { PipelineCanvas } from '@/lib/components/pipeline/PipelineCanvas';
import { NodePreviewPanel } from '@/lib/components/pipeline/NodePreviewPanel';
import { PipelineNodeList } from '@/lib/components/pipeline/PipelineNodeList';
import { AddFoundryDataDialog } from '@/lib/components/pipeline/AddFoundryDataDialog';
import { TransformStackEditor } from '@/lib/components/pipeline/TransformStackEditor';
import { composeTransformStackSql, type TransformStack } from '@/lib/components/pipeline/transformStack';
import { JoinEditor } from '@/lib/components/pipeline/JoinEditor';
import { composeJoinSql, newJoinDraft, type JoinDraft } from '@/lib/components/pipeline/joinDraft';
import { UnionEditor } from '@/lib/components/pipeline/UnionEditor';
import { composeUnionSql, newUnionDraft, type UnionDraft } from '@/lib/components/pipeline/unionDraft';
import { OutputDrawer, type OutputDraft } from '@/lib/components/pipeline/OutputDrawer';
import { DeployDrawer } from '@/lib/components/pipeline/DeployDrawer';
import { previewDataset, type Dataset } from '@/lib/api/datasets';

function parseJson<T>(value: string, fallback: T): T {
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

type PipelineOutputConfig = {
  kind?: string;
  object_type_id?: string;
  object_type_name?: string;
  primary_key?: string;
};

function outputConfigForNode(node?: PipelineNode): PipelineOutputConfig {
  if (!node || typeof node.config !== 'object' || node.config === null) return {};
  const config = node.config as Record<string, unknown>;
  const output = config._output;
  if (typeof output === 'object' && output !== null) return output as PipelineOutputConfig;
  return config as PipelineOutputConfig;
}

function isPipelineObjectOutput(node?: PipelineNode): node is PipelineNode {
  if (!node) return false;
  const output = outputConfigForNode(node);
  return output.kind === 'object_type' || node.transform_type.toLowerCase().includes('object');
}

export function PipelineEditPage() {
  const { id = '', runId } = useParams<{ id: string; runId?: string }>();
  const [pipeline, setPipeline] = useState<Pipeline | null>(null);
  const [runs, setRuns] = useState<PipelineRun[]>([]);
  const [versions, setVersions] = useState<PipelineVersion[]>([]);
  const [validation, setValidation] = useState<PipelineValidationResponse | null>(null);
  const [tab, setTab] = useState<'canvas' | 'nodes' | 'config' | 'runs' | 'history' | 'validate'>(runId ? 'runs' : 'canvas');

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [statusValue, setStatusValue] = useState('draft');
  const [branchName, setBranchName] = useState('main');
  const [nodesJson, setNodesJson] = useState('');
  const [scheduleJson, setScheduleJson] = useState('');
  const [retryJson, setRetryJson] = useState('');

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [historyBusy, setHistoryBusy] = useState(false);
  const [addDataOpen, setAddDataOpen] = useState(false);
  const [transformStack, setTransformStack] = useState<TransformStack | null>(null);
  const [transformOriginNodeId, setTransformOriginNodeId] = useState<string | null>(null);
  const [transformEditingNodeId, setTransformEditingNodeId] = useState<string | null>(null);
  const [joinDraft, setJoinDraft] = useState<JoinDraft | null>(null);
  const [joinOriginIds, setJoinOriginIds] = useState<{ left: string; right: string } | null>(null);
  const [joinLeftSchema, setJoinLeftSchema] = useState<string[]>([]);
  const [joinRightSchema, setJoinRightSchema] = useState<string[]>([]);
  const [joinGuidance, setJoinGuidance] = useState<PipelineJoinSchemaGuidance | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  function resolveSourceDatasetId(nodeId: string, allNodes: PipelineNode[]): string | null {
    const seen = new Set<string>();
    let queue: string[] = [nodeId];
    while (queue.length > 0) {
      const next: string[] = [];
      for (const id of queue) {
        if (seen.has(id)) continue;
        seen.add(id);
        const node = allNodes.find((entry) => entry.id === id);
        if (!node) continue;
        const config = node.config as { dataset_id?: unknown } | undefined;
        if (typeof config?.dataset_id === 'string') return config.dataset_id;
        if (node.input_dataset_ids[0]) return node.input_dataset_ids[0];
        next.push(...node.depends_on);
      }
      queue = next;
    }
    return null;
  }

  function currentDAG(): PipelineDAG {
    return parseJson<PipelineDAG>(nodesJson, pipeline?.dag ?? []);
  }

  function currentNodes(): PipelineNode[] {
    return pipelineNodesFromDAG(currentDAG());
  }

  function setPipelineNodes(nodes: PipelineNode[]) {
    setNodesJson(JSON.stringify(pipelineDAGWithNodes(currentDAG(), nodes), null, 2));
  }

  async function fetchSchemaForNode(node: PipelineNode, allNodes: PipelineNode[]): Promise<string[]> {
    const datasetId = resolveSourceDatasetId(node.id, allNodes);
    if (!datasetId) return [];
    try {
      const preview = await previewDataset(datasetId, { limit: 1 });
      return preview.columns?.map((column) => column.name) ?? [];
    } catch {
      return [];
    }
  }

  function handleStartJoin(left: PipelineNode, right: PipelineNode) {
    const draft = newJoinDraft({ id: left.id, label: left.label }, { id: right.id, label: right.label });
    setJoinDraft(draft);
    setJoinOriginIds({ left: left.id, right: right.id });
    const allNodes = currentNodes();
    setJoinLeftSchema([]);
    setJoinRightSchema([]);
    setJoinGuidance(null);
    void fetchSchemaForNode(left, allNodes).then(setJoinLeftSchema);
    void fetchSchemaForNode(right, allNodes).then(setJoinRightSchema);
    if (pipeline) {
      void pipelineSchemaGuidanceById(pipeline.id, {
        dag: currentDAG(),
        kind: 'join',
        left_node_id: left.id,
        right_node_id: right.id,
        join: draft as unknown as Record<string, unknown>,
      }).then((response) => {
        if (response.join) {
          setJoinGuidance(response.join);
          setJoinLeftSchema(response.join.left_schema.map((field) => field.name));
          setJoinRightSchema(response.join.right_schema.map((field) => field.name));
        }
      }).catch(() => {
        // Dataset preview fallback remains available when guidance is unavailable.
      });
    }
  }

  function handleApplyJoin(next: JoinDraft) {
    if (!joinOriginIds) return;
    const existing = currentNodes();
    const sql = composeJoinSql(next);
    const newId = makeNodeId('join');
    const newNode: PipelineNode = {
      id: newId,
      label: next.display_name || `Join ${next.left_node_label}`,
      transform_type: 'sql',
      config: { sql, _join: next },
      depends_on: [joinOriginIds.left, joinOriginIds.right],
      input_dataset_ids: [],
      output_dataset_id: null,
    };
    setPipelineNodes([...existing, newNode]);
  }

  const [unionDraft, setUnionDraft] = useState<UnionDraft | null>(null);
  const [unionInputIds, setUnionInputIds] = useState<string[]>([]);
  const [unionGuidance, setUnionGuidance] = useState<PipelineUnionSchemaGuidance | null>(null);
  const [outputDraft, setOutputDraft] = useState<OutputDraft | null>(null);
  const [outputNodeId, setOutputNodeId] = useState<string | null>(null);
  const [deployOpen, setDeployOpen] = useState(false);
  const [aipOpen, setAipOpen] = useState(false);
  const [aipPrompt, setAipPrompt] = useState('');
  const [aipBusy, setAipBusy] = useState(false);
  const [aipMessage, setAipMessage] = useState('');

  function handleAddOutput(source: PipelineNode, kind: 'dataset' | 'object_type' | 'link_type' | 'time_series' | 'virtual_table') {
    if (kind !== 'dataset' && kind !== 'object_type' && kind !== 'link_type') return;
    const existing = currentNodes();
    const stamp = new Date().toLocaleString('en-US', {
      weekday: 'short',
      month: 'short',
      day: 'numeric',
      year: 'numeric',
      hour: 'numeric',
      minute: '2-digit',
      second: '2-digit',
    });
    const displayName = kind === 'object_type' ? `New object type ${stamp}` : kind === 'link_type' ? `New link type ${stamp}` : `New dataset ${stamp}`;
    const sourceConfig = source.config as { _stack?: { blocks?: unknown[] }; _join?: unknown; _union?: unknown } | undefined;
    const totalColumns = (() => {
      // Best-effort estimate without running the engine.
      if (sourceConfig?._join) return 11;
      if (sourceConfig?._union) return 11;
      return 11;
    })();
    const newId = makeNodeId('output');
    const targetDatasetId = crypto.randomUUID();
    const datasetRID = `ri.foundry.main.dataset.${targetDatasetId}`;
    const targetObjectTypeId = crypto.randomUUID();
    const targetLinkTypeId = crypto.randomUUID();
    const objectTypeName = `${source.label || source.id} object`.replace(/[^a-zA-Z0-9_]+/g, '_').replace(/^_+|_+$/g, '') || 'PipelineObject';
    const objectOutputs = existing.filter(isPipelineObjectOutput);
    const sourceObjectOutput = isPipelineObjectOutput(source) ? source : objectOutputs[0];
    const targetObjectOutput = objectOutputs.find((node) => node.id !== sourceObjectOutput?.id);
    const sourceObjectOutputConfig = outputConfigForNode(sourceObjectOutput);
    const targetObjectOutputConfig = outputConfigForNode(targetObjectOutput);
    const sourceObjectName = sourceObjectOutput?.label || sourceObjectOutputConfig.object_type_name || 'Source';
    const targetObjectName = targetObjectOutput?.label || targetObjectOutputConfig.object_type_name || 'Target';
    const linkTypeName = `${sourceObjectName} to ${targetObjectName}`.replace(/[^a-zA-Z0-9_]+/g, '_').replace(/^_+|_+$/g, '') || 'PipelineLink';
    const sourceObjectTypeId = sourceObjectOutputConfig.object_type_id || crypto.randomUUID();
    const targetObjectTypeIdForLink = targetObjectOutputConfig.object_type_id || crypto.randomUUID();
    const sourcePrimaryKey = sourceObjectOutputConfig.primary_key || 'id';
    const targetPrimaryKey = targetObjectOutputConfig.primary_key || 'id';
    const dependsOn = kind === 'link_type'
      ? Array.from(new Set([source.id, sourceObjectOutput?.id, targetObjectOutput?.id].filter(Boolean) as string[]))
      : [source.id];
    const newNode: PipelineNode = {
      id: newId,
      label: displayName,
      transform_type: kind === 'object_type' ? 'output_object_type' : kind === 'link_type' ? 'output_link_type' : 'output_dataset',
      config: {
        _output: {
          kind,
          dataset_id: targetDatasetId,
          dataset_rid: datasetRID,
          dataset_name: displayName,
          display_name: displayName,
          branch: 'main',
          write_mode: 'SNAPSHOT',
          file_format: 'PARQUET',
          logical_path: 'part-00000.ndjson',
          ...(kind === 'object_type' ? {
            object_type_id: targetObjectTypeId,
            object_type_name: objectTypeName,
            plural_display_name: `${displayName}s`,
            primary_key: 'id',
            icon: 'cube',
            color: '#2d72d2',
            editable: true,
            allow_edits: true,
            property_mapping: [
              { source_field: 'id', target_property: 'id', property_type: 'string', display_name: 'ID', required: true, unique_constraint: true },
            ],
          } : kind === 'link_type' ? {
            link_type_id: targetLinkTypeId,
            link_type_name: linkTypeName,
            link_display_name: displayName,
            cardinality: 'many_to_many',
            source_object_node_id: sourceObjectOutput?.id || '',
            target_object_node_id: targetObjectOutput?.id || '',
            source_object_type_id: sourceObjectTypeId,
            target_object_type_id: targetObjectTypeIdForLink,
            source_primary_key: sourcePrimaryKey,
            target_primary_key: targetPrimaryKey,
            source_key_column: sourcePrimaryKey,
            target_key_column: targetPrimaryKey,
            tenant: 'default',
          } : {}),
          columns_total: totalColumns,
          columns_mapped: totalColumns,
        },
      },
      depends_on: dependsOn,
      input_dataset_ids: [],
      output_dataset_id: targetDatasetId,
    };
    setPipelineNodes([...existing, newNode]);
    setOutputDraft({
      kind,
      display_name: displayName,
      source_node_id: source.id,
      source_node_label: source.label,
      columns_total: totalColumns,
      columns_mapped: totalColumns,
    });
    setOutputNodeId(newId);
  }

  function handleRenameOutput(name: string) {
    setOutputDraft((current) => (current ? { ...current, display_name: name } : current));
    if (!outputNodeId) return;
    const existing = currentNodes();
    const updated = existing.map((node) => {
      if (node.id !== outputNodeId) return node;
      const currentConfig = (node.config ?? {}) as Record<string, unknown>;
      const currentOutput = (currentConfig._output ?? {}) as Record<string, unknown>;
      return {
        ...node,
        label: name,
        config: {
          ...currentConfig,
          _output: { ...currentOutput, dataset_name: name, display_name: name },
        },
      };
    });
    setPipelineNodes(updated);
  }

  function handleStartUnion(inputs: PipelineNode[]) {
    const draft = newUnionDraft(inputs.map((entry) => ({ id: entry.id, label: entry.label })));
    setUnionDraft(draft);
    setUnionInputIds(inputs.map((entry) => entry.id));
    setUnionGuidance(null);
    if (pipeline) {
      void pipelineSchemaGuidanceById(pipeline.id, {
        dag: currentDAG(),
        kind: 'union',
        input_node_ids: inputs.map((entry) => entry.id),
        union: { union_type: draft.union_type },
      }).then((response) => {
        if (response.union) setUnionGuidance(response.union);
      }).catch(() => {
        // Union can still be authored; strict validation will catch issues later.
      });
    }
  }

  function handleApplyUnion(next: UnionDraft) {
    if (unionInputIds.length < 2) return;
    const existing = currentNodes();
    const sql = composeUnionSql(next);
    const newId = makeNodeId('union');
    const newNode: PipelineNode = {
      id: newId,
      label: next.display_name || 'Union',
      transform_type: 'sql',
      config: { sql, _union: next },
      depends_on: [...unionInputIds],
      input_dataset_ids: [],
      output_dataset_id: null,
    };
    setPipelineNodes([...existing, newNode]);
  }

  function handleOpenTransform(node: PipelineNode) {
    const config = node.config as Record<string, unknown> | undefined;
    const storedStack = config?._stack as TransformStack | undefined;
    if (storedStack) {
      setTransformStack(storedStack);
      setTransformOriginNodeId(node.depends_on[0] ?? node.id);
      setTransformEditingNodeId(node.id);
      return;
    }
    const datasetName =
      typeof config?.dataset_name === 'string'
        ? (config.dataset_name as string)
        : node.label.replace(/^Read\s+/, '');
    const datasetId = node.input_dataset_ids[0] ?? (typeof config?.dataset_id === 'string' ? (config.dataset_id as string) : '');
    setTransformStack({
      source_dataset_id: datasetId,
      source_dataset_name: datasetName,
      display_name: `Clean ${datasetName}`,
      blocks: [],
    });
    setTransformOriginNodeId(node.id);
    setTransformEditingNodeId(null);
  }

  function handleApplyTransformStack(next: TransformStack) {
    if (!transformOriginNodeId) return;
    const sourceNodeId = transformEditingNodeId
      ? (currentNodes().find((entry) => entry.id === transformEditingNodeId)?.depends_on[0] ?? transformOriginNodeId)
      : transformOriginNodeId;
    const existing = currentNodes();
    const sql = composeTransformStackSql(next);
    if (transformEditingNodeId) {
      const updated = existing.map((entry) => {
        if (entry.id !== transformEditingNodeId) return entry;
        return {
          ...entry,
          label: next.display_name || entry.label,
          transform_type: 'sql',
          config: { sql, _stack: next },
        };
      });
      setPipelineNodes(updated);
      return;
    }
    const newId = makeNodeId('transform');
    const newNode: PipelineNode = {
      id: newId,
      label: next.display_name || `Transform ${next.source_dataset_name}`,
      transform_type: 'sql',
      config: { sql, _stack: next },
      depends_on: [sourceNodeId],
      input_dataset_ids: next.source_dataset_id ? [next.source_dataset_id] : [],
      output_dataset_id: null,
    };
    setPipelineNodes([...existing, newNode]);
    setTransformEditingNodeId(newId);
  }

  function isPipelineEmpty(nodes: PipelineNode[]) {
    if (nodes.length === 0) return true;
    if (nodes.length > 1) return false;
    const only = nodes[0];
    if (only.transform_type !== 'sql') return false;
    const config = only.config as { sql?: string } | undefined;
    return Boolean(config?.sql && /^SELECT\s+1\s+AS/i.test(config.sql.trim()));
  }

  function makeNodeId(prefix = 'source') {
    if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
    return `${prefix}_${Date.now()}_${Math.floor(Math.random() * 1e6)}`;
  }

  function handleAddDatasets(datasets: Dataset[]) {
    if (datasets.length === 0) return;
    const existing = currentNodes();
    const startsEmpty = isPipelineEmpty(existing);
    const baseNodes: PipelineNode[] = startsEmpty ? [] : [...existing];
    for (const dataset of datasets) {
      baseNodes.push({
        id: makeNodeId('source'),
        label: `Read ${dataset.name}`,
        transform_type: 'external',
        config: { source_kind: 'dataset', dataset_id: dataset.id, dataset_name: dataset.name },
        depends_on: [],
        input_dataset_ids: [dataset.id],
        output_dataset_id: null,
      });
    }
    setPipelineNodes(baseNodes);
  }

  async function load() {
    if (!id) return;
    setLoading(true);
    setError('');
    try {
      const nextPipeline = await getPipeline(id);
      setPipeline(nextPipeline);
      setName(nextPipeline.name);
      setDescription(nextPipeline.description);
      setStatusValue(nextPipeline.status);
      setBranchName(nextPipeline.branch_name || 'main');
      setNodesJson(JSON.stringify(nextPipeline.dag, null, 2));
      setScheduleJson(JSON.stringify(nextPipeline.schedule_config, null, 2));
      setRetryJson(JSON.stringify(nextPipeline.retry_policy, null, 2));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load pipeline');
    } finally {
      setLoading(false);
    }
  }

  async function loadRuns() {
    if (!id) return;
    try {
      const res = await listRuns(id, { per_page: 50 });
      setRuns(res.data);
    } catch {
      // Runs are helpful context, but the editor should still load without them.
    }
  }

  async function loadVersions() {
    if (!id) return;
    try {
      const res = await listPipelineVersions(id);
      setVersions(res.data);
    } catch {
      // History is additive context; authoring should still work if the backend is not wired.
      setVersions([]);
    }
  }

  useEffect(() => {
    void load();
    void loadRuns();
    void loadVersions();
  }, [id]);

  useEffect(() => {
    if (runId) setTab('runs');
  }, [runId]);

  async function save() {
    if (!pipeline) return;
    setSaving(true);
    setError('');
    try {
      const updated = await updatePipeline(pipeline.id, {
        name,
        description,
        status: statusValue,
        branch_name: branchName.trim() || 'main',
        dag: parseJson<PipelineDAG>(nodesJson, pipeline.dag),
        schedule_config: parseJson(scheduleJson, pipeline.schedule_config),
        retry_policy: parseJson(retryJson, pipeline.retry_policy),
      });
      setPipeline(updated);
      setBranchName(updated.branch_name || branchName.trim() || 'main');
      setNodesJson(JSON.stringify(updated.dag, null, 2));
      setScheduleJson(JSON.stringify(updated.schedule_config, null, 2));
      setRetryJson(JSON.stringify(updated.retry_policy, null, 2));
      await loadVersions();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setSaving(false);
    }
  }

  async function runNow() {
    if (!pipeline) return;
    setBusy(true);
    try {
      await triggerRun(pipeline.id);
      await loadRuns();
      setTab('runs');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Run failed');
    } finally {
      setBusy(false);
    }
  }

  async function retryRun(selectedRunId: string) {
    if (!pipeline) return;
    setBusy(true);
    try {
      await retryPipelineRun(pipeline.id, selectedRunId);
      await loadRuns();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Retry failed');
    } finally {
      setBusy(false);
    }
  }

  async function runValidate() {
    if (!pipeline) return;
    setBusy(true);
    try {
      const report = await validatePipelineById(pipeline.id);
      setValidation({
        valid: report.all_valid,
        errors: report.nodes.flatMap((node) => node.errors.map((issue) => `${node.node_id}: ${issue.message}`)),
        warnings: [],
        next_run_at: null,
        summary: { node_count: report.nodes.length, edge_count: 0, root_node_ids: [], leaf_node_ids: [] },
      });
      setTab('validate');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Validate failed');
    } finally {
      setBusy(false);
    }
  }

  async function generateAIPTransform() {
    if (!pipeline || aipPrompt.trim() === '') return;
    setAipBusy(true);
    setAipMessage('');
    setError('');
    try {
      const existing = currentNodes();
      const selected = selectedNodeId ? [selectedNodeId] : existing.length > 0 ? [existing[existing.length - 1].id] : [];
      const response = await generatePipelineTransformById(pipeline.id, {
        prompt: aipPrompt.trim(),
        dag: currentDAG(),
        selected_node_ids: selected,
        sample_size: 10,
      });
      if (response.nodes.length > 0) {
        setPipelineNodes([...existing, ...response.nodes]);
        setSelectedNodeId(response.nodes[response.nodes.length - 1].id);
      }
      setAipMessage(response.preview_error?.message || response.description);
      setAipPrompt('');
      setAipOpen(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'AIP generation failed');
    } finally {
      setAipBusy(false);
    }
  }

  function applyLifecyclePipeline(nextPipeline: Pipeline) {
    setPipeline(nextPipeline);
    setName(nextPipeline.name);
    setDescription(nextPipeline.description);
    setStatusValue(nextPipeline.status);
    setBranchName(nextPipeline.branch_name || 'main');
    setNodesJson(JSON.stringify(nextPipeline.dag, null, 2));
    setScheduleJson(JSON.stringify(nextPipeline.schedule_config, null, 2));
    setRetryJson(JSON.stringify(nextPipeline.retry_policy, null, 2));
  }

  async function openProposal() {
    if (!pipeline) return;
    setHistoryBusy(true);
    setError('');
    try {
      const title = `${name || pipeline.name} proposal`;
      const response = await createPipelineProposal(pipeline.id, {
        title,
        description: description || 'Pipeline draft ready for review.',
        branch_name: branchName.trim() || 'main',
      });
      applyLifecyclePipeline(response.pipeline);
      await loadVersions();
      setTab('history');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Proposal failed');
    } finally {
      setHistoryBusy(false);
    }
  }

  async function publishDraft() {
    if (!pipeline) return;
    setHistoryBusy(true);
    setError('');
    try {
      const response = await publishPipeline(pipeline.id, {
        message: `Published ${new Date().toLocaleString()}`,
        branch_name: branchName.trim() || 'main',
        proposal_title: pipeline.proposal_title || undefined,
        proposal_description: pipeline.proposal_description || undefined,
      });
      applyLifecyclePipeline(response.pipeline);
      await loadVersions();
      setTab('history');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Publish failed');
    } finally {
      setHistoryBusy(false);
    }
  }

  async function restoreVersion(version: PipelineVersion, asDraft = true) {
    if (!pipeline) return;
    const target = asDraft ? 'draft' : 'published pipeline';
    if (typeof window !== 'undefined' && !window.confirm(`Restore version ${version.version_number} as ${target}?`)) return;
    setHistoryBusy(true);
    setError('');
    try {
      const response = await restorePipelineVersion(pipeline.id, version.id, {
        as_draft: asDraft,
        message: `Restored version ${version.version_number}`,
      });
      applyLifecyclePipeline(response.pipeline);
      await loadVersions();
      setTab('history');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Restore failed');
    } finally {
      setHistoryBusy(false);
    }
  }

  const parsedDAG = currentDAG();
  const parsedNodes = pipelineNodesFromDAG(parsedDAG);
  const selectedPreviewNode = selectedNodeId ? (parsedNodes.find((node) => node.id === selectedNodeId) ?? null) : null;
  const highlightedRun = runId ? runs.find((run) => run.id === runId) : null;

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading pipeline...</p>
      </section>
    );
  }

  if (!pipeline) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/pipelines" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
          Back to pipelines
        </Link>
        <p className="of-status-danger" style={{ marginTop: 12 }}>
          {error || 'Pipeline not found'}
        </p>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 10 }}>
      <header className="of-panel" style={{ display: 'grid', gap: 8, padding: 10 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
          <div style={{ minWidth: 0 }}>
            <Link to="/pipelines" style={{ color: 'var(--text-muted)', fontSize: 12 }}>
              Back to pipelines
            </Link>
            <h1 className="of-heading-lg" style={{ marginTop: 4 }}>
              {pipeline.name}
            </h1>
            <p className="of-text-muted" style={{ marginTop: 2, fontSize: 11, fontFamily: 'var(--font-mono)' }}>
              {pipeline.id}
            </p>
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
            <span className="of-chip of-chip-active">{pipeline.status}</span>
            <span className="of-chip">{pipeline.branch_name || branchName || 'main'}</span>
            <span className="of-chip">{pipeline.proposal_state || 'none'}</span>
            <span className="of-chip">{pipeline.published_at ? `published ${new Date(pipeline.published_at).toLocaleDateString()}` : 'unpublished'}</span>
            <span className="of-chip">{pipeline.pipeline_type ?? 'BATCH'}</span>
          </div>
        </div>
        <div
          className="of-toolbar"
          style={{
            borderRadius: 0,
            margin: '0 -10px -10px',
            borderRight: 0,
            borderLeft: 0,
            borderBottom: 0,
            justifyContent: 'space-between',
          }}
        >
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
            <select value={statusValue} onChange={(event) => setStatusValue(event.target.value)} className="of-select" style={{ width: 120 }}>
              <option value="draft">draft</option>
              <option value="active">active</option>
              <option value="paused">paused</option>
              <option value="archived">archived</option>
            </select>
            <span className="of-text-muted" style={{ alignSelf: 'center', fontSize: 11 }}>
              {runs.length} run{runs.length === 1 ? '' : 's'} | {versions.length} version{versions.length === 1 ? '' : 's'}
            </span>
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
            <button type="button" onClick={() => setAipOpen(true)} disabled={aipBusy} className="of-button">
              <Glyph name="sparkles" size={12} /> AIP Generate
            </button>
            <button type="button" onClick={() => void runValidate()} disabled={busy} className="of-button">
              Validate
            </button>
            <button type="button" onClick={() => void openProposal()} disabled={historyBusy} className="of-button">
              Open proposal
            </button>
            <button type="button" onClick={() => void publishDraft()} disabled={historyBusy} className="of-button">
              Publish draft
            </button>
            <button type="button" onClick={() => void runNow()} disabled={busy} className="of-button">
              Run now
            </button>
            <button type="button" onClick={() => void save()} disabled={saving} className="of-button of-button--primary">
              {saving ? 'Saving...' : 'Save'}
            </button>
            <button
              type="button"
              onClick={() => setDeployOpen(true)}
              className="of-button"
              style={{ borderColor: '#15803d', color: '#15803d', fontWeight: 600 }}
            >
              Deploy
            </button>
          </div>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {error}
        </div>
      )}

      {highlightedRun && (
        <section className="of-panel" style={{ padding: 10 }}>
          <p className="of-eyebrow">Selected run</p>
          <p style={{ margin: '6px 0 0', fontSize: 13 }}>
            {highlightedRun.id} | {highlightedRun.status} | attempt {highlightedRun.attempt_number}
          </p>
        </section>
      )}

      <section className="of-panel" style={{ overflow: 'hidden' }}>
        <Tabs tabs={['canvas', 'nodes', 'config', 'runs', 'history', 'validate'] as const} active={tab} onChange={setTab} />

        <div style={{ padding: tab === 'canvas' ? 0 : 10 }}>
          {tab === 'canvas' && (
            <div style={{ position: 'relative' }}>
              <PipelineCanvas
                nodes={parsedNodes}
                status={statusValue}
                scheduleConfig={parseJson(scheduleJson, { enabled: false, cron: null })}
                onChange={(next) => setPipelineNodes(next)}
                onTransform={(node) => handleOpenTransform(node)}
                onJoinStart={(left, right) => handleStartJoin(left, right)}
                onUnionStart={(inputs) => handleStartUnion(inputs)}
                onAddOutput={(source, kind) => handleAddOutput(source, kind)}
                onSelect={(node) => setSelectedNodeId(node?.id ?? null)}
              />
              {isPipelineEmpty(parsedNodes) ? (
                <PipelineWelcomePanel onAddFoundryData={() => setAddDataOpen(true)} />
              ) : null}
              <NodePreviewPanel pipelineId={pipeline.id} node={selectedPreviewNode} draftDag={parsedDAG} draftKey={nodesJson} />
              {aipOpen && (
                <PipelineAIPGeneratePanel
                  prompt={aipPrompt}
                  busy={aipBusy}
                  selectedNodeLabel={selectedPreviewNode?.label ?? (parsedNodes.length > 0 ? parsedNodes[parsedNodes.length - 1].label : 'New graph')}
                  onPromptChange={setAipPrompt}
                  onClose={() => setAipOpen(false)}
                  onGenerate={() => void generateAIPTransform()}
                />
              )}
              {aipMessage && !aipOpen && (
                <div
                  className="of-panel"
                  style={{
                    position: 'absolute',
                    right: 12,
                    bottom: 12,
                    width: 340,
                    padding: 10,
                    zIndex: 8,
                    fontSize: 12,
                    boxShadow: '0 12px 24px rgba(15, 23, 42, 0.08)',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
                    <span>{aipMessage}</span>
                    <button type="button" className="of-button" style={{ fontSize: 11 }} onClick={() => setAipMessage('')}>
                      Dismiss
                    </button>
                  </div>
                </div>
              )}
            </div>
          )}

          {tab === 'nodes' && (
            <PipelineNodeList nodes={parsedNodes} onChange={(next) => setPipelineNodes(next)} />
          )}

          {tab === 'config' && (
            <section style={{ display: 'grid', gap: 8 }}>
              <label style={{ fontSize: 12 }}>
                Name
                <input value={name} onChange={(event) => setName(event.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 12 }}>
                Description
                <input
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  className="of-input"
                  style={{ marginTop: 4 }}
                />
              </label>
              <label style={{ fontSize: 12 }}>
                Branch
                <input
                  value={branchName}
                  onChange={(event) => setBranchName(event.target.value)}
                  className="of-input"
                  placeholder="main"
                  style={{ marginTop: 4 }}
                />
              </label>
              <JsonEditor label="Nodes JSON (DAG)" value={nodesJson} onChange={setNodesJson} minHeight={320} />
              <JsonEditor label="Schedule config JSON" value={scheduleJson} onChange={setScheduleJson} minHeight={80} />
              <JsonEditor label="Retry policy JSON" value={retryJson} onChange={setRetryJson} minHeight={80} />
            </section>
          )}

          {tab === 'runs' && (
            <table className="of-table">
              <thead>
                <tr>
                  <th>Status</th>
                  <th>Attempt</th>
                  <th>Trigger</th>
                  <th>Started</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {runs.map((run) => (
                  <tr key={run.id} style={run.id === runId ? { outline: '2px solid var(--accent-default)' } : undefined}>
                    <td>{run.status}</td>
                    <td>{run.attempt_number}</td>
                    <td>{run.trigger_type}</td>
                    <td>{new Date(run.started_at).toLocaleString()}</td>
                    <td style={{ textAlign: 'right' }}>
                      <button type="button" onClick={() => void retryRun(run.id)} disabled={busy} className="of-button" style={{ fontSize: 11 }}>
                        Retry
                      </button>
                    </td>
                  </tr>
                ))}
                {runs.length === 0 && (
                  <tr>
                    <td colSpan={5} className="of-text-muted">
                      No runs yet.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          )}

          {tab === 'history' && (
            <section style={{ display: 'grid', gap: 10 }}>
              <div className="of-toolbar" style={{ justifyContent: 'space-between' }}>
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
                  <span className="of-chip">draft: {pipeline.draft_updated_at ? new Date(pipeline.draft_updated_at).toLocaleString() : 'not saved'}</span>
                  <span className="of-chip">published: {pipeline.published_at ? new Date(pipeline.published_at).toLocaleString() : 'none'}</span>
                  <span className="of-chip">active: {pipeline.active_version_id ? pipeline.active_version_id.slice(0, 8) : 'none'}</span>
                </div>
                <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                  <button type="button" onClick={() => void loadVersions()} disabled={historyBusy} className="of-button">
                    Refresh
                  </button>
                  <button type="button" onClick={() => void publishDraft()} disabled={historyBusy} className="of-button of-button--primary">
                    Publish draft
                  </button>
                </div>
              </div>
              <table className="of-table">
                <thead>
                  <tr>
                    <th>Version</th>
                    <th>Kind</th>
                    <th>Branch</th>
                    <th>Message</th>
                    <th>Created</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {versions.map((version) => (
                    <tr key={version.id}>
                      <td style={{ fontFamily: 'var(--font-mono)' }}>v{version.version_number}</td>
                      <td>
                        <span className={version.id === pipeline.active_version_id ? 'of-chip of-chip-active' : 'of-chip'}>
                          {version.version_kind}
                        </span>
                      </td>
                      <td>{version.branch_name || 'main'}</td>
                      <td>
                        <div style={{ display: 'grid', gap: 2 }}>
                          <span>{version.message || `${version.name} snapshot`}</span>
                          {version.restored_from_version_id && (
                            <span className="of-text-muted" style={{ fontSize: 11 }}>
                              restored from {version.restored_from_version_id.slice(0, 8)}
                            </span>
                          )}
                        </div>
                      </td>
                      <td>{new Date(version.created_at).toLocaleString()}</td>
                      <td style={{ textAlign: 'right' }}>
                        <div style={{ display: 'flex', gap: 6, justifyContent: 'flex-end', flexWrap: 'wrap' }}>
                          <button
                            type="button"
                            onClick={() => void restoreVersion(version, true)}
                            disabled={historyBusy}
                            className="of-button"
                            style={{ fontSize: 11 }}
                          >
                            Restore draft
                          </button>
                          <button
                            type="button"
                            onClick={() => void restoreVersion(version, false)}
                            disabled={historyBusy}
                            className="of-button"
                            style={{ fontSize: 11 }}
                          >
                            Restore + publish
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                  {versions.length === 0 && (
                    <tr>
                      <td colSpan={6} className="of-text-muted">
                        No saved versions yet.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </section>
          )}

          {tab === 'validate' && (
            <section>
              {validation ? (
                <>
                  <p className="of-eyebrow">{validation.valid ? 'Valid' : 'Invalid'}</p>
                  {validation.errors.length > 0 && (
                    <ul style={{ marginTop: 8, paddingLeft: 18, fontSize: 12 }}>
                      {validation.errors.map((validationError, index) => (
                        <li key={index} style={{ color: '#b42318' }}>
                          {validationError}
                        </li>
                      ))}
                    </ul>
                  )}
                </>
              ) : (
                <p className="of-text-muted">Click "Validate" to run server-side DAG validation.</p>
              )}
            </section>
          )}
        </div>
      </section>

      <AddFoundryDataDialog
        open={addDataOpen}
        onClose={() => setAddDataOpen(false)}
        onAdd={handleAddDatasets}
      />

      <TransformStackEditor
        open={Boolean(transformStack)}
        stack={transformStack}
        onClose={() => {
          setTransformStack(null);
          setTransformOriginNodeId(null);
          setTransformEditingNodeId(null);
        }}
        onApplyAll={handleApplyTransformStack}
      />

      <JoinEditor
        open={Boolean(joinDraft)}
        draft={joinDraft}
        leftSchema={joinLeftSchema}
        rightSchema={joinRightSchema}
        onClose={() => {
          setJoinDraft(null);
          setJoinOriginIds(null);
          setJoinLeftSchema([]);
          setJoinRightSchema([]);
          setJoinGuidance(null);
        }}
        guidance={joinGuidance}
        onApply={handleApplyJoin}
      />

      <UnionEditor
        open={Boolean(unionDraft)}
        draft={unionDraft}
        onClose={() => {
          setUnionDraft(null);
          setUnionInputIds([]);
          setUnionGuidance(null);
        }}
        guidance={unionGuidance}
        onApply={handleApplyUnion}
      />

      <OutputDrawer
        open={Boolean(outputDraft)}
        draft={outputDraft}
        onClose={() => {
          setOutputDraft(null);
          setOutputNodeId(null);
        }}
        onChangeName={handleRenameOutput}
      />

      <DeployDrawer
        open={deployOpen}
        pipelineId={pipeline?.id ?? null}
        outputs={parsedNodes
          .filter((node) => node.transform_type === 'output_dataset')
          .map((node) => ({ id: node.id, label: node.label }))}
        lastDeploymentLabel={runs.length > 0 ? new Date(runs[0].started_at).toLocaleString() : 'None'}
        onClose={() => setDeployOpen(false)}
        onDeployed={() => {
          void loadRuns();
          setTab('runs');
        }}
      />
    </section>
  );
}

function PipelineWelcomePanel({ onAddFoundryData }: { onAddFoundryData: () => void }) {
  return (
    <div
      style={{
        position: 'absolute',
        top: 24,
        left: '50%',
        transform: 'translateX(-50%)',
        width: 'min(440px, calc(100% - 48px))',
        background: '#fff',
        border: '1px solid var(--border-default)',
        borderRadius: 6,
        boxShadow: '0 12px 24px rgba(15, 23, 42, 0.08)',
        zIndex: 5,
      }}
    >
      <div style={{ padding: '14px 16px', borderBottom: '1px solid var(--border-subtle)' }}>
        <p style={{ margin: 0, fontSize: 15, fontWeight: 600, color: 'var(--text-strong)' }}>
          Welcome to Pipeline Builder
        </p>
        <p style={{ margin: '4px 0 0', fontSize: 12.5, color: 'var(--text-muted)' }}>
          Get started by adding datasets, then define transform logic to derive target outputs.
        </p>
        <button
          type="button"
          className="of-button"
          style={{ marginTop: 10, fontSize: 12 }}
        >
          <Glyph name="info" size={12} /> Take a tour
        </button>
      </div>
      <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
        <PipelineWelcomeItem
          icon="database"
          iconTone="#2d72d2"
          label="Add Foundry data"
          description="Recommended if you have already ingested data into OpenFoundry."
          enabled
          onClick={onAddFoundryData}
        />
        <PipelineWelcomeItem
          icon="database"
          iconTone="#5c7080"
          label="Add data to OpenFoundry"
          description="Import data from outside OpenFoundry and start using it now."
          enabled={false}
        />
        <PipelineWelcomeItem
          icon="database"
          iconTone="#5c7080"
          label="Upload from your computer"
          description="Recommended if you have sample data available locally."
          enabled={false}
        />
        <PipelineWelcomeItem
          icon="document"
          iconTone="#5c7080"
          label="Manually enter data"
          description="Recommended if you do not have data available to import."
          enabled={false}
        />
      </ul>
    </div>
  );
}

function PipelineAIPGeneratePanel({
  prompt,
  busy,
  selectedNodeLabel,
  onPromptChange,
  onClose,
  onGenerate,
}: {
  prompt: string;
  busy: boolean;
  selectedNodeLabel: string;
  onPromptChange: (value: string) => void;
  onClose: () => void;
  onGenerate: () => void;
}) {
  const canGenerate = prompt.trim().length > 0 && !busy;
  return (
    <div
      className="of-panel"
      style={{
        position: 'absolute',
        top: 16,
        right: 16,
        width: 'min(420px, calc(100% - 32px))',
        padding: 12,
        zIndex: 9,
        boxShadow: '0 16px 32px rgba(15, 23, 42, 0.12)',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 10 }}>
        <div style={{ minWidth: 0 }}>
          <p className="of-eyebrow">AIP</p>
          <p style={{ margin: '3px 0 0', fontSize: 14, fontWeight: 700 }}>Generate transform</p>
          <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 12 }}>
            Target: {selectedNodeLabel}
          </p>
        </div>
        <button type="button" className="of-button" onClick={onClose} aria-label="Close AIP generator" style={{ width: 30, padding: 0 }}>
          <Glyph name="x" size={13} />
        </button>
      </div>
      <textarea
        className="of-input"
        value={prompt}
        onChange={(event) => onPromptChange(event.target.value)}
        rows={6}
        placeholder="Filter trail runs over 5 miles and add an LLM summary column."
        style={{ marginTop: 10, minHeight: 116, resize: 'vertical' }}
      />
      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 10 }}>
        <button type="button" className="of-button" onClick={onClose} disabled={busy}>
          Cancel
        </button>
        <button type="button" className="of-button of-button--primary" onClick={onGenerate} disabled={!canGenerate}>
          {busy ? 'Generating...' : 'Generate'}
        </button>
      </div>
    </div>
  );
}

function PipelineWelcomeItem({
  icon,
  iconTone,
  label,
  description,
  enabled,
  onClick,
}: {
  icon: 'database' | 'document';
  iconTone: string;
  label: string;
  description: string;
  enabled: boolean;
  onClick?: () => void;
}) {
  return (
    <li>
      <button
        type="button"
        onClick={enabled ? onClick : undefined}
        disabled={!enabled}
        style={{
          width: '100%',
          display: 'flex',
          alignItems: 'flex-start',
          gap: 12,
          padding: '12px 16px',
          border: 0,
          background: 'transparent',
          borderTop: '1px solid var(--border-subtle)',
          cursor: enabled ? 'pointer' : 'not-allowed',
          opacity: enabled ? 1 : 0.55,
          textAlign: 'left',
        }}
        onMouseEnter={(event) => {
          if (enabled) event.currentTarget.style.background = 'rgba(45, 114, 210, 0.04)';
        }}
        onMouseLeave={(event) => (event.currentTarget.style.background = 'transparent')}
      >
        <Glyph name={icon} size={16} tone={iconTone} />
        <span style={{ display: 'grid', gap: 2, minWidth: 0 }}>
          <span style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{label}</span>
          <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>{description}</span>
        </span>
      </button>
    </li>
  );
}
