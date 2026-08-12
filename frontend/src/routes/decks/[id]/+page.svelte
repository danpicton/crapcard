<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Deck, type Note, type QueueCounts } from '$lib/api';
	import { prefs } from '$lib/stores/prefs.svelte';
	import { summariseMarkdown, pageSizeOptionsFor } from '$lib/noteSummary';
	import Editor from '$lib/components/Editor.svelte';
	import BidirectionalIcon from '$lib/components/BidirectionalIcon.svelte';
	import CardPreviewModal from '$lib/components/CardPreviewModal.svelte';

	const deckId = Number(page.params.id);

	let deck = $state<Deck | null>(null);
	let notes = $state<Note[]>([]);
	let total = $state(0);
	let offset = $state(0);
	let counts = $state<QueueCounts | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	// Which note the preview modal is showing, if any.
	let previewNoteId = $state<number | null>(null);

	// The page size in force: the deck header's selector overrides the user's
	// setting, which overrides the deployment default.
	let pageSizeOverride = $state<number | null>(null);
	const pageSize = $derived(pageSizeOverride ?? prefs.pageSize);
	const pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)));
	const currentPage = $derived(Math.floor(offset / pageSize) + 1);
	const pageSizeOptions = $derived(pageSizeOptionsFor(pageSize));

	// Composer state. `editingId` is null when authoring a new note.
	let composing = $state(false);
	let editingId = $state<number | null>(null);
	let front = $state('');
	let back = $state('');
	let reversed = $state(false);
	let saving = $state(false);

	async function load() {
		loading = true;
		error = null;
		try {
			const [loadedDeck, page, loadedCounts] = await Promise.all([
				api.getDeck(deckId),
				api.listNotes({ deckId, limit: pageSize, offset }),
				api.deckCounts(deckId).catch(() => null),
			]);
			deck = loadedDeck;
			notes = page.items;
			total = page.total;
			counts = loadedCounts;

			// A deletion can empty the last page; step back rather than
			// showing an empty list under a pager that says there is more.
			if (notes.length === 0 && offset > 0) {
				offset = Math.max(0, offset - pageSize);
				await load();
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not load the deck';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	function goToPage(next: number) {
		const bounded = Math.min(Math.max(next, 1), pageCount);
		offset = (bounded - 1) * pageSize;
		void load();
	}

	function setPageSize(size: number) {
		pageSizeOverride = size;
		offset = 0;
		void load();
	}

	/** "3 days ago", or null when the note has never been answered. */
	function lastStudiedLabel(value: string | null): string | null {
		if (!value) return null;
		const then = new Date(value);
		if (Number.isNaN(then.getTime())) return null;

		const seconds = Math.max(0, (Date.now() - then.getTime()) / 1000);
		if (seconds < 60) return 'just now';
		if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
		if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
		const days = Math.floor(seconds / 86400);
		if (days < 30) return `${days}d ago`;
		if (days < 365) return `${Math.floor(days / 30)}mo ago`;
		return `${Math.floor(days / 365)}y ago`;
	}

	function startNew() {
		editingId = null;
		front = '';
		back = '';
		reversed = false;
		composing = true;
	}

	function startEdit(note: Note) {
		editingId = note.id;
		front = note.fields.front ?? '';
		back = note.fields.back ?? '';
		reversed = note.reversed;
		composing = true;
	}

	function cancel() {
		composing = false;
		editingId = null;
	}

	async function save() {
		if (!front.trim() || !back.trim()) {
			error = 'Both sides are required.';
			return;
		}

		saving = true;
		error = null;
		try {
			const fields = { front, back };
			if (editingId === null) {
				await api.createNote({ deck_id: deckId, type: 'basic', reversed, fields });
				// The list is newest first, so a new card lands on page one.
				// Staying put would save it out of sight.
				offset = 0;
			} else {
				await api.updateNote(editingId, { deck_id: deckId, reversed, fields });
			}
			composing = false;
			editingId = null;
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not save the card';
		} finally {
			saving = false;
		}
	}

	async function remove(note: Note) {
		if (!confirm('Delete this card and its review history?')) return;
		try {
			await api.deleteNote(note.id);
			await load();
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not delete the card';
		}
	}

</script>

<a class="back" href="/decks">← All decks</a>

{#if loading}
	<p class="muted">Loading…</p>
{:else if deck}
	<div class="head">
		<div>
			<h1>{deck.name}</h1>
			{#if deck.description}<p class="muted small">{deck.description}</p>{/if}
		</div>
		<div class="head-actions">
			{#if counts && counts.total > 0}
				<a class="primary button" href="/decks/{deckId}/study">Study {counts.total}</a>
			{/if}
			<button type="button" class="secondary" onclick={startNew}>Add card</button>
		</div>
	</div>

	{#if error}
		<p class="error">{error}</p>
	{/if}

	{#if composing}
		<section class="composer">
			<h2>{editingId === null ? 'New card' : 'Edit card'}</h2>

			<label class="field">
				<span>Front</span>
				<div class="editor-shell">
					{#key editingId ?? 'new'}
						<Editor
							bind:value={front}
							placeholder="Front of the card — paste an image straight in"
							onerror={(m) => (error = m)}
						/>
					{/key}
				</div>
			</label>

			<label class="field">
				<span>Back</span>
				<div class="editor-shell">
					{#key editingId ?? 'new'}
						<Editor
							bind:value={back}
							placeholder="Back of the card"
							onerror={(m) => (error = m)}
						/>
					{/key}
				</div>
			</label>

			<label class="checkbox">
				<input type="checkbox" bind:checked={reversed} />
				<span class="checkbox-label">
					<BidirectionalIcon />
					Bidirectional
					<small class="muted">Adds a second card asking the other way, scheduled independently.</small>
				</span>
			</label>

			<div class="composer-actions">
				<button type="button" class="primary" disabled={saving} onclick={save}>
					{saving ? 'Saving…' : 'Save'}
				</button>
				{#if editingId !== null}
					<button type="button" class="secondary" onclick={() => (previewNoteId = editingId)}>
						Preview
					</button>
				{/if}
				<button type="button" class="link" onclick={cancel}>Cancel</button>
			</div>
			{#if editingId === null}
				<p class="muted small hint">Save the card to preview how it will be asked.</p>
			{/if}
		</section>
	{/if}

	<div class="list-head">
		<h2 class="list-heading">
			{total}
			{total === 1 ? 'card' : 'cards'}
			{#if total > pageSize}
				<span class="muted small">
					· showing {offset + 1}–{Math.min(offset + notes.length, total)}
				</span>
			{/if}
		</h2>

		<label class="page-size muted small">
			Per page
			<select
				value={pageSize}
				onchange={(e) => setPageSize(Number((e.target as HTMLSelectElement).value))}
			>
				{#each pageSizeOptions as size (size)}
					<option value={size}>{size}</option>
				{/each}
			</select>
		</label>
	</div>

	{#if notes.length === 0}
		<p class="muted">No cards yet.</p>
	{:else}
		<ul class="notes">
			{#each notes as note (note.id)}
				<li class="note">
					<div class="note-text">
						<p class="note-front">{summariseMarkdown(note.fields.front ?? '')}</p>
						<p class="note-back muted small">{summariseMarkdown(note.fields.back ?? '')}</p>
						<p class="note-meta muted small">
							{#if lastStudiedLabel(note.last_studied)}
								Studied {lastStudiedLabel(note.last_studied)}
							{:else}
								Never studied
							{/if}
						</p>
					</div>
					<div class="note-actions">
						{#if note.reversed}<BidirectionalIcon />{/if}
						<button type="button" class="link" onclick={() => (previewNoteId = note.id)}>
							Preview
						</button>
						<button type="button" class="link" onclick={() => startEdit(note)}>Edit</button>
						<button type="button" class="link danger" onclick={() => remove(note)}>
							Delete
						</button>
					</div>
				</li>
			{/each}
		</ul>

		{#if pageCount > 1}
			<nav class="pager" aria-label="Card list pages">
				<button
					type="button"
					class="secondary"
					disabled={currentPage <= 1}
					onclick={() => goToPage(currentPage - 1)}
				>
					← Previous
				</button>
				<span class="muted small">Page {currentPage} of {pageCount}</span>
				<button
					type="button"
					class="secondary"
					disabled={currentPage >= pageCount}
					onclick={() => goToPage(currentPage + 1)}
				>
					Next →
				</button>
			</nav>
		{/if}
	{/if}
{/if}

{#if previewNoteId !== null}
	<CardPreviewModal noteId={previewNoteId} onclose={() => (previewNoteId = null)} />
{/if}

<style>
	.back {
		font-size: 0.8125rem;
		color: var(--text-3);
		text-decoration: none;
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		margin: 0.75rem 0 1.5rem;
	}

	h1 {
		font-family: var(--serif);
		font-size: 1.75rem;
		margin: 0;
	}

	h2 {
		font-family: var(--serif);
		font-size: 1.125rem;
		margin: 0 0 0.75rem;
	}

	.list-heading {
		color: var(--text-2);
		margin: 0;
	}

	.head-actions {
		display: flex;
		gap: 0.5rem;
	}

	.composer {
		background: var(--bg-alt);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 1rem;
	}

	.field {
		display: block;
		margin-bottom: 1rem;
	}

	.field > span {
		display: block;
		font-size: 0.8125rem;
		color: var(--text-2);
		margin-bottom: 0.25rem;
	}

	.editor-shell {
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg);
	}

	.checkbox {
		display: flex;
		align-items: flex-start;
		gap: 0.5rem;
		font-size: 0.875rem;
		margin-bottom: 1rem;
	}

	.checkbox small {
		display: block;
		font-size: 0.75rem;
	}

	.composer-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.notes {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.note {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg-alt);
	}

	.list-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		margin-top: 2rem;
	}

	.page-size {
		display: flex;
		align-items: center;
		gap: 0.375rem;
	}

	.page-size select {
		font: inherit;
		font-size: 0.8125rem;
		padding: 0.2rem 0.4rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg);
		color: var(--text);
	}

	.pager {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		margin-top: 1.25rem;
	}

	.pager button:disabled {
		opacity: 0.45;
		cursor: default;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 0.375rem;
	}

	.hint {
		margin: 0.5rem 0 0;
	}

	.note-meta {
		margin: 0.375rem 0 0;
		font-size: 0.75rem;
	}

	.note-front {
		margin: 0;
		font-weight: 500;
	}

	.note-back {
		margin: 0.125rem 0 0;
	}

	.note-actions {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		flex-shrink: 0;
	}

	.primary,
	.button,
	.secondary {
		font: inherit;
		font-size: 0.875rem;
		padding: 0.4rem 0.9rem;
		border-radius: 4px;
		cursor: pointer;
		text-decoration: none;
		display: inline-block;
		border: 1px solid var(--accent);
		background: var(--accent);
		color: #fff;
	}

	.secondary {
		background: var(--bg);
		color: var(--text);
		border-color: var(--border-md);
	}

	.primary:hover,
	.button:hover {
		background: var(--accent-dk);
	}

	.secondary:hover {
		background: var(--bg-hover);
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

	.danger:hover {
		color: var(--danger);
	}

	.muted {
		color: var(--text-3);
	}

	.small {
		font-size: 0.8125rem;
	}

	.error {
		color: var(--danger);
		background: var(--danger-bg);
		padding: 0.5rem 0.75rem;
		border-radius: 4px;
	}
</style>
