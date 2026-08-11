package db_test

import (
	"path/filepath"
	"testing"

	"github.com/danpicton/crapcard/internal/db"
)

func TestOpenInMemoryRunsMigrations(t *testing.T) {
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`,
	).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("schema_migrations table missing")
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crapcard.db")

	for i := range 2 {
		database, err := db.Open(db.Config{SQLitePath: path})
		if err != nil {
			t.Fatalf("Open #%d: %v", i+1, err)
		}
		database.Close()
	}
}

func TestMigrationsAreRecorded(t *testing.T) {
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	var applied int
	if err := database.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if applied == 0 {
		t.Fatalf("no migrations applied")
	}

	// The users table is the foundation every other table hangs off; if it is
	// missing, nothing downstream can have a user_id foreign key.
	var name string
	if err := database.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='users'`,
	).Scan(&name); err != nil {
		t.Fatalf("users table missing: %v", err)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	var fk int
	if err := database.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("pragma foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, want 1", fk)
	}
}
