// Package cards owns the cards table: the scheduled items generated from
// notes, their FSRS memory state, and the review log.
//
// It is the only package that writes those tables. Notes drive card creation
// through SyncForNote; review drives state changes through ApplyReview.
package cards

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/danpicton/crapcard/internal/db"
	"github.com/danpicton/crapcard/internal/srs"
)

// ErrNotFound is returned when a card does not exist or belongs to another
// user.
var ErrNotFound = errors.New("card not found")

// Card is one scheduled item generated from a note.
type Card struct {
	ID        int64
	NoteID    int64
	UserID    int64
	DeckID    int64
	Template  string
	Suspended bool
	State     srs.CardState
	CreatedAt time.Time
}

// Counts summarises a deck's queue.
type Counts struct {
	New      int `json:"new"`
	Learning int `json:"learning"`
	Due      int `json:"due"`
}

// Total returns how many cards are waiting overall.
func (c Counts) Total() int { return c.New + c.Learning + c.Due }

// SyncInput describes the cards a note should have after an edit.
type SyncInput struct {
	NoteID    int64
	UserID    int64
	DeckID    int64
	Templates []string
}

// execer is satisfied by both *sql.DB and *sql.Tx, so every method can either
// run standalone or join a caller's transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Repository provides access to the cards and review_log tables.
type Repository struct {
	db *db.DB
}

// NewRepository creates a Repository backed by the given database.
func NewRepository(database *db.DB) *Repository {
	return &Repository{db: database}
}

// conn returns the transaction if one was supplied, else the pool.
func (r *Repository) conn(tx execer) execer {
	if tx == nil {
		return r.db
	}
	return tx
}

const cardColumns = `id, note_id, user_id, deck_id, template, suspended,
	due, stability, difficulty, elapsed_days, scheduled_days, reps, lapses, state, last_review,
	created_at`

func scanCard(row interface{ Scan(...any) error }) (*Card, error) {
	c := &Card{}
	var suspended int
	var lastReview sql.NullTime
	if err := row.Scan(
		&c.ID, &c.NoteID, &c.UserID, &c.DeckID, &c.Template, &suspended,
		&c.State.Due, &c.State.Stability, &c.State.Difficulty,
		&c.State.ElapsedDays, &c.State.ScheduledDays, &c.State.Reps, &c.State.Lapses,
		&c.State.State, &lastReview,
		&c.CreatedAt,
	); err != nil {
		return nil, err
	}
	c.Suspended = suspended != 0
	if lastReview.Valid {
		c.State.LastReview = lastReview.Time
	}
	return c, nil
}

