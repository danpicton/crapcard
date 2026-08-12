<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, Rating, type StudyCard } from '$lib/api';
	import { auth } from '$lib/stores/auth.svelte';
	import Editor from '$lib/components/Editor.svelte';

	/**
	 * The landing screen: the next card that needs reviewing, from whichever
	 * deck was studied most recently. Signing in should put you to work rather
	 * than in front of a menu; the deck list is one click away.
	 */

	let card = $state<StudyCard | null>(null);
	let revealed = $state(false);
	let loading = $state(true);
	let submitting = $state(false);
	let reviewed = $state(0);
	let error = $state<string | null>(null);

	const answers = [
		{ key: 'again', label: 'Again', rating: Rating.Again },
		{ key: 'hard', label: 'Hard', rating: Rating.Hard },
		{ key: 'good', label: 'Good', rating: Rating.Good },
		{ key: 'easy', label: 'Easy', rating: Rating.Easy },
	] as const;

	async function load() {
		loading = true;
		try {
			card = await api.nextCardAnywhere();
			revealed = false;
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not load a card';
		} finally {
			loading = false;
		}
	}

	async function answer(rating: number) {
		// Grading a card the user has not looked at would feed the scheduler a
		// meaningless signal, and a double tap must not grade twice.
		if (!card || !revealed || submitting) return;

		submitting = true;
		try {
			await api.answerCard(card.card_id, rating);
			reviewed += 1;
			error = null;
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not save your answer';
		} finally {
			submitting = false;
		}
	}

	function onKeydown(event: KeyboardEvent) {
		const target = event.target as HTMLElement | null;
		if (target?.isContentEditable || target?.tagName === 'INPUT') return;
		if (!card) return;

		if (!revealed) {
			if (event.key === ' ' || event.key === 'Enter') {
				event.preventDefault();
				revealed = true;
			}
			return;
		}
		if (['1', '2', '3', '4'].includes(event.key)) {
			event.preventDefault();
			void answer(Number(event.key));
		} else if (event.key === ' ' || event.key === 'Enter') {
			event.preventDefault();
			void answer(Rating.Good);
		}
	}

	onMount(() => {
		if (auth.signedIn) void load();
		else loading = false;
		window.addEventListener('keydown', onKeydown);
	});

	onDestroy(() => window.removeEventListener('keydown', onKeydown));
</script>

{#if loading}
	<p class="muted">Loading…</p>
{:else if error && !card}
	<p class="error">{error}</p>
{:else if !card}
	<div class="done">
		<h1>Nothing due</h1>
		<p class="muted">
			You are up to date across every deck.
			{#if reviewed > 0}
				You reviewed {reviewed}
				{reviewed === 1 ? 'card' : 'cards'}.
			{/if}
		</p>
		<a class="primary button" href="/decks">Go to your decks</a>
	</div>
{:else}
	<div class="head">
		<p class="context muted small">
			Studying <a href="/decks/{card.deck_id}">{card.deck_name}</a>
		</p>
		<p class="counts">
			<span class="pill new">{card.counts.new} new</span>
			<span class="pill">{card.counts.learning} learning</span>
			<span class="pill">{card.counts.due} due</span>
		</p>
	</div>

	{#if error}
		<p class="error">{error}</p>
	{/if}

	<!-- Keyed on the card: Milkdown reads its value at creation, so without
	     this the next card would keep showing the previous one's text. -->
	{#key card.card_id}
		<article class="card-face">
			<Editor value={card.question} readonly />
		</article>

		{#if revealed}
			<hr />
			<article class="card-face answer">
				<Editor value={card.answer} readonly />
			</article>
		{/if}
	{/key}

	{#if revealed}
		<div class="answers">
			{#each answers as a (a.key)}
				<button
					type="button"
					class="answer-button"
					disabled={submitting}
					onclick={() => answer(a.rating)}
				>
					<span class="answer-label">{a.label}</span>
					<span class="answer-interval">{card.previews[a.key]?.label ?? ''}</span>
				</button>
			{/each}
		</div>
		<p class="hint muted small">Press 1–4 to grade, or space for Good.</p>
	{:else}
		<button type="button" class="primary reveal" onclick={() => (revealed = true)}>
			Show answer
		</button>
		<p class="hint muted small">Press space to reveal.</p>
	{/if}
{/if}

<style>
	.head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.25rem;
	}

	.context a {
		color: var(--text-2);
		font-weight: 600;
	}

	.counts {
		display: flex;
		gap: 0.375rem;
		margin: 0;
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

	.card-face {
		background: var(--bg-alt);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 1.5rem;
		font-size: 1.125rem;
	}

	.card-face.answer {
		background: var(--bg);
	}

	hr {
		border: none;
		border-top: 1px solid var(--border);
		margin: 1rem 0;
	}

	.answers {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.5rem;
		margin-top: 1.5rem;
	}

	.answer-button {
		font: inherit;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.125rem;
		padding: 0.625rem 0.5rem;
		border: 1px solid var(--border-md);
		border-radius: 4px;
		background: var(--bg-alt);
		color: var(--text);
		cursor: pointer;
	}

	.answer-button:hover:not(:disabled) {
		background: var(--bg-hover);
		border-color: var(--accent);
	}

	.answer-button:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.answer-label {
		font-size: 0.875rem;
		font-weight: 600;
	}

	.answer-interval {
		font-family: var(--mono);
		font-size: 0.75rem;
		color: var(--text-3);
	}

	.reveal {
		width: 100%;
		margin-top: 1.5rem;
		padding: 0.75rem;
	}

	.primary,
	.button {
		font: inherit;
		border: 1px solid var(--accent);
		border-radius: 4px;
		background: var(--accent);
		color: #fff;
		cursor: pointer;
		text-decoration: none;
		display: inline-block;
		padding: 0.5rem 1rem;
	}

	.primary:hover,
	.button:hover {
		background: var(--accent-dk);
	}

	.done {
		text-align: center;
		padding: 3rem 0;
	}

	.done h1 {
		font-family: var(--serif);
		margin: 0 0 0.5rem;
	}

	.hint {
		text-align: center;
		margin-top: 0.75rem;
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
		.answers {
			grid-template-columns: repeat(2, 1fr);
		}
	}
</style>
