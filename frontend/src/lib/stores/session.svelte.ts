import { api, type StudyCard, type AnswerResult, type QueueCounts } from '$lib/api';

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
}

const defaultDeps: SessionDeps = {
	nextCard: (deckId) => (deckId === null ? api.nextCardAnywhere() : api.nextCard(deckId)),
	answerCard: (cardId, rating) => api.answerCard(cardId, rating),
	undoAnswer: () => api.undoAnswer(),
};

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

	async function load() {
		loading = true;
		try {
			const next = await deps.nextCard(deckId);
			card = next;
			revealed = false;
			finished = next === null;
			if (next) counts = next.counts;
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not load the next card';
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
		/** True when there is an answer from this sitting to take back. */
		get canUndo() {
			return reviewed > 0;
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
			if (!card || !revealed || submitting) return;

			submitting = true;
			const answered = card;
			try {
				const result = await deps.answerCard(answered.card_id, rating);
				counts = result.counts;
				reviewed += 1;
				error = null;
				await load();
			} catch (err) {
				// Keep the card and its revealed state so the answer can simply
				// be given again.
				error = err instanceof Error ? err.message : 'could not save your answer';
			} finally {
				submitting = false;
			}
		},

		/**
		 * Take back the most recent answer: the card returns, already
		 * revealed, so the right grade can be given instead. Repeating it
		 * steps further back through this sitting's answers.
		 */
		async undo() {
			if (submitting || reviewed === 0) return;

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
