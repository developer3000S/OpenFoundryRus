import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { createObjectType } from '@/lib/api/ontology';

export function CreateObjectTypePage() {
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');
  const [icon, setIcon] = useState('');
  const [color, setColor] = useState('#4d8cf0');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) {
      setError('Name is required');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await createObjectType({
        name: name.trim(),
        display_name: displayName.trim() || undefined,
        description: description.trim() || undefined,
        icon: icon.trim() || undefined,
        color: color || undefined,
      });
      navigate('/ontology');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Create failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16, maxWidth: 720 }}>
      <Link to="/ontology" style={{ color: 'var(--text-muted)', fontSize: 13 }}>← Ontology</Link>
      <header>
        <h1 className="of-heading-xl">Create object type</h1>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
          Define metadata, identifier, visual treatment and semantic description before wiring properties and links.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <form onSubmit={submit} className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
        <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
          <div
            style={{
              width: 86,
              height: 86,
              borderRadius: 8,
              background: color,
              color: 'white',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 32,
            }}
          >
            {icon || '◧'}
          </div>
          <div style={{ display: 'grid', gap: 8, flex: 1 }}>
            <label style={{ fontSize: 13 }}>
              Name (identifier)
              <input value={name} onChange={(e) => setName(e.target.value)} placeholder="customer_invoice" required className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Display name
              <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="Customer Invoice" className="of-input" style={{ marginTop: 4 }} />
            </label>
            <label style={{ fontSize: 13 }}>
              Description
              <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="of-input" style={{ marginTop: 4 }} />
            </label>
            <div style={{ display: 'flex', gap: 8 }}>
              <label style={{ fontSize: 13, flex: 1 }}>
                Icon (emoji or codepoint)
                <input value={icon} onChange={(e) => setIcon(e.target.value)} placeholder="📄" className="of-input" style={{ marginTop: 4 }} />
              </label>
              <label style={{ fontSize: 13, flex: 1 }}>
                Color
                <input type="color" value={color} onChange={(e) => setColor(e.target.value)} className="of-input" style={{ marginTop: 4, padding: 2, height: 36 }} />
              </label>
            </div>
          </div>
        </div>
        <button type="submit" disabled={busy || !name.trim()} className="of-button of-button--primary">
          Create object type
        </button>
      </form>
    </section>
  );
}
