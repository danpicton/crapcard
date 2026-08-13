<script lang="ts">
	import { auth } from '$lib/stores/auth.svelte';
	import StudySession from '$lib/components/StudySession.svelte';

	/**
	 * The landing screen: the next card that needs reviewing, from whichever
	 * deck was studied most recently. Signing in should put you to work rather
	 * than in front of a menu; the deck list is one click away.
	 */
</script>

{#if auth.signedIn}
	<StudySession deckId={null} />
{/if}
