package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/danpicton/crapcard/internal/db"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// ── User repository ──────────────────────────────────────────────────────────

// UserRepo provides access to the users table.
type UserRepo struct {
	db *db.DB
}

// NewUserRepo creates a new UserRepo backed by the given database.
func NewUserRepo(database *db.DB) *UserRepo {
	return &UserRepo{db: database}
}

const userColumns = `id, username, password_hash, is_admin, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var isAdmin int
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &isAdmin, &u.CreatedAt); err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin != 0
	return u, nil
}

// Create inserts a new user and returns the populated User.
func (r *UserRepo) Create(ctx context.Context, username, passwordHash string, isAdmin bool) (*User, error) {
	isAdminInt := 0
	if isAdmin {
		isAdminInt = 1
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users(username, password_hash, is_admin) VALUES(?, ?, ?)`,
		username, passwordHash, isAdminInt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return r.FindByID(ctx, id)
}

// FindByUsername returns the user with the given username, or ErrNotFound.
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE username=?`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by username: %w", err)
	}
	return u, nil
}

// FindByID returns the user with the given id, or ErrNotFound.
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*User, error) {
	u, err := scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return u, nil
}

// SetPassword updates a user's stored password hash.
// Returns ErrNotFound if the user does not exist.
func (r *UserRepo) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns the total number of users. Used to detect first-run setup.
func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// Delete removes a user and all their associated data (cascaded via FK).
func (r *UserRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	return err
}

// ── Session repository ───────────────────────────────────────────────────────

// SessionRepo provides access to the sessions table.
type SessionRepo struct {
	db *db.DB
}

// NewSessionRepo creates a new SessionRepo backed by the given database.
func NewSessionRepo(database *db.DB) *SessionRepo {
	return &SessionRepo{db: database}
}

// Create generates a new session ID, inserts the session, and returns it.
func (r *SessionRepo) Create(ctx context.Context, userID int64, expiresAt time.Time) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions(id, user_id, expires_at) VALUES(?, ?, ?)`,
		id, userID, expiresAt.UTC(),
	); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return r.Find(ctx, id)
}

// Find returns the session with the given ID, or ErrNotFound.
func (r *SessionRepo) Find(ctx context.Context, id string) (*Session, error) {
	s := &Session{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, created_at FROM sessions WHERE id=?`, id,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}
	return s, nil
}

// Delete removes a session by ID (used on logout).
func (r *SessionRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id)
	return err
}

// DeleteForUser removes every session belonging to a user. Used to revoke
// access on security-sensitive events such as a password change.
func (r *SessionRepo) DeleteForUser(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}

// DeleteExpired removes all sessions whose expires_at is in the past.
func (r *SessionRepo) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	return err
}

// generateSessionID returns a cryptographically random 32-byte hex string.
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
