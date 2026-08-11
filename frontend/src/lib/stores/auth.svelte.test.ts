import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { auth } from './auth.svelte';
import { ApiError } from '$lib/api';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' },
	});
}

describe('auth store', () => {
	beforeEach(() => {
		vi.stubGlobal('fetch', vi.fn());
		auth.reset();
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('starts unauthenticated and unresolved', () => {
		expect(auth.user).toBeNull();
		expect(auth.ready).toBe(false);
	});

	it('resolves the signed-in user', async () => {
		vi.mocked(fetch).mockResolvedValue(
			jsonResponse({ id: 1, username: 'dan', is_admin: true, created_at: '' }),
		);

		await auth.refresh();

		expect(auth.user?.username).toBe('dan');
		expect(auth.ready).toBe(true);
	});

	it('treats a 401 as signed out rather than an error', async () => {
		// Not being logged in is the normal first visit, not a failure.
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ error: 'authentication required' }, 401));

		await auth.refresh();

		expect(auth.user).toBeNull();
		expect(auth.ready).toBe(true);
		expect(auth.error).toBeNull();
	});

	it('records a real failure so the UI can say something went wrong', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ error: 'boom' }, 500));

		await auth.refresh();

		expect(auth.error).toBe('boom');
		expect(auth.ready).toBe(true);
	});

	it('signs in and holds the user', async () => {
		vi.mocked(fetch)
			.mockResolvedValueOnce(jsonResponse({ ok: true }))
			.mockResolvedValueOnce(
				jsonResponse({ id: 1, username: 'dan', is_admin: true, created_at: '' }),
			);

		await auth.login('dan', 'hunter2hunter2');

		expect(auth.user?.username).toBe('dan');
	});

	it('reports bad credentials without signing in', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ error: 'invalid credentials' }, 401));

		await expect(auth.login('dan', 'wrong')).rejects.toBeInstanceOf(ApiError);
		expect(auth.user).toBeNull();
	});

	it('clears the user on sign out even if the request fails', async () => {
		vi.mocked(fetch)
			.mockResolvedValueOnce(jsonResponse({ ok: true }))
			.mockResolvedValueOnce(
				jsonResponse({ id: 1, username: 'dan', is_admin: true, created_at: '' }),
			);
		await auth.login('dan', 'hunter2hunter2');

		vi.mocked(fetch).mockRejectedValue(new TypeError('offline'));
		await auth.logout();

		// Leaving a stale user on screen after a logout the user asked for
		// would be worse than a lost request.
		expect(auth.user).toBeNull();
	});
});
