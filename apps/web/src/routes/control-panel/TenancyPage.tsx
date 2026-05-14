// SG.2: Enrollment, organization, and space model — Control Panel UI.
//
// This page is the admin-side companion to the tenancy-organizations-service
// SG.2 endpoints (admins / guests / tenancy_spaces / membership). It is
// intentionally compact: per-organization administrators, guests, and
// spaces are managed via inline forms, and free-form metadata /
// settings / quotas are surfaced as JSON editors so power users can
// inspect and update the structured config without a typed form for
// every key.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { JsonEditor, parseJsonOr } from '@/lib/components/JsonEditor';
import {
  checkOrganizationMembership,
  createOrganizationAdmin,
  createOrganizationGuest,
  createTenancySpace,
  deleteOrganizationAdmin,
  deleteOrganizationGuest,
  deleteTenancySpace,
  listOrganizationAdmins,
  listOrganizationGuests,
  listOrganizations,
  listTenancySpaces,
  updateOrganization,
  type MembershipCheck,
  type Organization,
  type OrganizationAdmin,
  type OrganizationGuest,
  type TenancySpace,
} from '@/lib/api/tenancy';

export function TenancyPage() {
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [selectedOrgId, setSelectedOrgId] = useState<string | null>(null);
  const [loadingOrgs, setLoadingOrgs] = useState(true);
  const [error, setError] = useState('');

  const refreshOrgs = useCallback(async () => {
    setLoadingOrgs(true);
    setError('');
    try {
      const items = await listOrganizations();
      setOrgs(items);
      if (items.length > 0 && !selectedOrgId) {
        setSelectedOrgId(items[0].id);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load organizations');
    } finally {
      setLoadingOrgs(false);
    }
  }, [selectedOrgId]);

  useEffect(() => {
    void refreshOrgs();
  }, [refreshOrgs]);

  const selectedOrg = useMemo(
    () => orgs.find((o) => o.id === selectedOrgId) ?? null,
    [orgs, selectedOrgId],
  );

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/control-panel" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
        ← Control panel
      </Link>

      <header>
        <h1 className="of-heading-xl">Tenancy: organizations &amp; spaces</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Manage organizations, their administrators, guest memberships, and
          Foundry-style spaces. See{' '}
          <a href="/docs/security-governance/security-overview" target="_blank" rel="noreferrer">
            security overview
          </a>{' '}
          and the{' '}
          <a href="/docs/security-governance/shared-responsibility-model" target="_blank" rel="noreferrer">
            shared responsibility model
          </a>{' '}
          before changing membership configuration.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
        <h2 className="of-heading-lg" style={{ fontSize: 14, textTransform: 'uppercase', letterSpacing: '0.12em' }}>
          Organizations
        </h2>
        {loadingOrgs ? (
          <p className="of-text-muted">Loading…</p>
        ) : orgs.length === 0 ? (
          <p className="of-text-muted">No organizations yet.</p>
        ) : (
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {orgs.map((o) => (
              <button
                key={o.id}
                onClick={() => setSelectedOrgId(o.id)}
                className={selectedOrgId === o.id ? 'of-button of-button--primary' : 'of-button'}
                style={{ fontSize: 13 }}
              >
                {o.display_name} <span style={{ opacity: 0.6 }}>({o.slug})</span>
              </button>
            ))}
          </div>
        )}
      </section>

      {selectedOrg && (
        <OrganizationDetail
          org={selectedOrg}
          onUpdated={(updated) => {
            setOrgs((prev) => prev.map((o) => (o.id === updated.id ? updated : o)));
          }}
          onError={(msg) => setError(msg)}
        />
      )}
    </section>
  );
}

