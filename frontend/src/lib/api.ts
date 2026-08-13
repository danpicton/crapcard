/**
 * Typed client for the crapcard API.
 *
 * Every call goes through request(), which keeps three things in one place:
 * cookies are always sent, non-2xx responses become ApiError carrying the
 * server's own message, and a network failure is wrapped rather than surfacing
 * as a bare TypeError.
 */

export class ApiError extends Error {
	readonly status: number;

	constructor(status: number, message: string) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
	}

	/** True when the session is missing or expired, so the app should log out. */
	get unauthorized(): boolean {
		return this.status === 401;
	}
}

export interface Deck {
	id: number;
	name: string;
	description: string;
	created_at: string;
	updated_at: string;
}

export interface CardSummary {
	id: number;
	template: string;
	state: string;
	due: string;
	reps: number;
	lapses: number;
	suspended: boolean;
}

export interface Note {
	id: number;
	deck_id: number;
	type: string;
	reversed: boolean;
	fields: Record<string, string>;
	cards?: CardSummary[];
	/** Null when no card of this note has ever been answered. */
	last_studied: string | null;
	created_at: string;
	updated_at: string;
}

export interface NoteInput {
	deck_id: number;
	type: string;
	reversed: boolean;
	fields: Record<string, string>;
}

export interface QueueCounts {
	new: number;
	learning: number;
	due: number;
	total: number;
}

export interface AnswerPreview {
	interval_seconds: number;
	label: string;
	due: string;
}

export interface StudyCard {
	card_id: number;
	note_id: number;
	deck_id: number;
	deck_name: string;
	template: string;
	question: string;
	answer: string;
	state: string;
	counts: QueueCounts;
	previews: Record<string, AnswerPreview>;
}

/** A whole study session's worth of cards, fetched in one go. */
export interface StudyQueue {
	deck_id: number;
	deck_name: string;
	counts: QueueCounts;
	cards: StudyCard[];
}

export interface AnswerResult {
	card_id: number;
	interval_seconds: number;
	interval_label: string;
	due: string;
	state: string;
	counts: QueueCounts;
}

export interface NoteType {
	type: string;
	fields: string[];
}

export interface User {
	id: number;
	username: string;
	is_admin: boolean;
	created_at: string;
}

export interface AppConfig {
	page_size: number;
	max_page_size: number;
}

/** One page of notes, with the total so a pager can be drawn. */
export interface NotePage {
	items: Note[];
	total: number;
	limit: number;
	offset: number;
}

/** One card of a note, rendered as the study screen would ask it. */
export interface CardPreview {
	template: string;
	question: string;
	answer: string;
}

export interface UploadedImage {
	id: string;
	url: string;
	mime_type: string;
	size: number;
}

/** Ratings, matching the server's persisted values. */
export const Rating = {
	Again: 1,
	Hard: 2,
	Good: 3,
	Easy: 4,
} as const;

export type RatingValue = (typeof Rating)[keyof typeof Rating];

/** The order the answer buttons appear in, and the keys previews come back under. */
export const RATING_KEYS = ['again', 'hard', 'good', 'easy'] as const;

interface RequestOptions {
	method?: string;
	body?: BodyInit | null;
	headers?: Record<string, string>;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
	let res: Response;
	try {
		res = await fetch(path, {
			method: options.method ?? 'GET',
			// The session is an HttpOnly cookie, so it has to ride along.
			credentials: 'same-origin',
			headers: options.headers,
			body: options.body ?? null,
		});
	} catch (err) {
		// fetch rejects with TypeError when the network is unreachable. Wrap it
		// so callers only ever have to handle one error type.
		throw new ApiError(0, err instanceof Error ? err.message : 'network error');
	}

	// 204 carries no body — the study queue uses it to mean "nothing due".
	if (res.status === 204) {
		return null as T;
	}

	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}

	if (!res.ok) {
		// An expired session makes every call 401. Bouncing to the sign-in
		// page beats surfacing "request failed (401)" on whatever button the
		// user happened to press. Auth endpoints are exempt: a failed login
		// is a message to show, not a redirect loop.
		if (res.status === 401 && !path.startsWith('/api/auth/')) {
			window.location.assign('/login');
		}
		const message =
			parsed && typeof parsed === 'object' && 'error' in parsed
				? String((parsed as { error: unknown }).error)
				: `request failed (${res.status})`;
		throw new ApiError(res.status, message);
	}

	return parsed as T;
}

/**
 * The client's UTC offset, sent with study calls so the server can gate
 * review cards on the end of *this* calendar day rather than an exact
 * timestamp — a card due at 14:00 belongs in the 09:00 session.
 */
