import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createSession } from './session.svelte';
import { ApiError, type StudyCard, type StudyQueue, type AnswerResult } from '$lib/api';

function card(overrides: Partial<StudyCard> = {}): StudyCard {
	return {
		card_id: 1,
		note_id: 1,
		deck_id: 1,
		deck_name: 'Italian',
		template: 'forward',
		question: 'ciao',
		answer: 'hello',
		state: 'new',
		flagged: false,
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

function studyQueue(cards: StudyCard[], overrides: Partial<StudyQueue> = {}): StudyQueue {
	return {
		deck_id: 1,
		deck_name: 'Italian',
		counts: { new: cards.length, learning: 0, due: 0, total: cards.length },
		cards,
		...overrides,
	};
}

function answerResult(overrides: Partial<AnswerResult> = {}): AnswerResult {
	return {
		card_id: 1,
		interval_seconds: 86400,
		interval_label: '1d',
		due: '',
		state: 'review',
		counts: { new: 1, learning: 0, due: 0, total: 1 },
		...overrides,
	};
}

function testDeps() {
	return {
		fetchQueue: vi.fn().mockResolvedValue(studyQueue([card()])),
		answerCard: vi.fn().mockResolvedValue(answerResult()),
		undoAnswer: vi.fn().mockResolvedValue(card()),
		suspendCard: vi.fn().mockResolvedValue({ card_id: 1, suspended: true }),
		flagCard: vi.fn().mockResolvedValue({ card_id: 1, flagged: true }),
		buryCard: vi.fn().mockResolvedValue({ card_id: 1, buried_until: null }),
		queueAnswer: vi.fn().mockReturnValue('outbox-1'),
		queueSuspend: vi.fn(),
		queueFlag: vi.fn(),
		queueBury: vi.fn(),
		removeQueuedAnswer: vi.fn().mockReturnValue(true),
		flushOutbox: vi.fn().mockResolvedValue(undefined),
	};
}

const networkDown = new ApiError(0, 'network error');

describe('study session', () => {
	let deps: ReturnType<typeof testDeps>;

	beforeEach(() => {
		localStorage.clear();
		deps = testDeps();
	});

	it('fetches the whole queue and serves the first card, hidden', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		const s = createSession(1, deps);
		await s.start();

		expect(s.card?.card_id).toBe(1);
		expect(s.remaining).toBe(2);
		expect(s.revealed).toBe(false);
		expect(s.finished).toBe(false);
	});

	it('hides the answer until it is asked for', async () => {
		const s = createSession(1, deps);
		await s.start();

		expect(s.revealed).toBe(false);
		s.reveal();
		expect(s.revealed).toBe(true);
	});

	it('refuses to answer before the card has been revealed', async () => {
		// Grading how well you recalled something you never tried to recall
		// is meaningless, and would corrupt the schedule.
		const s = createSession(1, deps);
		await s.start();

		await s.answer(3);

		expect(deps.answerCard).not.toHaveBeenCalled();
		expect(s.card?.card_id).toBe(1);
	});

	it('submits the rating and advances to the next card, hidden again', async () => {
		deps.fetchQueue.mockResolvedValue(
			studyQueue([card(), card({ card_id: 2, question: 'grazie' })]),
		);
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		await s.answer(3);

		expect(deps.answerCard).toHaveBeenCalledWith(1, 3);
		expect(s.card?.question).toBe('grazie');
		expect(s.revealed).toBe(false);
		// The queue was fetched once; advancing needs no network.
		expect(deps.fetchQueue).toHaveBeenCalledTimes(1);
	});

	it('finishes when the queue runs out', async () => {
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		await s.answer(4);

		expect(s.finished).toBe(true);
		expect(s.card).toBeNull();
	});

	it('starts finished when the deck has nothing due', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([]));
		const s = createSession(1, deps);
		await s.start();

		expect(s.finished).toBe(true);
	});

	it('puts a short-interval answer back into the queue', async () => {
		// "Again" comes back in a minute; the learning loop must survive the
		// move to a locally-held queue.
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		deps.answerCard.mockResolvedValue(answerResult({ interval_seconds: 60 }));
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		await s.answer(1);

		expect(s.remaining).toBe(2);
		expect(s.card?.card_id).toBe(2); // the due card first
		s.reveal();
		deps.answerCard.mockResolvedValue(answerResult({ interval_seconds: 999999 }));
		await s.answer(4);

		// The relearning copy is served, marked so its stale labels hide.
		expect(s.card?.card_id).toBe(1);
		expect(s.card?.stale_previews).toBe(true);
	});

	it('does not requeue a long-interval answer', async () => {
		deps.answerCard.mockResolvedValue(answerResult({ interval_seconds: 4 * 86400 }));
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		await s.answer(4);

		expect(s.finished).toBe(true);
	});

	it('takes the counts from each answer, which the server computed after it', async () => {
		const s = createSession(1, deps);
		await s.start();
		expect(s.counts?.total).toBe(1);

		s.reveal();
		deps.answerCard.mockResolvedValue(
			answerResult({ counts: { new: 0, learning: 0, due: 0, total: 0 } }),
		);
		await s.answer(3);

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

	it('surfaces a server rejection and keeps the card for a retry', async () => {
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		deps.answerCard.mockRejectedValue(new ApiError(409, 'already answered'));
		await s.answer(3);

		expect(s.error).toContain('already answered');
		expect(s.card?.card_id).toBe(1);
		expect(s.revealed).toBe(true);
		expect(deps.queueAnswer).not.toHaveBeenCalled();
	});

	it('studies from anywhere when no deck is named', async () => {
		const s = createSession(null, deps);
		await s.start();

		expect(deps.fetchQueue).toHaveBeenCalledWith(null);
		expect(s.deckName).toBe('Italian');
	});

	it('finishes cleanly when nothing is due anywhere', async () => {
		deps.fetchQueue.mockResolvedValue(null);
		const s = createSession(null, deps);
		await s.start();

		expect(s.finished).toBe(true);
	});

	it('refuses to undo before anything has been answered', async () => {
		const s = createSession(1, deps);
		await s.start();

		expect(s.canUndo).toBe(false);
		await s.undo();
		expect(deps.undoAnswer).not.toHaveBeenCalled();
	});

	it('brings a synced answer back through the server, revealed', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		const s = createSession(1, deps);
		await s.start();
		s.reveal();
		await s.answer(4);
		expect(s.card?.card_id).toBe(2);

		deps.undoAnswer.mockResolvedValue(card({ card_id: 1 }));
		await s.undo();

		expect(deps.undoAnswer).toHaveBeenCalled();
		expect(s.card?.card_id).toBe(1);
		expect(s.revealed).toBe(true);
		expect(s.reviewed).toBe(0);
		expect(s.remaining).toBe(2);
	});

	it('undoes its way back out of a finished session', async () => {
		// Grading the last card wrong and watching the deck end is exactly
		// when undo matters most.
		const s = createSession(1, deps);
		await s.start();
		s.reveal();
		await s.answer(4);
		expect(s.finished).toBe(true);

		deps.undoAnswer.mockResolvedValue(card({ card_id: 1 }));
		await s.undo();

		expect(s.finished).toBe(false);
		expect(s.card?.card_id).toBe(1);
		expect(s.revealed).toBe(true);
	});

	it('removes the requeued copy when a short-interval answer is undone', async () => {
		deps.answerCard.mockResolvedValue(answerResult({ interval_seconds: 60 }));
		const s = createSession(1, deps);
		await s.start();
		s.reveal();
		await s.answer(1);
		expect(s.remaining).toBe(1); // the relearning copy

		deps.undoAnswer.mockResolvedValue(card({ card_id: 1 }));
		await s.undo();

		expect(s.remaining).toBe(1); // the restored card, not two copies
		expect(s.card?.card_id).toBe(1);
	});
});

