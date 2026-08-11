import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		// The dev server proxies the API to the Go backend so cookies and
		// fetches behave exactly as they do in the built app.
		proxy: {
			'/api': 'http://localhost:8080',
		},
	},
});
