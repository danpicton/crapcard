package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danpicton/crapcard/internal/db"
)

// newTestServer builds the whole application against an in-memory database,
// so these tests exercise real routing, real middleware and real handlers.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	handler, err := newServer(database, Config{})
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	return handler
}

// client drives the API keeping hold of the session cookie, like a browser.
type client struct {
	t       *testing.T
	handler http.Handler
	cookie  *http.Cookie
}

func newClient(t *testing.T) *client {
	return &client{t: t, handler: newTestServer(t)}
}

func (c *client) do(method, target, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "session" && ck.Value != "" {
			c.cookie = ck
		}
	}
	return rec
}

// setupAndLogin completes first-run setup and leaves the client authenticated.
func (c *client) setupAndLogin() {
	c.t.Helper()
	if rec := c.do(http.MethodPost, "/api/auth/setup",
		`{"username":"dan","password":"hunter2hunter2"}`); rec.Code != http.StatusOK {
		c.t.Fatalf("setup failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := c.do(http.MethodPost, "/api/auth/login",
		`{"username":"dan","password":"hunter2hunter2"}`); rec.Code != http.StatusOK {
		c.t.Fatalf("login failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHealthzIsPublic(t *testing.T) {
	c := newClient(t)

	rec := c.do(http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestAPIRequiresAuthentication(t *testing.T) {
	// Every data route must be behind the session cookie. A regression here
	// would expose one user's cards to anyone.
	c := newClient(t)
	c.do(http.MethodPost, "/api/auth/setup", `{"username":"dan","password":"hunter2hunter2"}`)

	protected := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/decks"},
		{http.MethodPost, "/api/decks"},
		{http.MethodGet, "/api/decks/1"},
		{http.MethodPut, "/api/decks/1"},
		{http.MethodDelete, "/api/decks/1"},
		{http.MethodGet, "/api/notes"},
		{http.MethodPost, "/api/notes"},
		{http.MethodGet, "/api/notes/1"},
		{http.MethodPut, "/api/notes/1"},
		{http.MethodDelete, "/api/notes/1"},
		{http.MethodGet, "/api/note-types"},
		{http.MethodGet, "/api/decks/1/study/next"},
		{http.MethodGet, "/api/decks/1/study/counts"},
		{http.MethodPost, "/api/cards/1/answer"},
		{http.MethodPost, "/api/images"},
		{http.MethodGet, "/api/study/next"},
		{http.MethodGet, "/api/notes/1/preview"},
		{http.MethodGet, "/api/config"},
		{http.MethodGet, "/api/images/abc"},
		{http.MethodGet, "/api/auth/me"},
	}

	for _, p := range protected {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			req := httptest.NewRequest(p.method, p.path, strings.NewReader("{}"))
			rec := httptest.NewRecorder()
			c.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestSetupAndLoginFlow(t *testing.T) {
	c := newClient(t)

	rec := c.do(http.MethodGet, "/api/auth/setup-status", "")
	if !strings.Contains(rec.Body.String(), `"needs_setup":true`) {
		t.Fatalf("setup status = %s", rec.Body.String())
	}

	c.setupAndLogin()

	rec = c.do(http.MethodGet, "/api/auth/me", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"username":"dan"`) {
		t.Fatalf("me body = %s", rec.Body.String())
	}
}

// TestFullStudyJourney walks the whole product in one pass: sign up, make a
// deck, author a reversed note, study both of its cards, and see the queue
// empty out.
func TestFullStudyJourney(t *testing.T) {
	c := newClient(t)
	c.setupAndLogin()

	rec := c.do(http.MethodPost, "/api/decks", `{"name":"Italian","description":"verbs"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deck: %d %s", rec.Code, rec.Body.String())
	}
	var deck struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &deck); err != nil {
		t.Fatalf("decode deck: %v", err)
	}

	noteBody := `{
		"deck_id": ` + itoa(deck.ID) + `,
		"type": "basic",
		"reversed": true,
		"fields": {"front": "ciao", "back": "hello"}
	}`
	rec = c.do(http.MethodPost, "/api/notes", noteBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create note: %d %s", rec.Code, rec.Body.String())
	}

	rec = c.do(http.MethodGet, "/api/decks/"+itoa(deck.ID)+"/study/counts", "")
	if !strings.Contains(rec.Body.String(), `"new":2`) {
		t.Fatalf("counts = %s, want 2 new cards from a reversed note", rec.Body.String())
	}

	// Study both cards.
	asked := map[string]bool{}
	for i := range 2 {
		rec = c.do(http.MethodGet, "/api/decks/"+itoa(deck.ID)+"/study/next", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("study next #%d: %d %s", i+1, rec.Code, rec.Body.String())
		}
		var q struct {
			CardID   int64  `json:"card_id"`
			Question string `json:"question"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &q); err != nil {
			t.Fatalf("decode question: %v", err)
		}
		asked[q.Question] = true

		rec = c.do(http.MethodPost, "/api/cards/"+itoa(q.CardID)+"/answer", `{"rating":4}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("answer: %d %s", rec.Code, rec.Body.String())
		}
	}

	if !asked["ciao"] || !asked["hello"] {
		t.Fatalf("both directions should have been asked, got %v", asked)
	}

	// Nothing left today.
	rec = c.do(http.MethodGet, "/api/decks/"+itoa(deck.ID)+"/study/next", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("study next after finishing = %d, want 204", rec.Code)
	}
}

func TestImagePasteRoundTripThroughTheAPI(t *testing.T) {
	c := newClient(t)
	c.setupAndLogin()

	png := testPNG(t)
	req := httptest.NewRequest(http.MethodPost, "/api/images", bytes.NewReader(png))
	req.Header.Set("Content-Type", "image/png")
	req.AddCookie(c.cookie)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}

	var img struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &img); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = c.do(http.MethodGet, img.URL, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("fetch image: %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatalf("image bytes changed in transit")
	}
}

func TestSetupCannotBeReplayedOverTheAPI(t *testing.T) {
	c := newClient(t)
	c.setupAndLogin()

	rec := c.do(http.MethodPost, "/api/auth/setup", `{"username":"intruder","password":"hunter2hunter2"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}
}

func TestUnknownAPIRouteIs404NotTheSPA(t *testing.T) {
	// /api/* must never fall through to the single-page app shell, or a typo
	// in a fetch would surface as HTML parsed as JSON.
	c := newClient(t)
	c.setupAndLogin()

	rec := c.do(http.MethodGet, "/api/does-not-exist", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content type = %q, want JSON", ct)
	}
}

func TestSecurityHeadersArePresent(t *testing.T) {
	c := newClient(t)

	rec := c.do(http.MethodGet, "/healthz", "")
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestConfigEndpointReportsThePageSize(t *testing.T) {
	c := newClient(t)
	c.setupAndLogin()

	rec := c.do(http.MethodGet, "/api/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		PageSize int `json:"page_size"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PageSize != defaultPageSize {
		t.Fatalf("page_size = %d, want the default %d", got.PageSize, defaultPageSize)
	}
}

func TestConfiguredPageSizeReachesTheClient(t *testing.T) {
	database, err := db.Open(db.Config{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	handler, err := newServer(database, Config{PageSize: 25})
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	c := &client{t: t, handler: handler}
	c.setupAndLogin()

	rec := c.do(http.MethodGet, "/api/config", "")
	var got struct {
		PageSize int `json:"page_size"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PageSize != 25 {
		t.Fatalf("page_size = %d, want 25", got.PageSize)
	}
}

func TestLandingCardEndpointIsReachable(t *testing.T) {
	// Signing in should put a card in front of you; this is the route the
	// landing screen calls.
	c := newClient(t)
	c.setupAndLogin()

	rec := c.do(http.MethodPost, "/api/decks", `{"name":"Italian"}`)
	var deck struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &deck); err != nil {
		t.Fatalf("decode deck: %v", err)
	}
	c.do(http.MethodPost, "/api/notes", `{"deck_id":`+itoa(deck.ID)+
		`,"type":"basic","fields":{"front":"ciao","back":"hello"}}`)

	rec = c.do(http.MethodGet, "/api/study/next", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"deck_name":"Italian"`) {
		t.Fatalf("body = %s, want the deck name", rec.Body.String())
	}
}
