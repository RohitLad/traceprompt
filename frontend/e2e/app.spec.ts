import { expect, test } from '@playwright/test';

test('navbar renders and navigates to traces', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByTestId('navbar')).toContainText('traceprompt');
	await page.getByRole('link', { name: 'Traces' }).click();
	await expect(page).toHaveURL(/.*#\/traces/);
	await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible();
});

test('login page renders sign-in form', async ({ page }) => {
	await page.goto('/#/login');
	await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
	await expect(page.getByPlaceholder('Email')).toBeVisible();
	await expect(page.getByPlaceholder(/Password/)).toBeVisible();
});

test('navbar exposes all product areas', async ({ page }) => {
	await page.goto('/');
	const nav = page.getByTestId('navbar');
	for (const label of ['Dashboard', 'Traces', 'Sessions', 'Scores', 'Evals', 'Prompts', 'Playground', 'Datasets']) {
		await expect(nav.getByRole('link', { name: label })).toBeVisible();
	}
});
