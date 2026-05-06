import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { createGroup } from '@api/auth';
import { usePermissions } from '@/lib/auth/permissions';
import { groupsQuery, rolesQuery, settingsQueryKeys } from './queries';
import { toOptionalString } from './utils';

interface GroupsSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

export function GroupsSection({ setNotice, setError }: GroupsSectionProps) {
  const perms = usePermissions();
  const qc = useQueryClient();

  const groupsResult = useQuery({ ...groupsQuery, enabled: perms.canReadGroups });
  const rolesResult = useQuery({
    ...rolesQuery,
    enabled: perms.canReadGroups && perms.canManageGroups,
  });

  const groups = groupsResult.data ?? [];
  const roles = rolesResult.data ?? [];

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [roleIds, setRoleIds] = useState<string[]>([]);

  const createMutation = useMutation({
    mutationFn: () =>
      createGroup({ name, description: toOptionalString(description), role_ids: roleIds }),
    onSuccess: async () => {
      setName('');
      setDescription('');
      setRoleIds([]);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.groups });
      setNotice('Group created.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to create group'),
  });

  if (!perms.canReadGroups) return null;

  function toggleRole(id: string) {
    setRoleIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  }

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">Groups</p>
      <h2 className="of-heading-lg">Inherited access through groups</h2>

      <div style={{ display: 'grid', gap: 12, marginTop: 24 }}>
        {groups.map((group) => (
          <article key={group.id} className="of-panel-muted" style={{ padding: 16 }}>
            <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
              <div>
                <h3 className="of-heading-sm">{group.name}</h3>
                <p className="of-text-muted" style={{ fontSize: 13 }}>
                  {group.description ?? 'No description'}
                </p>
              </div>
              <span className="of-eyebrow" style={{ whiteSpace: 'nowrap' }}>
                {group.member_count} members
              </span>
            </header>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 12 }}>
              {group.roles.map((roleName) => (
                <span key={roleName} className="of-chip">
                  {roleName}
                </span>
              ))}
            </div>
          </article>
        ))}
        {groupsResult.isLoading && <p className="of-text-muted">Loading groups…</p>}
      </div>

      {perms.canManageGroups && (
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
          <div className="of-eyebrow">Create group</div>
          <input
            className="of-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Group name"
            required
          />
          <textarea
            className="of-textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder="Group description"
          />
          <div>
            <div style={{ marginBottom: 8, fontSize: 13, fontWeight: 500 }}>Attach roles</div>
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
              {roles.map((role) => (
                <label key={role.id} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input
                    type="checkbox"
                    checked={roleIds.includes(role.id)}
                    onChange={() => toggleRole(role.id)}
                  />
                  <span>{role.name}</span>
                </label>
              ))}
              {roles.length === 0 && (
                <span className="of-text-muted">
                  {rolesResult.isLoading ? 'Loading roles…' : 'No roles available.'}
                </span>
              )}
            </div>
          </div>
          <button type="submit" className="of-btn of-btn-primary" disabled={createMutation.isPending}>
            {createMutation.isPending ? 'Saving…' : 'Create group'}
          </button>
        </form>
      )}
    </section>
  );
}
