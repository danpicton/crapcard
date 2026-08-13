<script lang="ts">
	import { page } from '$app/state';
	import StudySession from '$lib/components/StudySession.svelte';

	const deckId = Number(page.params.id);
</script>

<a class="back" href="/decks/{deckId}">← Back to deck</a>

<div class="session">
	<StudySession {deckId} />
</div>

<style>
	.back {
		font-size: 0.8125rem;
		color: var(--text-3);
		text-decoration: none;
	}

	.session {
		margin-top: 0.75rem;
	}
</style>
