package decks_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/db"
	"github.com/danpicton/crapcard/internal/decks"
)

// testEnv gives each test a fresh database plus two users, so every test can
// assert that one user's decks are invisible to the other.
type testEnv struct {
	db    *db.DB
	repo  *decks.Repository
	user  int64
	other int64
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	users := auth.NewUserRepo(database)
	ctx := context.Background()
	u1, err := users.Create(ctx, "dan", "h", true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	u2, err := users.Create(ctx, "someone-else", "h", false)
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	return &testEnv{db: database, repo: decks.NewRepository(database), user: u1.ID, other: u2.ID}
}

func TestCreateAndGetDeck(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	created, err := env.repo.Create(ctx, env.user, "Italian", "verbs and vocab")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created deck has zero id")
	}
	if created.Name != "Italian" || created.Description != "verbs and vocab" {
		t.Fatalf("unexpected deck: %+v", created)
	}
	if created.UserID != env.user {
		t.Fatalf("deck user = %d, want %d", created.UserID, env.user)
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("created_at not populated")
	}

	got, err := env.repo.Get(ctx, env.user, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Italian" {
		t.Fatalf("Get returned %q", got.Name)
	}
}

func TestGetMissingDeckReturnsErrNotFound(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.repo.Get(context.Background(), env.user, 12345); !errors.Is(err, decks.ErrNotFound) {
		t.Fatalf("Get missing = %v, want ErrNotFound", err)
	}
}

func TestDeckIsScopedToItsOwner(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	mine, err := env.repo.Create(ctx, env.user, "Italian", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The other user must not be able to read, rename or delete it.
	if _, err := env.repo.Get(ctx, env.other, mine.ID); !errors.Is(err, decks.ErrNotFound) {
		t.Fatalf("cross-user Get = %v, want ErrNotFound", err)
	}
	if _, err := env.repo.Update(ctx, env.other, mine.ID, "Hijacked", ""); !errors.Is(err, decks.ErrNotFound) {
		t.Fatalf("cross-user Update = %v, want ErrNotFound", err)
	}
	if err := env.repo.Delete(ctx, env.other, mine.ID); !errors.Is(err, decks.ErrNotFound) {
		t.Fatalf("cross-user Delete = %v, want ErrNotFound", err)
	}

	// ...and it must still be intact afterwards.
	if _, err := env.repo.Get(ctx, env.user, mine.ID); err != nil {
		t.Fatalf("owner lost access to their own deck: %v", err)
	}
}

func TestDeckNameIsUniquePerUserNotGlobally(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.repo.Create(ctx, env.user, "Italian", ""); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := env.repo.Create(ctx, env.user, "Italian", ""); !errors.Is(err, decks.ErrDuplicateName) {
		t.Fatalf("duplicate name for same user = %v, want ErrDuplicateName", err)
	}
	// A different user may reuse the name.
	if _, err := env.repo.Create(ctx, env.other, "Italian", ""); err != nil {
		t.Fatalf("same name for a different user: %v", err)
	}
}

func TestListReturnsOnlyOwnDecksOrderedByName(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	for _, name := range []string{"Zoology", "Italian", "Anatomy"} {
		if _, err := env.repo.Create(ctx, env.user, name, ""); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}
	if _, err := env.repo.Create(ctx, env.other, "Not Mine", ""); err != nil {
		t.Fatalf("Create for other user: %v", err)
	}

	list, err := env.repo.List(ctx, env.user)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d decks, want 3", len(list))
	}
	want := []string{"Anatomy", "Italian", "Zoology"}
	for i, d := range list {
		if d.Name != want[i] {
			t.Fatalf("List[%d] = %q, want %q (expected name order)", i, d.Name, want[i])
		}
	}
}

func TestUpdateRenamesDeck(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	d, err := env.repo.Create(ctx, env.user, "Italian", "old")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := env.repo.Update(ctx, env.user, d.ID, "Italiano", "new")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "Italiano" || updated.Description != "new" {
		t.Fatalf("after update: %+v", updated)
	}

	got, _ := env.repo.Get(ctx, env.user, d.ID)
	if got.Name != "Italiano" {
		t.Fatalf("rename did not persist: %q", got.Name)
	}
}

func TestUpdateToAnExistingNameIsRejected(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	if _, err := env.repo.Create(ctx, env.user, "Italian", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := env.repo.Create(ctx, env.user, "Anatomy", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := env.repo.Update(ctx, env.user, second.ID, "Italian", ""); !errors.Is(err, decks.ErrDuplicateName) {
		t.Fatalf("rename onto an existing name = %v, want ErrDuplicateName", err)
	}
}

func TestDeleteRemovesDeck(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	d, err := env.repo.Create(ctx, env.user, "Italian", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := env.repo.Delete(ctx, env.user, d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := env.repo.Get(ctx, env.user, d.ID); !errors.Is(err, decks.ErrNotFound) {
		t.Fatalf("deck survived deletion: %v", err)
	}
	if err := env.repo.Delete(ctx, env.user, d.ID); !errors.Is(err, decks.ErrNotFound) {
		t.Fatalf("second Delete = %v, want ErrNotFound", err)
	}
}

func TestDecksCascadeOnUserDelete(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	d, err := env.repo.Create(ctx, env.user, "Italian", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := auth.NewUserRepo(env.db).Delete(ctx, env.user); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := env.repo.Get(ctx, env.user, d.ID); !errors.Is(err, decks.ErrNotFound) {
		t.Fatalf("deck survived its owner: %v", err)
	}
}

// itoa is a local helper so URL building in the handler tests stays readable.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }
