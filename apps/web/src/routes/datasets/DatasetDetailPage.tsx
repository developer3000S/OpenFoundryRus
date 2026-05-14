import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';

import { MetadataPanel } from '@/lib/components/dataset/MetadataPanel';
import { QualityDashboard } from '@/lib/components/dataset/QualityDashboard';
import { RetentionPoliciesTab } from '@/lib/components/dataset/RetentionPoliciesTab';
import { VirtualizedPreviewTable } from '@/lib/components/dataset/VirtualizedPreviewTable';
import { ConfirmDialog } from '@/lib/components/workspace/ConfirmDialog';
import { Tabs } from '@/lib/components/Tabs';
import { ApiError } from '@/lib/api/client';
import {
  deleteDataset,
  datasetFileDownloadUrl,
  exportDataset,
  getDataset,
  getDatasetHealth,
  getDatasetIncrementalReadiness,
  getDatasetLint,
  getDatasetQuality,
  getDatasetSchemaForBranch,
  getVersions,
  hardDeleteDataset,
  inferDatasetSchema,
  listBranches,
  listDatasetFiles,
  listDatasetTransactions,
  previewDataset,
  putDatasetSchemaForBranch,
  refreshDatasetQualityProfile,
  startDatasetBuild,
  updateDataset,
  type Dataset,
  type DatasetBackingFile,
  type DatasetBranch,
  type DatasetCsvOptions,
  type DatasetExportParams,
  type DatasetExportResponse,
  type DatasetField,
  type DatasetFileFormat,
  type DatasetHealthResponse,
  type DatasetIncrementalReadiness,
  type DatasetSchemaInferenceResponse,
  type DatasetSchemaPayload,
  type DatasetLintResponse,
  type DatasetPreviewResponse,
  type DatasetQualityResponse,
  type DatasetSchema,
  type DatasetSchemaResponse,
  type DatasetTransaction,
  type DatasetVersion,
  type IncrementalTransactionBoundary,
} from '@/lib/api/datasets';
import { BUILD_STATE_COLORS, listDatasetBuildsV1, type Build } from '@/lib/api/buildsV1';
import { listSchedules, type Schedule } from '@/lib/api/schedules';

type Tab = 'preview' | 'files' | 'details' | 'schema' | 'history' | 'jobs' | 'schedules' | 'health' | 'lineage' | 'retention';
type BusyAction = 'save' | 'delete' | 'hard-delete' | 'build' | 'profile' | 'export' | 'quality-rule' | null;
type Notice = { type: 'success' | 'info' | 'error'; text: string };

const DATASET_TABS = [
  { id: 'preview', label: 'Preview' },
  { id: 'files', label: 'Files' },
  { id: 'details', label: 'Details' },
  { id: 'schema', label: 'Schema' },
  { id: 'history', label: 'History' },
  { id: 'jobs', label: 'Jobs' },
  { id: 'schedules', label: 'Schedules' },
  { id: 'health', label: 'Health' },
  { id: 'lineage', label: 'Lineage' },
  { id: 'retention', label: 'Retention' },
] satisfies ReadonlyArray<{ id: Tab; label: string }>;

const TAB_IDS = new Set<Tab>(DATASET_TABS.map((t) => t.id));

