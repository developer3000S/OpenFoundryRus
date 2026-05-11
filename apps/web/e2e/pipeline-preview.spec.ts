import { expect, test } from '@playwright/test';

const pipelineId = '11111111-1111-1111-1111-111111111111';

const pipeline = {
  id: pipelineId,
  name: 'Trail running demo',
  description: 'Preview selected nodes without saving first.',
  owner_id: '00000000-0000-0000-0000-000000000001',
  dag: [
    {
      id: 'source_trails',
      label: 'Trail source',
      transform_type: 'external',
      config: {
        rows: [
          { trail: 'Anemone', distance: 2.9, gain: 700 },
          { trail: 'Mesa Trail', distance: 6.0, gain: 900 },
        ],
      },
      depends_on: [],
      input_dataset_ids: [],
      output_dataset_id: null,
    },
    {
      id: 'select_trails',
      label: 'Select fast trails',
      transform_type: 'select',
      config: { columns: ['trail', 'distance'] },
      depends_on: ['source_trails'],
      input_dataset_ids: [],
      output_dataset_id: null,
    },
  ],
  status: 'draft',
  schedule_config: { enabled: false, cron: null },
  retry_policy: { max_attempts: 1, retry_on_failure: false, allow_partial_reexecution: true },
  next_run_at: null,
  created_at: '2026-05-10T00:00:00Z',
  updated_at: '2026-05-10T00:00:00Z',
  pipeline_type: 'BATCH',
};

test('shows a selected pipeline node preview from the draft DAG', async ({ page }) => {
  await page.addInitScript(() => {
    window.localStorage.setItem('of_access_token', 'e2e-token');
    window.localStorage.setItem('of_refresh_token', 'e2e-refresh');
  });

  await page.route('**/api/v1/auth/bootstrap-status', async (route) => {
    await route.fulfill({ json: { requires_initial_admin: false } });
  });
  await page.route('**/api/v1/users/me', async (route) => {
    await route.fulfill({
      json: {
        id: '00000000-0000-0000-0000-000000000001',
        email: 'runner@example.com',
        name: 'Trail Runner',
        is_active: true,
        roles: ['admin'],
        groups: [],
        permissions: ['*'],
        organization_id: null,
        attributes: {},
        mfa_enabled: false,
        mfa_enforced: false,
        auth_source: 'local',
        created_at: '2026-05-10T00:00:00Z',
      },
    });
  });
  await page.route(`**/api/v1/pipelines/${pipelineId}`, async (route) => {
    await route.fulfill({ json: pipeline });
  });
  await page.route(`**/api/v1/pipelines/${pipelineId}/runs**`, async (route) => {
    await route.fulfill({ json: { data: [] } });
  });
  await page.route('**/api/v1/pipelines/_validate', async (route) => {
    await route.fulfill({
      json: {
        valid: true,
        errors: [],
        warnings: [],
        next_run_at: null,
        summary: { node_count: 2, edge_count: 1, root_node_ids: ['source_trails'], leaf_node_ids: ['select_trails'] },
      },
    });
  });
  await page.route(`**/api/v1/pipelines/${pipelineId}/nodes/select_trails/preview**`, async (route) => {
    const body = route.request().postDataJSON() as { dag?: unknown; sample_size?: number };
    expect(body.dag).toBeTruthy();
    expect(body.sample_size).toBe(50);
    await route.fulfill({
      json: {
        pipeline_id: pipelineId,
        node_id: 'select_trails',
        columns: ['trail', 'distance'],
        rows: [{ trail: 'Mesa Trail', distance: 6.0 }],
        sample_size: 50,
        generated_at: new Date().toISOString(),
        seed: 1,
        source_chain: ['source_trails', 'select_trails'],
        fresh: true,
      },
    });
  });

  await page.goto(`/pipelines/${pipelineId}/edit`);
  await page.getByText('Select fast trails').click();

  await expect(page.getByText('Preview')).toBeVisible();
  await expect(page.getByText('Mesa Trail')).toBeVisible();
  await expect(page.getByText('distance')).toBeVisible();
  await expect(page.getByText(/chain source_trails/)).toBeVisible();
});
