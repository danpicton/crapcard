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

// ErrStaleReview is returned when an answer arrives for a card whose state
// has moved on since the client read it — a double submit or a second tab.
// Applying it anyway would double-count the review and corrupt the history a
// future FSRS optimiser trains on.
var ErrStaleReview = errors.New("card was already answered")

// ErrNothingToUndo is returned when the user has no undoable review — none at
// all, or the latest predates snapshot support.
var ErrNothingToUndo = errors.New("nothing to undo")

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

// Horizon carries the two instants the queue queries gate on.
//
// Now is the exact moment, used for learning steps (a card answered Again
// really should wait its few minutes) and for new cards. ReviewDueBefore is
// the end of the user's local calendar day: a review card due at 14:00 counts
// as due at the 09:00 study session, because day-scale scheduling is only
// meaningful at day granularity — gating on the exact timestamp would make
// every interval silently drift toward the user's latest-ever study time.
type Horizon struct {
	Now             time.Time
	ReviewDueBefore time.Time
}

// maxTZOffsetMinutes bounds a client-supplied UTC offset to the range real
// timezones occupy (UTC-12 to UTC+14).
const maxTZOffsetMinutes = 14 * 60

// HorizonAt builds the study horizon for a user whose local clock is
// offsetMinutes east of UTC (what JavaScript's -getTimezoneOffset() reports).
// An out-of-range offset is treated as UTC rather than rejected — the worst a
// forged value can do is shift the caller's own day boundary.
func HorizonAt(now time.Time, offsetMinutes int) Horizon {
	if offsetMinutes < -maxTZOffsetMinutes || offsetMinutes > maxTZOffsetMinutes {
		offsetMinutes = 0
	}
	offset := time.Duration(offsetMinutes) * time.Minute
	local := now.UTC().Add(offset)
	y, m, d := local.Date()
	nextLocalMidnight := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return Horizon{Now: now, ReviewDueBefore: nextLocalMidnight.Add(-offset)}
}

// dueSQL builds the clause gating a card on the horizon: review cards on the
// day boundary, everything else on the exact moment. prefix qualifies the
// column names for queries that alias the table. Bind dueClauseArgs alongside.
func dueSQL(prefix string) string {
	return fmt.Sprintf(`((%[1]sstate=%[2]d AND %[1]sdue<?) OR (%[1]sstate<>%[2]d AND %[1]sdue<=?))`,
		prefix, srs.StateReview)
}

var dueClause = dueSQL("")

func (h Horizon) dueClauseArgs() []any {
	return []any{h.ReviewDueBefore.UTC(), h.Now.UTC()}
}

// Due returns cards from the deck that are ready to study, capped at limit.
//
// Cards mid-flight — learning, relearning and due reviews — come first,
// oldest due date first; cards never seen (state new, zero-time due) come
// last. Without that split, a batch of freshly authored cards would starve
// every review that FSRS actually scheduled for today.
func (r *Repository) Due(ctx context.Context, userID, deckID int64, h Horizon, limit int) ([]*Card, error) {
	if limit <= 0 {
		return []*Card{}, nil
	}

	args := append([]any{userID, deckID}, h.dueClauseArgs()...)
	args = append(args, int(srs.StateNew), limit)
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cardColumns+` FROM cards
		 WHERE user_id=? AND deck_id=? AND suspended=0 AND `+dueClause+`
		 ORDER BY CASE WHEN state=? THEN 1 ELSE 0 END, due, id
		 LIMIT ?`,
		args...)
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

// NextDeckToStudy picks which deck to open a study session in when the user
// has not chosen one — landing them straight on a card after signing in.
//
// Only decks with something actually due are considered, and among those the
// one studied most recently wins: resuming where you left off is the least
// surprising choice. A deck nobody has touched sorts last (its MAX is NULL,
// which SQLite orders last under DESC) so an in-progress deck is preferred
// over a fresh one.
//
// Returns ErrNotFound when nothing is due anywhere.
func (r *Repository) NextDeckToStudy(ctx context.Context, userID int64, h Horizon) (int64, error) {
	// Recency is measured across the deck's whole history, not just the cards
	// still due: answering a card pushes it out of the due set, so ranking on
	// the due cards alone would make the deck you just studied look untouched.
	var deckID int64
	args := append([]any{userID, userID}, h.dueClauseArgs()...)
	err := r.db.QueryRowContext(ctx,
		`SELECT ranked.deck_id
		 FROM (
		   SELECT deck_id, MAX(last_review) AS recency
		   FROM cards
		   WHERE user_id=?
		   GROUP BY deck_id
		 ) AS ranked
		 WHERE EXISTS (
		   SELECT 1 FROM cards c
		   WHERE c.user_id=? AND c.deck_id=ranked.deck_id AND c.suspended=0 AND `+dueSQL("c.")+`
		 )
		 ORDER BY ranked.recency DESC, ranked.deck_id
		 LIMIT 1`,
		args...,
	).Scan(&deckID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("next deck to study: %w", err)
	}
	return deckID, nil
}