function normalizeTab(value: string | null): Tab {
  if (value === 'transactions' || value === 'versions') return 'history';
  if (value === 'metadata') return 'details';
  if (value === 'quality') return 'health';
  return value && TAB_IDS.has(value as Tab) ? (value as Tab) : 'preview';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function formatBytes(bytes?: number | null) {
  if (bytes === undefined || bytes === null) return 'n/a';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatDate(value?: string | null) {
  if (!value) return 'n/a';
  return new Date(value).toLocaleString();
}

function shortId(value?: string | null, length = 12) {
  if (!value) return 'n/a';
  return value.length > length ? `${value.slice(0, length)}...` : value;
}

function shortHash(value?: string | null) {
  if (!value) return 'n/a';
  return value.length > 12 ? `${value.slice(0, 12)}...` : value;
}

function fileStorageURI(file: DatasetBackingFile) {
  const storage = file.storage_location;
  const uri = storage && typeof storage.uri === 'string' ? storage.uri : undefined;
  return uri || file.physical_uri || 'n/a';
}

function actionReference(payload: { id?: string; rid?: string; build_id?: string; export_id?: string; status?: string; state?: string; message?: string }) {
  const ref = payload.rid ?? payload.id ?? payload.build_id ?? payload.export_id;
  const status = payload.status ?? payload.state;
  const pieces = [ref ? `ref ${ref}` : '', status ? `status ${status}` : '', payload.message ?? ''].filter(Boolean);
  return pieces.length > 0 ? ` (${pieces.join(', ')})` : '';
}

function userFacingError(cause: unknown, fallback: string) {
  if (cause instanceof ApiError) {
    if (cause.status === 401) return 'Sign in again to view this dataset surface.';
    if (cause.status === 403) return 'You do not have permission to view this dataset surface.';
    if (cause.status === 404) return fallback;
  }
  return cause instanceof Error ? cause.message : fallback;
}

function parseVersionParam(value: string | null) {
  if (!value) return null;
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
}

export function DatasetDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [tab, setTab] = useState<Tab>(() => normalizeTab(searchParams.get('tab')));

  const [dataset, setDataset] = useState<Dataset | null>(null);
  const [preview, setPreview] = useState<DatasetPreviewResponse | null>(null);
  const [schema, setSchema] = useState<DatasetSchema | DatasetSchemaResponse | null>(null);
  const [files, setFiles] = useState<DatasetBackingFile[]>([]);
  const [branches, setBranches] = useState<DatasetBranch[]>([]);
  const [transactions, setTransactions] = useState<DatasetTransaction[]>([]);
  const [incrementalReadiness, setIncrementalReadiness] = useState<DatasetIncrementalReadiness | null>(null);
  const [versions, setVersions] = useState<DatasetVersion[]>([]);
  const [builds, setBuilds] = useState<Build[]>([]);
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [quality, setQuality] = useState<DatasetQualityResponse | null>(null);
  const [healthSnapshot, setHealthSnapshot] = useState<DatasetHealthResponse | null>(null);
  const [lint, setLint] = useState<DatasetLintResponse | null>(null);
  const [loadedTabs, setLoadedTabs] = useState<Partial<Record<Tab, boolean>>>({});
  const [tabLoading, setTabLoading] = useState<Partial<Record<Tab, boolean>>>({});
  const [tabErrors, setTabErrors] = useState<Partial<Record<Tab, string>>>({});
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState<BusyAction>(null);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState<Notice | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [hardDeleteOpen, setHardDeleteOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [exportError, setExportError] = useState('');
  const [exportResult, setExportResult] = useState<DatasetExportResponse | null>(null);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [tagsText, setTagsText] = useState('');
  const [folderPath, setFolderPath] = useState('/datasets');
  const [projectId, setProjectId] = useState('default');
  const [visibility, setVisibility] = useState('private');
  const [selectedBranchName, setSelectedBranchName] = useState('');
  const [previewColumnText, setPreviewColumnText] = useState('');
  const [previewFilter, setPreviewFilter] = useState('');
  const [previewSort, setPreviewSort] = useState('');
  const [previewLimit, setPreviewLimit] = useState(100);
  const [previewOffset, setPreviewOffset] = useState(0);
  const [previewSample, setPreviewSample] = useState(false);
  const [previewSampleSeed, setPreviewSampleSeed] = useState(1);

  const selectedTransactionId = searchParams.get('txn');
  const selectedVersion = parseVersionParam(searchParams.get('version'));
  const busy = busyAction !== null;

  useEffect(() => {
    const next = normalizeTab(searchParams.get('tab'));
    if (next !== tab) setTab(next);
  }, [searchParams, tab]);

  useEffect(() => {
    let cancelled = false;

    async function loadDataset() {
      if (!id) return;
      setLoading(true);
      setError('');
      setNotice(null);
      setPreview(null);
      setSchema(null);
      setFiles([]);
      setBranches([]);
      setTransactions([]);
      setIncrementalReadiness(null);
      setVersions([]);
      setBuilds([]);
      setSchedules([]);
      setQuality(null);
      setHealthSnapshot(null);
      setLint(null);
      setLoadedTabs({});
      setTabLoading({});
      setTabErrors({});
      try {
        const next = await getDataset(id);
        if (cancelled) return;
        setDataset(next);
        setName(next.name);
        setDescription(next.description);
        setTagsText(next.tags.join(', '));
        setFolderPath(next.folder_path || '/datasets');
        setProjectId(next.project_id || 'default');
        setVisibility(next.resource_visibility || 'private');
        const branchParam = searchParams.get('branch');
        setSelectedBranchName(branchParam || next.active_branch || 'main');
        listBranches(id)
          .then((rows) => {
            if (cancelled) return;
            setBranches(rows);
            if (!branchParam && rows.length > 0 && !rows.some((branch) => branch.name === next.active_branch)) {
              setSelectedBranchName(rows.find((branch) => branch.is_default)?.name ?? rows[0].name);
            }
          })
          .catch((cause) => {
            if (!cancelled) setTabErrors((prev) => ({ ...prev, details: userFacingError(cause, 'Branches are not available for this dataset.') }));
          });
        getVersions(id)
          .then((rows) => {
            if (!cancelled) setVersions(rows);
          })
          .catch(() => undefined);
      } catch (cause) {
        if (!cancelled) {
          setDataset(null);
          setError(userFacingError(cause, 'Dataset was not found or is not visible to you.'));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    void loadDataset();
    return () => { cancelled = true; };
  }, [id]);

  const previewRows = preview?.rows ?? [];
  const previewColumns = useMemo(() => {
    if (preview?.columns && preview.columns.length > 0) return preview.columns;
    if (previewRows.length === 0) return [];
    return Object.keys(previewRows[0]).map((columnName) => ({ name: columnName }));
  }, [preview?.columns, previewRows]);
  const activeBranch = selectedBranchName || dataset?.active_branch || 'main';
  const branchMissing = branches.length > 0 && !branches.some((branch) => branch.name === activeBranch);
  const branchTransactions = useMemo(() => {
    return transactions.filter((transaction) => !transaction.branch_name || transaction.branch_name === activeBranch);
  }, [activeBranch, transactions]);
  const selectedTransactionMissing = Boolean(
    selectedTransactionId && loadedTabs.history && !transactions.some((transaction) => transaction.id === selectedTransactionId),
  );
  const latestView = Boolean(dataset && !selectedVersion && !selectedTransactionId && activeBranch === dataset.active_branch);
  const currentTabLoaded = Boolean(loadedTabs[tab]);

  useEffect(() => {
    if (!dataset) return;
    const nextBranch = searchParams.get('branch') || dataset.active_branch || 'main';
    if (nextBranch === selectedBranchName) return;
    setSelectedBranchName(nextBranch);
    setPreview(null);
    setSchema(null);
    setFiles([]);
    setLoadedTabs((prev) => ({ ...prev, preview: false, files: false, schema: false, history: false }));
  }, [dataset, searchParams, selectedBranchName]);

  useEffect(() => {
    setLoadedTabs((prev) => (prev.preview ? { ...prev, preview: false } : prev));
    setPreview(null);
  }, [selectedVersion, selectedTransactionId]);

  useEffect(() => {
    if (!dataset) return;
    void ensureTabData(tab);
    // The data loader intentionally reads the latest tab caches from state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dataset, tab, activeBranch, selectedVersion, selectedTransactionId, previewOffset, currentTabLoaded]);

  async function ensureTabData(next: Tab, options: { force?: boolean } = {}) {
    if (!id) return;
    if (!options.force && loadedTabs[next]) return;

    setTabLoading((prev) => ({ ...prev, [next]: true }));
    setTabErrors((prev) => {
      const copy = { ...prev };
      delete copy[next];
      return copy;
    });
    try {
      if (branchMissing && ['preview', 'files', 'schema', 'history'].includes(next)) {
        setTabErrors((prev) => ({ ...prev, [next]: `Branch "${activeBranch}" was not found on this dataset.` }));
        setLoadedTabs((prev) => ({ ...prev, [next]: true }));
        return;
      }
      if (next === 'preview') {
        const selectedColumns = previewColumnText.split(',').map((column) => column.trim()).filter(Boolean);
        const selectedSort = previewSort.split(',').map((column) => column.trim()).filter(Boolean);
        const [previewResponse, txResponse] = await Promise.all([
          previewDataset(id, {
            limit: previewLimit,
            offset: previewOffset,
            branch: activeBranch,
            version: selectedVersion ?? undefined,
            transaction_id: selectedTransactionId,
            columns: selectedColumns,
            filter: previewFilter.trim() || undefined,
            sort: selectedSort,
            sample: previewSample,
            sample_size: previewSample ? previewLimit : undefined,
            sample_seed: previewSampleSeed,
          }),
          loadedTabs.history ? Promise.resolve<DatasetTransaction[] | null>(null) : listDatasetTransactions(id, { branch: activeBranch }).catch(() => null),
        ]);
        setPreview(previewResponse);
        setLoadedTabs((prev) => ({ ...prev, preview: true }));
        if (txResponse) {
          setTransactions(txResponse);
          setLoadedTabs((prev) => ({ ...prev, history: true }));
        }
      } else if (next === 'files') {
        const response = await listDatasetFiles(id, { branch: activeBranch });
        setFiles(response.files ?? response.data ?? []);
        setLoadedTabs((prev) => ({ ...prev, files: true }));
      } else if (next === 'schema') {
        if (!loadedTabs.files) {
          listDatasetFiles(id, { branch: activeBranch })
            .then((response) => {
              setFiles(response.files ?? response.data ?? []);
              setLoadedTabs((prev) => ({ ...prev, files: true }));
            })
            .catch(() => undefined);
        }
        try {
          setSchema(await getDatasetSchemaForBranch(id, activeBranch));
        } catch (cause) {
          setSchema(null);
          setTabErrors((prev) => ({ ...prev, schema: userFacingError(cause, 'No schema is available for this branch.') }));
        }
        setLoadedTabs((prev) => ({ ...prev, schema: true }));
      } else if (next === 'history') {
        const [txRows, versionRows, readiness] = await Promise.all([
          listDatasetTransactions(id, { branch: activeBranch }),
          getVersions(id),
          getDatasetIncrementalReadiness(id, { branch: activeBranch }),
        ]);
        setTransactions(txRows);
        setVersions(versionRows);
        setIncrementalReadiness(readiness);
        setLoadedTabs((prev) => ({ ...prev, history: true }));
      } else if (next === 'jobs') {
        const rid = dataset?.rid || id;
        const response = await listDatasetBuildsV1(rid);
        setBuilds(response.data ?? []);
        setLoadedTabs((prev) => ({ ...prev, jobs: true }));
      } else if (next === 'schedules') {
        const rid = dataset?.rid || id;
        const response = await listSchedules({ files: [rid], limit: 100, sort: 'updated_at' });
        setSchedules(response.data ?? []);
        setLoadedTabs((prev) => ({ ...prev, schedules: true }));
      } else if (next === 'health') {
        const rid = dataset?.rid || id;
        const [qualityResult, healthResult, lintResult] = await Promise.allSettled([
          getDatasetQuality(id),
          getDatasetHealth(rid),
          getDatasetLint(id),
        ]);
        const messages: string[] = [];
        if (qualityResult.status === 'fulfilled') setQuality(qualityResult.value);
        else messages.push(userFacingError(qualityResult.reason, 'Quality profile is not available.'));
        if (healthResult.status === 'fulfilled') setHealthSnapshot(healthResult.value);
        else if (!(healthResult.reason instanceof ApiError && healthResult.reason.status === 404)) {
          messages.push(userFacingError(healthResult.reason, 'Health snapshot is not available.'));
        }
        if (lintResult.status === 'fulfilled') setLint(lintResult.value);
        else messages.push(userFacingError(lintResult.reason, 'Lint summary is not available.'));
        if (messages.length > 0) setTabErrors((prev) => ({ ...prev, health: messages.join(' ') }));
        setLoadedTabs((prev) => ({ ...prev, health: true }));
      } else {
        setLoadedTabs((prev) => ({ ...prev, [next]: true }));
      }
    } catch (cause) {
      setTabErrors((prev) => ({ ...prev, [next]: userFacingError(cause, 'Failed to load tab data.') }));
      setLoadedTabs((prev) => ({ ...prev, [next]: true }));
    } finally {
      setTabLoading((prev) => ({ ...prev, [next]: false }));
    }
  }

  function setActiveTab(next: Tab) {
    setTab(next);
    const params = new URLSearchParams(searchParams);
    if (next === 'preview') params.delete('tab');
    else params.set('tab', next);
    setSearchParams(params);
    void ensureTabData(next);
  }

  function selectPreviewTransaction(txId: string | null) {
    const params = new URLSearchParams(searchParams);
    if (txId) params.set('txn', txId);
    else params.delete('txn');
    setSearchParams(params);
  }

  function applyPreviewControls() {
    setPreview(null);
    setLoadedTabs((prev) => ({ ...prev, preview: false }));
    void ensureTabData('preview', { force: true });
  }

  function pagePreview(direction: 1 | -1) {
    const nextOffset = Math.max(0, previewOffset + direction * previewLimit);
    setPreviewOffset(nextOffset);
    setPreview(null);
    setLoadedTabs((prev) => ({ ...prev, preview: false }));
  }

  function selectBranch(branchName: string) {
    setSelectedBranchName(branchName);
    setPreview(null);
    setSchema(null);
    setFiles([]);
    const params = new URLSearchParams(searchParams);
    if (branchName && branchName !== dataset?.active_branch) params.set('branch', branchName);
    else params.delete('branch');
    params.delete('txn');
    setSearchParams(params);
    setLoadedTabs((prev) => ({ ...prev, preview: false, files: false, schema: false, history: false }));
  }

  function selectVersion(version: number | null) {
    setPreview(null);
    const params = new URLSearchParams(searchParams);
    if (version) params.set('version', String(version));
    else params.delete('version');
    params.delete('txn');
    setSearchParams(params);
    setLoadedTabs((prev) => ({ ...prev, preview: false }));
  }

  async function copyText(text: string, label: string) {
    await navigator.clipboard?.writeText(text);
    setNotice({ type: 'success', text: `${label} copied.` });
  }

  async function saveMetadata() {
    if (!dataset) return;
    setBusyAction('save');
    setError('');
    setNotice(null);
    try {
      const updated = await updateDataset(dataset.id, {
        name: name.trim(),
        description,
        tags: tagsText.split(',').map((t) => t.trim()).filter(Boolean),
        folder_path: folderPath.trim() || '/datasets',
        project_id: projectId.trim() || 'default',
        resource_visibility: visibility,
      });
      setDataset(updated);
      setName(updated.name);
      setDescription(updated.description);
      setTagsText(updated.tags.join(', '));
      setNotice({ type: 'success', text: 'Dataset metadata saved.' });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Save failed');
    } finally {
      setBusyAction(null);
    }
  }

  async function removeDataset() {
    if (!dataset) return;
    setBusyAction('delete');
    setError('');
    try {
      await deleteDataset(dataset.id);
      navigate('/datasets');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
      setBusyAction(null);
      setDeleteOpen(false);
    }
  }

  async function hardRemoveDataset() {
    if (!dataset) return;
    setBusyAction('hard-delete');
    setError('');
    try {
      await hardDeleteDataset(dataset.id);
      navigate('/datasets');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Hard delete failed');
      setBusyAction(null);
      setHardDeleteOpen(false);
    }
  }

  async function runBuild() {
    if (!dataset) return;
    setBusyAction('build');
    setError('');
    setNotice(null);
    try {
      const response = await startDatasetBuild(dataset.id, {
        branch: dataset.active_branch,
        reason: 'manual dataset detail action',
      });
      setNotice({ type: 'success', text: `Build started${actionReference(response)}.` });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Build failed');
    } finally {
      setBusyAction(null);
    }
  }

  async function refreshQuality() {
    if (!dataset) return;
    setBusyAction('profile');
    setError('');
    setNotice(null);
    try {
      const response = await refreshDatasetQualityProfile(dataset.id);
      setQuality(response);
      setLoadedTabs((prev) => ({ ...prev, health: true }));
      setActiveTab('health');
      setNotice({ type: 'success', text: 'Quality profile refreshed.' });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Quality refresh failed');
    } finally {
      setBusyAction(null);
    }
  }

  async function submitExport(params: DatasetExportParams) {
    if (!dataset) return;
    setBusyAction('export');
    setExportError('');
    setExportResult(null);
    try {
      const response = await exportDataset(dataset.id, params);
      setExportResult(response);
    } catch (cause) {
      setExportError(cause instanceof Error ? cause.message : 'Export failed');
    } finally {
      setBusyAction(null);
    }
  }

  function openExportDialog() {
    setExportOpen(true);
    setExportError('');
    setExportResult(null);
  }

  function explorePipeline() {
    if (!dataset) return;
    navigate(`/lineage?dataset=${encodeURIComponent(dataset.id)}`);
  }

  async function copyDatasetLink() {
    if (!dataset) return;
    const href = `${window.location.origin}/datasets/${encodeURIComponent(dataset.id)}`;
    await copyText(href, 'Dataset link');
  }

  async function copyPreviewApi() {
    if (!dataset) return;
    const query = new URLSearchParams();
    if (activeBranch) query.set('branch', activeBranch);
    if (selectedVersion) query.set('version', String(selectedVersion));
    await copyText(`/api/v1/datasets/${encodeURIComponent(dataset.id)}/preview?${query.toString()}`, 'Preview API path');
  }

  async function refreshSchemaAfterApply(message: string) {
    const next = await getDatasetSchemaForBranch(id, activeBranch);
    setSchema(next);
    setLoadedTabs((prev) => ({ ...prev, schema: true }));
    setNotice({ type: 'success', text: message });
  }

  function openSqlPreview() {
    if (!dataset) return;
    navigate(`/queries?dataset=${encodeURIComponent(dataset.id)}`);
  }

  function openContour() {
    if (!dataset) return;
    navigate(`/contour?dataset=${encodeURIComponent(dataset.id)}`);
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <p className="of-text-muted">Loading...</p>
      </section>
    );
  }

  if (!dataset) {
    return (
      <section className="of-page" style={{ padding: 24 }}>
        <Link to="/datasets" style={{ color: 'var(--text-muted)', fontSize: 13 }}>Back to datasets</Link>
        <p className="of-status-danger" style={{ marginTop: 12, padding: 10, borderRadius: 'var(--radius-md)' }}>{error || 'Not found'}</p>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 10 }}>
      <header className="of-panel dataset-detail-header" style={{ padding: 12, display: 'grid', gap: 10 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12, flexWrap: 'wrap' }}>
          <div style={{ minWidth: 0 }}>
            <Link to="/datasets" style={{ color: 'var(--text-muted)', fontSize: 12 }}>Datasets</Link>
            <h1 className="of-heading-lg" style={{ marginTop: 4 }}>{dataset.name}</h1>
            <p className="of-text-muted" style={{ marginTop: 2, fontSize: 11, fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>
              {dataset.rid || dataset.id}
            </p>
            <p className="of-text-muted" style={{ marginTop: 2, fontSize: 12, overflowWrap: 'anywhere' }}>
              {dataset.path || dataset.storage_path}
            </p>
          </div>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
            <button type="button" onClick={() => void runBuild()} disabled={busy} className="of-button of-button--primary">
              {busyAction === 'build' ? 'Starting...' : 'Build'}
            </button>
            <button type="button" onClick={() => void refreshQuality()} disabled={busy} className="of-button">
              {busyAction === 'profile' ? 'Profiling...' : 'Profile data'}
            </button>
            <button type="button" onClick={openExportDialog} disabled={busy} className="of-button">
              Export
            </button>
            <button type="button" onClick={explorePipeline} className="of-button">
              Explore pipeline
            </button>
            <button type="button" onClick={() => void copyDatasetLink()} className="of-button">
              Copy link
            </button>
            <button type="button" onClick={() => void copyPreviewApi()} className="of-button">
              Copy API
            </button>
            <Link to={`/datasets/${dataset.id}/branches`} className="of-button">Branches</Link>
            <button type="button" onClick={() => setDeleteOpen(true)} disabled={busy} className="of-button" style={{ color: '#b42318', borderColor: '#e5b8b8' }}>
              Soft-delete
            </button>
            <button type="button" onClick={() => setHardDeleteOpen(true)} disabled={busy} className="of-button" style={{ color: '#8a1f11', borderColor: '#e5b8b8' }}>
              Hard-delete
            </button>
          </div>
        </div>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          <span className="of-chip">{dataset.format}</span>
          <span className="of-chip">{dataset.row_count.toLocaleString()} rows</span>
          <span className="of-chip">{formatBytes(dataset.size_bytes)}</span>
          <span className="of-chip of-chip-active">{dataset.active_branch}</span>
          <span className="of-chip">{dataset.resource_visibility || 'private'}</span>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 8, alignItems: 'end' }}>
          <label style={{ display: 'grid', gap: 4, fontSize: 11 }}>
            Branch
            <select value={activeBranch} onChange={(event) => selectBranch(event.target.value)} className="of-input" style={{ fontSize: 12 }}>
              {branches.length > 0 ? branches.map((branch) => (
                <option key={branch.name} value={branch.name}>{branch.name}{branch.is_default ? ' (default)' : ''}</option>
              )) : (
                <option value={activeBranch}>{activeBranch}</option>
              )}
            </select>
          </label>
          <label style={{ display: 'grid', gap: 4, fontSize: 11 }}>
            Version
            <select value={selectedVersion ?? ''} onChange={(event) => selectVersion(event.target.value ? Number(event.target.value) : null)} className="of-input" style={{ fontSize: 12 }}>
              <option value="">Latest on branch</option>
              {versions.map((version) => (
                <option key={version.id} value={version.version}>v{version.version}</option>
              ))}
              {versions.length === 0 && <option value={dataset.current_version}>v{dataset.current_version}</option>}
            </select>
          </label>
          <label style={{ display: 'grid', gap: 4, fontSize: 11 }}>
            Transaction
            <select value={selectedTransactionId ?? ''} onChange={(event) => selectPreviewTransaction(event.target.value || null)} className="of-input" style={{ fontSize: 12 }}>
              <option value="">Latest transaction</option>
              {branchTransactions.map((transaction) => (
                <option key={transaction.id} value={transaction.id}>
                  {shortId(transaction.id, 10)} - {transaction.status}
                </option>
              ))}
            </select>
          </label>
          <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
            <span className={latestView ? 'of-chip of-chip-active' : 'of-chip'}>
              {latestView ? 'Latest view' : 'Historical view'}
            </span>
            {branchMissing && <span className="of-chip" style={{ color: '#b42318', borderColor: '#e5b8b8' }}>Missing branch</span>}
            {selectedTransactionMissing && <span className="of-chip" style={{ color: '#b42318', borderColor: '#e5b8b8' }}>Missing transaction</span>}
          </div>
        </div>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {error}
        </div>
      )}

      {notice && (
        <div className={notice.type === 'error' ? 'of-status-danger' : notice.type === 'success' ? 'of-status-success' : 'of-status-info'} style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {notice.text}
        </div>
      )}

      <div className="dataset-detail-workspace">
        <MetadataPanel
          dataset={dataset}
          quality={quality}
          fileCount={loadedTabs.files ? files.length : undefined}
          transactionCount={loadedTabs.history ? branchTransactions.length : undefined}
        />

        <section className="of-panel" style={{ minWidth: 0, overflow: 'hidden' }}>
          <Tabs tabs={DATASET_TABS} active={tab} onChange={setActiveTab} />
          {renderTabContent()}
        </section>
      </div>

      <ExportDialog
        dataset={dataset}
        open={exportOpen}
        busy={busyAction === 'export'}
        error={exportError}
        result={exportResult}
        onClose={() => setExportOpen(false)}
        onSubmit={submitExport}
      />

      <ConfirmDialog
        open={deleteOpen}
        title="Delete dataset"
        message={`Delete ${dataset.name}? This removes the dataset from the catalog.`}
        confirmLabel="Delete"
        danger
        busy={busyAction === 'delete'}
        onCancel={() => setDeleteOpen(false)}
        onConfirm={() => void removeDataset()}
      />

      <ConfirmDialog
        open={hardDeleteOpen}
        title="Hard-delete dataset"
        message={`Permanently delete ${dataset.name}? This removes catalog metadata and cannot be restored.`}
        confirmLabel="Hard-delete"
        danger
        busy={busyAction === 'hard-delete'}
        onCancel={() => setHardDeleteOpen(false)}
        onConfirm={() => void hardRemoveDataset()}
      />
    </section>
  );

  function renderTabContent() {
    if (!dataset) return null;
    const currentDataset = dataset;
    const tabError = tabErrors[tab];
    if (tab === 'preview') {
      return (
        <div>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, justifyContent: 'space-between', padding: 8, borderBottom: '1px solid var(--border-default)', background: 'var(--bg-topbar)' }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              <button type="button" onClick={openSqlPreview} className="of-button" style={{ fontSize: 11 }}>SQL preview</button>
              <button type="button" onClick={openContour} className="of-button" style={{ fontSize: 11 }}>Analyze data</button>
              <button type="button" onClick={explorePipeline} className="of-button" style={{ fontSize: 11 }}>Explore pipeline</button>
            </div>
            {tabLoading.preview && <span className="of-text-muted" style={{ alignSelf: 'center', fontSize: 11 }}>Refreshing preview...</span>}
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 150px), 1fr))', gap: 6, padding: 8, borderBottom: '1px solid var(--border-default)', background: 'var(--bg-panel-muted)', alignItems: 'end' }}>
            <label style={{ display: 'grid', gap: 3, fontSize: 11 }}>
              Columns
              <input value={previewColumnText} onChange={(event) => setPreviewColumnText(event.target.value)} placeholder="id, amount" className="of-input" style={{ fontSize: 12 }} />
            </label>
            <label style={{ display: 'grid', gap: 3, fontSize: 11 }}>
              Filter
              <input value={previewFilter} onChange={(event) => setPreviewFilter(event.target.value)} placeholder="status=active" className="of-input" style={{ fontSize: 12 }} />
            </label>
            <label style={{ display: 'grid', gap: 3, fontSize: 11 }}>
              Sort
              <input value={previewSort} onChange={(event) => setPreviewSort(event.target.value)} placeholder="-updated_at" className="of-input" style={{ fontSize: 12 }} />
            </label>
            <label style={{ display: 'grid', gap: 3, fontSize: 11 }}>
              Limit
              <input type="number" min={1} max={1000} value={previewLimit} onChange={(event) => setPreviewLimit(Number(event.target.value) || 100)} className="of-input" style={{ fontSize: 12 }} />
            </label>
            <label style={{ display: 'grid', gap: 3, fontSize: 11 }}>
              Offset
              <input type="number" min={0} value={previewOffset} onChange={(event) => setPreviewOffset(Math.max(0, Number(event.target.value) || 0))} className="of-input" style={{ fontSize: 12 }} />
            </label>
            <label style={{ display: 'inline-flex', gap: 5, alignItems: 'center', fontSize: 12, minHeight: 30 }}>
              <input type="checkbox" checked={previewSample} onChange={(event) => setPreviewSample(event.target.checked)} />
              Sample
            </label>
            <label style={{ display: 'grid', gap: 3, fontSize: 11 }}>
              Seed
              <input type="number" value={previewSampleSeed} onChange={(event) => setPreviewSampleSeed(Number(event.target.value) || 1)} className="of-input" style={{ fontSize: 12 }} />
            </label>
            <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap' }}>
              <button type="button" onClick={applyPreviewControls} className="of-button of-button--primary" style={{ fontSize: 11 }}>Apply</button>
              <button type="button" onClick={() => pagePreview(-1)} disabled={previewOffset === 0} className="of-button" style={{ fontSize: 11 }}>Prev</button>
              <button type="button" onClick={() => pagePreview(1)} className="of-button" style={{ fontSize: 11 }}>Next</button>
            </div>
          </div>
          {tabError ? (
            <TabBody><PermissionState message={tabError} /></TabBody>
          ) : selectedTransactionMissing ? (
            <TabBody><PermissionState message={`Transaction "${selectedTransactionId}" was not found or is not visible on this dataset.`} /></TabBody>
          ) : tabLoading.preview && !preview ? (
            <LoadingBlock label="Loading preview..." />
          ) : preview ? (
            <>
              <PreviewMessages preview={preview} />
              <VirtualizedPreviewTable
                columns={previewColumns}
                rows={previewRows}
                transactions={transactions}
                selectedTransactionId={selectedTransactionId}
                onSelectTransaction={selectPreviewTransaction}
                fileFormat={preview.format ?? null}
                schemaInferred={Boolean((preview as DatasetPreviewResponse & { schema_inferred?: boolean }).schema_inferred)}
                viewportHeight={560}
              />
            </>
          ) : (
            <EmptyBlock label="No preview data is available yet." />
          )}
        </div>
      );
    }

    if (tab === 'files') {
      return (
        <TabBody>
          {tabError ? <PermissionState message={tabError} /> : tabLoading.files && files.length === 0 ? <LoadingBlock label="Loading files..." /> : <FilesTable datasetId={id} files={files} />}
        </TabBody>
      );
    }

    if (tab === 'details') {
      return (
        <TabBody>
          <DetailsTab
            dataset={currentDataset}
            name={name}
            description={description}
            tagsText={tagsText}
            folderPath={folderPath}
            projectId={projectId}
            visibility={visibility}
            busy={busy}
            busyAction={busyAction}
            activeBranch={activeBranch}
            selectedVersion={selectedVersion}
            onName={setName}
            onDescription={setDescription}
            onTagsText={setTagsText}
            onFolderPath={setFolderPath}
            onProjectId={setProjectId}
            onVisibility={setVisibility}
            onSave={() => void saveMetadata()}
            onCopy={(text, label) => void copyText(text, label)}
          />
        </TabBody>
      );
    }

    if (tab === 'schema') {
      return (
        <TabBody>
          {tabError && !schema ? (
            <PermissionState message={tabError} />
          ) : tabLoading.schema && !schema ? (
            <LoadingBlock label="Loading schema..." />
          ) : (
            <SchemaEditor
              datasetId={id}
              branch={activeBranch}
              schema={schema}
              files={files}
              filesLoading={tabLoading.files}
              onNeedFiles={() => ensureTabData('files')}
              onApplied={refreshSchemaAfterApply}
            />
          )}
        </TabBody>
      );
    }

    if (tab === 'history') {
      return (
        <TabBody>
          {tabError ? <PermissionState message={tabError} /> : tabLoading.history && transactions.length === 0 && versions.length === 0 ? (
            <LoadingBlock label="Loading history..." />
          ) : (
            <HistoryPanel
              transactions={branchTransactions}
              incrementalReadiness={incrementalReadiness}
              versions={versions}
              selectedTransactionId={selectedTransactionId}
              selectedVersion={selectedVersion}
              onSelectTransaction={selectPreviewTransaction}
              onSelectVersion={selectVersion}
            />
          )}
        </TabBody>
      );
    }

    if (tab === 'jobs') {
      return (
        <TabBody>
          {tabError ? <PermissionState message={tabError} /> : tabLoading.jobs && builds.length === 0 ? <LoadingBlock label="Loading jobs..." /> : <JobsTable builds={builds} />}
        </TabBody>
      );
    }

    if (tab === 'schedules') {
      return (
        <TabBody>
          {tabError ? <PermissionState message={tabError} /> : tabLoading.schedules && schedules.length === 0 ? <LoadingBlock label="Loading schedules..." /> : <SchedulesTable schedules={schedules} datasetRid={currentDataset.rid || currentDataset.id || id} />}
        </TabBody>
      );
    }

    if (tab === 'health') {
      return (
        <TabBody>
          {tabError && <PermissionState message={tabError} />}
          <HealthTab
            datasetRid={currentDataset.rid}
            quality={quality}
            health={healthSnapshot}
            lint={lint}
            loading={Boolean(tabLoading.health)}
            refreshing={busyAction === 'profile'}
            onRefreshProfile={() => void refreshQuality()}
          />
        </TabBody>
      );
    }

    if (tab === 'lineage') {
      return (
        <TabBody>
          <LineageTab dataset={currentDataset} activeBranch={activeBranch} onOpen={explorePipeline} onCopy={(text, label) => void copyText(text, label)} />
        </TabBody>
      );
    }

    return (
      <TabBody>
        <RetentionPoliciesTab datasetRid={currentDataset.rid || currentDataset.id} projectId={currentDataset.project_id} canManage />
      </TabBody>
    );
  }
}

