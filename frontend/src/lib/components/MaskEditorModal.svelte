<script lang="ts">
	import type { Occlusion, OcclusionRect } from '$lib/cloze';
	import { nextMaskId, rectFromDrag } from '$lib/maskEditing';

	/**
	 * Draw masks over an image to cloze it: drag rectangles over the areas to
	 * test, one card per mask. The note's occlusion config is handed back on
	 * save; geometry lives in maskEditing.ts where it is tested.
	 */
	interface Props {
		/** The image being masked, without any #occ= fragment. */
		src: string;
		/** The note's current masks, or null when there are none yet. */
		occlusion: Occlusion | null;
		onsave: (occlusion: Occlusion) => void;
		onclose: () => void;
	}

	let { src, occlusion, onsave, onclose }: Props = $props();

	// The modal deliberately snapshots the note's masks on open — edits stay
	// local until Save hands them back.
	// svelte-ignore state_referenced_locally
	let rects = $state<OcclusionRect[]>([...(occlusion?.rects ?? [])]);
	// svelte-ignore state_referenced_locally
	let mode = $state<'hide-one' | 'hide-all'>(occlusion?.mode === 'hide-all' ? 'hide-all' : 'hide-one');

	// Ids only ever go up, even past deleted masks, so a new mask never
	// inherits a deleted mask's card and review history.
	// svelte-ignore state_referenced_locally
	let highestId = nextMaskId(rects) - 1;

	let modalEl = $state<HTMLElement | null>(null);
	let imgEl = $state<HTMLImageElement | null>(null);

	// The rectangle being dragged out right now, in pixels relative to the image.
	let drag = $state<{ start: { x: number; y: number }; now: { x: number; y: number } } | null>(
		null,
	);

	$effect(() => {
		modalEl?.focus();
	});

	function pointOf(event: PointerEvent): { x: number; y: number } | null {
		if (!imgEl) return null;
		const box = imgEl.getBoundingClientRect();
		return { x: event.clientX - box.left, y: event.clientY - box.top };
	}

	function startDrag(event: PointerEvent) {
		const p = pointOf(event);
		if (!p) return;
		event.preventDefault();
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		drag = { start: p, now: p };
	}

	function moveDrag(event: PointerEvent) {
		if (!drag) return;
		const p = pointOf(event);
		if (p) drag = { ...drag, now: p };
	}

	function endDrag() {
		if (!drag || !imgEl) {
			drag = null;
			return;
		}
		const rect = rectFromDrag(drag.start, drag.now, imgEl.getBoundingClientRect(), highestId + 1);
		drag = null;
		if (rect) {
			highestId = rect.id;
			rects = [...rects, rect];
		}
	}

	function remove(id: number) {
		rects = rects.filter((r) => r.id !== id);
	}

	/** The in-progress drag as percentage styles, drawn like a finished mask. */
	const dragStyle = $derived.by(() => {
		if (!drag || !imgEl) return null;
		const box = imgEl.getBoundingClientRect();
		if (box.width <= 0 || box.height <= 0) return null;
		const x = Math.min(drag.start.x, drag.now.x) / box.width;
		const y = Math.min(drag.start.y, drag.now.y) / box.height;
		const w = Math.abs(drag.now.x - drag.start.x) / box.width;
		const h = Math.abs(drag.now.y - drag.start.y) / box.height;
		return rectStyle({ id: 0, x, y, w, h });
	});

	function rectStyle(r: OcclusionRect): string {
		return `left:${r.x * 100}%;top:${r.y * 100}%;width:${r.w * 100}%;height:${r.h * 100}%`;
	}

	function onKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') onclose();
	}

	function save() {
		onsave({ mode, rects });
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
		aria-label="Mask areas of this image"
		bind:this={modalEl}
		onclick={(e) => e.stopPropagation()}
	>
		<header>
			<h2>Mask the image</h2>
			<button type="button" class="link" onclick={onclose} aria-label="Close without saving">
				✕
			</button>
		</header>

		<p class="muted small">
			Drag over the areas to test. Each mask becomes its own card.
		</p>

		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div
			class="canvas"
			onpointerdown={startDrag}
			onpointermove={moveDrag}
			onpointerup={endDrag}
			onpointercancel={() => (drag = null)}
		>
			<img bind:this={imgEl} {src} alt="Being masked" draggable="false" />
			{#each rects as rect (rect.id)}
				<span class="mask" style={rectStyle(rect)}>
					<button
						type="button"
						class="mask-delete"
						aria-label="Delete this mask"
						onpointerdown={(e) => e.stopPropagation()}
						onclick={() => remove(rect.id)}
					>
						✕
					</button>
				</span>
			{/each}
			{#if dragStyle}
				<span class="mask drawing" style={dragStyle}></span>
			{/if}
		</div>

		<fieldset class="mode">
			<legend class="small muted">When a card is asked</legend>
			<label>
				<input type="radio" name="occlusion-mode" value="hide-one" bind:group={mode} />
				<span>
					Hide one
					<small class="muted">Only the tested area is covered.</small>
				</span>
			</label>
			<label>
				<input type="radio" name="occlusion-mode" value="hide-all" bind:group={mode} />
				<span>
					Hide all
					<small class="muted">
						Every mask is covered, the tested one marked — for images where neighbouring labels
						give the answer away.
					</small>
				</span>
			</label>
		</fieldset>

		<div class="actions">
			<button type="button" class="primary" onclick={save}>
				{rects.length === 0 ? 'Remove all masks' : `Save ${rects.length} ${rects.length === 1 ? 'mask' : 'masks'}`}
			</button>
			<button type="button" class="link" onclick={onclose}>Cancel</button>
		</div>
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
		max-width: 42rem;
		padding: 1.25rem;
	}

	header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		margin-bottom: 0.25rem;
	}

	h2 {
		font-family: var(--serif);
		font-size: 1.25rem;
		margin: 0;
	}

	.canvas {
		position: relative;
		display: inline-block;
		max-width: 100%;
		line-height: 0;
		cursor: crosshair;
		touch-action: none;
		user-select: none;
	}

	.canvas img {
		max-width: 100%;
		height: auto;
		border-radius: 4px;
		display: block;
	}

	.mask {
		position: absolute;
		box-sizing: border-box;
		background: color-mix(in srgb, var(--accent) 70%, transparent);
		border: 1px solid var(--accent);
		border-radius: 3px;
	}

	.mask.drawing {
		background: color-mix(in srgb, var(--accent) 35%, transparent);
		border-style: dashed;
	}

	.mask-delete {
		position: absolute;
		top: -8px;
		right: -8px;
		width: 18px;
		height: 18px;
		padding: 0;
		font-size: 10px;
		line-height: 1;
		border: 1px solid var(--border-md);
		border-radius: 50%;
		background: var(--bg);
		color: var(--text-2);
		cursor: pointer;
	}

	.mask-delete:hover {
		color: var(--danger);
		border-color: var(--danger);
	}

	.mode {
		border: none;
		margin: 1rem 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.mode legend {
		padding: 0;
		margin-bottom: 0.375rem;
	}

	.mode label {
		display: flex;
		gap: 0.5rem;
		align-items: flex-start;
		font-size: 0.875rem;
		cursor: pointer;
	}

	.mode small {
		display: block;
		font-size: 0.75rem;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1.25rem;
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

	.muted {
		color: var(--text-3);
	}

	.small {
		font-size: 0.8125rem;
	}
</style>
