package notes_test

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

type repoEnv struct {
	db        *db.DB
	repo      *notes.Repository
	cards     *cards.Repository
	user      int64
	other     int64
	deck      int64
	otherDeck int64
}

func newRepoEnv(t *testing.T) *repoEnv {
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

	deckRepo := decks.NewRepository(database)
	d, err := deckRepo.Create(ctx, u.ID, "Italian", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	od, err := deckRepo.Create(ctx, other.ID, "Theirs", "")
	if err != nil {
		t.Fatalf("create other deck: %v", err)
	}

	return &repoEnv{
		db: database, repo: notes.NewRepository(database), cards: cards.NewRepository(database),
		user: u.ID, other: other.ID, deck: d.ID, otherDeck: od.ID,
	}
}

func basicInput(deckID int64, front, back string, reversed bool) notes.CreateInput {
	return notes.CreateInput{
		DeckID: deckID,
		Type:   notes.TypeBasic,
		Config: notes.Config{Reversed: reversed},
		Fields: []notes.Field{{Name: "front", Value: front}, {Name: "back", Value: back}},
	}
}

func TestCreateStoresFieldsAndConfig(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", true))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.Type != notes.TypeBasic || !n.Config.Reversed {
		t.Fatalf("note = %+v", n)
	}
	if len(n.Fields) != 2 || n.Fields[0].Name != "front" || n.Fields[0].Value != "ciao" {
		t.Fatalf("fields = %+v", n.Fields)
	}

	got, err := e.repo.Get(ctx, e.user, n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Fields) != 2 || got.Fields[1].Value != "hello" {
		t.Fatalf("reloaded fields = %+v", got.Fields)
	}
}

func TestCreateRejectsInvalidContentWithoutWriting(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	if _, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "", false)); !errors.Is(err, notes.ErrInvalidNote) {
		t.Fatalf("Create with a blank back = %v, want ErrInvalidNote", err)
	}

	list, _ := e.repo.List(ctx, e.user, notes.ListFilter{})
	if len(list) != 0 {
		t.Fatalf("a rejected note was written anyway: %d notes", len(list))
	}
}

func TestCreateRejectsUnknownNoteType(t *testing.T) {
	e := newRepoEnv(t)

	in := basicInput(e.deck, "a", "b", false)
	in.Type = "sideways"
	if _, err := e.repo.Create(context.Background(), e.user, in); !errors.Is(err, notes.ErrUnknownNoteType) {
		t.Fatalf("Create with note type sideways = %v, want ErrUnknownNoteType", err)
	}
}

func TestCreateRefusesADeckTheUserDoesNotOwn(t *testing.T) {
	// The foreign key alone would happily attach this note to another user's
	// deck, which would then surface in their study queue.
	e := newRepoEnv(t)

	if _, err := e.repo.Create(context.Background(), e.user, basicInput(e.otherDeck, "a", "b", false)); !errors.Is(err, notes.ErrDeckNotFound) {
		t.Fatalf("Create into another user's deck = %v, want ErrDeckNotFound", err)
	}
}

func TestGetAndDeleteAreScopedToOwner(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := e.repo.Get(ctx, e.other, n.ID); !errors.Is(err, notes.ErrNotFound) {
		t.Fatalf("cross-user Get = %v, want ErrNotFound", err)
	}
	if err := e.repo.Delete(ctx, e.other, n.ID); !errors.Is(err, notes.ErrNotFound) {
		t.Fatalf("cross-user Delete = %v, want ErrNotFound", err)
	}
	if _, err := e.repo.Get(ctx, e.user, n.ID); err != nil {
		t.Fatalf("owner lost their note: %v", err)
	}
}