function TabBody({ children }: { children: React.ReactNode }) {
  return <div style={{ padding: 12, minHeight: 420 }}>{children}</div>;
}

function LoadingBlock({ label }: { label: string }) {
  return (
    <div className="of-text-muted" style={{ padding: 24, textAlign: 'center', fontSize: 13 }}>
      {label}
    </div>
  );
}

function EmptyBlock({ label }: { label: string }) {
  return (
    <div className="of-text-muted" style={{ padding: 32, textAlign: 'center', border: '1px dashed var(--border-default)', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
      {label}
    </div>
  );
}

function PermissionState({ message }: { message: string }) {
  return (
    <div className="of-status-info" style={{ padding: 12, borderRadius: 'var(--radius-md)', fontSize: 12, marginBottom: 10 }}>
      {message}
    </div>
  );
}

function DetailsTab({
  dataset,
  name,
  description,
  tagsText,
  folderPath,
  projectId,
  visibility,
  busy,
  busyAction,
  activeBranch,
  selectedVersion,
  onName,
  onDescription,
  onTagsText,
  onFolderPath,
  onProjectId,
  onVisibility,
  onSave,
  onCopy,
}: {
  dataset: Dataset;
  name: string;
  description: string;
  tagsText: string;
  folderPath: string;
  projectId: string;
  visibility: string;
  busy: boolean;
  busyAction: BusyAction;
  activeBranch: string;
  selectedVersion: number | null;
  onName: (value: string) => void;
  onDescription: (value: string) => void;
  onTagsText: (value: string) => void;
  onFolderPath: (value: string) => void;
  onProjectId: (value: string) => void;
  onVisibility: (value: string) => void;
  onSave: () => void;
  onCopy: (text: string, label: string) => void;
}) {
  const previewPath = `/api/v1/datasets/${encodeURIComponent(dataset.id)}/preview?branch=${encodeURIComponent(activeBranch)}${selectedVersion ? `&version=${selectedVersion}` : ''}`;
  return (
    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 280px), 1fr))', gap: 16, alignItems: 'start' }}>
      <div style={{ display: 'grid', gap: 10, maxWidth: 760 }}>
        <label style={{ fontSize: 12 }}>
          Name
          <input value={name} onChange={(event) => onName(event.target.value)} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Description
          <textarea value={description} onChange={(event) => onDescription(event.target.value)} rows={4} className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Tags
          <input value={tagsText} onChange={(event) => onTagsText(event.target.value)} placeholder="finance, monthly, curated" className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Folder
          <input value={folderPath} onChange={(event) => onFolderPath(event.target.value)} placeholder="/datasets" className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Project
          <input value={projectId} onChange={(event) => onProjectId(event.target.value)} placeholder="default" className="of-input" style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Visibility
          <select value={visibility} onChange={(event) => onVisibility(event.target.value)} className="of-input" style={{ marginTop: 4 }}>
            {['private', 'shared', 'organization', 'public'].map((option) => <option key={option} value={option}>{option}</option>)}
          </select>
        </label>
        <button type="button" onClick={onSave} disabled={busy} className="of-button of-button--primary" style={{ width: 'fit-content' }}>
          {busyAction === 'save' ? 'Saving...' : 'Save metadata'}
        </button>
      </div>
      <aside style={{ display: 'grid', gap: 10 }}>
        <section style={{ padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-md)', background: 'var(--bg-panel-muted)' }}>
          <p className="of-eyebrow">Resource</p>
          <dl style={{ display: 'grid', gridTemplateColumns: '88px minmax(0, 1fr)', gap: '6px 8px', margin: '8px 0 0', fontSize: 12 }}>
            <dt className="of-text-muted">RID</dt>
            <dd style={{ margin: 0, fontFamily: 'var(--font-mono)', overflowWrap: 'anywhere' }}>{dataset.rid || 'n/a'}</dd>
            <dt className="of-text-muted">Path</dt>
            <dd style={{ margin: 0, overflowWrap: 'anywhere' }}>{dataset.path || dataset.storage_path}</dd>
            <dt className="of-text-muted">Created</dt>
            <dd style={{ margin: 0 }}>{formatDate(dataset.created_at)}</dd>
            <dt className="of-text-muted">Updated</dt>
            <dd style={{ margin: 0 }}>{formatDate(dataset.updated_at)}</dd>
          </dl>
        </section>
        <section style={{ padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-md)', background: 'var(--bg-panel-muted)', display: 'grid', gap: 8 }}>
          <p className="of-eyebrow">API</p>
          {[
            ['Preview', previewPath],
            ['Files', `/api/v1/datasets/${encodeURIComponent(dataset.id)}/files?branch=${encodeURIComponent(activeBranch)}`],
            ['Schema', `/api/v1/datasets/${encodeURIComponent(dataset.id)}/schema?branch=${encodeURIComponent(activeBranch)}`],
          ].map(([label, path]) => (
            <div key={label} style={{ display: 'grid', gap: 4 }}>
              <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>{label}</span>
              <button type="button" onClick={() => onCopy(path, `${label} API path`)} className="of-button" style={{ justifyContent: 'flex-start', fontFamily: 'var(--font-mono)', fontSize: 11, overflowWrap: 'anywhere', whiteSpace: 'normal' }}>
                {path}
              </button>
            </div>
          ))}
        </section>
      </aside>
    </div>
  );
}

function PreviewMessages({ preview }: { preview: DatasetPreviewResponse }) {
  const warnings = preview.warnings ?? [];
  const errors = preview.errors ?? [];
  const parseErrors = preview.parse_errors ?? [];
  if (warnings.length === 0 && errors.length === 0 && parseErrors.length === 0 && !preview.message && !preview.sampled) return null;
  return (
    <div style={{ display: 'grid', gap: 6, padding: 8, borderBottom: '1px solid var(--border-default)' }}>
      {preview.message && <div className="of-status-info" style={{ padding: 8, borderRadius: 'var(--radius-sm)', fontSize: 12 }}>{preview.message}</div>}
      {preview.sampled && <div className="of-status-info" style={{ padding: 8, borderRadius: 'var(--radius-sm)', fontSize: 12 }}>Sampled preview rows.</div>}
      {warnings.map((warning) => (
        <div key={warning} className="of-status-warning" style={{ padding: 8, borderRadius: 'var(--radius-sm)', fontSize: 12 }}>{warning}</div>
      ))}
      {errors.map((previewError) => (
        <div key={previewError} className="of-status-danger" style={{ padding: 8, borderRadius: 'var(--radius-sm)', fontSize: 12 }}>{previewError}</div>
      ))}
      {parseErrors.slice(0, 5).map((parseError, index) => (
        <div key={`${parseError.file_path}-${parseError.row}-${parseError.field}-${index}`} className="of-status-danger" style={{ padding: 8, borderRadius: 'var(--radius-sm)', fontSize: 12 }}>
          {parseError.kind} in {parseError.file_path}{parseError.row ? ` row ${parseError.row}` : ''}{parseError.column ? ` column ${parseError.column}` : ''}{parseError.field ? ` (${parseError.field})` : ''}: {parseError.message}
        </div>
      ))}
      {parseErrors.length > 5 && <div className="of-text-muted" style={{ fontSize: 12 }}>{parseErrors.length - 5} more parse errors.</div>}
    </div>
  );
}

interface SchemaField {
  name?: string;
  type?: string | Record<string, unknown>;
  field_type?: string | Record<string, unknown>;
  data_type?: string;
  nullable?: boolean;
  description?: string;
}

function schemaFieldType(field: SchemaField) {
  const candidate = field.type ?? field.field_type ?? field.data_type;
  if (typeof candidate === 'string') return candidate;
  if (isRecord(candidate) && typeof candidate.type === 'string') return candidate.type;
  return 'n/a';
}

function normalizeSchemaFields(fields: unknown): SchemaField[] {
  if (Array.isArray(fields)) return fields.filter(isRecord).map((field) => field as SchemaField);
  if (isRecord(fields) && Array.isArray(fields.fields)) return fields.fields.filter(isRecord).map((field) => field as SchemaField);
  return [];
}

const SCHEMA_FIELD_TYPES = ['BOOLEAN', 'BYTE', 'SHORT', 'INTEGER', 'LONG', 'FLOAT', 'DOUBLE', 'STRING', 'BINARY', 'DATE', 'TIMESTAMP', 'DECIMAL', 'ARRAY', 'MAP', 'STRUCT'] as const;

const DEFAULT_SCHEMA_OPTIONS: DatasetCsvOptions = {
  delimiter: ',',
  quote: '"',
  escape: '\\',
  header: true,
  nullValue: '',
  charset: 'UTF-8',
  encoding: 'UTF-8',
  skipLines: 0,
  jaggedRowBehavior: 'FILL_NULLS',
  parseErrorBehavior: 'NULL',
  filePathColumn: false,
  importedAtColumn: false,
  rowNumberColumn: false,
  dynamicTyping: true,
};

function schemaPayload(schema: DatasetSchema | DatasetSchemaResponse | null): DatasetSchemaPayload {
  if (schema && 'schema' in schema) {
    return {
      fields: normalizeSchemaFields(schema.schema.fields).map((field) => normalizeDraftField(field as DatasetField)),
      file_format: (schema.schema.file_format || 'TEXT') as DatasetFileFormat,
      custom_metadata: schema.schema.custom_metadata ?? null,
    };
  }
  return {
    fields: schema ? normalizeSchemaFields(schema.fields).map((field) => normalizeDraftField(field as DatasetField)) : [],
    file_format: 'TEXT',
    custom_metadata: null,
  };
}

function schemaOptions(schema: DatasetSchema | DatasetSchemaResponse | null): DatasetCsvOptions {
  const csv = schemaPayload(schema).custom_metadata?.csv;
  if (!csv) return { ...DEFAULT_SCHEMA_OPTIONS };
  return {
    ...DEFAULT_SCHEMA_OPTIONS,
    ...csv,
    nullValue: csv.nullValue ?? csv.null_value ?? '',
    dateFormat: csv.dateFormat ?? csv.date_format,
    timestampFormat: csv.timestampFormat ?? csv.timestamp_format,
    encoding: csv.encoding || csv.charset || DEFAULT_SCHEMA_OPTIONS.encoding,
    charset: csv.charset || csv.encoding || DEFAULT_SCHEMA_OPTIONS.charset,
  };
}

function normalizeDraftField(field: DatasetField): DatasetField {
  const type = (field.type || schemaFieldType(field as SchemaField) || 'STRING') as DatasetField['type'];
  const next: DatasetField = {
    ...field,
    type,
    name: field.name || 'column',
    nullable: field.nullable ?? true,
    description: field.description ?? '',
  };
  if (type === 'DECIMAL') {
    next.precision = next.precision ?? 38;
    next.scale = next.scale ?? 18;
  }
  if (type === 'ARRAY') {
    next.arraySubtype = next.arraySubtype ?? next.arraySubType ?? { name: 'element', type: 'STRING', nullable: true };
    next.arraySubType = next.arraySubtype;
  }
  if (type === 'MAP') {
    next.mapKeyType = next.mapKeyType ?? { name: 'key', type: 'STRING', nullable: false };
    next.mapValueType = next.mapValueType ?? { name: 'value', type: 'STRING', nullable: true };
  }
  if (type === 'STRUCT') {
    next.subSchemas = next.subSchemas && next.subSchemas.length > 0 ? next.subSchemas : [{ name: 'value', type: 'STRING', nullable: true }];
  }
  return next;
}

function inferableFile(file: DatasetBackingFile) {
  const pathValue = `${file.logical_path || file.path || ''}`.toLowerCase();
  const media = `${file.media_type || file.content_type || ''}`.toLowerCase();
  return pathValue.endsWith('.csv') || pathValue.endsWith('.tsv') || pathValue.endsWith('.json') || pathValue.endsWith('.jsonl') || pathValue.endsWith('.ndjson') || media.includes('csv') || media.includes('json');
}

function fileFormatForPath(pathValue: string) {
  const lower = pathValue.toLowerCase();
  return lower.endsWith('.json') || lower.endsWith('.jsonl') || lower.endsWith('.ndjson') ? 'JSON' : 'CSV';
}

function cleanParserOptions(options: DatasetCsvOptions): DatasetCsvOptions {
  return {
    ...options,
    delimiter: options.delimiter || ',',
    quote: options.quote || '"',
    escape: options.escape || '\\',
    nullValue: options.nullValue ?? options.null_value ?? '',
    charset: options.charset || options.encoding || 'UTF-8',
    encoding: options.encoding || options.charset || 'UTF-8',
    skipLines: Number.isFinite(Number(options.skipLines)) ? Number(options.skipLines) : 0,
    jaggedRowBehavior: options.jaggedRowBehavior || 'FILL_NULLS',
    parseErrorBehavior: options.parseErrorBehavior || 'NULL',
  };
}

function SchemaEditor({
  datasetId,
  branch,
  schema,
  files,
  filesLoading,
  onNeedFiles,
  onApplied,
}: {
  datasetId: string;
  branch: string;
  schema: DatasetSchema | DatasetSchemaResponse | null;
  files: DatasetBackingFile[];
  filesLoading?: boolean;
  onNeedFiles: () => Promise<void> | void;
  onApplied: (message: string) => Promise<void>;
}) {
  const [format, setFormat] = useState('CSV');
  const [selectedPath, setSelectedPath] = useState('');
  const [options, setOptions] = useState<DatasetCsvOptions>(() => schemaOptions(schema));
  const [draftFields, setDraftFields] = useState<DatasetField[]>(() => schemaPayload(schema).fields);
  const [inference, setInference] = useState<DatasetSchemaInferenceResponse | null>(null);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [localError, setLocalError] = useState('');
  const [working, setWorking] = useState<'infer' | 'apply' | 'save' | null>(null);
  const availableFiles = files.filter(inferableFile);

  useEffect(() => {
    const payload = schemaPayload(schema);
    setDraftFields(payload.fields);
    setOptions(schemaOptions(schema));
  }, [schema]);

  useEffect(() => {
    if (selectedPath || availableFiles.length === 0) return;
    const pathValue = availableFiles[0].logical_path || availableFiles[0].path || '';
    setSelectedPath(pathValue);
    setFormat(fileFormatForPath(pathValue));
  }, [availableFiles, selectedPath]);

  function setOption<K extends keyof DatasetCsvOptions>(key: K, value: DatasetCsvOptions[K]) {
    setOptions((prev) => ({ ...prev, [key]: value }));
  }

  function updateField(index: number, patch: Partial<DatasetField>) {
    setDraftFields((prev) => prev.map((field, current) => (current === index ? normalizeDraftField({ ...field, ...patch }) : field)));
  }

  async function runInference(apply: boolean) {
    setWorking(apply ? 'apply' : 'infer');
    setLocalError('');
    try {
      if (availableFiles.length === 0 && files.length === 0) {
        await onNeedFiles();
      }
      const response = await inferDatasetSchema(datasetId, {
        branchName: branch,
        format,
        paths: selectedPath ? [selectedPath] : [],
        parserOptions: cleanParserOptions(options),
        apply,
        maxRows: 500,
      });
      setInference(response);
      setWarnings(response.warnings ?? []);
      setOptions(cleanParserOptions(response.parserOptions));
      setDraftFields(response.datasetSchema.fields.map(normalizeDraftField));
      if (apply) {
        await onApplied('Schema applied.');
      }
    } catch (cause) {
      setLocalError(cause instanceof Error ? cause.message : 'Schema inference failed.');
    } finally {
      setWorking(null);
    }
  }

  async function saveManualSchema() {
    setWorking('save');
    setLocalError('');
    try {
      await putDatasetSchemaForBranch(datasetId, branch, {
        fields: draftFields.map(normalizeDraftField),
        file_format: 'TEXT',
        custom_metadata: { csv: cleanParserOptions(options) },
      }, cleanParserOptions(options));
      await onApplied('Schema saved.');
    } catch (cause) {
      setLocalError(cause instanceof Error ? cause.message : 'Schema save failed.');
    } finally {
      setWorking(null);
    }
  }

  const busy = working !== null;
  const activeWarnings = warnings.length > 0 ? warnings : options.warnings ?? [];

  return (
    <div style={{ display: 'grid', gap: 12 }}>
      <section style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 180px), 1fr))', gap: 8, alignItems: 'end' }}>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          File
          <select value={selectedPath} onChange={(event) => { setSelectedPath(event.target.value); setFormat(fileFormatForPath(event.target.value)); }} className="of-input" style={{ fontSize: 12 }}>
            <option value="">{filesLoading ? 'Loading files...' : 'Auto-select sample'}</option>
            {availableFiles.map((file) => {
              const pathValue = file.logical_path || file.path || file.id;
              return <option key={file.id} value={pathValue}>{pathValue}</option>;
            })}
          </select>
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Format
          <select value={format} onChange={(event) => setFormat(event.target.value)} className="of-input" style={{ fontSize: 12 }}>
            <option value="CSV">CSV</option>
            <option value="JSON">JSON</option>
          </select>
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Delimiter
          <input value={options.delimiter} onChange={(event) => setOption('delimiter', event.target.value)} className="of-input" style={{ fontSize: 12 }} />
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Quote
          <input value={options.quote} onChange={(event) => setOption('quote', event.target.value)} className="of-input" style={{ fontSize: 12 }} />
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Escape
          <input value={options.escape} onChange={(event) => setOption('escape', event.target.value)} className="of-input" style={{ fontSize: 12 }} />
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Null value
          <input value={options.nullValue ?? ''} onChange={(event) => setOption('nullValue', event.target.value)} className="of-input" style={{ fontSize: 12 }} />
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Encoding
          <input value={options.encoding || options.charset} onChange={(event) => { setOption('encoding', event.target.value); setOption('charset', event.target.value); }} className="of-input" style={{ fontSize: 12 }} />
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Skip lines
          <input type="number" min={0} value={options.skipLines ?? 0} onChange={(event) => setOption('skipLines', Number(event.target.value))} className="of-input" style={{ fontSize: 12 }} />
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Jagged rows
          <select value={options.jaggedRowBehavior || 'FILL_NULLS'} onChange={(event) => setOption('jaggedRowBehavior', event.target.value)} className="of-input" style={{ fontSize: 12 }}>
            <option value="FILL_NULLS">Fill nulls</option>
            <option value="DROP_EXTRA">Drop extra</option>
            <option value="ERROR">Error</option>
          </select>
        </label>
        <label style={{ fontSize: 11, display: 'grid', gap: 4 }}>
          Parse errors
          <select value={options.parseErrorBehavior || 'NULL'} onChange={(event) => setOption('parseErrorBehavior', event.target.value)} className="of-input" style={{ fontSize: 12 }}>
            <option value="NULL">Null</option>
            <option value="SKIP_ROW">Skip row</option>
            <option value="ERROR">Error</option>
          </select>
        </label>
      </section>

      <section style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center', fontSize: 12 }}>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <input type="checkbox" checked={options.header} onChange={(event) => setOption('header', event.target.checked)} />
          Header
        </label>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <input type="checkbox" checked={Boolean(options.dynamicTyping)} onChange={(event) => setOption('dynamicTyping', event.target.checked)} />
          Dynamic inference
        </label>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <input type="checkbox" checked={Boolean(options.filePathColumn)} onChange={(event) => setOption('filePathColumn', event.target.checked)} />
          File path
        </label>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <input type="checkbox" checked={Boolean(options.importedAtColumn)} onChange={(event) => setOption('importedAtColumn', event.target.checked)} />
          Imported at
        </label>
        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
          <input type="checkbox" checked={Boolean(options.rowNumberColumn)} onChange={(event) => setOption('rowNumberColumn', event.target.checked)} />
          Row number
        </label>
      </section>

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        <button type="button" onClick={() => void runInference(false)} disabled={busy} className="of-button">
          {working === 'infer' ? 'Inferring...' : 'Infer schema'}
        </button>
        <button type="button" onClick={() => void runInference(true)} disabled={busy} className="of-button of-button--primary">
          {working === 'apply' ? 'Applying...' : 'Apply schema'}
        </button>
        <button type="button" onClick={() => setDraftFields((prev) => [...prev, { name: `column_${prev.length + 1}`, type: 'STRING', nullable: true, description: '' }])} disabled={busy} className="of-button">
          Add column
        </button>
        <button type="button" onClick={() => void saveManualSchema()} disabled={busy || draftFields.length === 0} className="of-button">
          {working === 'save' ? 'Saving...' : 'Save edits'}
        </button>
      </div>

      {localError && <div className="of-status-danger" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>{localError}</div>}
      {inference && <div className="of-status-info" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>Sampled {inference.sampleRows.toLocaleString()} rows on {inference.branchName}.</div>}
      {activeWarnings.length > 0 && (
        <div className="of-status-info" style={{ padding: '8px 10px', borderRadius: 'var(--radius-md)', fontSize: 12 }}>
          {activeWarnings.slice(0, 4).join(' ')}
        </div>
      )}

      {draftFields.length === 0 ? (
        <EmptyBlock label="No schema is available." />
      ) : (
        <table className="of-table" style={{ fontSize: 12 }}>
          <thead>
            <tr>
              {['Name', 'Type', 'Nullable', 'Description', ''].map((heading) => <th key={heading}>{heading}</th>)}
            </tr>
          </thead>
          <tbody>
            {draftFields.map((field, index) => (
              <tr key={`${field.name}-${index}`}>
                <td>
                  <input value={field.name} onChange={(event) => updateField(index, { name: event.target.value })} className="of-input" style={{ fontSize: 12, fontFamily: 'var(--font-mono)' }} />
                </td>
                <td>
                  <select value={field.type} onChange={(event) => updateField(index, { type: event.target.value as DatasetField['type'] })} className="of-input" style={{ fontSize: 12 }}>
                    {SCHEMA_FIELD_TYPES.map((type) => <option key={type} value={type}>{type}</option>)}
                  </select>
                </td>
                <td>
                  <input type="checkbox" checked={Boolean(field.nullable)} onChange={(event) => updateField(index, { nullable: event.target.checked })} />
                </td>
                <td>
                  <input value={field.description ?? ''} onChange={(event) => updateField(index, { description: event.target.value })} className="of-input" style={{ fontSize: 12 }} />
                </td>
                <td style={{ textAlign: 'right' }}>
                  <button type="button" onClick={() => setDraftFields((prev) => prev.filter((_, current) => current !== index))} disabled={busy} className="of-button" style={{ fontSize: 11 }}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function FilesTable({ datasetId, files }: { datasetId: string; files: DatasetBackingFile[] }) {
  return (
    <table className="of-table">
      <thead>
        <tr>
          <th>Path</th>
          <th>Media</th>
          <th>Size</th>
          <th>Rows</th>
          <th>Checksum</th>
          <th>Transaction</th>
          <th>Storage</th>
          <th>Status</th>
          <th>Modified</th>
          <th>Action</th>
        </tr>
      </thead>
      <tbody>
        {files.map((file) => (
          <tr key={file.id}>
            <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11, overflowWrap: 'anywhere' }}>{file.path || file.logical_path}</td>
            <td>{file.media_type || file.content_type || 'n/a'}</td>
            <td>{formatBytes(file.size_bytes)}</td>
            <td>{file.row_count_hint?.toLocaleString() ?? 'n/a'}</td>
            <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{shortHash(file.sha256)}</td>
            <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11, overflowWrap: 'anywhere' }}>{file.transaction_rid || file.transaction_id}</td>
            <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11, overflowWrap: 'anywhere' }}>{fileStorageURI(file)}</td>
            <td>{file.status}</td>
            <td>{formatDate(file.updated_time || file.modified_at)}</td>
            <td>
              {file.status === 'active' ? (
                <a className="of-button" href={datasetFileDownloadUrl(datasetId, file.id)} style={{ textDecoration: 'none', fontSize: 11, padding: '3px 8px' }}>
                  Download
                </a>
              ) : (
                <span className="of-text-muted">n/a</span>
              )}
            </td>
          </tr>
        ))}
        {files.length === 0 && <tr><td colSpan={10} className="of-text-muted">No files.</td></tr>}
      </tbody>
    </table>
  );
}

