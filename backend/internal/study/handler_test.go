package study_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/study"
)

func (e *env) serve(t *testing.T, userID int64, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	study.NewHandler(e.svc).Register(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := &auth.User{ID: userID, Username: "test"}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	})

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestNextEndpointReturnsACard(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)

	rec := e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/study/next", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		CardID   int64  `json:"card_id"`
		Question string `json:"question"`
		Answer   string `json:"answer"`
		Counts   struct {
			New int `json:"new"`
		} `json:"counts"`
		Previews map[string]struct {
			Seconds int64  `json:"interval_seconds"`
			Label   string `json:"label"`
		} `json:"previews"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CardID == 0 || got.Question != "ciao" || got.Answer != "hello" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if got.Counts.New != 1 {
		t.Fatalf("counts.new = %d, want 1", got.Counts.New)
	}
	for _, rating := range []string{"again", "hard", "good", "easy"} {
		p, ok := got.Previews[rating]
		if !ok {
			t.Fatalf("previews missing %q: %s", rating, rec.Body.String())
		}
		if p.Seconds <= 0 {
			t.Fatalf("preview %q has interval %d", rating, p.Seconds)
		}
		if p.Label == "" {
			t.Fatalf("preview %q has no human label", rating)
		}
	}
}

func TestNextEndpointReturns204WhenQueueEmpty(t *testing.T) {
	// A finished session is not an error; the client checks for 204.
	e := newEnv(t)

	rec := e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/study/next", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestAnswerEndpointSchedulesTheCard(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)

	q, err := e.svc.Next(context.Background(), e.user, e.deck, at(timeNow()))
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	rec := e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/answer", `{"rating":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		Interval int64  `json:"interval_seconds"`
		Label    string `json:"interval_label"`
		Due      string `json:"due"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Interval <= 0 || got.Label == "" || got.Due == "" {
		t.Fatalf("body = %s", rec.Body.String())
	}

	card, _ := e.cards.Get(context.Background(), e.user, q.CardID)
	if card.State.Reps != 1 {
		t.Fatalf("answer was not persisted")
	}
}

func TestAnswerEndpointRejectsInvalidRating(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	q, _ := e.svc.Next(context.Background(), e.user, e.deck, at(timeNow()))

	for _, body := range []string{`{"rating":0}`, `{"rating":5}`, `{"rating":-1}`, `{}`, `garbage`} {
		rec := e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/answer", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s gave status %d, want 400", body, rec.Code)
		}
	}

	card, _ := e.cards.Get(context.Background(), e.user, q.CardID)
	if card.State.Reps != 0 {
		t.Fatalf("an invalid rating advanced the card")
	}
}

func TestAnswerEndpointRefusesAnotherUsersCard(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	q, _ := e.svc.Next(context.Background(), e.user, e.deck, at(timeNow()))

	rec := e.serve(t, e.other, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/answer", `{"rating":3}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeckCountsEndpoint(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", true)

	rec := e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/study/counts", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		New   int `json:"new"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.New != 2 || got.Total != 2 {
		t.Fatalf("counts = %s, want 2 new from a reversed note", rec.Body.String())
	}
}

func TestNextAnywhereEndpointReturnsACardAndItsDeck(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)

	rec := e.serve(t, e.user, http.MethodGet, "/api/study/next", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		CardID   int64  `json:"card_id"`
		DeckID   int64  `json:"deck_id"`
		DeckName string `json:"deck_name"`
		Question string `json:"question"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CardID == 0 || got.Question != "ciao" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	// The landing screen has to say which deck it dropped you into.
	if got.DeckID != e.deck || got.DeckName != "Italian" {
		t.Fatalf("deck = %d/%q, want %d/Italian", got.DeckID, got.DeckName, e.deck)
	}
}

func TestNextAnywhereEndpointIs204WhenNothingIsDue(t *testing.T) {
	e := newEnv(t)

	rec := e.serve(t, e.user, http.MethodGet, "/api/study/next", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}
