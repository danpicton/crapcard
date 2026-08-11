package study_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/cards"
	"github.com/danpicton/crapcard/internal/db"
	"github.com/danpicton/crapcard/internal/decks"
	"github.com/danpicton/crapcard/internal/notes"
	"github.com/danpicton/crapcard/internal/srs"
	"github.com/danpicton/crapcard/internal/study"
)

type env struct {
	db    *db.DB
	svc   *study.Service
	notes *notes.Repository
	cards *cards.Repository
	user  int64
	other int64
	deck  int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	ctx := context.Background()
	users := auth.NewUserRepo(database)
	u, err := users.Create(ctx, "dan", "h", true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	other, err := users.Create(ctx, "someone-else", "h", false)
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	d, err := decks.NewRepository(database).Create(ctx, u.ID, "Italian", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}

	noteRepo := notes.NewRepository(database)
	cardRepo := cards.NewRepository(database)
	return &env{
		db:    database,
		svc:   study.NewService(noteRepo, cardRepo, srs.DefaultParams()),
		notes: noteRepo,
		cards: cardRepo,
		user:  u.ID,
		other: other.ID,
		deck:  d.ID,
	}
}

func (e *env) addNote(t *testing.T, front, back string, reversed bool) *notes.Note {
	t.Helper()
	n, err := e.notes.Create(context.Background(), e.user, notes.CreateInput{
		DeckID: e.deck,
		Type:   notes.TypeBasic,
		Config: notes.Config{Reversed: reversed},
		Fields: []notes.Field{{Name: "front", Value: front}, {Name: "back", Value: back}},
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	return n
}

func TestNextReturnsARenderedCard(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)

	q, err := e.svc.Next(context.Background(), e.user, e.deck, time.Now())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if q.CardID == 0 {
		t.Fatalf("no card id")
	}
	if q.Question != "ciao" || q.Answer != "hello" {
		t.Fatalf("rendered = %q / %q, want ciao / hello", q.Question, q.Answer)
	}
	if q.Template != notes.TemplateForward {
		t.Fatalf("template = %q", q.Template)
	}
}

func TestNextRendersAReversedCardBackwards(t *testing.T) {
	// Reversal is the whole point of the note/card split: two cards, one note,
	// each asking a different side.
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", true)
	ctx := context.Background()
	now := time.Now()

	seen := map[string]string{}
	for range 2 {
		q, err := e.svc.Next(ctx, e.user, e.deck, now)
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		seen[q.Template] = q.Question

		if _, err := e.svc.Answer(ctx, e.user, q.CardID, srs.RatingEasy, now); err != nil {
			t.Fatalf("Answer: %v", err)
		}
	}

	if seen[notes.TemplateForward] != "ciao" {
		t.Fatalf("forward asked %q, want ciao", seen[notes.TemplateForward])
	}
	if seen[notes.TemplateReverse] != "hello" {
		t.Fatalf("reverse asked %q, want hello", seen[notes.TemplateReverse])
	}
}

func TestNextIncludesTheIntervalEachAnswerWouldGive(t *testing.T) {
	// The four answer buttons are labelled with these, so they must cover
	// every rating and grow with it.
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)

	q, err := e.svc.Next(context.Background(), e.user, e.deck, time.Now())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	var last time.Duration
	for _, r := range []srs.Rating{srs.RatingAgain, srs.RatingHard, srs.RatingGood, srs.RatingEasy} {
		p, ok := q.Previews[r]
		if !ok {
			t.Fatalf("no preview for rating %v", r)
		}
		if p.Interval <= 0 {
			t.Fatalf("preview for %v has interval %v", r, p.Interval)
		}
		if p.Interval <= last {
			t.Fatalf("preview for %v (%v) is not longer than the previous (%v)", r, p.Interval, last)
		}
		last = p.Interval
	}
}

func TestNextReportsTheQueueCounts(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	e.addNote(t, "grazie", "thanks", false)

	q, err := e.svc.Next(context.Background(), e.user, e.deck, time.Now())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if q.Counts.New != 2 {
		t.Fatalf("counts = %+v, want 2 new", q.Counts)
	}
}

func TestNextReturnsErrQueueEmptyWhenNothingIsDue(t *testing.T) {
	e := newEnv(t)

	if _, err := e.svc.Next(context.Background(), e.user, e.deck, time.Now()); !errors.Is(err, study.ErrQueueEmpty) {
		t.Fatalf("Next on an empty deck = %v, want ErrQueueEmpty", err)
	}
}

func TestAnswerSchedulesTheCardForward(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	ctx := context.Background()
	now := time.Now()

	q, err := e.svc.Next(ctx, e.user, e.deck, now)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}

	res, err := e.svc.Answer(ctx, e.user, q.CardID, srs.RatingGood, now)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if !res.Due.After(now) {
		t.Fatalf("card due at %v, not after %v", res.Due, now)
	}
	if res.Interval <= 0 {
		t.Fatalf("interval = %v", res.Interval)
	}

	card, err := e.cards.Get(ctx, e.user, q.CardID)
	if err != nil {
		t.Fatalf("Get card: %v", err)
	}
	if card.State.Reps != 1 {
		t.Fatalf("reps = %d, want 1 — the answer was not persisted", card.State.Reps)
	}
}