function HistoryPanel({
  transactions,
  incrementalReadiness,
  versions,
  selectedTransactionId,
  selectedVersion,
  onSelectTransaction,
  onSelectVersion,
}: {
  transactions: DatasetTransaction[];
  incrementalReadiness: DatasetIncrementalReadiness | null;
  versions: DatasetVersion[];
  selectedTransactionId: string | null;
  selectedVersion: number | null;
  onSelectTransaction: (txId: string | null) => void;
  onSelectVersion: (version: number | null) => void;
}) {
  return (
    <div style={{ display: 'grid', gap: 16 }}>
      <IncrementalReadinessPanel readiness={incrementalReadiness} />
      <section>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <div>
            <p className="of-eyebrow">Transactions</p>
            <h2 className="of-heading-sm" style={{ marginTop: 2 }}>{transactions.length} transaction{transactions.length === 1 ? '' : 's'}</h2>
          </div>
          <select value={selectedTransactionId ?? ''} onChange={(event) => onSelectTransaction(event.target.value || null)} className="of-input" style={{ width: 260, fontSize: 12 }}>
            <option value="">Latest transaction</option>
            {transactions.map((transaction) => (
              <option key={transaction.id} value={transaction.id}>{shortId(transaction.id, 10)} - {transaction.status}</option>
            ))}
          </select>
        </header>
        <div style={{ marginTop: 8, overflow: 'auto' }}>
          <TransactionsTable transactions={transactions} selectedTransactionId={selectedTransactionId} />
        </div>
      </section>
      <section>
        <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <div>
            <p className="of-eyebrow">Versions</p>
            <h2 className="of-heading-sm" style={{ marginTop: 2 }}>{versions.length} version{versions.length === 1 ? '' : 's'}</h2>
          </div>
          <select value={selectedVersion ?? ''} onChange={(event) => onSelectVersion(event.target.value ? Number(event.target.value) : null)} className="of-input" style={{ width: 180, fontSize: 12 }}>
            <option value="">Latest version</option>
            {versions.map((version) => (
              <option key={version.id} value={version.version}>v{version.version}</option>
            ))}
          </select>
        </header>
        <div style={{ marginTop: 8, overflow: 'auto' }}>
          <VersionsTable versions={versions} />
        </div>
      </section>
    </div>
  );
}

