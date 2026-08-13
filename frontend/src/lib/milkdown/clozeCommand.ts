import { $prose } from '@milkdown/kit/utils';
import { Plugin, NodeSelection, TextSelection } from '@milkdown/kit/prose/state';
import type { EditorView } from '@milkdown/kit/prose/view';
import { nextClozeNumber, wrapAsCloze } from '$lib/cloze';

/**
 * The cloze command: Alt+C (or the composer's Cloze button) turns the
 * selection into a {{cN::…}} deletion, numbered one past the highest already
 * in the text. On a selected image it instead hands off to the mask editor —
 * an image is clozed by masking areas of it, not by wrapping its markdown.
 */

/** What pressing the shortcut (or the button) should insert, and where the
 * cursor should land inside it. */
export function clozeReplacementFor(
	selectedText: string,
	docText: string,
): { replacement: string; cursorOffset: number } {
	const n = nextClozeNumber(docText);
	const replacement = wrapAsCloze(selectedText, n);
	// With a selection the cursor moves past the marker to keep typing; with
	// nothing selected it lands inside, where the answer goes.
	const cursorOffset = selectedText === '' ? `{{c${n}::`.length : replacement.length;
	return { replacement, cursorOffset };
}

/** True for Alt+C alone. macOS composes Alt-modified letters into other
 * characters ("ç"), so the physical key code is checked too. */
export function isClozeShortcut(event: KeyboardEvent): boolean {
	if (!event.altKey || event.ctrlKey || event.metaKey) return false;
	return event.key === 'c' || event.key === 'C' || event.code === 'KeyC';
}

/**
 * Runs the cloze command against a live editor view. Returns true when it did
 * something. Shared by the keyboard shortcut and the composer button so the
 * two cannot behave differently.
 */
export function performCloze(view: EditorView, onImageCloze?: (src: string) => void): boolean {
	const { state } = view;
	const sel = state.selection;

	// A selected image: cloze means masking areas of it.
	if (sel instanceof NodeSelection && sel.node.type.name === 'image') {
		const src = String(sel.node.attrs.src ?? '');
		if (src && onImageCloze) {
			onImageCloze(src);
			return true;
		}
		return false;
	}

	const selectedText = state.doc.textBetween(sel.from, sel.to, '\n');
	// Markers are plain text, so the whole document's text is what numbering
	// must be computed over.
	const docText = state.doc.textBetween(0, state.doc.content.size, '\n');
	const { replacement, cursorOffset } = clozeReplacementFor(selectedText, docText);

	const tr = state.tr.insertText(replacement, sel.from, sel.to);
	const cursor = tr.doc.resolve(sel.from + cursorOffset);
	view.dispatch(tr.setSelection(TextSelection.near(cursor)).scrollIntoView());
	view.focus();
	return true;
}

/** The Milkdown plugin binding Alt+C inside the editor. */
export function createClozePlugin(onImageCloze?: (src: string) => void) {
	return $prose(
		() =>
			new Plugin({
				props: {
					handleKeyDown(view, event) {
						if (!isClozeShortcut(event)) return false;
						event.preventDefault();
						return performCloze(view, onImageCloze);
					},
				},
			}),
	);
}
