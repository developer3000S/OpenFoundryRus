import { useEffect, useMemo, useState } from 'react';

import { BranchManager, type BranchDraft } from '@/lib/components/code-repo/BranchManager';
import { CommitHistory, type CommitDraft } from '@/lib/components/code-repo/CommitHistory';
import { DiffViewer } from '@/lib/components/code-repo/DiffViewer';
import { FileViewer } from '@/lib/components/code-repo/FileViewer';
import { MergeRequestDetail, type CommentDraft } from '@/lib/components/code-repo/MergeRequestDetail';
import { MergeRequestList, type MergeRequestDraft } from '@/lib/components/code-repo/MergeRequestList';
import { RepoExplorer, type RepositoryDraft } from '@/lib/components/code-repo/RepoExplorer';
import {
  createBranch,
  createComment,
  createCommit,
  createMergeRequest,
  createRepository,
  getDiff,
  getMergeRequest,
  getOverview,
  listBranches,
  listCiRuns,
  listCommits,
  listFiles,
  listMergeRequests,
  listRepositories,
  mergeMergeRequest,
  searchFiles,
  triggerCiRun,
  updateMergeRequest,
  updateRepository,
  type BranchDefinition,
  type CiRun,
  type CommitDefinition,
  type MergeRequestDefinition,
  type MergeRequestDetail as MergeRequestDetailModel,
  type MergeRequestStatus,
  type RepositoryDefinition,
  type RepositoryFile,
  type RepositoryOverview,
  type ReviewerState,
  type SearchResult,
} from '@/lib/api/code-repos';
import { notifications } from '@stores/notifications';

function emptyRepoDraft(): RepositoryDraft {
  return {
    name: 'Foundry Widget Kit',
    slug: 'foundry-widget-kit',
    description: 'Shared widget primitives ready for marketplace publication.',
    owner: 'Platform UI',
    default_branch: 'main',
    visibility: 'private',
    object_store_backend: 'gitoxide-pack',
    package_kind: 'widget',
    tags_text: 'widgets, ui, marketplace',
    settings_text: JSON.stringify({ default_path: 'src/lib.rs', ci_required: true, allow_direct_commits_on_protected: false }, null, 2),
  };
}

function emptyBranchDraft(defaultBranch = 'main'): BranchDraft {
  return { name: 'feature/new-package-flow', base_branch: defaultBranch, protected: false };
}

function emptyCommitDraft(defaultBranch = 'main'): CommitDraft {
  return {
    branch_name: defaultBranch,
    title: 'Refine package manifest defaults',
    description: 'Tightens metadata and manifest defaults ahead of publication.',
    author_name: 'Platform UI',
    additions: '24',
    deletions: '6',
  };
}

function emptyMergeRequestDraft(defaultBranch = 'main'): MergeRequestDraft {
  return {
    title: 'Publish package flow improvements',
    description: 'Promotes the feature branch after CI and inline review are green.',
    source_branch: 'feature/new-package-flow',
    target_branch: defaultBranch,
    author: 'Platform UI',
    labels_text: 'preview, package',
    reviewers_text: 'Elena, Marco',
    approvals_required: '2',
    changed_files: '5',
  };
}

function emptyCommentDraft(filePath = 'src/lib.rs'): CommentDraft {
  return {
    author: 'Reviewer Bot',
    body: 'Please split the publishing helper into a smaller function before merge.',
    file_path: filePath,
    line_number: '12',
    resolved: false,
  };
}

