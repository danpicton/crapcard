import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { prefs, FALLBACK_PAGE_SIZE } from './prefs.svelte';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' },
	});
}

describe('preferences', () => {
	beforeEach(() => {
		localStorage.clear();
		vi.stubGlobal('fetch', vi.fn());
		prefs.reset();
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('uses the deployment default from the server', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ page_size: 25, max_page_size: 500 }));

		await prefs.load();

		expect(prefs.pageSize).toBe(25);
	});

	it('falls back to a sane page size if the server cannot be asked', async () => {
		vi.mocked(fetch).mockRejectedValue(new TypeError('offline'));

		await prefs.load();

		expect(prefs.pageSize).toBe(FALLBACK_PAGE_SIZE);
	});

	it('lets the user override the deployment default', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ page_size: 25, max_page_size: 500 }));
		await prefs.load();

		prefs.setPageSize(10);

		expect(prefs.pageSize).toBe(10);
	});

	it('remembers the user override across a reload', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ page_size: 25, max_page_size: 500 }));
		await prefs.load();
		prefs.setPageSize(10);

		prefs.reset();
		await prefs.load();

		expect(prefs.pageSize).toBe(10);
	});

	it('ignores a stored override that is not a usable number', async () => {
		localStorage.setItem('crapcard-page-size', 'lots');
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ page_size: 25, max_page_size: 500 }));

		await prefs.load();

		expect(prefs.pageSize).toBe(25);
	});

	it('refuses a page size the server would reject anyway', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ page_size: 25, max_page_size: 100 }));
		await prefs.load();

		prefs.setPageSize(100000);
		expect(prefs.pageSize).toBe(100);

		prefs.setPageSize(0);
		expect(prefs.pageSize).toBe(100);
	});

	it('clears the override, returning to the deployment default', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ page_size: 25, max_page_size: 500 }));
		await prefs.load();
		prefs.setPageSize(10);

		prefs.clearPageSize();

		expect(prefs.pageSize).toBe(25);
	});
});
