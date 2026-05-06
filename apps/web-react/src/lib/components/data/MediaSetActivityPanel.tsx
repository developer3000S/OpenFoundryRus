import { useEffect, useState } from 'react';

import { listEvents, type AnomalyAlert, type AuditEvent, type ClassificationCatalogEntry, type ClassificationLevel } from '@/lib/api/audit';
import type { MediaSet } from '@/lib/api/mediaSets';
import { AuditLogViewer, type EventDraft, type EventFilterDraft } from '@/lib/components/audit/AuditLogViewer';

interface Props {
  mediaSet: MediaSet;
}

const SOURCE_SERVICE = 'media-sets-service';

const CLASSIFICATIONS: ClassificationCatalogEntry[] = [
  { classification: 'public', description: 'Public' },
  { classification: 'confidential', description: 'Confidential' },
  { classification: 'pii', description: 'PII' },
];

export function MediaSetActivityPanel({ mediaSet }: Props) {
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [anomalies, setAnomalies] = useState<AnomalyAlert[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [filters, setFilters] = useState<EventFilterDraft>({
    source_service: SOURCE_SERVICE,
    subject_id: '',
    classification: '',
  });

  const [draft, setDraft] = useState<EventDraft>({
    source_service: SOURCE_SERVICE,
    channel: 'audit',
    actor: '',
    action: '',
    resource_type: 'media_set',
    resource_id: mediaSet.rid,
    status: 'success',
    severity: 'low',
    classification: 'public' as ClassificationLevel,
    subject_id: '',
    ip_address: '',
    location: '',
    labels_text: '',
    metadata_text: '{}',
    retention_days: '365',
  });

  async function load() {
    setLoading(true);
    setError('');
    try {
      const response = await listEvents({
        source_service: SOURCE_SERVICE,
        subject_id: filters.subject_id || undefined,
        classification: filters.classification || undefined,
        resource_id: mediaSet.rid,
      });
      setEvents(response.items);
      setAnomalies(response.anomalies);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load audit events');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaSet.rid]);

  return (
    <section className="space-y-4">
      <header className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Activity</h2>
          <p className="mt-1 text-xs text-slate-500">
            Every action that touches this media set or its items lands in the audit warehouse via the Postgres outbox (ADR-0022). The list below is filtered to <code className="font-mono">resource_id = {mediaSet.rid}</code>.
          </p>
        </div>
        <button type="button" className="rounded-xl border border-slate-200 px-3 py-1.5 text-xs hover:bg-slate-50 dark:border-gray-700 dark:hover:bg-gray-800" onClick={() => void load()} disabled={loading}>
          {loading ? 'Refreshing…' : 'Refresh'}
        </button>
      </header>

      {error ? (
        <div className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700 dark:border-rose-900/40 dark:bg-rose-950/40 dark:text-rose-300">{error}</div>
      ) : anomalies.length > 0 ? (
        <ul className="space-y-1 text-xs">
          {anomalies.map((a) => (
            <li key={a.id} className="rounded-lg border border-amber-300 bg-amber-50 px-3 py-1.5 text-amber-800">
              {a.title} — {a.recommended_action}
            </li>
          ))}
        </ul>
      ) : null}

      <AuditLogViewer
        events={events}
        classifications={CLASSIFICATIONS}
        filters={filters}
        draft={draft}
        busy={loading}
        onFilterChange={(patch) => setFilters((prev) => ({ ...prev, ...patch, source_service: SOURCE_SERVICE }))}
        onApplyFilters={() => void load()}
        onDraftChange={(patch) => setDraft((prev) => ({ ...prev, ...patch }))}
        onAppendEvent={() => {}}
        showProbeForm={false}
      />
    </section>
  );
}
