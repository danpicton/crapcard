// Package db handles database connection and schema migrations.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"math/rand"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// DB is an alias for sql.DB so callers only need to import this package.
type DB = sql.DB

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Config holds database connection settings.
type Config struct {
	SQLitePath string
}

// Open opens a SQLite database connection, runs any pending migrations, and
// returns the ready-to-use *DB. The caller is responsible for calling Close().
func Open(cfg Config) (*DB, error) {
	path := cfg.SQLitePath
	if path == "" {
		path = "crapcard.db"
	}

	var dsn string
	if path == ":memory:" {
		// Each :memory: call gets a unique named DB so test runs don't share state.
		dsn = fmt.Sprintf("file:memdb%d?mode=memory&cache=shared&_foreign_keys=on", rand.Int63())
	} else {
		dsn = fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&cache=shared", path)
	}

	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite requires a single writer; cap connections to avoid locking issues.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close() //nolint:errcheck
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := runMigrations(sqlDB); err != nil {
		sqlDB.Close() //nolint:errcheck
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return sqlDB, nil
}

// runMigrations applies any *.up.sql files not yet recorded in
// schema_migrations, in filename order.
func runMigrations(database *sql.DB) error {
	if _, err := database.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT     PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, filename := range upFiles {
		version := strings.TrimSuffix(filename, ".up.sql")

		var exists int
		if err := database.QueryRow(
			"SELECT COUNT(*) FROM schema_migrations WHERE version=?", version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists > 0 {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + filename)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}

		tx, err := database.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", filename, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback() //nolint:errcheck
			return fmt.Errorf("exec migration %s: %w", filename, err)
		}

		if _, err := tx.Exec(
			"INSERT INTO schema_migrations(version) VALUES(?)", version,
		); err != nil {
			tx.Rollback() //nolint:errcheck
			return fmt.Errorf("record migration %s: %w", filename, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", filename, err)
		}
	}

	return nil
}
