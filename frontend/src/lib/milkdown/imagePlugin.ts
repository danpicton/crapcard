import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import type { EditorView } from '@milkdown/kit/prose/view';
import { $prose as prosePlugin } from '@milkdown/kit/utils';
import { api } from '$lib/api';
import { imageBlobsFrom, type UploadFn } from './imagePaste';

/**
 * Inserts an image node at the current selection.
 *
 * The node is built from the schema rather than by splicing markdown text, so
 * the document stays a valid ProseMirror doc and undo treats the insert as one
 * step.
 */
function insertImageAt(view: EditorView, url: string) {
	const { state, dispatch } = view;
	const imageType = state.schema.nodes.image;
	if (!imageType) return;
	const node = imageType.create({ src: url, alt: '' });
	dispatch(state.tr.replaceSelectionWith(node).scrollIntoView());
}

/**
 * Handles images pasted or dropped into a card field.
 *
 * The upload function is injectable so this can be exercised against a stub;
 * imagePaste.ts holds the decision logic and is tested on its own.
 */
export function createImagePastePlugin(
	upload: UploadFn = (blob) => api.uploadImage(blob),
	onError: (message: string) => void = () => {},
) {
	return prosePlugin(() =>
		new Plugin({
			key: new PluginKey('crapcard-image-paste'),
			props: {
				handlePaste(view: EditorView, event: ClipboardEvent) {
					const blobs = imageBlobsFrom(event.clipboardData);
					if (blobs.length === 0) return false;

					// Claim the paste so the browser does not also drop in its
					// own representation of the image.
					event.preventDefault();
					void uploadAll(view, blobs, upload, onError);
					return true;
				},

				handleDrop(view: EditorView, event: DragEvent) {
					const blobs = imageBlobsFrom(event.dataTransfer);
					if (blobs.length === 0) return false;

					event.preventDefault();
					void uploadAll(view, blobs, upload, onError);
					return true;
				},
			},
		}),
	);
}

async function uploadAll(
	view: EditorView,
	blobs: File[],
	upload: UploadFn,
	onError: (message: string) => void,
) {
	for (const blob of blobs) {
		try {
			const { url } = await upload(blob);
			insertImageAt(view, url);
		} catch (err) {
			// Surface it: an image the user believes they pasted, but which
			// never uploaded, would only be noticed mid-review.
			onError(err instanceof Error ? err.message : 'image upload failed');
		}
	}
}