function incrementalTone(mode?: string) {
  switch (mode) {
    case 'append_only':
      return { background: 'rgba(22, 163, 74, 0.14)', color: '#166534' };
    case 'update_bearing':
    case 'delete_bearing':
    case 'mixed':
      return { background: 'rgba(220, 38, 38, 0.12)', color: '#991b1b' };
    case 'snapshot_based':
      return { background: 'rgba(217, 119, 6, 0.14)', color: '#92400e' };
    default:
      return { background: 'var(--bg-chip)', color: 'var(--text-muted)' };
  }
}

function titleCaseMode(mode: string) {
  return mode.split('_').map((part) => part.slice(0, 1).toUpperCase() + part.slice(1)).join(' ');
}

function IncrementalReadinessPanel({ readiness }: { readiness: DatasetIncrementalReadiness | null }) {
  if (!readiness) {
    return <EmptyBlock label="Incremental readiness has not been loaded yet." />;
  }
  const currentStart = readiness.current_view_start;
  const currentEnd = readiness.current_view_end;
  const warnings = readiness.warnings ?? [];
  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
        <div>
          <p className="of-eyebrow">Incremental readiness</p>
          <h2 className="of-heading-sm" style={{ marginTop: 2 }}>{titleCaseMode(readiness.mode)}</h2>
        </div>
        <span className="of-chip" style={incrementalTone(readiness.mode)}>
          {readiness.incremental_ready ? 'Ready for append-only incremental' : 'Needs review'}
        </span>
      </header>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 8 }}>
        <HealthStat label="Committed txns" value={readiness.total_committed.toLocaleString()} />
        <HealthStat label="APPEND" value={String(readiness.transaction_counts.APPEND ?? 0)} />
        <HealthStat label="SNAPSHOT" value={String(readiness.transaction_counts.SNAPSHOT ?? 0)} />
        <HealthStat label="UPDATE" value={String(readiness.transaction_counts.UPDATE ?? 0)} />
        <HealthStat label="DELETE" value={String(readiness.transaction_counts.DELETE ?? 0)} />
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: 8 }}>
        <BoundaryCard title="First snapshot" boundary={readiness.first_snapshot ?? null} empty="No snapshot baseline has been committed." />
        <BoundaryCard title="Current view start" boundary={currentStart ?? null} empty="No current view boundary." />
        <BoundaryCard title="Current view end" boundary={currentEnd ?? null} empty="No committed end boundary." />
      </div>
      {warnings.length > 0 && (
        <div style={{ display: 'grid', gap: 6 }}>
          {warnings.map((warning) => (
            <div key={`${warning.code}-${warning.transaction_rid ?? ''}`} className="of-callout of-callout--warning">
              <strong>{warning.code}</strong>
              <span style={{ display: 'block', marginTop: 2 }}>{warning.message}</span>
            </div>
          ))}
        </div>
      )}
      <div style={{ overflow: 'auto' }}>
        <table className="of-table">
          <thead>
            <tr><th>Window</th><th>Reason</th><th>Start</th><th>End</th><th>Transactions</th><th>Append-only</th></tr>
          </thead>
          <tbody>
            {readiness.view_boundaries.map((boundary, index) => (
              <tr key={`${boundary.start.transaction_id}-${boundary.end.transaction_id}`}>
                <td>{index + 1}</td>
                <td>{boundary.start_reason}</td>
                <td>{boundary.start.tx_type} · {shortId(boundary.start.transaction_rid)}</td>
                <td>{boundary.end.tx_type} · {shortId(boundary.end.transaction_rid)}</td>
                <td>{boundary.transaction_count}</td>
                <td>{boundary.append_only ? 'Yes' : 'No'}</td>
              </tr>
            ))}
            {readiness.view_boundaries.length === 0 && <tr><td colSpan={6} className="of-text-muted">No incremental view boundaries are available.</td></tr>}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function BoundaryCard({ title, boundary, empty }: { title: string; boundary: IncrementalTransactionBoundary | null; empty: string }) {
  return (
    <div style={{ border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-md)', padding: 12 }}>
      <div className="of-text-muted" style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.12em' }}>{title}</div>
      {boundary ? (
        <div style={{ marginTop: 6, display: 'grid', gap: 3 }}>
          <strong>{boundary.tx_type} · {shortId(boundary.transaction_rid)}</strong>
          <span className="of-text-muted">{formatDate(boundary.committed_at || boundary.started_at)}</span>
          <span className="of-text-muted">{boundary.file_count.toLocaleString()} files · {formatBytes(boundary.size_bytes)}</span>
        </div>
      ) : (
        <div className="of-text-muted" style={{ marginTop: 6 }}>{empty}</div>
      )}
    </div>
  );
}

function buildStateStyle(state: string) {
  const colors = BUILD_STATE_COLORS[state as keyof typeof BUILD_STATE_COLORS];
  return colors ? { background: colors.bg, color: colors.text } : { background: 'var(--bg-chip)', color: 'var(--text-default)' };
}

function JobsTable({ builds }: { builds: Build[] }) {
  return (
    <table className="of-table">
      <thead>
        <tr><th>Build</th><th>State</th><th>Branch</th><th>Trigger</th><th>Requested by</th><th>Started</th><th>Finished</th><th>Open</th></tr>
      </thead>
      <tbody>
        {builds.map((build) => (
          <tr key={build.rid}>
            <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11, overflowWrap: 'anywhere' }}>{build.rid}</td>
            <td><span className="of-chip" style={buildStateStyle(build.state)}>{build.state}</span></td>
            <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{build.build_branch}</td>
            <td>{build.trigger_kind}</td>
            <td>{build.requested_by || 'n/a'}</td>
            <td>{formatDate(build.started_at || build.queued_at || build.created_at)}</td>
            <td>{formatDate(build.finished_at)}</td>
            <td><Link className="of-button" to={`/builds/${encodeURIComponent(build.rid)}`} style={{ textDecoration: 'none', fontSize: 11, padding: '3px 8px' }}>Open</Link></td>
          </tr>
        ))}
        {builds.length === 0 && <tr><td colSpan={8} className="of-text-muted">No builds are linked to this dataset yet.</td></tr>}
      </tbody>
    </table>
  );
}

