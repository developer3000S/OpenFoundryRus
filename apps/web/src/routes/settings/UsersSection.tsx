import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import {
  addUserToGroup,
  assignUserRole,
  deactivateUser,
  removeUserFromGroup,
  removeUserRole,
  updateUser,
  type UserProfile,
} from '@api/auth';
import { ConfirmDialog } from '@components/ConfirmDialog';
import { usePermissions } from '@/lib/auth/permissions';
import { groupsQuery, rolesQuery, settingsQueryKeys, usersQuery } from './queries';

interface UsersSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

export function UsersSection({ setNotice, setError }: UsersSectionProps) {
  const perms = usePermissions();
  const qc = useQueryClient();

  const usersResult = useQuery({ ...usersQuery, enabled: perms.canReadUsers });
  const rolesResult = useQuery({ ...rolesQuery, enabled: perms.canReadRoles });
  const groupsResult = useQuery({ ...groupsQuery, enabled: perms.canReadGroups });

  const users = usersResult.data ?? [];
  const roles = rolesResult.data ?? [];
  const groups = groupsResult.data ?? [];

  const [savingKey, setSavingKey] = useState<string | null>(null);
  const [selectedRoleByUser, setSelectedRoleByUser] = useState<Record<string, string>>({});
  const [selectedGroupByUser, setSelectedGroupByUser] = useState<Record<string, string>>({});
  const [deactivateConfirm, setDeactivateConfirm] = useState<{ user: UserProfile; busy: boolean } | null>(null);

  if (!perms.canReadUsers) return null;

  function roleIdByName(name: string) {
    return roles.find((r) => r.name === name)?.id;
  }

  function groupIdByName(name: string) {
    return groups.find((g) => g.name === name)?.id;
  }

  async function withSaving(key: string, work: () => Promise<void>) {
    setSavingKey(key);
    setError('');
    setNotice('');
    try {
      await work();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Request failed');
    } finally {
      setSavingKey(null);
    }
  }

  async function handleToggleUser(user: UserProfile) {
    if (user.is_active) {
      setDeactivateConfirm({ user, busy: false });
      return;
    }
    await withSaving(`user-${user.id}`, async () => {
      await updateUser(user.id, { is_active: true });
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.users });
      setNotice('User state updated.');
    });
  }

  async function confirmDeactivate() {
    if (!deactivateConfirm) return;
    const target = deactivateConfirm.user;
    setDeactivateConfirm({ user: target, busy: true });
    try {
      await withSaving(`user-${target.id}`, async () => {
        await deactivateUser(target.id);
        await qc.invalidateQueries({ queryKey: settingsQueryKeys.users });
        setNotice('User state updated.');
      });
    } finally {
      setDeactivateConfirm(null);
    }
  }

  async function handleToggleMfaEnforcement(user: UserProfile) {
    await withSaving(`user-mfa-${user.id}`, async () => {
      await updateUser(user.id, { mfa_enforced: !user.mfa_enforced });
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.users });
      setNotice('MFA enforcement updated.');
    });
  }

  async function handleAssignRole(userId: string) {
    const roleId = selectedRoleByUser[userId];
    if (!roleId) return;
    await withSaving(`assign-role-${userId}`, async () => {
      await assignUserRole(userId, roleId);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.users });
      setSelectedRoleByUser((prev) => ({ ...prev, [userId]: '' }));
      setNotice('Role assigned.');
    });
  }

  async function handleRemoveRole(userId: string, roleName: string) {
    const roleId = roleIdByName(roleName);
    if (!roleId) return;
    await withSaving(`remove-role-${userId}-${roleId}`, async () => {
      await removeUserRole(userId, roleId);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.users });
      setNotice('Role removed.');
    });
  }

  async function handleAddUserToGroup(userId: string) {
    const groupId = selectedGroupByUser[userId];
    if (!groupId) return;
    await withSaving(`assign-group-${userId}`, async () => {
      await addUserToGroup(userId, groupId);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.users });
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.groups });
      setSelectedGroupByUser((prev) => ({ ...prev, [userId]: '' }));
      setNotice('User added to group.');
    });
  }

  async function handleRemoveUserFromGroup(userId: string, groupName: string) {
    const groupId = groupIdByName(groupName);
    if (!groupId) return;
    await withSaving(`remove-group-${userId}-${groupId}`, async () => {
      await removeUserFromGroup(userId, groupId);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.users });
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.groups });
      setNotice('User removed from group.');
    });
  }

  return (
    <section className="of-panel" style={{ padding: 24, marginTop: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}>
        <div>
          <p className="of-eyebrow">User directory</p>
          <h2 className="of-heading-lg">Users, roles and group membership</h2>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(420px, 1fr))', marginTop: 24 }}>
        {users.map((user) => (
          <article key={user.id} className="of-panel-muted" style={{ padding: 20 }}>
            <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
              <div>
                <h3 className="of-heading-md">{user.name}</h3>
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  {user.email}
                </p>
              </div>
              <div style={{ textAlign: 'right', fontSize: 11 }}>
                <span className={`of-chip ${user.is_active ? 'of-status-success' : ''}`}>
                  {user.is_active ? 'Active' : 'Inactive'}
                </span>
                <div className="of-text-muted" style={{ marginTop: 6, textTransform: 'uppercase', letterSpacing: '0.18em' }}>
                  {user.auth_source}
                </div>
              </div>
            </header>

            <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr', marginTop: 16, fontSize: 13 }}>
              <div>
                <div className="of-eyebrow">Roles</div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
                  {user.roles.length > 0 ? (
                    user.roles.map((roleName) => (
                      <span key={roleName} className="of-chip of-chip-active">
                        {roleName}
                        {perms.canManageRoles && (
                          <button
                            type="button"
                            className="of-btn of-btn-ghost"
                            style={{ minHeight: 0, padding: '0 4px' }}
                            onClick={() => handleRemoveRole(user.id, roleName)}
                          >
                            ×
                          </button>
                        )}
                      </span>
                    ))
                  ) : (
                    <span className="of-text-soft">No direct roles</span>
                  )}
                </div>
              </div>

              <div>
                <div className="of-eyebrow">Groups</div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
                  {user.groups.length > 0 ? (
                    user.groups.map((groupName) => (
                      <span key={groupName} className="of-chip">
                        {groupName}
                        {perms.canManageGroups && (
                          <button
                            type="button"
                            className="of-btn of-btn-ghost"
                            style={{ minHeight: 0, padding: '0 4px' }}
                            onClick={() => handleRemoveUserFromGroup(user.id, groupName)}
                          >
                            ×
                          </button>
                        )}
                      </span>
                    ))
                  ) : (
                    <span className="of-text-soft">No groups</span>
                  )}
                </div>
              </div>
            </div>

            <div className="of-text-muted" style={{ marginTop: 12, fontSize: 12 }}>
              <strong style={{ color: 'var(--text-strong)' }}>Permissions:</strong> {user.permissions.length}
              <span style={{ margin: '0 8px' }}>·</span>
              <strong style={{ color: 'var(--text-strong)' }}>Org:</strong> {user.organization_id ?? 'Not assigned'}
            </div>

            {(perms.canManageUsers || perms.canManageRoles || perms.canManageGroups) && (
              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', marginTop: 16 }}>
                {perms.canManageUsers && (
                  <div className="of-panel" style={{ padding: 12 }}>
                    <div className="of-eyebrow">Identity controls</div>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 8 }}>
                      <button
                        type="button"
                        className="of-btn"
                        onClick={() => handleToggleUser(user)}
                        disabled={savingKey === `user-${user.id}`}
                      >
                        {user.is_active ? 'Deactivate user' : 'Reactivate user'}
                      </button>
                      <button
                        type="button"
                        className="of-btn"
                        onClick={() => handleToggleMfaEnforcement(user)}
                        disabled={savingKey === `user-mfa-${user.id}`}
                      >
                        {user.mfa_enforced ? 'Unset MFA enforcement' : 'Force MFA'}
                      </button>
                    </div>
                  </div>
                )}

                {(perms.canManageRoles || perms.canManageGroups) && (
                  <div className="of-panel" style={{ padding: 12 }}>
                    <div className="of-eyebrow">Access assignment</div>
                    {perms.canManageRoles && (
                      <div style={{ display: 'flex', gap: 6, marginTop: 8 }}>
                        <select
                          className="of-select"
                          value={selectedRoleByUser[user.id] ?? ''}
                          onChange={(e) =>
                            setSelectedRoleByUser((prev) => ({ ...prev, [user.id]: e.target.value }))
                          }
                        >
                          <option value="">Assign role...</option>
                          {roles.map((role) => (
                            <option key={role.id} value={role.id}>
                              {role.name}
                            </option>
                          ))}
                        </select>
                        <button
                          type="button"
                          className="of-btn of-btn-primary"
                          onClick={() => handleAssignRole(user.id)}
                          disabled={savingKey === `assign-role-${user.id}`}
                        >
                          Assign
                        </button>
                      </div>
                    )}
                    {perms.canManageGroups && (
                      <div style={{ display: 'flex', gap: 6, marginTop: 8 }}>
                        <select
                          className="of-select"
                          value={selectedGroupByUser[user.id] ?? ''}
                          onChange={(e) =>
                            setSelectedGroupByUser((prev) => ({ ...prev, [user.id]: e.target.value }))
                          }
                        >
                          <option value="">Add to group...</option>
                          {groups.map((group) => (
                            <option key={group.id} value={group.id}>
                              {group.name}
                            </option>
                          ))}
                        </select>
                        <button
                          type="button"
                          className="of-btn"
                          onClick={() => handleAddUserToGroup(user.id)}
                          disabled={savingKey === `assign-group-${user.id}`}
                        >
                          Add
                        </button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}
          </article>
        ))}
        {usersResult.isLoading && <p className="of-text-muted">Loading users…</p>}
        {!usersResult.isLoading && users.length === 0 && (
          <p className="of-text-muted">No users to display.</p>
        )}
      </div>

      <ConfirmDialog
        open={!!deactivateConfirm}
        title="Deactivate user"
        message={
          deactivateConfirm
            ? `Deactivate ${deactivateConfirm.user.name} (${deactivateConfirm.user.email})? They will lose access immediately.`
            : ''
        }
        confirmLabel="Deactivate"
        danger
        busy={deactivateConfirm?.busy ?? false}
        onConfirm={confirmDeactivate}
        onCancel={() => setDeactivateConfirm(null)}
      />
    </section>
  );
}
