import { useEffect, useState } from 'react';

import type { KnowledgeBase, KnowledgeDocument, KnowledgeSearchResult } from '@/lib/api/ai';

export interface KnowledgeBaseDraft {
  id?: string;
  name: string;
  description: string;
  status: string;
  embedding_provider: string;
  chunking_strategy: string;
  tags_text: string;
}

export interface DocumentDraft {
  title: string;
  content: string;
  source_uri: string;
  metadata_text: string;
}

export interface SearchDraft {
  query: string;
  top_k: number;
  min_score: number;
}

interface Props {
  knowledgeBases: KnowledgeBase[];
  documents: KnowledgeDocument[];
  selectedKnowledgeBaseId: string;
  knowledgeBaseDraft: KnowledgeBaseDraft;
  documentDraft: DocumentDraft;
  searchDraft: SearchDraft;
  searchResults: KnowledgeSearchResult[];
  busy?: boolean;
  onSelectKnowledgeBase?: (knowledgeBaseId: string) => void;
  onKnowledgeBaseDraftChange?: (draft: KnowledgeBaseDraft) => void;
  onDocumentDraftChange?: (draft: DocumentDraft) => void;
  onSearchDraftChange?: (draft: SearchDraft) => void;
  onSaveKnowledgeBase?: () => void;
  onSaveDocument?: () => void;
  onSearch?: () => void;
  onResetKnowledgeBase?: () => void;
}

