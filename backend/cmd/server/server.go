package main

import (
	"net/http"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/cards"
	"github.com/danpicton/crapcard/internal/db"
	"github.com/danpicton/crapcard/internal/decks"
	"github.com/danpicton/crapcard/internal/httpx"
	"github.com/danpicton/crapcard/internal/images"
	"github.com/danpicton/crapcard/internal/notes"
	"github.com/danpicton/crapcard/internal/srs"
	"github.com/danpicton/crapcard/internal/study"
)

// defaultPageSize is how many cards a deck listing shows per page when the
// deployment does not set one.
const defaultPageSize = 50

// Config holds runtime settings that are not the database itself.
type Config struct {
	// PageSize is the deployment-wide default page size for card listings.
	// A user's own preference, and the per-deck selector, override it in the
	// client; this is only the starting point. Zero means defaultPageSize.
	PageSize int
	// TrustProxy honours X-Forwarded-* headers. Only enable it when a
	// reverse proxy you control strips inbound copies of them.
	TrustProxy bool
	// Images bounds image storage. The zero value means the defaults.
	Images images.Config
}

// newServer wires the whole application onto one mux and returns it.
//
// Routing is deliberately explicit rather than looped: this list is the
// authoritative statement of what the API exposes and what guards it, and a
// test walks it asserting every data route is behind RequireAuth.
func newServer(database *db.DB, cfg Config) (http.Handler, error) {
	if cfg.Images.MaxImageSize == 0 {
		cfg.Images = images.DefaultConfig()
	}
	if cfg.PageSize <= 0 {
		cfg.PageSize = defaultPageSize
	}

	authSvc := auth.NewService(auth.NewUserRepo(database), auth.NewSessionRepo(database))
	authHandler := auth.NewHandler(authSvc)
	requireAuth := authHandler.RequireAuth

	noteRepo := notes.NewRepository(database)
	cardRepo := cards.NewRepository(database)

	mux := http.NewServeMux()

	// Public: liveness and the first-run setup handshake.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /api/auth/setup-status", authHandler.SetupStatus)
	mux.HandleFunc("POST /api/auth/setup", authHandler.Setup)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/auth/logout", authHandler.Logout)

	// Authenticated.
	mux.Handle("GET /api/auth/me", requireAuth(http.HandlerFunc(authHandler.Me)))

	// Deployment settings the client needs to know about.
	mux.Handle("GET /api/config", requireAuth(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, map[string]any{
				"page_size":     cfg.PageSize,
				"max_page_size": notes.MaxListLimit,
			})
		})))

	decks.NewHandler(decks.NewRepository(database)).Register(mux, requireAuth)
	notes.NewHandler(noteRepo, cardRepo).Register(mux, requireAuth)
	study.NewHandler(study.NewService(noteRepo, cardRepo, decks.NewRepository(database), srs.DefaultParams())).Register(mux, requireAuth)
	images.NewHandlerWith(database, cfg.Images).Register(mux, requireAuth)

	// Anything under /api that matched no route is a JSON 404. Without this
	// it would fall through to the SPA shell and a typo in a fetch would
	// surface as HTML being parsed as JSON.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(w, http.StatusNotFound, "no such endpoint")
	})

	// The single-page app, and its client-side routes.
	mux.Handle("/", uiHandler())

	var handler http.Handler = mux
	handler = limitBodies(handler)
	handler = securityHeaders(handler)
	if cfg.TrustProxy {
		handler = httpx.TrustProxy(handler)
	}
	return handler, nil
}

// maxJSONBody caps every request body except image uploads. Card fields are
// markdown a person typed; a megabyte of it is not a card, it is a payload.
const maxJSONBody = 1 << 20

// limitBodies stops a request body from buffering unbounded memory. The
// unauthenticated login and setup endpoints would otherwise decode an
// arbitrarily large JSON string from anyone. Image uploads are exempt — they
// enforce their own, larger limit.
func limitBodies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isUpload := r.Method == http.MethodPost && r.URL.Path == "/api/images"
		if r.Body != nil && !isUpload {
			r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders applies the headers that apply to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
