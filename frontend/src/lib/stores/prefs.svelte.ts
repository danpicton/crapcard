import { api } from '$lib/api';

const PAGE_SIZE_KEY = 'crapcard-page-size';

/**
 * Used when the server cannot be asked — an offline first paint still has to
 * pick some page size rather than rendering nothing.
 */
export const FALLBACK_PAGE_SIZE = 50;

/**
 * Client preferences.
 *
 * Page size resolves in three layers, most specific winning: the user's own
 * choice on this device, then the deployment default the server reports from
 * CRAPCARD_PAGE_SIZE, then a hardcoded fallback. Storing the override locally
 * keeps it a per-device display preference rather than account state.
 */
function createPrefsStore() {
	let serverPageSize = $state(FALLBACK_PAGE_SIZE);
	let maxPageSize = $state(500);
	let override = $state<number | null>(null);
	let loaded = $state(false);

	function readOverride(): number | null {
		const raw = localStorage.getItem(PAGE_SIZE_KEY);
		if (raw === null) return null;
		const value = Number(raw);
		// A stale or hand-edited value must not leave the list unable to load.
		if (!Number.isFinite(value) || value <= 0) return null;
		return Math.round(value);
	}

	return {
		get pageSize() {
			const size = override ?? serverPageSize;
			return Math.min(Math.max(size, 1), maxPageSize);
		},
		get deploymentPageSize() {
			return serverPageSize;
		},
		get maxPageSize() {
			return maxPageSize;
		},
		get isOverridden() {
			return override !== null;
		},
		get loaded() {
			return loaded;
		},

		/** Fetch the deployment defaults and apply any stored override. */
		async load() {
			override = readOverride();
			try {
				const config = await api.config();
				if (config.page_size > 0) serverPageSize = config.page_size;
				if (config.max_page_size > 0) maxPageSize = config.max_page_size;
			} catch {
				// Offline or unauthenticated: the fallback still gives a
				// usable list rather than an empty screen.
			} finally {
				loaded = true;
			}
		},

		/**
		 * Set this device's page size, capped at what the server would accept.
		 * A value below one is meaningless, so it is ignored rather than
		 * clamped — leaving the previous setting is less surprising than
		 * silently switching to a page of one.
		 */
		setPageSize(size: number) {
			if (!Number.isFinite(size) || size < 1) return;
			const bounded = Math.min(Math.round(size), maxPageSize);
			override = bounded;
			localStorage.setItem(PAGE_SIZE_KEY, String(bounded));
		},

		/** Drop the override and go back to the deployment default. */
		clearPageSize() {
			override = null;
			localStorage.removeItem(PAGE_SIZE_KEY);
		},

		/** Test seam. */
		reset() {
			serverPageSize = FALLBACK_PAGE_SIZE;
			maxPageSize = 500;
			override = null;
			loaded = false;
		},
	};
}

export const prefs = createPrefsStore();
