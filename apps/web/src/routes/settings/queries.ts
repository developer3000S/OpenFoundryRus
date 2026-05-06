import {
  getMfaStatus,
  listApiKeys,
  listGroups,
  listPermissions,
  listPolicies,
  listRestrictedViews,
  listRoles,
  listSsoProviders,
  listUsers,
} from '@api/auth';

export const settingsQueryKeys = {
  users: ['settings', 'users'] as const,
  permissions: ['settings', 'permissions'] as const,
  roles: ['settings', 'roles'] as const,
  groups: ['settings', 'groups'] as const,
  policies: ['settings', 'policies'] as const,
  restrictedViews: ['settings', 'restricted-views'] as const,
  mfa: ['settings', 'mfa'] as const,
  apiKeys: ['settings', 'api-keys'] as const,
  ssoProviders: ['settings', 'sso-providers'] as const,
};

export const usersQuery = {
  queryKey: settingsQueryKeys.users,
  queryFn: () => listUsers(),
};

export const permissionsQuery = {
  queryKey: settingsQueryKeys.permissions,
  queryFn: () => listPermissions(),
};

export const rolesQuery = {
  queryKey: settingsQueryKeys.roles,
  queryFn: () => listRoles(),
};

export const groupsQuery = {
  queryKey: settingsQueryKeys.groups,
  queryFn: () => listGroups(),
};

export const policiesQuery = {
  queryKey: settingsQueryKeys.policies,
  queryFn: () => listPolicies(),
};

export const restrictedViewsQuery = {
  queryKey: settingsQueryKeys.restrictedViews,
  queryFn: () => listRestrictedViews(),
};

export const mfaQuery = {
  queryKey: settingsQueryKeys.mfa,
  queryFn: () => getMfaStatus(),
};

export const apiKeysQuery = {
  queryKey: settingsQueryKeys.apiKeys,
  queryFn: () => listApiKeys(),
};

export const ssoProvidersQuery = {
  queryKey: settingsQueryKeys.ssoProviders,
  queryFn: () => listSsoProviders(),
};
