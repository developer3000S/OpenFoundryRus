import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';

export interface CommandAction {
  id: string;
  label: string;
  hint?: string;
  shortcut?: string;
  run: () => void | Promise<void>;
}

export function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement | null>(null);
  const navigate = useNavigate();

  const staticCommands: CommandAction[] = useMemo(
    () => [
      { id: 'builds.open', label: 'Open builds', hint: 'Cross-pipeline build queue', shortcut: 'g b', run: () => navigate('/builds') },
      { id: 'builds.run', label: 'Run build', hint: 'Open the Run-build modal in /builds', shortcut: 'r', run: () => navigate('/builds?run=1') },
      { id: 'pipelines.open', label: 'Open Pipeline Builder', hint: 'Browse and edit pipelines', run: () => navigate('/pipelines') },
      { id: 'datasets.open', label: 'Open datasets', hint: 'Catalog browser', run: () => navigate('/datasets') },
      { id: 'ontology.open', label: 'Open ontology', hint: 'Object types + semantic search', run: () => navigate('/ontology') },
      { id: 'settings.open', label: 'Open settings', hint: 'Identity, RBAC, MFA, API keys, SSO', run: () => navigate('/settings') },
    ],
    [navigate],
  );

  function dynamicCommands(q: string): CommandAction[] {
    const trimmed = q.trim();
    if (!trimmed) return [];
    const result: CommandAction[] = [];
    if (/^ri\.foundry\.main\.build\.[a-f0-9-]+/i.test(trimmed)) {
      result.push({
        id: `builds.view.${trimmed}`,
        label: `View build ${trimmed}`,
        hint: '/builds/{rid}',
        run: () => navigate(`/builds/${encodeURIComponent(trimmed)}`),
      });
    } else if (/^[a-f0-9-]{8,}$/i.test(trimmed)) {
      const rid = `ri.foundry.main.build.${trimmed}`;
      result.push({
        id: `builds.view.${trimmed}`,
        label: `View build ${rid}`,
        hint: 'Treat the suffix as a build UUID',
        run: () => navigate(`/builds/${encodeURIComponent(rid)}`),
      });
    }
    return result;
  }

  const allCommands = useMemo(() => {
    const q = query.trim().toLowerCase();
    return [
      ...staticCommands.filter((c) => !q || c.label.toLowerCase().includes(q)),
      ...dynamicCommands(query),
    ];
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, staticCommands]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const cmdKey = e.metaKey || e.ctrlKey;
      if (cmdKey && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setOpen((o) => !o);
        return;
      }
      if (e.key === 'Escape') {
        setOpen(false);
        setQuery('');
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => {
    if (open) {
      const id = setTimeout(() => inputRef.current?.focus(), 0);
      return () => clearTimeout(id);
    }
  }, [open]);

  if (!open) return null;

  async function runFirst() {
    const first = allCommands[0];
    if (!first) return;
    setOpen(false);
    setQuery('');
    await first.run();
  }

  return (
    <div
      role="presentation"
      onClick={() => setOpen(false)}
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(2,6,23,0.6)',
        zIndex: 100,
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'center',
        paddingTop: 100,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        style={{
          width: 560,
          maxWidth: 'calc(100% - 32px)',
          background: '#0f172a',
          border: '1px solid #1e293b',
          borderRadius: 12,
          color: '#e2e8f0',
          overflow: 'hidden',
          boxShadow: '0 20px 50px rgba(0,0,0,0.5)',
        }}
      >
        <input
          ref={inputRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              void runFirst();
            }
          }}
          placeholder="Type a command, route, or build RID…"
          style={{
            width: '100%',
            padding: '14px 18px',
            background: 'transparent',
            border: 'none',
            color: 'inherit',
            fontSize: 15,
            outline: 'none',
            borderBottom: '1px solid #1e293b',
          }}
        />
        <ul style={{ margin: 0, padding: 0, listStyle: 'none', maxHeight: 360, overflow: 'auto' }}>
          {allCommands.map((cmd, idx) => (
            <li key={cmd.id}>
              <button
                type="button"
                onClick={async () => {
                  setOpen(false);
                  setQuery('');
                  await cmd.run();
                }}
                style={{
                  width: '100%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '10px 18px',
                  background: idx === 0 ? '#1e293b' : 'transparent',
                  border: 'none',
                  color: 'inherit',
                  cursor: 'pointer',
                  textAlign: 'left',
                  fontSize: 13,
                }}
              >
                <span>
                  <strong>{cmd.label}</strong>
                  {cmd.hint && <span style={{ color: '#94a3b8', marginLeft: 8 }}>· {cmd.hint}</span>}
                </span>
                {cmd.shortcut && (
                  <span style={{ fontSize: 10, color: '#94a3b8', fontFamily: 'var(--font-mono)' }}>{cmd.shortcut}</span>
                )}
              </button>
            </li>
          ))}
          {allCommands.length === 0 && <li style={{ padding: 16, color: '#94a3b8', fontSize: 12 }}>No matches.</li>}
        </ul>
      </div>
    </div>
  );
}
