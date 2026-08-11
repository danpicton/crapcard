import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import path from 'path';

// Tests run against the plain Svelte plugin rather than the SvelteKit one, so
// the suite does not depend on a `svelte-kit sync` having been run. SvelteKit's
// $app modules are stubbed instead.
export default defineConfig({
	plugins: [svelte({ hot: false })],
	resolve: {
		conditions: ['browser'],
		alias: {
			$lib: path.resolve('./src/lib'),
			$app: path.resolve('./src/__mocks__/app'),
			'lucide-svelte': path.resolve('./src/__mocks__/lucide-svelte.ts'),
		},
	},
	test: {
		environment: 'jsdom',
		globals: true,
		setupFiles: ['./src/test-setup.ts'],
		include: ['src/**/*.{test,spec}.{js,ts}'],
	},
});
