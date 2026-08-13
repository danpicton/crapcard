package notes

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/cards"
	"github.com/danpicton/crapcard/internal/httpx"
)

// DefaultListLimit is the page size used when the client does not ask for one.
const DefaultListLimit = 50

// MaxListLimit is the largest page a client may request. A caller asking for
// everything must not be able to make the server materialise an unbounded
// result set.
const MaxListLimit = 500

// Handler serves the note REST endpoints.
type Handler struct {
	repo  *Repository
	cards *cards.Repository
}

// NewHandler creates a note Handler.
func NewHandler(repo *Repository, cardRepo *cards.Repository) *Handler {
	return &Handler{repo: repo, cards: cardRepo}
}

// Middleware wraps a handler, typically with authentication.
type Middleware func(http.Handler) http.Handler

// Register mounts the note routes.
func (h *Handler) Register(mux *http.ServeMux, requireAuth Middleware) {
	mux.Handle("GET /api/note-types", requireAuth(http.HandlerFunc(h.NoteTypes)))
	mux.Handle("GET /api/notes", requireAuth(http.HandlerFunc(h.List)))
	mux.Handle("POST /api/notes", requireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/notes/{id}", requireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("GET /api/notes/{id}/preview", requireAuth(http.HandlerFunc(h.Preview)))
	mux.Handle("PUT /api/notes/{id}", requireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /api/notes/{id}", requireAuth(http.HandlerFunc(h.Delete)))
}

// noteRequest is the wire shape of a note.
//
// Fields arrive as a name→markdown map rather than an array: the client
// should not have to know a type's field order, and the order it is stored in
// comes from the note type itself.
type noteRequest struct {
	DeckID int64  `json:"deck_id"`
	Type   string `json:"type"`
	// Reversed is a pointer so an update that omits the key keeps the note's
	// existing setting. Decoding it as a plain bool would read "omitted" as
	// false — and flipping reversed off deletes the reverse card along with
	// its entire review history.
	Reversed *bool `json:"reversed"`
	// Occlusion is a pointer for the same reason: an update that omits it
	// keeps the note's masks — wiping them would delete every mask card's
	// review history.
	Occlusion *Occlusion        `json:"occlusion"`
	Fields    map[string]string `json:"fields"`
}

// fieldsInTypeOrder converts the request's field map into the order the note
// type declares, so stored ordinals are stable and an unknown field is
// surfaced rather than silently dropped.
func fieldsInTypeOrder(gen Generator, in map[string]string) []Field {
	out := make([]Field, 0, len(in))
	for _, name := range gen.Fields() {
		if v, ok := in[name]; ok {
			out = append(out, Field{Name: name, Value: v})
		}
	}
	// Anything the type does not declare is passed through so Validate can
	// reject it by name.
	known := map[string]bool{}
	for _, name := range gen.Fields() {
		known[name] = true
	}
	for name, v := range in {
		if !known[name] {
			out = append(out, Field{Name: name, Value: v})
		}
	}
	return out
}

// writeNoteError maps domain errors onto status codes.
func writeNoteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "note not found")
	case errors.Is(err, ErrDeckNotFound):
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
	case errors.Is(err, ErrInvalidNote), errors.Is(err, ErrUnknownNoteType):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		slog.Error("note request failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "request failed")
	}
}

