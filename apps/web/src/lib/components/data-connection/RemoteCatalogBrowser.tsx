import { useEffect, useMemo, useRef, useState } from 'react';

import { virtualTables, type DiscoveredEntry } from '@/lib/api/virtual-tables';

interface Props {
  sourceRid: string;
  onSelect?: (entry: DiscoveredEntry) => void;
}

interface NodeState {
  entry: DiscoveredEntry;
  expanded: boolean;
  loading: boolean;
  error: string | null;
  children: NodeState[] | null;
}

const CACHE_TTL_MS = 60_000;

function iconFor(kind: DiscoveredEntry['kind']): string {
  switch (kind) {
    case 'database': return '🗄';
    case 'schema': return '📁';
    case 'table': return '🟦';
    case 'view': return '🪟';
    case 'materialized_view': return '🟪';
    case 'iceberg_namespace': return '🧊';
    case 'iceberg_table': return '❄️';
    case 'file_prefix': return '🗂';
    default: return '·';
  }
}

function toNode(entry: DiscoveredEntry): NodeState {
  return { entry, expanded: false, loading: false, error: null, children: null };
}

export function RemoteCatalogBrowser({ sourceRid, onSelect }: Props) {
  const [roots, setRoots] = useState<NodeState[]>([]);
  const [rootError, setRootError] = useState('');
  const [rootLoading, setRootLoading] = useState(false);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cacheRef = useRef<Map<string, { entries: DiscoveredEntry[]; cachedAt: number }>>(new Map());

  const cacheKey = (path: string | undefined) => `${sourceRid}::${path ?? ''}`;

  async function fetchEntries(path?: string): Promise<DiscoveredEntry[]> {
    const key = cacheKey(path);
    const hit = cacheRef.current.get(key);
    if (hit && Date.now() - hit.cachedAt < CACHE_TTL_MS) return hit.entries;
    const response = await virtualTables.discoverRemoteCatalog(sourceRid, path);
    cacheRef.current.set(key, { entries: response.data, cachedAt: Date.now() });
    return response.data;
  }

  async function loadRoots() {
    setRootLoading(true);
    setRootError('');
    try {
      const entries = await fetchEntries();
      setRoots(entries.map(toNode));
    } catch (err) {
      setRootError(err instanceof Error ? err.message : 'Failed to load catalog');
    } finally {
      setRootLoading(false);
    }
  }

  useEffect(() => {
    if (sourceRid) void loadRoots();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sourceRid]);

  function onSearchInput(value: string) {
    setSearch(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => setDebouncedSearch(value.trim().toLowerCase()), 300);
  }

  const matches = useMemo(() => (node: NodeState): boolean => {
    if (!debouncedSearch) return true;
    return node.entry.display_name.toLowerCase().includes(debouncedSearch);
  }, [debouncedSearch]);

  async function toggle(node: NodeState, parent: NodeState[] | null) {
    const next = (n: NodeState): NodeState => {
      if (n !== node) return n;
      if (n.expanded) return { ...n, expanded: false };
      return { ...n, expanded: true };
    };
    const update = (list: NodeState[] | null): NodeState[] => (list ?? []).map((n) => {
      if (n === node) return next(n);
      if (n.children) return { ...n, children: update(n.children) };
      return n;
    });
    setRoots((prev) => update(prev));
    if (node.expanded) return;
    if (node.children !== null) return;

    void parent; // unused
    try {
      const children = await fetchEntries(node.entry.path);
      const childNodes = children.map(toNode);
      setRoots((prev) => {
        const apply = (list: NodeState[]): NodeState[] => list.map((n) => {
          if (n.entry.path === node.entry.path) return { ...n, expanded: true, loading: false, error: null, children: childNodes };
          if (n.children) return { ...n, children: apply(n.children) };
          return n;
        });
        return apply(prev);
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to load children';
      setRoots((prev) => {
        const apply = (list: NodeState[]): NodeState[] => list.map((n) => {
          if (n.entry.path === node.entry.path) return { ...n, loading: false, error: msg };
          if (n.children) return { ...n, children: apply(n.children) };
          return n;
        });
        return apply(prev);
      });
    }
  }

  function select(node: NodeState) {
    setSelectedPath(node.entry.path);
    onSelect?.(node.entry);
  }

  function renderBranch(node: NodeState, depth: number, idx: number): React.ReactElement {
    const isLeaf = node.entry.kind === 'table' || node.entry.kind === 'iceberg_table';
    return (
      <li key={node.entry.path || `n-${depth}-${idx}`} role="treeitem" aria-expanded={node.expanded}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 4, paddingRight: 8, paddingLeft: depth * 12 + 4, background: node.entry.path === selectedPath ? '#ecfeff' : undefined }}>
          <button
            type="button"
            aria-label={node.expanded ? 'Collapse' : 'Expand'}
            onClick={() => void toggle(node, null)}
            disabled={isLeaf}
            style={{ width: 20, background: 'transparent', border: 'none', cursor: isLeaf ? 'default' : 'pointer', color: '#6b7280', opacity: isLeaf ? 0.3 : 1 }}
          >
            {node.expanded ? '▾' : '▸'}
          </button>
          <button type="button" onClick={() => select(node)} style={{ flex: 1, textAlign: 'left', background: 'transparent', border: 'none', cursor: 'pointer', display: 'flex', gap: 8, alignItems: 'center', padding: '4px 0', fontSize: 14 }}>
            <span style={{ width: 16 }} aria-hidden="true">{iconFor(node.entry.kind)}</span>
            <span>{node.entry.display_name}</span>
            {node.entry.registrable && (
              <span title={node.entry.inferred_table_type ?? ''} style={{ marginLeft: 'auto', fontSize: 10, background: '#ecfdf5', color: '#047857', padding: '1px 6px', borderRadius: 4, border: '1px solid #6ee7b7' }}>registrable</span>
            )}
          </button>
        </div>
        {node.expanded && (
          node.loading ? (
            <div style={{ padding: '8px 12px', fontSize: 14, color: '#4b5563', paddingLeft: depth * 12 + 28 }}>Loading…</div>
          ) : node.error ? (
            <div style={{ padding: '8px 12px', fontSize: 14, color: '#b91c1c', paddingLeft: depth * 12 + 28 }}>{node.error}</div>
          ) : node.children ? (
            <ul role="group" style={{ listStyle: 'none', padding: 0, margin: 0 }}>
              {node.children.filter(matches).map((child, j) => renderBranch(child, depth + 1, j))}
            </ul>
          ) : null
        )}
      </li>
    );
  }

  return (
    <div style={{ border: '1px solid #e5e7eb', borderRadius: 8, background: '#fff', display: 'flex', flexDirection: 'column', minHeight: 360, maxHeight: 520 }}>
      <div style={{ borderBottom: '1px solid #e5e7eb', padding: 8 }}>
        <input type="text" placeholder="Filter…" value={search} onChange={(e) => onSearchInput(e.target.value)} style={{ width: '100%', padding: '6px 8px', fontSize: 14, border: '1px solid #d1d5db', borderRadius: 4 }} />
      </div>
      {rootLoading ? (
        <div style={{ padding: '8px 12px', fontSize: 14, color: '#4b5563' }}>Loading remote catalog…</div>
      ) : rootError ? (
        <div role="alert" style={{ padding: '8px 12px', fontSize: 14, color: '#b91c1c' }}>{rootError}</div>
      ) : roots.length === 0 ? (
        <div style={{ padding: '8px 12px', fontSize: 14, color: '#4b5563' }}>No entries.</div>
      ) : (
        <ul role="tree" style={{ listStyle: 'none', padding: '4px 0', margin: 0, overflow: 'auto', flex: 1 }}>
          {roots.filter(matches).map((n, i) => renderBranch(n, 0, i))}
        </ul>
      )}
    </div>
  );
}