function scheduleTargetSummary(schedule: Schedule) {
  const target = schedule.target?.kind;
  if (!target || typeof target !== 'object') return 'n/a';
  const keys = Object.keys(target);
  return keys.length > 0 ? keys.join(', ') : 'n/a';
}

function SchedulesTable({ schedules, datasetRid }: { schedules: Schedule[]; datasetRid: string }) {
  return (
    <section style={{ display: 'grid', gap: 10 }}>
      <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
        <Link className="of-button of-button--primary" to={`/schedules/new?event_target=${encodeURIComponent(datasetRid)}`}>
          New schedule
        </Link>
      </div>
      <table className="of-table">
        <thead>
          <tr><th>Name</th><th>RID</th><th>Target</th><th>Status</th><th>Last run</th><th>Updated</th><th>Open</th></tr>
        </thead>
        <tbody>
          {schedules.map((schedule) => (
            <tr key={schedule.rid}>
              <td>{schedule.name}</td>
              <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11, overflowWrap: 'anywhere' }}>{schedule.rid}</td>
              <td>{scheduleTargetSummary(schedule)}</td>
              <td>{schedule.paused ? `Paused${schedule.paused_reason ? `: ${schedule.paused_reason}` : ''}` : 'Active'}</td>
              <td>{formatDate(schedule.last_run_at)}</td>
              <td>{formatDate(schedule.updated_at)}</td>
              <td><Link className="of-button" to={`/schedules/${encodeURIComponent(schedule.rid)}`} style={{ textDecoration: 'none', fontSize: 11, padding: '3px 8px' }}>Open</Link></td>
            </tr>
          ))}
          {schedules.length === 0 && <tr><td colSpan={7} className="of-text-muted">No schedules target this dataset yet.</td></tr>}
        </tbody>
      </table>
    </section>
  );
}

