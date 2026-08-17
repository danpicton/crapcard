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
	if err := e.repo.ApplyReview(ctx, e.user, forward.ID, forward.State, reviewed.Card, reviewed.Log); err != nil {
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

	if err := e.repo.ApplyReview(ctx, e.user, card.ID, card.State, res.Card, res.Log); err != nil {
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

	if err := e.repo.ApplyReview(ctx, e.other, card.ID, card.State, res.Card, res.Log); !errors.Is(err, cards.ErrNotFound) {
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

	due, err := e.repo.Due(ctx, e.user, e.deck, at(now), 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("got %d due cards, want the new card", len(due))
	}

	// Answer it, pushing it into the future.
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(due[0].State, now, srs.RatingEasy)
	if err := e.repo.ApplyReview(ctx, e.user, due[0].ID, due[0].State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	due, _ = e.repo.Due(ctx, e.user, e.deck, at(now), 10)
	if len(due) != 0 {
		t.Fatalf("card is due again immediately after being answered Easy")
	}

	// It comes back once its due date arrives.
	due, _ = e.repo.Due(ctx, e.user, e.deck, at(res.Card.Due.Add(time.Minute)), 10)
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

	due, _ := e.repo.Due(ctx, e.user, e.deck, at(now), 10)
	if len(due) != 1 {
		t.Fatalf("deck queue returned %d cards, want 1 — another deck leaked in", len(due))
	}

	if due, _ = e.repo.Due(ctx, e.user, e.deck, at(now), 0); len(due) != 0 {
		t.Fatalf("limit 0 returned %d cards", len(due))
	}
	if due, _ = e.repo.Due(ctx, e.other, e.deck, at(now), 10); len(due) != 0 {
		t.Fatalf("another user saw %d cards from this deck", len(due))
	}
}

func TestCountsSummariseTheQueue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	counts, err := e.repo.Counts(ctx, e.user, e.deck, at(now))
	if err != nil {
		t.Fatalf("Counts: %v", err)
	}
	if counts.New != 1 || counts.Learning != 0 || counts.Due != 0 {
		t.Fatalf("counts = %+v, want one new card", counts)
	}

	due, _ := e.repo.Due(ctx, e.user, e.deck, at(now), 10)
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(due[0].State, now, srs.RatingAgain)
	if err := e.repo.ApplyReview(ctx, e.user, due[0].ID, due[0].State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	counts, _ = e.repo.Counts(ctx, e.user, e.deck, at(now))
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

func TestNextDeckToStudyPrefersTheMostRecentlyStudied(t *testing.T) {
	// Landing straight on a card means picking a deck for the user; the one
	// they were last working through is the least surprising choice.
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	older, err := decks.NewRepository(e.db).Create(ctx, e.user, "Older", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	recent, err := decks.NewRepository(e.db).Create(ctx, e.user, "Recent", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}

	noteRepo := notes.NewRepository(e.db)
	sched := srs.NewScheduler(srs.DefaultParams())
	// Each deck gets two notes so answering one leaves something still due.
	for _, deckID := range []int64{older.ID, recent.ID} {
		for range 2 {
			if _, err := noteRepo.Create(ctx, e.user, notes.CreateInput{
				DeckID: deckID, Type: notes.TypeBasic,
				Fields: []notes.Field{{Name: "front", Value: "a"}, {Name: "back", Value: "b"}},
			}); err != nil {
				t.Fatalf("create note: %v", err)
			}
		}
	}

	// Study one card in each deck, the "Recent" one more recently.
	study := func(deckID int64, when time.Time) {
		t.Helper()
		due, err := e.repo.Due(ctx, e.user, deckID, at(now), 1)
		if err != nil || len(due) == 0 {
			t.Fatalf("no due card in deck %d: %v", deckID, err)
		}
		res := sched.Review(due[0].State, when, srs.RatingGood)
		if err := e.repo.ApplyReview(ctx, e.user, due[0].ID, due[0].State, res.Card, res.Log); err != nil {
			t.Fatalf("ApplyReview: %v", err)
		}
	}
	study(older.ID, now.Add(-72*time.Hour))
	study(recent.ID, now.Add(-1*time.Hour))

	got, err := e.repo.NextDeckToStudy(ctx, e.user, at(now))
	if err != nil {
		t.Fatalf("NextDeckToStudy: %v", err)
	}
	if got != recent.ID {
		t.Fatalf("chose deck %d, want the most recently studied %d", got, recent.ID)
	}
}

func TestNextDeckToStudyFallsBackToAnUnstudiedDeck(t *testing.T) {
	// A brand new user has studied nothing, but still has cards waiting.
	e := newEnv(t)

	got, err := e.repo.NextDeckToStudy(context.Background(), e.user, at(time.Now()))
	if err != nil {
		t.Fatalf("NextDeckToStudy: %v", err)
	}
	if got != e.deck {
		t.Fatalf("chose deck %d, want %d", got, e.deck)
	}
}

func TestNextDeckToStudySkipsDecksWithNothingDue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	// Answer the only card in the seeded deck, so it has nothing left.
	due, _ := e.repo.Due(ctx, e.user, e.deck, at(now), 1)
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(due[0].State, now, srs.RatingEasy)
	if err := e.repo.ApplyReview(ctx, e.user, due[0].ID, due[0].State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	// A second deck still has one.
	other, err := decks.NewRepository(e.db).Create(ctx, e.user, "Anatomy", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	if _, err := notes.NewRepository(e.db).Create(ctx, e.user, notes.CreateInput{
		DeckID: other.ID, Type: notes.TypeBasic,
		Fields: []notes.Field{{Name: "front", Value: "femur"}, {Name: "back", Value: "thigh"}},
	}); err != nil {
		t.Fatalf("create note: %v", err)
	}

	got, err := e.repo.NextDeckToStudy(ctx, e.user, at(now))
	if err != nil {
		t.Fatalf("NextDeckToStudy: %v", err)
	}
	if got != other.ID {
		t.Fatalf("chose deck %d, want the deck that still has cards due (%d)", got, other.ID)
	}
}

func TestNextDeckToStudyReturnsErrNotFoundWhenNothingIsDueAnywhere(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	due, _ := e.repo.Due(ctx, e.user, e.deck, at(now), 1)
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(due[0].State, now, srs.RatingEasy)
	if err := e.repo.ApplyReview(ctx, e.user, due[0].ID, due[0].State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	if _, err := e.repo.NextDeckToStudy(ctx, e.user, at(now)); !errors.Is(err, cards.ErrNotFound) {
		t.Fatalf("NextDeckToStudy with nothing due = %v, want ErrNotFound", err)
	}
}

func TestNextDeckToStudyIsScopedToTheOwner(t *testing.T) {
	e := newEnv(t)

	if _, err := e.repo.NextDeckToStudy(context.Background(), e.other, at(time.Now())); !errors.Is(err, cards.ErrNotFound) {
		t.Fatalf("another user was offered a deck: %v", err)
	}
}

// at wraps a moment in the UTC-day horizon the queue queries expect.
func at(now time.Time) cards.Horizon { return cards.HorizonAt(now, 0) }

// addCard creates one more basic note in the env's deck and returns its
// card's id.
func addCard(t *testing.T, e *env) int64 {
	t.Helper()
	ctx := context.Background()
	n, err := notes.NewRepository(e.db).Create(ctx, e.user, notes.CreateInput{
		DeckID: e.deck, Type: notes.TypeBasic,
		Fields: []notes.Field{{Name: "front", Value: "a"}, {Name: "back", Value: "b"}},
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	list, err := e.repo.ListForNote(ctx, e.user, n.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("cards for new note: %v", err)
	}
	return list[0].ID
}

// review forces a card into review state with the given due date, as if FSRS
// had scheduled it there, so queue tests can stage precise scenarios.
func review(t *testing.T, e *env, cardID int64, due time.Time) {
	t.Helper()
	state := srs.CardState{
		Due: due, Stability: 10, Difficulty: 5,
		Reps: 3, State: srs.StateReview, LastReview: due.Add(-72 * time.Hour),
	}
	log := srs.ReviewLog{Rating: srs.RatingGood, State: srs.StateReview, Review: state.LastReview}
	prev, err := e.repo.Get(context.Background(), e.user, cardID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := e.repo.ApplyReview(context.Background(), e.user, cardID, prev.State, state, log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}
}

func TestDueServesReviewsBeforeNewCards(t *testing.T) {
	// A batch of freshly authored cards must not starve the reviews FSRS
	// actually scheduled for today: new cards carry the zero-time due date,
	// which would otherwise sort them to the very front forever.
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	overdue := list[0].ID
	review(t, e, overdue, now.Add(-2*time.Hour))
	newCard := addCard(t, e)

	due, err := e.repo.Due(ctx, e.user, e.deck, at(now), 10)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("got %d due cards, want 2", len(due))
	}
	if due[0].ID != overdue || due[1].ID != newCard {
		t.Fatalf("queue order = [%d %d], want the overdue review before the new card", due[0].ID, due[1].ID)
	}
}

func TestReviewDueLaterTodayCountsAsDue(t *testing.T) {
	// Day-scale scheduling is only meaningful at day granularity: a review due
	// at 14:00 must show up at the morning study session, not silently drift.
	e := newEnv(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	card := list[0].ID
	review(t, e, card, now.Add(5*time.Hour)) // due 14:00, same UTC day

	due, _ := e.repo.Due(ctx, e.user, e.deck, at(now), 10)
	if len(due) != 1 {
		t.Fatalf("review due later today missing from the queue (got %d cards)", len(due))
	}
	counts, _ := e.repo.Counts(ctx, e.user, e.deck, at(now))
	if counts.Due != 1 {
		t.Fatalf("counts.Due = %d, want 1", counts.Due)
	}

	// The same card due tomorrow is not due today.
	review(t, e, card, now.Add(20*time.Hour))
	if due, _ = e.repo.Due(ctx, e.user, e.deck, at(now), 10); len(due) != 0 {
		t.Fatalf("review due tomorrow served today")
	}
}

func TestHorizonFollowsTheClientDay(t *testing.T) {
	// 9:00 UTC, client at UTC+10 (19:00 local): their day ends at 14:00 UTC.
	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	h := cards.HorizonAt(now, 600)
	want := time.Date(2026, 3, 10, 14, 0, 0, 0, time.UTC)
	if !h.ReviewDueBefore.Equal(want) {
		t.Fatalf("ReviewDueBefore = %v, want %v", h.ReviewDueBefore, want)
	}

	// An absurd offset falls back to UTC days rather than a broken horizon.
	h = cards.HorizonAt(now, 100000)
	want = time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	if !h.ReviewDueBefore.Equal(want) {
		t.Fatalf("clamped ReviewDueBefore = %v, want %v", h.ReviewDueBefore, want)
	}
}

func TestUndoLastReviewRestoresTheCard(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	card := list[0]

	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(card.State, time.Now(), srs.RatingGood)
	if err := e.repo.ApplyReview(ctx, e.user, card.ID, card.State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	gotID, err := e.repo.UndoLastReview(ctx, e.user)
	if err != nil {
		t.Fatalf("UndoLastReview: %v", err)
	}
	if gotID != card.ID {
		t.Fatalf("undid card %d, want %d", gotID, card.ID)
	}

	after, _ := e.repo.Get(ctx, e.user, card.ID)
	if after.State.Reps != 0 || after.State.State != srs.StateNew {
		t.Fatalf("card not restored: reps=%d state=%v", after.State.Reps, after.State.State)
	}
	logs, _ := e.repo.ReviewLog(ctx, e.user, card.ID)
	if len(logs) != 0 {
		t.Fatalf("review log still has %d entries after undo", len(logs))
	}

	// With the history gone there is nothing left to undo.
	if _, err := e.repo.UndoLastReview(ctx, e.user); !errors.Is(err, cards.ErrNothingToUndo) {
		t.Fatalf("second undo = %v, want ErrNothingToUndo", err)
	}
}

func TestUndoIsScopedToTheUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	card := list[0]

	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(card.State, time.Now(), srs.RatingGood)
	if err := e.repo.ApplyReview(ctx, e.user, card.ID, card.State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	if _, err := e.repo.UndoLastReview(ctx, e.other); !errors.Is(err, cards.ErrNothingToUndo) {
		t.Fatalf("another user undid a review: %v", err)
	}
}

func TestApplyReviewRefusesADuplicateSubmit(t *testing.T) {
	// A retry or a second tab submitting the same answer twice must not
	// double-count the review.
	e := newEnv(t)
	ctx := context.Background()
	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	card := list[0]

	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(card.State, time.Now(), srs.RatingGood)
	if err := e.repo.ApplyReview(ctx, e.user, card.ID, card.State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}
	if err := e.repo.ApplyReview(ctx, e.user, card.ID, card.State, res.Card, res.Log); !errors.Is(err, cards.ErrStaleReview) {
		t.Fatalf("duplicate ApplyReview = %v, want ErrStaleReview", err)
	}
	logs, _ := e.repo.ReviewLog(ctx, e.user, card.ID)
	if len(logs) != 1 {
		t.Fatalf("duplicate submit left %d log entries, want 1", len(logs))
	}
}

func TestSetFlagStoresAndClearsTheReason(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	id := list[0].ID

	if err := e.repo.SetFlag(ctx, e.user, id, true, "answer feels ambiguous"); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}
	c, _ := e.repo.Get(ctx, e.user, id)
	if !c.Flagged || c.FlagReason != "answer feels ambiguous" {
		t.Fatalf("after flagging: flagged=%v reason=%q", c.Flagged, c.FlagReason)
	}

	// Unflagging clears the reason so a later flag starts blank.
	if err := e.repo.SetFlag(ctx, e.user, id, false, "ignored"); err != nil {
		t.Fatalf("SetFlag off: %v", err)
	}
	c, _ = e.repo.Get(ctx, e.user, id)
	if c.Flagged || c.FlagReason != "" {
		t.Fatalf("after unflagging: flagged=%v reason=%q", c.Flagged, c.FlagReason)
	}

	if err := e.repo.SetFlag(ctx, e.other, id, true, "not mine"); !errors.Is(err, cards.ErrNotFound) {
		t.Fatalf("flagging another user's card: err=%v, want ErrNotFound", err)
	}
}

func TestBuriedCardsSitOutTheQueueUntilTheirTime(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	id := list[0].ID
	until := now.Add(24 * time.Hour)

	if err := e.repo.SetBuriedUntil(ctx, e.user, id, until); err != nil {
		t.Fatalf("SetBuriedUntil: %v", err)
	}

	if due, _ := e.repo.Due(ctx, e.user, e.deck, at(now), 10); len(due) != 0 {
		t.Fatalf("buried card still in the queue")
	}
	if counts, _ := e.repo.Counts(ctx, e.user, e.deck, at(now)); counts.Total() != 0 {
		t.Fatalf("buried card still counted: %+v", counts)
	}
	if _, err := e.repo.NextDeckToStudy(ctx, e.user, at(now)); !errors.Is(err, cards.ErrNotFound) {
		t.Fatalf("deck with only a buried card still offered for study: %v", err)
	}

	// Once the burial lapses the card is simply back — nothing has to unbury it.
	later := until.Add(time.Minute)
	if due, _ := e.repo.Due(ctx, e.user, e.deck, at(later), 10); len(due) != 1 {
		t.Fatalf("card did not return after its burial lapsed")
	}

	c, _ := e.repo.Get(ctx, e.user, id)
	if !c.Buried(now) || c.Buried(later) {
		t.Fatalf("Buried() wrong: at now=%v at later=%v", c.Buried(now), c.Buried(later))
	}
}

func TestSetBuriedUntilZeroUnburies(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	now := time.Now()

	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	id := list[0].ID

	if err := e.repo.SetBuriedUntil(ctx, e.user, id, now.Add(48*time.Hour)); err != nil {
		t.Fatalf("SetBuriedUntil: %v", err)
	}
	if err := e.repo.SetBuriedUntil(ctx, e.user, id, time.Time{}); err != nil {
		t.Fatalf("unbury: %v", err)
	}
	if due, _ := e.repo.Due(ctx, e.user, e.deck, at(now), 10); len(due) != 1 {
		t.Fatalf("unburied card missing from the queue")
	}

	if err := e.repo.SetBuriedUntil(ctx, e.other, id, time.Time{}); !errors.Is(err, cards.ErrNotFound) {
		t.Fatalf("burying another user's card: err=%v, want ErrNotFound", err)
	}
}

func TestListFlaggedIsScopedToDeckAndOwner(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	list, _ := e.repo.ListForNote(ctx, e.user, e.note)
	if err := e.repo.SetFlag(ctx, e.user, list[0].ID, true, "check this"); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}

	flagged, err := e.repo.ListFlagged(ctx, e.user, e.deck)
	if err != nil {
		t.Fatalf("ListFlagged: %v", err)
	}
	if len(flagged) != 1 || flagged[0].FlagReason != "check this" {
		t.Fatalf("flagged = %+v, want the one flagged card with its reason", flagged)
	}

	if flagged, _ := e.repo.ListFlagged(ctx, e.other, e.deck); len(flagged) != 0 {
		t.Fatalf("another user saw flagged cards from this deck")
	}
	if flagged, _ := e.repo.ListFlagged(ctx, e.user, e.deck+999); len(flagged) != 0 {
		t.Fatalf("another deck saw this deck's flagged cards")
	}
}

func TestListForNotesGroupsCardsByNote(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	second, err := notes.NewRepository(e.db).Create(ctx, e.user, notes.CreateInput{
		DeckID: e.deck,
		Type:   notes.TypeBasic,
		Fields: []notes.Field{{Name: "front", Value: "grazie"}, {Name: "back", Value: "thanks"}},
	})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	got, err := e.repo.ListForNotes(ctx, e.user, []int64{e.note, second.ID})
	if err != nil {
		t.Fatalf("ListForNotes: %v", err)
	}
	if len(got) != 2 || len(got[e.note]) != 1 || len(got[second.ID]) != 1 {
		t.Fatalf("ListForNotes = %+v, want one card under each note", got)
	}

	if got, _ := e.repo.ListForNotes(ctx, e.other, []int64{e.note}); len(got) != 0 {
		t.Fatalf("another user saw cards through ListForNotes")
	}
	if got, _ := e.repo.ListForNotes(ctx, e.user, nil); len(got) != 0 {
		t.Fatalf("empty id list returned cards")
	}
}
