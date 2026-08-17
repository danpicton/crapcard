package notes_test

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/danpicton/crapcard/internal/notes"
)

func occlusionConfig(mode string, ids ...int) notes.Config {
	rects := make([]notes.OcclusionRect, 0, len(ids))
	for i, id := range ids {
		rects = append(rects, notes.OcclusionRect{
			ID: id, X: 0.1 * float64(i), Y: 0.2, W: 0.15, H: 0.1,
		})
	}
	return notes.Config{Occlusion: &notes.Occlusion{Mode: mode, Rects: rects}}
}

const imageFront = "Name the organs\n\n![a cow](/api/images/abc123)"

func TestImageClozeGeneratesOneCardPerMask(t *testing.T) {
	gen, err := notes.GeneratorFor(notes.TypeImageCloze)
	if err != nil {
		t.Fatalf("GeneratorFor(image-cloze): %v", err)
	}

	specs, err := gen.Generate(fields("front", imageFront), occlusionConfig("hide-one", 1, 2, 5))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := []string{"occ:1", "occ:2", "occ:5"}
	if len(specs) != len(want) {
		t.Fatalf("got %d cards, want %d", len(specs), len(want))
	}
	for i := range want {
		if specs[i].Template != want[i] {
			t.Fatalf("template[%d] = %q, want %q", i, specs[i].Template, want[i])
		}
	}
}

func TestImageClozeValidation(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeImageCloze)

	cases := map[string]struct {
		fields []notes.Field
		cfg    notes.Config
	}{
		"no image in front":  {fields("front", "words only"), occlusionConfig("hide-one", 1)},
		"no masks":           {fields("front", imageFront), occlusionConfig("hide-one")},
		"no occlusion":       {fields("front", imageFront), notes.Config{}},
		"bad mode":           {fields("front", imageFront), occlusionConfig("sideways", 1)},
		"unknown field":      {fields("front", imageFront, "x", "y"), occlusionConfig("hide-one", 1)},
		"rect out of bounds": {fields("front", imageFront), notes.Config{Occlusion: &notes.Occlusion{Mode: "hide-one", Rects: []notes.OcclusionRect{{ID: 1, X: 0.9, Y: 0, W: 0.5, H: 0.1}}}}},
		"rect without size":  {fields("front", imageFront), notes.Config{Occlusion: &notes.Occlusion{Mode: "hide-one", Rects: []notes.OcclusionRect{{ID: 1, X: 0.1, Y: 0.1, W: 0, H: 0.1}}}}},
		"duplicate mask ids": {fields("front", imageFront), occlusionConfig("hide-one", 3, 3)},
	}
	for name, tc := range cases {
		if err := gen.Validate(tc.fields, tc.cfg); !errors.Is(err, notes.ErrInvalidNote) {
			t.Errorf("%s: Validate = %v, want ErrInvalidNote", name, err)
		}
	}

	if err := gen.Validate(fields("front", imageFront), occlusionConfig("hide-all", 1)); err != nil {
		t.Errorf("hide-all should validate, got %v", err)
	}
}

// occPayload mirrors the JSON the render embeds in the image URL fragment.
type occPayload struct {
	Mode  string                `json:"mode"`
	Side  string                `json:"side"`
	Test  int                   `json:"test"`
	Rects []notes.OcclusionRect `json:"rects"`
}

// fragmentPayload digs the #occ= payload out of the first image URL in the
// rendered markdown.
func fragmentPayload(t *testing.T, markdown string) occPayload {
	t.Helper()
	i := strings.Index(markdown, "#occ=")
	if i < 0 {
		t.Fatalf("no #occ= fragment in %q", markdown)
	}
	rest := markdown[i+len("#occ="):]
	if j := strings.IndexByte(rest, ')'); j >= 0 {
		rest = rest[:j]
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil {
		t.Fatalf("unescape fragment: %v", err)
	}
	var p occPayload
	if err := json.Unmarshal([]byte(decoded), &p); err != nil {
		t.Fatalf("decode fragment %q: %v", decoded, err)
	}
	return p
}

func TestImageClozeRenderEmbedsOcclusionFragment(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeImageCloze)
	cfg := occlusionConfig("hide-all", 1, 2)

	r, err := gen.Render(fields("front", imageFront), cfg, "occ:2")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	q := fragmentPayload(t, r.Question)
	if q.Side != "q" || q.Test != 2 || q.Mode != "hide-all" || len(q.Rects) != 2 {
		t.Errorf("question payload = %+v", q)
	}
	a := fragmentPayload(t, r.Answer)
	if a.Side != "a" || a.Test != 2 || len(a.Rects) != 2 {
		t.Errorf("answer payload = %+v", a)
	}

	// The rest of the markdown — surrounding text, image alt — is untouched.
	if !strings.HasPrefix(r.Question, "Name the organs\n\n![a cow](/api/images/abc123#occ=") {
		t.Errorf("question markdown mangled: %q", r.Question)
	}
}

func TestImageClozeRenderDefaultsToHideOne(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeImageCloze)
	cfg := occlusionConfig("", 1)

	r, err := gen.Render(fields("front", imageFront), cfg, "occ:1")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if p := fragmentPayload(t, r.Question); p.Mode != "hide-one" {
		t.Errorf("mode = %q, want hide-one", p.Mode)
	}
}

func TestImageClozeIgnoresBack(t *testing.T) {
	// Same rule as text cloze: the back is disabled while the note is a
	// cloze, so it must not appear on either side.
	gen, _ := notes.GeneratorFor(notes.TypeImageCloze)

	r, err := gen.Render(
		fields("front", imageFront, "back", "The rumen is the first stomach."),
		occlusionConfig("hide-one", 1), "occ:1",
	)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(r.Answer, "first stomach") {
		t.Errorf("answer leaks the disabled back: %q", r.Answer)
	}
	if strings.Contains(r.Question, "first stomach") {
		t.Errorf("question leaks the disabled back: %q", r.Question)
	}
}

func TestImageClozeRenderRejectsUnknownMask(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeImageCloze)

	for _, tmpl := range []string{"occ:9", "cloze:1", "forward", "occ:x"} {
		if _, err := gen.Render(fields("front", imageFront), occlusionConfig("hide-one", 1), tmpl); !errors.Is(err, notes.ErrInvalidNote) {
			t.Errorf("Render(%q) = %v, want ErrInvalidNote", tmpl, err)
		}
	}
}

func TestKnownTypesIncludesImageCloze(t *testing.T) {
	found := false
	for _, tt := range notes.KnownTypes() {
		if tt == notes.TypeImageCloze {
			found = true
		}
	}
	if !found {
		t.Fatalf("KnownTypes() = %v, missing image-cloze", notes.KnownTypes())
	}
}
