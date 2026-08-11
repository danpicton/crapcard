<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Deck, type Note, type QueueCounts } from '$lib/api';
	import Editor from '$lib/components/Editor.svelte';

	const deckId = Number(page.params.id);

	let deck = $state<Deck | null>(null);
	let notes = $state<Note[]>([]);
	let counts = $state<QueueCounts | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

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
			[deck, notes, counts] = await Promise.all([
				api.getDeck(deckId),
				api.listNotes(deckId),
				api.deckCounts(deckId).catch(() => null),
			]);
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not load the deck';
		} finally {
			loading = false;
		}
	}

	onMount(load);

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

	/** First line of a field, for the list summary. Markdown is left as-is. */
	function summarise(value: string): string {
		const line = value.split('\n').find((l) => l.trim() !== '') ?? '';
		return line.length > 80 ? `${line.slice(0, 80)}…` : line;
	}
</script>

<a class="back" href="/">← All decks</a>

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
				<span>
					Also test back → front
					<small class="muted">Adds a second card, scheduled independently.</small>
				</span>
			</label>

			<div class="composer-actions">
				<button type="button" class="primary" disabled={saving} onclick={save}>
					{saving ? 'Saving…' : 'Save'}
				</button>
				<button type="button" class="link" onclick={cancel}>Cancel</button>
			</div>
		</section>
	{/if}

	<h2 class="list-heading">{notes.length} {notes.length === 1 ? 'card' : 'cards'}</h2>

	{#if notes.length === 0}
		<p class="muted">No cards yet.</p>
	{:else}
		<ul class="notes">
			{#each notes as note (note.id)}
				<li class="note">
					<div class="note-text">
						<p class="note-front">{summarise(note.fields.front ?? '')}</p>
						<p class="note-back muted small">{summarise(note.fields.back ?? '')}</p>
					</div>
					<div class="note-actions">
						{#if note.reversed}<span class="pill">both ways</span>{/if}
						<button type="button" class="link" onclick={() => startEdit(note)}>Edit</button>
						<button type="button" class="link danger" onclick={() => remove(note)}>
							Delete
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
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
		margin-top: 2rem;
		color: var(--text-2);
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

	.pill {
		font-size: 0.6875rem;
		padding: 0.125rem 0.4rem;
		border-radius: 999px;
		background: var(--bg-hover);
		color: var(--text-3);
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
