<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { theme } from '$lib/stores/theme.svelte';
	import { auth } from '$lib/stores/auth.svelte';
	import { api } from '$lib/api';

	let { children } = $props();

	let checking = $state(true);

	// Routes reachable without a session.
	const publicRoutes = ['/login', '/setup'];

	onMount(async () => {
		theme.init();
		await auth.refresh();

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

		{#if auth.signedIn}
			<nav>
				<a href="/" class:active={page.url.pathname === '/'}>Decks</a>
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
		min-height: 100vh;
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
</style>
