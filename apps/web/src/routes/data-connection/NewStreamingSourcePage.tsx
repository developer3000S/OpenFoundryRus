import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import api from '@/lib/api/client';

interface FieldDescriptor {
  name: string;
  kind: 'string' | 'int' | 'secret';
  required: boolean;
  description: string;
}

interface StreamingSourceContract {
  kind: string;
  display_name: string;
  description: string;
  requires_agent: boolean;
  config_fields: FieldDescriptor[];
}

export function NewStreamingSourcePage() {
  const [contracts, setContracts] = useState<StreamingSourceContract[]>([]);
  const [selectedKind, setSelectedKind] = useState<string | null>(null);
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const [targetStreamRid, setTargetStreamRid] = useState('');
  const [batchSize, setBatchSize] = useState(100);
  const [pollIntervalMs, setPollIntervalMs] = useState(1000);
  const [schemaInference, setSchemaInference] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const selectedContract = contracts.find((c) => c.kind === selectedKind) ?? null;

  useEffect(() => {
    api
      .get<{ data: StreamingSourceContract[] }>('/data-connection/streaming-sources')
      .then((res) => setContracts(res.data))
      .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : 'Failed to load contracts'));
  }, []);

  async function submit() {
    if (!selectedContract) return;
    setBusy(true);
    setError('');
    try {
      for (const f of selectedContract.config_fields) {
        if (f.required && !(formValues[f.name] ?? '').trim()) {
          throw new Error(`Field "${f.name}" is required.`);
        }
      }
      const config: Record<string, unknown> = {};
      for (const f of selectedContract.config_fields) {
        const raw = formValues[f.name] ?? '';
        if (f.kind === 'int') config[f.name] = raw ? Number(raw) : null;
        else config[f.name] = raw;
      }
      const created = await api.post<{ id: string }>('/data-connection/streaming-sources', {
        kind: selectedContract.kind,
        config,
        target_stream_rid: targetStreamRid || null,
        batch_size: batchSize,
        poll_interval_ms: pollIntervalMs,
        schema_inference: schemaInference,
      });
      navigate(`/data-connection/sources/${created.id}`);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Submit failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/data-connection" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Back to sources</Link>
      <header>
        <h1 className="of-heading-xl">New streaming source</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Connect a streaming source (Kafka, Kinesis, Pub/Sub…) into a managed stream.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {!selectedContract ? (
        <section className="of-panel" style={{ padding: 16 }}>
          <p className="of-eyebrow">Pick a connector</p>
          <ul style={{ marginTop: 8, paddingLeft: 0, listStyle: 'none', display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))' }}>
            {contracts.map((c) => (
              <li key={c.kind}>
                <button
                  type="button"
                  onClick={() => { setSelectedKind(c.kind); setFormValues({}); }}
                  style={{
                    width: '100%',
                    textAlign: 'left',
                    padding: 12,
                    borderRadius: 12,
                    border: '1px solid var(--border-default)',
                    background: 'transparent',
                    cursor: 'pointer',
                  }}
                >
                  <strong>{c.display_name}</strong>
                  <p style={{ fontSize: 11, color: 'var(--text-muted)', margin: '4px 0' }}>{c.description}</p>
                  {c.requires_agent && <span style={{ fontSize: 10, padding: '2px 6px', background: '#fef3c7', borderRadius: 999 }}>requires agent</span>}
                </button>
              </li>
            ))}
            {contracts.length === 0 && <li className="of-text-muted">No streaming source contracts.</li>}
          </ul>
        </section>
      ) : (
        <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <p className="of-eyebrow">{selectedContract.display_name}</p>
          {selectedContract.config_fields.map((f) => (
            <label key={f.name} style={{ fontSize: 13 }}>
              {f.name}{f.required ? ' *' : ''}
              <input
                type={f.kind === 'secret' ? 'password' : f.kind === 'int' ? 'number' : 'text'}
                value={formValues[f.name] ?? ''}
                onChange={(e) => setFormValues((v) => ({ ...v, [f.name]: e.target.value }))}
                placeholder={f.description}
                className="of-input"
                style={{ marginTop: 4 }}
              />
            </label>
          ))}
          <label style={{ fontSize: 13 }}>
            Target stream RID
            <input value={targetStreamRid} onChange={(e) => setTargetStreamRid(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <div style={{ display: 'flex', gap: 8 }}>
            <label style={{ fontSize: 13, flex: 1 }}>
              Batch size
              <input type="number" value={batchSize} onChange={(e) => setBatchSize(Number(e.target.value) || 0)} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13, flex: 1 }}>
              Poll interval (ms)
              <input type="number" value={pollIntervalMs} onChange={(e) => setPollIntervalMs(Number(e.target.value) || 0)} className="of-input" style={{ marginTop: 4 }} />
            </label>
          </div>
          <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
            <input type="checkbox" checked={schemaInference} onChange={(e) => setSchemaInference(e.target.checked)} />
            Enable schema inference
          </label>
          <div style={{ display: 'flex', gap: 6 }}>
            <button type="button" onClick={() => { setSelectedKind(null); setFormValues({}); }} className="of-button">← Back</button>
            <button type="button" onClick={() => void submit()} disabled={busy} className="of-button of-button--primary">
              Create streaming source
            </button>
          </div>
        </section>
      )}
    </section>
  );
}
