<script lang="ts">
	import { api, type CardPreview } from '$lib/api';
	import Editor from '$lib/components/Editor.svelte';
	import BidirectionalIcon from '$lib/components/BidirectionalIcon.svelte';

	/**
	 * Shows every card a note produces, exactly as review will ask them.
	 *
	 * The rendering comes from the server's own generator rather than being
	 * re-derived here, so a preview cannot drift from the real thing. Images
	 * are replaced by a note of their alt text, which makes an undescribed
	 * image obvious.
	 */
	interface Props {
		noteId: number;
		onclose: () => void;
	}

	let { noteId, onclose }: Props = $props();

	let cards = $state<CardPreview[] | null>(null);
	let error = $state<string | null>(null);

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

	function label(template: string): string {
		if (template === 'forward') return 'Front → back';
		if (template === 'reverse') return 'Back → front';
		return template;
	}
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
				<p class="muted small note-kind">
					<BidirectionalIcon />
					This note makes {cards.length} cards.
				</p>
			{/if}

			{#each cards as card (card.template)}
				<section class="preview-card">
					<h3>{label(card.template)}</h3>

					<div class="face">
						<span class="face-label">Asks</span>
						<Editor value={card.question} readonly />
					</div>
					<div class="face">
						<span class="face-label">Answer</span>
						<Editor value={card.answer} readonly />
					</div>
				</section>
			{/each}
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
		gap: 0.375rem;
		margin: 0 0 1rem;
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
</style>
