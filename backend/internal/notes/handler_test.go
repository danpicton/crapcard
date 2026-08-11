package notes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/danpicton/crapcard/internal/auth"
	"github.com/danpicton/crapcard/internal/notes"
)

func (e *repoEnv) serve(t *testing.T, userID int64, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	notes.NewHandler(e.repo, e.cards).Register(mux, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := &auth.User{ID: userID, Username: "test"}
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), u)))
		})
	})

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestCreateNoteEndpoint(t *testing.T) {
	e := newRepoEnv(t)

	body := `{
		"deck_id": ` + itoa(e.deck) + `,
		"type": "basic",
		"reversed": true,
		"fields": {"front": "ciao", "back": "hello"}
	}`
	rec := e.serve(t, e.user, http.MethodPost, "/api/notes", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		ID       int64             `json:"id"`
		Type     string            `json:"type"`
		Reversed bool              `json:"reversed"`
		Fields   map[string]string `json:"fields"`
		Cards    []struct {
			ID       int64  `json:"id"`
			Template string `json:"template"`
		} `json:"cards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == 0 || got.Type != "basic" || !got.Reversed {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if got.Fields["front"] != "ciao" || got.Fields["back"] != "hello" {
		t.Fatalf("fields = %v", got.Fields)
	}
	if len(got.Cards) != 2 {
		t.Fatalf("got %d cards for a reversed note, want 2", len(got.Cards))
	}
}

func TestCreateNoteRejectsMissingFields(t *testing.T) {
	e := newRepoEnv(t)

	body := `{"deck_id": ` + itoa(e.deck) + `, "type": "basic", "fields": {"front": "ciao"}}`
	rec := e.serve(t, e.user, http.MethodPost, "/api/notes", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateNoteRejectsUnknownType(t *testing.T) {
	e := newRepoEnv(t)

	body := `{"deck_id": ` + itoa(e.deck) + `, "type": "cloze", "fields": {"text": "x"}}`
	rec := e.serve(t, e.user, http.MethodPost, "/api/notes", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a note type that is not implemented yet", rec.Code)
	}
}

func TestCreateNoteIntoAnotherUsersDeckIs404(t *testing.T) {
	e := newRepoEnv(t)

	body := `{"deck_id": ` + itoa(e.otherDeck) + `, "type": "basic", "fields": {"front": "a", "back": "b"}}`
	rec := e.serve(t, e.user, http.MethodPost, "/api/notes", body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestListNotesEndpointFiltersByDeck(t *testing.T) {
	e := newRepoEnv(t)
	ctx := context.Background()
	if _, err := e.repo.Create(ctx, e.user, basicInput(e.deck, "ciao", "hello", false)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := e.serve(t, e.user, http.MethodGet, "/api/notes?deck_id="+itoa(e.deck), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d notes, want 1", len(got))
	}
}

func TestListNotesReturnsEmptyArrayNotNull(t *testing.T) {
	e := newRepoEnv(t)

	rec := e.serve(t, e.user, http.MethodGet, "/api/notes", "")
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Fatalf("body = %s, want []", body)
	}
}

func TestGetNoteEndpointIncludesItsCards(t *testing.T) {
	e := newRepoEnv(t)
	n, err := e.repo.Create(context.Background(), e.user, basicInput(e.deck, "ciao", "hello", true))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := e.serve(t, e.user, http.MethodGet, "/api/notes/"+itoa(n.ID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got struct {
		Cards []struct {
			Template string `json:"template"`
			State    string `json:"state"`
		} `json:"cards"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Cards) != 2 {
		t.Fatalf("got %d cards, want 2", len(got.Cards))
	}
	if got.Cards[0].State != "new" {
		t.Fatalf("card state = %q, want new", got.Cards[0].State)
	}
}

func TestGetNoteOfAnotherUserIs404(t *testing.T) {
	e := newRepoEnv(t)
	n, err := e.repo.Create(context.Background(), e.other, notes.CreateInput{
		DeckID: e.otherDeck, Type: notes.TypeBasic,
		Fields: []notes.Field{{Name: "front", Value: "x"}, {Name: "back", Value: "y"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := e.serve(t, e.user, http.MethodGet, "/api/notes/"+itoa(n.ID), "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateNoteEndpoint(t *testing.T) {
	e := newRepoEnv(t)
	n, err := e.repo.Create(context.Background(), e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	body := `{
		"deck_id": ` + itoa(e.deck) + `,
		"reversed": true,
		"fields": {"front": "buongiorno", "back": "good morning"}
	}`
	rec := e.serve(t, e.user, http.MethodPut, "/api/notes/"+itoa(n.ID), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	got, _ := e.repo.Get(context.Background(), e.user, n.ID)
	if got.Fields[0].Value != "buongiorno" || !got.Config.Reversed {
		t.Fatalf("note = %+v", got)
	}
	list, _ := e.cards.ListForNote(context.Background(), e.user, n.ID)
	if len(list) != 2 {
		t.Fatalf("got %d cards after enabling reversal, want 2", len(list))
	}
}

func TestDeleteNoteEndpoint(t *testing.T) {
	e := newRepoEnv(t)
	n, err := e.repo.Create(context.Background(), e.user, basicInput(e.deck, "ciao", "hello", false))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := e.serve(t, e.user, http.MethodDelete, "/api/notes/"+itoa(n.ID), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if _, err := e.repo.Get(context.Background(), e.user, n.ID); err == nil {
		t.Fatalf("note survived DELETE")
	}
}

func TestNoteTypesEndpointAdvertisesWhatIsImplemented(t *testing.T) {
	// The client builds its editor from this, so it must list the fields each
	// type expects — and must not advertise cloze before it works.
	e := newRepoEnv(t)

	rec := e.serve(t, e.user, http.MethodGet, "/api/note-types", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got []struct {
		Type   string   `json:"type"`
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Type != "basic" {
		t.Fatalf("note types = %s, want just basic", rec.Body.String())
	}
	if len(got[0].Fields) != 2 || got[0].Fields[0] != "front" || got[0].Fields[1] != "back" {
		t.Fatalf("basic fields = %v", got[0].Fields)
	}
}
