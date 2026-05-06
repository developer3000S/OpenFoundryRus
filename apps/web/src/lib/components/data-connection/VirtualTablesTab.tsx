import { useEffect, useMemo, useState } from 'react';

import {
  BULK_REGISTER_PROVIDERS,
  providerLabel,
  virtualTables,
  type DiscoveredEntry,
  type VirtualTable,
  type VirtualTableProvider,
  type VirtualTableSourceLink,
} from '@/lib/api/virtual-tables';

import { AutoRegistrationCard } from './AutoRegistrationCard';
import { BulkRegisterDialog } from './BulkRegisterDialog';
import { CreateAutoRegistrationModal } from './CreateAutoRegistrationModal';
import { CreateVirtualTableModal } from './CreateVirtualTableModal';
import { RemoteCatalogBrowser } from './RemoteCatalogBrowser';
import { VirtualTableInspector } from './VirtualTableInspector';

interface Props {
  sourceRid: string;
  provider: VirtualTableProvider;
}

export function VirtualTablesTab({ sourceRid, provider }: Props) {
  const [link, setLink] = useState<VirtualTableSourceLink | null>(null);
  const [loading, setLoading] = useState(true);
  const [banner, setBanner] = useState('');
  const [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<DiscoveredEntry | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [autoRegOpen, setAutoRegOpen] = useState(false);
  const [lastCreated, setLastCreated] = useState<VirtualTable | null>(null);
  const [bulkSummary, setBulkSummary] = useState<{ registered: number; failed: number } | null>(null);

  const supportsBulk = useMemo(() => BULK_REGISTER_PROVIDERS.includes(provider), [provider]);

  async function loadLink() {
    setLoading(true);
    setBanner('');
    try {
      const ping = await virtualTables.discoverRemoteCatalog(sourceRid).catch(() => null);
      if (ping) {
        setLink({
          source_rid: sourceRid,
          provider,
          virtual_tables_enabled: true,
          code_imports_enabled: false,
          export_controls: {},
          auto_register_project_rid: null,
          auto_register_enabled: false,
          auto_register_interval_seconds: null,
          auto_register_tag_filters: [],
          iceberg_catalog_kind: null,
          iceberg_catalog_config: null,
          created_at: '',
          updated_at: '',
        });
      } else {
        setLink(null);
      }
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void loadLink(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [sourceRid, provider]);

  async function enable() {
    setBusy(true);
    setBanner('');
    try {
      setLink(await virtualTables.enableOnSource(sourceRid, { provider }));
    } catch (err) {
      setBanner(err instanceof Error ? err.message : 'Failed to enable virtual tables');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {loading ? (
        <div style={loadingStyle}>Loading…</div>
      ) : !link?.virtual_tables_enabled ? (
        <div style={enableBannerStyle}>
          <h3 style={{ margin: '0 0 8px' }}>Enable virtual tables on this source</h3>
          <p>Virtual tables let Foundry query <strong>{providerLabel(provider)}</strong> directly without copying data. Once enabled you can browse the remote catalog, register tables manually, or bulk-register a list at once.</p>
          <p style={{ ...mutedStyle, fontSize: 12 }}>See Foundry docs § "Set up a connection for a virtual table" for the full walk-through.</p>
          <button type="button" onClick={() => void enable()} disabled={busy} style={enableButtonStyle}>{busy ? 'Enabling…' : 'Enable virtual tables'}</button>
          {banner && <div style={errorStyle}>{banner}</div>}
        </div>
      ) : (
        <>
          <AutoRegistrationCard
            sourceRid={sourceRid}
            provider={provider}
            link={link}
            onOpenWizard={() => setAutoRegOpen(true)}
            onChanged={(updated) => setLink(updated)}
            onDisabled={() => setLink((prev) => prev ? { ...prev, auto_register_enabled: false } : prev)}
          />

          <header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
            <h2 style={{ margin: 0, fontSize: 16 }}>Virtual tables</h2>
            <div style={{ display: 'flex', gap: 8 }}>
              <button type="button" onClick={() => setCreateOpen(true)} style={ctaPrimary}>Create virtual table</button>
              {supportsBulk && (
                <button type="button" onClick={() => setBulkOpen(true)} style={ctaSecondary}>Bulk register</button>
              )}
            </div>
          </header>

          {lastCreated && (
            <div style={successStyle}>
              Created <a href={`/virtual-tables/${encodeURIComponent(lastCreated.rid)}`}>{lastCreated.name}</a> in project <code>{lastCreated.project_rid}</code>.
              <button onClick={() => setLastCreated(null)} aria-label="Dismiss" style={dismissStyle}>×</button>
            </div>
          )}
          {bulkSummary && (
            <div style={bulkSummary.failed > 0 ? warningStyle : successStyle}>
              Bulk register: {bulkSummary.registered} registered, {bulkSummary.failed} failed.
              <button onClick={() => setBulkSummary(null)} aria-label="Dismiss" style={dismissStyle}>×</button>
            </div>
          )}

          <div style={{ display: 'grid', gridTemplateColumns: 'minmax(240px, 1fr) minmax(280px, 1fr)', gap: 12 }}>
            <RemoteCatalogBrowser sourceRid={sourceRid} onSelect={(entry) => setSelected(entry)} />
            <VirtualTableInspector
              provider={provider}
              entry={selected}
              onCreate={(entry) => { setSelected(entry); setCreateOpen(true); }}
            />
          </div>
        </>
      )}

      <CreateVirtualTableModal
        open={createOpen}
        sourceRid={sourceRid}
        provider={provider}
        entry={selected}
        onClose={() => setCreateOpen(false)}
        onCreated={(table) => setLastCreated(table)}
      />
      <BulkRegisterDialog
        open={bulkOpen}
        sourceRid={sourceRid}
        provider={provider}
        onClose={() => setBulkOpen(false)}
        onCompleted={(registered, failed) => setBulkSummary({ registered, failed })}
      />
      <CreateAutoRegistrationModal
        open={autoRegOpen}
        sourceRid={sourceRid}
        provider={provider}
        onClose={() => setAutoRegOpen(false)}
        onEnabled={(updated) => setLink(updated)}
      />
    </section>
  );
}

const loadingStyle: React.CSSProperties = { padding: '8px 12px', borderRadius: 4, fontSize: 14, display: 'flex', alignItems: 'center', gap: 8 };
const enableBannerStyle: React.CSSProperties = { border: '1px solid #e5e7eb', borderRadius: 8, padding: 20, background: '#fff' };
const enableButtonStyle: React.CSSProperties = { marginTop: 8, padding: '8px 12px', background: '#1d4ed8', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 14 };
const ctaPrimary: React.CSSProperties = { padding: '6px 12px', background: '#1d4ed8', color: '#fff', border: '1px solid #1d4ed8', borderRadius: 4, cursor: 'pointer', fontSize: 14 };
const ctaSecondary: React.CSSProperties = { ...ctaPrimary, background: '#fff', color: '#1d4ed8' };
const errorStyle: React.CSSProperties = { background: '#fef2f2', color: '#b91c1c', border: '1px solid #fecaca', padding: '8px 12px', borderRadius: 4, fontSize: 14, marginTop: 8 };
const successStyle: React.CSSProperties = { background: '#ecfdf5', color: '#047857', border: '1px solid #6ee7b7', padding: '8px 12px', borderRadius: 4, fontSize: 14, display: 'flex', alignItems: 'center', gap: 8 };
const warningStyle: React.CSSProperties = { background: '#fef9c3', color: '#854d0e', border: '1px solid #fde047', padding: '8px 12px', borderRadius: 4, fontSize: 14, display: 'flex', alignItems: 'center', gap: 8 };
const dismissStyle: React.CSSProperties = { marginLeft: 'auto', background: 'transparent', border: 'none', cursor: 'pointer', fontSize: 16 };
const mutedStyle: React.CSSProperties = { color: '#6b7280' };
