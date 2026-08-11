// The whole app is a static bundle served by the Go binary, so there is no
// server-side rendering: every page runs in the browser against the API.
export const ssr = false;
export const prerender = false;