export function KnowledgeManager({
  knowledgeBases, documents, selectedKnowledgeBaseId,
  knowledgeBaseDraft, documentDraft, searchDraft, searchResults,
  busy = false,
  onSelectKnowledgeBase, onKnowledgeBaseDraftChange, onDocumentDraftChange, onSearchDraftChange,
  onSaveKnowledgeBase, onSaveDocument, onSearch, onResetKnowledgeBase,
}: Props) {
  const [kbDraft, setKbDraft] = useState<KnowledgeBaseDraft>(knowledgeBaseDraft);
  const [docDraft, setDocDraft] = useState<DocumentDraft>(documentDraft);
  const [searchD, setSearchD] = useState<SearchDraft>(searchDraft);

  useEffect(() => { setKbDraft(knowledgeBaseDraft); }, [knowledgeBaseDraft]);
  useEffect(() => { setDocDraft(documentDraft); }, [documentDraft]);
  useEffect(() => { setSearchD(searchDraft); }, [searchDraft]);

  function updateKb<K extends keyof KnowledgeBaseDraft>(key: K, value: KnowledgeBaseDraft[K]) {
    const next = { ...kbDraft, [key]: value };
    setKbDraft(next);
    onKnowledgeBaseDraftChange?.(next);
  }
  function updateDoc<K extends keyof DocumentDraft>(key: K, value: DocumentDraft[K]) {
    const next = { ...docDraft, [key]: value };
    setDocDraft(next);
    onDocumentDraftChange?.(next);
  }
  function updateSearch<K extends keyof SearchDraft>(key: K, value: SearchDraft[K]) {
    const next = { ...searchD, [key]: value };
    setSearchD(next);
    onSearchDraftChange?.(next);
  }

  return (
    <section className="rounded-[28px] border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-950">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.28em] text-slate-500">Knowledge Manager</div>
          <h2 className="mt-2 text-xl font-semibold text-slate-900 dark:text-slate-100">Index playbooks, upload docs, and test retrieval</h2>
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={() => onResetKnowledgeBase?.()} disabled={busy} className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-900">New KB</button>
          <button type="button" onClick={() => onSaveKnowledgeBase?.()} disabled={busy} className="rounded-full bg-slate-950 px-3 py-1.5 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-950">Save KB</button>
        </div>
      </div>

      <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]">
        <div className="space-y-3">
          {knowledgeBases.map((kb) => (
            <button
              key={kb.id}
              type="button"
              onClick={() => onSelectKnowledgeBase?.(kb.id)}
              className={`w-full rounded-2xl border px-4 py-3 text-left transition ${selectedKnowledgeBaseId === kb.id ? 'border-cyan-400 bg-cyan-50 dark:border-cyan-700 dark:bg-cyan-950/30' : 'border-slate-200 bg-slate-50 hover:border-slate-300 dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700'}`}
            >
              <div className="flex items-center justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{kb.name}</div>
                  <div className="mt-1 text-xs text-slate-500">{kb.document_count} docs • {kb.chunk_count} chunks</div>
                </div>
                <span className="rounded-full bg-white px-2 py-1 text-[11px] uppercase tracking-[0.2em] text-slate-500 dark:bg-slate-950">{kb.status}</span>
              </div>
            </button>
          ))}
        </div>

        <div className="grid gap-4">
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Name"><input className={inputCls} value={kbDraft.name} onChange={(e) => updateKb('name', e.target.value)} /></Field>
            <Field label="Embedding Provider"><input className={inputCls} value={kbDraft.embedding_provider} onChange={(e) => updateKb('embedding_provider', e.target.value)} /></Field>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <Field label="Description"><input className={inputCls} value={kbDraft.description} onChange={(e) => updateKb('description', e.target.value)} /></Field>
            <Field label="Chunk Strategy"><input className={inputCls} value={kbDraft.chunking_strategy} onChange={(e) => updateKb('chunking_strategy', e.target.value)} /></Field>
          </div>
          <Field label="Tags"><input className={inputCls} value={kbDraft.tags_text} onChange={(e) => updateKb('tags_text', e.target.value)} /></Field>

          <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
              <div className="flex items-center justify-between gap-3">
                <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Documents</div>
                <button type="button" onClick={() => onSaveDocument?.()} disabled={busy || !selectedKnowledgeBaseId} className="rounded-full border border-slate-300 px-3 py-1 text-xs text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-200 dark:hover:bg-slate-800">Add document</button>
              </div>
              <div className="mt-3 space-y-3">
                <input className={smallInputCls} value={docDraft.title} onChange={(e) => updateDoc('title', e.target.value)} placeholder="Incident Triage Checklist" />
                <input className={smallInputCls} value={docDraft.source_uri} onChange={(e) => updateDoc('source_uri', e.target.value)} placeholder="kb://source" />
                <textarea className={`${smallInputCls} h-24`} value={docDraft.content} onChange={(e) => updateDoc('content', e.target.value)} />
                <textarea className={`${smallInputCls} h-20`} value={docDraft.metadata_text} onChange={(e) => updateDoc('metadata_text', e.target.value)} />
              </div>
              <div className="mt-4 space-y-2">
                {documents.length === 0 ? (
                  <p className="text-sm text-slate-500">No documents loaded for this knowledge base.</p>
                ) : (
                  documents.map((d) => (
                    <div key={d.id} className="rounded-xl border border-slate-200 bg-white px-3 py-2 dark:border-slate-800 dark:bg-slate-950">
                      <div className="text-sm font-medium text-slate-900 dark:text-slate-100">{d.title}</div>
                      <div className="mt-1 text-xs text-slate-500">{d.chunk_count} chunks • {d.status}</div>
                    </div>
                  ))
                )}
              </div>
            </div>

            <div className="rounded-2xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-900">
              <div className="flex items-center justify-between gap-3">
                <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">Semantic Retrieval</div>
                <button type="button" onClick={() => onSearch?.()} disabled={busy || !selectedKnowledgeBaseId} className="rounded-full border border-cyan-300 px-3 py-1 text-xs text-cyan-700 hover:bg-cyan-50 dark:border-cyan-800 dark:text-cyan-300 dark:hover:bg-cyan-950/40">Run search</button>
              </div>
              <div className="mt-3 grid gap-3">
                <input className={smallInputCls} value={searchD.query} onChange={(e) => updateSearch('query', e.target.value)} placeholder="How should providers fail over?" />
                <div className="grid gap-3 md:grid-cols-2">
                  <input type="number" className={smallInputCls} value={String(searchD.top_k)} onChange={(e) => updateSearch('top_k', Number(e.target.value) || 4)} />
                  <input type="number" step={0.01} className={smallInputCls} value={String(searchD.min_score)} onChange={(e) => updateSearch('min_score', Number(e.target.value) || 0.55)} />
                </div>
              </div>
              <div className="mt-4 space-y-2">
                {searchResults.length === 0 ? (
                  <p className="text-sm text-slate-500">Search results will appear here.</p>
                ) : (
                  searchResults.map((r, i) => (
                    <div key={i} className="rounded-xl border border-slate-200 bg-white px-3 py-3 dark:border-slate-800 dark:bg-slate-950">
                      <div className="flex items-center justify-between gap-3">
                        <div className="text-sm font-medium text-slate-900 dark:text-slate-100">{r.document_title}</div>
                        <div className="text-xs text-cyan-700 dark:text-cyan-300">score {r.score.toFixed(2)}</div>
                      </div>
                      <p className="mt-2 text-sm leading-6 text-slate-600 dark:text-slate-300">{r.excerpt}</p>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900">
      <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-slate-500">{label}</div>
      <div className="mt-2">{children}</div>
    </label>
  );
}

const inputCls = 'w-full bg-transparent text-sm text-slate-900 outline-none dark:text-slate-100';
const smallInputCls = 'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm dark:border-slate-800 dark:bg-slate-950';
