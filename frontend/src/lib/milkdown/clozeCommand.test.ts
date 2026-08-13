import { describe, expect, it } from 'vitest';
import { clozeReplacementFor, isClozeShortcut } from './clozeCommand';

describe('clozeReplacementFor', () => {
	it('wraps the selection with the next unused number', () => {
		const r = clozeReplacementFor('Ottawa', 'Already has {{c1::one}} marker');
		expect(r.replacement).toBe('{{c2::Ottawa}}');
		// Cursor lands after the whole marker, ready to keep typing.
		expect(r.cursorOffset).toBe(r.replacement.length);
	});

	it('starts at c1 in fresh text', () => {
		expect(clozeReplacementFor('Ottawa', 'Ottawa is nice').replacement).toBe('{{c1::Ottawa}}');
	});

	it('opens an empty marker when nothing is selected, cursor inside', () => {
		const r = clozeReplacementFor('', 'plain');
		expect(r.replacement).toBe('{{c1::}}');
		// Cursor sits between :: and }} so the answer can be typed straight in.
		expect(r.cursorOffset).toBe('{{c1::'.length);
	});
});

describe('isClozeShortcut', () => {
	const event = (init: Partial<KeyboardEvent>) => init as KeyboardEvent;

	it('matches Alt+C', () => {
		expect(isClozeShortcut(event({ key: 'c', altKey: true }))).toBe(true);
		// macOS reports Alt-modified letters oddly; the code is the anchor.
		expect(isClozeShortcut(event({ key: 'ç', code: 'KeyC', altKey: true }))).toBe(true);
	});

	it('ignores plain c, Ctrl+C and Alt with another key', () => {
		expect(isClozeShortcut(event({ key: 'c' }))).toBe(false);
		expect(isClozeShortcut(event({ key: 'c', ctrlKey: true }))).toBe(false);
		expect(isClozeShortcut(event({ key: 'x', code: 'KeyX', altKey: true }))).toBe(false);
		expect(isClozeShortcut(event({ key: 'c', altKey: true, ctrlKey: true }))).toBe(false);
	});
});
