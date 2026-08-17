import { api, ApiError, type NoteInput, type Note, type AnswerResult } from '$lib/api';

/**
 * Offline awareness and the outbox.
 *
 * Anything the user does that must reach the server — an answer, an autosaved
 * edit, a new card — goes through here when the network is away. Entries are
 * persisted (a closed tab must not lose work) and replayed in order the
 * moment the connection returns, so the server sees the same sequence of
 * events it would have seen online.
 */

const OUTBOX_KEY = 'crapcard-outbox';

/** How often to probe for the server while offline. */
const RECONNECT_PROBE_MS = 10_000;

export type OutboxEntry =
	| { id: string; kind: 'answer'; cardId: number; rating: number }
	| { id: string; kind: 'note-update'; noteId: number; input: NoteInput }
	| { id: string; kind: 'note-create'; ref: string; input: NoteInput };

interface SyncDeps {
	answerCard: (cardId: number, rating: number) => Promise<AnswerResult>;
	updateNote: (id: number, input: NoteInput) => Promise<Note>;
	createNote: (input: NoteInput) => Promise<Note>;
	/** A cheap reachability check — anything that resolves means online. */
	ping: () => Promise<void>;
}

const defaultDeps: SyncDeps = {
	answerCard: (cardId, rating) => api.answerCard(cardId, rating),
	updateNote: (id, input) => api.updateNote(id, input),
	createNote: (input) => api.createNote(input),
	ping: async () => {
		const res = await fetch('/healthz', { cache: 'no-store' });
		if (!res.ok) throw new Error('unreachable');
	},
};

function newId(): string {
	return crypto.randomUUID();
}

/** True for the failure shape that means "no network", not "server said no". */
function isNetworkFailure(err: unknown): boolean {
	return err instanceof ApiError && err.status === 0;
}