func TestUpdateEditsFieldsWithoutRecreatingSurvivingCards(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, _ := e.cards.ListForNote(ctx, e.user, n.ID)

	updated, err := e.repo.Update(ctx, e.user, n.ID, notes.UpdateInput{
		DeckID: e.deck,
		Fields: []notes.Field{{Name: "front", Value: "buongiorno"}, {Name: "back", Value: "good morning"}},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Fields[0].Value != "buongiorno" {
		t.Fatalf("field not updated: %+v", updated.Fields)
	}

	after, _ := e.cards.ListForNote(ctx, e.user, n.ID)
	if len(after) != 1 || after[0].ID != before[0].ID {
		t.Fatalf("editing the wording recreated the card (%d -> %d)", before[0].ID, after[0].ID)
	}
}

func TestTurningReversalOnAndOffAddsAndRemovesTheReverseCard(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if list, _ := e.cards.ListForNote(ctx, e.user, n.ID); len(list) != 1 {
		t.Fatalf("got %d cards, want 1", len(list))
	}

	if _, err := e.repo.Update(ctx, e.user, n.ID, notes.UpdateInput{
		DeckID: e.deck,
		Config: notes.Config{Reversed: true},
		Fields: n.Fields,
	}); err != nil {
		t.Fatalf("Update on: %v", err)
	}
	if list, _ := e.cards.ListForNote(ctx, e.user, n.ID); len(list) != 2 {
		t.Fatalf("got %d cards after enabling reversal, want 2", len(list))
	}

	if _, err := e.repo.Update(ctx, e.user, n.ID, notes.UpdateInput{
		DeckID: e.deck,
		Config: notes.Config{Reversed: false},
		Fields: n.Fields,
	}); err != nil {
		t.Fatalf("Update off: %v", err)
	}
	list, _ := e.cards.ListForNote(ctx, e.user, n.ID)
	if len(list) != 1 || list[0].Template != notes.TemplateForward {
		t.Fatalf("after disabling reversal: %+v", list)
	}
}

func TestMovingANoteMovesItsCards(t *testing.T) {
	// Cards carry deck_id for the queue index, so a move that misses them
	// would leave the card being studied from the old deck forever.
	e := newRepoEnv(t)
	ctx := context.Background()

	target, err := decks.NewRepository(e.db).Create(ctx, e.user, "Anatomy", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", true))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := e.repo.Update(ctx, e.user, n.ID, notes.UpdateInput{
		DeckID: target.ID,
		Config: n.Config,
		Fields: n.Fields,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	list, _ := e.cards.ListForNote(ctx, e.user, n.ID)
	if len(list) != 2 {
		t.Fatalf("got %d cards, want 2", len(list))
	}
	for _, c := range list {
		if c.DeckID != target.ID {
			t.Fatalf("card %d still in deck %d, want %d", c.ID, c.DeckID, target.ID)
		}
	}
}

func TestUpdateRefusesToMoveIntoAnotherUsersDeck(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := e.repo.Update(ctx, e.user, n.ID, notes.UpdateInput{
		DeckID: e.otherDeck, Fields: n.Fields,
	}); !errors.Is(err, notes.ErrDeckNotFound) {
		t.Fatalf("move into another user's deck = %v, want ErrDeckNotFound", err)
	}

	got, _ := e.repo.Get(ctx, e.user, n.ID)
	if got.DeckID != e.deck {
		t.Fatalf("note moved anyway: deck %d", got.DeckID)
	}
}

func TestUpdateRejectingContentLeavesTheNoteUntouched(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := e.repo.Update(ctx, e.user, n.ID, notes.UpdateInput{
		DeckID: e.deck,
		Fields: []notes.Field{{Name: "front", Value: ""}, {Name: "back", Value: "hello"}},
	}); !errors.Is(err, notes.ErrInvalidNote) {
		t.Fatalf("Update with a blank front = %v, want ErrInvalidNote", err)
	}

	got, _ := e.repo.Get(ctx, e.user, n.ID)
	if got.Fields[0].Value != "ciao" {
		t.Fatalf("a rejected edit changed the stored note: %+v", got.Fields)
	}
}

func TestListFiltersByDeckAndOwner(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	second, err := decks.NewRepository(e.db).Create(ctx, e.user, "Anatomy", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	if _, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := e.repo.Create(ctx, e.user, basicInput(second.ID, "femur", "thigh bone", false)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := e.repo.Create(ctx, e.other, basicInput(e.otherDeck, "x", "y", false)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := e.repo.List(ctx, e.user, notes.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List returned %d notes, want the caller's 2", len(all))
	}

	filtered, _ := e.repo.List(ctx, e.user, notes.ListFilter{DeckID: second.ID})
	if len(filtered) != 1 || filtered[0].Fields[0].Value != "femur" {
		t.Fatalf("deck filter returned %+v", filtered)
	}
}

func TestDeletingADeckDeletesItsNotes(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := decks.NewRepository(e.db).Delete(ctx, e.user, e.deck); err != nil {
		t.Fatalf("delete deck: %v", err)
	}

	if _, err := e.repo.Get(ctx, e.user, n.ID); !errors.Is(err, notes.ErrNotFound) {
		t.Fatalf("note survived its deck: %v", err)
	}
}

func TestCountForDeck(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	if _, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	n, err := e.repo.CountForDeck(ctx, e.user, e.deck)
	if err != nil {
		t.Fatalf("CountForDeck: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestNoteReportsWhenItWasLastStudied(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Never studied: no timestamp rather than a zero time, so the UI can say
	// "never" instead of printing year 1.
	if n.LastStudied != nil {
		t.Fatalf("a brand new note reports LastStudied = %v", n.LastStudied)
	}

	list, _ := e.cards.ListForNote(ctx, e.user, n.ID)
	sched := srs.NewScheduler(srs.DefaultParams())
	reviewedAt := time.Now().UTC().Truncate(time.Second)
	res := sched.Review(list[0].State, reviewedAt, srs.RatingGood)
	if err := e.cards.ApplyReview(ctx, e.user, list[0].ID, list[0].State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	got, err := e.repo.Get(ctx, e.user, n.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LastStudied == nil {
		t.Fatalf("LastStudied is nil after a review")
	}
	if got.LastStudied.Sub(reviewedAt).Abs() > 2*time.Second {
		t.Fatalf("LastStudied = %v, want about %v", got.LastStudied, reviewedAt)
	}
}

func TestLastStudiedIsTheMostRecentOfANotesCards(t *testing.T) {
	// A bidirectional note has two cards; the note was last studied whenever
	// either of them was.
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", true))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, _ := e.cards.ListForNote(ctx, e.user, n.ID)
	sched := srs.NewScheduler(srs.DefaultParams())

	older := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	newer := time.Now().UTC().Truncate(time.Second)

	first := sched.Review(list[0].State, older, srs.RatingGood)
	if err := e.cards.ApplyReview(ctx, e.user, list[0].ID, list[0].State, first.Card, first.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}
	second := sched.Review(list[1].State, newer, srs.RatingGood)
	if err := e.cards.ApplyReview(ctx, e.user, list[1].ID, list[1].State, second.Card, second.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	got, _ := e.repo.Get(ctx, e.user, n.ID)
	if got.LastStudied == nil || got.LastStudied.Sub(newer).Abs() > 2*time.Second {
		t.Fatalf("LastStudied = %v, want the more recent review at %v", got.LastStudied, newer)
	}
}

func TestListCarriesLastStudied(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	n, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	list, _ := e.cards.ListForNote(ctx, e.user, n.ID)
	sched := srs.NewScheduler(srs.DefaultParams())
	res := sched.Review(list[0].State, time.Now(), srs.RatingGood)
	if err := e.cards.ApplyReview(ctx, e.user, list[0].ID, list[0].State, res.Card, res.Log); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	notesList, err := e.repo.List(ctx, e.user, notes.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(notesList) != 1 || notesList[0].LastStudied == nil {
		t.Fatalf("list did not carry LastStudied: %+v", notesList[0])
	}
}

func TestCountForPagination(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	second, err := decks.NewRepository(e.db).Create(ctx, e.user, "Anatomy", "")
	if err != nil {
		t.Fatalf("create deck: %v", err)
	}
	for range 3 {
		if _, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if _, err := e.repo.Create(ctx, e.user, basicInput(second.ID, "femur", "thigh", false)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	all, err := e.repo.Count(ctx, e.user, notes.ListFilter{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if all != 4 {
		t.Fatalf("Count = %d, want 4", all)
	}

	// The count must ignore limit/offset, or the last page would report
	// itself as the whole collection.
	paged, _ := e.repo.Count(ctx, e.user, notes.ListFilter{DeckID: e.deck, Limit: 2, Offset: 2})
	if paged != 3 {
		t.Fatalf("Count with a deck filter = %d, want 3", paged)
	}

	if other, _ := e.repo.Count(ctx, e.other, notes.ListFilter{}); other != 0 {
		t.Fatalf("Count for another user = %d, want 0", other)
	}
}

func TestListPaginates(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()

	for i := range 5 {
		if _, err := e.repo.Create(ctx, e.user,
			basicInput(e.deck, "front "+itoa(int64(i)), "back", false)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	first, err := e.repo.List(ctx, e.user, notes.ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first page has %d notes, want 2", len(first))
	}

	second, _ := e.repo.List(ctx, e.user, notes.ListFilter{Limit: 2, Offset: 2})
	if len(second) != 2 {
		t.Fatalf("second page has %d notes, want 2", len(second))
	}
	if first[0].ID == second[0].ID {
		t.Fatalf("offset did not advance the page")
	}

	last, _ := e.repo.List(ctx, e.user, notes.ListFilter{Limit: 2, Offset: 4})
	if len(last) != 1 {
		t.Fatalf("last page has %d notes, want 1", len(last))
	}
}
