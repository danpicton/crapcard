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
	"github.com/danpicton/crapcard/internal/decks"
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
	mux.Handle("GET /api/decks/{id}/study/queue", requireAuth(http.HandlerFunc(h.Queue)))
	mux.Handle("GET /api/study/queue", requireAuth(http.HandlerFunc(h.QueueAnywhere)))
	mux.Handle("POST /api/cards/{id}/suspend", requireAuth(http.HandlerFunc(h.Suspend)))
	mux.Handle("POST /api/cards/{id}/flag", requireAuth(http.HandlerFunc(h.Flag)))
	mux.Handle("POST /api/cards/{id}/bury", requireAuth(http.HandlerFunc(h.Bury)))
	mux.Handle("GET /api/decks/{id}/flagged", requireAuth(http.HandlerFunc(h.Flagged)))
}

// Queue handles GET /api/decks/{id}/study/queue: every due card in the deck,
// rendered, so the client can study through them without coming back — the
// offline session's fetch. An empty queue is an empty list, not a 204: the
// counts still matter.
func (h *Handler) Queue(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	deckID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
		return
	}

	q, err := h.svc.DeckQueue(r.Context(), u.ID, deckID, h.horizon(r))
	if err != nil {
		writeStudyError(w, err)
		return
	}
	writeQueue(w, q)
}

// QueueAnywhere handles GET /api/study/queue: the queue of whichever deck
// the user would land on. 204 when nothing is due in any deck.
func (h *Handler) QueueAnywhere(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())

	q, err := h.svc.QueueAnywhere(r.Context(), u.ID, h.horizon(r))
	if errors.Is(err, ErrQueueEmpty) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeStudyError(w, err)
		return
	}
	writeQueue(w, q)
}

func writeQueue(w http.ResponseWriter, q *Queue) {
	cardsOut := make([]map[string]any, 0, len(q.Cards))
	for _, question := range q.Cards {
		cardsOut = append(cardsOut, questionJSON(question))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deck_id":   q.DeckID,
		"deck_name": q.DeckName,
		"counts":    countsJSON(q.Counts),
		"cards":     cardsOut,
	})
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

// questionJSON is a Question in the study API's card shape.
func questionJSON(q *Question) map[string]any {
	previews := make(map[string]any, len(q.Previews))
	for rating, p := range q.Previews {
		previews[rating.String()] = map[string]any{
			"interval_seconds": int64(p.Interval.Seconds()),
			"label":            FormatInterval(p.Interval),
			"due":              p.Due,
		}
	}

	return map[string]any{
		"card_id":   q.CardID,
		"note_id":   q.NoteID,
		"deck_id":   q.DeckID,
		"deck_name": q.DeckName,
		"template":  q.Template,
		"question":  q.Question,
		"answer":    q.Answer,
		"state":     q.State.String(),
		"flagged":   q.Flagged,
		"counts":    countsJSON(q.Counts),
		"previews":  previews,
	}
}

func writeQuestion(w http.ResponseWriter, q *Question) {
	httpx.WriteJSON(w, http.StatusOK, questionJSON(q))
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

// Suspend handles POST /api/cards/{id}/suspend with {"suspended": bool} —
// one endpoint for both directions, so resume cannot drift from suspend.
func (h *Handler) Suspend(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	cardID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "card not found")
		return
	}

	var req struct {
		Suspended bool `json:"suspended"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.SetSuspended(r.Context(), u.ID, cardID, req.Suspended); err != nil {
		writeStudyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"card_id":   cardID,
		"suspended": req.Suspended,
	})
}

// MaxFlagReasonLen bounds the flag reason: a note to your future self, not an
// essay.
const MaxFlagReasonLen = 2000

// Flag handles POST /api/cards/{id}/flag with {"flagged": bool, "reason": "…"}.
// The reason is optional and only kept while the card stays flagged.
func (h *Handler) Flag(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	cardID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "card not found")
		return
	}

	var req struct {
		Flagged bool   `json:"flagged"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Reason) > MaxFlagReasonLen {
		httpx.WriteError(w, http.StatusBadRequest, "flag reason is too long")
		return
	}

	if err := h.svc.Flag(r.Context(), u.ID, cardID, req.Flagged, req.Reason); err != nil {
		writeStudyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"card_id": cardID,
		"flagged": req.Flagged,
	})
}

// Bury handles POST /api/cards/{id}/bury with {"days": n}: 1 hides the card
// until tomorrow, n until n-1 days after that, 0 unburies. Days are the
// user's own (via tz_offset), so "tomorrow" is their tomorrow.
func (h *Handler) Bury(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	cardID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "card not found")
		return
	}

	var req struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	until, err := h.svc.Bury(r.Context(), u.ID, cardID, req.Days, h.horizon(r))
	if err != nil {
		writeStudyError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"card_id":      cardID,
		"buried_until": nullableTimeJSON(until),
	})
}

// Flagged handles GET /api/decks/{id}/flagged: the deck's flagged cards with
// their reasons — the one place the reason is ever sent to the client.
func (h *Handler) Flagged(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFromContext(r.Context())
	deckID, ok := httpx.PathID(r, "id")
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
		return
	}

	list, err := h.svc.FlaggedCards(r.Context(), u.ID, deckID)
	if err != nil {
		writeStudyError(w, err)
		return
	}

	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, map[string]any{
			"card_id":      c.CardID,
			"note_id":      c.NoteID,
			"template":     c.Template,
			"question":     c.Question,
			"reason":       c.Reason,
			"suspended":    c.Suspended,
			"buried_until": nullableTimeJSON(c.BuriedUntil),
			"state":        c.State.String(),
			"due":          c.Due,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"cards": out})
}

// nullableTimeJSON renders a time as JSON null when it is the zero value.
func nullableTimeJSON(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
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
	case errors.Is(err, decks.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "deck not found")
	case errors.Is(err, cards.ErrStaleReview):
		httpx.WriteError(w, http.StatusConflict, "this card was already answered")
	case errors.Is(err, ErrCardSuspended):
		httpx.WriteError(w, http.StatusConflict, "this card is suspended")
	case errors.Is(err, ErrBadBuryDays):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
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
