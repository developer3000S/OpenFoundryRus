import { useState } from 'react';

import { virtualTables, type VirtualTableProvider, type VirtualTableSourceLink } from '@/lib/api/virtual-tables';

interface Props {
  sourceRid: string;
  provider: VirtualTableProvider;
  link: VirtualTableSourceLink | null;
  onOpenWizard: () => void;
  onChanged?: (link: VirtualTableSourceLink) => void;
  onDisabled?: () => void;
}

function layoutLabel(kind: string | null | undefined): string {
  if (kind === 'FLAT') return 'Flat (database__schema__table)';
  return 'Nested (database / schema / table)';
}

export function AutoRegistrationCard({ sourceRid, provider, link, onOpenWizard, onDisabled }: Props) {
  const [busy, setBusy] = useState<'scan' | 'disable' | null>(null);
  const [banner, setBanner] = useState<string | null>(null);
  const [lastScan, setLastScan] = useState<{ added: number; updated: number; orphaned: number } | null>(null);

  async function scanNow() {
    setBusy('scan');
    setBanner(null);
    try {
      setLastScan(await virtualTables.scanAutoRegistrationNow(sourceRid));
    } catch (err) {
      setBanner(err instanceof Error ? err.message : 'Scan failed');
    } finally {
      setBusy(null);
    }
  }

  async function disable() {
    setBusy('disable');
    setBanner(null);
    try {
      await virtualTables.disableAutoRegistration(sourceRid);
      onDisabled?.();
    } catch (err) {
      setBanner(err instanceof Error ? err.message : 'Disable failed');
    } finally {
      setBusy(null);
    }
  }

  const enabled = link?.auto_register_enabled;
  const linkExt = link as unknown as { auto_register_folder_mirror_kind?: string; auto_register_table_tag_filters?: string[] } | null;

  return (
    <section style={cardStyle}>
      <header style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <h3 style={{ margin: 0, fontSize: 15 }}>
          Auto-registration <span style={mutedStyle}>({provider})</span>
        </h3>
        <span style={enabled ? badgeActiveStyle : badgeMutedStyle}>{enabled ? 'on' : 'off'}</span>
      </header>

      {!enabled ? (
        <>
          <p style={ledeStyle}>
            Mirror this source's catalog into a Foundry-managed project. Tables in the source are auto-registered as virtual tables and kept in sync on a schedule. Per Foundry doc, deleting a table at the source does <em>not</em> delete the virtual table — reads return <code>410 GONE_AT_SOURCE</code> instead.
          </p>
          <button type="button" onClick={onOpenWizard} style={primaryStyle}>Enable auto-registration</button>
        </>
      ) : (
        <>
          <dl style={kvStyle}>
            <dt style={dtStyle}>Managed project</dt>
            <dd style={{ margin: 0 }}>
              {link.auto_register_project_rid ? (
                <a href={`/projects/${encodeURIComponent(link.auto_register_project_rid)}`} style={{ fontFamily: 'ui-monospace, SFMono-Regular, monospace' }}>{link.auto_register_project_rid}</a>
              ) : <span style={mutedStyle}>—</span>}
            </dd>
            <dt style={dtStyle}>Layout</dt>
            <dd style={{ margin: 0 }}>{layoutLabel(linkExt?.auto_register_folder_mirror_kind)}</dd>
            <dt style={dtStyle}>Tag filters</dt>
            <dd style={{ margin: 0 }}>
              {linkExt?.auto_register_table_tag_filters?.length ? linkExt.auto_register_table_tag_filters.map((tag) => (
                <span key={tag} style={chipStyle}>{tag}</span>
              )) : <span style={mutedStyle}>none</span>}
            </dd>
            <dt style={dtStyle}>Poll interval</dt>
            <dd style={{ margin: 0 }}>{link.auto_register_interval_seconds ?? '—'}{link.auto_register_interval_seconds ? 's' : ''}</dd>
            <dt style={dtStyle}>Last run</dt>
            <dd style={{ margin: 0 }}>
              {lastScan ? (
                <span>+{lastScan.added} / ~{lastScan.updated} / ✗{lastScan.orphaned}</span>
              ) : <span style={mutedStyle}>no run yet</span>}
            </dd>
          </dl>
          <div style={{ display: 'flex', gap: 8 }}>
            <button type="button" onClick={() => void scanNow()} disabled={busy !== null} style={actionBtnStyle}>{busy === 'scan' ? 'Scanning…' : 'Trigger now'}</button>
            <button type="button" onClick={() => void disable()} disabled={busy !== null} style={dangerBtnStyle}>{busy === 'disable' ? 'Disabling…' : 'Disable'}</button>
          </div>
        </>
      )}

      {banner && <div role="alert" style={errorStyle}>{banner}</div>}
    </section>
  );
}

const cardStyle: React.CSSProperties = { border: '1px solid #e5e7eb', borderRadius: 8, padding: 16, background: '#fff', display: 'flex', flexDirection: 'column', gap: 12 };
const ledeStyle: React.CSSProperties = { margin: 0, color: '#4b5563', fontSize: 14 };
const mutedStyle: React.CSSProperties = { color: '#6b7280' };
const badgeActiveStyle: React.CSSProperties = { marginLeft: 'auto', fontSize: 10, padding: '2px 8px', borderRadius: 999, textTransform: 'uppercase', letterSpacing: '0.05em', background: '#d1fae5', color: '#065f46' };
const badgeMutedStyle: React.CSSProperties = { marginLeft: 'auto', fontSize: 10, padding: '2px 8px', borderRadius: 999, textTransform: 'uppercase', letterSpacing: '0.05em', background: '#f3f4f6', color: '#4b5563' };
const primaryStyle: React.CSSProperties = { alignSelf: 'flex-start', background: '#1d4ed8', color: '#fff', border: '1px solid #1d4ed8', borderRadius: 4, padding: '8px 12px', fontSize: 14, cursor: 'pointer' };
const kvStyle: React.CSSProperties = { display: 'grid', gridTemplateColumns: 'max-content 1fr', gap: '4px 12px', margin: 0, fontSize: 14 };
const dtStyle: React.CSSProperties = { color: '#6b7280', fontSize: 12, textTransform: 'uppercase', letterSpacing: '0.05em', alignSelf: 'center' };
const chipStyle: React.CSSProperties = { display: 'inline-block', padding: '1px 6px', marginRight: 4, fontSize: 12, borderRadius: 4, background: '#f3f4f6', border: '1px solid #e5e7eb' };
const actionBtnStyle: React.CSSProperties = { padding: '6px 12px', border: '1px solid #d1d5db', borderRadius: 4, background: '#fff', cursor: 'pointer', fontSize: 14 };
const dangerBtnStyle: React.CSSProperties = { ...actionBtnStyle, color: '#b91c1c', borderColor: '#fca5a5' };
const errorStyle: React.CSSProperties = { background: '#fef2f2', color: '#b91c1c', border: '1px solid #fecaca', borderRadius: 4, padding: '6px 8px', fontSize: 14 };
