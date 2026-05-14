// SG.4: user-administration client. Mirrors the identity-federation-service
// admin surface (search/list, preregister, soft-delete/undelete,
// inspect, revoke tokens).

import api from './client';

export interface AdminUser {
  id: string;
  email: string;
  username: string | null;
  name: string;
  is_active: boolean;
  auth_source: string;
  realm: string;
  mfa_enforced: boolean;
  organization_id: string | null;
  attributes?: Record<string, unknown> | null;
  last_login_at: string | null;
  last_login_ip: string | null;
  preregistered: boolean;
  invited_by: string | null;
  deleted_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface UserGroupBrief {
  id: string;
  name: string;
}

export interface UserTokenSummary {
  active_count: number;
  revoked_count: number;
  next_expires_at?: string | null;
  api_keys_active: number;
}

export interface UserExternalBinding {
  provider: string;
  external_id: string;
  email: string;
  last_login_at?: string | null;
  created_at: string;
}

export interface UserInspection {
  user: AdminUser;
  roles: string[];
  groups: UserGroupBrief[];
  tokens: UserTokenSummary;
  external_identities: UserExternalBinding[];
}

export interface SearchUsersFilter {
  q?: string;
  organization_id?: string;
  realm?: string;
  status?: 'active' | 'inactive';
  include_deleted?: boolean;
  limit?: number;
  offset?: number;
}

export interface SearchUsersResponse {
  items: AdminUser[];
  total: number;
}

function toQuery(filter: SearchUsersFilter): string {
  const params = new URLSearchParams();
  if (filter.q) params.set('q', filter.q);
  if (filter.organization_id) params.set('organization_id', filter.organization_id);
  if (filter.realm) params.set('realm', filter.realm);
  if (filter.status) params.set('status', filter.status);
  if (filter.include_deleted) params.set('include_deleted', 'true');
  if (filter.limit !== undefined) params.set('limit', String(filter.limit));
  if (filter.offset !== undefined) params.set('offset', String(filter.offset));
  const q = params.toString();
  return q ? `?${q}` : '';
}

export function searchUsers(filter: SearchUsersFilter = {}): Promise<SearchUsersResponse> {
  return api.get<SearchUsersResponse>(`/users/search${toQuery(filter)}`);
}

export function inspectUser(userId: string): Promise<UserInspection> {
  return api.get<UserInspection>(`/users/${userId}/inspect`);
}

export function patchUser(
  userId: string,
  patch: Partial<{
    name: string;
    username: string;
    realm: string;
    is_active: boolean;
    mfa_enforced: boolean;
    organization_id: string | null;
    attributes: Record<string, unknown>;
  }>,
): Promise<AdminUser> {
  return api.fetch<AdminUser>(`/users/${userId}`, { method: 'PATCH', body: patch });
}

export function softDeleteUser(userId: string): Promise<void> {
  return api.delete<void>(`/users/${userId}`);
}

export function hardDeleteUser(userId: string): Promise<void> {
  return api.delete<void>(`/users/${userId}?hard=true`);
}

export function restoreUser(userId: string): Promise<AdminUser> {
  return api.post<AdminUser>(`/users/${userId}/restore`, {});
}

export function revokeUserTokens(userId: string): Promise<{ user_id: string; revoked: number }> {
  return api.post<{ user_id: string; revoked: number }>(`/users/${userId}/revoke-tokens`, {});
}

export interface PreregisterUserBody {
  email: string;
  name: string;
  username?: string;
  realm?: string;
  organization_id?: string;
  attributes?: Record<string, unknown>;
  roles?: string[];
  groups?: string[];
}

export function preregisterUser(body: PreregisterUserBody): Promise<AdminUser> {
  return api.post<AdminUser>('/users/preregister', body);
}
