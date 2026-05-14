// SG.4: User administration — Control Panel UI.
//
// Mirrors the identity-federation-service admin endpoints:
//   - search + paginate users by q / organization_id / realm /
//     status / include_deleted
//   - preregister a new user (admin-only)
//   - activate / deactivate / soft-delete / restore / revoke tokens
//   - inspect a user (roles, groups, token summary, IdP bindings)

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  inspectUser,
  patchUser,
  preregisterUser,
  restoreUser,
  revokeUserTokens,
  searchUsers,
  softDeleteUser,
  type AdminUser,
  type SearchUsersFilter,
  type UserInspection,
} from '@/lib/api/users-admin';

export function UsersPage() {
  const [filter, setFilter] = useState<SearchUsersFilter>({ limit: 50, offset: 0 });
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [inspection, setInspection] = useState<UserInspection | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const res = await searchUsers(filter);
      setUsers(res.items);
      setTotal(res.total);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load users');
    } finally {
      setLoading(false);
    }
  }, [filter]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const loadInspection = useCallback(async (userId: string) => {
    try {
      const res = await inspectUser(userId);
      setInspection(res);
      setSelectedId(userId);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to inspect user');
    }
  }, []);

  async function toggleActive(u: AdminUser) {
    try {
      await patchUser(u.id, { is_active: !u.is_active });
      await refresh();
      if (selectedId === u.id) {
        await loadInspection(u.id);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to update user');
    }
  }

  async function deactivateAndSoftDelete(u: AdminUser) {
    if (!confirm(`Soft-delete user ${u.email}? Tokens will be revoked.`)) {
      return;
    }
    try {
      await softDeleteUser(u.id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to delete user');
    }
  }

  async function restore(u: AdminUser) {
    try {
      await restoreUser(u.id);
      await refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to restore user');
    }
  }

  async function revokeTokens(u: AdminUser) {
    try {
      const res = await revokeUserTokens(u.id);
      setError(`Revoked ${res.revoked} refresh token(s) for ${u.email}.`);
      if (selectedId === u.id) {
        await loadInspection(u.id);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to revoke tokens');
    }
  }

  const pages = Math.max(1, Math.ceil(total / (filter.limit ?? 50)));
  const currentPage = Math.floor((filter.offset ?? 0) / (filter.limit ?? 50)) + 1;

  return (
    <section className="of-page" style={{ padding: 24, display: 'grid', gap: 16 }}>
      <Link to="/control-panel" style={{ color: 'var(--text-muted)', fontSize: 13 }}>
        ← Control panel
      </Link>

      <header>
        <h1 className="of-heading-xl">Users</h1>
        <p className="of-text-muted" style={{ marginTop: 4 }}>
          Search, preregister, activate / inactivate, soft-delete / restore, inspect, and revoke
          refresh tokens. Deactivation automatically revokes refresh tokens. See{' '}
          <a href="/docs/security-governance/identity-and-access" target="_blank" rel="noreferrer">
            identity and access
          </a>{' '}
          for the parity scope.
        </p>
      </header>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      <FilterBar filter={filter} onChange={setFilter} />

      <PreregisterSection onCreated={() => void refresh()} onError={setError} />

      <section className="of-panel" style={{ padding: 0, overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead style={{ background: 'var(--bg-subtle)', textAlign: 'left' }}>
            <tr>
              <th style={{ padding: 10 }}>Email</th>
              <th style={{ padding: 10 }}>Name</th>
              <th style={{ padding: 10 }}>Realm</th>
              <th style={{ padding: 10 }}>Status</th>
              <th style={{ padding: 10 }}>Last login</th>
              <th style={{ padding: 10 }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {loading && (
              <tr>
                <td colSpan={6} style={{ padding: 10 }} className="of-text-muted">
                  Loading…
                </td>
              </tr>
            )}
            {!loading && users.length === 0 && (
              <tr>
                <td colSpan={6} style={{ padding: 10 }} className="of-text-muted">
                  No users match the filter.
                </td>
              </tr>
            )}
            {users.map((u) => (
              <tr key={u.id} style={{ borderTop: '1px solid var(--border-subtle)' }}>
                <td style={{ padding: 10 }}>
                  <button
                    onClick={() => void loadInspection(u.id)}
                    style={{
                      background: 'transparent',
                      border: 'none',
                      padding: 0,
                      color: 'var(--text-accent)',
                      cursor: 'pointer',
                      textDecoration: 'underline',
                    }}
                  >
                    {u.email}
                  </button>
                  {u.username && (
                    <div className="of-text-muted" style={{ fontSize: 11 }}>
                      @{u.username}
                    </div>
                  )}
                </td>
                <td style={{ padding: 10 }}>{u.name}</td>
                <td style={{ padding: 10 }}>{u.realm}</td>
                <td style={{ padding: 10 }}>{renderStatus(u)}</td>
                <td style={{ padding: 10 }}>
                  {u.last_login_at ? new Date(u.last_login_at).toLocaleString() : '—'}
                </td>
                <td style={{ padding: 10 }}>
                  <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
                    <button className="of-button of-button--ghost" onClick={() => void toggleActive(u)}>
                      {u.is_active ? 'Deactivate' : 'Activate'}
                    </button>
                    <button className="of-button of-button--ghost" onClick={() => void revokeTokens(u)}>
                      Revoke tokens
                    </button>
                    {u.deleted_at ? (
                      <button className="of-button of-button--ghost" onClick={() => void restore(u)}>
                        Restore
                      </button>
                    ) : (
                      <button
                        className="of-button of-button--ghost"
                        style={{ color: '#b91c1c' }}
                        onClick={() => void deactivateAndSoftDelete(u)}
                      >
                        Soft-delete
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <Pagination
        currentPage={currentPage}
        pages={pages}
        total={total}
        onJump={(page) =>
          setFilter((prev) => ({
            ...prev,
            offset: Math.max(0, (page - 1) * (prev.limit ?? 50)),
          }))
        }
      />

      {inspection && <InspectionPanel inspection={inspection} onClose={() => setInspection(null)} />}
    </section>
  );
}

function renderStatus(u: AdminUser) {
  if (u.deleted_at) {
    return <span style={{ color: '#b91c1c' }}>deleted</span>;
  }
  if (!u.is_active) {
    return <span style={{ color: '#92400e' }}>inactive</span>;
  }
  if (u.preregistered) {
    return <span style={{ color: '#0369a1' }}>preregistered</span>;
  }
  return <span style={{ color: '#15803d' }}>active</span>;
}

function FilterBar({
  filter,
  onChange,
}: {
  filter: SearchUsersFilter;
  onChange: (next: SearchUsersFilter) => void;
}) {
  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <h2 className="of-heading-lg" style={{ fontSize: 14, textTransform: 'uppercase', letterSpacing: '0.12em' }}>
        Search
      </h2>
      <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <label style={{ fontSize: 12 }}>
          Query (email / username / name)
          <input
            className="of-input"
            value={filter.q ?? ''}
            onChange={(e) => onChange({ ...filter, q: e.target.value, offset: 0 })}
            style={{ marginTop: 4 }}
          />
        </label>
        <label style={{ fontSize: 12 }}>
          Organization ID
          <input
            className="of-input"
            value={filter.organization_id ?? ''}
            onChange={(e) =>
              onChange({ ...filter, organization_id: e.target.value || undefined, offset: 0 })
            }
            style={{ marginTop: 4 }}
          />
        </label>
        <label style={{ fontSize: 12 }}>
          Realm
          <input
            className="of-input"
            value={filter.realm ?? ''}
            onChange={(e) => onChange({ ...filter, realm: e.target.value || undefined, offset: 0 })}
            style={{ marginTop: 4 }}
          />
        </label>
        <label style={{ fontSize: 12 }}>
          Status
          <select
            className="of-input"
            value={filter.status ?? ''}
            onChange={(e) =>
              onChange({
                ...filter,
                status: (e.target.value || undefined) as 'active' | 'inactive' | undefined,
                offset: 0,
              })
            }
            style={{ marginTop: 4 }}
          >
            <option value="">any</option>
            <option value="active">active</option>
            <option value="inactive">inactive</option>
          </select>
        </label>
        <label style={{ fontSize: 12, display: 'flex', alignItems: 'flex-end', gap: 6 }}>
          <input
            type="checkbox"
            checked={Boolean(filter.include_deleted)}
            onChange={(e) => onChange({ ...filter, include_deleted: e.target.checked, offset: 0 })}
          />
          Include soft-deleted
        </label>
      </div>
    </section>
  );
}

function Pagination({
  currentPage,
  pages,
  total,
  onJump,
}: {
  currentPage: number;
  pages: number;
  total: number;
  onJump: (page: number) => void;
}) {
  return (
    <div className="of-text-muted" style={{ display: 'flex', gap: 12, alignItems: 'center', fontSize: 12 }}>
      <span>
        Page {currentPage} / {pages} · {total} user(s)
      </span>
      <button
        className="of-button of-button--ghost"
        disabled={currentPage <= 1}
        onClick={() => onJump(currentPage - 1)}
      >
        ← Prev
      </button>
      <button
        className="of-button of-button--ghost"
        disabled={currentPage >= pages}
        onClick={() => onJump(currentPage + 1)}
      >
        Next →
      </button>
    </div>
  );
}

function PreregisterSection({
  onCreated,
  onError,
}: {
  onCreated: () => void;
  onError: (msg: string) => void;
}) {
  const [email, setEmail] = useState('');
  const [name, setName] = useState('');
  const [username, setUsername] = useState('');
  const [realm, setRealm] = useState('local');
  const [organizationID, setOrganizationID] = useState('');
  const [roles, setRoles] = useState('viewer');
  const [busy, setBusy] = useState(false);

  async function create() {
    if (!email.trim() || !name.trim()) {
      onError('email and name are required');
      return;
    }
    setBusy(true);
    try {
      await preregisterUser({
        email: email.trim(),
        name: name.trim(),
        username: username.trim() || undefined,
        realm: realm.trim() || undefined,
        organization_id: organizationID.trim() || undefined,
        roles: roles
          .split(',')
          .map((r) => r.trim())
          .filter((r) => r.length > 0),
      });
      setEmail('');
      setName('');
      setUsername('');
      setOrganizationID('');
      onCreated();
    } catch (cause) {
      onError(cause instanceof Error ? cause.message : 'Failed to preregister user');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <h2 className="of-heading-lg" style={{ fontSize: 14, textTransform: 'uppercase', letterSpacing: '0.12em' }}>
        Preregister user
      </h2>
      <p className="of-text-muted" style={{ fontSize: 12 }}>
        Admin-only. Seeds a user row before they sign up. SSO callback or self-service registration promotes them to active.
      </p>
      <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))' }}>
        <label style={{ fontSize: 12 }}>
          Email
          <input className="of-input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Name
          <input className="of-input" value={name} onChange={(e) => setName(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Username (optional)
          <input className="of-input" value={username} onChange={(e) => setUsername(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Realm
          <input className="of-input" value={realm} onChange={(e) => setRealm(e.target.value)} style={{ marginTop: 4 }} />
        </label>
        <label style={{ fontSize: 12 }}>
          Organization ID (optional)
          <input
            className="of-input"
            value={organizationID}
            onChange={(e) => setOrganizationID(e.target.value)}
            style={{ marginTop: 4 }}
          />
        </label>
        <label style={{ fontSize: 12 }}>
          Roles (comma-separated)
          <input className="of-input" value={roles} onChange={(e) => setRoles(e.target.value)} style={{ marginTop: 4 }} />
        </label>
      </div>
      <div>
        <button className="of-button of-button--primary" disabled={busy} onClick={() => void create()}>
          {busy ? 'Creating…' : 'Preregister'}
        </button>
      </div>
    </section>
  );
}

function InspectionPanel({
  inspection,
  onClose,
}: {
  inspection: UserInspection;
  onClose: () => void;
}) {
  const u = inspection.user;
  const roles = useMemo(() => inspection.roles.join(', ') || '—', [inspection.roles]);
  return (
    <section className="of-panel" style={{ padding: 16, display: 'grid', gap: 8 }}>
      <header style={{ display: 'flex', justifyContent: 'space-between' }}>
        <h2 className="of-heading-lg" style={{ fontSize: 14, textTransform: 'uppercase', letterSpacing: '0.12em' }}>
          Inspect: {u.email}
        </h2>
        <button className="of-button of-button--ghost" onClick={onClose}>
          Close
        </button>
      </header>
      <dl style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px', fontSize: 12 }}>
        <dt className="of-text-muted">ID</dt>
        <dd><code>{u.id}</code></dd>
        <dt className="of-text-muted">Username</dt>
        <dd>{u.username ?? '—'}</dd>
        <dt className="of-text-muted">Realm</dt>
        <dd>{u.realm}</dd>
        <dt className="of-text-muted">Auth source</dt>
        <dd>{u.auth_source}</dd>
        <dt className="of-text-muted">Roles</dt>
        <dd>{roles}</dd>
        <dt className="of-text-muted">Groups</dt>
        <dd>{inspection.groups.length > 0 ? inspection.groups.map((g) => g.name).join(', ') : '—'}</dd>
        <dt className="of-text-muted">Tokens (active)</dt>
        <dd>{inspection.tokens.active_count} (api keys: {inspection.tokens.api_keys_active})</dd>
        <dt className="of-text-muted">Tokens (revoked)</dt>
        <dd>{inspection.tokens.revoked_count}</dd>
        <dt className="of-text-muted">Last login</dt>
        <dd>{u.last_login_at ? `${new Date(u.last_login_at).toLocaleString()} (${u.last_login_ip ?? 'unknown ip'})` : '—'}</dd>
        <dt className="of-text-muted">External identities</dt>
        <dd>
          {inspection.external_identities.length === 0
            ? '—'
            : inspection.external_identities.map((ei) => `${ei.provider}:${ei.external_id}`).join(', ')}
        </dd>
        <dt className="of-text-muted">Soft-deleted</dt>
        <dd>{u.deleted_at ? new Date(u.deleted_at).toLocaleString() : 'no'}</dd>
      </dl>
    </section>
  );
}
