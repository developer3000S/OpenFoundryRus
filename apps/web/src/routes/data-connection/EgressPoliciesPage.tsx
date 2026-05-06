import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  dataConnection,
  type EgressEndpointKind,
  type EgressPolicyKind,
  type EgressPortKind,
  type NetworkEgressPolicy,
} from '@/lib/api/data-connection';

export function EgressPoliciesPage() {
  const [policies, setPolicies] = useState<NetworkEgressPolicy[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  // wizard
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [kind, setKind] = useState<EgressPolicyKind>('direct');
  const [addressKind, setAddressKind] = useState<EgressEndpointKind>('host');
  const [addressValue, setAddressValue] = useState('');
  const [portKind, setPortKind] = useState<EgressPortKind>('single');
  const [portValue, setPortValue] = useState('443');
  const [isGlobal, setIsGlobal] = useState(false);
  const [permissionsRaw, setPermissionsRaw] = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try {
      setPolicies(await dataConnection.listEgressPolicies());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load policies');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void load();
  }, []);

  function reset() {
    setName('');
    setDescription('');
    setKind('direct');
    setAddressKind('host');
    setAddressValue('');
    setPortKind('single');
    setPortValue('443');
    setIsGlobal(false);
    setPermissionsRaw('');
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !addressValue.trim() || (portKind !== 'any' && !portValue.trim())) {
      setError('Name, address, and port are required.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await dataConnection.createEgressPolicy({
        name: name.trim(),
        description: description.trim(),
        kind,
        address: { kind: addressKind, value: addressValue.trim() },
        port: { kind: portKind, value: portKind === 'any' ? '' : portValue.trim() },
        is_global: isGlobal,
        permissions: permissionsRaw.split(/[,\n]/).map((p) => p.trim()).filter(Boolean),
      });
      setOpen(false);
      reset();
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  async function remove(p: NetworkEgressPolicy) {
    if (typeof window !== 'undefined' && !window.confirm(`Delete policy "${p.name}"?`)) return;
    setBusy(true);
    try {
      await dataConnection.deleteEgressPolicy(p.id);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Delete failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/data-connection" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Back to sources</Link>
      <header style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
        <div>
          <h1 className="of-heading-xl">Egress policies</h1>
          <p className="of-text-muted" style={{ marginTop: 4, maxWidth: 720 }}>
            Allow-listed network destinations + permission groups for source connections.
          </p>
        </div>
        <button type="button" onClick={() => setOpen(!open)} className="of-button of-button--primary">
          {open ? 'Cancel' : '+ New policy'}
        </button>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {open && (
        <form onSubmit={submit} className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
          <label style={{ fontSize: 13 }}>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <label style={{ fontSize: 13 }}>
            Description
            <input value={description} onChange={(e) => setDescription(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
          </label>
          <div style={{ display: 'flex', gap: 8 }}>
            <label style={{ fontSize: 13, flex: 1 }}>
              Kind
              <select value={kind} onChange={(e) => setKind(e.target.value as EgressPolicyKind)} className="of-input" style={{ marginTop: 4 }}>
                <option value="direct">direct</option>
                <option value="agent_proxy">agent_proxy</option>
              </select>
            </label>
            <label style={{ fontSize: 13, flex: 1 }}>
              Address kind
              <select value={addressKind} onChange={(e) => setAddressKind(e.target.value as EgressEndpointKind)} className="of-input" style={{ marginTop: 4 }}>
                <option value="host">host</option>
                <option value="ip">ip</option>
                <option value="cidr">cidr</option>
              </select>
            </label>
          </div>
          <label style={{ fontSize: 13 }}>
            Address value
            <input value={addressValue} onChange={(e) => setAddressValue(e.target.value)} placeholder="api.example.com" className="of-input" style={{ marginTop: 4 }} />
          </label>
          <div style={{ display: 'flex', gap: 8 }}>
            <label style={{ fontSize: 13, flex: 1 }}>
              Port kind
              <select value={portKind} onChange={(e) => setPortKind(e.target.value as EgressPortKind)} className="of-input" style={{ marginTop: 4 }}>
                <option value="single">single</option>
                <option value="range">range</option>
                <option value="any">any</option>
              </select>
            </label>
            {portKind !== 'any' && (
              <label style={{ fontSize: 13, flex: 1 }}>
                Port value {portKind === 'range' ? '(e.g., 8000-9000)' : '(e.g., 443)'}
                <input value={portValue} onChange={(e) => setPortValue(e.target.value)} className="of-input" style={{ marginTop: 4 }} />
              </label>
            )}
          </div>
          <label style={{ fontSize: 13, display: 'flex', alignItems: 'center', gap: 6 }}>
            <input type="checkbox" checked={isGlobal} onChange={(e) => setIsGlobal(e.target.checked)} />
            Global (visible to all sources)
          </label>
          <label style={{ fontSize: 13 }}>
            Permissions (comma or newline separated)
            <textarea value={permissionsRaw} onChange={(e) => setPermissionsRaw(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11 }} />
          </label>
          <button type="submit" disabled={busy} className="of-button of-button--primary">
            Create policy
          </button>
        </form>
      )}

      {loading ? (
        <p className="of-text-muted">Loading…</p>
      ) : (
        <section className="of-panel" style={{ padding: 16, overflow: 'auto' }}>
          <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
            <thead>
              <tr>
                {['Name', 'Kind', 'Address', 'Port', 'Global', 'Permissions', ''].map((h) => (
                  <th key={h} style={{ textAlign: 'left', padding: 6, borderBottom: '1px solid var(--border-default)' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {policies.map((p) => (
                <tr key={p.id} style={{ borderBottom: '1px solid var(--border-subtle)' }}>
                  <td style={{ padding: 6, fontWeight: 600 }}>{p.name}</td>
                  <td style={{ padding: 6 }}>{p.kind}</td>
                  <td style={{ padding: 6, fontFamily: 'var(--font-mono)' }}>{p.address.kind}:{p.address.value}</td>
                  <td style={{ padding: 6, fontFamily: 'var(--font-mono)' }}>{p.port.kind === 'any' ? 'any' : p.port.value}</td>
                  <td style={{ padding: 6 }}>{p.is_global ? '✓' : '—'}</td>
                  <td style={{ padding: 6, fontSize: 11 }}>{p.permissions.join(', ') || '—'}</td>
                  <td style={{ padding: 6, textAlign: 'right' }}>
                    <button type="button" onClick={() => void remove(p)} disabled={busy} className="of-button" style={{ fontSize: 11, color: '#b91c1c', borderColor: '#fecaca' }}>
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
              {policies.length === 0 && (
                <tr><td colSpan={7} className="of-text-muted" style={{ padding: 18, textAlign: 'center' }}>No egress policies.</td></tr>
              )}
            </tbody>
          </table>
        </section>
      )}
    </section>
  );
}
