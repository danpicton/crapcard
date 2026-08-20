<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import { auth } from '$lib/stores/auth.svelte';
	import { prefs } from '$lib/stores/prefs.svelte';
	import { sync } from '$lib/stores/sync.svelte';
	import { api } from '$lib/api';

	let { children } = $props();

	let checking = $state(true);

	// The mobile nav drawer. Closed on every navigation.
	let menuOpen = $state(false);

	$effect(() => {
		void page.url.pathname;
		menuOpen = false;
	});

	// Study screens pin their controls to the bottom on mobile; the layout
	// hands them a fixed-height column to do it in.
	const studyRoute = $derived(
		page.url.pathname === '/' || /^\/decks\/\d+\/study\/?$/.test(page.url.pathname),
	);

	// Routes reachable without a session.
	const publicRoutes = ['/login', '/setup'];

	onMount(async () => {
		theme.init();
		sync.init();

		// A cached session renders the app immediately — no waiting on the
		// network — and the server check runs behind it, signing out only
		// when the server definitively answers 401.
		auth.init();
		if (auth.signedIn) {
			checking = false;
			void prefs.load();
			void auth.refresh().then(() => {
				if (!auth.signedIn && !publicRoutes.includes(page.url.pathname)) {
					void goto('/login');
				}
			});
			return;
		}

		await auth.refresh();
		if (auth.signedIn) await prefs.load();

		if (!auth.signedIn && !publicRoutes.includes(page.url.pathname)) {
			// A brand new instance should land on setup, not on a login form
			// nobody can satisfy yet.
			try {
				const status = await api.setupStatus();
				await goto(status.needs_setup ? '/setup' : '/login');
			} catch {
				await goto('/login');
			}
		}
		checking = false;
	});

	async function signOut() {
		await auth.logout();
		await goto('/login');
	}
</script>

<div class="app" class:study={studyRoute}>
	<header class="topbar">
		{#if auth.signedIn}
			<button
				type="button"
				class="menu-toggle"
				aria-label="Menu"
				aria-expanded={menuOpen}
				onclick={() => (menuOpen = !menuOpen)}
			>
				<svg
					width="22"
					height="22"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					aria-hidden="true"
				>
					<path d="M4 6h16" />
					<path d="M4 12h16" />
					<path d="M4 18h16" />
				</svg>
			</button>
		{/if}
		<a class="wordmark" href="/">crapcard</a>

		{#if !sync.online}
			<span class="sync-badge offline" title="Changes sync when back online">
				● Offline{sync.pending > 0 ? ` · ${sync.pending} queued` : ''}
			</span>
		{:else if sync.flushing}
			<span class="sync-badge">Syncing…</span>
		{:else if sync.pending > 0}
			<span class="sync-badge">{sync.pending} to sync</span>
		{/if}

		{#if auth.signedIn}
			<nav class:open={menuOpen}>
				<a href="/" class:active={page.url.pathname === '/'}>Study</a>
				<a href="/decks" class:active={page.url.pathname.startsWith('/decks')}>Decks</a>
				<a href="/settings" class:active={page.url.pathname === '/settings'}>Settings</a>
				<button type="button" class="link" onclick={signOut}>Sign out</button>
			</nav>
		{/if}
	</header>

	<main class:study={studyRoute}>
		{#if checking}
			<p class="muted">Loading…</p>
		{:else}
			{@render children?.()}
		{/if}
	</main>
</div>

<style>
	.app {
		/* dvh tracks the visible viewport as mobile browser chrome shows and
		   hides; vh stays as the fallback for older engines. */
		min-height: 100vh;
		min-height: 100dvh;
		display: flex;
		flex-direction: column;
	}

	.topbar {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem 1.25rem;
		border-bottom: 1px solid var(--border);
		background: var(--bg-toolbar);
	}

	/* The drawer toggle exists only on mobile. */
	.menu-toggle {
		display: none;
		background: none;
		border: none;
		padding: 0.5rem;
		margin: -0.5rem 0 -0.5rem -0.5rem;
		color: var(--text);
		cursor: pointer;
		align-items: center;
	}

	.wordmark {
		font-family: var(--serif);
		font-size: 1.25rem;
		font-weight: 700;
		letter-spacing: -0.02em;
		color: var(--text);
		text-decoration: none;
	}

	/* Quiet when all is well; the badge only appears when something is
	   queued, syncing, or the network is away. */
	.sync-badge {
		font-size: 0.75rem;
		padding: 0.125rem 0.5rem;
		border-radius: 999px;
		background: var(--bg-hover);
		color: var(--text-2);
		margin-right: auto;
	}

	.sync-badge.offline {
		color: var(--danger);
		background: var(--danger-bg);
	}

	nav {
		display: flex;
		align-items: baseline;
		gap: 1rem;
		font-size: 0.875rem;
	}

	nav a,
	.link {
		color: var(--text-2);
		text-decoration: none;
		background: none;
		border: none;
		padding: 0;
		font: inherit;
		cursor: pointer;
	}

	nav a:hover,
	.link:hover {
		color: var(--accent-tx);
	}

	nav a.active {
		color: var(--text);
		font-weight: 600;
	}

	main {
		flex: 1;
		width: 100%;
		max-width: 46rem;
		margin: 0 auto;
		padding: 1.5rem 1.25rem 4rem;
	}

	.muted {
		color: var(--text-3);
	}

	@media (max-width: 640px) {
		.topbar {
			position: relative;
			align-items: center;
			gap: 0.625rem;
			padding: 0.625rem 1rem;
		}

		.menu-toggle {
			display: flex;
		}

		/* The nav lives behind the hamburger: a drawer dropping from the bar,
		   one full-width tap target per item. */
		nav {
			display: none;
		}

		nav.open {
			display: flex;
			flex-direction: column;
			align-items: stretch;
			gap: 0;
			position: absolute;
			top: 100%;
			left: 0;
			right: 0;
			background: var(--bg-toolbar);
			border-bottom: 1px solid var(--border);
			box-shadow: var(--shadow);
			padding: 0.25rem 1rem 0.5rem;
			font-size: 1rem;
			z-index: 40;
		}

		nav.open a,
		nav.open .link {
			padding: 0.75rem 0.25rem;
			text-align: left;
		}

		main {
			padding: 1rem 1rem 3rem;
		}

		/* Study screens: the app becomes a fixed-height column so the session
		   can pin its controls to the bottom and scroll the card instead. */
		.app.study {
			height: 100vh;
			height: 100dvh;
			overflow: hidden;
		}

		main.study {
			display: flex;
			flex-direction: column;
			min-height: 0;
			overflow: hidden;
			padding-bottom: 0.75rem;
		}
	}
</style>
