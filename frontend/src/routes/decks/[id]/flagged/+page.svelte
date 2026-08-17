<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Deck, type FlaggedCard } from '$lib/api';
	import { summariseMarkdown } from '$lib/noteSummary';
	import { templateLabel } from '$lib/cloze';
	import FlagIcon from '$lib/components/FlagIcon.svelte';
	import PauseIcon from '$lib/components/PauseIcon.svelte';
	import SpadeIcon from '$lib/components/SpadeIcon.svelte';
	import CardPreviewModal from '$lib/components/CardPreviewModal.svelte';

	/**
	 * The deck's flagged cards, reasons and all — the only place a flag's
	 * reason is ever shown. Each row offers to lift whatever the card is
	 * under: the flag itself, a suspension, a burial.
	 */

	const deckId = Number(page.params.id);

	let deck = $state<Deck | null>(null);
	let cards = $state<FlaggedCard[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let previewNoteId = $state<number | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			const [loadedDeck, flagged] = await Promise.all([
				api.getDeck(deckId),
				api.flaggedCards(deckId),
			]);
			deck = loadedDeck;
			cards = flagged.cards;
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not load the flagged cards';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function buriedNow(c: FlaggedCard): boolean {
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
			<FlagIcon size={18} title="" />
			Flagged cards
			<span class="muted deck-name">· {deck.name}</span>
		</h1>
	</div>

	{#if error}
		<p class="error">{error}</p>
	{/if}

	{#if cards.length === 0}
		<p class="muted">
			Nothing is flagged in this deck. Flag a card while studying — or from the deck's card
			list — to park it here with a note to your future self.
		</p>
	{:else}
		<ul class="flagged">
			{#each cards as c (c.card_id)}
				<li class="card">
					<div class="card-text">
						<p class="question">{summariseMarkdown(c.question)}</p>
						{#if c.reason}
							<p class="reason">{c.reason}</p>
						{:else}
							<p class="reason muted">No reason given.</p>
						{/if}
					</div>
					<div class="card-side">
						<span class="meta muted small">
							{templateLabel(c.template)}
							{#if c.suspended}<PauseIcon />{/if}
							{#if buriedNow(c)}<SpadeIcon />{/if}
						</span>
						<span class="actions">
							<button type="button" class="link" onclick={() => (previewNoteId = c.note_id)}>
								Preview
							</button>
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
							<button
								type="button"
								class="link"
								title="Remove the flag (and its reason)"
								onclick={() =>
									act(() => api.flagCard(c.card_id, false), 'could not unflag the card')}
							>
								Unflag
							</button>
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

	.flagged {
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

	/* The reason reads as a margin note: the user talking to themselves. */
	.reason {
		margin: 0.25rem 0 0;
		font-size: 0.8125rem;
		color: var(--text-2);
		border-left: 2px solid #d0912b;
		padding-left: 0.5rem;
		white-space: pre-wrap;
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
</style>
