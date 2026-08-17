<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { createSession } from '$lib/stores/session.svelte';
	import { sync } from '$lib/stores/sync.svelte';
	import { Rating } from '$lib/api';
	import Editor from '$lib/components/Editor.svelte';
	import FlagIcon from '$lib/components/FlagIcon.svelte';
	import FlagCardModal from '$lib/components/FlagCardModal.svelte';

	/**
	 * The whole review loop as one component, shared by the landing screen
	 * (deckId null: next card from anywhere) and the per-deck study screen.
	 * One implementation means the two cannot drift apart.
	 */
	interface Props {
		/** Deck to study, or null for "whatever is due next anywhere". */
		deckId: number | null;
	}

	let { deckId }: Props = $props();

	// The session deliberately captures deckId at mount: a different deck is
	// a different session, and the routes remount this component per deck.
	// svelte-ignore state_referenced_locally
	const session = createSession(deckId);

	// Answer buttons in rating order, with the key their preview arrives under.
	const answers = [
		{ key: 'again', label: 'Again', rating: Rating.Again },
		{ key: 'hard', label: 'Hard', rating: Rating.Hard },
		{ key: 'good', label: 'Good', rating: Rating.Good },
		{ key: 'easy', label: 'Easy', rating: Rating.Easy },
	] as const;

	// When the answer was revealed. Space doubles as "grade Good", and
	// without a beat between reveal and grade, a held key or nervous double
	// tap grades a card the user never actually read.
	let revealedAt = 0;
	const GRADE_COOLDOWN_MS = 300;

	function reveal() {
		session.reveal();
		revealedAt = Date.now();
	}

	// Which card-action dialog is open, if any. Null while studying.
	let cardModal = $state<'flag' | 'suspend' | null>(null);

	function onCardAction(choice: { flag: boolean; reason: string; suspend: boolean }) {
		cardModal = null;
		if (choice.flag) {
			void session.setFlag(true, choice.reason, choice.suspend);
		} else if (choice.suspend) {
			void session.suspend(null);
		}
	}

	/** Bury for a stretch the user picks — "not tomorrow, but soon". */
	function buryMore() {
		const raw = prompt('Bury for how many days?', '3');
		if (raw === null) return;
		const days = Number(raw);
		if (!Number.isInteger(days) || days < 1 || days > 365) return;
		void session.bury(days);
	}

	onMount(() => {
		void session.start();
		window.addEventListener('keydown', onKeydown);
	});

	// The moment the connection returns, replay the queued answers and pick
	// the session back up without the user having to do anything.
	$effect(() => {
		if (sync.online && session.stalled && !session.submitting) {
			void session.resume();
		}
	});

	onDestroy(() => {
		window.removeEventListener('keydown', onKeydown);
	});

	/**
	 * Keyboard review: space reveals, then 1–4 grade (or space again for
	 * Good), and U takes the last answer back. This is how the app is
	 * actually usable for a long session — reaching for the mouse on every
	 * card is what makes reviewing feel like work.
	 */
	function onKeydown(event: KeyboardEvent) {
		// A dialog owns the keyboard while it is open.
		if (cardModal !== null) return;
		const target = event.target as HTMLElement | null;
		if (target?.isContentEditable || target?.tagName === 'INPUT' || target?.tagName === 'TEXTAREA')
			return;
		// A held key auto-repeats; each press must be deliberate.
		if (event.repeat) return;

		if (event.key === 'u' || event.key === 'U') {
			event.preventDefault();
			void session.undo();
			return;
		}
		if (!session.card) return;

		if (!session.revealed) {
			if (event.key === ' ' || event.key === 'Enter') {
				event.preventDefault();
				reveal();
			}
			return;
		}

		if (['1', '2', '3', '4'].includes(event.key)) {
			event.preventDefault();
			void session.answer(Number(event.key));
		} else if (event.key === ' ' || event.key === 'Enter') {
			// Space does double duty: reveal, then accept the common answer —
			// but not so fast that the reveal press and the grade press blur
			// into one.
			event.preventDefault();
			if (Date.now() - revealedAt >= GRADE_COOLDOWN_MS) {
				void session.answer(Rating.Good);
			}
		}
	}
</script>

