import {
	api,
	ApiError,
	RATING_KEYS,
	type StudyCard,
	type StudyQueue,
	type AnswerResult,
	type QueueCounts,
} from '$lib/api';
import { sync } from '$lib/stores/sync.svelte';

/**
 * The study loop, kept out of the component so the state machine can be
 * tested on its own.
 *
 * The whole due queue is fetched up front and studied locally, so grading
 * continues with no network: an answer posts to the server when it can and
 * parks in the outbox when it cannot, and either way the session moves to
 * the next card. The queue is persisted, so a PWA opened offline resumes the
 * cards it fetched last time.
 *
 * The rules enforced here are the ones that would corrupt a schedule if the
 * UI got them wrong: an answer is only accepted once the card has been
 * revealed, and only one answer is in flight at a time.
 */

/** A card in the local queue, with client-side scheduling metadata. */
export interface QueuedCard extends StudyCard {
	/**
	 * When a short-interval answer predicts this card comes back, in epoch
	 * ms. Absent on cards straight from the server, which are due already.
	 */
	predicted_due_ms?: number;
	/**
	 * True once the card has been graded this sitting: its previews were
	 * computed from the state it no longer has, so the labels are hidden.
	 */
	stale_previews?: boolean;
}

/** An answer given this sitting, kept so it can be taken back. */
interface AnsweredCard {
	card: QueuedCard;
	rating: number;
	/** The outbox entry when the answer never reached the server. */
	outboxId: string | null;
}

/**
 * Answers predicting a return within this window put the card back into the
 * local queue — the learning loop ("Again" comes back in minutes) keeps
 * working offline. Longer intervals belong to future sessions.
 */
const REQUEUE_HORIZON_S = 60 * 60;

interface SessionDeps {
	fetchQueue: (deckId: number | null) => Promise<StudyQueue | null>;
	answerCard: (cardId: number, rating: number) => Promise<AnswerResult>;
	undoAnswer: () => Promise<StudyCard | null>;
	suspendCard: (cardId: number, suspended: boolean, reason: string) => Promise<unknown>;
	flagCard: (cardId: number, flagged: boolean, reason: string) => Promise<unknown>;
	buryCard: (cardId: number, days: number) => Promise<unknown>;
	/** Park an answer that could not reach the server; returns its id. */
	queueAnswer: (cardId: number, rating: number) => string;
	/** Park a card action that could not reach the server. */
	queueSuspend: (cardId: number, suspended: boolean, reason: string) => void;
	queueFlag: (cardId: number, flagged: boolean, reason: string) => void;
	queueBury: (cardId: number, days: number) => void;
	/** Take a parked answer back; false if it already synced. */
	removeQueuedAnswer: (id: string) => boolean;
	/** Drain everything parked, in order. */
	flushOutbox: () => Promise<void>;
}

const defaultDeps: SessionDeps = {
	fetchQueue: (deckId) => api.studyQueue(deckId),
	answerCard: (cardId, rating) => api.answerCard(cardId, rating),
	undoAnswer: () => api.undoAnswer(),
	suspendCard: (cardId, suspended, reason) => api.suspendCard(cardId, suspended, reason),
	flagCard: (cardId, flagged, reason) => api.flagCard(cardId, flagged, reason),
	buryCard: (cardId, days) => api.buryCard(cardId, days),
	queueAnswer: (cardId, rating) => {
		sync.markOffline();
		return sync.queueAnswer(cardId, rating);
	},
	queueSuspend: (cardId, suspended, reason) => {
		sync.markOffline();
		sync.queueSuspend(cardId, suspended, reason);
	},
	queueFlag: (cardId, flagged, reason) => {
		sync.markOffline();
		sync.queueFlag(cardId, flagged, reason);
	},
	queueBury: (cardId, days) => {
		sync.markOffline();
		sync.queueBury(cardId, days);
	},
	removeQueuedAnswer: (id) => sync.removeAnswer(id),
	flushOutbox: () => sync.flush(),
};

/** The failure shape that means "no network", not "the server said no". */
function isNetworkFailure(err: unknown): boolean {
	return err instanceof ApiError && err.status === 0;
}

function ratingKey(rating: number): (typeof RATING_KEYS)[number] {
	return RATING_KEYS[rating - 1] ?? 'good';
}

function storageKey(deckId: number | null): string {
	return `crapcard-study-queue:${deckId ?? 'anywhere'}`;
}

/**
 * Creates a study session. Pass a deck id to study one deck, or null to
 * study whatever is due next anywhere — the landing screen's mode.
 */