function tzQuery(): string {
	return `tz_offset=${-new Date().getTimezoneOffset()}`;
}

function postJSON<T>(path: string, body: unknown, method = 'POST'): Promise<T> {
	return request<T>(path, {
		method,
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body),
	});
}

export const api = {
	config: () => request<AppConfig>('/api/config'),

	// ── Auth ────────────────────────────────────────────────────────────────
	setupStatus: () => request<{ needs_setup: boolean }>('/api/auth/setup-status'),
	setup: (username: string, password: string) =>
		postJSON<{ ok: boolean }>('/api/auth/setup', { username, password }),
	login: (username: string, password: string) =>
		postJSON<{ ok: boolean }>('/api/auth/login', { username, password }),
	logout: () => postJSON<{ ok: boolean }>('/api/auth/logout', {}),
	me: () => request<User>('/api/auth/me'),

	// ── Decks ───────────────────────────────────────────────────────────────
	listDecks: () => request<Deck[]>('/api/decks'),
	getDeck: (id: number) => request<Deck>(`/api/decks/${id}`),
	createDeck: (name: string, description = '') =>
		postJSON<Deck>('/api/decks', { name, description }),
	updateDeck: (id: number, name: string, description = '') =>
		postJSON<Deck>(`/api/decks/${id}`, { name, description }, 'PUT'),
	deleteDeck: (id: number) => request<null>(`/api/decks/${id}`, { method: 'DELETE' }),

	// ── Notes ───────────────────────────────────────────────────────────────
	noteTypes: () => request<NoteType[]>('/api/note-types'),
	listNotes: (opts: { deckId?: number; limit?: number; offset?: number } = {}) => {
		const params = new URLSearchParams();
		if (opts.deckId) params.set('deck_id', String(opts.deckId));
		if (opts.limit) params.set('limit', String(opts.limit));
		if (opts.offset) params.set('offset', String(opts.offset));
		const query = params.toString();
		return request<NotePage>(query ? `/api/notes?${query}` : '/api/notes');
	},
	previewNote: (id: number) => request<CardPreview[]>(`/api/notes/${id}/preview`),
	getNote: (id: number) => request<Note>(`/api/notes/${id}`),
	createNote: (input: NoteInput) => postJSON<Note>('/api/notes', input),
	updateNote: (id: number, input: Omit<NoteInput, 'type'>) =>
		postJSON<Note>(`/api/notes/${id}`, input, 'PUT'),
	deleteNote: (id: number) => request<null>(`/api/notes/${id}`, { method: 'DELETE' }),

	// ── Study ───────────────────────────────────────────────────────────────
	/** Returns null when nothing is due (the server answers 204). */
	nextCard: (deckId: number) =>
		request<StudyCard | null>(`/api/decks/${deckId}/study/next?${tzQuery()}`),
	/**
	 * The next due card without naming a deck — the landing screen's call.
	 * Null when nothing is due anywhere.
	 */
	nextCardAnywhere: () => request<StudyCard | null>(`/api/study/next?${tzQuery()}`),
	deckCounts: (deckId: number) =>
		request<QueueCounts>(`/api/decks/${deckId}/study/counts?${tzQuery()}`),
	/**
	 * Every card currently due, rendered — one fetch and the whole session
	 * can run without the network. Pass null for the deck the user would
	 * land on (null result when nothing is due anywhere).
	 */
	studyQueue: (deckId: number | null) =>
		deckId === null
			? request<StudyQueue | null>(`/api/study/queue?${tzQuery()}`)
			: request<StudyQueue | null>(`/api/decks/${deckId}/study/queue?${tzQuery()}`),
	answerCard: (cardId: number, rating: RatingValue | number) =>
		postJSON<AnswerResult>(`/api/cards/${cardId}/answer?${tzQuery()}`, { rating }),
	/**
	 * Reverts the most recent answer and returns the card, ready to be graded
	 * again. Null when there is nothing to undo (the server answers 204).
	 */
	undoAnswer: () => postJSON<StudyCard | null>(`/api/study/undo?${tzQuery()}`, {}),

	// ── Images ──────────────────────────────────────────────────────────────
	/**
	 * Uploads raw image bytes. A clipboard paste hands us a Blob directly, so
	 * it is posted as the body rather than wrapped in multipart form data.
	 */
	uploadImage: (blob: Blob) =>
		request<UploadedImage>('/api/images', {
			method: 'POST',
			headers: { 'Content-Type': blob.type || 'application/octet-stream' },
			body: blob,
		}),
};
