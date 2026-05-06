import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { searchOntology, type SearchResult } from '@/lib/api/ontology';

const KIND_LABELS: Record<string, string> = {
  object_type: 'Object types',
  interface: 'Interfaces',
  link_type: 'Link types',
  action_type: 'Actions',
  object_instance: 'Objects',
  object: 'Objects',
  link: 'Links',
  action: 'Actions',
};

const KIND_ORDER = [
  'object_type', 'interface', 'link_type', 'action_type', 'object_instance', 'object', 'link', 'action',
];

interface OntologySearchProps {
  open: boolean;
  initialQuery?: string;
  onClose: () => void;
}

export function OntologySearch({ open, initialQuery = '', onClose }: OntologySearchProps) {
  const [query, setQuery] = useState(initialQuery);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [highlight, setHighlight] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const navigate = useNavigate();

  useEffect(() => {
    if (open) {
      setQuery(initialQuery);
      setResults([]);
      setHighlight(0);
      setError('');
      setTimeout(() => inputRef.current?.focus(), 0);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialQuery]);

  useEffect(() => {
    if (!open) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    if (!query.trim()) { setResults([]); return; }
    debounceRef.current = setTimeout(async () => {
      setLoading(true);
      setError('');
      try {
        const res = await searchOntology({ query: query.trim(), limit: 50 });
        setResults(res.data);
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : String(cause));
      } finally {
        setLoading(false);
      }
    }, 180);
    return () => { if (debounceRef.current) clearTimeout(debounceRef.current); };
  }, [query, open]);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') { e.preventDefault(); onClose(); }
      else if (e.key === 'ArrowDown') { e.preventDefault(); setHighlight((h) => Math.min(h + 1, results.length - 1)); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); setHighlight((h) => Math.max(0, h - 1)); }
      else if (e.key === 'Enter' && results[highlight]) {
        e.preventDefault();
        navigate(results[highlight].route);
        onClose();
      }
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, results, highlight, navigate, onClose]);

  const groupedResults = useMemo(() => {
    const map = new Map<string, SearchResult[]>();
    for (const r of results) {
      const arr = map.get(r.kind) ?? [];
      arr.push(r);
      map.set(r.kind, arr);
    }
    const ordered: { kind: string; entries: SearchResult[] }[] = [];
    for (const k of KIND_ORDER) {
      const e = map.get(k);
      if (e) ordered.push({ kind: k, entries: e });
    }
    for (const [k, e] of map) {
      if (!KIND_ORDER.includes(k)) ordered.push({ kind: k, entries: e });
    }
    return ordered;
  }, [results]);

  if (!open) return null;

  let cursor = 0;

  return (
    <div role="presentation" onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(2,6,23,0.6)', zIndex: 100, display: 'flex', alignItems: 'flex-start', justifyContent: 'center', paddingTop: 100 }}>
      <div onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" style={{ width: 720, maxWidth: 'calc(100% - 32px)', background: '#0f172a', border: '1px solid #1e293b', borderRadius: 12, color: '#e2e8f0', overflow: 'hidden', boxShadow: '0 20px 50px rgba(0,0,0,0.5)' }}>
        <input
          ref={inputRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search the ontology…"
          style={{ width: '100%', padding: '14px 18px', background: 'transparent', border: 'none', color: 'inherit', fontSize: 15, outline: 'none', borderBottom: '1px solid #1e293b' }}
        />
        {loading && <p style={{ padding: 16, fontSize: 12, color: '#94a3b8', margin: 0 }}>Searching…</p>}
        {error && <p style={{ padding: 16, fontSize: 12, color: '#fca5a5', margin: 0 }}>{error}</p>}
        <div style={{ maxHeight: 420, overflow: 'auto' }}>
          {groupedResults.map((group) => (
            <section key={group.kind}>
              <p style={{ padding: '8px 18px', margin: 0, fontSize: 11, textTransform: 'uppercase', color: '#94a3b8', letterSpacing: '0.05em', background: '#020617' }}>
                {KIND_LABELS[group.kind] ?? group.kind}
              </p>
              <ul style={{ margin: 0, padding: 0, listStyle: 'none' }}>
                {group.entries.map((r) => {
                  const idx = cursor++;
                  const selected = idx === highlight;
                  return (
                    <li key={`${r.kind}-${r.id}`}>
                      <button
                        type="button"
                        onMouseEnter={() => setHighlight(idx)}
                        onClick={() => { navigate(r.route); onClose(); }}
                        style={{
                          width: '100%',
                          padding: '8px 18px',
                          textAlign: 'left',
                          background: selected ? '#1e293b' : 'transparent',
                          border: 'none',
                          color: 'inherit',
                          cursor: 'pointer',
                          fontSize: 13,
                        }}
                      >
                        <strong>{r.title || r.id}</strong>
                        {r.subtitle && <span style={{ marginLeft: 6, color: '#94a3b8', fontSize: 11 }}>· {r.subtitle}</span>}
                        {r.snippet && <p style={{ margin: 0, color: '#94a3b8', fontSize: 11 }}>{r.snippet}</p>}
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
          {!loading && !error && query.trim() && results.length === 0 && (
            <p style={{ padding: 16, fontSize: 12, color: '#94a3b8', margin: 0 }}>No matches.</p>
          )}
        </div>
      </div>
    </div>
  );
}