function OrganizationDetail({
  org,
  onUpdated,
  onError,
}: {
  org: Organization;
  onUpdated: (org: Organization) => void;
  onError: (msg: string) => void;
}) {
  const [metaJson, setMetaJson] = useState(JSON.stringify(org.metadata ?? {}, null, 2));
  const [settingsJson, setSettingsJson] = useState(JSON.stringify(org.settings ?? {}, null, 2));
  const [quotasJson, setQuotasJson] = useState(JSON.stringify(org.quotas ?? {}, null, 2));
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setMetaJson(JSON.stringify(org.metadata ?? {}, null, 2));
    setSettingsJson(JSON.stringify(org.settings ?? {}, null, 2));
    setQuotasJson(JSON.stringify(org.quotas ?? {}, null, 2));
  }, [org]);

  async function save() {
    setBusy(true);
    try {
      const updated = await updateOrganization(org.id, {
        metadata: parseJsonOr<Record<string, unknown>>(metaJson, {}),
        settings: parseJsonOr<Record<string, unknown>>(settingsJson, {}),
        quotas: parseJsonOr<Record<string, unknown>>(quotasJson, {}),
      });
      onUpdated(updated);
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to save');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section style={{ display: 'grid', gap: 16 }}>
      <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 12 }}>
        <div>
          <h2 className="of-heading-lg" style={{ fontSize: 16 }}>{org.display_name}</h2>
          <p className="of-text-muted" style={{ fontSize: 12 }}>
            ID {org.id} · slug <code>{org.slug}</code> · status {org.status}
          </p>
          {org.description && <p style={{ fontSize: 13, marginTop: 8 }}>{org.description}</p>}
          {org.contact_email && (
            <p className="of-text-muted" style={{ fontSize: 12, marginTop: 4 }}>
              Contact: {org.contact_email}
            </p>
          )}
        </div>

        <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }}>
          <label style={{ fontSize: 12 }}>
            Metadata
            <JsonEditor value={metaJson} onChange={setMetaJson} minHeight={140} />
          </label>
          <label style={{ fontSize: 12 }}>
            Settings
            <JsonEditor value={settingsJson} onChange={setSettingsJson} minHeight={140} />
          </label>
          <label style={{ fontSize: 12 }}>
            Quotas
            <JsonEditor value={quotasJson} onChange={setQuotasJson} minHeight={140} />
          </label>
        </div>

        <div>
          <button className="of-button of-button--primary" disabled={busy} onClick={() => void save()}>
            {busy ? 'Saving…' : 'Save metadata / settings / quotas'}
          </button>
        </div>
      </section>

      <AdminSection orgId={org.id} onError={onError} />
      <GuestSection orgId={org.id} onError={onError} />
      <SpacesSection orgId={org.id} onError={onError} />
      <MembershipProbe orgId={org.id} onError={onError} />
    </section>
  );
}

