import { useEffect, useRef, useState } from 'react';

import { listGroups, listUsers, type GroupRecord, type UserProfile } from '@/lib/api/auth';

interface PrincipalPickerProps {
  kind: 'user' | 'group';
  value: string;
  onChange: (next: { id: string; label: string }) => void;
  placeholder?: string;
}

export function PrincipalPicker({ kind, value, onChange, placeholder = 'Search by name or email…' }: PrincipalPickerProps) {
  const [users, setUsers] = useState<UserProfile[]>([]);
  const [groups, setGroups] = useState<GroupRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [highlight, setHighlight] = useState(0);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      setLoading(true);
      setError('');
      try {
        if (kind === 'user') {
          setUsers(await listUsers(query ? { q: query, limit: 50 } : { limit: 50 }));
        } else {
          setGroups(await listGroups(query ? { q: query, limit: 50 } : { limit: 50 }));
        }
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        setLoading(false);
      }
    }, 200);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); };
  }, [kind, query]);

  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [open]);

  const items = kind === 'user'
    ? users.map((u) => ({ id: u.id, label: u.name, sublabel: u.email }))
    : groups.map((g) => ({ id: g.id, label: g.name, sublabel: g.description ?? '' }));

  return (
    <div ref={containerRef} style={{ position: 'relative' }}>
      <input
        value={query}
        onChange={(e) => { setQuery(e.target.value); setOpen(true); setHighlight(0); }}
        onFocus={() => setOpen(true)}
        onKeyDown={(e) => {
          if (!open) return;
          if (e.key === 'ArrowDown') { e.preventDefault(); setHighlight((h) => Math.min(h + 1, items.length - 1)); }
          else if (e.key === 'ArrowUp') { e.preventDefault(); setHighlight((h) => Math.max(0, h - 1)); }
          else if (e.key === 'Enter' && items[highlight]) {
            e.preventDefault();
            onChange({ id: items[highlight].id, label: items[highlight].label });
            setOpen(false);
          }
        }}
        placeholder={placeholder}
        className="of-input"
        style={{ width: '100%' }}
      />
      {value && !query && (
        <p className="of-text-muted" style={{ fontSize: 11, marginTop: 4 }}>Selected id: <code>{value}</code></p>
      )}
      {open && (
        <div style={{ position: 'absolute', top: '100%', left: 0, right: 0, zIndex: 30, marginTop: 4, background: '#0f172a', border: '1px solid #1e293b', borderRadius: 6, maxHeight: 240, overflow: 'auto' }}>
          {loading && <p style={{ padding: 12, fontSize: 12, color: '#94a3b8', margin: 0 }}>Searching…</p>}
          {error && <p style={{ padding: 12, fontSize: 12, color: '#fca5a5', margin: 0 }}>{error}</p>}
          {items.length === 0 && !loading && <p style={{ padding: 12, fontSize: 12, color: '#94a3b8', margin: 0 }}>No matches.</p>}
          {items.map((it, i) => (
            <button
              key={it.id}
              type="button"
              onMouseEnter={() => setHighlight(i)}
              onClick={() => { onChange({ id: it.id, label: it.label }); setOpen(false); setQuery(''); }}
              style={{
                width: '100%',
                textAlign: 'left',
                padding: '8px 12px',
                background: i === highlight ? '#1e293b' : 'transparent',
                border: 'none',
                color: 'inherit',
                cursor: 'pointer',
                fontSize: 13,
              }}
            >
              <strong>{it.label}</strong>
              {it.sublabel && <p style={{ margin: 0, color: '#94a3b8', fontSize: 11 }}>{it.sublabel}</p>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