export function createSession(deckId: number | null, deps: SessionDeps = defaultDeps) {
	let queue = $state<QueuedCard[]>([]);
	let deckName = $state<string | null>(null);
	let cardDeckId = $state<number | null>(deckId);
	let revealed = $state(false);
	let started = $state(false);
	let loading = $state(false);
	let submitting = $state(false);
	let reviewed = $state(0);
	let counts = $state<QueueCounts | null>(null);
	let error = $state<string | null>(null);
	// True when offline with nothing usable: no fetched queue, no leftovers.
	let stalled = $state(false);
	let history: AnsweredCard[] = [];

	function persist() {
		try {
			localStorage.setItem(
				storageKey(deckId),
				JSON.stringify({ deck_id: cardDeckId, deck_name: deckName, counts, cards: queue }),
			);
		} catch {
			// Storage full: offline resume degrades, the live session works.
		}
	}

	function restore(): boolean {
		try {
			const raw = localStorage.getItem(storageKey(deckId));
			if (!raw) return false;
			const saved = JSON.parse(raw) as {
				deck_id: number | null;
				deck_name: string | null;
				counts: QueueCounts | null;
				cards: QueuedCard[];
			};
			if (!Array.isArray(saved.cards) || saved.cards.length === 0) return false;
			queue = saved.cards;
			deckName = saved.deck_name;
			cardDeckId = saved.deck_id;
			counts = saved.counts;
			return true;
		} catch {
			return false;
		}
	}

	/** Put a card graded moments ago back where its predicted due slots it:
	 * after everything already due, in predicted order among its kind. */
	function requeue(card: QueuedCard, predictedDueMs: number) {
		const entry: QueuedCard = { ...card, predicted_due_ms: predictedDueMs, stale_previews: true };
		let at = queue.length;
		for (let i = 0; i < queue.length; i++) {
			const other = queue[i].predicted_due_ms;
			if (other !== undefined && other > predictedDueMs) {
				at = i;
				break;
			}
		}
		queue = [...queue.slice(0, at), entry, ...queue.slice(at)];
	}

	async function load() {
		loading = true;
		try {
			const fetched = await deps.fetchQueue(deckId);
			stalled = false;
			if (fetched === null) {
				queue = [];
			} else {
				queue = fetched.cards;
				deckName = fetched.deck_name;
				cardDeckId = fetched.deck_id;
				counts = fetched.counts;
			}
			persist();
		} catch (err) {
			if (isNetworkFailure(err)) {
				// Offline: yesterday's fetch is today's session.
				if (!restore()) stalled = true;
			} else {
				error = err instanceof Error ? err.message : 'could not load the study queue';
			}
		} finally {
			started = true;
			loading = false;
		}
	}

	return {
		get card(): QueuedCard | null {
			return queue[0] ?? null;
		},
		get deckName() {
			return deckName;
		},
		get deckId() {
			return cardDeckId;
		},
		get remaining() {
			return queue.length;
		},
		get revealed() {
			return revealed;
		},
		get finished() {
			return started && !stalled && queue.length === 0;
		},
		get loading() {
			return loading;
		},
		get submitting() {
			return submitting;
		},
		get reviewed() {
			return reviewed;
		},
		get counts() {
			return counts;
		},
		get error() {
			return error;
		},
		get stalled() {
			return stalled;
		},
		/** True when there is an answer from this sitting to take back. */
		get canUndo() {
			return history.length > 0;
		},

		/** Fetch the queue (or restore the persisted one when offline). */
		start: load,

		/** Show the answer side. */
		reveal() {
			if (queue.length > 0) revealed = true;
		},

		/**
		 * Grade the current card and move on.
		 *
		 * Does nothing before the answer has been revealed — grading recall of
		 * something never recalled would feed the scheduler a meaningless
		 * signal — and nothing while a previous answer is still in flight, so
		 * a double tap cannot grade the same card twice.
		 *
		 * Offline the answer parks in the outbox and the session keeps going;
		 * a short predicted interval (from the card's own FSRS previews) puts
		 * the card back in the queue, so the learning loop works anywhere.
		 */
		async answer(rating: number) {
			const card = queue[0];
			if (!card || !revealed || submitting) return;

			submitting = true;
			try {
				let intervalSeconds: number;
				let outboxId: string | null = null;
				try {
					const result = await deps.answerCard(card.card_id, rating);
					counts = result.counts;
					intervalSeconds = result.interval_seconds;
				} catch (err) {
					if (!isNetworkFailure(err)) {
						error = err instanceof Error ? err.message : 'could not save your answer';
						return;
					}
					outboxId = deps.queueAnswer(card.card_id, rating);
					intervalSeconds =
						card.previews[ratingKey(rating)]?.interval_seconds ?? REQUEUE_HORIZON_S + 1;
				}

				history.push({ card, rating, outboxId });
				reviewed += 1;
				error = null;
				queue = queue.slice(1);
				if (intervalSeconds <= REQUEUE_HORIZON_S) {
					requeue(card, Date.now() + intervalSeconds * 1000);
				}
				revealed = false;
				persist();
			} finally {
				submitting = false;
			}
		},

		/**
		 * Flag or unflag the current card, optionally suspending it in the
		 * same breath. A flag alone keeps the card in the session — it is an
		 * annotation, not a removal — so only the icon changes; suspending
		 * takes it out of the queue on the spot.
		 *
		 * Offline the actions park in the outbox like answers do.
		 */
		async setFlag(flagged: boolean, reason: string, alsoSuspend = false) {
			const card = queue[0];
			if (!card || submitting) return;

			submitting = true;
			try {
				try {
					await deps.flagCard(card.card_id, flagged, reason);
					if (alsoSuspend) await deps.suspendCard(card.card_id, true, '');
				} catch (err) {
					if (!isNetworkFailure(err)) {
						error = err instanceof Error ? err.message : 'could not flag the card';
						return;
					}
					deps.queueFlag(card.card_id, flagged, reason);
					if (alsoSuspend) deps.queueSuspend(card.card_id, true, '');
				}
				error = null;
				if (alsoSuspend) {
					queue = queue.filter((c) => c.card_id !== card.card_id);
					revealed = false;
				} else {
					queue = queue.map((c) =>
						c.card_id === card.card_id ? { ...c, flagged } : c,
					);
				}
				persist();
			} finally {
				submitting = false;
			}
		},

		/**
		 * Suspend the current card and move on without grading it. The reason
		 * belongs to the suspension itself, read back in the attention view;
		 * alsoFlag additionally flags the card, at the user's discretion.
		 */
		async suspend(reason = '', alsoFlag = false) {
			const card = queue[0];
			if (!card || submitting) return;

			submitting = true;
			try {
				try {
					await deps.suspendCard(card.card_id, true, reason);
					if (alsoFlag) await deps.flagCard(card.card_id, true, '');
				} catch (err) {
					if (!isNetworkFailure(err)) {
						error = err instanceof Error ? err.message : 'could not suspend the card';
						return;
					}
					deps.queueSuspend(card.card_id, true, reason);
					if (alsoFlag) deps.queueFlag(card.card_id, true, '');
				}
				error = null;
				queue = queue.filter((c) => c.card_id !== card.card_id);
				revealed = false;
				persist();
			} finally {
				submitting = false;
			}
		},

		/**
		 * Bury the current card — hide it until tomorrow (1) or for a longer
		 * stretch — and move on without grading it.
		 */
		async bury(days: number) {
			const card = queue[0];
			if (!card || submitting || days < 1) return;

			submitting = true;
			try {
				try {
					await deps.buryCard(card.card_id, days);
				} catch (err) {
					if (!isNetworkFailure(err)) {
						error = err instanceof Error ? err.message : 'could not bury the card';
						return;
					}
					deps.queueBury(card.card_id, days);
				}
				error = null;
				queue = queue.filter((c) => c.card_id !== card.card_id);
				revealed = false;
				persist();
			} finally {
				submitting = false;
			}
		},

		/**
		 * Take back the most recent answer: the card returns, already
		 * revealed, so the right grade can be given instead. An answer still
		 * parked in the outbox is simply withdrawn; one the server has seen
		 * is reverted there.
		 */
		async undo() {
			const last = history[history.length - 1];
			if (!last || submitting) return;

			submitting = true;
			try {
				let restored: QueuedCard | null = null;
				if (last.outboxId !== null) {
					if (deps.removeQueuedAnswer(last.outboxId)) {
						restored = last.card;
					} else {
						// It synced while we hesitated; undo it server-side.
						last.outboxId = null;
					}
				}
				if (restored === null && last.outboxId === null) {
					const fromServer = await deps.undoAnswer();
					if (fromServer === null) {
						history.pop();
						return;
					}
					counts = fromServer.counts;
					restored = fromServer;
				}
				if (restored === null) return;

				history.pop();
				// Drop any short-interval copy waiting further down the queue.
				queue = [restored, ...queue.filter((c) => c.card_id !== restored.card_id)];
				revealed = true;
				stalled = false;
				reviewed = Math.max(0, reviewed - 1);
				error = null;
				persist();
			} catch (err) {
				if (isNetworkFailure(err)) {
					error = 'Cannot undo a synced answer while offline.';
				} else {
					error = err instanceof Error ? err.message : 'could not undo the answer';
				}
			} finally {
				submitting = false;
			}
		},

		/**
		 * Pick the session back up after an offline start with nothing
		 * cached: replay the outbox first — the server must apply parked
		 * answers before composing the queue — then fetch it.
		 */
		async resume() {
			if (submitting) return;
			submitting = true;
			try {
				await deps.flushOutbox();
			} finally {
				submitting = false;
			}
			await load();
		},
	};
}

export type Session = ReturnType<typeof createSession>;
