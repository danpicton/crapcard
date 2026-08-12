<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Deck, type QueueCounts } from '$lib/api';
	import { auth } from '$lib/stores/auth.svelte';

	interface DeckRow {
		deck: Deck;
		counts: QueueCounts | null;
	}

	let rows = $state<DeckRow[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	let creating = $state(false);
	let newName = $state('');
	let newDescription = $state('');

	async function load() {
		loading = true;
		error = null;
		try {
			const decks = await api.listDecks();
			// Counts come from a second call per deck; fetched together so the
			// list does not pop in one row at a time.
			rows = await Promise.all(
				decks.map(async (deck) => ({
					deck,
					counts: await api.deckCounts(deck.id).catch(() => null),
				})),
			);
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not load your decks';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		if (auth.signedIn) void load();
	});

	async function createDeck(event: SubmitEvent) {
		event.preventDefault();
		const name = newName.trim();
		if (!name) return;

		error = null;
		try {
			await api.createDeck(name, newDescription.trim());
			newName = '';
			newDescription = '';
			creating = false;
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not create the deck';
		}
	}

	async function removeDeck(deck: Deck) {
		if (!confirm(`Delete “${deck.name}” and every card in it? This cannot be undone.`)) return;
		try {
			await api.deleteDeck(deck.id);
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not delete the deck';
		}
	}
</script>

<div class="head">
	<h1>Decks</h1>
	<button type="button" class="primary" onclick={() => (creating = !creating)}>
		{creating ? 'Cancel' : 'New deck'}
	</button>
</div>

{#if error}
	<p class="error">{error}</p>
{/if}

{#if creating}
	<form class="card new-deck" onsubmit={createDeck}>
		<label>
			Name
			<input bind:value={newName} placeholder="Italian" required />
		</label>
		<label>
			Description
			<input bind:value={newDescription} placeholder="verbs and vocab" />
		</label>
		<button type="submit" class="primary">Create</button>
	</form>
{/if}

{#if loading}
	<p class="muted">Loading…</p>
{:else if rows.length === 0}
	<p class="muted">No decks yet. Create one to start making cards.</p>
{:else}
	<ul class="decks">
		{#each rows as row (row.deck.id)}
			<li class="card">
				<div class="deck-main">
					<a class="deck-name" href="/decks/{row.deck.id}">{row.deck.name}</a>
					{#if row.deck.description}
						<p class="muted small">{row.deck.description}</p>
					{/if}
					{#if row.counts}
						<p class="counts">
							<span class="pill new">{row.counts.new} new</span>
							<span class="pill learning">{row.counts.learning} learning</span>
							<span class="pill due">{row.counts.due} due</span>
						</p>
					{/if}
				</div>

				<div class="deck-actions">
					{#if row.counts && row.counts.total > 0}
						<a class="primary button" href="/decks/{row.deck.id}/study">Study</a>
					{:else if row.counts}
						<span class="muted small">Nothing due</span>
					{:else}
						<!-- The counts call failed; claiming "nothing due" would be a lie. -->
						<span class="muted small">Counts unavailable</span>
					{/if}
					<button type="button" class="danger link" onclick={() => removeDeck(row.deck)}>
						Delete
					</button>
				</div>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 1.25rem;
	}

	h1 {
		font-family: var(--serif);
		font-size: 1.75rem;
		margin: 0;
	}

	.decks {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.card {
		background: var(--bg-alt);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 1rem;
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
	}

	.new-deck {
		flex-direction: column;
		align-items: stretch;
		margin-bottom: 1.25rem;
	}

	.new-deck label {
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

	.deck-name {
		font-family: var(--serif);
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--text);
		text-decoration: none;
	}

	.deck-name:hover {
		color: var(--accent-tx);
	}

	.counts {
		display: flex;
		gap: 0.375rem;
		margin: 0.5rem 0 0;
	}

	.pill {
		font-size: 0.75rem;
		padding: 0.125rem 0.5rem;
		border-radius: 999px;
		background: var(--bg-hover);
		color: var(--text-2);
	}

	.pill.new {
		color: var(--accent-tx);
	}

	.deck-actions {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.5rem;
	}

	.primary,
	.button {
		font: inherit;
		font-size: 0.875rem;
		padding: 0.4rem 0.9rem;
		border: 1px solid var(--accent);
		border-radius: 4px;
		background: var(--accent);
		color: #fff;
		cursor: pointer;
		text-decoration: none;
		display: inline-block;
	}

	.primary:hover,
	.button:hover {
		background: var(--accent-dk);
		border-color: var(--accent-dk);
	}

	.link {
		background: none;
		border: none;
		padding: 0;
		font: inherit;
		font-size: 0.8125rem;
		cursor: pointer;
		color: var(--text-3);
	}

	.danger:hover {
		color: var(--danger);
	}

	.muted {
		color: var(--text-3);
	}

	.small {
		font-size: 0.8125rem;
		margin: 0.25rem 0 0;
	}

	.error {
		color: var(--danger);
		background: var(--danger-bg);
		padding: 0.5rem 0.75rem;
		border-radius: 4px;
	}
</style>
