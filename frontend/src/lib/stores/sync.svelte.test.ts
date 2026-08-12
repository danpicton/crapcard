import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createSyncStore } from './sync.svelte';
import { ApiError } from '$lib/api';

function testDeps() {
	return {
		answerCard: vi.fn().mockResolvedValue({}),
		updateNote: vi.fn().mockResolvedValue({ id: 1 }),
		createNote: vi.fn().mockResolvedValue({ id: 42 }),
		ping: vi.fn().mockResolvedValue(undefined),
	};
}

const networkDown = new ApiError(0, 'network error');

describe('sync store', () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it('replays the outbox in the order things happened', async () => {
		const deps = testDeps();
		const calls: string[] = [];
		deps.answerCard.mockImplementation(async (id: number) => {
			calls.push(`answer:${id}`);
			return {};
		});
		deps.updateNote.mockImplementation(async (id: number) => {
			calls.push(`update:${id}`);
			return { id };
		});

		const s = createSyncStore(deps);
		s.queueAnswer(1, 3);
		s.queueNoteUpdate(7, { deck_id: 1, reversed: false, fields: {} });
		s.queueAnswer(2, 4);
		await s.flush();

		expect(calls).toEqual(['answer:1', 'update:7', 'answer:2']);
		expect(s.pending).toBe(0);
	});

	it('coalesces repeated saves of the same note into one entry', () => {
		const s = createSyncStore(testDeps());
		s.queueNoteUpdate(7, { deck_id: 1, reversed: false, fields: { front: 'a', back: 'b' } });
		s.queueNoteUpdate(7, { deck_id: 1, reversed: false, fields: { front: 'ab', back: 'b' } });

		expect(s.pending).toBe(1);
	});

	it('stops flushing at a network failure and keeps the rest queued', async () => {
		const deps = testDeps();
		deps.answerCard.mockRejectedValue(networkDown);

		const s = createSyncStore(deps);
		s.queueAnswer(1, 3);
		s.queueAnswer(2, 3);
		await s.flush();

		expect(s.pending).toBe(2);
		expect(s.online).toBe(false);
		s.reset();
	});

	it('drops an entry the server rejects outright and keeps going', async () => {
		// A 409 (already answered) retried forever would wedge everything
		// queued behind it.
		const deps = testDeps();
		deps.answerCard
			.mockRejectedValueOnce(new ApiError(409, 'already answered'))
			.mockResolvedValue({});

		const s = createSyncStore(deps);
		s.queueAnswer(1, 3);
		s.queueAnswer(2, 3);
		await s.flush();

		expect(s.pending).toBe(0);
		expect(deps.answerCard).toHaveBeenCalledTimes(2);
	});

	it('records the real id of a note created from the queue', async () => {
		const deps = testDeps();
		const s = createSyncStore(deps);
		s.queueNoteCreate('ref-1', {
			deck_id: 1,
			type: 'basic',
			reversed: false,
			fields: { front: 'a', back: 'b' },
		});
		expect(s.createdIdFor('ref-1')).toBeNull();

		await s.flush();
		expect(s.createdIdFor('ref-1')).toBe(42);
	});

	it('persists the outbox so a closed tab loses nothing', async () => {
		const first = createSyncStore(testDeps());
		first.queueAnswer(1, 3);

		const deps = testDeps();
		const second = createSyncStore(deps);
		second.init(); // restores the outbox and starts flushing it
		await vi.waitFor(() => expect(deps.answerCard).toHaveBeenCalledWith(1, 3));
	});
});
