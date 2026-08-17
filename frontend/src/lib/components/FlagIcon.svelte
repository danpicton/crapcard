<script lang="ts">
	/**
	 * Marks a flagged card. Fixed hue rather than a theme variable, like the
	 * grade buttons: the themes carry no flag colour, and this reads on all
	 * of them, light or dark.
	 */
	interface Props {
		/** Shown as a tooltip and to assistive technology. */
		title?: string;
		size?: number;
	}

	let { title = 'Flagged', size = 14 }: Props = $props();
</script>

<span class="icon" {title} role="img" aria-label={title}>
	<svg
		width={size}
		height={size}
		viewBox="0 0 24 24"
		fill="currentColor"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z" />
		<line x1="4" y1="22" x2="4" y2="15" />
	</svg>
</span>

<style>
	.icon {
		display: inline-flex;
		align-items: center;
		color: #d0912b;
	}
</style>
