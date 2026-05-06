import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { createPermission } from '@api/auth';
import { usePermissions } from '@/lib/auth/permissions';
import { permissionsQuery, settingsQueryKeys } from './queries';
import { toOptionalString } from './utils';

interface PermissionsSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

export function PermissionsSection({ setNotice, setError }: PermissionsSectionProps) {
  const perms = usePermissions();
  const qc = useQueryClient();

  const result = useQuery({ ...permissionsQuery, enabled: perms.canReadPermissions });
  const permissions = result.data ?? [];

  const [resource, setResource] = useState('');
  const [action, setAction] = useState('');
  const [description, setDescription] = useState('');

  const createMutation = useMutation({
    mutationFn: () =>
      createPermission({ resource, action, description: toOptionalString(description) }),
    onSuccess: async () => {
      setResource('');
      setAction('');
      setDescription('');
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.permissions });
      // A new permission shifts the role-creation form, so refresh roles too.
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.roles });
      setNotice('Permission created.');
    },
    onError: (err) =>
      setError(err instanceof Error ? err.message : 'Failed to create permission'),
  });

  if (!perms.canReadPermissions) return null;

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">Permission catalog</p>
      <h2 className="of-heading-lg">Permission registry</h2>

      <div style={{ display: 'grid', gap: 8, marginTop: 24 }}>
        {permissions.map((permission) => (
          <div key={permission.id} className="of-panel-muted" style={{ padding: '12px 16px', fontSize: 13 }}>
            <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>
              {permission.resource}:{permission.action}
            </div>
            <div className="of-text-muted">{permission.description ?? 'No description'}</div>
          </div>
        ))}
        {result.isLoading && <p className="of-text-muted">Loading permissions…</p>}
        {!result.isLoading && permissions.length === 0 && (
          <p className="of-text-muted">No permissions registered.</p>
        )}
      </div>

      {perms.canManagePermissions && (
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
          <div className="of-eyebrow">Create permission</div>
          <input
            className="of-input"
            value={resource}
            onChange={(e) => setResource(e.target.value)}
            placeholder="Resource, e.g. notebooks"
            required
          />
          <input
            className="of-input"
            value={action}
            onChange={(e) => setAction(e.target.value)}
            placeholder="Action, e.g. read"
            required
          />
          <textarea
            className="of-textarea"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder="Description"
          />
          <button type="submit" className="of-btn of-btn-primary" disabled={createMutation.isPending}>
            {createMutation.isPending ? 'Saving…' : 'Create permission'}
          </button>
        </form>
      )}
    </section>
  );
}
