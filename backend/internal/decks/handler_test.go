package decks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/decks"
)

// serve routes a request through a mux with the deck handler mounted and the
// given user already injected, standing in for RequireAuth.
func (env *testEnv) serve(t *testing.T, userID int64, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	decks.NewHandler(env.repo).Register(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := &auth.User{ID: userID, Username: "test"}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	})

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestCreateDeckEndpoint(t *testing.T) {
	env := newTestEnv(t)

	rec := env.serve(t, env.user, http.MethodPost, "/api/decks", `{"name":"Italian","description":"verbs"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == 0 || got.Name != "Italian" || got.Description != "verbs" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}

	if _, err := env.repo.Get(context.Background(), env.user, got.ID); err != nil {
		t.Fatalf("deck was not persisted: %v", err)
	}
}

func TestCreateDeckRejectsEmptyName(t *testing.T) {
	env := newTestEnv(t)

	rec := env.serve(t, env.user, http.MethodPost, "/api/decks", `{"name":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateDeckRejectsDuplicateNameWithConflict(t *testing.T) {
	env := newTestEnv(t)

	env.serve(t, env.user, http.MethodPost, "/api/decks", `{"name":"Italian"}`)
	rec := env.serve(t, env.user, http.MethodPost, "/api/decks", `{"name":"Italian"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestListDecksEndpointReturnsOnlyOwnDecks(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if _, err := env.repo.Create(ctx, env.user, "Mine", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := env.repo.Create(ctx, env.other, "Theirs", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := env.serve(t, env.user, http.MethodGet, "/api/decks", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Mine" {
		t.Fatalf("list = %s, want just the caller's deck", rec.Body.String())
	}
}

func TestListDecksReturnsEmptyArrayNotNull(t *testing.T) {
	env := newTestEnv(t)

	rec := env.serve(t, env.user, http.MethodGet, "/api/decks", "")
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Fatalf("empty list body = %s, want [] (null breaks .map() on the client)", body)
	}
}

func TestGetDeckEndpoint(t *testing.T) {
	env := newTestEnv(t)
	d, err := env.repo.Create(context.Background(), env.user, "Italian", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := env.serve(t, env.user, http.MethodGet, "/api/decks/"+itoa(d.ID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGetDeckOfAnotherUserIs404(t *testing.T) {
	env := newTestEnv(t)
	d, err := env.repo.Create(context.Background(), env.other, "Theirs", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := env.serve(t, env.user, http.MethodGet, "/api/decks/"+itoa(d.ID), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetDeckWithNonNumericIDIs404(t *testing.T) {
	env := newTestEnv(t)

	rec := env.serve(t, env.user, http.MethodGet, "/api/decks/not-a-number", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateDeckEndpoint(t *testing.T) {
	env := newTestEnv(t)
	d, err := env.repo.Create(context.Background(), env.user, "Italian", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := env.serve(t, env.user, http.MethodPut, "/api/decks/"+itoa(d.ID), `{"name":"Italiano","description":"x"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	got, _ := env.repo.Get(context.Background(), env.user, d.ID)
	if got.Name != "Italiano" {
		t.Fatalf("name = %q, want Italiano", got.Name)
	}
}

func TestDeleteDeckEndpoint(t *testing.T) {
	env := newTestEnv(t)
	d, err := env.repo.Create(context.Background(), env.user, "Italian", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := env.serve(t, env.user, http.MethodDelete, "/api/decks/"+itoa(d.ID), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if _, err := env.repo.Get(context.Background(), env.user, d.ID); err == nil {
		t.Fatalf("deck survived DELETE")
	}
}

func TestDeleteDeckOfAnotherUserIs404(t *testing.T) {
	env := newTestEnv(t)
	d, err := env.repo.Create(context.Background(), env.other, "Theirs", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := env.serve(t, env.user, http.MethodDelete, "/api/decks/"+itoa(d.ID), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if _, err := env.repo.Get(context.Background(), env.other, d.ID); err != nil {
		t.Fatalf("another user's deck was deleted: %v", err)
	}
}
