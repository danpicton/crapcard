package main

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
)

// uiFS holds the built SvelteKit app, copied into ui/build by `make frontend`
// before the production build. In a development build the directory holds
// only .gitkeep, and uiHandler serves a placeholder instead.
//
//go:embed all:ui/build
var uiFS embed.FS

// uiHandler serves the single-page app: static assets where they exist, and
// index.html for every other path so client-side routing works on a hard
// refresh or a shared link.
func uiHandler() http.Handler {
	build, err := fs.Sub(uiFS, "ui/build")
	if err != nil {
		slog.Error("embedded ui", "err", err)
		return placeholderHandler()
	}
	if _, err := fs.Stat(build, "index.html"); err != nil {
		return placeholderHandler()
	}

	files := http.FileServer(http.FS(build))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(build, path); err != nil {
			// Unknown path: hand it to the SPA rather than 404ing, so a deep
			// link like /decks/3/study loads.
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

// placeholderHandler stands in when the frontend has not been built into the
// binary, so `make run` still serves something explanatory.
func placeholderHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8">
<title>crapcard</title>
<p>The API is running. The frontend is not embedded in this build —
run <code>make build-prod</code>, or <code>npm run dev</code> in
<code>frontend/</code> for the dev server.</p>`))
	})
}
