package notes_test

import (
	"errors"
	"testing"

	"github.com/danpicton/crapcard/internal/notes"
)

func TestClozeNoteGeneratesOneCardPerDeletionNumber(t *testing.T) {
	gen, err := notes.GeneratorFor(notes.TypeCloze)
	if err != nil {
		t.Fatalf("GeneratorFor(cloze): %v", err)
	}

	specs, err := gen.Generate(
		fields("front", "{{c1::Ottawa}} is the capital of {{c2::Canada}}. Founded {{c1::1855}}."),
		notes.Config{},
	)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := []string{}
	for _, s := range specs {
		got = append(got, s.Template)
	}
	want := []string{"cloze:1", "cloze:2"}
	if len(got) != len(want) {
		t.Fatalf("templates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("templates = %v, want %v", got, want)
		}
	}
}

func TestClozeNoteRequiresAtLeastOneDeletion(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeCloze)

	cases := map[string][]notes.Field{
		"no markers":  fields("front", "just text"),
		"empty front": fields("front", "  "),
		"no front":    fields("back", "extra only"),
	}
	for name, f := range cases {
		if err := gen.Validate(f, notes.Config{}); !errors.Is(err, notes.ErrInvalidNote) {
			t.Errorf("%s: Validate = %v, want ErrInvalidNote", name, err)
		}
	}
}

func TestClozeNoteBackIsOptional(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeCloze)
	if err := gen.Validate(fields("front", "{{c1::x}}"), notes.Config{}); err != nil {
		t.Fatalf("Validate without back: %v", err)
	}
}

func TestClozeNoteRejectsUnknownField(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeCloze)
	err := gen.Validate(fields("front", "{{c1::x}}", "sideways", "y"), notes.Config{})
	if !errors.Is(err, notes.ErrInvalidNote) {
		t.Fatalf("Validate = %v, want ErrInvalidNote", err)
	}
}

func TestClozeNoteRenderBlanksTestedDeletion(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeCloze)
	f := fields("front", "{{c1::Ottawa}} is in {{c2::Canada::country}}.")

	r, err := gen.Render(f, notes.Config{}, "cloze:2")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if r.Question != "Ottawa is in [country]." {
		t.Errorf("question = %q", r.Question)
	}
	if r.Answer != "Ottawa is in **Canada**." {
		t.Errorf("answer = %q", r.Answer)
	}
}

func TestClozeNoteIgnoresBack(t *testing.T) {
	// The back is disabled while a note is a cloze — the deletions are the
	// whole card. It stays stored (removing the last marker brings it back),
	// but it must not leak into either side.
	gen, _ := notes.GeneratorFor(notes.TypeCloze)
	f := fields("front", "{{c1::Ottawa}}", "back", "It sits on the Ottawa River.")

	r, err := gen.Render(f, notes.Config{}, "cloze:1")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if r.Question != "[...]" {
		t.Errorf("question = %q", r.Question)
	}
	if r.Answer != "**Ottawa**" {
		t.Errorf("answer = %q", r.Answer)
	}
}

func TestClozeNoteRenderRejectsUnknownTemplate(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeCloze)
	f := fields("front", "{{c1::x}}")

	for _, tmpl := range []string{"cloze:2", "forward", "cloze:x", "cloze:0"} {
		if _, err := gen.Render(f, notes.Config{}, tmpl); !errors.Is(err, notes.ErrInvalidNote) {
			t.Errorf("Render(%q) = %v, want ErrInvalidNote", tmpl, err)
		}
	}
}

func TestKnownTypesIncludesCloze(t *testing.T) {
	found := false
	for _, tt := range notes.KnownTypes() {
		if tt == notes.TypeCloze {
			found = true
		}
	}
	if !found {
		t.Fatalf("KnownTypes() = %v, missing cloze", notes.KnownTypes())
	}
}
