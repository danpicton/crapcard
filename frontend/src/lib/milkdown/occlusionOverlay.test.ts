// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { renderOcclusionOverlay } from './occlusionOverlay';

function srcWith(payload: unknown): string {
	return '/api/images/abc#occ=' + encodeURIComponent(JSON.stringify(payload));
}

const rects = [
	{ id: 1, x: 0.1, y: 0.2, w: 0.3, h: 0.4 },
	{ id: 2, x: 0.6, y: 0.6, w: 0.2, h: 0.1 },
];

describe('renderOcclusionOverlay', () => {
	it('does nothing for a plain image', () => {
		const host = document.createElement('span');
		expect(renderOcclusionOverlay(host, '/api/images/abc')).toBe(false);
		expect(host.children.length).toBe(0);
	});

	it('draws the tested mask on a hide-one question, skips the rest', () => {
		const host = document.createElement('span');
		const occluded = renderOcclusionOverlay(
			host,
			srcWith({ mode: 'hide-one', side: 'q', test: 1, rects }),
		);
		expect(occluded).toBe(true);

		const masks = host.querySelectorAll('.crapcard-occ-mask');
		expect(masks.length).toBe(1);
		const el = masks[0] as HTMLElement;
		expect(el.classList.contains('crapcard-occ-covered-tested')).toBe(true);
		expect(el.style.left).toBe('10%');
		expect(el.style.top).toBe('20%');
		expect(el.style.width).toBe('30%');
		expect(el.style.height).toBe('40%');
	});

	it('covers every mask on a hide-all question', () => {
		const host = document.createElement('span');
		renderOcclusionOverlay(host, srcWith({ mode: 'hide-all', side: 'q', test: 1, rects }));
		expect(host.querySelectorAll('.crapcard-occ-mask').length).toBe(2);
		expect(host.querySelectorAll('.crapcard-occ-covered-tested').length).toBe(1);
		expect(host.querySelectorAll('.crapcard-occ-covered').length).toBe(1);
	});

	it('outlines only the tested mask on the answer', () => {
		const host = document.createElement('span');
		renderOcclusionOverlay(host, srcWith({ mode: 'hide-all', side: 'a', test: 2, rects }));
		const masks = host.querySelectorAll('.crapcard-occ-mask');
		expect(masks.length).toBe(1);
		expect(masks[0].classList.contains('crapcard-occ-outline')).toBe(true);
	});

	it('clears stale masks when re-rendered with a plain image', () => {
		const host = document.createElement('span');
		renderOcclusionOverlay(host, srcWith({ mode: 'hide-one', side: 'q', test: 1, rects }));
		renderOcclusionOverlay(host, '/api/images/abc');
		expect(host.querySelectorAll('.crapcard-occ-mask').length).toBe(0);
	});
});
