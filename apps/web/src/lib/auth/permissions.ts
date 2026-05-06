import { useCurrentUser } from '@stores/auth';

export function usePermissions() {
  const user = useCurrentUser();
  const has = (permission: string) => user?.permissions.includes(permission) ?? false;

  return {
    canReadUsers: has('users:read') || has('users:write'),
    canManageUsers: has('users:write'),
    canReadRoles: has('roles:read') || has('roles:write'),
    canManageRoles: has('roles:write'),
    canReadGroups: has('groups:read') || has('groups:write'),
    canManageGroups: has('groups:write'),
    canReadPermissions: has('permissions:read') || has('permissions:write'),
    canManagePermissions: has('permissions:write'),
    canReadPolicies: has('policies:read') || has('policies:write'),
    canManagePolicies: has('policies:write'),
    canReadSso: has('sso:read') || has('sso:write'),
    canManageSso: has('sso:write'),
  };
}
