<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Editor, rootCtx, defaultValueCtx, editorViewOptionsCtx } from '@milkdown/kit/core';
	import { commonmark } from '@milkdown/kit/preset/commonmark';
	import { gfm } from '@milkdown/kit/preset/gfm';
	import { history } from '@milkdown/kit/plugin/history';
	import { listener, listenerCtx } from '@milkdown/kit/plugin/listener';
	import { createImagePastePlugin } from '$lib/milkdown/imagePlugin';

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

	/* Placeholder for an empty first paragraph. */
	.editor :global(.crapcard-prose[data-placeholder] p.is-empty:first-child::before) {
		content: attr(data-placeholder);
		color: var(--text-3);
		float: left;
		height: 0;
		pointer-events: none;
	}
</style>
