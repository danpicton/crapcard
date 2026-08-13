import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// The whole app is served by the Go binary from an embedded copy of
		// this build, so it is prerendered to static files with a SPA
		// fallback for client-side routes.
		adapter: adapter({ fallback: 'index.html', strict: false }),
	},
};

export default config;
