import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import {
  createNotepadDocument,
  deleteNotepadDocument,
  listNotepadDocuments,
  type NotepadDocument,
} from '@/lib/api/notepad';

interface Template {
  key: string;
  name: string;
  description: string;
  content: string;
  widgets: Array<Record<string, unknown>>;
}

const TEMPLATES: Template[] = [
  {
    key: 'executive-brief',
    name: 'Executive Brief',
    description: 'One-page summary with highlights, decisions, and next moves.',
    content: `# Executive brief

## Situation
- Summarize the current state in plain language.

## What changed
- Highlight the biggest movement.

## Decisions
- Record approvals, blockers, and owners.

## Next week
- List the actions that need to happen next.`,
    widgets: [
      {
        kind: 'contour',
        title: 'Top-down trend',
        summary: 'Embed a Contour board or exported insight snapshot.',
      },
    ],
  },
  {
    key: 'investigation',
    name: 'Investigation',
    description: 'Evidence-first writeup with hypotheses and findings.',
    content: `# Investigation log

## Hypothesis
- State the working theory.

## Evidence
- Capture the signals that support or contradict it.

## Findings
- List the confirmed facts.

## Follow-up
- Record the next analysis steps.`,
    widgets: [
      {
        kind: 'quiver',
        title: 'Object/time-series lens',
        summary: 'Attach Quiver object analytics and relationship snapshots.',
      },
    ],
  },
  {
    key: 'operating-review',
    name: 'Operating Review',
    description: 'Recurring operating cadence with metrics, narrative, and actions.',
    content: `# Operating review

## KPI pulse
- Describe the current business pulse.

## Risks
- Call out material risks.

## Opportunities
- Capture upside and experiments.

## Commitments
- Make ownership explicit.`,
    widgets: [
      {
        kind: 'report',
        title: 'Scheduled report',
        summary: 'Link the latest report execution or exported deck.',
      },
      {
        kind: 'fusion',
        title: 'Spreadsheet decision log',
        summary: 'Reference Fusion edits and reconciliations.',
      },
    ],
  },
];

export function NotepadListPage() {
  const navigate = useNavigate();
  const [documents, setDocuments] = useState<NotepadDocument[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try {
      const response = await listNotepadDocuments({
        search: search.trim() || undefined,
        per_page: 100,
      });
      setDocuments(response.data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load notepad documents');
      setDocuments([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function createFromTemplate(template: Template) {
    setCreating(true);
    try {
      const document = await createNotepadDocument({
        title: template.name,
        description: template.description,
        content: template.content,
        template_key: template.key,
        widgets: template.widgets,
      });
      navigate(`/notepad/${document.id}`);
    } finally {
      setCreating(false);
    }
  }

  async function createBlankDocument() {
    setCreating(true);
    try {
      const document = await createNotepadDocument({
        title: 'Untitled document',
        content: '# New document\n\nStart writing here.',
        widgets: [],
      });
      navigate(`/notepad/${document.id}`);
    } finally {
      setCreating(false);
    }
  }

  async function removeDocument(id: string) {
    if (!window.confirm('Delete this notepad document?')) return;
    await deleteNotepadDocument(id);
    await load();
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#0e7490' }}>
              Notepad
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Collaborative documents with live workspace embeds
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              Capture narrative, decisions, and evidence in one place, then export or index the
              document into AIP knowledge.
            </p>
          </div>
          <button
            type="button"
            className="of-btn of-btn-primary"
            onClick={() => void createBlankDocument()}
            disabled={creating}
          >
            {creating ? 'Creating…' : 'New document'}
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.1fr) minmax(320px, 0.9fr)' }}>
        <section className="of-panel" style={{ padding: 24 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
            <div>
              <p className="of-eyebrow">Documents</p>
              <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                Persistent operating notes
              </h2>
            </div>
            <input
              className="of-input"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void load();
              }}
              placeholder="Search title or description"
              style={{ width: 320 }}
            />
          </div>

          {error && (
            <div
              className="of-status-danger"
              style={{ marginTop: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
            >
              {error}
            </div>
          )}

          {loading ? (
            <p className="of-text-muted" style={{ marginTop: 32, fontSize: 13 }}>
              Loading documents…
            </p>
          ) : documents.length === 0 ? (
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
              No documents yet. Start from a template or create a blank note.
            </div>
          ) : (
            <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
              {documents.map((doc) => (
                <div key={doc.id} className="of-panel-muted" style={{ padding: 16 }}>
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                    <div style={{ minWidth: 0 }}>
                      <Link
                        to={`/notepad/${doc.id}`}
                        style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-strong)', textDecoration: 'none' }}
                      >
                        {doc.title}
                      </Link>
                      <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                        {doc.description || 'No description yet.'}
                      </p>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
                        {doc.template_key && (
                          <span className="of-chip" style={{ fontSize: 11 }}>
                            {doc.template_key}
                          </span>
                        )}
                        <span className="of-chip" style={{ fontSize: 11 }}>
                          {doc.widgets.length} embeds
                        </span>
                        {doc.last_indexed_at && (
                          <span className="of-chip of-status-success" style={{ fontSize: 11 }}>
                            Indexed in AIP
                          </span>
                        )}
                      </div>
                    </div>
                    <button
                      type="button"
                      className="of-btn of-btn-danger"
                      onClick={() => void removeDocument(doc.id)}
                      style={{ minHeight: 30, fontSize: 12 }}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>

        <section className="of-panel" style={{ padding: 24 }}>
          <p className="of-eyebrow">Templates</p>
          <h2 className="of-heading-md" style={{ marginTop: 4 }}>
            Start from a structured playbook
          </h2>

          <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
            {TEMPLATES.map((template) => (
              <button
                key={template.key}
                type="button"
                onClick={() => void createFromTemplate(template)}
                disabled={creating}
                style={{
                  width: '100%',
                  textAlign: 'left',
                  padding: 16,
                  border: '1px solid var(--border-default)',
                  borderRadius: 'var(--radius-md)',
                  background: 'var(--bg-panel-muted)',
                  cursor: creating ? 'not-allowed' : 'pointer',
                  opacity: creating ? 0.6 : 1,
                }}
              >
                <div style={{ fontSize: 14, fontWeight: 600, color: 'var(--text-strong)' }}>
                  {template.name}
                </div>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                  {template.description}
                </p>
                <div className="of-text-muted" style={{ marginTop: 12, fontSize: 11 }}>
                  {template.widgets.length} starter embeds
                </div>
              </button>
            ))}
          </div>
        </section>
      </div>
    </section>
  );
}
