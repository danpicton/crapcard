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

func TestSuspendEndpointTogglesSuspension(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	q, _ := e.svc.Next(context.Background(), e.user, e.deck, at(timeNow()))

	rec := e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/suspend", `{"suspended":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	card, _ := e.cards.Get(context.Background(), e.user, q.CardID)
	if !card.Suspended {
		t.Fatalf("card not suspended")
	}
	// A suspended card leaves the queue at once.
	rec = e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/study/next", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("suspended card still served: %d", rec.Code)
	}

	rec = e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/suspend", `{"suspended":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume status = %d", rec.Code)
	}
	card, _ = e.cards.Get(context.Background(), e.user, q.CardID)
	if card.Suspended {
		t.Fatalf("card still suspended after resume")
	}

	rec = e.serve(t, e.other, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/suspend", `{"suspended":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("another user suspending: %d, want 404", rec.Code)
	}
}

func TestFlagEndpointStoresTheReason(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	q, _ := e.svc.Next(context.Background(), e.user, e.deck, at(timeNow()))

	rec := e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/flag",
		`{"flagged":true,"reason":"back is wrong?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	card, _ := e.cards.Get(context.Background(), e.user, q.CardID)
	if !card.Flagged || card.FlagReason != "back is wrong?" {
		t.Fatalf("flag not stored: %+v", card)
	}

	// A flagged card keeps studying — flags are annotations, not suspensions.
	rec = e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/study/next", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("flagged card left the queue: %d", rec.Code)
	}
	var got struct {
		Flagged bool `json:"flagged"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Flagged {
		t.Fatalf("study card does not carry the flag: %s", rec.Body.String())
	}

	tooLong := strings.Repeat("x", study.MaxFlagReasonLen+1)
	rec = e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/flag",
		`{"flagged":true,"reason":"`+tooLong+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized reason accepted: %d", rec.Code)
	}
}

func TestBuryEndpointHidesTheCardForItsDays(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	q, _ := e.svc.Next(context.Background(), e.user, e.deck, at(timeNow()))

	rec := e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/bury", `{"days":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		BuriedUntil *string `json:"buried_until"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.BuriedUntil == nil {
		t.Fatalf("no buried_until in response: %s", rec.Body.String())
	}

	rec = e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/study/next", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("buried card still served: %d", rec.Code)
	}

	// Days 0 unburies.
	rec = e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/bury", `{"days":0}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unbury status = %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.BuriedUntil != nil {
		t.Fatalf("unbury still reports buried_until: %s", rec.Body.String())
	}
	rec = e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/study/next", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("unburied card not served: %d", rec.Code)
	}

	for _, body := range []string{`{"days":-1}`, `{"days":9999}`, `garbage`} {
		rec = e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/bury", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s gave %d, want 400", body, rec.Code)
		}
	}
}

func TestFlaggedEndpointListsReasonsPerDeck(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	q, _ := e.svc.Next(context.Background(), e.user, e.deck, at(timeNow()))

	e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/flag",
		`{"flagged":true,"reason":"needs a better example"}`)
	e.serve(t, e.user, http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/suspend", `{"suspended":true}`)

	rec := e.serve(t, e.user, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/flagged", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Cards []struct {
			CardID    int64  `json:"card_id"`
			Question  string `json:"question"`
			Reason    string `json:"reason"`
			Suspended bool   `json:"suspended"`
		} `json:"cards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Cards) != 1 {
		t.Fatalf("cards = %s, want 1", rec.Body.String())
	}
	c := got.Cards[0]
	if c.CardID != q.CardID || c.Question != "ciao" || c.Reason != "needs a better example" || !c.Suspended {
		t.Fatalf("flagged card = %+v", c)
	}

	// Unknown deck 404s; another user's deck too.
	rec = e.serve(t, e.user, http.MethodGet, "/api/decks/99999/flagged", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown deck: %d, want 404", rec.Code)
	}
	rec = e.serve(t, e.other, http.MethodGet, "/api/decks/"+itoa(e.deck)+"/flagged", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("another user's deck: %d, want 404", rec.Code)
	}
}
