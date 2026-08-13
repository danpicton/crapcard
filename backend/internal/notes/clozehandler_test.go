package notes_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danpicton/crapcard/internal/notes"
)

type noteResponse struct {
	ID        int64             `json:"id"`
	Type      string            `json:"type"`
	Fields    map[string]string `json:"fields"`
	Occlusion *notes.Occlusion  `json:"occlusion"`
	Cards     []struct {
		Template string `json:"template"`
	} `json:"cards"`
}

func decodeNote(t *testing.T, body []byte) noteResponse {
	t.Helper()
	var got noteResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode note: %v (%s)", err, body)
	}
	return got
}

func TestCreateClozeNoteEndpoint(t *testing.T) {
	e := newRepoEnv(t)

	body := `{
		"deck_id": ` + itoa(e.deck) + `,
		"type": "cloze",
		"fields": {"front": "{{c1::Ottawa}} is in {{c2::Canada}}.", "back": ""}
	}`
	rec := e.serve(t, e.user, http.MethodPost, "/api/notes", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	got := decodeNote(t, rec.Body.Bytes())
	if got.Type != "cloze" || len(got.Cards) != 2 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if got.Cards[0].Template != "cloze:1" || got.Cards[1].Template != "cloze:2" {
		t.Fatalf("cards = %+v", got.Cards)
	}
}

func TestUpdateSwitchesNoteTypeAndKeepsSurvivingCards(t *testing.T) {
	e := newRepoEnv(t)

	// Born basic…
	rec := e.serve(t, e.user, http.MethodPost, "/api/notes",
		`{"deck_id": `+itoa(e.deck)+`, "type": "basic", "fields": {"front": "plain", "back": "b"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := decodeNote(t, rec.Body.Bytes())

	// …edited into a cloze note, invisibly to the user.
	rec = e.serve(t, e.user, http.MethodPut, "/api/notes/"+itoa(created.ID),
		`{"deck_id": `+itoa(e.deck)+`, "type": "cloze", "fields": {"front": "{{c1::plain}}", "back": "b"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update to cloze: %d %s", rec.Code, rec.Body.String())
	}
	got := decodeNote(t, rec.Body.Bytes())
	if got.Type != "cloze" || len(got.Cards) != 1 || got.Cards[0].Template != "cloze:1" {
		t.Fatalf("after switch: %s", rec.Body.String())
	}

	// …and back to basic when the markers go away.
	rec = e.serve(t, e.user, http.MethodPut, "/api/notes/"+itoa(created.ID),
		`{"deck_id": `+itoa(e.deck)+`, "type": "basic", "fields": {"front": "plain again", "back": "b"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update to basic: %d %s", rec.Code, rec.Body.String())
	}
	got = decodeNote(t, rec.Body.Bytes())
	if got.Type != "basic" || len(got.Cards) != 1 || got.Cards[0].Template != "forward" {
		t.Fatalf("after switch back: %s", rec.Body.String())
	}
}

func TestUpdateWithoutTypeKeepsExistingType(t *testing.T) {
	e := newRepoEnv(t)

	rec := e.serve(t, e.user, http.MethodPost, "/api/notes",
		`{"deck_id": `+itoa(e.deck)+`, "type": "cloze", "fields": {"front": "{{c1::x}}", "back": ""}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := decodeNote(t, rec.Body.Bytes())

	// An old client that never sends type must not reset the note to basic.
	rec = e.serve(t, e.user, http.MethodPut, "/api/notes/"+itoa(created.ID),
		`{"deck_id": `+itoa(e.deck)+`, "fields": {"front": "{{c1::y}}", "back": ""}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if got := decodeNote(t, rec.Body.Bytes()); got.Type != "cloze" {
		t.Fatalf("type = %q, want cloze", got.Type)
	}
}

func TestCreateImageClozeNoteEndpointRoundTripsOcclusion(t *testing.T) {
	e := newRepoEnv(t)

	body := `{
		"deck_id": ` + itoa(e.deck) + `,
		"type": "image-cloze",
		"fields": {"front": "![cow](/api/images/abc)", "back": ""},
		"occlusion": {"mode": "hide-all", "rects": [
			{"id": 1, "x": 0.1, "y": 0.1, "w": 0.2, "h": 0.1},
			{"id": 2, "x": 0.5, "y": 0.5, "w": 0.2, "h": 0.1}
		]}
	}`
	rec := e.serve(t, e.user, http.MethodPost, "/api/notes", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	got := decodeNote(t, rec.Body.Bytes())
	if got.Type != "image-cloze" || len(got.Cards) != 2 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if got.Cards[0].Template != "occ:1" || got.Cards[1].Template != "occ:2" {
		t.Fatalf("cards = %+v", got.Cards)
	}
	if got.Occlusion == nil || got.Occlusion.Mode != "hide-all" || len(got.Occlusion.Rects) != 2 {
		t.Fatalf("occlusion did not round-trip: %s", rec.Body.String())
	}
}

func TestUpdateWithoutOcclusionKeepsExistingMasks(t *testing.T) {
	e := newRepoEnv(t)

	rec := e.serve(t, e.user, http.MethodPost, "/api/notes",
		`{"deck_id": `+itoa(e.deck)+`, "type": "image-cloze",
		  "fields": {"front": "![cow](/api/images/abc)", "back": ""},
		  "occlusion": {"mode": "hide-one", "rects": [{"id": 1, "x": 0.1, "y": 0.1, "w": 0.2, "h": 0.1}]}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := decodeNote(t, rec.Body.Bytes())

	rec = e.serve(t, e.user, http.MethodPut, "/api/notes/"+itoa(created.ID),
		`{"deck_id": `+itoa(e.deck)+`, "type": "image-cloze",
		  "fields": {"front": "![cow, again](/api/images/abc)", "back": ""}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	got := decodeNote(t, rec.Body.Bytes())
	if got.Occlusion == nil || len(got.Occlusion.Rects) != 1 {
		t.Fatalf("masks lost on an update that omitted occlusion: %s", rec.Body.String())
	}
}
