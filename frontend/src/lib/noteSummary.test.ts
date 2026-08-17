import { describe, it, expect } from 'vitest';
import { summariseMarkdown, pageSizeOptionsFor } from './noteSummary';

describe('summariseMarkdown', () => {
	it('flattens cloze markers to their answer text', () => {
		expect(summariseMarkdown('{{c1::Ottawa}} is in {{c2::Canada::country}}.')).toBe(
			'Ottawa is in Canada.',
		);
	});

	it('strips emphasis so a list row reads as prose', () => {
		expect(summariseMarkdown('Bone **7**?')).toBe('Bone 7?');
		expect(summariseMarkdown('a *word* and _another_')).toBe('a word and another');
		expect(summariseMarkdown('~~struck~~')).toBe('struck');
	});

	it('describes an image by its alt text, and says when there is none', () => {
		expect(summariseMarkdown('![the femur](/api/images/a?w=300)')).toBe('image: the femur');
		expect(summariseMarkdown('![](/api/images/a)')).toBe('image: no alt text');
	});

	it('keeps link text and drops the target', () => {
		expect(summariseMarkdown('see [the femur](https://example.com)')).toBe('see the femur');
	});

	it('strips heading, quote and list markers', () => {
		expect(summariseMarkdown('## Heading')).toBe('Heading');
		expect(summariseMarkdown('> quoted')).toBe('quoted');
		expect(summariseMarkdown('- item')).toBe('item');
		expect(summariseMarkdown('1. item')).toBe('item');
	});

	it('unwraps inline code', () => {
		expect(summariseMarkdown('the `femur` bone')).toBe('the femur bone');
	});

	it('uses the first line that has content', () => {
		expect(summariseMarkdown('\n\n  \nThe femur\n\nmore')).toBe('The femur');
	});

	it('truncates a long line', () => {
		const long = 'x'.repeat(200);
		const result = summariseMarkdown(long);
		expect(result.length).toBeLessThanOrEqual(81);
		expect(result.endsWith('…')).toBe(true);
	});

	it('copes with empty content', () => {
		expect(summariseMarkdown('')).toBe('');
		expect(summariseMarkdown('   ')).toBe('');
	});
});

describe('pageSizeOptionsFor', () => {
	it('offers the standard sizes', () => {
		expect(pageSizeOptionsFor(50)).toContain(50);
		expect(pageSizeOptionsFor(50)).toContain(10);
	});

	it('includes the active size when it is not a standard one', () => {
		// A deployment can set any CRAPCARD_PAGE_SIZE it likes; without this
		// the dropdown would render blank because nothing matches.
		const options = pageSizeOptionsFor(30);
		expect(options).toContain(30);
	});

	it('keeps the list sorted and free of duplicates', () => {
		const options = pageSizeOptionsFor(25);
		expect(options).toEqual([...new Set(options)].sort((a, b) => a - b));
	});
});
