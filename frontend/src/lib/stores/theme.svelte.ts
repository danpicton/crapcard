const STORAGE_KEY = 'crapcard-theme';

/**
 * Theme identifiers. These match crapnote's exactly, and each has a
 * corresponding [data-theme="..."] block of CSS custom properties in
 * app.html — adding one there is all it takes for it to work here.
 */
export type ThemeId = 'light' | 'dark' | 'console-2001' | 'rosso' | 'bianco' | 'rawblock' | 'verdana';

export interface ThemeOption {
	id: ThemeId;
	label: string;
}

export const THEMES: ThemeOption[] = [
	{ id: 'light', label: 'Claude' },
	{ id: 'dark', label: 'Claude Dark' },
	{ id: 'console-2001', label: 'Console 2001' },
	{ id: 'rosso', label: 'Rosso' },
	{ id: 'bianco', label: 'Bianco' },
	{ id: 'rawblock', label: 'Rawblock' },
	{ id: 'verdana', label: 'Verdana' },
];

function isThemeId(value: unknown): value is ThemeId {
	return THEMES.some((t) => t.id === value);
}

/**
 * The theme a new install starts on. Verdana is the default rather than the
 * Claude theme; a device with no stored preference gets this one.
 */
export const DEFAULT_THEME: ThemeId = 'verdana';

function createThemeStore() {
	let current = $state<ThemeId>(DEFAULT_THEME);

	function applyToDOM(t: ThemeId) {
		document.documentElement.setAttribute('data-theme', t);
		syncBrowserThemeColor();
	}

	/**
	 * Keep <meta name="theme-color"> in step with the active theme so the
	 * browser chrome — and the status bar of an installed PWA — matches the
	 * app instead of staying on the light theme's cream.
	 *
	 * The value is read back from the computed --bg rather than duplicated in
	 * a lookup table here, so adding a theme to app.html is all it takes.
	 */
	function syncBrowserThemeColor() {
		const meta = document.querySelector('meta[name="theme-color"]');
		if (!meta) return;
		const bg = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim();
		// Empty before the stylesheet has applied — leave the markup default
		// in place rather than blanking it.
		if (bg) meta.setAttribute('content', bg);
	}

	return {
		get current() {
			return current;
		},

		/** Resolve and apply the stored theme. Call once, on mount. */
		init() {
			const stored = localStorage.getItem(STORAGE_KEY);
			current = isThemeId(stored) ? stored : DEFAULT_THEME;
			applyToDOM(current);
		},

		/** Switch theme and remember the choice on this device. */
		set(t: ThemeId) {
			if (!isThemeId(t)) return;
			current = t;
			localStorage.setItem(STORAGE_KEY, t);
			applyToDOM(t);
		},
	};
}

export const theme = createThemeStore();
