/**
 * Helpers for showing notes in a list.
 */

import { stripClozeMarkers } from './cloze';

const MAX_SUMMARY_LENGTH = 80;

/**
 * Reduces a card field to one line of readable prose for a list row.
 *
 * The stored value is markdown, and showing it raw puts asterisks and image
 * URLs in front of the user. This is deliberately a display-only flattening,
 * not a parser: the real rendering happens in the editor and in preview.
 *
 * Images become their alt text, matching how the preview describes them, so
 * an undescribed image reads as a bare [image] in both places.
 */
export function summariseMarkdown(value: string): string {
	const line = value.split('\n').find((l) => l.trim() !== '') ?? '';

	// Cloze markers flatten to their answer text before anything else: the
	// list shows what the note says, not how it is tested.
	const flattened = stripClozeMarkers(line)
		// Images first, so their alt text is not mistaken for link text. An
		// undescribed image says so explicitly — that is how you notice.
		.replace(/!\[([^\]]*)\]\([^)]*\)/g, (_match, alt: string) =>
			alt.trim() ? `image: ${alt.trim()}` : 'image: no alt text',
		)
		// Links keep their text, lose their target.
		.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
		// Leading block markers: heading, quote, bullet, ordered item.
		.replace(/^\s{0,3}(#{1,6}\s+|>\s?|[-*+]\s+|\d+\.\s+)/, '')
		// Emphasis and strikethrough.
		.replace(/(\*\*|__)(.*?)\1/g, '$2')
		.replace(/(\*|_)(.*?)\1/g, '$2')
		.replace(/~~(.*?)~~/g, '$1')
		// Inline code.
		.replace(/`([^`]*)`/g, '$1')
		.trim();

	return flattened.length > MAX_SUMMARY_LENGTH
		? `${flattened.slice(0, MAX_SUMMARY_LENGTH)}…`
		: flattened;
}

/** The page sizes offered in the pickers. */
const STANDARD_PAGE_SIZES = [10, 25, 50, 100, 200];

/**
 * Page sizes to offer, always including the one currently in force.
 *
 * A deployment can set CRAPCARD_PAGE_SIZE to anything; without folding that
 * value in, the dropdown would have nothing matching selected and render
 * blank.
 */
export function pageSizeOptionsFor(current: number): number[] {
	const sizes = new Set(STANDARD_PAGE_SIZES);
	if (Number.isFinite(current) && current > 0) sizes.add(Math.round(current));
	return [...sizes].sort((a, b) => a - b);
}
