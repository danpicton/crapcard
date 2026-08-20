<script lang="ts">
	import { api, type CardPreview } from '$lib/api';
	import { templateLabel } from '$lib/cloze';
	import Editor from '$lib/components/Editor.svelte';
	import BidirectionalIcon from '$lib/components/BidirectionalIcon.svelte';
	import ClozeIcon from '$lib/components/ClozeIcon.svelte';

	/**
	 * Shows every card a note produces, exactly as review will ask them —
	 * images included, rendered by the same readonly editor the study screen
	 * uses.
	 *
	 * The markdown comes from the server's own generator rather than being
	 * re-derived here, so a preview cannot drift from the real thing.
	 */
	interface Props {
		noteId: number;
		onclose: () => void;
	}

	let { noteId, onclose }: Props = $props();

	let cards = $state<CardPreview[] | null>(null);
	let error = $state<string | null>(null);
	let modalEl = $state<HTMLElement | null>(null);
	// A bidirectional note shows one card at a time; the other is a click
	// away rather than doubling the modal's height.
	let activeIndex = $state(0);

	// Focus lands in the dialog on open, so Escape and screen readers see it.
	$effect(() => {
		modalEl?.focus();
	});

	$effect(() => {
		let cancelled = false;
		api
			.previewNote(noteId)
			.then((result) => {
				if (!cancelled) cards = result;
			})
			.catch((err) => {
				if (!cancelled) error = err instanceof Error ? err.message : 'could not build a preview';
			});
		return () => {
			cancelled = true;
		};
	});

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') onclose();
	}

	const label = templateLabel;

	// Which glyph explains why this note has several cards.
	const isCloze = $derived(
		(cards ?? []).some((c) => c.template.startsWith('cloze:') || c.template.startsWith('occ:')),
	);
</script>

<svelte:window onkeydown={onKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="backdrop" onclick={onclose}>
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div
		class="modal"
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-label="Card preview"
		bind:this={modalEl}
		onclick={(e) => e.stopPropagation()}
	>
		<header>
			<h2>Preview</h2>
			<button type="button" class="link" onclick={onclose} aria-label="Close preview">✕</button>
		</header>

		{#if error}
			<p class="error">{error}</p>
		{:else if cards === null}
			<p class="muted">Building the preview…</p>
		{:else}
			{#if cards.length > 1}
				<div class="card-tabs" role="tablist" aria-label="Cards of this note">
					<span class="note-kind muted small">
						{#if isCloze}<ClozeIcon />{:else}<BidirectionalIcon />{/if}
					</span>
					{#each cards as card, i (card.template)}
						<button
							type="button"
							role="tab"
							class="tab"
							class:active={i === activeIndex}
							aria-selected={i === activeIndex}
							onclick={() => (activeIndex = i)}
						>
							{label(card.template)}
						</button>
					{/each}
				</div>
			{/if}

			{#if cards[activeIndex]}
				{#key cards[activeIndex].template}
					<section class="preview-card">
						{#if cards.length === 1}
							<h3>{label(cards[activeIndex].template)}</h3>
						{/if}

						<div class="face">
							<span class="face-label">Asks</span>
							<Editor value={cards[activeIndex].question} readonly />
						</div>
						<div class="face">
							<span class="face-label">Answer</span>
							<Editor value={cards[activeIndex].answer} readonly />
						</div>
					</section>
				{/key}
			{/if}
		{/if}
	</div>
</div>

<style>
	.backdrop {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.45);
		display: flex;
		align-items: flex-start;
		justify-content: center;
		padding: 3rem 1rem;
		z-index: 50;
		overflow-y: auto;
	}

	.modal {
		background: var(--bg);
		border: 1px solid var(--border-md);
		border-radius: 6px;
		width: 100%;
		max-width: 36rem;
		padding: 1.25rem;
	}

	header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: 0.75rem;
	}

	h2 {
		font-family: var(--serif);
		font-size: 1.25rem;
		margin: 0;
	}

	h3 {
		font-size: 0.8125rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-3);
		margin: 0 0 0.5rem;
	}

	.note-kind {
		display: flex;
		align-items: center;
	}

	.card-tabs {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0 0 1rem;
	}

	.tab {
		font: inherit;
		font-size: 0.8125rem;
		padding: 0.25rem 0.75rem;
		border: 1px solid var(--border);
		border-radius: 999px;
		background: var(--bg);
		color: var(--text-2);
		cursor: pointer;
	}

	.tab:hover {
		background: var(--bg-hover);
	}

	.tab.active {
		border-color: var(--accent);
		color: var(--text);
		font-weight: 600;
	}

	.preview-card {
		margin-bottom: 1.5rem;
	}

	.face {
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg-alt);
		padding: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.face-label {
		display: block;
		font-size: 0.6875rem;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--text-3);
		margin-bottom: 0.25rem;
	}

	.link {
		background: none;
		border: none;
		padding: 0;
		font: inherit;
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
		.backdrop {
			padding: 1rem 0.75rem;
		}

		.modal {
			padding: 1rem;
		}

		header .link {
			padding: 0.5rem 0.625rem;
			margin: -0.5rem -0.625rem;
		}

		.card-tabs {
			flex-wrap: wrap;
		}

		.tab {
			padding: 0.45rem 0.875rem;
		}
	}
</style>
