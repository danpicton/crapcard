// Package notes holds the authored side of a flashcard: a note carries the
// content the user typed, and its note type expands that content into the
// cards actually scheduled for review.
//
// The note/card split is what keeps cloze deletion an additive change. A note
// type answers two questions — which cards does this note produce
// (Generate), and what is shown on each side of one of them (Render) — so
// review and scheduling never learn about note types at all. Adding cloze
// means registering another Generator here, not touching the review queue.
package notes

import (
	"errors"
	"fmt"
	"strings"
)

// NoteType names a family of notes that share a field set and a card layout.
type NoteType string

// TypeBasic is a two-sided note with a front and a back, optionally reversed
// into a second card.
const TypeBasic NoteType = "basic"

// TypeCloze is a note whose front carries {{c1::…}} deletions, one card per
// deletion number. The back, when present, is extra context shown with the
// answer.
const TypeCloze NoteType = "cloze"

// Card templates. A template names one card a note produces; it is stored on
// the card row and handed back to Render at review time. Cloze adds
// per-deletion templates of the form "cloze:1", "cloze:2", ...
const (
	TemplateForward = "forward"
	TemplateReverse = "reverse"
)

// Field is one named piece of note content. Values are markdown, stored and
// returned verbatim — they may embed image references, and rewriting them
// would corrupt what the user typed.
type Field struct {
	Name  string
	Value string
}

// Config carries per-note options that vary by note type. It is persisted as
// JSON so a new note type can add options without a migration.
type Config struct {
	// Reversed asks a basic note for a second card testing back to front.
	Reversed bool `json:"reversed"`
	// Occlusion carries an image-cloze note's masks. Nil for other types;
	// omitempty keeps stored configs of older notes byte-identical.
	Occlusion *Occlusion `json:"occlusion,omitempty"`
}

// CardSpec describes one card a note should produce.
type CardSpec struct {
	// Template identifies which card of the note this is.
	Template string
}

// Rendered is the two sides of a card, as markdown.
type Rendered struct {
	Question string
	Answer   string
}

// ErrInvalidNote is returned when note content does not satisfy its type.
var ErrInvalidNote = errors.New("invalid note")

// ErrUnknownNoteType is returned for a note type with no registered
// generator.
var ErrUnknownNoteType = errors.New("unknown note type")

// Generator expands a note of one type into cards and renders their sides.
type Generator interface {
	// Fields returns the field names this type expects, in display order.
	Fields() []string
	// Validate reports whether the content is well-formed for this type.
	Validate(fields []Field, cfg Config) error
	// Generate lists the cards the note produces, in a stable order.
	Generate(fields []Field, cfg Config) ([]CardSpec, error)
	// Render returns what to show for one of those cards.
	Render(fields []Field, cfg Config, template string) (Rendered, error)
}

// generators is the note type registry. Adding a type is a matter of adding
// an entry here.
var generators = map[NoteType]Generator{
	TypeBasic:      basicGenerator{},
	TypeCloze:      clozeGenerator{},
	TypeImageCloze: occlusionGenerator{},
}

// GeneratorFor returns the generator for a note type, or ErrUnknownNoteType.
func GeneratorFor(t NoteType) (Generator, error) {
	g, ok := generators[t]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownNoteType, t)
	}
	return g, nil
}

// KnownTypes lists the registered note types, for the client to offer.
func KnownTypes() []NoteType {
	return []NoteType{TypeBasic, TypeCloze, TypeImageCloze}
}

// fieldValue looks up a field by name.
func fieldValue(fields []Field, name string) (string, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f.Value, true
		}
	}
	return "", false
}

// ── Basic ────────────────────────────────────────────────────────────────────

// Basic note field names.
const (
	FieldFront = "front"
	FieldBack  = "back"
)

type basicGenerator struct{}

func (basicGenerator) Fields() []string { return []string{FieldFront, FieldBack} }

// Validate requires both fields to be present and non-blank, and rejects any
// field the type does not define. An unrecognised field means the client is
// out of step with the server, and dropping it silently would lose content.
func (g basicGenerator) Validate(fields []Field, _ Config) error {
	known := map[string]bool{FieldFront: true, FieldBack: true}
	seen := map[string]bool{}

	for _, f := range fields {
		if !known[f.Name] {
			return fmt.Errorf("%w: unknown field %q", ErrInvalidNote, f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("%w: duplicate field %q", ErrInvalidNote, f.Name)
		}
		seen[f.Name] = true
	}

	for _, name := range g.Fields() {
		v, ok := fieldValue(fields, name)
		if !ok || strings.TrimSpace(v) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidNote, name)
		}
	}
	return nil
}

// Generate returns the forward card, plus the reverse card when the note asks
// for it. The order is stable so card templates keep their identity across
// edits.
func (g basicGenerator) Generate(fields []Field, cfg Config) ([]CardSpec, error) {
	if err := g.Validate(fields, cfg); err != nil {
		return nil, err
	}
	specs := []CardSpec{{Template: TemplateForward}}
	if cfg.Reversed {
		specs = append(specs, CardSpec{Template: TemplateReverse})
	}
	return specs, nil
}

// Render maps a template onto which field is asked and which is answered.
func (g basicGenerator) Render(fields []Field, _ Config, template string) (Rendered, error) {
	front, _ := fieldValue(fields, FieldFront)
	back, _ := fieldValue(fields, FieldBack)

	switch template {
	case TemplateForward:
		return Rendered{Question: front, Answer: back}, nil
	case TemplateReverse:
		return Rendered{Question: back, Answer: front}, nil
	default:
		return Rendered{}, fmt.Errorf("%w: template %q", ErrInvalidNote, template)
	}
}
