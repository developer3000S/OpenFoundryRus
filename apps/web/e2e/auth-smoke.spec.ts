import { expect, test } from '@playwright/test';

test('renders the login flow entry point', async ({ page }) => {
  await page.route('**/api/v1/users/me', async (route) => {
    await route.fulfill({ status: 401, json: { error: 'unauthenticated' } });
  });
  await page.route('**/api/v1/auth/bootstrap-status', async (route) => {
    await route.fulfill({ json: { requires_initial_admin: false } });
  });
  await page.route('**/api/v1/auth/sso/providers', async (route) => {
    await route.fulfill({ json: [] });
  });

  await page.goto('/auth/login');

  await expect(page.getByAltText('OpenFoundry')).toBeVisible();
  await expect(page.locator('input[type="email"]')).toBeVisible();
  await expect(page.locator('button[type="submit"]')).toBeVisible();
});