{#if !session.finished && (session.counts || session.remaining > 0)}
	<div class="session-head">
		{#if deckId === null && session.deckName !== null}
			<p class="context muted small">
				Studying <a href="/decks/{session.deckId}">{session.deckName}</a>
			</p>
		{/if}
		<p class="counts">
			{#if sync.online && session.counts}
				<span class="pill new">{session.counts.new} new</span>
				<span class="pill">{session.counts.learning} learning</span>
				<span class="pill">{session.counts.due} due</span>
			{:else}
				<!-- Server counts go stale offline; the local queue does not. -->
				<span class="pill">{session.remaining} left</span>
			{/if}
			{#if session.reviewed > 0}
				<span class="muted small">{session.reviewed} reviewed</span>
			{/if}
		</p>
	</div>
{/if}

{#if session.error}
	<p class="error">{session.error}</p>
{/if}

{#if session.stalled}
	<div class="offline-note">
		<p>
			<strong>You're offline</strong> and no cards are cached for this session yet. Studying
			resumes as soon as the connection returns.
		</p>
		<button type="button" class="secondary" onclick={() => session.resume()}> Try now </button>
	</div>
{:else if session.loading && !session.card}
	<p class="muted">Loading…</p>
{:else if session.finished}
	<div class="done">
		<h1>{deckId === null ? 'Nothing due' : 'All done'}</h1>
		<p class="muted">
			{deckId === null
				? 'You are up to date across every deck.'
				: 'Nothing else is due in this deck right now.'}
			{#if session.reviewed > 0}
				You reviewed {session.reviewed}
				{session.reviewed === 1 ? 'card' : 'cards'}.
			{/if}
		</p>
		{#if sync.pendingAnswers > 0}
			<p class="muted small">
				{sync.pendingAnswers}
				{sync.pendingAnswers === 1 ? 'answer' : 'answers'} will sync when the connection returns.
			</p>
		{/if}
		<div class="done-actions">
			<a class="primary button" href="/decks">Go to your decks</a>
			{#if session.canUndo}
				<button type="button" class="link" onclick={() => session.undo()}>
					Undo last answer
				</button>
			{/if}
		</div>
	</div>
{:else if session.card}
	<!-- Housekeeping actions for the card in front of you: annotate it or set
	     it aside without grading it. Deliberately quiet — grading is the job,
	     these are the exceptions. -->
	<div class="card-tools">
		{#if session.card.flagged}
			<FlagIcon />
			<button
				type="button"
				class="link"
				disabled={session.submitting}
				onclick={() => session.setFlag(false, '')}
			>
				Unflag
			</button>
		{:else}
			<button
				type="button"
				class="link"
				disabled={session.submitting}
				onclick={() => (cardModal = 'flag')}
			>
				Flag
			</button>
		{/if}
		<button
			type="button"
			class="link"
			disabled={session.submitting}
			onclick={() => (cardModal = 'suspend')}
		>
			Suspend
		</button>
		<button
			type="button"
			class="link"
			disabled={session.submitting}
			onclick={() => session.bury(1)}
			title="Hide this card until tomorrow"
		>
			Bury
		</button>
		<button
			type="button"
			class="link"
			disabled={session.submitting}
			onclick={buryMore}
			title="Hide this card for a number of days"
		>
			Bury…
		</button>
	</div>

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
					title="{answer.label} ({answer.rating}{answer.key === 'good' ? ' or space' : ''})"
				>
					<span class="answer-label">{answer.label}</span>
					<span class="answer-interval">
						<!-- A regraded card's previews were computed from state it
						     no longer has; better no label than a wrong one. -->
						{session.card?.stale_previews ? '' : (session.card?.previews[answer.key]?.label ?? '')}
					</span>
				</button>
			{/each}
		</div>
		{#if session.canUndo}
			<div class="under-answers">
				<button
					type="button"
					class="link undo"
					disabled={session.submitting}
					onclick={() => session.undo()}
					title="Undo (U)"
				>
					Undo
				</button>
			</div>
		{/if}
	{:else}
		<button type="button" class="primary reveal" onclick={reveal} title="Show answer (space)">
			Show answer
		</button>
		{#if session.canUndo}
			<div class="under-answers">
				<button
					type="button"
					class="link undo"
					disabled={session.submitting}
					onclick={() => session.undo()}
					title="Undo (U)"
				>
					Undo
				</button>
			</div>
		{/if}
	{/if}
{/if}

{#if cardModal !== null}
	<FlagCardModal mode={cardModal} onconfirm={onCardAction} onclose={() => (cardModal = null)} />
{/if}

<style>
	.card-tools {
		display: flex;
		align-items: center;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-bottom: 0.375rem;
	}

	.session-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.context {
		margin: 0;
	}

	.context a {
		color: var(--text-2);
		font-weight: 600;
	}

	.counts {
		display: flex;
		align-items: center;
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

	.under-answers {
		display: flex;
		justify-content: flex-end;
		margin-top: 0.5rem;
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

	/* Grade colours follow spaced-repetition convention (Anki's) so the four
	   buttons can be told apart at a glance mid-session. Fixed hues rather
	   than theme variables: the themes carry no hard/good/easy palette, and
	   these read on every one of them, light or dark. */
	.answer-button {
		--grade: var(--text-2);
		font: inherit;
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.125rem;
		padding: 0.625rem 0.5rem;
		border: 1px solid var(--border-md);
		border-bottom: 2px solid var(--grade);
		border-radius: 4px;
		background: var(--bg-alt);
		color: var(--text);
		cursor: pointer;
	}

	.answer-button.again {
		--grade: #d9564a;
	}

	.answer-button.hard {
		--grade: #d0912b;
	}

	.answer-button.good {
		--grade: #4a9d5b;
	}

	.answer-button.easy {
		--grade: #4a8fd9;
	}

	.answer-button:hover:not(:disabled) {
		background: var(--bg-hover);
		border-color: var(--grade);
	}

	.answer-button:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.answer-label {
		font-size: 0.875rem;
		font-weight: 600;
		color: var(--grade);
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

	.done-actions {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
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

	.link:hover:not(:disabled) {
		color: var(--accent-tx);
	}

	.link:disabled {
		opacity: 0.5;
		cursor: default;
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

	.offline-note {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		border: 1px solid var(--border-md);
		border-radius: 6px;
		background: var(--bg-alt);
		padding: 1rem;
		margin: 1.5rem 0;
	}

	.offline-note p {
		margin: 0;
		color: var(--text-2);
	}

	.secondary {
		font: inherit;
		font-size: 0.875rem;
		padding: 0.4rem 0.9rem;
		border: 1px solid var(--border-md);
		border-radius: 4px;
		background: var(--bg);
		color: var(--text);
		cursor: pointer;
		flex-shrink: 0;
	}

	.secondary:hover {
		background: var(--bg-hover);
	}

	@media (max-width: 640px) {
		.answers {
			grid-template-columns: repeat(2, 1fr);
		}
	}
</style>
