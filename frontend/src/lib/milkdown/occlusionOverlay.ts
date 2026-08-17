import { maskStateFor, parseOcclusionFragment } from '$lib/cloze';

/**
 * Draws an image-cloze card's masks over its image.
 *
 * The server carries the mask set in the image URL's #occ= fragment, with
 * rectangles normalised to the image (0–1), so the overlay is pure CSS
 * percentages — no need to know the image's pixel size, and it stays right
 * as the image scales responsively.
 *
 * Returns true when the src carried masks. Re-rendering with a plain src
 * clears whatever was drawn before.
 */
export function renderOcclusionOverlay(host: HTMLElement, src: string): boolean {
	for (const stale of Array.from(host.querySelectorAll('.crapcard-occ-mask'))) {
		stale.remove();
	}

	const frag = parseOcclusionFragment(src);
	if (!frag) return false;

	for (const rect of frag.rects) {
		const state = maskStateFor(rect.id, frag);
		if (state === 'hidden') continue;

		const mask = document.createElement('span');
		mask.className = `crapcard-occ-mask crapcard-occ-${state}`;
		mask.style.left = `${rect.x * 100}%`;
		mask.style.top = `${rect.y * 100}%`;
		mask.style.width = `${rect.w * 100}%`;
		mask.style.height = `${rect.h * 100}%`;
		host.appendChild(mask);
	}
	return true;
}