function HealthTab({
  datasetRid,
  quality,
  health,
  lint,
  loading,
  refreshing,
  onRefreshProfile,
}: {
  datasetRid?: string;
  quality: DatasetQualityResponse | null;
  health: DatasetHealthResponse | null;
  lint: DatasetLintResponse | null;
  loading: boolean;
  refreshing: boolean;
  onRefreshProfile: () => void;
}) {
  return (
    <div style={{ display: 'grid', gap: 14 }}>
      <QualityDashboard
        datasetRid={datasetRid}
        quality={quality}
        loading={loading}
        refreshing={refreshing}
        onRefreshProfile={onRefreshProfile}
      />
      <section style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 220px), 1fr))', gap: 8 }}>
        <HealthStat label="Rows" value={health ? health.row_count.toLocaleString() : 'n/a'} />
        <HealthStat label="Columns" value={health ? health.col_count.toLocaleString() : 'n/a'} />
        <HealthStat label="Freshness" value={health ? `${Math.round(health.freshness_seconds / 60)} min` : 'n/a'} />
        <HealthStat label="Last build" value={health?.last_build_status ?? 'n/a'} />
        <HealthStat label="Schema drift" value={health?.schema_drift_flag ? 'Detected' : health ? 'None' : 'n/a'} />
        <HealthStat label="Lint findings" value={lint ? lint.summary.total_findings.toLocaleString() : 'n/a'} />
      </section>
      {lint && lint.findings.length > 0 && (
        <table className="of-table">
          <thead><tr><th>Severity</th><th>Finding</th><th>Recommendation</th></tr></thead>
          <tbody>
            {lint.findings.map((finding) => (
              <tr key={finding.code}>
                <td>{finding.severity}</td>
                <td>{finding.title}</td>
                <td className="of-text-muted">{finding.recommendation}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {!loading && !health && !quality?.profile && !lint && <EmptyBlock label="No health data is available for this dataset yet." />}
    </div>
  );
}

function HealthStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ padding: 10, border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-md)', background: 'var(--bg-panel-muted)' }}>
      <p className="of-eyebrow">{label}</p>
      <p style={{ marginTop: 4, fontSize: 18, fontWeight: 600, color: 'var(--text-strong)' }}>{value}</p>
    </div>
  );
}

