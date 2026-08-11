import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createSession } from './session.svelte';
import type { StudyCard, AnswerResult } from '$lib/api';

function card(overrides: Partial<StudyCard> = {}): StudyCard {
	return {
		card_id: 1,
		note_id: 1,
		deck_id: 1,
		template: 'forward',
		question: 'ciao',
		answer: 'hello',
		state: 'new',
		counts: { new: 2, learning: 0, due: 0, total: 2 },
		previews: {
			again: { interval_seconds: 60, label: '1m', due: '' },
			hard: { interval_seconds: 600, label: '10m', due: '' },
			good: { interval_seconds: 86400, label: '1d', due: '' },
			easy: { interval_seconds: 345600, label: '4d', due: '' },
		},
		...overrides,
	};
}

function answerResult(overrides: Partial<AnswerResult> = {}): AnswerResult {
	return {
		card_id: 1,
		interval_seconds: 86400,
		interval_label: '1d',
		due: '',
		state: 'learning',
		counts: { new: 1, learning: 0, due: 0, total: 1 },
		...overrides,
	};
}

describe('study session', () => {
	let deps: {
		nextCard: ReturnType<typeof vi.fn>;
		answerCard: ReturnType<typeof vi.fn>;
	};

	beforeEach(() => {
		deps = {
			nextCard: vi.fn().mockResolvedValue(card()),
			answerCard: vi.fn().mockResolvedValue(answerResult()),
		};
	});

	it('loads the first card with its answer hidden', async () => {
		const s = createSession(1, deps);
		await s.start();

		expect(s.card?.question).toBe('ciao');
		expect(s.revealed).toBe(false);
		expect(s.finished).toBe(false);
	});

	it('hides the answer until it is asked for', async () => {
		// Showing the answer with the question would defeat the exercise.
		const s = createSession(1, deps);
		await s.start();

		expect(s.revealed).toBe(false);
		s.reveal();
		expect(s.revealed).toBe(true);
	});

	it('refuses to answer before the card has been revealed', async () => {
		// Grading how well you recalled something you never tried to recall is
		// meaningless, and would corrupt the schedule.
		const s = createSession(1, deps);
		await s.start();

		await s.answer(3);

		expect(deps.answerCard).not.toHaveBeenCalled();
		expect(s.card?.card_id).toBe(1);
	});

	it('submits the rating and loads the next card, hidden again', async () => {
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		deps.nextCard.mockResolvedValue(card({ card_id: 2, question: 'grazie', answer: 'thanks' }));
		await s.answer(3);

		expect(deps.answerCard).toHaveBeenCalledWith(1, 3);
		expect(s.card?.question).toBe('grazie');
		expect(s.revealed).toBe(false);
	});

	it('finishes when the queue runs out', async () => {
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		deps.nextCard.mockResolvedValue(null);
		await s.answer(4);

		expect(s.finished).toBe(true);
		expect(s.card).toBeNull();
	});

	it('starts finished when the deck has nothing due', async () => {
		deps.nextCard.mockResolvedValue(null);
		const s = createSession(1, deps);
		await s.start();

		expect(s.finished).toBe(true);
	});

	it('tracks how many cards have been answered this sitting', async () => {
		const s = createSession(1, deps);
		await s.start();

		s.reveal();
		await s.answer(3);
		s.reveal();
		await s.answer(3);

		expect(s.reviewed).toBe(2);
	});

	it('takes the counts from the next card, which the server computed after the answer', async () => {
		const s = createSession(1, deps);
		await s.start();
		expect(s.counts?.total).toBe(2);

		s.reveal();
		deps.nextCard.mockResolvedValue(
			card({ card_id: 2, counts: { new: 1, learning: 0, due: 0, total: 1 } }),
		);
		await s.answer(3);

		expect(s.counts?.total).toBe(1);
	});

	it('falls back to the answer response counts when that answer emptied the queue', async () => {
		// There is no next card to read fresher counts from, so the answer's
		// own counts are all there is.
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		deps.nextCard.mockResolvedValue(null);
		deps.answerCard.mockResolvedValue(
			answerResult({ counts: { new: 0, learning: 0, due: 0, total: 0 } }),
		);
		await s.answer(4);

		expect(s.finished).toBe(true);
		expect(s.counts?.total).toBe(0);
	});

	it('ignores a second answer fired while one is in flight', async () => {
		// Double-tapping a rating button must not grade the card twice.
		let release: (v: AnswerResult) => void = () => {};
		deps.answerCard.mockReturnValue(
			new Promise<AnswerResult>((resolve) => {
				release = resolve;
			}),
		);

		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		const first = s.answer(3);
		const second = s.answer(3);
		release(answerResult());
		await Promise.all([first, second]);

		expect(deps.answerCard).toHaveBeenCalledTimes(1);
	});

	it('surfaces a failure and keeps the card so the answer can be retried', async () => {
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		deps.answerCard.mockRejectedValue(new Error('network error'));
		await s.answer(3);

		expect(s.error).toContain('network error');
		expect(s.card?.card_id).toBe(1);
		expect(s.revealed).toBe(true);
	});

	it('clears a previous error on the next successful answer', async () => {
		const s = createSession(1, deps);
		await s.start();
		s.reveal();
		deps.answerCard.mockRejectedValueOnce(new Error('network error'));
		await s.answer(3);
		expect(s.error).toBeTruthy();

		await s.answer(3);
		expect(s.error).toBeNull();
	});
});
