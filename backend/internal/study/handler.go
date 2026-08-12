package study

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/cards"
	"github.com/danpicton/crapcard/internal/httpx"
	"github.com/danpicton/crapcard/internal/notes"
	"github.com/danpicton/crapcard/internal/srs"
)

// Handler serves the study endpoints.
type Handler struct {
	svc *Service
	// now is overridable so tests can study at a fixed moment.
	now func() time.Time
}

// NewHandler creates a study Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, now: time.Now}
}

// Middleware wraps a handler, typically with authentication.
type Middleware func(http.Handler) http.Handler

// Register mounts the study routes.
func (h *Handler) Register(mux *http.ServeMux, requireAuth Middleware) {
	mux.Handle("GET /api/study/next", requireAuth(http.HandlerFunc(h.NextAnywhere)))
	mux.Handle("GET /api/decks/{id}/study/next", requireAuth(http.HandlerFunc(h.Next)))
	mux.Handle("GET /api/decks/{id}/study/counts", requireAuth(http.HandlerFunc(h.Counts)))
	mux.Handle("POST /api/cards/{id}/answer", requireAuth(http.HandlerFunc(h.Answer)))
	mux.Handle("POST /api/study/undo", requireAuth(http.HandlerFunc(h.Undo)))
}

// Undo handles POST /api/study/undo: revert the most recent answer and hand
// the card back so it can be graded again. 204 when there is nothing to undo
// — that is the state a fresh session starts in, not an error.
func (h *Handler) Undo(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())

	q, err := h.svc.Undo(r.Context(), u.ID, h.horizon(r))
	if errors.Is(err, cards.ErrNothingToUndo) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeStudyError(w, err)
		return
	}
	writeQuestion(w, q)
}

// horizon builds the study horizon from the request's tz_offset parameter —
// the client's minutes east of UTC, so review cards can be gated on the end
// of the user's own calendar day. A missing or malformed value falls back to
// UTC days, which is still day-granularity, just with a shifted boundary.
func (h *Handler) horizon(r *http.Request) cards.Horizon {
	offset := 0
	if raw := r.URL.Query().Get("tz_offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			offset = n
		}
	}
	return cards.HorizonAt(h.now(), offset)
}

// Next handles GET /api/decks/{id}/study/next.
//
// An empty queue is 204 rather than 404: finishing a deck is a normal
// outcome, not a missing resource.
func (h *Handler) Next(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	deckID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
		return
	}

	q, err := h.svc.Next(r.Context(), u.ID, deckID, h.horizon(r))
	if errors.Is(err, ErrQueueEmpty) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeStudyError(w, err)
		return
	}
	writeQuestion(w, q)
}

// NextAnywhere handles GET /api/study/next: the next due card without the
// caller naming a deck, for the landing screen straight after signing in.
// Like Next, an empty queue is 204.
func (h *Handler) NextAnywhere(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())

	q, err := h.svc.NextAnywhere(r.Context(), u.ID, h.horizon(r))
	if errors.Is(err, ErrQueueEmpty) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeStudyError(w, err)
		return
	}
	writeQuestion(w, q)
}

// writeQuestion renders a Question as the study API's card shape.
func writeQuestion(w http.ResponseWriter, q *Question) {
	previews := make(map[string]any, len(q.Previews))
	for rating, p := range q.Previews {
		previews[rating.String()] = map[string]any{
			"interval_seconds": int64(p.Interval.Seconds()),
			"label":            FormatInterval(p.Interval),
			"due":              p.Due,
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"card_id":   q.CardID,
		"note_id":   q.NoteID,
		"deck_id":   q.DeckID,
		"deck_name": q.DeckName,
		"template":  q.Template,
		"question": q.Question,
		"answer":   q.Answer,
		"state":    q.State.String(),
		"counts":   countsJSON(q.Counts),
		"previews": previews,
	})
}

// Counts handles GET /api/decks/{id}/study/counts.
func (h *Handler) Counts(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	deckID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
		return
	}

	counts, err := h.svc.DeckCounts(r.Context(), u.ID, deckID, h.horizon(r))
	if err != nil {
		writeStudyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, countsJSON(counts))
}

// Answer handles POST /api/cards/{id}/answer with {"rating": 1..4}.
func (h *Handler) Answer(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	cardID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "card not found")
		return
	}

	var req struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rating, err := srs.ParseRating(req.Rating)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.svc.Answer(r.Context(), u.ID, cardID, rating, h.horizon(r))
	if err != nil {
		writeStudyError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"card_id":          res.CardID,
		"interval_seconds": int64(res.Interval.Seconds()),
		"interval_label":   FormatInterval(res.Interval),
		"due":              res.Due,
		"state":            res.State.String(),
		"counts":           countsJSON(res.Counts),
	})
}

func countsJSON(c cards.Counts) map[string]any {
	return map[string]any{
		"new":      c.New,
		"learning": c.Learning,
		"due":      c.Due,
		"total":    c.Total(),
	}
}

// writeStudyError maps domain errors onto status codes.
func writeStudyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cards.ErrNotFound), errors.Is(err, notes.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "card not found")
	case errors.Is(err, cards.ErrStaleReview):
		httpx.WriteError(w, http.StatusConflict, "this card was already answered")
	case errors.Is(err, ErrCardSuspended):
		httpx.WriteError(w, http.StatusConflict, "this card is suspended")
	case errors.Is(err, ErrQueueEmpty):
		w.WriteHeader(http.StatusNoContent)
	default:
		slog.Error("study request failed", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "request failed")
	}
}

// FormatInterval renders a scheduling interval the way a study UI wants it on
// an answer button: coarse, short, and never more precise than it is
// meaningful ("10m", "3d", "1.2mo").
func FormatInterval(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(math.Round(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(math.Round(d.Minutes())))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(math.Round(d.Hours())))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(math.Round(d.Hours()/24)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%.1fmo", d.Hours()/24/30)
	default:
		return fmt.Sprintf("%.1fy", d.Hours()/24/365)
	}
}
