import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
	testDir: './e2e',
	retries: 0,
	use: { baseURL: 'http://localhost:5173', trace: 'on-first-retry' },
	webServer: { command: 'npm run dev -- --port 5173', port: 5173, reuseExistingServer: true },
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
