import { test, expect } from '@playwright/test';

test('dashboard loads with three mocked profiles and no console errors', async ({ page }) => {
  const consoleErrors: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });

  await page.goto('/');

  await expect(page.getByRole('banner').getByText(/^ccx$/)).toBeVisible();
  await expect(page.getByTestId('profile-card')).toHaveCount(3);
  await expect(page.getByTestId('time-series-chart')).toBeVisible();
  await expect(page.getByText(/top projects/i)).toBeVisible();
  await expect(page.getByText(/recent sessions/i)).toBeVisible();
  expect(consoleErrors).toEqual([]);
});

test('profile picker filters projects', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('profile-card')).toHaveCount(3);

  await page.getByRole('button', { name: /filter/i }).click();
  await page.getByRole('menuitem', { name: /^work$/ }).click();

  await expect(page.getByTestId('profile-card')).toHaveCount(1);
});
