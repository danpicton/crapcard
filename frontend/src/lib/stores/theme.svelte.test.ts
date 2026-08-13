import { describe, it, expect, beforeEach } from 'vitest';
import { theme, THEMES, DEFAULT_THEME, type ThemeId } from './theme.svelte';

const STORAGE_KEY = 'crapcard-theme';

describe('theme store', () => {
	beforeEach(() => {
		localStorage.clear();
		document.documentElement.removeAttribute('data-theme');
		// Inline custom properties set by an earlier test would otherwise leak
		// into the next one's computed --bg.
		document.documentElement.removeAttribute('style');
		document.head.innerHTML = '<meta name="theme-color" content="#faf8f4" />';
	});

	it('offers the same themes as crapnote so the two apps match', () => {
		const ids = THEMES.map((t) => t.id);
		expect(ids).toEqual([
			'light',
			'dark',
			'console-2001',
			'rosso',
			'bianco',
			'rawblock',
			'verdana',
		]);
	});

	it('defaults to Verdana when nothing is stored', () => {
		theme.init();
		expect(theme.current).toBe(DEFAULT_THEME);
		expect(theme.current).toBe('verdana');
		expect(document.documentElement.getAttribute('data-theme')).toBe('verdana');
	});

	it('a stored choice still wins over the default', () => {
		localStorage.setItem(STORAGE_KEY, 'rosso');
		theme.init();
		expect(theme.current).toBe('rosso');
		expect(document.documentElement.getAttribute('data-theme')).toBe('rosso');
	});

	it('ignores a stored value that is not a real theme', () => {
		// A stale or hand-edited value must not leave the app unstyled.
		localStorage.setItem(STORAGE_KEY, 'not-a-theme');
		theme.init();
		expect(theme.current).toBe(DEFAULT_THEME);
	});

	it('applies and persists a theme when set', () => {
		theme.init();
		theme.set('console-2001');

		expect(theme.current).toBe('console-2001');
		expect(document.documentElement.getAttribute('data-theme')).toBe('console-2001');
		expect(localStorage.getItem(STORAGE_KEY)).toBe('console-2001');
	});

	it('refuses to set a theme that does not exist', () => {
		theme.init();
		theme.set('nonsense' as ThemeId);
		expect(theme.current).toBe(DEFAULT_THEME);
	});

	it('keeps the browser chrome colour in step with the theme', () => {
		// The PWA status bar reads this; leaving it on the light theme's cream
		// looks broken in a dark theme.
		document.documentElement.style.setProperty('--bg', '#181818');
		theme.init();
		theme.set('rosso');

		const meta = document.querySelector('meta[name="theme-color"]');
		expect(meta?.getAttribute('content')).toBe('#181818');
	});

	it('leaves the markup default alone when no --bg has resolved yet', () => {
		// Before the stylesheet applies, computed --bg is empty; blanking the
		// meta tag would be worse than leaving it.
		theme.init();
		theme.set('dark');

		const meta = document.querySelector('meta[name="theme-color"]');
		expect(meta?.getAttribute('content')).toBe('#faf8f4');
	});
});