function AdminSection({ orgId, onError }: { orgId: string; onError: (msg: string) => void }) {
  const [admins, setAdmins] = useState<OrganizationAdmin[]>([]);
  const [userId, setUserId] = useState('');
  const [scope, setScope] = useState('enrollment_admin');

  const refresh = useCallback(async () => {
    try {
      setAdmins(await listOrganizationAdmins(orgId));
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to load admins');
    }
  }, [orgId, onError]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function add() {
    if (!userId.trim()) {
      return;
    }
    try {
      await createOrganizationAdmin(orgId, { user_id: userId.trim(), scope: scope.trim() || undefined });
      setUserId('');
      await refresh();
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to add admin');
    }
  }

  async function remove(a: OrganizationAdmin) {
    try {
      await deleteOrganizationAdmin(orgId, a.user_id, a.scope);
      await refresh();
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to remove admin');
    }
  }

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <h3 className="of-heading-lg" style={{ fontSize: 14 }}>Administrators</h3>
      <ul style={{ display: 'grid', gap: 4, fontSize: 13, listStyle: 'none', padding: 0, margin: 0 }}>
        {admins.length === 0 && <li className="of-text-muted">No administrators yet.</li>}
        {admins.map((a) => (
          <li key={`${a.user_id}-${a.scope}`} style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <span>
              <code>{a.user_id}</code> · {a.scope}
            </span>
            <button className="of-button of-button--ghost" onClick={() => void remove(a)} style={{ fontSize: 12 }}>
              Remove
            </button>
          </li>
        ))}
      </ul>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <label style={{ fontSize: 12 }}>
          User ID
          <input className="of-input" value={userId} onChange={(e) => setUserId(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Scope
          <input className="of-input" value={scope} onChange={(e) => setScope(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <button className="of-button" onClick={() => void add()}>Add administrator</button>
      </div>
    </section>
  );
}

function GuestSection({ orgId, onError }: { orgId: string; onError: (msg: string) => void }) {
  const [guests, setGuests] = useState<OrganizationGuest[]>([]);
  const [userId, setUserId] = useState('');
  const [primaryOrgId, setPrimaryOrgId] = useState('');

  const refresh = useCallback(async () => {
    try {
      setGuests(await listOrganizationGuests(orgId));
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to load guests');
    }
  }, [orgId, onError]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function add() {
    if (!userId.trim() || !primaryOrgId.trim()) {
      return;
    }
    try {
      await createOrganizationGuest(orgId, {
        user_id: userId.trim(),
        primary_organization_id: primaryOrgId.trim(),
      });
      setUserId('');
      setPrimaryOrgId('');
      await refresh();
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to add guest');
    }
  }

  async function remove(g: OrganizationGuest) {
    try {
      await deleteOrganizationGuest(orgId, g.user_id);
      await refresh();
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to remove guest');
    }
  }

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <h3 className="of-heading-lg" style={{ fontSize: 14 }}>Guest memberships</h3>
      <ul style={{ display: 'grid', gap: 4, fontSize: 13, listStyle: 'none', padding: 0, margin: 0 }}>
        {guests.length === 0 && <li className="of-text-muted">No guests yet.</li>}
        {guests.map((g) => (
          <li key={g.user_id} style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <span>
              <code>{g.user_id}</code> · primary org <code>{g.primary_organization_id}</code> · {g.status}
            </span>
            <button className="of-button of-button--ghost" onClick={() => void remove(g)} style={{ fontSize: 12 }}>
              Remove
            </button>
          </li>
        ))}
      </ul>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <label style={{ fontSize: 12 }}>
          User ID
          <input className="of-input" value={userId} onChange={(e) => setUserId(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Primary organization ID
          <input className="of-input" value={primaryOrgId} onChange={(e) => setPrimaryOrgId(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <button className="of-button" onClick={() => void add()}>Add guest</button>
      </div>
    </section>
  );
}

function SpacesSection({ orgId, onError }: { orgId: string; onError: (msg: string) => void }) {
  const [spaces, setSpaces] = useState<TenancySpace[]>([]);
  const [slug, setSlug] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [description, setDescription] = useState('');

  const refresh = useCallback(async () => {
    try {
      setSpaces(await listTenancySpaces(orgId));
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to load spaces');
    }
  }, [orgId, onError]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function add() {
    if (!slug.trim() || !displayName.trim()) {
      return;
    }
    try {
      await createTenancySpace(orgId, {
        slug: slug.trim(),
        display_name: displayName.trim(),
        description: description.trim() || undefined,
      });
      setSlug('');
      setDisplayName('');
      setDescription('');
      await refresh();
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to create space');
    }
  }

  async function remove(s: TenancySpace) {
    try {
      await deleteTenancySpace(s.id);
      await refresh();
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to delete space');
    }
  }

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <h3 className="of-heading-lg" style={{ fontSize: 14 }}>Spaces</h3>
      <p className="of-text-muted" style={{ fontSize: 12 }}>
        Foundry-style spaces (store / administration boundary). Distinct from nexus federation spaces.
      </p>
      <ul style={{ display: 'grid', gap: 4, fontSize: 13, listStyle: 'none', padding: 0, margin: 0 }}>
        {spaces.length === 0 && <li className="of-text-muted">No spaces yet.</li>}
        {spaces.map((s) => (
          <li key={s.id} style={{ display: 'flex', justifyContent: 'space-between', gap: 8 }}>
            <span>
              <strong>{s.display_name}</strong>{' '}
              <span className="of-text-muted">({s.slug}) · {s.status}</span>
            </span>
            <button className="of-button of-button--ghost" onClick={() => void remove(s)} style={{ fontSize: 12 }}>
              Delete
            </button>
          </li>
        ))}
      </ul>
      <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <label style={{ fontSize: 12 }}>
          Slug
          <input className="of-input" value={slug} onChange={(e) => setSlug(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Display name
          <input className="of-input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Description
          <input className="of-input" value={description} onChange={(e) => setDescription(e.target.value)} style={{ marginTop: 4 }} />
        </label>
      </div>
      <div>
        <button className="of-button" onClick={() => void add()}>Create space</button>
      </div>
    </section>
  );
}

function MembershipProbe({ orgId, onError }: { orgId: string; onError: (msg: string) => void }) {
  const [userId, setUserId] = useState('');
  const [result, setResult] = useState<MembershipCheck | null>(null);

  async function probe() {
    try {
      setResult(await checkOrganizationMembership(orgId, userId.trim() || undefined));
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to check membership');
    }
  }

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <h3 className="of-heading-lg" style={{ fontSize: 14 }}>Membership probe</h3>
      <p className="of-text-muted" style={{ fontSize: 12 }}>
        Verify whether a user has membership in this organization via
        enrollment, admin grant, or active guest record. Empty user ID
        checks the currently authenticated caller.
      </p>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'flex-end' }}>
        <label style={{ fontSize: 12 }}>
          User ID (optional)
          <input className="of-input" value={userId} onChange={(e) => setUserId(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <button className="of-button" onClick={() => void probe()}>Check membership</button>
      </div>
      {result && (
        <pre style={{ padding: 12, background: 'var(--bg-subtle)', fontSize: 11, fontFamily: 'var(--font-mono)', borderRadius: 12, overflow: 'auto' }}>
          {JSON.stringify(result, null, 2)}
        </pre>
      )}
    </section>
  );
}
