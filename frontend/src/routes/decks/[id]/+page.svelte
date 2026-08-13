<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { slide } from 'svelte/transition';
	import { page } from '$app/state';
	import { api, ApiError, type Deck, type Note, type Occlusion, type QueueCounts } from '$lib/api';
	import { prefs } from '$lib/stores/prefs.svelte';
	import { sync } from '$lib/stores/sync.svelte';
	import { summariseMarkdown, pageSizeOptionsFor } from '$lib/noteSummary';
	import { detectNoteType, srcWithoutOcclusion } from '$lib/cloze';
	import Editor from '$lib/components/Editor.svelte';
	import BidirectionalIcon from '$lib/components/BidirectionalIcon.svelte';
	import ClozeIcon from '$lib/components/ClozeIcon.svelte';
	import CardPreviewModal from '$lib/components/CardPreviewModal.svelte';
	import MaskEditorModal from '$lib/components/MaskEditorModal.svelte';

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
	// Masks over the front's image, when the note is an image cloze.
	let occlusion = $state<Occlusion | null>(null);
	// The image the mask editor is open on, or null when it is closed.
	let maskSrc = $state<string | null>(null);
	// The front editor, so the Cloze button can run the same command as Alt+C.
	let frontEditor = $state<ReturnType<typeof Editor> | null>(null);

	// The note's type is never picked: it follows from what the note contains,
	// re-detected on every save. Adding a {{c1::…}} or masking an image
	// converts the note; removing them converts it back.
	const noteType = $derived(detectNoteType(front, occlusion));
	// What the note's reversed flag was at the last save, so switching it
	// off can warn about the review history it would delete.
	let wasReversed = $state(false);
	let composerEl = $state<HTMLElement | null>(null);
	// Editors are keyed on this, not on editingId: an autosave that creates
	// the note mid-typing must not remount them and eat the cursor.
	let composerKey = $state(0);

	// ── Autosave ────────────────────────────────────────────────────────
	// The composer saves itself: a pause in typing is the save button.
	type SaveStatus = 'idle' | 'saving' | 'saved' | 'queued' | 'incomplete' | 'failed';
	let saveStatus = $state<SaveStatus>('idle');
	let saveTimer: ReturnType<typeof setTimeout> | null = null;
	// Content as of the last successful (or queued) save, to tell a real
	// edit from the composer merely being populated. Occlusion is compared as
	// JSON: the mask editor hands back a fresh object every save.
	let lastSaved = { front: '', back: '', reversed: false, occlusion: 'null' };
	// Outbox ref for a note first created while offline, until its real id
	// arrives from a flush.
	let pendingCreateRef = $state<string | null>(null);
	// Whether this composer session started from "Add card" — closing then
	// jumps to page one, where the new card lands.
	let startedNew = $state(false);

	const AUTOSAVE_DEBOUNCE_MS = 1200;

	const saveLabel: Record<SaveStatus, string> = {
		idle: '',
		saving: 'Saving…',
		saved: 'Saved',
		queued: 'Saved offline — will sync',
		incomplete: 'Waiting for both sides',
		failed: 'Could not save',
	};

	$effect(() => {
		// Read the fields so the effect re-runs on every edit.
		const snapshot = { front, back, reversed, occlusion: JSON.stringify(occlusion) };
		if (!composing) return;
		if (
			snapshot.front === lastSaved.front &&
			snapshot.back === lastSaved.back &&
			snapshot.reversed === lastSaved.reversed &&
			snapshot.occlusion === lastSaved.occlusion
		) {
			return;
		}
		if (saveTimer !== null) clearTimeout(saveTimer);
		saveTimer = setTimeout(() => void autosave(), AUTOSAVE_DEBOUNCE_MS);
	});

	// A note created while offline gets its real id when the outbox flushes;
	// adopt it so further edits become ordinary updates.
	$effect(() => {
		if (pendingCreateRef === null) return;
		const id = sync.createdIdFor(pendingCreateRef);
		if (id !== null) {
			editingId = id;
			pendingCreateRef = null;
		}
	});

	async function autosave() {
		if (!composing) return;
		// A cloze note is complete with just its front — the deletions (or
		// masks) are the answers, and the back is optional extra context.
		const complete =
			noteType === 'basic' ? !!front.trim() && !!back.trim() : !!front.trim();
		if (!complete) {
			saveStatus = 'incomplete';
			return;
		}

		const snapshot = { front, back, reversed, occlusion: JSON.stringify(occlusion) };
		const input = {
			deck_id: deckId,
			type: noteType,
			reversed: reversed,
			// Omitted while null so an edit that never opened the mask editor
			// cannot touch the note's masks; present — even empty — replaces
			// them.
			...(occlusion !== null ? { occlusion } : {}),
			fields: { front: snapshot.front, back: snapshot.back },
		};
		saveStatus = 'saving';
		try {
			if (pendingCreateRef !== null) {
				// Still waiting offline for the create to flush: refresh it.
				sync.queueNoteCreate(pendingCreateRef, input);
				saveStatus = 'queued';
			} else if (editingId === null) {
				const note = await api.createNote(input);
				editingId = note.id;
				saveStatus = 'saved';
			} else {
				await api.updateNote(editingId, input);
				saveStatus = 'saved';
			}
			lastSaved = snapshot;
			wasReversed = snapshot.reversed;
		} catch (err) {
			if (err instanceof ApiError && err.status === 0) {
				// Offline: park the save and keep typing.
				sync.markOffline();
				if (editingId !== null) {
					sync.queueNoteUpdate(editingId, input);
				} else {
					pendingCreateRef = crypto.randomUUID();
					sync.queueNoteCreate(pendingCreateRef, input);
				}
				lastSaved = snapshot;
				wasReversed = snapshot.reversed;
				saveStatus = 'queued';
			} else {
				saveStatus = 'failed';
				error = err instanceof Error ? err.message : 'could not save the card';
			}
		}
	}

	/** Turning bidirectional off deletes the reverse card outright —
	 * including every review it has ever been given. Not a silent change. */
	function onReversedToggle(event: Event) {
		const box = event.target as HTMLInputElement;
		if (
			wasReversed &&
			!box.checked &&
			editingId !== null &&
			!confirm(
				'Turning off bidirectional deletes the reverse card and all of its review history. Continue?',
			)
		) {
			box.checked = true;
			reversed = true;
			return;
		}
		reversed = box.checked;
	}

	// Deck header editing.
	let editingDeck = $state(false);
	let deckName = $state('');
	let deckDescription = $state('');
	let savingDeck = $state(false);

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

	/** Bring the composer into view — "Edit" far down the list opens it at
	 * the top of the page, out of sight otherwise. */
	async function revealComposer() {
		await tick();
		composerEl?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
	}

	function startNew() {
		editingId = null;
		pendingCreateRef = null;
		front = '';
		back = '';
		reversed = false;
		wasReversed = false;
		occlusion = null;
		maskSrc = null;
		lastSaved = { front: '', back: '', reversed: false, occlusion: 'null' };
		saveStatus = 'idle';
		startedNew = true;
		composerKey += 1;
		composing = true;
		void revealComposer();
	}

	function startEdit(note: Note) {
		editingId = note.id;
		pendingCreateRef = null;
		front = note.fields.front ?? '';
		back = note.fields.back ?? '';
		reversed = note.reversed;
		wasReversed = note.reversed;
		occlusion = note.occlusion;
		maskSrc = null;
		lastSaved = { front, back, reversed, occlusion: JSON.stringify(occlusion) };
		saveStatus = 'idle';
		startedNew = false;
		composerKey += 1;
		composing = true;
		void revealComposer();
	}

	/** True for a note the list should mark with the cloze glyph. */
	function isClozeNote(note: Note): boolean {
		return note.type === 'cloze' || note.type === 'image-cloze';
	}

	/** Close the composer: flush any pending edit, then refresh the list. */
	async function done() {
		if (saveTimer !== null) {
			clearTimeout(saveTimer);
			saveTimer = null;
		}
		await autosave();
		if (saveStatus === 'failed') return; // Leave the composer open to retry.
		const created = startedNew && (editingId !== null || pendingCreateRef !== null);
		composing = false;
		editingId = null;
		pendingCreateRef = null;
		if (created) {
			// The list is newest first, so a new card lands on page one.
			offset = 0;
		}
		await load();
	}

	/** Abandon a card that never had enough content to be created. */
	function discard() {
		if (saveTimer !== null) {
			clearTimeout(saveTimer);
			saveTimer = null;
		}
		composing = false;
		editingId = null;
		pendingCreateRef = null;
	}

	function startDeckEdit() {
		if (!deck) return;
		deckName = deck.name;
		deckDescription = deck.description;
		editingDeck = true;
	}

	async function saveDeck(event: SubmitEvent) {
		event.preventDefault();
		const name = deckName.trim();
		if (!name) return;

		savingDeck = true;
		error = null;
		try {
			deck = await api.updateDeck(deckId, name, deckDescription.trim());
			editingDeck = false;
		} catch (err) {
			error = err instanceof Error ? err.message : 'could not save the deck';
		} finally {
			savingDeck = false;
		}
	}

	async function remove(note: Note) {
		const message =
			note.reversed || isClozeNote(note)
				? 'Delete this note — all of its cards and their review history?'
				: 'Delete this card and its review history?';
		if (!confirm(message)) return;
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
	{#if editingDeck}
		<form class="deck-edit" onsubmit={saveDeck}>
			<label>
				Name
				<input bind:value={deckName} required />
			</label>
			<label>
				Description
				<input bind:value={deckDescription} placeholder="optional" />
			</label>
			<div class="composer-actions">
				<button type="submit" class="primary" disabled={savingDeck}>
					{savingDeck ? 'Saving…' : 'Save'}
				</button>
				<button type="button" class="link" onclick={() => (editingDeck = false)}>Cancel</button>
			</div>
		</form>
	{:else}
		<div class="head">
			<div>
				<h1>
					{deck.name}
					<button
						type="button"
						class="link rename"
						onclick={startDeckEdit}
						aria-label="Rename this deck"
					>
						Edit
					</button>
				</h1>
				{#if deck.description}<p class="muted small">{deck.description}</p>{/if}
			</div>
			<div class="head-actions">
				{#if counts && counts.total > 0}
					<a class="primary button" href="/decks/{deckId}/study">Study {counts.total}</a>
				{/if}
				<button type="button" class="secondary" onclick={startNew}>Add card</button>
			</div>
		</div>
	{/if}

	{#if error}
		<p class="error">{error}</p>
	{/if}

	{#if composing}
		<section class="composer" bind:this={composerEl}>
			<h2>{startedNew ? 'New card' : 'Edit card'}</h2>

			<div class="field">
				<span class="field-head">
					<span id="front-label">Front</span>
					<span class="field-tools">
						{#if noteType === 'image-cloze'}
							<button
								type="button"
								class="link tool"
								onclick={() => {
									const m = front.match(/!\[[^\]]*\]\(([^)]*)\)/);
									if (m) maskSrc = srcWithoutOcclusion(m[1]);
								}}
							>
								Masks · {occlusion?.rects.length ?? 0}
							</button>
						{/if}
						<button type="button" class="link tool" onclick={() => frontEditor?.cloze()}>
							<ClozeIcon title="" /> Cloze
							<kbd>Alt+C</kbd>
						</button>
					</span>
				</span>
				<div class="editor-shell" aria-labelledby="front-label">
					{#key composerKey}
						<Editor
							bind:this={frontEditor}
							bind:value={front}
							placeholder="Front"
							onerror={(m) => (error = m)}
							onmaskrequest={(src) => (maskSrc = srcWithoutOcclusion(src))}
						/>
					{/key}
				</div>
			</div>

			<!-- A cloze note has no back: the deletions are the answers. The
			     field slides away when a deletion or mask appears, and comes
			     back — content intact — when the last one is removed. -->
			{#if noteType === 'basic'}
				<label class="field" transition:slide={{ duration: 150 }}>
					<span>Back</span>
					<div class="editor-shell">
						{#key composerKey}
							<Editor bind:value={back} placeholder="Back" onerror={(m) => (error = m)} />
						{/key}
					</div>
				</label>
			{/if}

			{#if noteType === 'basic'}
				<label class="checkbox" title="Adds a second card asking the other way">
					<input type="checkbox" checked={reversed} onchange={onReversedToggle} />
					<span class="checkbox-label">
						<BidirectionalIcon title="" />
						Bidirectional
					</span>
				</label>
			{/if}

			<div class="composer-actions">
				<button type="button" class="primary" onclick={done}>Done</button>
				{#if editingId !== null}
					<button type="button" class="secondary" onclick={() => (previewNoteId = editingId)}>
						Preview
					</button>
				{:else if pendingCreateRef === null}
					<button type="button" class="link" onclick={discard}>Discard</button>
				{/if}
				<span class="save-status muted small" role="status">{saveLabel[saveStatus]}</span>
			</div>
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
					</div>
					<p class="note-meta muted">
						{#if lastStudiedLabel(note.last_studied)}
							Studied {lastStudiedLabel(note.last_studied)}
						{:else}
							Never studied
						{/if}
					</p>
					<div class="note-actions">
						{#if isClozeNote(note)}<ClozeIcon />{/if}
						{#if note.reversed && !isClozeNote(note)}<BidirectionalIcon />{/if}
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

{#if maskSrc !== null}
	<MaskEditorModal
		src={maskSrc}
		{occlusion}
		onsave={(occ) => {
			occlusion = occ;
			maskSrc = null;
		}}
		onclose={() => (maskSrc = null)}
	/>
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

	.rename {
		vertical-align: middle;
		margin-left: 0.25rem;
	}

	.deck-edit {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		background: var(--bg-alt);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 1rem;
		margin: 0.75rem 0 1.5rem;
	}

	.deck-edit label {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.8125rem;
		color: var(--text-2);
	}

	.deck-edit input {
		font: inherit;
		padding: 0.5rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg);
		color: var(--text);
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

	/* Child selector to outweigh `.field > span`, which would otherwise keep
	   this block-level and let the tools fall in line with the label. */
	.field > .field-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
	}

	.field-tools {
		display: inline-flex;
		align-items: baseline;
		gap: 0.875rem;
	}

	.tool {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
	}

	.tool kbd {
		font-family: var(--mono);
		font-size: 0.6875rem;
		color: var(--text-3);
		border: 1px solid var(--border);
		border-radius: 3px;
		padding: 0 0.25rem;
	}

	.editor-shell {
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg);
	}

	.checkbox {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		margin-bottom: 1rem;
	}

	.composer-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.save-status {
		margin-left: auto;
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
		position: relative;
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		/* The extra bottom padding reserves the corner the studied-stamp
		   sits in, so a long back line cannot run underneath it. */
		padding: 0.75rem 0.75rem 1.375rem;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg-alt);
	}

	.list-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		margin: 2rem 0 0.875rem;
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

	.note-meta {
		position: absolute;
		right: 0.75rem;
		bottom: 0.375rem;
		margin: 0;
		font-size: 0.6875rem;
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
