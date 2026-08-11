<script lang="ts">
	import { theme, THEMES } from '$lib/stores/theme.svelte';
	import { auth } from '$lib/stores/auth.svelte';
</script>

<h1>Settings</h1>

{#if auth.user}
	<p class="muted small">Signed in as {auth.user.username}.</p>
{/if}

<section>
	<h2>Theme</h2>
	<p class="muted small">
		The same themes as crapnote. Your choice is remembered on this device.
	</p>

	<div class="themes">
		{#each THEMES as option (option.id)}
			<button
				type="button"
				class="theme"
				class:selected={theme.current === option.id}
				onclick={() => theme.set(option.id)}
			>
				{option.label}
			</button>
		{/each}
	</div>
</section>

<style>
	h1 {
		font-family: var(--serif);
		font-size: 1.75rem;
		margin: 0 0 0.25rem;
	}

	h2 {
		font-family: var(--serif);
		font-size: 1.125rem;
		margin: 0 0 0.25rem;
	}

	section {
		margin-top: 2rem;
	}

	.themes {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.875rem;
	}

	.theme {
		font: inherit;
		font-size: 0.875rem;
		padding: 0.4rem 0.9rem;
		border: 1px solid var(--border-md);
		border-radius: 4px;
		background: var(--bg-alt);
		color: var(--text);
		cursor: pointer;
	}

	.theme:hover {
		background: var(--bg-hover);
	}

	.theme.selected {
		border-color: var(--accent);
		color: var(--accent-tx);
		font-weight: 600;
	}

	.muted {
		color: var(--text-3);
	}

	.small {
		font-size: 0.8125rem;
	}
</style>
