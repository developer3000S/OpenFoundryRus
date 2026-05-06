import { useEffect, useMemo, useState } from 'react';

import { Glyph } from '@/lib/components/ui/Glyph';

export interface FolderNode {
  id: string;
  name: string;
  parent_folder_id: string | null;
}

interface FolderTreeProps {
  folders: FolderNode[];
  selectedId?: string | null;
  rootLabel?: string;
  onSelect?: (folderId: string | null) => void;
  onDrop?: (folderId: string | null) => void | Promise<void>;
  canDrop?: (folderId: string | null) => boolean;
}

interface TreeNode extends FolderNode { children: TreeNode[] }

export function FolderTree({ folders, selectedId = null, rootLabel = 'Project root', onSelect, onDrop, canDrop }: FolderTreeProps) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [dragOverId, setDragOverId] = useState<string | null | undefined>(undefined);

  const tree = useMemo<TreeNode[]>(() => {
    const byId = new Map<string, TreeNode>(folders.map((f) => [f.id, { ...f, children: [] }]));
    const roots: TreeNode[] = [];
    for (const node of byId.values()) {
      const parent = node.parent_folder_id ? byId.get(node.parent_folder_id) : undefined;
      if (parent) parent.children.push(node);
      else roots.push(node);
    }
    const sortRec = (list: TreeNode[]) => {
      list.sort((a, b) => a.name.localeCompare(b.name));
      for (const node of list) sortRec(node.children);
    };
    sortRec(roots);
    return roots;
  }, [folders]);

  useEffect(() => {
    if (!selectedId) return;
    const byId = new Map(folders.map((f) => [f.id, f]));
    let cursor: string | null = selectedId;
    setExpanded((prev) => {
      const next = new Set(prev);
      while (cursor) {
        const node = byId.get(cursor);
        if (!node) break;
        next.add(node.id);
        cursor = node.parent_folder_id;
      }
      return next;
    });
  }, [selectedId, folders]);

  function accepts(id: string | null) {
    if (!onDrop) return false;
    return canDrop ? canDrop(id) : true;
  }

  function toggle(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function renderRow(node: TreeNode, depth: number) {
    const isExpanded = expanded.has(node.id);
    const selected = selectedId === node.id;
    const isOver = dragOverId === node.id;
    return (
      <li key={node.id}>
        <div
          onClick={() => onSelect?.(node.id)}
          onDragOver={(e) => {
            if (!accepts(node.id)) return;
            e.preventDefault();
            if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
            setDragOverId(node.id);
          }}
          onDragLeave={() => { if (dragOverId === node.id) setDragOverId(undefined); }}
          onDrop={(e) => {
            if (!accepts(node.id)) return;
            e.preventDefault();
            setDragOverId(undefined);
            void onDrop?.(node.id);
          }}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            padding: '4px 8px',
            paddingLeft: 8 + depth * 16,
            cursor: 'pointer',
            background: selected ? '#1e293b' : isOver ? '#1d4ed8' : 'transparent',
            color: selected || isOver ? '#fff' : 'inherit',
            borderRadius: 4,
            fontSize: 12,
          }}
        >
          {node.children.length > 0 ? (
            <button type="button" onClick={(e) => { e.stopPropagation(); toggle(node.id); }} style={{ background: 'transparent', border: 'none', color: 'inherit', cursor: 'pointer', padding: 0 }}>
              <Glyph name={isExpanded ? 'chevron-down' : 'chevron-right'} size={12} />
            </button>
          ) : (
            <span style={{ display: 'inline-block', width: 12 }} />
          )}
          <Glyph name="folder" size={12} />
          <span>{node.name}</span>
        </div>
        {isExpanded && node.children.length > 0 && (
          <ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>
            {node.children.map((c) => renderRow(c, depth + 1))}
          </ul>
        )}
      </li>
    );
  }

  return (
    <nav aria-label="Folder tree" style={{ fontSize: 12 }}>
      <ul style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 2 }}>
        <li>
          <div
            onClick={() => onSelect?.(null)}
            onDragOver={(e) => { if (!accepts(null)) return; e.preventDefault(); setDragOverId(null); }}
            onDragLeave={() => { if (dragOverId === null) setDragOverId(undefined); }}
            onDrop={(e) => {
              if (!accepts(null)) return;
              e.preventDefault();
              setDragOverId(undefined);
              void onDrop?.(null);
            }}
            style={{
              padding: '4px 8px',
              cursor: 'pointer',
              background: selectedId === null ? '#1e293b' : dragOverId === null ? '#1d4ed8' : 'transparent',
              borderRadius: 4,
              fontWeight: 600,
            }}
          >
            <Glyph name="home" size={12} /> {rootLabel}
          </div>
        </li>
        {tree.map((n) => renderRow(n, 0))}
      </ul>
    </nav>
  );
}
