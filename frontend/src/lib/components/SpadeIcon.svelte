<script lang="ts">
	/**
	 * Marks a buried card: a digging spade. The tooltip carries when it
	 * comes back.
	 */
	interface Props {
		/** Shown as a tooltip and to assistive technology. */
		title?: string;
		size?: number;
	}

	let { title = 'Buried', size = 14 }: Props = $props();
</script>

<span class="icon" {title} role="img" aria-label={title}>
	<svg
		width={size}
		height={size}
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<!-- Handle, shaft, and blade of a digging spade. -->
		<path d="M9 2h6" />
		<path d="M12 2v9" />
		<path d="M7 13c0-1.5 2-2 5-2s5 0.5 5 2v4a5 5 0 0 1-10 0z" />
	</svg>
</span>

<style>
	.icon {
		display: inline-flex;
		align-items: center;
		color: var(--text-3);
	}
</style>
