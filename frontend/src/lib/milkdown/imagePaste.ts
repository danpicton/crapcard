/**
 * Clipboard image handling for the card editor.
 *
 * Kept out of the editor component so it can be tested without instantiating
 * Milkdown: given the clipboard contents and a way to upload, it decides
 * whether it owns the paste and what markdown to insert.
 */

export interface UploadResult {
	url: string;
}

export type UploadFn = (blob: Blob) => Promise<UploadResult>;

export interface PasteOutcome {
	/** True when the paste contained images, so the caller should preventDefault. */
	handled: boolean;
	/** Markdown to insert. Empty when every upload failed. */
	markdown: string;
	/** A message to show the user when one or more uploads failed. */
	error?: string;
}

/** Extracts the image files from a clipboard payload, ignoring everything else. */
export function imageBlobsFrom(data: DataTransfer | null): File[] {
	if (!data?.items) return [];

	const blobs: File[] = [];
	for (const item of Array.from(data.items)) {
		if (item.kind !== 'file' || !item.type.startsWith('image/')) continue;
		const file = item.getAsFile();
		// An item can advertise itself as a file and still yield nothing.
		if (file) blobs.push(file);
	}
	return blobs;
}

/**
 * Uploads any images on the clipboard and returns the markdown for them.
 *
 * When the clipboard holds no image the paste is left alone, so ordinary text
 * pasting is untouched. A failed upload reports an error rather than
 * inserting a link to an image that was never stored — a dead reference in a
 * card would only be discovered mid-review.
 */
export async function handleImagePaste(
	data: DataTransfer | null,
	upload: UploadFn,
): Promise<PasteOutcome> {
	const blobs = imageBlobsFrom(data);
	if (blobs.length === 0) {
		return { handled: false, markdown: '' };
	}

	const urls: string[] = [];
	const failures: string[] = [];

	for (const blob of blobs) {
		try {
			const uploaded = await upload(blob);
			urls.push(uploaded.url);
		} catch (err) {
			failures.push(err instanceof Error ? err.message : 'upload failed');
		}
	}

	const outcome: PasteOutcome = {
		handled: true,
		markdown: urls.map((url) => `![](${url})`).join('\n\n'),
	};
	if (failures.length > 0) {
		outcome.error =
			failures.length === 1
				? `Could not paste image: ${failures[0]}`
				: `Could not paste ${failures.length} images: ${failures.join('; ')}`;
	}
	return outcome;
}
