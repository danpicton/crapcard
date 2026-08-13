package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is deliberately above the library default: password hashing is a
// once-per-login cost, and the extra work factor is cheap insurance.
const bcryptCost = 12

// SessionTTL is how long a session remains valid after login.
const SessionTTL = 30 * 24 * time.Hour

// dummyPasswordHash is a syntactically valid cost-12 bcrypt hash, compared
// against when the username does not exist so that a missing user and a wrong
// password take the same wall-clock time. Without it, response latency alone
// reveals which usernames are registered.
var dummyPasswordHash = []byte("$2a$12$C6UzMDM.H6dfI/f/IKcEe.7RfBqTgQFvJ7EdOtDkqLzMFYFrPzKPS")

// ErrInvalidCredentials is returned when a username/password pair does not
// authenticate. It is deliberately indistinguishable between "no such user"
// and "wrong password".
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrInvalidSession is returned when a session ID is unknown or expired.
var ErrInvalidSession = errors.New("invalid session")

// Service holds the authentication use cases.
type Service struct {
	users    *UserRepo
	sessions *SessionRepo
}

// NewService creates a new auth Service.
func NewService(users *UserRepo, sessions *SessionRepo) *Service {
	return &Service{users: users, sessions: sessions}
}

// NeedsSetup reports whether the instance has no users yet, meaning the
// first-run setup flow should be offered.
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := s.users.Count(ctx)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// SeedAdmin creates the initial admin user. It is a no-op when any user
// already exists, so it is safe to call on every start-up.
func (s *Service) SeedAdmin(ctx context.Context, username, password string) error {
	needs, err := s.NeedsSetup(ctx)
	if err != nil {
		return err
	}
	if !needs {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if _, err := s.users.Create(ctx, username, string(hash), true); err != nil {
		return err
	}
	return nil
}

// Login verifies credentials and returns a new session on success.
func (s *Service) Login(ctx context.Context, username, password string) (*Session, error) {
	u, err := s.users.FindByUsername(ctx, username)
	if errors.Is(err, ErrNotFound) {
		// Burn the same bcrypt time as a real comparison would.
		bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password)) //nolint:errcheck
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.sessions.Create(ctx, u.ID, time.Now().Add(SessionTTL))
}

// Logout deletes the session, if it exists.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessions.Delete(ctx, sessionID)
}

// ValidateSession resolves a session ID to its user, rejecting unknown and
// expired sessions alike.
func (s *Service) ValidateSession(ctx context.Context, sessionID string) (*User, error) {
	sess, err := s.sessions.Find(ctx, sessionID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	if !time.Now().Before(sess.ExpiresAt) {
		return nil, ErrInvalidSession
	}

	u, err := s.users.FindByID(ctx, sess.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// UserByID returns a user by id.
func (s *Service) UserByID(ctx context.Context, id int64) (*User, error) {
	return s.users.FindByID(ctx, id)
}

// ChangePassword sets a new password and revokes every existing session for
// that user, so a stolen session cannot outlive the credential it came from.
func (s *Service) ChangePassword(ctx context.Context, userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.users.SetPassword(ctx, userID, string(hash)); err != nil {
		return err
	}
	return s.sessions.DeleteForUser(ctx, userID)
}