// SyncForNote makes the note's cards match the given templates.
//
// Cards whose template is still present are left untouched — that is what
// stops an edit to a note's wording from resetting scheduling the user has
// already earned. Cards whose template has gone (reversal switched off, a
// cloze deletion removed) are deleted along with their review history.
//
// Pass a non-nil tx to run inside the caller's transaction.
func (r *Repository) SyncForNote(ctx context.Context, tx execer, in SyncInput) error {
	conn := r.conn(tx)

	wanted := make(map[string]bool, len(in.Templates))
	for _, tmpl := range in.Templates {
		wanted[tmpl] = true
	}

	rows, err := conn.QueryContext(ctx,
		`SELECT template FROM cards WHERE note_id=?`, in.NoteID)
	if err != nil {
		return fmt.Errorf("read existing templates: %w", err)
	}
	existing := map[string]bool{}
	for rows.Next() {
		var tmpl string
		if err := rows.Scan(&tmpl); err != nil {
			rows.Close() //nolint:errcheck
			return fmt.Errorf("scan template: %w", err)
		}
		existing[tmpl] = true
	}
	rows.Close() //nolint:errcheck
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read existing templates: %w", err)
	}

	// Insert the templates that are new. A brand new card is due at the zero
	// time so it sorts to the front of the queue.
	fresh := srs.NewCard()
	for _, tmpl := range in.Templates {
		if existing[tmpl] {
			continue
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO cards(note_id, user_id, deck_id, template, due, state)
			 VALUES(?, ?, ?, ?, ?, ?)`,
			in.NoteID, in.UserID, in.DeckID, tmpl, fresh.Due.UTC(), int(fresh.State),
		); err != nil {
			return fmt.Errorf("insert card %q: %w", tmpl, err)
		}
	}

	// Remove the ones no longer generated.
	for tmpl := range existing {
		if wanted[tmpl] {
			continue
		}
		if _, err := conn.ExecContext(ctx,
			`DELETE FROM cards WHERE note_id=? AND template=?`, in.NoteID, tmpl,
		); err != nil {
			return fmt.Errorf("delete card %q: %w", tmpl, err)
		}
	}

	// Keep the denormalised deck in step, for the case where the note moved.
	if _, err := conn.ExecContext(ctx,
		`UPDATE cards SET deck_id=? WHERE note_id=? AND deck_id<>?`,
		in.DeckID, in.NoteID, in.DeckID,
	); err != nil {
		return fmt.Errorf("update card deck: %w", err)
	}

	return nil
}

// ListForNote returns the user's cards for one note, in template order.
func (r *Repository) ListForNote(ctx context.Context, userID, noteID int64) ([]*Card, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cardColumns+` FROM cards WHERE note_id=? AND user_id=? ORDER BY template`,
		noteID, userID)
	if err != nil {
		return nil, fmt.Errorf("list cards for note: %w", err)
	}
	defer rows.Close()

	list := []*Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan card: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// Get returns one card by id, scoped to its owner.
func (r *Repository) Get(ctx context.Context, userID, cardID int64) (*Card, error) {
	c, err := scanCard(r.db.QueryRowContext(ctx,
		`SELECT `+cardColumns+` FROM cards WHERE id=? AND user_id=?`, cardID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get card: %w", err)
	}
	return c, nil
}

// Due returns cards from the deck that are ready to study at `now`, oldest
// due date first, capped at limit.
func (r *Repository) Due(ctx context.Context, userID, deckID int64, now time.Time, limit int) ([]*Card, error) {
	if limit <= 0 {
		return []*Card{}, nil
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cardColumns+` FROM cards
		 WHERE user_id=? AND deck_id=? AND suspended=0 AND due<=?
		 ORDER BY due, id
		 LIMIT ?`,
		userID, deckID, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("due cards: %w", err)
	}
	defer rows.Close()

	list := []*Card{}
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan card: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// Counts summarises the deck's queue at `now`, splitting waiting cards by
// whether they have never been seen, are mid-learning, or have come back
// round for review.
//
// New and Due are gated on the due date, but Learning is not. A card answered
// Again is due a few minutes later, and gating it would empty the counts the
// instant the user answered — the card is still in flight for this session,
// so it keeps being counted until it graduates.
func (r *Repository) Counts(ctx context.Context, userID, deckID int64, now time.Time) (Counts, error) {
	var c Counts
	err := r.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(CASE WHEN state=? AND due<=? THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN state IN (?, ?)        THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN state=? AND due<=? THEN 1 ELSE 0 END), 0)
		 FROM cards
		 WHERE user_id=? AND deck_id=? AND suspended=0`,
		int(srs.StateNew), now.UTC(),
		int(srs.StateLearning), int(srs.StateRelearning),
		int(srs.StateReview), now.UTC(),
		userID, deckID,
	).Scan(&c.New, &c.Learning, &c.Due)
	if err != nil {
		return c, fmt.Errorf("queue counts: %w", err)
	}
	return c, nil
}

// ApplyReview stores the card's new scheduling state and appends the answer
// to the review log, in one transaction. The card must belong to the user.
func (r *Repository) ApplyReview(ctx context.Context, userID, cardID int64, state srs.CardState, entry srs.ReviewLog) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin review tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var lastReview any
	if !state.LastReview.IsZero() {
		lastReview = state.LastReview.UTC()
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE cards SET
		   due=?, stability=?, difficulty=?, elapsed_days=?, scheduled_days=?,
		   reps=?, lapses=?, state=?, last_review=?
		 WHERE id=? AND user_id=?`,
		state.Due.UTC(), state.Stability, state.Difficulty,
		state.ElapsedDays, state.ScheduledDays,
		state.Reps, state.Lapses, int(state.State), lastReview,
		cardID, userID,
	)
	if err != nil {
		return fmt.Errorf("update card state: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO review_log(card_id, user_id, rating, state, elapsed_days, scheduled_days, reviewed_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		cardID, userID, int(entry.Rating), int(entry.State),
		entry.ElapsedDays, entry.ScheduledDays, entry.Review.UTC(),
	); err != nil {
		return fmt.Errorf("insert review log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit review: %w", err)
	}
	return nil
}

// ReviewLog returns a card's answer history, oldest first.
func (r *Repository) ReviewLog(ctx context.Context, userID, cardID int64) ([]srs.ReviewLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT rating, state, elapsed_days, scheduled_days, reviewed_at
		 FROM review_log WHERE card_id=? AND user_id=? ORDER BY reviewed_at, id`,
		cardID, userID)
	if err != nil {
		return nil, fmt.Errorf("review log: %w", err)
	}
	defer rows.Close()

	list := []srs.ReviewLog{}
	for rows.Next() {
		var e srs.ReviewLog
		var rating, state int
		if err := rows.Scan(&rating, &state, &e.ElapsedDays, &e.ScheduledDays, &e.Review); err != nil {
			return nil, fmt.Errorf("scan review log: %w", err)
		}
		e.Rating = srs.Rating(rating)
		e.State = srs.State(state)
		list = append(list, e)
	}
	return list, rows.Err()
}

// SetSuspended suspends or resumes a card, taking it out of or back into the
// study queue without losing its history.
func (r *Repository) SetSuspended(ctx context.Context, userID, cardID int64, suspended bool) error {
	v := 0
	if suspended {
		v = 1
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE cards SET suspended=? WHERE id=? AND user_id=?`, v, cardID, userID)
	if err != nil {
		return fmt.Errorf("set suspended: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
