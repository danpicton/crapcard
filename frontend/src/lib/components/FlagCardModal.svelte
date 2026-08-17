<script lang="ts">
	/**
	 * The one dialog behind both "flag this card" and "suspend this card".
	 *
	 * The two actions are siblings: a flag can suspend in the same breath, a
	 * suspension can leave a flag explaining itself. One component keeps the
	 * pairing symmetrical, and keeps the reason box — the note to your future
	 * self, read back in the flagged-cards view — in a single place.
	 */
	interface Props {
		/** Which action the user reached for; the other rides along optionally. */
		mode: 'flag' | 'suspend';
		onconfirm: (choice: { flag: boolean; reason: string; suspend: boolean }) => void;
		onclose: () => void;
	}

	let { mode, onconfirm, onclose }: Props = $props();

	let reason = $state('');
	// In flag mode the extra is suspension; in suspend mode it is the flag.
	let alsoOther = $state(false);
	let modalEl = $state<HTMLElement | null>(null);
	let reasonEl = $state<HTMLTextAreaElement | null>(null);

	$effect(() => {
		(reasonEl ?? modalEl)?.focus();
	});

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') onclose();
	}

	// The reason belongs to the action the dialog is for; the "also" action
	// rides along without one.
	function confirm(event: SubmitEvent) {
		event.preventDefault();
		onconfirm({
			flag: mode === 'flag' || alsoOther,
			reason: reason.trim(),
			suspend: mode === 'suspend' || alsoOther,
		});
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
		aria-label={mode === 'flag' ? 'Flag card' : 'Suspend card'}
		bind:this={modalEl}
		onclick={(e) => e.stopPropagation()}
	>
		<header>
			<h2>{mode === 'flag' ? 'Flag card' : 'Suspend card'}</h2>
			<button type="button" class="link" onclick={onclose} aria-label="Close">✕</button>
		</header>

		<form onsubmit={confirm}>
			<label class="field">
				<span>Reason</span>
				<textarea
					bind:this={reasonEl}
					bind:value={reason}
					rows="3"
					maxlength="2000"
					placeholder="optional"
				></textarea>
			</label>

			<label class="checkbox">
				<input type="checkbox" bind:checked={alsoOther} />
				{mode === 'flag' ? 'Also suspend' : 'Also flag'}
			</label>

			<div class="actions">
				<button type="submit" class="primary">
					{#if mode === 'flag'}
						{alsoOther ? 'Flag & suspend' : 'Flag'}
					{:else}
						{alsoOther ? 'Suspend & flag' : 'Suspend'}
					{/if}
				</button>
				<button type="button" class="link" onclick={onclose}>Cancel</button>
			</div>
		</form>
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
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 1rem 1.25rem 1.25rem;
		width: min(28rem, 100%);
	}

	header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: 0.75rem;
	}

	h2 {
		font-family: var(--serif);
		font-size: 1.125rem;
		margin: 0;
	}

	form {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.8125rem;
		color: var(--text-2);
	}

	textarea {
		font: inherit;
		font-size: 0.875rem;
		padding: 0.5rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg);
		color: var(--text);
		resize: vertical;
	}

	.checkbox {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 0.25rem;
	}

	.primary {
		font: inherit;
		font-size: 0.875rem;
		padding: 0.4rem 0.9rem;
		border: 1px solid var(--accent);
		border-radius: 4px;
		background: var(--accent);
		color: #fff;
		cursor: pointer;
	}

	.primary:hover {
		background: var(--accent-dk);
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

	@media (max-width: 640px) {
		.backdrop {
			padding: 1rem 0.75rem;
		}

		header .link {
			padding: 0.5rem 0.625rem;
			margin: -0.5rem -0.625rem;
		}

		.primary {
			padding: 0.55rem 1.1rem;
		}

		.actions .link {
			padding: 0.5rem;
		}
	}
</style>