describe('study session while offline', () => {
	let deps: ReturnType<typeof testDeps>;

	beforeEach(() => {
		localStorage.clear();
		deps = testDeps();
	});

	it('keeps studying: a refused answer parks in the outbox and the next card comes up', async () => {
		deps.fetchQueue.mockResolvedValue(
			studyQueue([card(), card({ card_id: 2, question: 'grazie' })]),
		);
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		deps.answerCard.mockRejectedValue(networkDown);
		await s.answer(4);

		expect(deps.queueAnswer).toHaveBeenCalledWith(1, 4);
		expect(s.error).toBeNull();
		expect(s.card?.card_id).toBe(2);
		expect(s.reviewed).toBe(1);
	});

	it('uses the card previews to requeue a short offline answer', async () => {
		// No server verdict offline — the FSRS previews fetched with the card
		// are the best available prediction.
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		deps.answerCard.mockRejectedValue(networkDown);
		await s.answer(1); // "again" previews at 60s

		expect(s.remaining).toBe(1);
		expect(s.card?.card_id).toBe(1);
		expect(s.card?.stale_previews).toBe(true);
	});

	it('withdraws an unsynced answer from the outbox on undo, no server needed', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		const s = createSession(1, deps);
		await s.start();
		s.reveal();
		deps.answerCard.mockRejectedValue(networkDown);
		await s.answer(4);

		await s.undo();

		expect(deps.removeQueuedAnswer).toHaveBeenCalledWith('outbox-1');
		expect(deps.undoAnswer).not.toHaveBeenCalled();
		expect(s.card?.card_id).toBe(1);
		expect(s.revealed).toBe(true);
		expect(s.reviewed).toBe(0);
	});

	it('falls back to a server undo when the parked answer already synced', async () => {
		const s = createSession(1, deps);
		await s.start();
		s.reveal();
		deps.answerCard.mockRejectedValue(networkDown);
		await s.answer(4);

		deps.removeQueuedAnswer.mockReturnValue(false);
		deps.undoAnswer.mockResolvedValue(card({ card_id: 1 }));
		await s.undo();

		expect(deps.undoAnswer).toHaveBeenCalled();
		expect(s.card?.card_id).toBe(1);
	});

	it('resumes from the persisted queue when the fetch fails', async () => {
		// The PWA opened with no network: yesterday's fetch is the session.
		const first = createSession(1, deps);
		await first.start(); // persists the queue

		const offlineDeps = testDeps();
		offlineDeps.fetchQueue.mockRejectedValue(networkDown);
		const second = createSession(1, offlineDeps);
		await second.start();

		expect(second.card?.card_id).toBe(1);
		expect(second.stalled).toBe(false);
	});

	it('stalls only when offline with nothing cached', async () => {
		deps.fetchQueue.mockRejectedValue(networkDown);
		const s = createSession(1, deps);
		await s.start();

		expect(s.stalled).toBe(true);
		expect(s.finished).toBe(false);
		expect(s.error).toBeNull();
	});

	it('resume flushes the outbox before refetching the queue', async () => {
		const order: string[] = [];
		deps.flushOutbox.mockImplementation(async () => {
			order.push('flush');
		});
		deps.fetchQueue.mockImplementation(async () => {
			order.push('fetch');
			return studyQueue([card({ card_id: 2 })]);
		});

		const s = createSession(1, deps);
		await s.resume();

		expect(order).toEqual(['flush', 'fetch']);
		expect(s.card?.card_id).toBe(2);
	});
});

