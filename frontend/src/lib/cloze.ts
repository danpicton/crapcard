/**
 * Cloze deletion, client side.
 *
 * The user never picks a note type: they type {{c1::…}} markers (or press
 * Alt+C), or mask areas of an image, and the note's type is re-detected from
 * its content on every save. These helpers keep that detection — and the
 * marker syntax — in one place, mirroring the server's parser.
 */

/** One mask over an image, in coordinates normalised to the image (0–1). */
export interface OcclusionRect {
	id: number;
	x: number;
	y: number;
	w: number;
	h: number;
}

/** The mask set of an image-cloze note, stored in the note's config. */
export interface Occlusion {
	/** 'hide-one' covers only the tested mask; 'hide-all' covers every mask. */
	mode: 'hide-one' | 'hide-all';
	rects: OcclusionRect[];
}

/** What the server embeds in a rendered image URL's #occ= fragment. */
export interface OcclusionFragment {
	mode: string;
	/** 'q' for the question side, 'a' for the answer side. */
	side: string;
	/** The mask this card asks about. */
	test: number;
	rects: OcclusionRect[];
}

/**
 * Anki's marker syntax: {{c1::answer}} or {{c1::answer::hint}}. Numbering
 * starts at 1, and nesting is not supported — same rules as the server.
 */
const CLOZE_MARKER = /\{\{c([1-9][0-9]*)::(.*?)\}\}/g;

/**
 * The type a note's content implies. Masks win over text markers: a note is
 * one type, and masks are the more deliberate act.
 */
export function detectNoteType(
	front: string,
	occlusion: Occlusion | null,
): 'basic' | 'cloze' | 'image-cloze' {
	if (occlusion && occlusion.rects.length > 0) return 'image-cloze';
	if (front.match(CLOZE_MARKER)) return 'cloze';
	return 'basic';
}

/** The number Alt+C should use next: one past the highest in the text. */
export function nextClozeNumber(front: string): number {
	let highest = 0;
	for (const m of front.matchAll(CLOZE_MARKER)) {
		highest = Math.max(highest, Number(m[1]));
	}
	return highest + 1;
}

/** Wraps selected text as one deletion. */
export function wrapAsCloze(text: string, number: number): string {
	return `{{c${number}::${text}}}`;
}

/**
 * Reduces markers to their answer text, for the note list — the list shows
 * what the note says, not how it is tested.
 */
export function stripClozeMarkers(text: string): string {
	return text.replace(CLOZE_MARKER, (_match, _n: string, content: string) => {
		const i = content.indexOf('::');
		return i >= 0 ? content.slice(0, i) : content;
	});
}

/**
 * Reads the mask payload out of a rendered image URL, or null when there is
 * none — or it is mangled, which must degrade to a plain image rather than
 * blowing up mid-render.
 */
export function parseOcclusionFragment(src: string): OcclusionFragment | null {
	const i = src.indexOf('#occ=');
	if (i < 0) return null;
	try {
		const parsed = JSON.parse(decodeURIComponent(src.slice(i + '#occ='.length)));
		if (!parsed || !Array.isArray(parsed.rects)) return null;
		return parsed as OcclusionFragment;
	} catch {
		return null;
	}
}

/** A card template's human name, for the preview's tabs and headings. */
export function templateLabel(template: string): string {
	if (template === 'forward') return 'Front → back';
	if (template === 'reverse') return 'Back → front';
	const cloze = template.match(/^cloze:(\d+)$/);
	if (cloze) return `Cloze ${cloze[1]}`;
	const occ = template.match(/^occ:(\d+)$/);
	if (occ) return `Mask ${occ[1]}`;
	return template;
}

/** How one mask should be drawn on a rendered card. */
export type MaskState = 'covered' | 'covered-tested' | 'outline' | 'hidden';

/**
 * The rendering rule for a mask, given which side of which card is showing.
 *
 * Question: the tested mask is always covered (and marked, so in hide-all
 * mode the user knows which blank is being asked). hide-one leaves the others
 * revealed; hide-all covers them too. Answer: everything is revealed, with
 * the tested area outlined so the eye lands on what was asked.
 */
export function maskStateFor(rectId: number, frag: OcclusionFragment): MaskState {
	if (frag.side === 'a') {
		return rectId === frag.test ? 'outline' : 'hidden';
	}
	if (rectId === frag.test) return 'covered-tested';
	return frag.mode === 'hide-all' ? 'covered' : 'hidden';
}

/** The URL to actually fetch: everything before the fragment. */
export function srcWithoutOcclusion(src: string): string {
	const i = src.indexOf('#occ=');
	return i < 0 ? src : src.slice(0, i);
}