// NoteTypes handles GET /api/note-types, telling the client which note types
// exist and what fields each expects. It lists only what is implemented, so
// the editor never offers a type the server would reject.
func (h *Handler) NoteTypes(w http.ResponseWriter, r *http.Request) {
	out := make([]map[string]any, 0, len(KnownTypes()))
	for _, t := range KnownTypes() {
		gen, err := GeneratorFor(t)
		if err != nil {
			continue
		}
		out = append(out, map[string]any{
			"type":   string(t),
			"fields": gen.Fields(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Create handles POST /api/notes.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	gen, err := GeneratorFor(NoteType(req.Type))
	if err != nil {
		writeNoteError(w, err)
		return
	}

	n, err := h.repo.Create(r.Context(), u.ID, CreateInput{
		DeckID: req.DeckID,
		Type:   NoteType(req.Type),
		Config: Config{
			Reversed:  req.Reversed != nil && *req.Reversed,
			Occlusion: req.Occlusion,
		},
		Fields: fieldsInTypeOrder(gen, req.Fields),
	})
	if err != nil {
		writeNoteError(w, err)
		return
	}
	h.writeNote(w, http.StatusCreated, u.ID, n, r)
}

// List handles GET /api/notes, optionally filtered by ?deck_id= and windowed
// by ?limit=/?offset=.
//
// It answers with a page envelope rather than a bare array: a pager cannot be
// drawn, nor "50 of 380" written, without knowing the total.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())

	f := ListFilter{Limit: DefaultListLimit}
	if raw := r.URL.Query().Get("deck_id"); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			f.DeckID = id
		}
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			f.Limit = min(n, MaxListLimit)
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			f.Offset = n
		}
	}

	list, err := h.repo.List(r.Context(), u.ID, f)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	total, err := h.repo.Count(r.Context(), u.ID, f)
	if err != nil {
		writeNoteError(w, err)
		return
	}

	items := make([]map[string]any, 0, len(list))
	for _, n := range list {
		items = append(items, publicNote(n, nil))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// Preview handles GET /api/notes/{id}/preview, rendering every card the note
// currently produces.
//
// It exists so an author can check a card reads correctly without studying
// it, and it renders through the same generator review uses, so a preview
// cannot drift from what will actually be asked. The markdown comes back
// verbatim — images and all — and the client renders it exactly as the study
// screen would.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "note not found")
		return
	}

	n, err := h.repo.Get(r.Context(), u.ID, id)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	gen, err := GeneratorFor(n.Type)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	specs, err := gen.Generate(n.Fields, n.Config)
	if err != nil {
		writeNoteError(w, err)
		return
	}

	out := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		rendered, err := gen.Render(n.Fields, n.Config, spec.Template)
		if err != nil {
			writeNoteError(w, err)
			return
		}
		out = append(out, map[string]any{
			"template": spec.Template,
			"question": rendered.Question,
			"answer":   rendered.Answer,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Get handles GET /api/notes/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "note not found")
		return
	}

	n, err := h.repo.Get(r.Context(), u.ID, id)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	h.writeNote(w, http.StatusOK, u.ID, n, r)
}

// Update handles PUT /api/notes/{id}.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "note not found")
		return
	}

	existing, err := h.repo.Get(r.Context(), u.ID, id)
	if err != nil {
		writeNoteError(w, err)
		return
	}

	var req noteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// The client re-detects the note's type from its content on every save —
	// adding a cloze to a basic note converts it in place. An omitted type
	// (an old client) keeps the note as it is.
	newType := existing.Type
	if req.Type != "" {
		newType = NoteType(req.Type)
	}
	gen, err := GeneratorFor(newType)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	if req.DeckID == 0 {
		req.DeckID = existing.DeckID
	}
	reversed := existing.Config.Reversed
	if req.Reversed != nil {
		reversed = *req.Reversed
	}
	occlusion := existing.Config.Occlusion
	if req.Occlusion != nil {
		occlusion = req.Occlusion
	}

	n, err := h.repo.Update(r.Context(), u.ID, id, UpdateInput{
		DeckID: req.DeckID,
		Type:   newType,
		Config: Config{Reversed: reversed, Occlusion: occlusion},
		Fields: fieldsInTypeOrder(gen, req.Fields),
	})
	if err != nil {
		writeNoteError(w, err)
		return
	}
	h.writeNote(w, http.StatusOK, u.ID, n, r)
}

// Delete handles DELETE /api/notes/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	id, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "note not found")
		return
	}

	if err := h.repo.Delete(r.Context(), u.ID, id); err != nil {
		writeNoteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeNote responds with a note and the cards it currently generates.
func (h *Handler) writeNote(w http.ResponseWriter, status int, userID int64, n *Note, r *http.Request) {
	list, err := h.cards.ListForNote(r.Context(), userID, n.ID)
	if err != nil {
		writeNoteError(w, err)
		return
	}
	httpx.WriteJSON(w, status, publicNote(n, list))
}

// publicNote projects a note onto its JSON shape. Fields come back as a map
// for symmetry with the request.
func publicNote(n *Note, list []*cards.Card) map[string]any {
	f := make(map[string]string, len(n.Fields))
	for _, field := range n.Fields {
		f[field.Name] = field.Value
	}

	out := map[string]any{
		"id":         n.ID,
		"deck_id":    n.DeckID,
		"type":       string(n.Type),
		"reversed":   n.Config.Reversed,
		// Null for notes without masks, so old clients see nothing new.
		"occlusion": n.Config.Occlusion,
		"fields":    f,
		"created_at": n.CreatedAt,
		"updated_at": n.UpdatedAt,
		// Null rather than a zero time when never studied, so the UI can say
		// "never" instead of printing year 1.
		"last_studied": n.LastStudied,
	}

	if list != nil {
		cardsOut := make([]map[string]any, 0, len(list))
		for _, c := range list {
			cardsOut = append(cardsOut, map[string]any{
				"id":        c.ID,
				"template":  c.Template,
				"state":     c.State.State.String(),
				"due":       c.State.Due,
				"reps":      c.State.Reps,
				"lapses":    c.State.Lapses,
				"suspended": c.Suspended,
			})
		}
		out["cards"] = cardsOut
	}
	return out
}
