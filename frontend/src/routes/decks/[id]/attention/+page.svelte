<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Deck, type AttentionCard } from '$lib/api';
	import { summariseMarkdown } from '$lib/noteSummary';
	import { templateLabel } from '$lib/cloze';
	import FlagIcon from '$lib/components/FlagIcon.svelte';
	import PauseIcon from '$lib/components/PauseIcon.svelte';
	import SpadeIcon from '$lib/components/SpadeIcon.svelte';
	import CardPreviewModal from '$lib/components/CardPreviewModal.svelte';

	/**
	 * The deck's flagged and suspended cards, reasons and all — the only
	 * place reasons are ever shown. Each row offers to lift whatever the
	 * card is under: the flag, the suspension, a burial.
	 */

	const deckId = Number(page.params.id);

	let deck = $state<Deck | null>(null);
	let cards = $state<AttentionCard[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let previewNoteId = $state<number | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			const [loadedDeck, attention] = await Promise.all([
				api.getDeck(deckId),
				api.attentionCards(deckId),
			]);
			deck = loadedDeck;
			cards = attention.cards;
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not load the cards';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function buriedNow(c: AttentionCard): boolean {
		return c.buried_until !== null && new Date(c.buried_until).getTime() > Date.now();
	}

	/** Run one card action against the server and refresh the list. */
	async function act(fn: () => Promise<unknown>, failure: string) {
		try {
			await fn();
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : failure;
		}
	}
</script>

<a class="back" href="/decks/{deckId}">← Back to deck</a>

{#if loading}
	<p class="muted">Loading…</p>
{:else if deck}
	<div class="head">
		<h1>
			Attention
			<span class="muted deck-name">· {deck.name}</span>
		</h1>
	</div>

	{#if error}
		<p class="error">{error}</p>
	{/if}

	{#if cards.length === 0}
		<p class="muted">No flagged or suspended cards.</p>
	{:else}
		<ul class="cards">
			{#each cards as c (c.card_id)}
				<li class="card">
					<div class="card-text">
						<p class="question">{summariseMarkdown(c.question)}</p>
						{#if c.flagged}
							<p class="reason flag-reason">
								<FlagIcon />
								{c.flag_reason || 'Flagged'}
							</p>
						{/if}
						{#if c.suspended}
							<p class="reason suspend-reason">
								<PauseIcon />
								{c.suspend_reason || 'Suspended'}
							</p>
						{/if}
					</div>
					<div class="card-side">
						<span class="meta muted small">
							{templateLabel(c.template)}
							{#if buriedNow(c)}<SpadeIcon />{/if}
						</span>
						<span class="actions">
							<button type="button" class="link" onclick={() => (previewNoteId = c.note_id)}>
								Preview
							</button>
							{#if c.flagged}
								<button
									type="button"
									class="link"
									onclick={() =>
										act(() => api.flagCard(c.card_id, false), 'could not unflag the card')}
								>
									Unflag
								</button>
							{/if}
							{#if c.suspended}
								<button
									type="button"
									class="link"
									onclick={() =>
										act(() => api.suspendCard(c.card_id, false), 'could not resume the card')}
								>
									Resume
								</button>
							{:else}
								<button
									type="button"
									class="link"
									onclick={() =>
										act(() => api.suspendCard(c.card_id, true), 'could not suspend the card')}
								>
									Suspend
								</button>
							{/if}
							{#if buriedNow(c)}
								<button
									type="button"
									class="link"
									onclick={() =>
										act(() => api.buryCard(c.card_id, 0), 'could not unbury the card')}
								>
									Unbury
								</button>
							{/if}
						</span>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
{:else if error}
	<p class="error">{error}</p>
{/if}

{#if previewNoteId !== null}
	<CardPreviewModal noteId={previewNoteId} onclose={() => (previewNoteId = null)} />
{/if}

<style>
	.back {
		font-size: 0.8125rem;
		color: var(--text-3);
		text-decoration: none;
	}

	.head {
		margin: 0.75rem 0 1.5rem;
	}

	h1 {
		font-family: var(--serif);
		font-size: 1.75rem;
		margin: 0;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.deck-name {
		font-size: 1.125rem;
		font-weight: 400;
	}

	.cards {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.card {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg-alt);
	}

	.card-text {
		min-width: 0;
	}

	.question {
		margin: 0;
		font-weight: 500;
	}

	/* Reasons read as margin notes: the user talking to themselves. */
	.reason {
		display: flex;
		align-items: baseline;
		gap: 0.375rem;
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: var(--text-2);
		padding-left: 0.5rem;
		white-space: pre-wrap;
	}

	.flag-reason {
		border-left: 2px solid #d0912b;
	}

	.suspend-reason {
		border-left: 2px solid var(--border-md);
	}

	.card-side {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.375rem;
		flex-shrink: 0;
	}

	.meta {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
	}

	.actions {
		display: inline-flex;
		align-items: center;
		gap: 0.625rem;
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

	.link:hover {
		color: var(--accent-tx);
	}

	.muted {
		color: var(--text-3);
	}

	.small {
		font-size: 0.8125rem;
	}

	.error {
		color: var(--danger);
		background: var(--danger-bg);
		padding: 0.5rem 0.75rem;
		border-radius: 4px;
	}

	@media (max-width: 640px) {
		.back {
			display: inline-block;
			padding: 0.5rem 0.75rem 0.5rem 0;
		}

		h1 {
			flex-wrap: wrap;
		}

		/* Card rows stack: question and reasons on top, meta and actions in a
		   row below with finger-sized links. */
		.card {
			flex-direction: column;
			gap: 0.5rem;
		}

		.card-side {
			flex-direction: row;
			align-items: center;
			justify-content: space-between;
			width: 100%;
		}

		.actions {
			flex-wrap: wrap;
			gap: 0.25rem;
		}

		.actions .link {
			padding: 0.5rem 0.375rem;
			font-size: 0.875rem;
		}
	}
</style>
