package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danpicton/crapcard/internal/auth"
)

// okHandler records whether the protected handler was reached and which user
// the middleware injected.
func okHandler(reached *bool, gotUser **auth.User) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		*gotUser = auth.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuthAllowsValidSession(t *testing.T) {
	svc, _ := testService(t)
	h := auth.NewHandler(svc)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	sess, _ := svc.Login(ctx, "dan", "hunter2hunter2")

	var reached bool
	var got *auth.User
	req := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.ID})
	rec := httptest.NewRecorder()

	h.RequireAuth(okHandler(&reached, &got)).ServeHTTP(rec, req)

	if !reached {
		t.Fatalf("protected handler was not reached")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got == nil || got.Username != "dan" {
		t.Fatalf("middleware injected user = %+v, want dan", got)
	}
}

func TestRequireAuthRejectsMissingCookie(t *testing.T) {
	svc, _ := testService(t)
	h := auth.NewHandler(svc)

	var reached bool
	var got *auth.User
	rec := httptest.NewRecorder()
	h.RequireAuth(okHandler(&reached, &got)).ServeHTTP(
		rec, httptest.NewRequest(http.MethodGet, "/api/decks", nil))

	if reached {
		t.Fatalf("unauthenticated request reached the protected handler")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuthRejectsUnknownAndExpiredSessions(t *testing.T) {
	svc, database := testService(t)
	h := auth.NewHandler(svc)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	u, _ := svc.UserByID(ctx, 1)
	expired, err := auth.NewSessionRepo(database).Create(ctx, u.ID, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	for name, value := range map[string]string{
		"unknown": "deadbeef",
		"expired": expired.ID,
	} {
		t.Run(name, func(t *testing.T) {
			var reached bool
			var got *auth.User
			req := httptest.NewRequest(http.MethodGet, "/api/decks", nil)
			req.AddCookie(&http.Cookie{Name: "session", Value: value})
			rec := httptest.NewRecorder()

			h.RequireAuth(okHandler(&reached, &got)).ServeHTTP(rec, req)

			if reached {
				t.Fatalf("%s session reached the protected handler", name)
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s session status = %d, want 401", name, rec.Code)
			}
		})
	}
}

func TestUserFromContextIsNilWhenUnset(t *testing.T) {
	if u := auth.UserFromContext(context.Background()); u != nil {
		t.Fatalf("UserFromContext on a bare context = %+v, want nil", u)
	}
}
