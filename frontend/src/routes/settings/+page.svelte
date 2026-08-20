<script lang="ts">
	import { theme, THEMES, type ThemeId } from '$lib/stores/theme.svelte';
	import { prefs } from '$lib/stores/prefs.svelte';
	import { pageSizeOptionsFor } from '$lib/noteSummary';
	import { auth } from '$lib/stores/auth.svelte';
</script>

<h1>Settings</h1>

{#if auth.user}
	<p class="muted small">Signed in as {auth.user.username}.</p>
{/if}

<section>
	<h2>Theme</h2>

	<label class="setting">
		<span class="setting-label">Theme</span>
		<select
			value={theme.current}
			onchange={(e) => theme.set((e.target as HTMLSelectElement).value as ThemeId)}
		>
			{#each THEMES as option (option.id)}
				<option value={option.id}>{option.label}</option>
			{/each}
		</select>
	</label>
</section>

<section>
	<h2>Card lists</h2>

	<label class="setting">
		<span class="setting-label">Cards per page</span>
		<select
			value={prefs.isOverridden ? prefs.pageSize : 'default'}
			onchange={(e) => {
				const value = (e.target as HTMLSelectElement).value;
				if (value === 'default') prefs.clearPageSize();
				else prefs.setPageSize(Number(value));
			}}
		>
			<option value="default">Server default ({prefs.deploymentPageSize})</option>
			{#each pageSizeOptionsFor(prefs.pageSize) as size (size)}
				<option value={size}>{size}</option>
			{/each}
		</select>
	</label>
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

	.setting {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 0.875rem;
	}

	.setting-label {
		font-size: 0.8125rem;
		color: var(--text-2);
		min-width: 8rem;
	}

	select {
		font: inherit;
		font-size: 0.875rem;
		padding: 0.35rem 0.6rem;
		border: 1px solid var(--border-md);
		border-radius: 4px;
		background: var(--bg-alt);
		color: var(--text);
		cursor: pointer;
		min-width: 14rem;
	}

	select:hover {
		background: var(--bg-hover);
	}

	.muted {
		color: var(--text-3);
	}

	.small {
		font-size: 0.8125rem;
	}

	@media (max-width: 640px) {
		.setting {
			flex-direction: column;
			align-items: stretch;
			gap: 0.375rem;
		}

		.setting-label {
			min-width: 0;
		}

		select {
			min-width: 0;
			width: 100%;
			padding: 0.55rem 0.6rem;
		}
	}
</style>
