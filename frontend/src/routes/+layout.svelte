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

	// Routes reachable without a session.
	const publicRoutes = ['/login', '/setup'];

	onMount(async () => {
		theme.init();
		sync.init();
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

<div class="app">
	<header class="topbar">
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
			<nav>
				<a href="/" class:active={page.url.pathname === '/'}>Study</a>
				<a href="/decks" class:active={page.url.pathname.startsWith('/decks')}>Decks</a>
				<a href="/settings" class:active={page.url.pathname === '/settings'}>Settings</a>
				<button type="button" class="link" onclick={signOut}>Sign out</button>
			</nav>
		{/if}
	</header>

	<main>
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
			flex-wrap: wrap;
			gap: 0.25rem 0.75rem;
			padding: 0.625rem 1rem;
		}

		/* Room for a finger on every nav item; negative vertical margin keeps
		   the bar's visual height unchanged. */
		nav {
			gap: 0.25rem;
		}

		nav a,
		.link {
			padding: 0.5rem;
			margin: -0.375rem 0;
		}

		main {
			padding: 1rem 1rem 3rem;
		}
	}
</style>
