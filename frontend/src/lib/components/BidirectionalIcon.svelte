<script lang="ts">
	/**
	 * Marks a note that generates a card in each direction. Used on the note
	 * list, where a label would crowd the row.
	 */
	interface Props {
		/** Shown as a tooltip and to assistive technology. */
		title?: string;
		size?: number;
	}

	let { title = 'Bidirectional', size = 14 }: Props = $props();
</script>

<span class="icon" {title} role="img" aria-label={title}>
	<svg
		width={size}
		height={size}
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2.25"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<!-- Two arrows, one each way. -->
		<path d="M4 9h16" />
		<path d="M16 5l4 4" />
		<path d="M20 15H4" />
		<path d="M8 19l-4-4" />
	</svg>
</span>

<style>
	.icon {
		display: inline-flex;
		align-items: center;
		color: var(--accent-tx);
	}
</style>
