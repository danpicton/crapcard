package decks

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/httpx"
)

// MaxNameLength bounds a deck name so the UI can rely on it fitting.
const MaxNameLength = 200

// Handler serves the deck REST endpoints.
type Handler struct {
	repo *Repository
}

// NewHandler creates a deck Handler.
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// Middleware wraps a handler, typically with authentication.
type Middleware func(http.Handler) http.Handler

// Register mounts the deck routes on the mux, each wrapped in requireAuth.
func (h *Handler) Register(mux *http.ServeMux, requireAuth Middleware) {
	mux.Handle("GET /api/decks", requireAuth(http.HandlerFunc(h.List)))
	mux.Handle("POST /api/decks", requireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/decks/{id}", requireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PUT /api/decks/{id}", requireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /api/decks/{id}", requireAuth(http.HandlerFunc(h.Delete)))
}

// deckRequest is the shared body for create and update.
type deckRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// decodeDeckRequest reads and validates the body, writing the error response
// itself and returning ok=false when the body is unusable.
func decodeDeckRequest(w http.ResponseWriter, r *http.Request) (deckRequest, bool) {
	var req deckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name is required")
		return req, false
	}
	if len(req.Name) > MaxNameLength {
		httpx.WriteError(w, http.StatusBadRequest, "name is too long")
		return req, false
	}
	return req, true
}

// writeRepoError maps repository errors onto status codes. ErrNotFound covers
// both "no such deck" and "someone else's deck", so a probe cannot tell them
// apart.
func writeRepoError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
	case errors.Is(err, ErrDuplicateName):
		httpx.WriteError(w, http.StatusConflict, "a deck with that name already exists")
	default:
		slog.Error("deck request failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "request failed")
	}
}

// List handles GET /api/decks.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	list, err := h.repo.List(r.Context(), u.ID)
	if err != nil {
		writeRepoError(w, err)
		return
	}

	// Always an array — a null body would break .map() on the client.
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, publicDeck(d))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Create handles POST /api/decks.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	req, ok := decodeDeckRequest(w, r)
	if !ok {
		return
	}

	d, err := h.repo.Create(r.Context(), u.ID, req.Name, req.Description)
	if err != nil {
		writeRepoError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, publicDeck(d))
}

// Get handles GET /api/decks/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
		return
	}

	d, err := h.repo.Get(r.Context(), u.ID, id)
	if err != nil {
		writeRepoError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, publicDeck(d))
}

// Update handles PUT /api/decks/{id}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
		return
	}
	req, ok := decodeDeckRequest(w, r)
	if !ok {
		return
	}

	d, err := h.repo.Update(r.Context(), u.ID, id, req.Name, req.Description)
	if err != nil {
		writeRepoError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, publicDeck(d))
}

// Delete handles DELETE /api/decks/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
		return
	}

	if err := h.repo.Delete(r.Context(), u.ID, id); err != nil {
		writeRepoError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// publicDeck projects a Deck onto its JSON shape.
func publicDeck(d *Deck) map[string]any {
	return map[string]any{
		"id":          d.ID,
		"name":        d.Name,
		"description": d.Description,
		"created_at":  d.CreatedAt,
		"updated_at":  d.UpdatedAt,
	}
}
