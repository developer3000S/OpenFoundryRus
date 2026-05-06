import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { createRole } from '@api/auth';
import { usePermissions } from '@/lib/auth/permissions';
import { permissionsQuery, rolesQuery, settingsQueryKeys } from './queries';
import { toOptionalString } from './utils';

interface RolesSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

export function RolesSection({ setNotice, setError }: RolesSectionProps) {
  const perms = usePermissions();
  const qc = useQueryClient();

  const rolesResult = useQuery({ ...rolesQuery, enabled: perms.canReadRoles });
  const permissionsResult = useQuery({
    ...permissionsQuery,
    enabled: perms.canReadRoles && perms.canManageRoles,
  });

  const roles = rolesResult.data ?? [];
  const permissions = permissionsResult.data ?? [];

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [permissionIds, setPermissionIds] = useState<string[]>([]);

  const createMutation = useMutation({
    mutationFn: () =>
      createRole({ name, description: toOptionalString(description), permission_ids: permissionIds }),
    onSuccess: async () => {
      setName('');
      setDescription('');
      setPermissionIds([]);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.roles });
      setNotice('Role created.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to create role'),
  });

  if (!perms.canReadRoles) return null;

  function togglePermission(id: string) {
    setPermissionIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">RBAC</p>
      <h2 className="of-heading-lg">Roles and permissions</h2>

      <div style={{ display: 'grid', gap: 12, marginTop: 24 }}>
        {roles.map((role) => (
          <article key={role.id} className="of-panel-muted" style={{ padding: 16 }}>
            <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
              <div>
                <h3 className="of-heading-sm">{role.name}</h3>
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  {role.description ?? 'No description'}
                </p>
              </div>
              <span className="of-eyebrow" style={{ whiteSpace: 'nowrap' }}>
                {role.permissions.length} permissions
              </span>
            </header>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
              {role.permissions.map((permission) => (
                <span key={permission} className="of-chip of-chip-active">
                  {permission}
                </span>
              ))}
            </div>
          </article>
        ))}
        {rolesResult.isLoading && <p className="of-text-muted">Loading roles…</p>}
      </div>

      {perms.canManageRoles && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            createMutation.mutate();
          }}
          style={{
            display: 'grid',
            gap: 12,
            marginTop: 24,
            padding: 20,
            border: '1px dashed var(--border-default)',
            borderRadius: 'var(--radius-md)',
          }}
        >
          <div className="of-eyebrow">Create role</div>
          <input
            className="of-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Role name"
            required
          />
          <textarea
            className="of-textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder="Role description"
          />
          <div>
            <div style={{ marginBottom: 8, fontSize: 13, fontWeight: 500 }}>Permissions</div>
            <div
              className="of-scrollbar"
              style={{
                display: 'grid',
                gap: 6,
                maxHeight: 200,
                overflow: 'auto',
                padding: 12,
                border: '1px solid var(--border-default)',
                borderRadius: 'var(--radius-md)',
                fontSize: 13,
              }}
            >
              {permissions.map((permission) => (
                <label key={permission.id} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input
                    type="checkbox"
                    checked={permissionIds.includes(permission.id)}
                    onChange={() => togglePermission(permission.id)}
                  />
                  <span>
                    {permission.resource}:{permission.action}
                  </span>
                </label>
              ))}
              {permissions.length === 0 && (
                <span className="of-text-muted">
                  {permissionsResult.isLoading ? 'Loading permissions…' : 'No permissions available.'}
                </span>
              )}
            </div>
          </div>
          <button type="submit" className="of-btn of-btn-primary" disabled={createMutation.isPending}>
            {createMutation.isPending ? 'Saving…' : 'Create role'}
          </button>
        </form>
      )}
    </section>
  );
}
