/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true"/>
/// <reference lib="esnext" />
/// <reference lib="webworker" />

/**
 * The offline shell. Build assets and static files are cached on install, so
 * the app opens and navigates with no network; API calls are never cached —
 * data freshness and the offline outbox are the app's job, not the worker's.
 */

const sw = self as unknown as ServiceWorkerGlobalScope;

import { build, files, version } from '$service-worker';

const CACHE = `crapcard-${version}`;

// Everything Vite emitted, everything in static/, and the SPA shell itself
// (the fallback page the Go server hands out for every client-side route).
const ASSETS = [...build, ...files, '/'];

sw.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(CACHE)
			.then((cache) => cache.addAll(ASSETS))
			.then(() => sw.skipWaiting()),
	);
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key))))
			.then(() => sw.clients.claim()),
	);
});

sw.addEventListener('fetch', (event) => {
	const { request } = event;
	if (request.method !== 'GET') return;

	const url = new URL(request.url);
	if (url.origin !== sw.location.origin) return;
	// Data and images stay live; the app degrades deliberately when offline.
	if (url.pathname.startsWith('/api/') || url.pathname === '/healthz') return;

	// Immutable build assets: cache-first, they never change under one name.
	if (ASSETS.includes(url.pathname)) {
		event.respondWith(
			caches.match(url.pathname).then((cached) => cached ?? fetch(request)),
		);
		return;
	}

	// Navigations: the network's copy when it is there, the cached shell when
	// it is not — this is what makes the app open offline at any route.
	if (request.mode === 'navigate') {
		event.respondWith(
			fetch(request).catch(async () => {
				const shell = await caches.match('/');
				return shell ?? Response.error();
			}),
		);
	}
});
