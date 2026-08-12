<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Editor, rootCtx, defaultValueCtx, editorViewOptionsCtx } from '@milkdown/kit/core';
	import { commonmark } from '@milkdown/kit/preset/commonmark';
	import { gfm } from '@milkdown/kit/preset/gfm';
	import { history } from '@milkdown/kit/plugin/history';
	import { listener, listenerCtx } from '@milkdown/kit/plugin/listener';
	import { createImagePastePlugin } from '$lib/milkdown/imagePlugin';
	import { imageView } from '$lib/milkdown/imageView';

	interface Props {
		/** Markdown content. Bindable so the parent can read what was typed. */
		value?: string;
		placeholder?: string;
		readonly?: boolean;
		/** Reported when a pasted image fails to upload. */
		onerror?: (message: string) => void;
	}

	let {
		value = $bindable(''),
		placeholder = '',
		readonly = false,
		onerror = () => {},
	}: Props = $props();

	let host = $state<HTMLDivElement | null>(null);
	let editor: Editor | null = null;

	// The value Milkdown last reported. Used to tell an edit the user made from
	// a value the parent pushed in, so the editor is not recreated on its own
	// keystrokes — which would lose the cursor on every character.
	let lastEmitted = '';

	onMount(async () => {
		if (!host) return;

		editor = await Editor.make()
			.config((ctx) => {
				ctx.set(rootCtx, host);
				ctx.set(defaultValueCtx, value);
				ctx.update(editorViewOptionsCtx, (prev) => ({
					...prev,
					editable: () => !readonly,
					attributes: {
						class: 'crapcard-prose',
						'data-placeholder': placeholder,
					},
				}));
				ctx.get(listenerCtx).markdownUpdated((_ctx, markdown) => {
					lastEmitted = markdown;
					value = markdown;
				});
			})
			.use(commonmark)
			.use(gfm)
			.use(history)
			.use(listener)
			.use(createImagePastePlugin(undefined, onerror))
			.use(imageView)
			.create();
	});

	onDestroy(() => {
		editor?.destroy();
		editor = null;
	});
</script>

<div class="editor" class:readonly bind:this={host}></div>

<style>
	.editor {
		width: 100%;
	}

	/* The typographic rules live in app.html alongside the themes, so an
	   editor and a rendered card look identical. Only layout is set here. */
	.editor :global(.crapcard-prose) {
		outline: none;
		min-height: 4.5rem;
		padding: 0.625rem 0.75rem;
	}

	.editor.readonly :global(.crapcard-prose) {
		padding: 0;
		min-height: 0;
	}

	.editor :global(.crapcard-prose p:first-child) {
		margin-top: 0;
	}

	.editor :global(.crapcard-prose p:last-child) {
		margin-bottom: 0;
	}

	/* Pasted images must not blow out the card. */
	.editor :global(.crapcard-prose img) {
		max-width: 100%;
		height: auto;
		border-radius: 4px;
	}

	/* ── Image node view: resize handle and alt text ──────────────────── */

	.editor :global(.crapcard-img) {
		position: relative;
		display: inline-block;
		line-height: 0;
		max-width: 100%;
	}

	/* The grab corner. Shown on hover so it does not clutter a card that is
	   simply being read back. */
	.editor :global(.crapcard-img-handle) {
		position: absolute;
		right: -5px;
		bottom: -5px;
		width: 14px;
		height: 14px;
		border: 2px solid var(--bg);
		border-radius: 3px;
		background: var(--accent);
		cursor: nwse-resize;
		opacity: 0;
		transition: opacity 0.12s ease;
	}

	.editor :global(.crapcard-img:hover .crapcard-img-handle),
	.editor :global(.crapcard-img:focus-within .crapcard-img-handle) {
		opacity: 1;
	}

	/* Alt text box, over the foot of the image. It stays visible while
	   focused so it can actually be typed into.
	
	   min-width rather than matching the image: a small image would otherwise
	   clip the box down to a few unusable characters, and describing a small
	   image matters just as much as describing a large one. */
	.editor :global(.crapcard-img-toolbar) {
		position: absolute;
		left: 0;
		bottom: 0;
		min-width: 15rem;
		max-width: 100vw;
		padding: 4px;
		background: color-mix(in srgb, var(--bg) 82%, transparent);
		border-bottom-left-radius: 4px;
		border-bottom-right-radius: 4px;
		opacity: 0;
		transition: opacity 0.12s ease;
		line-height: normal;
	}

	.editor :global(.crapcard-img:hover .crapcard-img-toolbar),
	.editor :global(.crapcard-img:focus-within .crapcard-img-toolbar) {
		opacity: 1;
	}

	.editor :global(.crapcard-img-alt) {
		width: 100%;
		font: inherit;
		font-size: 0.75rem;
		font-family: var(--sans);
		padding: 3px 6px;
		border: 1px solid var(--border);
		border-radius: 3px;
		background: var(--bg);
		color: var(--text);
	}

	/* An image nobody has described: the placeholder is the warning. */
	.editor :global(.crapcard-img-alt:placeholder-shown) {
		border-color: var(--accent);
	}

	.editor.readonly :global(.crapcard-img-toolbar),
	.editor.readonly :global(.crapcard-img-handle) {
		display: none;
	}

	/* Placeholder for an empty first paragraph. */
	.editor :global(.crapcard-prose[data-placeholder] p.is-empty:first-child::before) {
		content: attr(data-placeholder);
		color: var(--text-3);
		float: left;
		height: 0;
		pointer-events: none;
	}
</style>
