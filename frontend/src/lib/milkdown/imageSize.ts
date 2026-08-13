/**
 * Image sizing, carried in the image URL as ?w=<pixels>.
 *
 * The width lives in the URL rather than in markdown syntax or an HTML tag so
 * that a card stays plain CommonMark: any other renderer still shows the
 * image (just at its natural size), the alt text stays real alt text, and no
 * user-authored HTML has to be rendered from our own origin. The server
 * ignores the parameter — it only ever means something to the editor and the
 * card renderer.
 */

/** Below this an image is too small to see, and too small to grab and resize. */
export const MIN_IMAGE_WIDTH = 32;

/** Above this a drag has clearly run away. */
export const MAX_IMAGE_WIDTH = 2000;

/** Splits a src into its path and its query parameters. */
function parts(src: string): { path: string; params: URLSearchParams } {
	const index = src.indexOf('?');
	if (index === -1) return { path: src, params: new URLSearchParams() };
	return { path: src.slice(0, index), params: new URLSearchParams(src.slice(index + 1)) };
}

/** Reads the width an image carries, or null when it has none or a bad one. */
export function widthFromSrc(src: string | null | undefined): number | null {
	if (!src) return null;
	const raw = parts(src).params.get('w');
	if (raw === null || raw.trim() === '') return null;

	const width = Number(raw);
	// A nonsense value is treated as unsized rather than rendering something
	// broken.
	if (!Number.isFinite(width) || width <= 0) return null;
	return Math.round(width);
}

/** Returns the src with the width set, replaced, or (given null) removed. */
export function srcWithWidth(src: string, width: number | null): string {
	const { path, params } = parts(src);

	if (width === null) {
		params.delete('w');
	} else {
		params.set('w', String(Math.round(width)));
	}

	const query = params.toString();
	return query ? `${path}?${query}` : path;
}

/** Holds a dragged width inside what is usable. */
export function clampWidth(width: number): number {
	if (!Number.isFinite(width)) return MIN_IMAGE_WIDTH;
	return Math.round(Math.min(Math.max(width, MIN_IMAGE_WIDTH), MAX_IMAGE_WIDTH));
}
