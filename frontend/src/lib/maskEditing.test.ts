import { describe, expect, it } from 'vitest';
import { nextMaskId, rectFromDrag } from './maskEditing';

const bounds = { width: 400, height: 200 };

describe('rectFromDrag', () => {
	it('normalises a drag to the image', () => {
		const rect = rectFromDrag({ x: 40, y: 20 }, { x: 140, y: 120 }, bounds, 1);
		expect(rect).toEqual({ id: 1, x: 0.1, y: 0.1, w: 0.25, h: 0.5 });
	});

	it('accepts a drag in any direction', () => {
		const rect = rectFromDrag({ x: 140, y: 120 }, { x: 40, y: 20 }, bounds, 3);
		expect(rect).toEqual({ id: 3, x: 0.1, y: 0.1, w: 0.25, h: 0.5 });
	});

	it('clamps a drag that leaves the image', () => {
		const rect = rectFromDrag({ x: 380, y: 190 }, { x: 500, y: 300 }, bounds, 1);
		expect(rect).not.toBeNull();
		expect(rect!.x + rect!.w).toBeLessThanOrEqual(1);
		expect(rect!.y + rect!.h).toBeLessThanOrEqual(1);
	});

	it('rejects a click-sized drag — an accidental mask nobody can see', () => {
		expect(rectFromDrag({ x: 40, y: 20 }, { x: 42, y: 21 }, bounds, 1)).toBeNull();
	});

	it('rejects a degenerate image', () => {
		expect(rectFromDrag({ x: 0, y: 0 }, { x: 10, y: 10 }, { width: 0, height: 0 }, 1)).toBeNull();
	});
});

describe('nextMaskId', () => {
	it('starts at 1', () => {
		expect(nextMaskId([])).toBe(1);
	});

	it('never reuses an id, even after deletions', () => {
		// Masks 1 and 3 exist; 2 was deleted. Reusing 2 would attach the new
		// mask's card to the deleted mask's review history.
		expect(
			nextMaskId([
				{ id: 1, x: 0, y: 0, w: 0.1, h: 0.1 },
				{ id: 3, x: 0.5, y: 0.5, w: 0.1, h: 0.1 },
			]),
		).toBe(4);
	});
});
