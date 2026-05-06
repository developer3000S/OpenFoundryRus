import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';

import {
  createKnowledgeDocument,
  listKnowledgeBases,
  type KnowledgeBase,
} from '@/lib/api/ai';
import {
  exportNotepadDocument,
  getNotepadDocument,
  listNotepadPresence,
  updateNotepadDocument,
  upsertNotepadPresence,
  type NotepadDocument,
  type NotepadExportPayload,
  type NotepadPresence,
} from '@/lib/api/notepad';
import { useCurrentUser } from '@stores/auth';

interface NotepadWidgetDraft {
  id?: string;
  kind: string;
  title: string;
  summary: string;
  source_ref: string;
}

function emptyWidgetDraft(): NotepadWidgetDraft {
  return { kind: 'contour', title: '', summary: '', source_ref: '' };
}

function documentWidgets(doc: NotepadDocument | null) {
  return Array.isArray(doc?.widgets) ? doc.widgets : [];
}

export function NotepadDetailPage() {
  const { id } = useParams<{ id: string }>();
  const documentId = id ?? '';
  const navigate = useNavigate();
  const user = useCurrentUser();

  const [doc, setDoc] = useState<NotepadDocument | null>(null);
  const [exportPayload, setExportPayload] = useState<NotepadExportPayload | null>(null);
  const [presence, setPresence] = useState<NotepadPresence[]>([]);
  const [knowledgeBases, setKnowledgeBases] = useState<KnowledgeBase[]>([]);
  const [selectedKnowledgeBaseId, setSelectedKnowledgeBaseId] = useState('');
  const [widgetDraft, setWidgetDraft] = useState<NotepadWidgetDraft>(emptyWidgetDraft());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [indexing, setIndexing] = useState(false);
  const [error, setError] = useState('');

  const sessionIdRef = useRef<string>(crypto.randomUUID?.() ?? Math.random().toString(36).slice(2));

  const sendPresence = useCallback(
    async (cursorLabel = 'editing document') => {
      if (!doc || !user) return;
      try {
        await upsertNotepadPresence(doc.id, {
          session_id: sessionIdRef.current,
          display_name: user.name,
          cursor_label: cursorLabel,
          color: '#0f766e',
        });
      } catch {
        // Presence should never block editing.
      }
    },
    [doc, user],
  );

  const refreshPresence = useCallback(async () => {
    if (!doc) return;
    try {
      const result = await listNotepadPresence(doc.id);
      setPresence(result.data);
    } catch {
      // Ignore transient polling failures.
    }
  }, [doc]);

  // Initial load.
  useEffect(() => {
    if (!documentId) return;
    let cancelled = false;
    setLoading(true);
    setError('');
    void (async () => {
      try {
        const [d, exp, pres, kbs] = await Promise.all([
          getNotepadDocument(documentId),
          exportNotepadDocument(documentId),
          listNotepadPresence(documentId),
          listKnowledgeBases().catch(() => ({ data: [] as KnowledgeBase[] })),
        ]);
        if (cancelled) return;
        setDoc(d);
        setExportPayload(exp);
        setPresence(pres.data);
        setKnowledgeBases(kbs.data);
        setSelectedKnowledgeBaseId(kbs.data[0]?.id ?? '');
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Failed to load document');
          setDoc(null);
          setExportPayload(null);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [documentId]);

  // Heartbeat + presence polling once the document is loaded.
  useEffect(() => {
    if (!doc) return;
    void sendPresence();
    const heartbeat = setInterval(() => void sendPresence('editing document'), 15_000);
    const polling = setInterval(() => void refreshPresence(), 12_000);
    return () => {
      clearInterval(heartbeat);
      clearInterval(polling);
    };
  }, [doc, sendPresence, refreshPresence]);

  function patchDoc(patch: Partial<NotepadDocument>) {
    setDoc((current) => (current ? { ...current, ...patch } : current));
  }

  async function saveDocument() {
    if (!doc) return;
    setSaving(true);
    setError('');
    try {
      const updated = await updateNotepadDocument(doc.id, {
        title: doc.title,
        description: doc.description,
        content: doc.content,
        widgets: doc.widgets,
      });
      setDoc(updated);
      const exp = await exportNotepadDocument(updated.id);
      setExportPayload(exp);
      await sendPresence('reviewing latest changes');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to save document');
    } finally {
      setSaving(false);
    }
  }

  function addWidget() {
    if (!doc || !widgetDraft.title.trim()) return;
    const next = {
      id: crypto.randomUUID?.() ?? Math.random().toString(36).slice(2),
      kind: widgetDraft.kind,
      title: widgetDraft.title.trim(),
      summary: widgetDraft.summary.trim() || 'Live widget reference attached to the document.',
      source_ref: widgetDraft.source_ref.trim() || null,
    };
    patchDoc({ widgets: [...documentWidgets(doc), next] });
    setWidgetDraft(emptyWidgetDraft());
  }

  function removeWidget(widgetId: string) {
    if (!doc) return;
    patchDoc({
      widgets: documentWidgets(doc).filter((widget) => String(widget.id ?? '') !== widgetId),
    });
  }

  async function indexInKnowledgeBase() {
    if (!doc || !selectedKnowledgeBaseId) return;
    setIndexing(true);
    setError('');
    try {
      await createKnowledgeDocument(selectedKnowledgeBaseId, {
        title: doc.title,
        content: [
          doc.content,
          '',
          ...documentWidgets(doc).map(
            (widget) => `- ${widget.title ?? 'Widget'}: ${widget.summary ?? ''}`,
          ),
        ].join('\n'),
        source_uri: `notepad://${doc.id}`,
        metadata: {
          source: 'notepad',
          widget_count: documentWidgets(doc).length,
        },
      });
      const updated = await updateNotepadDocument(doc.id, {
        last_indexed_at: new Date().toISOString(),
      });
      setDoc(updated);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to index document');
    } finally {
      setIndexing(false);
    }
  }

  function openPrintView() {
    if (!exportPayload) return;
    const windowRef = window.open('', '_blank', 'noopener,noreferrer');
    if (!windowRef) return;
    windowRef.document.write(exportPayload.html);
    windowRef.document.close();
    windowRef.focus();
    windowRef.print();
  }

  if (loading) {
    return (
      <section className="of-page" style={{ padding: 80, textAlign: 'center', color: 'var(--text-muted)' }}>
        Loading document…
      </section>
    );
  }

  if (!doc) {
    return (
      <section className="of-page" style={{ padding: 80, textAlign: 'center' }}>
        <h1 className="of-heading-lg">Document not found</h1>
        <Link to="/notepad" className="of-btn of-btn-primary" style={{ display: 'inline-flex', marginTop: 24 }}>
          Back to Notepad
        </Link>
      </section>
    );
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div style={{ maxWidth: 720, display: 'grid', gap: 12 }}>
            <Link to="/notepad" className="of-link" style={{ fontSize: 13 }}>
              ← Back to notepad
            </Link>
            <input
              type="text"
              value={doc.title}
              onChange={(e) => patchDoc({ title: e.target.value })}
              placeholder="Document title"
              style={{
                width: '100%',
                background: 'transparent',
                fontSize: 28,
                fontWeight: 700,
                letterSpacing: '-0.02em',
                color: 'var(--text-strong)',
                border: 0,
                outline: 'none',
              }}
            />
            <textarea
              rows={2}
              value={doc.description}
              onChange={(e) => patchDoc({ description: e.target.value })}
              placeholder="What should readers understand after opening this document?"
              style={{
                width: '100%',
                resize: 'none',
                background: 'transparent',
                fontSize: 14,
                lineHeight: 1.7,
                color: 'var(--text-muted)',
                border: 0,
                outline: 'none',
              }}
            />
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {doc.template_key && (
                <span className="of-chip" style={{ fontSize: 11 }}>
                  {doc.template_key}
                </span>
              )}
              <span className="of-chip" style={{ fontSize: 11 }}>
                {documentWidgets(doc).length} embeds
              </span>
              {doc.last_indexed_at && (
                <span className="of-chip of-status-success" style={{ fontSize: 11 }}>
                  Indexed in AIP
                </span>
              )}
            </div>
          </div>

          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            <button type="button" className="of-btn" onClick={() => navigate('/notepad')}>
              Close
            </button>
            <button type="button" className="of-btn" onClick={openPrintView}>
              Print
            </button>
            <button
              type="button"
              className="of-btn of-btn-primary"
              onClick={() => void saveDocument()}
              disabled={saving}
            >
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        </div>

        {error && (
          <div
            className="of-status-danger"
            style={{ marginTop: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
          >
            {error}
          </div>
        )}
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.1fr) minmax(380px, 0.9fr)' }}>
        <div style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ padding: 24 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <p className="of-eyebrow">Document body</p>
                <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                  Markdown-first collaborative note
                </h2>
              </div>
              <span className="of-text-muted" style={{ fontSize: 12 }}>
                {presence.length} active collaborators
              </span>
            </div>
            <textarea
              rows={24}
              value={doc.content}
              onChange={(e) => patchDoc({ content: e.target.value })}
              onFocus={() => void sendPresence('editing body')}
              placeholder={'# Narrative\n\nWrite the decision memo here.'}
              style={{
                marginTop: 16,
                minHeight: 520,
                width: '100%',
                borderRadius: 'var(--radius-md)',
                border: '1px solid var(--border-default)',
                background: 'var(--bg-panel-muted)',
                padding: 16,
                fontFamily: 'var(--font-mono)',
                fontSize: 13,
                lineHeight: 1.6,
                outline: 'none',
                resize: 'vertical',
              }}
            />
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Embeds</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Attach live workspace context
            </h2>

            <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 16 }}>
              <Field label="Kind">
                <select
                  className="of-select"
                  value={widgetDraft.kind}
                  onChange={(e) => setWidgetDraft((prev) => ({ ...prev, kind: e.target.value }))}
                >
                  <option value="contour">Contour</option>
                  <option value="quiver">Quiver</option>
                  <option value="report">Report</option>
                  <option value="fusion">Fusion</option>
                </select>
              </Field>
              <Field label="Title">
                <input
                  className="of-input"
                  value={widgetDraft.title}
                  onChange={(e) => setWidgetDraft((prev) => ({ ...prev, title: e.target.value }))}
                  placeholder="Executive trend board"
                />
              </Field>
              <Field label="Summary" fullWidth>
                <input
                  className="of-input"
                  value={widgetDraft.summary}
                  onChange={(e) => setWidgetDraft((prev) => ({ ...prev, summary: e.target.value }))}
                  placeholder="Why this widget matters in the narrative."
                />
              </Field>
              <Field label="Source reference" fullWidth>
                <input
                  className="of-input"
                  value={widgetDraft.source_ref}
                  onChange={(e) => setWidgetDraft((prev) => ({ ...prev, source_ref: e.target.value }))}
                  placeholder="/contour or report execution id"
                />
              </Field>
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: 16 }}>
              <button type="button" className="of-btn" onClick={addWidget}>
                Add embed
              </button>
            </div>

            <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
              {documentWidgets(doc).length === 0 ? (
                <div
                  style={{
                    border: '1px dashed var(--border-default)',
                    borderRadius: 'var(--radius-md)',
                    padding: '16px 16px',
                    fontSize: 13,
                    color: 'var(--text-muted)',
                  }}
                >
                  No embedded widgets yet.
                </div>
              ) : (
                documentWidgets(doc).map((widget) => (
                  <div key={String(widget.id ?? '')} className="of-panel-muted" style={{ padding: 16 }}>
                    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                      <div style={{ minWidth: 0 }}>
                        <p className="of-eyebrow" style={{ color: '#0e7490' }}>
                          {String(widget.kind ?? 'widget')}
                        </p>
                        <div style={{ marginTop: 4, fontSize: 14, fontWeight: 500, color: 'var(--text-strong)' }}>
                          {String(widget.title ?? 'Untitled widget')}
                        </div>
                        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                          {String(widget.summary ?? 'No summary.')}
                        </p>
                      </div>
                      <button
                        type="button"
                        className="of-btn"
                        onClick={() => removeWidget(String(widget.id ?? ''))}
                        style={{ minHeight: 28, fontSize: 11 }}
                      >
                        Remove
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>
          </section>
        </div>

        <aside style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Presence</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Who is in the document
            </h2>

            <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
              {presence.length === 0 ? (
                <div
                  style={{
                    border: '1px dashed var(--border-default)',
                    borderRadius: 'var(--radius-md)',
                    padding: '16px',
                    fontSize: 13,
                    color: 'var(--text-muted)',
                  }}
                >
                  No active collaborators right now.
                </div>
              ) : (
                presence.map((collaborator) => (
                  <div key={collaborator.id} className="of-panel-muted" style={{ padding: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                      <span
                        style={{
                          width: 12,
                          height: 12,
                          borderRadius: '50%',
                          background: collaborator.color,
                        }}
                      />
                      <div>
                        <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                          {collaborator.display_name}
                        </div>
                        <div className="of-text-muted" style={{ fontSize: 12 }}>
                          {collaborator.cursor_label || 'Browsing the document'}
                        </div>
                      </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">AIP Assist</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Index the document into knowledge
            </h2>

            <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
              <Field label="Knowledge base">
                <select
                  className="of-select"
                  value={selectedKnowledgeBaseId}
                  onChange={(e) => setSelectedKnowledgeBaseId(e.target.value)}
                >
                  {knowledgeBases.map((kb) => (
                    <option key={kb.id} value={kb.id}>
                      {kb.name}
                    </option>
                  ))}
                </select>
              </Field>
              <button
                type="button"
                className="of-btn of-btn-primary"
                disabled={!selectedKnowledgeBaseId || indexing}
                onClick={() => void indexInKnowledgeBase()}
              >
                {indexing ? 'Indexing…' : 'Index in AIP'}
              </button>
            </div>
          </section>

          <section className="of-panel" style={{ padding: 24 }}>
            <p className="of-eyebrow">Preview</p>
            <h2 className="of-heading-md" style={{ marginTop: 4 }}>
              Rendered export
            </h2>

            {exportPayload ? (
              <iframe
                title="Notepad preview"
                srcDoc={exportPayload.html}
                style={{
                  marginTop: 16,
                  height: 540,
                  width: '100%',
                  borderRadius: 'var(--radius-md)',
                  border: '1px solid var(--border-default)',
                  background: '#fff',
                }}
              />
            ) : (
              <div
                style={{
                  marginTop: 16,
                  border: '1px dashed var(--border-default)',
                  borderRadius: 'var(--radius-md)',
                  padding: '20px 16px',
                  fontSize: 13,
                  color: 'var(--text-muted)',
                }}
              >
                Save the document to refresh the rendered preview.
              </div>
            )}
          </section>
        </aside>
      </div>
    </section>
  );
}

interface FieldProps {
  label: string;
  children: React.ReactNode;
  fullWidth?: boolean;
}

function Field({ label, children, fullWidth }: FieldProps) {
  return (
    <label
      style={{ display: 'block', fontSize: 13, gridColumn: fullWidth ? '1 / -1' : undefined }}
    >
      <div className="of-eyebrow" style={{ marginBottom: 6 }}>
        {label}
      </div>
      {children}
    </label>
  );
}
