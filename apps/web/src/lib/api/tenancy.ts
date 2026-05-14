// Client for tenancy-organizations-service (foundation slice + SG.2
// administrators / guests / spaces / membership).
//
// All endpoints live under /api/v1 on the tenancy service. The web app
// proxies through the edge gateway so the relative API_BASE in
// ./client is sufficient.

import api from './client';

export interface Organization {
  id: string;
  slug: string;
  display_name: string;
  description: string;
  contact_email: string | null;
  organization_type: string;
  default_workspace: string | null;
  tenant_tier: string | null;
  status: string;
  metadata: Record<string, unknown>;
  settings: Record<string, unknown>;
  quotas: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface OrganizationAdmin {
  organization_id: string;
  user_id: string;
  scope: string;
  granted_by: string | null;
  created_at: string;
}

export interface OrganizationGuest {
  organization_id: string;
  user_id: string;
  primary_organization_id: string;
  status: string;
  invited_by: string | null;
  expires_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface TenancySpace {
  id: string;
  organization_id: string;
  slug: string;
  display_name: string;
  description: string;
  settings: Record<string, unknown>;
  quotas: Record<string, unknown>;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface MembershipCheck {
  organization_id: string;
  user_id: string;
  is_member: boolean;
  is_admin: boolean;
}

interface ListEnvelope<T> {
  items: T[];
}

export async function listOrganizations(): Promise<Organization[]> {
  const res = await api.fetch<ListEnvelope<Organization>>('/organizations');
  return res.items;
}

export async function getOrganization(id: string): Promise<Organization> {
  return api.fetch<Organization>(`/organizations/${id}`);
}

export async function updateOrganization(
  id: string,
  patch: Partial<{
    display_name: string;
    description: string;
    contact_email: string | null;
    organization_type: string;
    default_workspace: string | null;
    tenant_tier: string | null;
    status: string;
    metadata: Record<string, unknown>;
    settings: Record<string, unknown>;
    quotas: Record<string, unknown>;
  }>,
): Promise<Organization> {
  return api.fetch<Organization>(`/organizations/${id}`, {
    method: 'PATCH',
    body: patch,
  });
}

export async function listOrganizationAdmins(orgId: string): Promise<OrganizationAdmin[]> {
  const res = await api.fetch<ListEnvelope<OrganizationAdmin>>(
    `/organizations/${orgId}/admins`,
  );
  return res.items;
}

export async function createOrganizationAdmin(
  orgId: string,
  body: { user_id: string; scope?: string; granted_by?: string },
): Promise<OrganizationAdmin> {
  return api.fetch<OrganizationAdmin>(`/organizations/${orgId}/admins`, {
    method: 'POST',
    body,
  });
}

export async function deleteOrganizationAdmin(
  orgId: string,
  userId: string,
  scope = 'enrollment_admin',
): Promise<void> {
  await api.fetch<void>(
    `/organizations/${orgId}/admins/${userId}?scope=${encodeURIComponent(scope)}`,
    { method: 'DELETE' },
  );
}

export async function listOrganizationGuests(orgId: string): Promise<OrganizationGuest[]> {
  const res = await api.fetch<ListEnvelope<OrganizationGuest>>(
    `/organizations/${orgId}/guests`,
  );
  return res.items;
}

export async function createOrganizationGuest(
  orgId: string,
  body: {
    user_id: string;
    primary_organization_id: string;
    status?: string;
    invited_by?: string;
    expires_at?: string;
  },
): Promise<OrganizationGuest> {
  return api.fetch<OrganizationGuest>(`/organizations/${orgId}/guests`, {
    method: 'POST',
    body,
  });
}

export async function deleteOrganizationGuest(
  orgId: string,
  userId: string,
): Promise<void> {
  await api.fetch<void>(`/organizations/${orgId}/guests/${userId}`, { method: 'DELETE' });
}

export async function listTenancySpaces(orgId: string): Promise<TenancySpace[]> {
  const res = await api.fetch<ListEnvelope<TenancySpace>>(
    `/organizations/${orgId}/spaces`,
  );
  return res.items;
}

export async function createTenancySpace(
  orgId: string,
  body: {
    slug: string;
    display_name: string;
    description?: string;
    settings?: Record<string, unknown>;
    quotas?: Record<string, unknown>;
    status?: string;
  },
): Promise<TenancySpace> {
  return api.fetch<TenancySpace>(`/organizations/${orgId}/spaces`, {
    method: 'POST',
    body,
  });
}

export async function updateTenancySpace(
  spaceId: string,
  patch: Partial<{
    display_name: string;
    description: string;
    settings: Record<string, unknown>;
    quotas: Record<string, unknown>;
    status: string;
  }>,
): Promise<TenancySpace> {
  return api.fetch<TenancySpace>(`/tenancy-spaces/${spaceId}`, {
    method: 'PATCH',
    body: patch,
  });
}

export async function deleteTenancySpace(spaceId: string): Promise<void> {
  await api.fetch<void>(`/tenancy-spaces/${spaceId}`, { method: 'DELETE' });
}

export async function checkOrganizationMembership(
  orgId: string,
  userId?: string,
): Promise<MembershipCheck> {
  const qs = userId ? `?user_id=${encodeURIComponent(userId)}` : '';
  return api.fetch<MembershipCheck>(`/organizations/${orgId}/membership${qs}`);
}