describe('card actions during study', () => {
	let deps: ReturnType<typeof testDeps>;

	beforeEach(() => {
		localStorage.clear();
		deps = testDeps();
	});

	it('flagging alone marks the card and keeps it in the session', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		const s = createSession(1, deps);
		await s.start();

		await s.setFlag(true, 'dubious answer');
		expect(deps.flagCard).toHaveBeenCalledWith(1, true, 'dubious answer');
		expect(deps.suspendCard).not.toHaveBeenCalled();
		expect(s.card?.card_id).toBe(1);
		expect(s.card?.flagged).toBe(true);
		expect(s.remaining).toBe(2);
	});

	it('flag with suspend removes the card without grading it', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		const s = createSession(1, deps);
		await s.start();
		s.reveal();

		await s.setFlag(true, 'needs a rewrite', true);
		expect(deps.flagCard).toHaveBeenCalledWith(1, true, 'needs a rewrite');
		expect(deps.suspendCard).toHaveBeenCalledWith(1, true);
		expect(deps.answerCard).not.toHaveBeenCalled();
		expect(s.card?.card_id).toBe(2);
		expect(s.revealed).toBe(false);
	});

	it('suspend with a reason flags too, so the reason survives to the flagged view', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		const s = createSession(1, deps);
		await s.start();

		await s.suspend('too niche for now');
		expect(deps.flagCard).toHaveBeenCalledWith(1, true, 'too niche for now');
		expect(deps.suspendCard).toHaveBeenCalledWith(1, true);
		expect(s.card?.card_id).toBe(2);
	});

	it('suspend without a reason never flags', async () => {
		const s = createSession(1, deps);
		await s.start();

		await s.suspend();
		expect(deps.flagCard).not.toHaveBeenCalled();
		expect(deps.suspendCard).toHaveBeenCalledWith(1, true);
	});

	it('bury removes the card, including its requeued learning copy', async () => {
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		deps.answerCard.mockResolvedValue(answerResult({ interval_seconds: 60 }));
		const s = createSession(1, deps);
		await s.start();

		// Answer card 1 with a short interval so a copy requeues behind card 2,
		// then bury card 2 — only card 1's copy should remain.
		s.reveal();
		await s.answer(1);
		expect(s.card?.card_id).toBe(2);
		await s.bury(3);
		expect(deps.buryCard).toHaveBeenCalledWith(2, 3);
		expect(s.card?.card_id).toBe(1);
		expect(s.remaining).toBe(1);
	});

	it('rejects a bury of less than one day', async () => {
		const s = createSession(1, deps);
		await s.start();
		await s.bury(0);
		expect(deps.buryCard).not.toHaveBeenCalled();
		expect(s.remaining).toBe(1);
	});

	it('parks the actions in the outbox when the network is away', async () => {
		deps.flagCard.mockRejectedValue(networkDown);
		deps.suspendCard.mockRejectedValue(networkDown);
		deps.fetchQueue.mockResolvedValue(studyQueue([card(), card({ card_id: 2 })]));
		const s = createSession(1, deps);
		await s.start();

		await s.setFlag(true, 'check later', true);
		expect(deps.queueFlag).toHaveBeenCalledWith(1, true, 'check later');
		expect(deps.queueSuspend).toHaveBeenCalledWith(1, true);
		// The session moves on exactly as it would online.
		expect(s.card?.card_id).toBe(2);

		deps.buryCard.mockRejectedValue(networkDown);
		await s.bury(1);
		expect(deps.queueBury).toHaveBeenCalledWith(2, 1);
		expect(s.remaining).toBe(0);
	});

	it('keeps the card and reports a server rejection', async () => {
		deps.suspendCard.mockRejectedValue(new ApiError(500, 'server exploded'));
		const s = createSession(1, deps);
		await s.start();

		await s.suspend();
		expect(s.error).toBe('server exploded');
		expect(s.card?.card_id).toBe(1);
		expect(deps.queueSuspend).not.toHaveBeenCalled();
	});
});
