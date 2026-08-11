package cards_test

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
)

type env struct {
	db    *db.DB
	repo  *cards.Repository
	user  int64
	other int64
	deck  int64
	note  int64
}

// newEnv builds a database holding one user with one deck and one note row,
// so card tests can work against a real foreign key graph.
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

	deck, err := decks.NewRepository(database).Create(ctx, u.ID, "Italian", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}

	noteRepo := notes.NewRepository(database)
	n, err := noteRepo.Create(ctx, u.ID, notes.CreateInput{
		DeckID: deck.ID,
		Type:   notes.TypeBasic,
		Fields: []notes.Field{{Name: "front", Value: "ciao"}, {Name: "back", Value: "hello"}},
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	return &env{
		db:    database,
		repo:  cards.NewRepository(database),
		user:  u.ID,
		other: other.ID,
		deck:  deck.ID,
		note:  n.ID,
	}
}

func TestNoteCreationMaterialisesItsCards(t *testing.T) {
	e := newEnv(t)

	list, err := e.repo.ListForNote(context.Background(), e.user, e.note)
	if err != nil {
		t.Fatalf("ListForNote: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("got %d cards for a non-reversed basic note, want 1", len(list))
	}
	c := list[0]
	if c.Template != notes.TemplateForward {
		t.Fatalf("template = %q, want forward", c.Template)
	}
	if c.State.State != srs.StateNew {
		t.Fatalf("new card state = %v, want New", c.State.State)
	}
	if c.DeckID != e.deck {
		t.Fatalf("card deck = %d, want %d (cards carry the deck for the queue index)", c.DeckID, e.deck)
	}
}

func TestSyncAddsAndRemovesCardsForChangedTemplates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// Turning reversal on adds a second card.
	if err := e.repo.SyncForNote(ctx, nil, cards.SyncInput{
		NoteID: e.note, UserID: e.user, DeckID: e.deck,
		Templates: []string{notes.TemplateForward, notes.TemplateReverse},
	}); err != nil {
		t.Fatalf("SyncForNote add: %v", err)
	}
	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	if len(list) != 2 {
		t.Fatalf("got %d cards after enabling reversal, want 2", len(list))
	}

	// Turning it back off removes the reverse card only.
	if err := e.repo.SyncForNote(ctx, nil, cards.SyncInput{
		NoteID: e.note, UserID: e.user, DeckID: e.deck,
		Templates: []string{notes.TemplateForward},
	}); err != nil {
		t.Fatalf("SyncForNote remove: %v", err)
	}
	list, _ = e.repo.ListForNote(ctx, e.user, e.note)
	if len(list) != 1 || list[0].Template != notes.TemplateForward {
		t.Fatalf("after disabling reversal: %+v", list)
	}
}

func TestSyncPreservesSchedulingOfCardsThatSurvive(t *testing.T) {
	// Editing a note must not throw away what the user has already learned.
	e := newEnv(t)
	ctx := context.Background()

	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	forward := list[0]

	sched := srs.NewScheduler(srs.DefaultParams())
	reviewed := sched.Review(forward.State, time.Now(), srs.RatingGood)
	if err := e.repo.ApplyReview(ctx, e.user, forward.ID, reviewed.Card, reviewed.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	// A sync that still includes the forward template must leave it alone.
	if err := e.repo.SyncForNote(ctx, nil, cards.SyncInput{
		NoteID: e.note, UserID: e.user, DeckID: e.deck,
		Templates: []string{notes.TemplateForward, notes.TemplateReverse},
	}); err != nil {
		t.Fatalf("SyncForNote: %v", err)
	}

	after, _ := e.repo.ListForNote(ctx, e.user, e.note)
	var got *cards.Card
	for _, c := range after {
		if c.Template == notes.TemplateForward {
			got = c
		}
	}
	if got == nil {
		t.Fatalf("forward card disappeared")
	}
	if got.ID != forward.ID {
		t.Fatalf("forward card was recreated (id %d -> %d), losing its history", forward.ID, got.ID)
	}
	if got.State.Reps != 1 {
		t.Fatalf("reps = %d after a review and a sync, want 1", got.State.Reps)
	}
	if got.State.State == srs.StateNew {
		t.Fatalf("card was reset to New by an unrelated edit")
	}
}

func TestApplyReviewPersistsStateAndWritesALogEntry(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	card := list[0]

	now := time.Now().UTC().Truncate(time.Second)
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(card.State, now, srs.RatingHard)

	if err := e.repo.ApplyReview(ctx, e.user, card.ID, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	after, _ := e.repo.ListForNote(ctx, e.user, e.note)
	got := after[0]
	if got.State.Reps != res.Card.Reps {
		t.Fatalf("reps = %d, want %d", got.State.Reps, res.Card.Reps)
	}
	if !got.State.Due.Equal(res.Card.Due.UTC().Truncate(time.Second)) &&
		got.State.Due.Sub(res.Card.Due).Abs() > time.Second {
		t.Fatalf("due = %v, want %v", got.State.Due, res.Card.Due)
	}

	logs, err := e.repo.ReviewLog(ctx, e.user, card.ID)
	if err != nil {
		t.Fatalf("ReviewLog: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d log entries, want 1", len(logs))
	}
	if logs[0].Rating != srs.RatingHard {
		t.Fatalf("logged rating = %v, want Hard", logs[0].Rating)
	}
}

func TestApplyReviewRefusesAnotherUsersCard(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	card := list[0]

	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(card.State, time.Now(), srs.RatingEasy)

	if err := e.repo.ApplyReview(ctx, e.other, card.ID, res.Card, res.Log); !errors.Is(err, cards.ErrNotFound) {
		t.Fatalf("cross-user ApplyReview = %v, want ErrNotFound", err)
	}

	after, _ := e.repo.ListForNote(ctx, e.user, e.note)
	if after[0].State.Reps != 0 {
		t.Fatalf("another user advanced the card's schedule")
	}
}

func TestListForNoteIsScopedToOwner(t *testing.T) {
	e := newEnv(t)

	list, err := e.repo.ListForNote(context.Background(), e.other, e.note)
	if err != nil {
		t.Fatalf("ListForNote: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("another user saw %d cards", len(list))
	}
}

func TestDueReturnsOnlyCardsThatAreReady(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	due, err := e.repo.Due(ctx, e.user, e.deck, now, 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("got %d due cards, want the new card", len(due))
	}

	// Answer it, pushing it into the future.
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(due[0].State, now, srs.RatingEasy)
	if err := e.repo.ApplyReview(ctx, e.user, due[0].ID, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	due, _ = e.repo.Due(ctx, e.user, e.deck, now, 10)
	if len(due) != 0 {
		t.Fatalf("card is due again immediately after being answered Easy")
	}

	// It comes back once its due date arrives.
	due, _ = e.repo.Due(ctx, e.user, e.deck, res.Card.Due.Add(time.Minute), 10)
	if len(due) != 1 {
		t.Fatalf("card did not return to the queue at its due date")
	}
}

func TestDueRespectsLimitAndDeckAndOwner(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	// A second deck with its own note must not leak into the first deck's queue.
	otherDeck, err := decks.NewRepository(e.db).Create(ctx, e.user, "Anatomy", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	if _, err := notes.NewRepository(e.db).Create(ctx, e.user, notes.CreateInput{
		DeckID: otherDeck.ID,
		Type:   notes.TypeBasic,
		Fields: []notes.Field{{Name: "front", Value: "femur"}, {Name: "back", Value: "thigh bone"}},
	}); err != nil {
		t.Fatalf("create note: %v", err)
	}

	due, _ := e.repo.Due(ctx, e.user, e.deck, now, 10)
	if len(due) != 1 {
		t.Fatalf("deck queue returned %d cards, want 1 — another deck leaked in", len(due))
	}

	if due, _ = e.repo.Due(ctx, e.user, e.deck, now, 0); len(due) != 0 {
		t.Fatalf("limit 0 returned %d cards", len(due))
	}
	if due, _ = e.repo.Due(ctx, e.other, e.deck, now, 10); len(due) != 0 {
		t.Fatalf("another user saw %d cards from this deck", len(due))
	}
}

func TestCountsSummariseTheQueue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	counts, err := e.repo.Counts(ctx, e.user, e.deck, now)
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if counts.New != 1 || counts.Learning != 0 || counts.Due != 0 {
		t.Fatalf("counts = %+v, want one new card", counts)
	}

	due, _ := e.repo.Due(ctx, e.user, e.deck, now, 10)
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(due[0].State, now, srs.RatingAgain)
	if err := e.repo.ApplyReview(ctx, e.user, due[0].ID, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	counts, _ = e.repo.Counts(ctx, e.user, e.deck, now)
	if counts.New != 0 {
		t.Fatalf("card still counted as new after being answered: %+v", counts)
	}
	if counts.Learning != 1 {
		t.Fatalf("counts = %+v, want the card in learning", counts)
	}
}

func TestCardsCascadeWhenTheirNoteGoes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	if err := notes.NewRepository(e.db).Delete(ctx, e.user, e.note); err != nil {
		t.Fatalf("delete note: %v", err)
	}
	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	if len(list) != 0 {
		t.Fatalf("%d cards survived their note", len(list))
	}
}
