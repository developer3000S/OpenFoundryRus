import { Link } from 'react-router-dom';

export function AgentsPage() {
  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/data-connection" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Back to sources</Link>
      <header>
        <h1 className="of-heading-xl">Data Connection agents</h1>
        <p className="of-text-muted" style={{ marginTop: 4, maxWidth: 720 }}>
          An agent is a downloadable program installed within your organisational network. Agents power connections
          leveraging agent proxy policies and agent worker connections.
        </p>
      </header>

      <div style={{ padding: 16, background: '#fef3c7', color: '#92400e', borderRadius: 12, fontSize: 13 }}>
        <p style={{ fontWeight: 600 }}>Agent worker is in the legacy phase</p>
        <p style={{ marginTop: 4, fontSize: 12 }}>
          For new sources, prefer Foundry worker — it benefits from containerised, scalable job execution with no agent
          maintenance overhead. Agent worker support is not included in the Data Connection MVP.
        </p>
      </div>

      <section className="of-panel" style={{ padding: 16 }}>
        <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse' }}>
          <thead>
            <tr>
              {['Agent', 'Host', 'Status', 'Last heartbeat'].map((h) => (
                <th key={h} style={{ textAlign: 'left', padding: 6, borderBottom: '1px solid var(--border-default)' }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            <tr>
              <td colSpan={4} style={{ padding: 18, textAlign: 'center' }} className="of-text-muted">No agents registered.</td>
            </tr>
          </tbody>
        </table>
      </section>

      <button type="button" disabled className="of-button" style={{ opacity: 0.6, cursor: 'not-allowed' }} title="Agent worker is out of MVP scope">
        Download agent (unavailable)
      </button>
    </section>
  );
}
