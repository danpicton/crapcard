package notes_test

import (
	"errors"
	"testing"

	"github.com/danpicton/crapcard/internal/notes"
)

func fields(pairs ...string) []notes.Field {
	out := make([]notes.Field, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, notes.Field{Name: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func TestBasicNoteGeneratesOneForwardCard(t *testing.T) {
	gen, err := notes.GeneratorFor(notes.TypeBasic)
	if err != nil {
		t.Fatalf("GeneratorFor(basic): %v", err)
	}

	specs, err := gen.Generate(fields("front", "ciao", "back", "hello"), notes.Config{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("got %d cards, want 1", len(specs))
	}
	if specs[0].Template != notes.TemplateForward {
		t.Fatalf("template = %q, want %q", specs[0].Template, notes.TemplateForward)
	}
}

func TestBasicNoteWithReversalGeneratesTwoCards(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeBasic)

	specs, err := gen.Generate(fields("front", "ciao", "back", "hello"), notes.Config{Reversed: true})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("got %d cards, want 2", len(specs))
	}

	got := []string{specs[0].Template, specs[1].Template}
	want := []string{notes.TemplateForward, notes.TemplateReverse}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("templates = %v, want %v", got, want)
		}
	}
}

func TestBasicNoteRequiresBothFields(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeBasic)

	cases := map[string][]notes.Field{
		"missing back":  fields("front", "ciao"),
		"missing front": fields("back", "hello"),
		"blank front":   fields("front", "   ", "back", "hello"),
		"blank back":    fields("front", "ciao", "back", ""),
		"no fields":     nil,
	}

	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if err := gen.Validate(f, notes.Config{}); !errors.Is(err, notes.ErrInvalidNote) {
				t.Fatalf("Validate = %v, want ErrInvalidNote", err)
			}
		})
	}
}

func TestBasicNoteAcceptsValidFields(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeBasic)

	if err := gen.Validate(fields("front", "ciao", "back", "hello"), notes.Config{}); err != nil {
		t.Fatalf("Validate on a well-formed note: %v", err)
	}
}

func TestBasicNoteRejectsUnknownFields(t *testing.T) {
	// An unknown field is a client bug or a stale schema; silently dropping it
	// would lose the user's typing.
	gen, _ := notes.GeneratorFor(notes.TypeBasic)

	err := gen.Validate(fields("front", "ciao", "back", "hello", "extra", "?"), notes.Config{})
	if !errors.Is(err, notes.ErrInvalidNote) {
		t.Fatalf("Validate with an unknown field = %v, want ErrInvalidNote", err)
	}
}

func TestUnknownNoteTypeIsRejected(t *testing.T) {
	if _, err := notes.GeneratorFor(notes.NoteType("sideways")); !errors.Is(err, notes.ErrUnknownNoteType) {
		t.Fatalf("GeneratorFor(sideways) = %v, want ErrUnknownNoteType", err)
	}
	if _, err := notes.GeneratorFor(notes.NoteType("")); !errors.Is(err, notes.ErrUnknownNoteType) {
		t.Fatalf("GeneratorFor(\"\") = %v, want ErrUnknownNoteType", err)
	}
}

func TestBasicRenderPicksQuestionAndAnswerPerTemplate(t *testing.T) {
	// Rendering is what lets the review API stay generic: it asks the note
	// type which side to show, rather than assuming front/back.
	gen, _ := notes.GeneratorFor(notes.TypeBasic)
	f := fields("front", "ciao", "back", "hello")

	forward, err := gen.Render(f, notes.Config{}, notes.TemplateForward)
	if err != nil {
		t.Fatalf("Render forward: %v", err)
	}
	if forward.Question != "ciao" || forward.Answer != "hello" {
		t.Fatalf("forward = %+v, want question=ciao answer=hello", forward)
	}

	reverse, err := gen.Render(f, notes.Config{Reversed: true}, notes.TemplateReverse)
	if err != nil {
		t.Fatalf("Render reverse: %v", err)
	}
	if reverse.Question != "hello" || reverse.Answer != "ciao" {
		t.Fatalf("reverse = %+v, want question=hello answer=ciao", reverse)
	}
}

func TestRenderRejectsUnknownTemplate(t *testing.T) {
	gen, _ := notes.GeneratorFor(notes.TypeBasic)

	if _, err := gen.Render(fields("front", "a", "back", "b"), notes.Config{}, "cloze:1"); err == nil {
		t.Fatalf("Render with an unknown template succeeded")
	}
}

func TestFieldValuesArePreservedVerbatimAsMarkdown(t *testing.T) {
	// Field values are markdown and may embed image references; the domain
	// must not rewrite or escape them.
	gen, _ := notes.GeneratorFor(notes.TypeBasic)
	md := "**ciao** ![](/api/images/7)\n\n- a\n- b"

	if err := gen.Validate(fields("front", md, "back", "hello"), notes.Config{}); err != nil {
		t.Fatalf("Validate markdown: %v", err)
	}
	out, err := gen.Render(fields("front", md, "back", "hello"), notes.Config{}, notes.TemplateForward)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Question != md {
		t.Fatalf("markdown was altered:\n got %q\nwant %q", out.Question, md)
	}
}
