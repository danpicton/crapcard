package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func testService(t *testing.T) (*auth.Service, *db.DB) {
	t.Helper()
	database := testDB(t)
	svc := auth.NewService(auth.NewUserRepo(database), auth.NewSessionRepo(database))
	return svc, database
}

func TestSeedAdminCreatesFirstUserAndHashesPassword(t *testing.T) {
	svc, database := testService(t)
	ctx := context.Background()

	if err := svc.SeedAdmin(ctx, "dan", "correct horse battery"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	u, err := auth.NewUserRepo(database).FindByUsername(ctx, "dan")
	if err != nil {
		t.Fatalf("FindByUsername: %v", err)
	}
	if !u.IsAdmin {
		t.Fatalf("seeded user is not an admin")
	}
	if u.PasswordHash == "correct horse battery" {
		t.Fatalf("password was stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("correct horse battery")); err != nil {
		t.Fatalf("stored hash does not verify: %v", err)
	}
}

func TestSeedAdminIsNoOpWhenUsersExist(t *testing.T) {
	svc, database := testService(t)
	ctx := context.Background()

	if err := svc.SeedAdmin(ctx, "dan", "pw1"); err != nil {
		t.Fatalf("first SeedAdmin: %v", err)
	}
	if err := svc.SeedAdmin(ctx, "someone-else", "pw2"); err != nil {
		t.Fatalf("second SeedAdmin: %v", err)
	}

	n, _ := auth.NewUserRepo(database).Count(ctx)
	if n != 1 {
		t.Fatalf("user count = %d, want 1 (seed must not run twice)", n)
	}
}

func TestLoginWithValidCredentialsReturnsSession(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	s, err := svc.Login(ctx, "dan", "hunter2hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if s.ID == "" {
		t.Fatalf("login returned empty session id")
	}
	if !s.ExpiresAt.After(time.Now()) {
		t.Fatalf("session already expired at %v", s.ExpiresAt)
	}
}

func TestLoginRejectsWrongPasswordAndUnknownUser(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	if _, err := svc.Login(ctx, "dan", "wrong"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Login(ctx, "nobody", "hunter2hunter2"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("unknown user err = %v, want ErrInvalidCredentials", err)
	}
}

func TestValidateSessionReturnsUser(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	s, _ := svc.Login(ctx, "dan", "hunter2hunter2")

	u, err := svc.ValidateSession(ctx, s.ID)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if u.Username != "dan" {
		t.Fatalf("session resolved to %q", u.Username)
	}
}

func TestValidateSessionRejectsUnknownAndExpired(t *testing.T) {
	svc, database := testService(t)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	if _, err := svc.ValidateSession(ctx, "no-such-session"); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("unknown session err = %v, want ErrInvalidSession", err)
	}

	u, _ := auth.NewUserRepo(database).FindByUsername(ctx, "dan")
	expired, err := auth.NewSessionRepo(database).Create(ctx, u.ID, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if _, err := svc.ValidateSession(ctx, expired.ID); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("expired session err = %v, want ErrInvalidSession", err)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	s, _ := svc.Login(ctx, "dan", "hunter2hunter2")

	if err := svc.Logout(ctx, s.ID); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := svc.ValidateSession(ctx, s.ID); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("session still valid after logout: %v", err)
	}
}

func TestChangePasswordRevokesExistingSessions(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	s, _ := svc.Login(ctx, "dan", "hunter2hunter2")
	u, _ := svc.ValidateSession(ctx, s.ID)

	if err := svc.ChangePassword(ctx, u.ID, "a-brand-new-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if _, err := svc.ValidateSession(ctx, s.ID); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("old session survived a password change: %v", err)
	}
	if _, err := svc.Login(ctx, "dan", "a-brand-new-password"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	if _, err := svc.Login(ctx, "dan", "hunter2hunter2"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("old password still works after change")
	}
}

func TestNeedsSetupReportsWhetherAnyUserExists(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	needs, err := svc.NeedsSetup(ctx)
	if err != nil {
		t.Fatalf("NeedsSetup: %v", err)
	}
	if !needs {
		t.Fatalf("NeedsSetup = false on an empty database")
	}

	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	if needs, _ = svc.NeedsSetup(ctx); needs {
		t.Fatalf("NeedsSetup = true after a user exists")
	}
}