function parseCsv(value: string) {
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function parseJson<T>(value: string) {
  return JSON.parse(value) as T;
}

function preferredCommitBranch(defaultBranch: string, branches: BranchDefinition[]) {
  return branches.find((branch) => !branch.protected)?.name ?? defaultBranch;
}

function repoToDraft(repository: RepositoryDefinition): RepositoryDraft {
  return {
    id: repository.id,
    name: repository.name,
    slug: repository.slug,
    description: repository.description,
    owner: repository.owner,
    default_branch: repository.default_branch,
    visibility: repository.visibility,
    object_store_backend: repository.object_store_backend,
    package_kind: repository.package_kind,
    tags_text: repository.tags.join(', '),
    settings_text: JSON.stringify(repository.settings, null, 2),
  };
}

export function CodeReposPage() {
  const [overview, setOverview] = useState<RepositoryOverview | null>(null);
  const [repositories, setRepositories] = useState<RepositoryDefinition[]>([]);
  const [branches, setBranches] = useState<BranchDefinition[]>([]);
  const [commits, setCommits] = useState<CommitDefinition[]>([]);
  const [files, setFiles] = useState<RepositoryFile[]>([]);
  const [ciRuns, setCiRuns] = useState<CiRun[]>([]);
  const [mergeRequests, setMergeRequests] = useState<MergeRequestDefinition[]>([]);
  const [mergeRequestDetail, setMergeRequestDetail] = useState<MergeRequestDetailModel | null>(null);
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [selectedRepositoryId, setSelectedRepositoryId] = useState('');
  const [selectedMergeRequestId, setSelectedMergeRequestId] = useState('');
  const [selectedFilePath, setSelectedFilePath] = useState('');
  const [searchQuery, setSearchQuery] = useState('widget');
  const [diffBranch, setDiffBranch] = useState('main');
  const [diffPatch, setDiffPatch] = useState('');
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [uiError, setUiError] = useState('');

  const [repositoryDraft, setRepositoryDraft] = useState<RepositoryDraft>(emptyRepoDraft);
  const [branchDraft, setBranchDraft] = useState<BranchDraft>(emptyBranchDraft);
  const [commitDraft, setCommitDraft] = useState<CommitDraft>(emptyCommitDraft);
  const [mergeRequestDraft, setMergeRequestDraft] = useState<MergeRequestDraft>(emptyMergeRequestDraft);
  const [commentDraft, setCommentDraft] = useState<CommentDraft>(() => emptyCommentDraft());

  const busy = loading || busyAction.length > 0;
  const branchOptions = useMemo(() => branches.map((branch) => branch.name), [branches]);
  const currentRepository = useMemo(
    () => repositories.find((r) => r.id === selectedRepositoryId) ?? null,
    [repositories, selectedRepositoryId],
  );

  function repositoryCiRequired(repository: RepositoryDefinition | null) {
    return repository?.settings?.['ci_required'] !== false;
  }

  function latestCiRunForBranch(branchName: string) {
    return ciRuns.find((run) => run.branch_name === branchName) ?? null;
  }

  function targetBranch(branchName: string) {
    return branches.find((branch) => branch.name === branchName) ?? null;
  }

  function mergeBlockers(detail: MergeRequestDetailModel | null) {
    if (!detail) return [];
    const blockers: string[] = [];
    const target = targetBranch(detail.merge_request.target_branch);
    const latestSourceCi = latestCiRunForBranch(detail.merge_request.source_branch);
    const requiredApprovals = detail.merge_request.approvals_required;
    if (target?.protected && detail.approval_count < requiredApprovals) {
      blockers.push(`Protected branch requires ${requiredApprovals} approval(s); only ${detail.approval_count} recorded.`);
    }
    if (repositoryCiRequired(currentRepository)) {
      if (!latestSourceCi) {
        blockers.push(`Branch ${detail.merge_request.source_branch} has no CI run on record.`);
      } else if (latestSourceCi.commit_sha !== targetBranch(detail.merge_request.source_branch)?.head_sha) {
        blockers.push(`Latest CI does not cover the current head of ${detail.merge_request.source_branch}.`);
      } else if (latestSourceCi.status !== 'passed') {
        blockers.push(`Latest CI on ${detail.merge_request.source_branch} is ${latestSourceCi.status}.`);
      }
    }
    if (detail.merge_request.status === 'closed') {
      blockers.push('Closed merge requests cannot be merged until reopened.');
    }
    if (detail.merge_request.status === 'merged') {
      blockers.push('This merge request is already merged.');
    }
    return blockers;
  }

  async function loadRepositoryContext(repositoryId: string, preferredMergeRequestId?: string, notify = true) {
    setSelectedRepositoryId(repositoryId);
    const repository = repositories.find((entry) => entry.id === repositoryId) ?? null;
    setRepositoryDraft(repository ? repoToDraft(repository) : emptyRepoDraft());
    const defaultBranch = repository?.default_branch ?? 'main';
    setBranchDraft(emptyBranchDraft(defaultBranch));
    setMergeRequestDraft(emptyMergeRequestDraft(defaultBranch));
    setDiffBranch(defaultBranch);

    const [branchesResponse, commitsResponse, filesResponse, ciRunsResponse, diffResponse, mergeRequestsResponse] = await Promise.all([
      listBranches(repositoryId),
      listCommits(repositoryId),
      listFiles(repositoryId),
      listCiRuns(repositoryId),
      getDiff(repositoryId, defaultBranch),
      listMergeRequests(repositoryId),
    ]);

    setBranches(branchesResponse.items);
    setCommits(commitsResponse.items);
    setFiles(filesResponse.items);
    setCiRuns(ciRunsResponse.items);
    setDiffPatch(diffResponse.patch);
    setMergeRequests(mergeRequestsResponse.items);
    setCommitDraft(emptyCommitDraft(preferredCommitBranch(defaultBranch, branchesResponse.items)));
    const initialFilePath = filesResponse.items[0]?.path ?? '';
    setSelectedFilePath(initialFilePath);
    setCommentDraft(emptyCommentDraft(initialFilePath || 'src/lib.rs'));
    setSearchResults([]);

    const nextMergeRequestId = preferredMergeRequestId ?? selectedMergeRequestId ?? mergeRequestsResponse.items[0]?.id ?? '';
    if (nextMergeRequestId) {
      try {
        const detail = await getMergeRequest(nextMergeRequestId);
        setSelectedMergeRequestId(nextMergeRequestId);
        setMergeRequestDetail(detail);
      } catch (error) {
        const message = error instanceof Error ? error.message : 'Unable to load merge request';
        setUiError(message);
      }
    } else {
      setSelectedMergeRequestId('');
      setMergeRequestDetail(null);
    }

    if (notify && repository) {
      notifications.info(`Loaded ${repository.name}`);
    }
  }

  async function refreshAll(preferredRepositoryId?: string, preferredMergeRequestId?: string) {
    setLoading(true);
    setUiError('');
    try {
      const [overviewResponse, repositoriesResponse] = await Promise.all([getOverview(), listRepositories()]);
      setOverview(overviewResponse);
      setRepositories(repositoriesResponse.items);
      const nextRepositoryId = preferredRepositoryId ?? selectedRepositoryId ?? repositoriesResponse.items[0]?.id ?? '';
      if (nextRepositoryId) {
        await loadRepositoryContext(nextRepositoryId, preferredMergeRequestId, false);
      } else {
        setBranches([]);
        setCommits([]);
        setFiles([]);
        setCiRuns([]);
        setMergeRequests([]);
        setMergeRequestDetail(null);
        setDiffPatch('');
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load repository surfaces';
      setUiError(message);
      notifications.error(message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refreshAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function selectRepository(repositoryId: string, notify = true) {
    if (!repositoryId) {
      setSelectedRepositoryId('');
      setRepositoryDraft(emptyRepoDraft());
      return;
    }
    setBusyAction('repository');
    setUiError('');
    try {
      await loadRepositoryContext(repositoryId, undefined, notify);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load repository';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function refreshDiff(branchName: string) {
    if (!selectedRepositoryId) return;
    setBusyAction('diff');
    try {
      const response = await getDiff(selectedRepositoryId, branchName);
      setDiffBranch(response.branch_name);
      setDiffPatch(response.patch);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to refresh diff';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function selectMergeRequest(mergeRequestId: string, notify = true) {
    setBusyAction('merge-request');
    try {
      setSelectedMergeRequestId(mergeRequestId);
      const detail = await getMergeRequest(mergeRequestId);
      setMergeRequestDetail(detail);
      setCommentDraft((current) => ({
        ...current,
        file_path: detail.comments[0]?.file_path || selectedFilePath || 'src/lib.rs',
      }));
      if (notify) {
        notifications.info(`Loaded ${detail.merge_request.title}`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load merge request';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function saveRepository() {
    setBusyAction('save-repository');
    setUiError('');
    try {
      const payload = {
        name: repositoryDraft.name,
        slug: repositoryDraft.slug,
        description: repositoryDraft.description,
        owner: repositoryDraft.owner,
        default_branch: repositoryDraft.default_branch,
        visibility: repositoryDraft.visibility,
        object_store_backend: repositoryDraft.object_store_backend,
        package_kind: repositoryDraft.package_kind,
        tags: parseCsv(repositoryDraft.tags_text),
        settings: parseJson<Record<string, unknown>>(repositoryDraft.settings_text),
      };
      const repository = repositoryDraft.id
        ? await updateRepository(repositoryDraft.id, payload)
        : await createRepository(payload);
      await refreshAll(repository.id, selectedMergeRequestId || undefined);
      notifications.success(`${repositoryDraft.id ? 'Updated' : 'Created'} ${repository.name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save repository';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createBranchAction() {
    if (!selectedRepositoryId) {
      notifications.warning('Select a repository before creating a branch');
      return;
    }
    setBusyAction('branch');
    try {
      await createBranch(selectedRepositoryId, {
        name: branchDraft.name,
        base_branch: branchDraft.base_branch,
        protected: branchDraft.protected,
      });
      await loadRepositoryContext(selectedRepositoryId, selectedMergeRequestId || undefined, false);
      notifications.success(`Created branch ${branchDraft.name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create branch';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createCommitAction() {
    if (!selectedRepositoryId) {
      notifications.warning('Select a repository before committing');
      return;
    }
    setBusyAction('commit');
    try {
      await createCommit(selectedRepositoryId, {
        branch_name: commitDraft.branch_name,
        title: commitDraft.title,
        description: commitDraft.description,
        author_name: commitDraft.author_name,
        additions: Number(commitDraft.additions),
        deletions: Number(commitDraft.deletions),
      });
      await loadRepositoryContext(selectedRepositoryId, selectedMergeRequestId || undefined, false);
      notifications.success(`Created commit ${commitDraft.title} and queued branch CI`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create commit';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function triggerCiAction() {
    if (!selectedRepositoryId) {
      notifications.warning('Select a repository before triggering CI');
      return;
    }
    setBusyAction('ci');
    try {
      const run = await triggerCiRun(selectedRepositoryId, { branch_name: commitDraft.branch_name });
      setCiRuns((current) => [run, ...current]);
      notifications.success(`Triggered ${run.pipeline_name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to trigger CI';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runSearchAction() {
    if (!selectedRepositoryId) {
      notifications.warning('Select a repository before searching files');
      return;
    }
    setBusyAction('search');
    try {
      const response = await searchFiles(selectedRepositoryId, searchQuery);
      setSearchResults(response.results);
      notifications.success(`Found ${response.results.length} matches`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to search repository files';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createMergeRequestAction() {
    if (!selectedRepositoryId) {
      notifications.warning('Select a repository before opening a merge request');
      return;
    }
    setBusyAction('create-mr');
    try {
      const reviewers: ReviewerState[] = parseCsv(mergeRequestDraft.reviewers_text).map((reviewer) => ({
        reviewer,
        approved: false,
        state: 'pending',
      }));
      const mergeRequest = await createMergeRequest({
        repository_id: selectedRepositoryId,
        title: mergeRequestDraft.title,
        description: mergeRequestDraft.description,
        source_branch: mergeRequestDraft.source_branch,
        target_branch: mergeRequestDraft.target_branch,
        author: mergeRequestDraft.author,
        labels: parseCsv(mergeRequestDraft.labels_text),
        reviewers,
        approvals_required: Number(mergeRequestDraft.approvals_required),
        changed_files: Number(mergeRequestDraft.changed_files),
      });
      await loadRepositoryContext(selectedRepositoryId, mergeRequest.id, false);
      notifications.success(`Opened ${mergeRequest.title}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create merge request';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function changeMergeRequestStatus(status: MergeRequestStatus) {
    if (!selectedMergeRequestId) return;
    setBusyAction('mr-status');
    try {
      await updateMergeRequest(selectedMergeRequestId, { status });
      await loadRepositoryContext(selectedRepositoryId, selectedMergeRequestId, false);
      notifications.success(`Marked merge request as ${status}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to update merge request';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function updateReviewerState(reviewerName: string, approved: boolean, state: string) {
    if (!mergeRequestDetail) return;
    setBusyAction('mr-review');
    try {
      const reviewers = mergeRequestDetail.merge_request.reviewers.map((reviewer) =>
        reviewer.reviewer === reviewerName ? { ...reviewer, approved, state } : reviewer,
      );
      await updateMergeRequest(mergeRequestDetail.merge_request.id, { reviewers });
      await loadRepositoryContext(selectedRepositoryId, selectedMergeRequestId, false);
      notifications.success(`Updated review state for ${reviewerName}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to update reviewer state';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function mergeSelectedMergeRequest() {
    if (!selectedMergeRequestId || !mergeRequestDetail) return;
    setBusyAction('merge-mr');
    try {
      const result = await mergeMergeRequest(selectedMergeRequestId, {
        merged_by: commentDraft.author || mergeRequestDetail.merge_request.author,
      });
      await loadRepositoryContext(selectedRepositoryId, selectedMergeRequestId, false);
      notifications.success(`Merged into ${result.target_branch} at ${result.merge_commit_sha.slice(0, 8)}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to merge merge request';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createCommentAction() {
    if (!selectedMergeRequestId) {
      notifications.warning('Select a merge request before adding comments');
      return;
    }
    setBusyAction('comment');
    try {
      await createComment(selectedMergeRequestId, {
        author: commentDraft.author,
        body: commentDraft.body,
        file_path: commentDraft.file_path,
        line_number: commentDraft.line_number ? Number(commentDraft.line_number) : undefined,
        resolved: commentDraft.resolved,
      });
      await selectMergeRequest(selectedMergeRequestId, false);
      notifications.success('Added review comment');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create review comment';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  function selectFile(path: string) {
    setSelectedFilePath(path);
    setCommentDraft((current) => ({ ...current, file_path: path }));
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <section
        style={{
          overflow: 'hidden',
          borderRadius: 32,
          padding: 24,
          color: '#f8fafc',
          background: 'linear-gradient(135deg, #082f49 0%, #1c1917 50%, #4a044e 100%)',
        }}
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.28em', color: '#7dd3fc' }}>
              Code Repositories
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 12, color: '#f8fafc' }}>
              Object-backed repos, branches, commits, CI, and merge reviews
            </h1>
            <p style={{ marginTop: 12, fontSize: 13, lineHeight: 1.6, color: 'rgba(248, 250, 252, 0.85)' }}>
              Operate the full repository lifecycle from one workspace: define repos, push commits, run search,
              open merge requests, and gate merges with branch protection + CI policy.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(2, 1fr)' }}>
            <div style={{ borderRadius: 16, background: 'rgba(255,255,255,0.1)', padding: 12 }}>
              <p style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#7dd3fc' }}>Repos</p>
              <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.repository_count ?? 0}</p>
            </div>
            <div style={{ borderRadius: 16, background: 'rgba(255,255,255,0.1)', padding: 12 }}>
              <p style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#7dd3fc' }}>Open MRs</p>
              <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{overview?.open_merge_request_count ?? 0}</p>
            </div>
            <div style={{ borderRadius: 16, background: 'rgba(255,255,255,0.1)', padding: 12 }}>
              <p style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#7dd3fc' }}>Files</p>
              <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{files.length}</p>
            </div>
            <div style={{ borderRadius: 16, background: 'rgba(255,255,255,0.1)', padding: 12 }}>
              <p style={{ fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#7dd3fc' }}>CI Runs</p>
              <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{ciRuns.length}</p>
            </div>
          </div>
        </div>
      </section>

      {uiError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {uiError}
        </div>
      )}

      <RepoExplorer
        overview={overview}
        repositories={repositories}
        selectedRepositoryId={selectedRepositoryId}
        draft={repositoryDraft}
        busy={busy}
        onSelectRepository={(id) => void selectRepository(id)}
        onDraftChange={(patch) => setRepositoryDraft((current) => ({ ...current, ...patch }))}
        onSave={() => void saveRepository()}
        onReset={() => {
          setSelectedRepositoryId('');
          setRepositoryDraft(emptyRepoDraft());
        }}
      />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.02fr) minmax(0, 0.98fr)' }}>
        <FileViewer
          files={files}
          selectedFilePath={selectedFilePath}
          searchQuery={searchQuery}
          searchResults={searchResults}
          busy={busy}
          onSelectFile={selectFile}
          onSearchQueryChange={(query) => setSearchQuery(query)}
          onRunSearch={() => void runSearchAction()}
        />
        <DiffViewer
          availableBranches={branchOptions}
          branchName={diffBranch}
          patch={diffPatch}
          busy={busy}
          onSelectBranch={(branch) => void refreshDiff(branch)}
        />
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.92fr) minmax(0, 1.08fr)' }}>
        <BranchManager
          branches={branches}
          draft={branchDraft}
          busy={busy}
          onDraftChange={(patch) => setBranchDraft((current) => ({ ...current, ...patch }))}
          onCreateBranch={() => void createBranchAction()}
        />
        <CommitHistory
          branches={branches}
          commits={commits}
          ciRuns={ciRuns}
          draft={commitDraft}
          busy={busy}
          onDraftChange={(patch) => setCommitDraft((current) => ({ ...current, ...patch }))}
          onCreateCommit={() => void createCommitAction()}
          onTriggerCi={() => void triggerCiAction()}
        />
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.92fr) minmax(0, 1.08fr)' }}>
        <MergeRequestList
          mergeRequests={mergeRequests}
          selectedMergeRequestId={selectedMergeRequestId}
          branchOptions={branchOptions}
          draft={mergeRequestDraft}
          busy={busy}
          onSelectMergeRequest={(id) => void selectMergeRequest(id)}
          onDraftChange={(patch) => setMergeRequestDraft((current) => ({ ...current, ...patch }))}
          onCreateMergeRequest={() => void createMergeRequestAction()}
        />
        <MergeRequestDetail
          detail={mergeRequestDetail}
          draft={commentDraft}
          busy={busy}
          mergeBlockers={mergeBlockers(mergeRequestDetail)}
          latestSourceCi={
            mergeRequestDetail ? latestCiRunForBranch(mergeRequestDetail.merge_request.source_branch) : null
          }
          targetBranchProtected={
            mergeRequestDetail
              ? Boolean(
                  targetBranch(mergeRequestDetail.merge_request.target_branch)?.protected ??
                    (mergeRequestDetail.merge_request.target_branch === currentRepository?.default_branch),
                )
              : false
          }
          onDraftChange={(patch) => setCommentDraft((current) => ({ ...current, ...patch }))}
          onCreateComment={() => void createCommentAction()}
          onStatusChange={(status) => void changeMergeRequestStatus(status)}
          onReviewerStateChange={(reviewer, approved, state) => void updateReviewerState(reviewer, approved, state)}
          onMerge={() => void mergeSelectedMergeRequest()}
        />
      </div>
    </section>
  );
}
