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
	nextCard: (deckId: number) => Promise<StudyCard | null>;
	answerCard: (cardId: number, rating: number) => Promise<AnswerResult>;
}

const defaultDeps: SessionDeps = {
	nextCard: (deckId) => api.nextCard(deckId),
	answerCard: (cardId, rating) => api.answerCard(cardId, rating),
};

export function createSession(deckId: number, deps: SessionDeps = defaultDeps) {
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
	};
}

export type Session = ReturnType<typeof createSession>;
