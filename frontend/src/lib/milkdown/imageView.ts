import { $view } from '@milkdown/kit/utils';
import { imageSchema } from '@milkdown/kit/preset/commonmark';
import type { Node as ProseMirrorNode } from '@milkdown/kit/prose/model';
import type { EditorView } from '@milkdown/kit/prose/view';
import { widthFromSrc, srcWithWidth, clampWidth, MIN_IMAGE_WIDTH } from './imageSize';

/**
 * Node view for images in the card editor: drag the corner to resize, and
 * describe the image in the alt-text box that appears on hover.
 *
 * Both are edits to the image's own attributes, dispatched as one transaction
 * each so undo steps back through them normally.
 */

/** Only our own uploads are resizable; a remote image is left alone. */
function isOwnImage(src: unknown): boolean {
	return typeof src === 'string' && src.startsWith('/api/images/');
}

/**
 * The URL to actually request: the width is a rendering hint the server does
 * not understand, and leaving it on would make every resize a fresh cache
 * entry for identical bytes.
 */
function requestSrc(src: string): string {
	return srcWithWidth(src, null);
}

export const imageView = $view(
	imageSchema.node,
	() =>
		(
			initialNode: ProseMirrorNode,
			view: EditorView,
			getPos: (() => number | undefined) | boolean,
		) => {
			let currentNode = initialNode;

			const wrapper = document.createElement('span');
			wrapper.className = 'crapcard-img';
			wrapper.contentEditable = 'false';

			const img = document.createElement('img');
			img.draggable = false;
			wrapper.appendChild(img);

			/** Applies one attribute change to the document. */
			function setAttrs(attrs: Record<string, unknown>) {
				const pos = typeof getPos === 'function' ? getPos() : undefined;
				if (pos === undefined) return;
				view.dispatch(
					view.state.tr.setNodeMarkup(pos, undefined, { ...currentNode.attrs, ...attrs }),
				);
			}

			function syncFromNode(node: ProseMirrorNode) {
				const src = String(node.attrs.src ?? '');
				img.src = requestSrc(src);
				img.alt = String(node.attrs.alt ?? '');

				const width = widthFromSrc(src);
				img.style.width = width ? `${width}px` : '';
			}

			syncFromNode(initialNode);

			// ── Alt text ────────────────────────────────────────────────────
			// An undescribed image is one nobody can review from a screen
			// reader, and it is also what the card preview will flag, so the
			// box makes its absence visible while editing.
			const toolbar = document.createElement('span');
			toolbar.className = 'crapcard-img-toolbar';
			toolbar.contentEditable = 'false';

			const altInput = document.createElement('input');
			altInput.type = 'text';
			altInput.className = 'crapcard-img-alt';
			altInput.placeholder = 'Describe this image…';
			altInput.value = String(initialNode.attrs.alt ?? '');
			altInput.setAttribute('aria-label', 'Image alt text');
			toolbar.appendChild(altInput);
			wrapper.appendChild(toolbar);

			// Committed on blur and on Enter rather than per keystroke: a
			// transaction per character would bury the edit history.
			function commitAlt() {
				const next = altInput.value;
				if (next !== String(currentNode.attrs.alt ?? '')) setAttrs({ alt: next });
			}
			altInput.addEventListener('blur', commitAlt);
			altInput.addEventListener('keydown', (event: KeyboardEvent) => {
				if (event.key === 'Enter') {
					event.preventDefault();
					commitAlt();
					altInput.blur();
				} else if (event.key === 'Escape') {
					altInput.value = String(currentNode.attrs.alt ?? '');
					altInput.blur();
				}
				// Typing in the box must not reach the editor's own key handling.
				event.stopPropagation();
			});

			// ── Resize ──────────────────────────────────────────────────────
			if (isOwnImage(initialNode.attrs.src)) {
				const handle = document.createElement('span');
				handle.className = 'crapcard-img-handle';
				handle.setAttribute('aria-label', 'Drag to resize');

				handle.addEventListener('mousedown', (event: MouseEvent) => {
					event.preventDefault();
					event.stopPropagation();

					const startX = event.clientX;
					const startWidth = img.offsetWidth || MIN_IMAGE_WIDTH;

					// While dragging only the style changes, so the document
					// gets one undoable transaction at the end rather than one
					// per mouse move.
					const onMove = (moveEvent: MouseEvent) => {
						img.style.width = `${clampWidth(startWidth + moveEvent.clientX - startX)}px`;
					};
					const onUp = (upEvent: MouseEvent) => {
						document.removeEventListener('mousemove', onMove);
						document.removeEventListener('mouseup', onUp);
						const width = clampWidth(startWidth + upEvent.clientX - startX);
						setAttrs({ src: srcWithWidth(String(currentNode.attrs.src ?? ''), width) });
					};

					document.addEventListener('mousemove', onMove);
					document.addEventListener('mouseup', onUp);
				});

				// Double-clicking the handle restores the natural size, which
				// is otherwise impossible to hit by dragging.
				handle.addEventListener('dblclick', (event: MouseEvent) => {
					event.preventDefault();
					event.stopPropagation();
					img.style.width = '';
					setAttrs({ src: srcWithWidth(String(currentNode.attrs.src ?? ''), null) });
				});

				wrapper.appendChild(handle);
			}

			return {
				dom: wrapper as unknown as HTMLElement,
				update(updatedNode: ProseMirrorNode) {
					if (updatedNode.type !== currentNode.type) return false;
					currentNode = updatedNode;
					syncFromNode(updatedNode);
					// Do not fight the user while they are mid-edit.
					if (document.activeElement !== altInput) {
						altInput.value = String(updatedNode.attrs.alt ?? '');
					}
					return true;
				},
				// The alt box is part of the node view, not the document, so
				// ProseMirror must not try to map selections into it.
				ignoreMutation: () => true,
				stopEvent: (event: Event) => event.target === altInput,
				destroy() {},
			};
		},
);