// Counts summarises the deck's queue at `now`, splitting waiting cards by
// whether they have never been seen, are mid-learning, or have come back
// round for review.
//
// New and Due are gated on the due date, but Learning is not. A card answered
// Again is due a few minutes later, and gating it would empty the counts the
// instant the user answered — the card is still in flight for this session,
// so it keeps being counted until it graduates.
func (r *Repository) Counts(ctx context.Context, userID, deckID int64, h Horizon) (Counts, error) {
	var c Counts
	err := r.db.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(CASE WHEN state=? AND due<=? THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN state IN (?, ?)        THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN state=? AND due<? THEN 1 ELSE 0 END), 0)
		 FROM cards
		 WHERE user_id=? AND deck_id=? AND suspended=0`,
		int(srs.StateNew), h.Now.UTC(),
		int(srs.StateLearning), int(srs.StateRelearning),
		int(srs.StateReview), h.ReviewDueBefore.UTC(),
		userID, deckID,
	).Scan(&c.New, &c.Learning, &c.Due)
	if err != nil {
		return c, fmt.Errorf("queue counts: %w", err)
	}
	return c, nil
}

// nullableTime renders a time for a nullable DATETIME column: the zero time
// becomes NULL rather than year 1.
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

// ApplyReview stores the card's new scheduling state and appends the answer
// to the review log — which also snapshots the state the card is leaving, so
// the review can be undone. Everything happens in one transaction, and the
// card must belong to the user.
//
// prev is the state the caller computed the review from; the update is
// guarded on its rep count so a duplicate submit (a retry, a second tab)
// returns ErrStaleReview instead of double-counting the answer.
func (r *Repository) ApplyReview(ctx context.Context, userID, cardID int64, prev, state srs.CardState, entry srs.ReviewLog) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin review tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.ExecContext(ctx,
		`UPDATE cards SET
		   due=?, stability=?, difficulty=?, elapsed_days=?, scheduled_days=?,
		   reps=?, lapses=?, state=?, last_review=?
		 WHERE id=? AND user_id=? AND reps=?`,
		state.Due.UTC(), state.Stability, state.Difficulty,
		state.ElapsedDays, state.ScheduledDays,
		state.Reps, state.Lapses, int(state.State), nullableTime(state.LastReview),
		cardID, userID, prev.Reps,
	)
	if err != nil {
		return fmt.Errorf("update card state: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Missing card and stale state both land here; look again to tell
		// the caller which it was.
		var exists int
		if err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM cards WHERE id=? AND user_id=?`, cardID, userID,
		).Scan(&exists); err == nil {
			return ErrStaleReview
		}
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO review_log(
		   card_id, user_id, rating, state, elapsed_days, scheduled_days, reviewed_at,
		   prev_due, prev_stability, prev_difficulty, prev_elapsed_days,
		   prev_scheduled_days, prev_reps, prev_lapses, prev_state, prev_last_review)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cardID, userID, int(entry.Rating), int(entry.State),
		entry.ElapsedDays, entry.ScheduledDays, entry.Review.UTC(),
		prev.Due.UTC(), prev.Stability, prev.Difficulty, prev.ElapsedDays,
		prev.ScheduledDays, prev.Reps, prev.Lapses, int(prev.State),
		nullableTime(prev.LastReview),
	); err != nil {
		return fmt.Errorf("insert review log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit review: %w", err)
	}
	return nil
}

// UndoLastReview reverts the user's most recent answer: the card goes back to
// exactly the state the review log snapshotted, and the log entry disappears
// as if the answer had never been given. Returns the card's id.
//
// Only the single latest review (across all the user's decks) is undoable —
// this is the study screen's "oops, wrong button", not history editing.
func (r *Repository) UndoLastReview(ctx context.Context, userID int64) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin undo tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var (
		logID, cardID int64
		prevDue       sql.NullTime
		prev          srs.CardState
		prevState     sql.NullInt64
		prevLast      sql.NullTime
	)
	err = tx.QueryRowContext(ctx,
		`SELECT id, card_id, prev_due, prev_stability, prev_difficulty,
		        prev_elapsed_days, prev_scheduled_days, prev_reps, prev_lapses,
		        prev_state, prev_last_review
		 FROM review_log WHERE user_id=? ORDER BY id DESC LIMIT 1`,
		userID,
	).Scan(&logID, &cardID, &prevDue,
		&nullFloat{&prev.Stability}, &nullFloat{&prev.Difficulty},
		&nullUint{&prev.ElapsedDays}, &nullUint{&prev.ScheduledDays},
		&nullUint{&prev.Reps}, &nullUint{&prev.Lapses},
		&prevState, &prevLast)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNothingToUndo
	}
	if err != nil {
		return 0, fmt.Errorf("read last review: %w", err)
	}
	if !prevDue.Valid || !prevState.Valid {
		// Logged before snapshots existed; nothing safe to restore.
		return 0, ErrNothingToUndo
	}
	prev.Due = prevDue.Time
	prev.State = srs.State(prevState.Int64)
	if prevLast.Valid {
		prev.LastReview = prevLast.Time
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE cards SET
		   due=?, stability=?, difficulty=?, elapsed_days=?, scheduled_days=?,
		   reps=?, lapses=?, state=?, last_review=?
		 WHERE id=? AND user_id=?`,
		prev.Due.UTC(), prev.Stability, prev.Difficulty,
		prev.ElapsedDays, prev.ScheduledDays,
		prev.Reps, prev.Lapses, int(prev.State), nullableTime(prev.LastReview),
		cardID, userID,
	); err != nil {
		return 0, fmt.Errorf("restore card state: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM review_log WHERE id=?`, logID,
	); err != nil {
		return 0, fmt.Errorf("delete review log entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit undo: %w", err)
	}
	return cardID, nil
}

// nullFloat and nullUint scan nullable numeric columns straight into the
// snapshot's fields, reading NULL as zero.
type nullFloat struct{ v *float64 }

func (n *nullFloat) Scan(src any) error {
	var f sql.NullFloat64
	if err := f.Scan(src); err != nil {
		return err
	}
	*n.v = f.Float64
	return nil
}

type nullUint struct{ v *uint64 }

func (n *nullUint) Scan(src any) error {
	var i sql.NullInt64
	if err := i.Scan(src); err != nil {
		return err
	}
	*n.v = uint64(max(i.Int64, 0))
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
