package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestUserRepoCreateAndFind(t *testing.T) {
	repo := auth.NewUserRepo(testDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, "dan", "hash123", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created user has zero ID")
	}
	if created.Username != "dan" || created.PasswordHash != "hash123" || !created.IsAdmin {
		t.Fatalf("unexpected user: %+v", created)
	}

	byName, err := repo.FindByUsername(ctx, "dan")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if byName.ID != created.ID {
		t.Fatalf("FindByUsername returned id %d, want %d", byName.ID, created.ID)
	}

	byID, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if byID.Username != "dan" {
		t.Fatalf("FindByID returned %q", byID.Username)
	}
}

func TestUserRepoFindMissingReturnsErrNotFound(t *testing.T) {
	repo := auth.NewUserRepo(testDB(t))
	ctx := context.Background()

	if _, err := repo.FindByUsername(ctx, "nobody"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("FindByUsername err = %v, want ErrNotFound", err)
	}
	if _, err := repo.FindByID(ctx, 999); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("FindByID err = %v, want ErrNotFound", err)
	}
}

func TestUserRepoUsernameIsUnique(t *testing.T) {
	repo := auth.NewUserRepo(testDB(t))
	ctx := context.Background()

	if _, err := repo.Create(ctx, "dan", "h", false); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := repo.Create(ctx, "dan", "h2", false); err == nil {
		t.Fatalf("duplicate username was accepted")
	}
}

func TestUserRepoCount(t *testing.T) {
	repo := auth.NewUserRepo(testDB(t))
	ctx := context.Background()

	n, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Fatalf("Count = %d on empty db, want 0", n)
	}

	if _, err := repo.Create(ctx, "dan", "h", true); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n, _ = repo.Count(ctx); n != 1 {
		t.Fatalf("Count = %d after one insert, want 1", n)
	}
}

func TestUserRepoSetPassword(t *testing.T) {
	repo := auth.NewUserRepo(testDB(t))
	ctx := context.Background()

	u, err := repo.Create(ctx, "dan", "old", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SetPassword(ctx, u.ID, "new"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}

	got, _ := repo.FindByID(ctx, u.ID)
	if got.PasswordHash != "new" {
		t.Fatalf("password hash = %q, want %q", got.PasswordHash, "new")
	}

	if err := repo.SetPassword(ctx, 999, "x"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("SetPassword on missing user = %v, want ErrNotFound", err)
	}
}

func TestSessionRepoCreateFindDelete(t *testing.T) {
	database := testDB(t)
	users := auth.NewUserRepo(database)
	sessions := auth.NewSessionRepo(database)
	ctx := context.Background()

	u, err := users.Create(ctx, "dan", "h", false)
	if err != nil {
		t.Fatalf("Create user: %v", err)
	}

	expires := time.Now().Add(time.Hour)
	s, err := sessions.Create(ctx, u.ID, expires)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	if len(s.ID) != 64 {
		t.Fatalf("session id length = %d, want 64 hex chars", len(s.ID))
	}
	if s.UserID != u.ID {
		t.Fatalf("session user = %d, want %d", s.UserID, u.ID)
	}

	found, err := sessions.Find(ctx, s.ID)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.ID != s.ID {
		t.Fatalf("Find returned %q, want %q", found.ID, s.ID)
	}

	if err := sessions.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := sessions.Find(ctx, s.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("Find after delete = %v, want ErrNotFound", err)
	}
}

func TestSessionRepoIDsAreUnpredictable(t *testing.T) {
	database := testDB(t)
	users := auth.NewUserRepo(database)
	sessions := auth.NewSessionRepo(database)
	ctx := context.Background()

	u, _ := users.Create(ctx, "dan", "h", false)
	seen := map[string]bool{}
	for range 20 {
		s, err := sessions.Create(ctx, u.ID, time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("Create session: %v", err)
		}
		if seen[s.ID] {
			t.Fatalf("duplicate session id %q", s.ID)
		}
		seen[s.ID] = true
	}
}

func TestSessionRepoDeleteExpired(t *testing.T) {
	database := testDB(t)
	users := auth.NewUserRepo(database)
	sessions := auth.NewSessionRepo(database)
	ctx := context.Background()

	u, _ := users.Create(ctx, "dan", "h", false)
	stale, _ := sessions.Create(ctx, u.ID, time.Now().Add(-time.Hour))
	live, _ := sessions.Create(ctx, u.ID, time.Now().Add(time.Hour))

	if err := sessions.DeleteExpired(ctx); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if _, err := sessions.Find(ctx, stale.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("expired session survived: %v", err)
	}
	if _, err := sessions.Find(ctx, live.ID); err != nil {
		t.Fatalf("live session was deleted: %v", err)
	}
}

func TestSessionsCascadeOnUserDelete(t *testing.T) {
	database := testDB(t)
	users := auth.NewUserRepo(database)
	sessions := auth.NewSessionRepo(database)
	ctx := context.Background()

	u, _ := users.Create(ctx, "dan", "h", false)
	s, _ := sessions.Create(ctx, u.ID, time.Now().Add(time.Hour))

	if err := users.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete user: %v", err)
	}
	if _, err := sessions.Find(ctx, s.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("session survived user deletion: %v", err)
	}
}
