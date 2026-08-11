package notes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/danpicton/crapcard/internal/cards"
	"github.com/danpicton/crapcard/internal/db"
)

// ErrNotFound is returned when a note does not exist or belongs to another
// user.
var ErrNotFound = errors.New("note not found")

// ErrDeckNotFound is returned when the target deck does not exist or belongs
// to another user.
var ErrDeckNotFound = errors.New("deck not found")

// Note is a stored note with its fields.
type Note struct {
	ID        int64
	UserID    int64
	DeckID    int64
	Type      NoteType
	Config    Config
	Fields    []Field
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateInput is the content needed to author a note.
type CreateInput struct {
	DeckID int64
	Type   NoteType
	Config Config
	Fields []Field
}

// UpdateInput is the content of an edit. DeckID may differ from the note's
// current deck, which moves it.
type UpdateInput struct {
	DeckID int64
	Config Config
	Fields []Field
}

// ListFilter narrows a note listing.
type ListFilter struct {
	DeckID int64 // 0 means every deck
	Limit  int
	Offset int
}

// Repository stores notes and keeps their generated cards in step.
type Repository struct {
	db    *db.DB
	cards *cards.Repository
}

// NewRepository creates a note Repository.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database, cards: cards.NewRepository(database)}
}

// Create validates the note against its type, stores it, and materialises its
// cards — all in one transaction, so a note can never exist without the cards
// it is supposed to generate.
func (r *Repository) Create(ctx context.Context, userID int64, in CreateInput) (*Note, error) {
	gen, err := GeneratorFor(in.Type)
	if err != nil {
		return nil, err
	}
	specs, err := gen.Generate(in.Fields, in.Config)
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin create tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := assertDeckBelongsTo(ctx, tx, userID, in.DeckID); err != nil {
		return nil, err
	}

	cfg, err := json.Marshal(in.Config)
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO notes(user_id, deck_id, note_type, config) VALUES(?, ?, ?, ?)`,
		userID, in.DeckID, string(in.Type), string(cfg),
	)
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}
	noteID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	if err := replaceFields(ctx, tx, noteID, in.Fields); err != nil {
		return nil, err
	}
	if err := r.cards.SyncForNote(ctx, tx, cards.SyncInput{
		NoteID: noteID, UserID: userID, DeckID: in.DeckID, Templates: templatesOf(specs),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit create: %w", err)
	}
	return r.Get(ctx, userID, noteID)
}

// Update replaces a note's fields, config and deck, then re-syncs its cards.
// Cards whose template survives keep their scheduling.
func (r *Repository) Update(ctx context.Context, userID, noteID int64, in UpdateInput) (*Note, error) {
	existing, err := r.Get(ctx, userID, noteID)
	if err != nil {
		return nil, err
	}

	gen, err := GeneratorFor(existing.Type)
	if err != nil {
		return nil, err
	}
	specs, err := gen.Generate(in.Fields, in.Config)
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin update tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := assertDeckBelongsTo(ctx, tx, userID, in.DeckID); err != nil {
		return nil, err
	}

	cfg, err := json.Marshal(in.Config)
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE notes SET deck_id=?, config=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id=? AND user_id=?`,
		in.DeckID, string(cfg), noteID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("update note: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}

	if err := replaceFields(ctx, tx, noteID, in.Fields); err != nil {
		return nil, err
	}
	if err := r.cards.SyncForNote(ctx, tx, cards.SyncInput{
		NoteID: noteID, UserID: userID, DeckID: in.DeckID, Templates: templatesOf(specs),
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update: %w", err)
	}
	return r.Get(ctx, userID, noteID)
}

// Get returns one note with its fields.
func (r *Repository) Get(ctx context.Context, userID, noteID int64) (*Note, error) {
	n := &Note{}
	var noteType, cfg string
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, deck_id, note_type, config, created_at, updated_at
		 FROM notes WHERE id=? AND user_id=?`, noteID, userID,
	).Scan(&n.ID, &n.UserID, &n.DeckID, &noteType, &cfg, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get note: %w", err)
	}
	n.Type = NoteType(noteType)
	if err := json.Unmarshal([]byte(cfg), &n.Config); err != nil {
		return nil, fmt.Errorf("decode note config: %w", err)
	}

	n.Fields, err = r.loadFields(ctx, n.ID)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// List returns the user's notes, newest first, optionally filtered to a deck.
func (r *Repository) List(ctx context.Context, userID int64, f ListFilter) ([]*Note, error) {
	query := `SELECT id, user_id, deck_id, note_type, config, created_at, updated_at
	          FROM notes WHERE user_id=?`
	args := []any{userID}
	if f.DeckID > 0 {
		query += ` AND deck_id=?`
		args = append(args, f.DeckID)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	if f.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, f.Limit, f.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()

	list := []*Note{}
	for rows.Next() {
		n := &Note{}
		var noteType, cfg string
		if err := rows.Scan(&n.ID, &n.UserID, &n.DeckID, &noteType, &cfg, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		n.Type = NoteType(noteType)
		if err := json.Unmarshal([]byte(cfg), &n.Config); err != nil {
			return nil, fmt.Errorf("decode note config: %w", err)
		}
		list = append(list, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, n := range list {
		if n.Fields, err = r.loadFields(ctx, n.ID); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// Delete removes a note; its fields, cards and review history cascade.
func (r *Repository) Delete(ctx context.Context, userID, noteID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM notes WHERE id=? AND user_id=?`, noteID, userID)
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountForDeck returns how many notes a deck holds.
func (r *Repository) CountForDeck(ctx context.Context, userID, deckID int64) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notes WHERE user_id=? AND deck_id=?`, userID, deckID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count notes: %w", err)
	}
	return n, nil
}

func (r *Repository) loadFields(ctx context.Context, noteID int64) ([]Field, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT name, value FROM note_fields WHERE note_id=? ORDER BY ord`, noteID)
	if err != nil {
		return nil, fmt.Errorf("load fields: %w", err)
	}
	defer rows.Close()

	out := []Field{}
	for rows.Next() {
		var f Field
		if err := rows.Scan(&f.Name, &f.Value); err != nil {
			return nil, fmt.Errorf("scan field: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// replaceFields rewrites a note's fields wholesale. Fields are small and
// always saved together, so a delete-then-insert is simpler than diffing and
// costs nothing at this size.
func replaceFields(ctx context.Context, tx *sql.Tx, noteID int64, fields []Field) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_fields WHERE note_id=?`, noteID); err != nil {
		return fmt.Errorf("clear fields: %w", err)
	}
	for i, f := range fields {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO note_fields(note_id, ord, name, value) VALUES(?, ?, ?, ?)`,
			noteID, i, f.Name, f.Value,
		); err != nil {
			return fmt.Errorf("insert field %q: %w", f.Name, err)
		}
	}
	return nil
}

// assertDeckBelongsTo rejects a note pointed at a deck the user does not own.
// Without this the foreign key alone would happily attach the note to someone
// else's deck.
func assertDeckBelongsTo(ctx context.Context, tx *sql.Tx, userID, deckID int64) error {
	var found int64
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM decks WHERE id=? AND user_id=?`, deckID, userID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDeckNotFound
	}
	if err != nil {
		return fmt.Errorf("check deck: %w", err)
	}
	return nil
}

func templatesOf(specs []CardSpec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.Template)
	}
	return out
}
