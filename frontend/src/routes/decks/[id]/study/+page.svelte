<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { createSession } from '$lib/stores/session.svelte';
	import { Rating } from '$lib/api';
	import Editor from '$lib/components/Editor.svelte';

	const deckId = Number(page.params.id);
	const session = createSession(deckId);

	// Answer buttons in rating order, with the key their preview arrives under.
	const answers = [
		{ key: 'again', label: 'Again', rating: Rating.Again },
		{ key: 'hard', label: 'Hard', rating: Rating.Hard },
		{ key: 'good', label: 'Good', rating: Rating.Good },
		{ key: 'easy', label: 'Easy', rating: Rating.Easy },
	] as const;

	onMount(() => {
		void session.start();
		window.addEventListener('keydown', onKeydown);
	});

	onDestroy(() => {
		window.removeEventListener('keydown', onKeydown);
	});

	/**
	 * Keyboard review: space reveals, then 1–4 grade. This is how the app is
	 * actually usable for a long session — reaching for the mouse on every
	 * card is what makes reviewing feel like work.
	 */
	function onKeydown(event: KeyboardEvent) {
		const target = event.target as HTMLElement | null;
		if (target?.isContentEditable || target?.tagName === 'INPUT') return;

		if (!session.revealed) {
			if (event.key === ' ' || event.key === 'Enter') {
				event.preventDefault();
				session.reveal();
			}
			return;
		}

		if (['1', '2', '3', '4'].includes(event.key)) {
			event.preventDefault();
			void session.answer(Number(event.key));
		} else if (event.key === ' ' || event.key === 'Enter') {
			// Space does double duty: reveal, then accept the common answer.
			event.preventDefault();
			void session.answer(Rating.Good);
		}
	}
</script>

<a class="back" href="/decks/{deckId}">← Back to deck</a>

{#if session.counts}
	<p class="counts">
		<span class="pill new">{session.counts.new} new</span>
		<span class="pill">{session.counts.learning} learning</span>
		<span class="pill">{session.counts.due} due</span>
		<span class="muted small">{session.reviewed} reviewed this sitting</span>
	</p>
{/if}

{#if session.error}
	<p class="error">{session.error}</p>
{/if}

{#if session.loading && !session.card}
	<p class="muted">Loading…</p>
{:else if session.finished}
	<div class="done">
		<h1>All done</h1>
		<p class="muted">
			Nothing else is due in this deck right now.
			{#if session.reviewed > 0}
				You reviewed {session.reviewed}
				{session.reviewed === 1 ? 'card' : 'cards'}.
			{/if}
		</p>
		<a class="primary button" href="/decks">Back to decks</a>
	</div>
{:else if session.card}
	<!-- Keyed on the card so the editor remounts with new content: Milkdown
	     takes its value at creation time and does not track prop changes. -->
	{#key session.card.card_id}
		<article class="card-face">
			<Editor value={session.card.question} readonly />
		</article>

		{#if session.revealed}
			<hr />
			<article class="card-face answer">
				<Editor value={session.card.answer} readonly />
			</article>
		{/if}
	{/key}

	{#if session.revealed}
		<div class="answers">
			{#each answers as answer (answer.key)}
				<button
					type="button"
					class="answer-button {answer.key}"
					disabled={session.submitting}
					onclick={() => session.answer(answer.rating)}
				>
					<span class="answer-label">{answer.label}</span>
					<span class="answer-interval">
						{session.card?.previews[answer.key]?.label ?? ''}
					</span>
				</button>
			{/each}
		</div>
		<p class="hint muted small">Press 1–4 to grade, or space for Good.</p>
	{:else}
		<button type="button" class="primary reveal" onclick={() => session.reveal()}>
			Show answer
		</button>
		<p class="hint muted small">Press space to reveal.</p>
	{/if}
{/if}

<style>
	.back {
		font-size: 0.8125rem;
		color: var(--text-3);
		text-decoration: none;
	}

	.counts {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		margin: 0.75rem 0 1.5rem;
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
