import { api, ApiError, type User } from '$lib/api';

/**
 * Who is signed in.
 *
 * `ready` distinguishes "we have not asked yet" from "asked, nobody is signed
 * in" — without it the app flashes the login screen on every reload before
 * the session check comes back.
 */
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

		/** Ask the server who we are. A 401 simply means nobody. */
		async refresh() {
			error = null;
			try {
				user = await api.me();
			} catch (err) {
				user = null;
				// Not being logged in is the normal first visit, not a failure
				// worth showing the user.
				if (!(err instanceof ApiError && err.unauthorized)) {
					error = err instanceof Error ? err.message : 'could not check your session';
				}
			} finally {
				ready = true;
			}
		},

		async login(username: string, password: string) {
			await api.login(username, password);
			user = await api.me();
			ready = true;
		},

		async setup(username: string, password: string) {
			await api.setup(username, password);
			await api.login(username, password);
			user = await api.me();
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
			ready = true;
		},

		/** Test seam: forget everything. */
		reset() {
			user = null;
			ready = false;
			error = null;
		},
	};
}

export const auth = createAuthStore();
