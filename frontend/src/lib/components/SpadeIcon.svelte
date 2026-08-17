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
		<!-- Blade, shaft, D-handle: Lucide's shovel (ISC licence). -->
		<path d="M2 22v-5l5-5 5 5-5 5z" />
		<path d="M9.5 14.5 16 8" />
		<path d="m17 2 5 5-.5.5a3.53 3.53 0 0 1-5 0s0 0 0 0a3.53 3.53 0 0 1 0-5L17 2" />
	</svg>
</span>

<style>
	.icon {
		display: inline-flex;
		align-items: center;
		color: var(--text-3);
	}
</style>
