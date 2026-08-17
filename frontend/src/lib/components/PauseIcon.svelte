<script lang="ts">
	/**
	 * Marks a suspended card: paused, not gone.
	 */
	interface Props {
		/** Shown as a tooltip and to assistive technology. */
		title?: string;
		size?: number;
	}

	let { title = 'Suspended', size = 14 }: Props = $props();
</script>

<span class="icon" {title} role="img" aria-label={title}>
	<svg
		width={size}
		height={size}
		viewBox="0 0 24 24"
		fill="currentColor"
		stroke="none"
		aria-hidden="true"
	>
		<rect x="6" y="4" width="4" height="16" rx="1" />
		<rect x="14" y="4" width="4" height="16" rx="1" />
	</svg>
</span>

<style>
	.icon {
		display: inline-flex;
		align-items: center;
		color: var(--text-3);
	}
</style>
