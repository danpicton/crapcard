import { describe, it, expect, vi } from 'vitest';
import { imageBlobsFrom, handleImagePaste } from './imagePaste';

/** Builds a DataTransfer-like object matching what a paste event carries. */
function clipboardWith(items: Array<{ kind: string; type: string; file?: File | null }>) {
	return {
		items: items.map((i) => ({
			kind: i.kind,
			type: i.type,
			getAsFile: () => i.file ?? null,
		})),
		files: items.map((i) => i.file).filter(Boolean) as File[],
		types: items.map((i) => i.type),
	} as unknown as DataTransfer;
}

function pngFile(name = 'image.png'): File {
	return new File([new Uint8Array([137, 80, 78, 71])], name, { type: 'image/png' });
}

describe('imageBlobsFrom', () => {
	it('finds a pasted screenshot', () => {
		const data = clipboardWith([{ kind: 'file', type: 'image/png', file: pngFile() }]);
		expect(imageBlobsFrom(data)).toHaveLength(1);
	});

	it('ignores pasted text', () => {
		const data = clipboardWith([{ kind: 'string', type: 'text/plain' }]);
		expect(imageBlobsFrom(data)).toHaveLength(0);
	});

	it('ignores non-image files such as a dragged PDF', () => {
		const pdf = new File([new Uint8Array([1])], 'notes.pdf', { type: 'application/pdf' });
		const data = clipboardWith([{ kind: 'file', type: 'application/pdf', file: pdf }]);
		expect(imageBlobsFrom(data)).toHaveLength(0);
	});

	it('picks up every image when several are pasted at once', () => {
		const data = clipboardWith([
			{ kind: 'file', type: 'image/png', file: pngFile('a.png') },
			{ kind: 'file', type: 'image/jpeg', file: pngFile('b.jpg') },
		]);
		expect(imageBlobsFrom(data)).toHaveLength(2);
	});

	it('copes with an empty clipboard', () => {
		expect(imageBlobsFrom(null)).toHaveLength(0);
		expect(imageBlobsFrom(clipboardWith([]))).toHaveLength(0);
	});

	it('skips an item that claims to be a file but yields nothing', () => {
		const data = clipboardWith([{ kind: 'file', type: 'image/png', file: null }]);
		expect(imageBlobsFrom(data)).toHaveLength(0);
	});
});

describe('handleImagePaste', () => {
	it('uploads the image and returns markdown pointing at it', async () => {
		const upload = vi.fn().mockResolvedValue({ url: '/api/images/abc123' });
		const data = clipboardWith([{ kind: 'file', type: 'image/png', file: pngFile() }]);

		const result = await handleImagePaste(data, upload);

		expect(upload).toHaveBeenCalledOnce();
		expect(result.handled).toBe(true);
		expect(result.markdown).toBe('![](/api/images/abc123)');
	});

	it('joins several pasted images into one markdown block', async () => {
		const upload = vi
			.fn()
			.mockResolvedValueOnce({ url: '/api/images/one' })
			.mockResolvedValueOnce({ url: '/api/images/two' });
		const data = clipboardWith([
			{ kind: 'file', type: 'image/png', file: pngFile('a.png') },
			{ kind: 'file', type: 'image/png', file: pngFile('b.png') },
		]);

		const result = await handleImagePaste(data, upload);

		expect(result.markdown).toBe('![](/api/images/one)\n\n![](/api/images/two)');
	});

	it('does not claim the paste when there is no image, so text pastes normally', async () => {
		const upload = vi.fn();
		const data = clipboardWith([{ kind: 'string', type: 'text/plain' }]);

		const result = await handleImagePaste(data, upload);

		expect(result.handled).toBe(false);
		expect(upload).not.toHaveBeenCalled();
	});

	it('reports a failed upload instead of inserting a broken image', async () => {
		// A dead link in a card is worse than a visible error: the user would
		// only discover it mid-review.
		const upload = vi.fn().mockRejectedValue(new Error('image is too large'));
		const data = clipboardWith([{ kind: 'file', type: 'image/png', file: pngFile() }]);

		const result = await handleImagePaste(data, upload);

		expect(result.handled).toBe(true);
		expect(result.markdown).toBe('');
		expect(result.error).toContain('too large');
	});

	it('inserts the images that did upload when only some fail', async () => {
		const upload = vi
			.fn()
			.mockResolvedValueOnce({ url: '/api/images/ok' })
			.mockRejectedValueOnce(new Error('nope'));
		const data = clipboardWith([
			{ kind: 'file', type: 'image/png', file: pngFile('a.png') },
			{ kind: 'file', type: 'image/png', file: pngFile('b.png') },
		]);

		const result = await handleImagePaste(data, upload);

		expect(result.markdown).toBe('![](/api/images/ok)');
		expect(result.error).toBeTruthy();
	});
});
