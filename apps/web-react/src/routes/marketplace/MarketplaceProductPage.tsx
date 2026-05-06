import { useState } from 'react';
import { useParams } from 'react-router-dom';

import { previewInstallSchedules, type ProductScheduleManifest } from '@/lib/api/marketplace-schedules';

type Tab = 'overview' | 'schedules';

export function MarketplaceProductPage() {
  const params = useParams();
  const productId = params.id ?? '';

  const [activeTab, setActiveTab] = useState<Tab>('overview');
  const [manifests, setManifests] = useState<ProductScheduleManifest[]>([]);
  const [activated, setActivated] = useState<Set<string>>(new Set());
  const [materialised, setMaterialised] = useState<ProductScheduleManifest[] | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const [productVersionId, setProductVersionId] = useState('');

  async function previewInstall() {
    try {
      setErrorMsg(null);
      const res = await previewInstallSchedules(productId, {
        product_version_id: productVersionId,
        activate_manifests: Array.from(activated),
      });
      setMaterialised(res.materialised);
      setManifests(res.materialised);
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err));
    }
  }

  function toggleManifest(name: string) {
    setActivated((current) => {
      const next = new Set(current);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }

  return (
    <main className="of-page" data-testid="marketplace-product-page" style={{ padding: 24, maxWidth: 1024, margin: '0 auto' }}>
      <header style={{ display: 'flex', alignItems: 'baseline', gap: 12, marginBottom: 16 }}>
        <h1 className="of-heading-xl">Marketplace product</h1>
        <code style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-muted)' }}>{productId}</code>
      </header>

      <nav role="tablist" style={{ display: 'flex', gap: 4, borderBottom: '1px solid var(--border-default)', marginBottom: 12 }}>
        {(['overview', 'schedules'] as Tab[]).map((tab) => {
          const active = activeTab === tab;
          return (
            <button
              key={tab}
              type="button"
              role="tab"
              aria-selected={active}
              data-testid={`${tab}-tab`}
              onClick={() => setActiveTab(tab)}
              style={{
                background: 'transparent',
                border: 'none',
                color: active ? 'var(--text-strong)' : 'var(--text-muted)',
                padding: '8px 14px',
                cursor: 'pointer',
                borderBottom: `2px solid ${active ? '#38bdf8' : 'transparent'}`,
                textTransform: 'capitalize',
                fontSize: 13,
                fontWeight: active ? 600 : 400,
              }}
            >
              {tab}
            </button>
          );
        })}
      </nav>

      {errorMsg && (
        <p role="alert" className="of-status-danger" style={{ padding: '10px 12px', borderRadius: 'var(--radius-md)', fontSize: 13, marginBottom: 12 }}>
          {errorMsg}
        </p>
      )}

      {activeTab === 'schedules' && (
        <section data-testid="product-schedules" className="of-panel" style={{ padding: 16, display: 'grid', gap: 10 }}>
          <label style={{ fontSize: 13, display: 'grid', gap: 6 }}>
            <span style={{ fontWeight: 600 }}>Product version id</span>
            <input
              type="text"
              value={productVersionId}
              onChange={(e) => setProductVersionId(e.target.value)}
              placeholder="00000000-0000-0000-0000-000000000000"
              data-testid="product-version-input"
              className="of-input"
            />
          </label>
          <button
            type="button"
            data-testid="preview-install-button"
            onClick={() => void previewInstall()}
            disabled={!productVersionId}
            className="of-button of-button--primary"
            style={{ width: 'fit-content' }}
          >
            Preview install
          </button>

          {manifests.length === 0 ? (
            <p style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>No schedule manifests for this product yet.</p>
          ) : (
            <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'grid', gap: 4 }}>
              {manifests.map((m) => (
                <li key={m.name} data-testid="manifest-row" className="of-panel-muted" style={{ padding: '8px 12px' }}>
                  <label style={{ display: 'flex', gap: 8, alignItems: 'center', fontSize: 13 }}>
                    <input
                      type="checkbox"
                      checked={activated.has(m.name)}
                      onChange={() => toggleManifest(m.name)}
                      data-testid="manifest-activate-checkbox"
                    />
                    <strong>{m.name}</strong>
                    <span style={{ color: '#6ee7b7', fontSize: 11 }}>[{m.scope_kind || 'USER'}]</span>
                    <span className="of-text-muted">{m.description}</span>
                  </label>
                </li>
              ))}
            </ul>
          )}

          {materialised && (
            <section
              data-testid="materialised-preview"
              style={{
                background: '#0c0a09',
                border: '1px solid #44403c',
                borderRadius: 'var(--radius-md)',
                padding: 12,
                color: '#d6d3d1',
              }}
            >
              <h3 className="of-heading-md" style={{ color: '#f5f5f4', marginBottom: 8 }}>
                Resolved manifests
              </h3>
              <pre style={{ margin: 0, fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                {JSON.stringify(materialised, null, 2)}
              </pre>
            </section>
          )}
        </section>
      )}
    </main>
  );
}
