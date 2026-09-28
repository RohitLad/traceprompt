import { expect, test } from '@playwright/test';

test('navbar renders and navigates to traces', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByTestId('navbar')).toContainText('traceprompt');
	await page.getByRole('link', { name: 'Traces' }).click();
	await expect(page).toHaveURL(/.*#\/traces/);
	await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible();
});