func TestAnsweringEverythingEmptiesTheQueue(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	ctx := context.Background()
	now := time.Now()

	q, err := e.svc.Next(ctx, e.user, e.deck, now)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if _, err := e.svc.Answer(ctx, e.user, q.CardID, srs.RatingEasy, now); err != nil {
		t.Fatalf("Answer: %v", err)
	}

	if _, err := e.svc.Next(ctx, e.user, e.deck, now); !errors.Is(err, study.ErrQueueEmpty) {
		t.Fatalf("Next after answering the only card = %v, want ErrQueueEmpty", err)
	}
}

func TestAnswerRefusesAnotherUsersCard(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	ctx := context.Background()
	now := time.Now()

	q, _ := e.svc.Next(ctx, e.user, e.deck, now)

	if _, err := e.svc.Answer(ctx, e.other, q.CardID, srs.RatingGood, now); !errors.Is(err, cards.ErrNotFound) {
		t.Fatalf("cross-user Answer = %v, want ErrNotFound", err)
	}
	card, _ := e.cards.Get(ctx, e.user, q.CardID)
	if card.State.Reps != 0 {
		t.Fatalf("another user advanced the card")
	}
}

func TestNextIsScopedToTheOwner(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)

	if _, err := e.svc.Next(context.Background(), e.other, e.deck, time.Now()); !errors.Is(err, study.ErrQueueEmpty) {
		t.Fatalf("another user got a card from this deck: %v", err)
	}
}

func TestAgainBringsTheCardBackSoon(t *testing.T) {
	e := newEnv(t)
	e.addNote(t, "ciao", "hello", false)
	ctx := context.Background()
	now := time.Now()

	q, _ := e.svc.Next(ctx, e.user, e.deck, now)
	res, err := e.svc.Answer(ctx, e.user, q.CardID, srs.RatingAgain, now)
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if res.Interval > time.Hour {
		t.Fatalf("Again scheduled the card %v out, want a short learning step", res.Interval)
	}

	// It should be back in the queue once that short step elapses.
	if _, err := e.svc.Next(ctx, e.user, e.deck, res.Due.Add(time.Second)); err != nil {
		t.Fatalf("card did not return after Again: %v", err)
	}
}

func TestFieldMarkdownReachesTheClientUntouched(t *testing.T) {
	e := newEnv(t)
	md := "**ciao** ![](/api/images/7)"
	e.addNote(t, md, "hello", false)

	q, err := e.svc.Next(context.Background(), e.user, e.deck, time.Now())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if q.Question != md {
		t.Fatalf("markdown altered:\n got %q\nwant %q", q.Question, md)
	}
}

// timeNow is a seam for the handler tests, which drive the service directly
// to set a card up before exercising an endpoint.
func timeNow() time.Time { return time.Now() }
