import type { OcclusionRect } from './cloze';

/**
 * The geometry of drawing masks over an image, kept out of the modal
 * component so it can be tested as plain functions.
 */

/** Below this fraction of the image a mask is an accidental click. */
const MIN_MASK_FRACTION = 0.01;

/**
 * Turns a drag between two points (in pixels, relative to the rendered
 * image) into a mask normalised to it. Null for a drag too small to mean
 * anything, or over an image with no size yet.
 */
export function rectFromDrag(
	start: { x: number; y: number },
	end: { x: number; y: number },
	bounds: { width: number; height: number },
	id: number,
): OcclusionRect | null {
	if (bounds.width <= 0 || bounds.height <= 0) return null;

	const clamp = (v: number) => Math.min(Math.max(v, 0), 1);
	const x1 = clamp(Math.min(start.x, end.x) / bounds.width);
	const x2 = clamp(Math.max(start.x, end.x) / bounds.width);
	const y1 = clamp(Math.min(start.y, end.y) / bounds.height);
	const y2 = clamp(Math.max(start.y, end.y) / bounds.height);

	const w = x2 - x1;
	const h = y2 - y1;
	if (w < MIN_MASK_FRACTION || h < MIN_MASK_FRACTION) return null;

	// Four decimals is well under a pixel at any real image size, and keeps
	// the persisted JSON free of floating-point noise.
	const round = (v: number) => Math.round(v * 10000) / 10000;
	return { id, x: round(x1), y: round(y1), w: round(w), h: round(h) };
}

/**
 * The id for a newly drawn mask: one past the highest ever used, never
 * refilling gaps — a card's template is "occ:<id>", so reusing a deleted
 * mask's id would attach the new mask to the old mask's review history.
 */
export function nextMaskId(rects: OcclusionRect[]): number {
	return rects.reduce((highest, r) => Math.max(highest, r.id), 0) + 1;
}
