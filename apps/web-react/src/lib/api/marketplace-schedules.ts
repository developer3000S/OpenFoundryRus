import { api } from './client';

const MARKETPLACE_BASE = '/marketplace';

export interface ProductScheduleManifest {
  name: string;
  description: string;
  trigger: unknown;
  target: unknown;
  scope_kind: string;
  defaults: {
    time_zone?: string;
    timezone_override?: string;
    force_build?: boolean;
  };
}

export function listProductScheduleManifests(productId: string) {
  return api.get<{ manifests: ProductScheduleManifest[] }>(
    `${MARKETPLACE_BASE}/products/${encodeURIComponent(productId)}/schedules`,
  );
}

export function previewInstallSchedules(
  productId: string,
  body: {
    product_version_id: string;
    rid_mapping?: { pipeline?: Record<string, string>; dataset?: Record<string, string> };
    activate_manifests?: string[];
  },
) {
  return api.post<{ product_version_id: string; materialised: ProductScheduleManifest[] }>(
    `${MARKETPLACE_BASE}/products/${encodeURIComponent(productId)}/install:schedules`,
    body,
  );
}
