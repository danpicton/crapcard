import { api, ApiError, type User } from '$lib/api';

const USER_KEY = 'crapcard-user';

/**
 * Who is signed in.
 *
 * `ready` distinguishes "we have not asked yet" from "asked, nobody is signed
 * in" — without it the app flashes the login screen on every reload before
 * the session check comes back.
 *
 * The user is also cached locally: an offline start adopts the cached session
 * and renders at once, and only a definitive answer from the server — a 401 —
 * signs it out. A network failure is not evidence about the session.
 */
function readCachedUser(): User | null {
	try {
		const raw = localStorage.getItem(USER_KEY);
		if (!raw) return null;
		const parsed: unknown = JSON.parse(raw);
		if (parsed && typeof parsed === 'object' && 'username' in parsed) return parsed as User;
		return null;
	} catch {
		return null;
	}
}

function writeCachedUser(user: User | null) {
	try {
		if (user === null) localStorage.removeItem(USER_KEY);
		else localStorage.setItem(USER_KEY, JSON.stringify(user));
	} catch {
		// Storage full or unavailable: only offline resume degrades.
	}
}

function createAuthStore() {
	let user = $state<User | null>(null);
	let ready = $state(false);
	let error = $state<string | null>(null);

	return {
		get user() {
			return user;
		},
		get ready() {
			return ready;
		},
		get error() {
			return error;
		},
		get signedIn() {
			return user !== null;
		},

		/** Adopt the cached session, so the app can render without waiting on
		 * the network. Verify with refresh() afterwards. */
		init() {
			const cached = readCachedUser();
			if (cached !== null) {
				user = cached;
				ready = true;
			}
		},

		/** Ask the server who we are. A 401 simply means nobody. */
		async refresh() {
			error = null;
			try {
				user = await api.me();
				writeCachedUser(user);
			} catch (err) {
				if (err instanceof ApiError && err.status === 0) {
					// Offline: the cached session stands until the server can
					// actually be asked.
					user = user ?? readCachedUser();
				} else {
					user = null;
					// Not being logged in is the normal first visit, not a failure
					// worth showing the user.
					if (err instanceof ApiError && err.unauthorized) {
						writeCachedUser(null);
					} else {
						error = err instanceof Error ? err.message : 'could not check your session';
					}
				}
			} finally {
				ready = true;
			}
		},

		async login(username: string, password: string) {
			await api.login(username, password);
			user = await api.me();
			writeCachedUser(user);
			ready = true;
		},

		async setup(username: string, password: string) {
			await api.setup(username, password);
			await api.login(username, password);
			user = await api.me();
			writeCachedUser(user);
			ready = true;
		},

		async logout() {
			try {
				await api.logout();
			} catch {
				// The cookie may already be gone, or the network may be down.
				// Either way the user asked to sign out, so honour it locally
				// rather than leaving a stale user on screen.
			}
			user = null;
			writeCachedUser(null);
			ready = true;
		},

		/** Test seam: forget everything. */
		reset() {
			user = null;
			ready = false;
			error = null;
			writeCachedUser(null);
		},
	};
}

export const auth = createAuthStore();
