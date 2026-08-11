// Package decks provides the deck a card belongs to, and the study unit the
// review queue is filtered by.
package decks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/danpicton/crapcard/internal/db"
)

// ErrNotFound is returned when a deck does not exist or belongs to another
// user. The two cases are deliberately indistinguishable so the API cannot be
// used to probe for other users' deck IDs.
var ErrNotFound = errors.New("deck not found")

// ErrDuplicateName is returned when a user already has a deck by that name.
var ErrDuplicateName = errors.New("a deck with that name already exists")

// Deck is a named collection of notes, and the unit a study session covers.
type Deck struct {
	ID          int64
	UserID      int64
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Repository provides access to the decks table.
type Repository struct {
	db *db.DB
}

// NewRepository creates a Repository backed by the given database.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

const deckColumns = `id, user_id, name, description, created_at, updated_at`

func scanDeck(row interface{ Scan(...any) error }) (*Deck, error) {
	d := &Deck{}
	if err := row.Scan(&d.ID, &d.UserID, &d.Name, &d.Description, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return nil, err
	}
	return d, nil
}

// isUniqueViolation reports whether err is SQLite's unique-constraint error
// for the deck name index.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// Create inserts a new deck for the user.
func (r *Repository) Create(ctx context.Context, userID int64, name, description string) (*Deck, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO decks(user_id, name, description) VALUES(?, ?, ?)`,
		userID, name, description,
	)
	if isUniqueViolation(err) {
		return nil, ErrDuplicateName
	}
	if err != nil {
		return nil, fmt.Errorf("create deck: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return r.Get(ctx, userID, id)
}

// Get returns the user's deck with the given id, or ErrNotFound.
func (r *Repository) Get(ctx context.Context, userID, id int64) (*Deck, error) {
	d, err := scanDeck(r.db.QueryRowContext(ctx,
		`SELECT `+deckColumns+` FROM decks WHERE id=? AND user_id=?`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get deck: %w", err)
	}
	return d, nil
}

// List returns all of the user's decks, ordered by name.
func (r *Repository) List(ctx context.Context, userID int64) ([]*Deck, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+deckColumns+` FROM decks WHERE user_id=? ORDER BY name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, fmt.Errorf("list decks: %w", err)
	}
	defer rows.Close()

	list := []*Deck{}
	for rows.Next() {
		d, err := scanDeck(rows)
		if err != nil {
			return nil, fmt.Errorf("scan deck: %w", err)
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

// Update renames a deck and replaces its description.
func (r *Repository) Update(ctx context.Context, userID, id int64, name, description string) (*Deck, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE decks SET name=?, description=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=? AND user_id=?`,
		name, description, id, userID,
	)
	if isUniqueViolation(err) {
		return nil, ErrDuplicateName
	}
	if err != nil {
		return nil, fmt.Errorf("update deck: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return r.Get(ctx, userID, id)
}

// Delete removes a deck. Its notes and cards go with it via ON DELETE CASCADE.
func (r *Repository) Delete(ctx context.Context, userID, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM decks WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return fmt.Errorf("delete deck: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
