import { api, ApiError, type StudyCard, type AnswerResult, type QueueCounts } from '$lib/api';
import { sync } from '$lib/stores/sync.svelte';

/**
 * The study loop, kept out of the component so the state machine can be
 * tested on its own.
 *
 * The rules it enforces are the ones that would corrupt a schedule if the UI
 * got them wrong: an answer is only accepted once the card has been revealed,
 * and only one answer is in flight at a time.
 */

interface SessionDeps {
	nextCard: (deckId: number | null) => Promise<StudyCard | null>;
	answerCard: (cardId: number, rating: number) => Promise<AnswerResult>;
	undoAnswer: () => Promise<StudyCard | null>;
	/** Park an answer that could not reach the server. */
	queueAnswer: (cardId: number, rating: number) => void;
	/** Drain everything parked, in order, before asking for more cards. */
	flushOutbox: () => Promise<void>;
}

const defaultDeps: SessionDeps = {
	nextCard: (deckId) => (deckId === null ? api.nextCardAnywhere() : api.nextCard(deckId)),
	answerCard: (cardId, rating) => api.answerCard(cardId, rating),
	undoAnswer: () => api.undoAnswer(),
	queueAnswer: (cardId, rating) => {
		sync.markOffline();
		sync.queueAnswer(cardId, rating);
	},
	flushOutbox: () => sync.flush(),
};

/** The failure shape that means "no network", not "the server said no". */
function isNetworkFailure(err: unknown): boolean {
	return err instanceof ApiError && err.status === 0;
}

/**
 * Creates a study session. Pass a deck id to study one deck, or null to
 * study whatever is due next anywhere — the landing screen's mode.
 */
export function createSession(deckId: number | null, deps: SessionDeps = defaultDeps) {
	let card = $state<StudyCard | null>(null);
	let revealed = $state(false);
	let finished = $state(false);
	let loading = $state(false);
	let submitting = $state(false);
	let reviewed = $state(0);
	let counts = $state<QueueCounts | null>(null);
	let error = $state<string | null>(null);
	// True when the network went away mid-session: the last answer is parked
	// in the outbox and no further cards can be fetched until it returns.
	let stalled = $state(false);

	async function load() {
		loading = true;
		try {
			const next = await deps.nextCard(deckId);
			card = next;
			revealed = false;
			finished = next === null;
			stalled = false;
			if (next) counts = next.counts;
		} catch (err) {
			if (isNetworkFailure(err)) {
				stalled = true;
			} else {
				error = err instanceof Error ? err.message : 'could not load the next card';
			}
		} finally {
			loading = false;
		}
	}

	return {
		get card() {
			return card;
		},
		get revealed() {
			return revealed;
		},
		get finished() {
			return finished;
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
			return reviewed > 0 && !stalled;
		},

		/** Load the first card. */
		start: load,

		/** Show the answer side. */
		reveal() {
			if (card) revealed = true;
		},

		/**
		 * Grade the current card and move on.
		 *
		 * Does nothing before the answer has been revealed — grading recall of
		 * something never recalled would feed the scheduler a meaningless
		 * signal — and nothing while a previous answer is still in flight, so
		 * a double tap cannot grade the same card twice.
		 */
		async answer(rating: number) {
			if (!card || !revealed || submitting || stalled) return;

			submitting = true;
			const answered = card;
			try {
				const result = await deps.answerCard(answered.card_id, rating);
				counts = result.counts;
				reviewed += 1;
				error = null;
				await load();
			} catch (err) {
				if (isNetworkFailure(err)) {
					// The answer is not lost — it goes to the outbox and the
					// session pauses until the network returns.
					deps.queueAnswer(answered.card_id, rating);
					reviewed += 1;
					error = null;
					stalled = true;
				} else {
					// Keep the card and its revealed state so the answer can
					// simply be given again.
					error = err instanceof Error ? err.message : 'could not save your answer';
				}
			} finally {
				submitting = false;
			}
		},

		/**
		 * Pick the session back up after a stall: replay the outbox first —
		 * the server must apply the parked answers before choosing the next
		 * card — then load whatever is due now.
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

		/**
		 * Take back the most recent answer: the card returns, already
		 * revealed, so the right grade can be given instead. Repeating it
		 * steps further back through this sitting's answers.
		 */
		async undo() {
			if (submitting || reviewed === 0 || stalled) return;

			submitting = true;
			try {
				const restored = await deps.undoAnswer();
				if (restored === null) {
					reviewed = 0;
					return;
				}
				card = restored;
				counts = restored.counts;
				revealed = true;
				finished = false;
				reviewed = Math.max(0, reviewed - 1);
				error = null;
			} catch (err) {
				error = err instanceof Error ? err.message : 'could not undo the answer';
			} finally {
				submitting = false;
			}
		},
	};
}

export type Session = ReturnType<typeof createSession>;
