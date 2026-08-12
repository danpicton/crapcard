import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { api, ApiError } from './api';

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' },
	});
}

describe('api', () => {
	beforeEach(() => {
		vi.stubGlobal('fetch', vi.fn());
	});
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('sends cookies so the session travels with every request', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse([]));

		await api.listDecks();

		const [, init] = vi.mocked(fetch).mock.calls[0];
		expect(init?.credentials).toBe('same-origin');
	});

	it('parses a JSON response body', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse([{ id: 1, name: 'Italian' }]));

		const decks = await api.listDecks();

		expect(decks).toEqual([{ id: 1, name: 'Italian' }]);
	});

	it('throws ApiError carrying the status and the server message', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ error: 'deck not found' }, 404));

		await expect(api.getDeck(7)).rejects.toMatchObject({
			status: 404,
			message: 'deck not found',
		});
	});

	it('reports unauthenticated separately so the app can redirect to login', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ error: 'authentication required' }, 401));

		try {
			await api.me();
			throw new Error('expected a rejection');
		} catch (err) {
			expect(err).toBeInstanceOf(ApiError);
			expect((err as ApiError).unauthorized).toBe(true);
		}
	});

	it('returns null for the 204 that means the study queue is empty', async () => {
		// A finished deck is not an error, and 204 has no body to parse.
		vi.mocked(fetch).mockResolvedValue(new Response(null, { status: 204 }));

		await expect(api.nextCard(1)).resolves.toBeNull();
	});

	it('posts an answer as a rating integer', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ interval_label: '4d' }));

		await api.answerCard(12, 3);

		const [url, init] = vi.mocked(fetch).mock.calls[0];
		expect(url).toMatch(/^\/api\/cards\/12\/answer\?tz_offset=-?\d+$/);
		expect(init?.method).toBe('POST');
		expect(JSON.parse(init?.body as string)).toEqual({ rating: 3 });
	});

	it('sends a note as a field map, not an ordered array', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ id: 1 }, 201));

		await api.createNote({
			deck_id: 2,
			type: 'basic',
			reversed: true,
			fields: { front: 'ciao', back: 'hello' },
		});

		const [url, init] = vi.mocked(fetch).mock.calls[0];
		expect(url).toBe('/api/notes');
		expect(JSON.parse(init?.body as string)).toEqual({
			deck_id: 2,
			type: 'basic',
			reversed: true,
			fields: { front: 'ciao', back: 'hello' },
		});
	});

	it('uploads an image as raw bytes with its mime type', async () => {
		// This is what a clipboard paste hands us — no multipart wrapper.
		vi.mocked(fetch).mockResolvedValue(jsonResponse({ url: '/api/images/abc' }, 201));
		const blob = new Blob([new Uint8Array([1, 2, 3])], { type: 'image/png' });

		const result = await api.uploadImage(blob);

		const [url, init] = vi.mocked(fetch).mock.calls[0];
		expect(url).toBe('/api/images');
		expect(init?.method).toBe('POST');
		expect((init?.headers as Record<string, string>)['Content-Type']).toBe('image/png');
		expect(init?.body).toBe(blob);
		expect(result.url).toBe('/api/images/abc');
	});

	it('surfaces a network failure as an ApiError rather than a raw TypeError', async () => {
		vi.mocked(fetch).mockRejectedValue(new TypeError('Failed to fetch'));

		await expect(api.listDecks()).rejects.toBeInstanceOf(ApiError);
	});
});
