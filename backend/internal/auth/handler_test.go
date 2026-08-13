package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danpicton/crapcard/internal/auth"
)

func testHandler(t *testing.T) (*auth.Handler, *auth.Service) {
	t.Helper()
	svc, _ := testService(t)
	return auth.NewHandler(svc), svc
}

func sessionCookie(t *testing.T, res *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	return nil
}

func TestLoginSetsHttpOnlySessionCookie(t *testing.T) {
	h, svc := testHandler(t)
	if err := svc.SeedAdmin(context.Background(), "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"dan","password":"hunter2hunter2"}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	c := sessionCookie(t, rec.Result())
	if c == nil {
		t.Fatalf("no session cookie set")
	}
	if !c.HttpOnly {
		t.Fatalf("session cookie is not HttpOnly — readable by scripts")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie SameSite = %v, want Strict", c.SameSite)
	}
	if c.Path != "/" {
		t.Fatalf("session cookie path = %q, want /", c.Path)
	}
	if c.Value == "" {
		t.Fatalf("session cookie has empty value")
	}
}

func TestLoginRejectsBadCredentialsWithoutCookie(t *testing.T) {
	h, svc := testHandler(t)
	if err := svc.SeedAdmin(context.Background(), "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"dan","password":"nope"}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if c := sessionCookie(t, rec.Result()); c != nil && c.Value != "" {
		t.Fatalf("session cookie issued on a failed login")
	}
}

func TestLoginThrottlesRepeatedFailures(t *testing.T) {
	// bcrypt alone is the only per-guess cost otherwise; an online dictionary
	// attack should hit a wall, and the wall should get further away.
	h, svc := testHandler(t)
	if err := svc.SeedAdmin(context.Background(), "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	now := time.Now()
	h.SetNowForTest(func() time.Time { return now })

	attempt := func(password string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"username":"dan","password":"`+password+`"}`))
		rec := httptest.NewRecorder()
		h.Login(rec, req)
		return rec
	}

	for range 6 {
		if rec := attempt("wrong-wrong"); rec.Code != http.StatusUnauthorized && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("failed attempt = %d", rec.Code)
		}
	}

	// Even the right password is refused while throttled — the throttle must
	// not be an oracle.
	rec := attempt("hunter2hunter2")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d during backoff, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatalf("429 without Retry-After")
	}

	// Once the backoff lapses, the real password works again.
	now = now.Add(10 * time.Minute)
	if rec := attempt("hunter2hunter2"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d after backoff lapsed, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSetupRejectsAPasswordBeyondBcryptsLimit(t *testing.T) {
	// bcrypt refuses >72 bytes; without this check the user gets an opaque
	// 500 instead of being told what is wrong.
	h, _ := testHandler(t)
	long := strings.Repeat("a", 73)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup",
		strings.NewReader(`{"username":"dan","password":"`+long+`"}`))
	rec := httptest.NewRecorder()
	h.Setup(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLoginRejectsMalformedBody(t *testing.T) {
	h, _ := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLoginCookieIsNotSecureOverPlainHTTP(t *testing.T) {
	// Browsers silently discard Secure cookies sent over plain HTTP, which
	// would make every session invalid the moment it was issued on a
	// non-TLS deployment.
	h, svc := testHandler(t)
	if err := svc.SeedAdmin(context.Background(), "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"dan","password":"hunter2hunter2"}`))
	rec := httptest.NewRecorder()
	h.Login(rec, req)

	c := sessionCookie(t, rec.Result())
	if c == nil {
		t.Fatalf("no session cookie set")
	}
	if c.Secure {
		t.Fatalf("Secure flag set on a plain-HTTP request")
	}
}

func TestLogoutClearsCookieAndSession(t *testing.T) {
	h, svc := testHandler(t)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	sess, _ := svc.Login(ctx, "dan", "hunter2hunter2")

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.ID})
	rec := httptest.NewRecorder()
	h.Logout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	c := sessionCookie(t, rec.Result())
	if c == nil || c.MaxAge >= 0 {
		t.Fatalf("logout did not expire the cookie: %+v", c)
	}
	if _, err := svc.ValidateSession(ctx, sess.ID); err == nil {
		t.Fatalf("session still valid after logout")
	}
}

func TestMeReturnsCurrentUser(t *testing.T) {
	h, svc := testHandler(t)
	ctx := context.Background()
	if err := svc.SeedAdmin(ctx, "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	u, _ := svc.UserByID(ctx, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req = req.WithContext(auth.WithUser(req.Context(), u))
	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["username"] != "dan" {
		t.Fatalf("username = %v, want dan", body["username"])
	}
	if _, leaked := body["password_hash"]; leaked {
		t.Fatalf("response leaked the password hash: %s", rec.Body.String())
	}
}

func TestSetupStatusReportsNeedsSetup(t *testing.T) {
	h, svc := testHandler(t)

	rec := httptest.NewRecorder()
	h.SetupStatus(rec, httptest.NewRequest(http.MethodGet, "/api/auth/setup-status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		NeedsSetup bool `json:"needs_setup"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.NeedsSetup {
		t.Fatalf("needs_setup = false on an empty instance")
	}

	if err := svc.SeedAdmin(context.Background(), "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("SeedAdmin: %v", err)
	}
	rec = httptest.NewRecorder()
	h.SetupStatus(rec, httptest.NewRequest(http.MethodGet, "/api/auth/setup-status", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.NeedsSetup {
		t.Fatalf("needs_setup = true after setup")
	}
}

func TestSetupCreatesFirstAdminThenRefuses(t *testing.T) {
	h, svc := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup",
		strings.NewReader(`{"username":"dan","password":"hunter2hunter2"}`))
	rec := httptest.NewRecorder()
	h.Setup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first setup status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := svc.Login(context.Background(), "dan", "hunter2hunter2"); err != nil {
		t.Fatalf("cannot log in after setup: %v", err)
	}

	// A second call must not let an anonymous caller add another admin.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/setup",
		strings.NewReader(`{"username":"intruder","password":"hunter2hunter2"}`))
	rec = httptest.NewRecorder()
	h.Setup(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d, want 409", rec.Code)
	}
	if _, err := svc.Login(context.Background(), "intruder", "hunter2hunter2"); err == nil {
		t.Fatalf("second setup created a user")
	}
}

func TestSetupRejectsShortPassword(t *testing.T) {
	h, _ := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup",
		strings.NewReader(`{"username":"dan","password":"short"}`))
	rec := httptest.NewRecorder()
	h.Setup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
