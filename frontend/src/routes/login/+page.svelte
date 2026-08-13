<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/stores/auth.svelte';

	let username = $state('');
	let password = $state('');
	let error = $state<string | null>(null);
	let busy = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		busy = true;
		error = null;
		try {
			await auth.login(username, password);
			await goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not sign in';
		} finally {
			busy = false;
		}
	}
</script>

<form class="auth" onsubmit={submit}>
	<h1>Sign in</h1>

	{#if error}<p class="error">{error}</p>{/if}

	<label>
		Username
		<input bind:value={username} autocomplete="username" required />
	</label>
	<label>
		Password
		<input type="password" bind:value={password} autocomplete="current-password" required />
	</label>

	<button type="submit" class="primary" disabled={busy}>
		{busy ? 'Signing in…' : 'Sign in'}
	</button>
</form>

<style>
	.auth {
		max-width: 20rem;
		margin: 3rem auto;
		display: flex;
		flex-direction: column;
		gap: 0.875rem;
	}

	h1 {
		font-family: var(--serif);
		font-size: 1.5rem;
		margin: 0;
	}

	label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.8125rem;
		color: var(--text-2);
	}

	input {
		font: inherit;
		padding: 0.5rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg);
		color: var(--text);
	}

	.primary {
		font: inherit;
		padding: 0.5rem 1rem;
		border: 1px solid var(--accent);
		border-radius: 4px;
		background: var(--accent);
		color: #fff;
		cursor: pointer;
	}

	.primary:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.error {
		color: var(--danger);
		background: var(--danger-bg);
		padding: 0.5rem 0.75rem;
		border-radius: 4px;
		margin: 0;
	}
</style>
