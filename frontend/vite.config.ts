/// <reference types="vitest/config" />
import tailwindcss from '@tailwindcss/vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

// https://vite.dev/config/
export default defineConfig({
	plugins: [tailwindcss(), svelte()],
	test: {
		environment: 'jsdom',
		include: ['src/**/*.test.ts'],
		coverage: { provider: 'v8', reporter: ['text', 'lcov'] }
	}
});