function LineageTab({ dataset, activeBranch, onOpen, onCopy }: { dataset: Dataset; activeBranch: string; onOpen: () => void; onCopy: (text: string, label: string) => void }) {
  const lineagePath = dataset.links?.lineage || `/lineage?dataset=${encodeURIComponent(dataset.id)}`;
  return (
    <div style={{ display: 'grid', gap: 12, maxWidth: 780 }}>
      <section style={{ padding: 12, border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-md)', background: 'var(--bg-panel-muted)' }}>
        <p className="of-eyebrow">Lineage entrypoint</p>
        <h2 className="of-heading-sm" style={{ marginTop: 4 }}>Open the dataset in Data Lineage</h2>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
          Active branch: <code>{activeBranch}</code>. Dataset RID: <code>{dataset.rid || dataset.id}</code>.
        </p>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 10 }}>
          <button type="button" onClick={onOpen} className="of-button of-button--primary">Open lineage</button>
          <button type="button" onClick={() => onCopy(lineagePath, 'Lineage path')} className="of-button">Copy lineage path</button>
        </div>
      </section>
      <section style={{ padding: 12, border: '1px dashed var(--border-default)', borderRadius: 'var(--radius-md)' }}>
        <p className="of-text-muted" style={{ margin: 0, fontSize: 13 }}>
          Upstream and downstream graph expansion is handled by the Data Lineage app. This tab keeps Dataset Preview anchored to the same graph entrypoint and branch context.
        </p>
      </section>
    </div>
  );
}

function TransactionsTable({ transactions, selectedTransactionId }: { transactions: DatasetTransaction[]; selectedTransactionId: string | null }) {
  return (
    <table className="of-table">
      <thead>
        <tr><th>ID</th><th>Operation</th><th>Branch</th><th>Status</th><th>Created</th><th>Closed</th><th>Summary</th></tr>
      </thead>
      <tbody>
        {transactions.map((transaction) => {
          const selected = selectedTransactionId === transaction.id;
          return (
            <tr key={transaction.id} style={{ background: selected ? 'var(--status-info-bg)' : undefined }}>
              <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{transaction.id}</td>
              <td>{transaction.operation}</td>
              <td style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{transaction.branch_name ?? 'n/a'}</td>
              <td>{transaction.status}</td>
              <td>{formatDate(transaction.created_at)}</td>
              <td>{formatDate(transaction.closedTime || transaction.committed_at || transaction.aborted_at)}</td>
              <td className="of-text-muted">{transaction.summary || 'n/a'}</td>
            </tr>
          );
        })}
        {transactions.length === 0 && <tr><td colSpan={7} className="of-text-muted">No transactions.</td></tr>}
      </tbody>
    </table>
  );
}

function VersionsTable({ versions }: { versions: DatasetVersion[] }) {
  return (
    <table className="of-table">
      <thead>
        <tr><th>Version</th><th>Message</th><th>Rows</th><th>Size</th><th>Created</th></tr>
      </thead>
      <tbody>
        {versions.map((version) => (
          <tr key={version.id}>
            <td>v{version.version}</td>
            <td>{version.message || 'n/a'}</td>
            <td>{version.row_count.toLocaleString()}</td>
            <td>{formatBytes(version.size_bytes)}</td>
            <td>{formatDate(version.created_at)}</td>
          </tr>
        ))}
        {versions.length === 0 && <tr><td colSpan={5} className="of-text-muted">No versions.</td></tr>}
      </tbody>
    </table>
  );
}

function ExportDialog({
  dataset,
  open,
  busy,
  error,
  result,
  onClose,
  onSubmit,
}: {
  dataset: Dataset;
  open: boolean;
  busy: boolean;
  error: string;
  result: DatasetExportResponse | null;
  onClose: () => void;
  onSubmit: (params: DatasetExportParams) => void | Promise<void>;
}) {
  const [format, setFormat] = useState<DatasetExportParams['format']>('CSV');
  const [includeSchema, setIncludeSchema] = useState(true);

  useEffect(() => {
    if (!open) return;
    setFormat('CSV');
    setIncludeSchema(true);
  }, [open, dataset.id]);

  if (!open) return null;

  return (
    <div role="dialog" aria-modal="true" aria-label="Export dataset" style={{ position: 'fixed', inset: 0, background: 'rgba(15,23,42,0.38)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100, padding: 16 }}>
      <div className="of-panel" style={{ width: '100%', maxWidth: 500, padding: 16, display: 'grid', gap: 12 }}>
        <header>
          <h2 className="of-heading-sm">Export dataset</h2>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
            Create an export from branch <code>{dataset.active_branch}</code>, version <code>v{dataset.current_version}</code>.
          </p>
        </header>

        <label style={{ display: 'grid', gap: 4, fontSize: 12 }}>
          Format
          <select value={format} onChange={(event) => setFormat(event.target.value as DatasetExportParams['format'])} className="of-input">
            <option value="CSV">CSV</option>
            <option value="PARQUET">Parquet</option>
          </select>
        </label>

        <label style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 12 }}>
          <input type="checkbox" checked={includeSchema} onChange={(event) => setIncludeSchema(event.target.checked)} />
          Include schema sidecar
        </label>

        {error && <div className="of-status-danger" style={{ padding: 8, borderRadius: 'var(--radius-sm)', fontSize: 12 }}>{error}</div>}
        {result && (
          <div className="of-status-success" style={{ padding: 8, borderRadius: 'var(--radius-sm)', fontSize: 12 }}>
            Export requested{actionReference(result)}.
            {result.download_url && (
              <a href={result.download_url} style={{ marginLeft: 6, color: 'inherit', textDecoration: 'underline' }}>Open download</a>
            )}
          </div>
        )}

        <footer style={{ display: 'flex', justifyContent: 'flex-end', gap: 6 }}>
          <button type="button" onClick={onClose} disabled={busy} className="of-button">Close</button>
          <button
            type="button"
            onClick={() => void onSubmit({ format, branch: dataset.active_branch, version: dataset.current_version, include_schema: includeSchema })}
            disabled={busy}
            className="of-button of-button--primary"
          >
            {busy ? 'Exporting...' : 'Start export'}
          </button>
        </footer>
      </div>
    </div>
  );
}
