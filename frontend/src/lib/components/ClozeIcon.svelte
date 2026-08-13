<script lang="ts">
	/**
	 * Marks a cloze note — one row in the list, several cards behind it. Same
	 * role as BidirectionalIcon: a glyph where a label would crowd the row.
	 */
	interface Props {
		/** Shown as a tooltip and to assistive technology. */
		title?: string;
		size?: number;
	}

	let { title = 'Cloze — one card per deletion', size = 14 }: Props = $props();
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
		<!-- Square brackets around a gap: the blank of a cloze. -->
		<path d="M8 5H5v14h3" />
		<path d="M16 5h3v14h-3" />
		<circle cx="12" cy="12" r="1" fill="currentColor" stroke="none" />
	</svg>
</span>

<style>
	.icon {
		display: inline-flex;
		align-items: center;
		color: var(--accent-tx);
	}
</style>