export function createSyncStore(deps: SyncDeps = defaultDeps) {
	let online = $state(true);
	let flushing = $state(false);
	let outbox = $state<OutboxEntry[]>([]);
	// Real note ids for creates that were queued offline, keyed by their ref,
	// so a composer still holding the ref can adopt the created note.
	let created = $state<Record<string, number>>({});
	let probeTimer: ReturnType<typeof setInterval> | null = null;
	let listenersBound = false;

	function persist() {
		try {
			localStorage.setItem(OUTBOX_KEY, JSON.stringify(outbox));
		} catch {
			// Storage full or unavailable: the queue still works in memory.
		}
	}

	function restore() {
		try {
			const raw = localStorage.getItem(OUTBOX_KEY);
			if (!raw) return;
			const parsed = JSON.parse(raw);
			if (Array.isArray(parsed)) outbox = parsed as OutboxEntry[];
		} catch {
			// A corrupt outbox must not brick the app.
			localStorage.removeItem(OUTBOX_KEY);
		}
	}

	function stopProbe() {
		if (probeTimer !== null) {
			clearInterval(probeTimer);
			probeTimer = null;
		}
	}

	/** Periodically look for the server until it answers, then flush. */
	function startProbe() {
		if (probeTimer !== null) return;
		probeTimer = setInterval(() => {
			void deps
				.ping()
				.then(() => markOnline())
				.catch(() => {});
		}, RECONNECT_PROBE_MS);
	}

	function markOffline() {
		if (!online) return;
		online = false;
		startProbe();
	}

	function markOnline() {
		stopProbe();
		if (!online) {
			online = true;
		}
		void flush();
	}

	/** Apply one entry to the server. */
	async function send(entry: OutboxEntry): Promise<void> {
		switch (entry.kind) {
			case 'answer':
				await deps.answerCard(entry.cardId, entry.rating);
				return;
			case 'note-update':
				await deps.updateNote(entry.noteId, entry.input);
				return;
			case 'note-create': {
				const note = await deps.createNote(entry.input);
				created = { ...created, [entry.ref]: note.id };
				return;
			}
		}
	}

	/**
	 * Replay the outbox in order. Stops at the first network failure (still
	 * offline); an entry the server rejects outright — already-answered,
	 * validation — is dropped, because retrying it forever would wedge
	 * everything queued behind it.
	 */
	async function flush(): Promise<void> {
		if (flushing) return;
		flushing = true;
		try {
			while (outbox.length > 0) {
				const entry = outbox[0];
				try {
					await send(entry);
				} catch (err) {
					if (isNetworkFailure(err)) {
						markOffline();
						return;
					}
					if (err instanceof ApiError && err.status >= 500) {
						// The server is there but unwell; try again later.
						return;
					}
					// Rejected on its merits (409 double answer, 404 deleted
					// note): drop it and keep going.
				}
				outbox = outbox.slice(1);
				persist();
			}
		} finally {
			flushing = false;
		}
	}

	return {
		get online() {
			return online;
		},
		get flushing() {
			return flushing;
		},
		get pending() {
			return outbox.length;
		},
		get pendingAnswers() {
			return outbox.filter((e) => e.kind === 'answer').length;
		},
		/** The created note's id for a queued-offline create, once it exists. */
		createdIdFor(ref: string): number | null {
			return created[ref] ?? null;
		},

		/** Restore the outbox and start watching connectivity. */
		init() {
			restore();
			if (!listenersBound && typeof window !== 'undefined') {
				listenersBound = true;
				window.addEventListener('online', () => markOnline());
				window.addEventListener('offline', () => markOffline());
				if (!navigator.onLine) markOffline();
			}
			if (online && outbox.length > 0) void flush();
		},

		/** Callers report a network-shaped failure here. */
		markOffline,
		/** And a sign of life here (a probe or any successful call). */
		markOnline,
		flush,

		/** Returns the entry's id so the caller can take the answer back
		 * before it ever reaches the server. */
		queueAnswer(cardId: number, rating: number): string {
			const id = newId();
			outbox = [...outbox, { id, kind: 'answer', cardId, rating }];
			persist();
			return id;
		},

		/** Remove a queued answer that was undone before syncing. False when
		 * it already left the queue (a flush got there first). */
		removeAnswer(id: string): boolean {
			const before = outbox.length;
			outbox = outbox.filter((e) => !(e.kind === 'answer' && e.id === id));
			if (outbox.length === before) return false;
			persist();
			return true;
		},

		/** Queue a note save, replacing any earlier queued save of the same
		 * note — only the newest content matters. */
		queueNoteUpdate(noteId: number, input: NoteInput) {
			const existing = outbox.findIndex(
				(e) => e.kind === 'note-update' && e.noteId === noteId,
			);
			const entry: OutboxEntry = { id: newId(), kind: 'note-update', noteId, input };
			if (existing >= 0) {
				const next = [...outbox];
				next[existing] = { ...entry, id: outbox[existing].id };
				outbox = next;
			} else {
				outbox = [...outbox, entry];
			}
			persist();
		},

		/** Queue a note creation (or refresh a queued one's content), keyed by
		 * the caller's ref so more typing keeps updating the same entry. */
		queueNoteCreate(ref: string, input: NoteInput) {
			const existing = outbox.findIndex((e) => e.kind === 'note-create' && e.ref === ref);
			const entry: OutboxEntry = { id: newId(), kind: 'note-create', ref, input };
			if (existing >= 0) {
				const next = [...outbox];
				next[existing] = { ...entry, id: outbox[existing].id };
				outbox = next;
			} else {
				outbox = [...outbox, entry];
			}
			persist();
		},

		/** Test seam. */
		reset() {
			stopProbe();
			online = true;
			flushing = false;
			outbox = [];
			created = {};
			localStorage.removeItem(OUTBOX_KEY);
		},
	};
}

export const sync = createSyncStore();
export type SyncStore = ReturnType<typeof createSyncStore>;
