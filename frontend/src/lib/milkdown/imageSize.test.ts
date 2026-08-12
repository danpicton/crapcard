import { describe, it, expect } from 'vitest';
import { widthFromSrc, srcWithWidth, clampWidth, MIN_IMAGE_WIDTH, MAX_IMAGE_WIDTH } from './imageSize';

describe('widthFromSrc', () => {
	it('reads the width a sized image carries', () => {
		expect(widthFromSrc('/api/images/abc?w=420')).toBe(420);
	});

	it('returns null for an unsized image', () => {
		expect(widthFromSrc('/api/images/abc')).toBeNull();
	});

	it('ignores a nonsense width rather than rendering a broken image', () => {
		for (const src of [
			'/api/images/abc?w=',
			'/api/images/abc?w=wide',
			'/api/images/abc?w=0',
			'/api/images/abc?w=-100',
			'/api/images/abc?w=NaN',
		]) {
			expect(widthFromSrc(src)).toBeNull();
		}
	});

	it('copes with other query parameters being present', () => {
		expect(widthFromSrc('/api/images/abc?v=2&w=300')).toBe(300);
	});

	it('copes with an empty or absent src', () => {
		expect(widthFromSrc('')).toBeNull();
		expect(widthFromSrc(null)).toBeNull();
	});
});

describe('srcWithWidth', () => {
	it('adds a width to a bare url', () => {
		expect(srcWithWidth('/api/images/abc', 420)).toBe('/api/images/abc?w=420');
	});

	it('replaces a width that is already there rather than stacking', () => {
		expect(srcWithWidth('/api/images/abc?w=100', 420)).toBe('/api/images/abc?w=420');
	});

	it('keeps other query parameters', () => {
		expect(srcWithWidth('/api/images/abc?v=2', 420)).toBe('/api/images/abc?v=2&w=420');
	});

	it('removes the width when given null, restoring the natural size', () => {
		expect(srcWithWidth('/api/images/abc?w=420', null)).toBe('/api/images/abc');
		expect(srcWithWidth('/api/images/abc?v=2&w=420', null)).toBe('/api/images/abc?v=2');
	});

	it('rounds a fractional width, since a drag produces one', () => {
		expect(srcWithWidth('/api/images/abc', 420.7)).toBe('/api/images/abc?w=421');
	});

	it('leaves the url alone when there is nothing to remove', () => {
		expect(srcWithWidth('/api/images/abc', null)).toBe('/api/images/abc');
	});
});

describe('clampWidth', () => {
	it('keeps a sensible width untouched', () => {
		expect(clampWidth(400)).toBe(400);
	});

	it('refuses to shrink an image into invisibility', () => {
		// A drag past the left edge would otherwise leave a card with an image
		// too small to see and too small to grab again.
		expect(clampWidth(2)).toBe(MIN_IMAGE_WIDTH);
		expect(clampWidth(-50)).toBe(MIN_IMAGE_WIDTH);
	});

	it('caps a runaway drag', () => {
		expect(clampWidth(99999)).toBe(MAX_IMAGE_WIDTH);
	});

	it('rounds to whole pixels', () => {
		expect(clampWidth(300.4)).toBe(300);
	});

	it('falls back to the minimum for a non-number', () => {
		expect(clampWidth(Number.NaN)).toBe(MIN_IMAGE_WIDTH);
	});
});
