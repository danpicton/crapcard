import { readable } from 'svelte/store';
export const page = readable({ params: {}, url: new URL('http://localhost/') });
