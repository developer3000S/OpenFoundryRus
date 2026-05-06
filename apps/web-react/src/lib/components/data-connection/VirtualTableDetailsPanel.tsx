import { useState } from 'react';

import {
  capabilityChips,
  virtualTables,
  type Capabilities,
  type UpdateDetectionPollResult,
  type VirtualTable,
} from '@/lib/api/virtual-tables';

interface Props {
  table: VirtualTable;
  onChanged?: (next: VirtualTable) => void;
}

const PUSHDOWN_LABELS: Record<string, string> = {
  ibis: 'Ibis',
  pyspark: 'PySpark',
  snowpark: 'Snowpark',
};

export function VirtualTableDetailsPanel({ table, onChanged }: Props) {
  const [busy, setBusy] = useState<'toggle' | 'poll' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lastPoll, setLastPoll] = useState<UpdateDetectionPollResult | null>(null);
  const [intervalSeconds, setIntervalSeconds] = useState<number>(table.update_detection_interval_seconds ?? 3600);

  const caps: Capabilities = table.capabilities;
  const orphaned = Boolean((table.properties as Record<string, unknown>)?.orphaned);
  const pushdownLabel = caps?.compute_pushdown ? PUSHDOWN_LABELS[caps.compute_pushdown] ?? caps.compute_pushdown : 'None';

  async function toggle() {
    setBusy('toggle');
    setError(null);
    try {
      const next = await virtualTables.setUpdateDetection(table.rid, {
        enabled: !table.update_detection_enabled,
        interval_seconds: Math.max(60, Number(intervalSeconds) || 3600),
      });
      onChanged?.(next);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Toggle failed');
    } finally {
      setBusy(null);
    }
  }

  async function pollNow() {
    setBusy('poll');
    setError(null);
    try {
      setLastPoll(await virtualTables.pollUpdateDetectionNow(table.rid));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Poll failed');
    } finally {
      setBusy(null);
    }
  }

  return (
    <section data-testid="vt-details-panel" style={{ display: 'grid', gap: 12 }}>
      {orphaned && (
        <div
          role="alert"
          data-testid="vt-orphan-banner"
          className="of-status-danger"
          style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
        >
          <strong>Backing source table no longer exists.</strong> Reads will fail with{' '}
          <code>410 GONE_AT_SOURCE</code>. The virtual table is preserved per Foundry doc § "Auto-registration"; remove
          it manually if you no longer need the pointer.
        </div>
      )}

      <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))' }}>
        <article data-testid="vt-card-incremental" className="of-panel" style={{ padding: 14 }}>
          <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <h4 style={{ margin: 0, fontSize: 14 }}>Incremental</h4>
            <span className="of-chip" style={caps?.incremental ? { background: '#ecfdf5', color: '#047857' } : { background: '#fef2f2', color: '#b91c1c' }}>
              {caps?.incremental ? 'Supported' : 'Not supported'}
            </span>
          </header>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
            {caps?.incremental
              ? 'Foundry can configure incremental pipelines so downstream builds only process new rows.'
              : 'Pipelines reading this table re-process the full snapshot on every build.'}
          </p>
          {caps?.incremental && (
            <a
              href={`/pipelines/new?virtual_table=${encodeURIComponent(table.rid)}`}
              style={{ marginTop: 8, fontSize: 13, color: '#1d4ed8' }}
            >
              Configure incremental pipeline →
            </a>
          )}
        </article>

        <article data-testid="vt-card-versioning" className="of-panel" style={{ padding: 14 }}>
          <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <h4 style={{ margin: 0, fontSize: 14 }}>Versioning</h4>
            <span className="of-chip" style={caps?.versioning ? { background: '#ecfdf5', color: '#047857' } : { background: '#fef2f2', color: '#b91c1c' }}>
              {caps?.versioning ? 'Supported' : 'Not supported'}
            </span>
          </header>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
            {caps?.versioning
              ? 'Foundry detects source-side changes via snapshot id / commit time and skips downstream builds when the data has not changed.'
              : 'Without versioning, every poll is treated as a potential update — downstream schedules may run on each tick.'}
          </p>
        </article>

        <article data-testid="vt-card-update-detection" className="of-panel" style={{ padding: 14 }}>
          <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <h4 style={{ margin: 0, fontSize: 14 }}>Update detection</h4>
            <label style={{ display: 'inline-flex', alignItems: 'center', gap: 4, fontSize: 13 }}>
              <input
                type="checkbox"
                checked={table.update_detection_enabled}
                onChange={() => void toggle()}
                disabled={busy !== null}
                data-testid="vt-update-detection-toggle"
              />
              {table.update_detection_enabled ? 'On' : 'Off'}
            </label>
          </header>
          {table.update_detection_enabled ? (
            <>
              <dl style={{ margin: '8px 0 0', fontSize: 13, display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px' }}>
                <dt>Interval</dt>
                <dd>
                  <input
                    type="number"
                    min={60}
                    value={intervalSeconds}
                    onChange={(e) => setIntervalSeconds(Number(e.target.value))}
                    onBlur={() => void toggle()}
                    data-testid="vt-update-detection-interval"
                    style={{ width: 100 }}
                  />{' '}
                  seconds
                </dd>
                <dt>Last polled at</dt>
                <dd>{table.last_polled_at ?? '—'}</dd>
                <dt>Last observed version</dt>
                <dd style={{ fontFamily: 'var(--font-mono)' }}>{table.last_observed_version ?? '—'}</dd>
              </dl>
              <button
                type="button"
                onClick={() => void pollNow()}
                disabled={busy !== null}
                className="of-button"
                style={{ marginTop: 8, fontSize: 13 }}
                data-testid="vt-update-detection-poll-now"
              >
                {busy === 'poll' ? 'Polling…' : 'Poll now'}
              </button>
              {lastPoll && (
                <p
                  data-testid="vt-update-detection-last-poll"
                  className="of-text-muted"
                  style={{ marginTop: 8, fontSize: 12 }}
                >
                  Last poll: <strong>{lastPoll.outcome}</strong>
                  {lastPoll.event_emitted && <> · downstream event emitted</>}
                  {lastPoll.observed_version && (
                    <>
                      {' · v='}<code>{lastPoll.observed_version}</code>
                    </>
                  )}
                  {' '}({lastPoll.latency_ms} ms)
                </p>
              )}
            </>
          ) : (
            <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
              Enable polling to wake any downstream schedule that registered an{' '}
              <code>EventTrigger {'{ type: DATA_UPDATED }'}</code> on this virtual table.
            </p>
          )}
        </article>

        <article data-testid="vt-card-pushdown" className="of-panel" style={{ padding: 14 }}>
          <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <h4 style={{ margin: 0, fontSize: 14 }}>Compute pushdown</h4>
            <span className="of-chip" style={caps?.compute_pushdown ? { background: '#ecfdf5', color: '#047857' } : {}}>
              {pushdownLabel}
            </span>
          </header>
          <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
            {caps?.compute_pushdown
              ? 'Queries can be pushed down to the source system to limit data transfer. Activated via the Code Repositories + Pipeline Builder integrations (P6).'
              : 'No native pushdown engine for this source × table type.'}
          </p>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
            {capabilityChips(caps).map((chip) => (
              <span key={chip} className="of-chip">
                {chip}
              </span>
            ))}
          </div>
        </article>
      </div>

      {error && (
        <div className="of-status-danger" role="alert" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}
    </section>
  );
}
