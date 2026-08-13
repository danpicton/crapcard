package notes

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Image occlusion: a note whose front holds an image, with rectangles of it
// masked off. Each mask is one card.
//
// The masks are rendered client-side. Render embeds them in the image URL's
// fragment — "#occ=<url-escaped JSON>" — which the browser never sends to the
// server, so the image bytes stay cacheable while the card's markdown stays
// plain markdown all the way through study, preview and the offline queue.

// Occlusion mode names. HideOne covers only the tested mask on the question;
// HideAll covers every mask and highlights the tested one — for images where
// the neighbouring labels would give the answer away.
const (
	OcclusionHideOne = "hide-one"
	OcclusionHideAll = "hide-all"
)

// OcclusionRect is one mask, in coordinates normalised to the image (0–1 for
// x, y, width and height), so it needs no knowledge of the image's pixel size.
type OcclusionRect struct {
	ID int     `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	W  float64 `json:"w"`
	H  float64 `json:"h"`
}

// Occlusion is the mask set of an image-cloze note, persisted in the note's
// config. IDs are assigned by the client and never reused, so a card keeps
// its identity — and review history — while other masks come and go.
type Occlusion struct {
	// Mode is OcclusionHideOne or OcclusionHideAll; empty means hide-one.
	Mode  string          `json:"mode"`
	Rects []OcclusionRect `json:"rects"`
}

// TypeImageCloze is a note whose front carries an image with masked areas,
// one card per mask.
const TypeImageCloze NoteType = "image-cloze"

// occTemplatePrefix starts every image-cloze card template: "occ:1", "occ:2"…
const occTemplatePrefix = "occ:"

// occTemplate names the card testing one mask.
func occTemplate(id int) string {
	return occTemplatePrefix + strconv.Itoa(id)
}

// markdownImage finds "![alt](url)" so Render can rewrite the URL of the
// front's first image. Titles and nested parens are not part of what the
// editor produces.
var markdownImage = regexp.MustCompile(`!\[[^\]]*\]\(([^)]*)\)`)

// occlusionGenerator expands an image-cloze note into one card per mask. Like
// cloze it reuses front/back, so a basic note grows masks — or loses them —
// without its content moving anywhere.
type occlusionGenerator struct{}

func (occlusionGenerator) Fields() []string { return []string{FieldFront, FieldBack} }

// Validate requires an image on the front and at least one well-formed mask.
func (g occlusionGenerator) Validate(fields []Field, cfg Config) error {
	for _, f := range fields {
		if f.Name != FieldFront && f.Name != FieldBack {
			return fmt.Errorf("%w: unknown field %q", ErrInvalidNote, f.Name)
		}
	}
	front, _ := fieldValue(fields, FieldFront)
	if !markdownImage.MatchString(front) {
		return fmt.Errorf("%w: an image-cloze note needs an image on the front", ErrInvalidNote)
	}

	occ := cfg.Occlusion
	if occ == nil || len(occ.Rects) == 0 {
		return fmt.Errorf("%w: an image-cloze note needs at least one mask", ErrInvalidNote)
	}
	if occ.Mode != "" && occ.Mode != OcclusionHideOne && occ.Mode != OcclusionHideAll {
		return fmt.Errorf("%w: unknown occlusion mode %q", ErrInvalidNote, occ.Mode)
	}
	seen := map[int]bool{}
	for _, r := range occ.Rects {
		if r.ID < 1 {
			return fmt.Errorf("%w: mask id %d", ErrInvalidNote, r.ID)
		}
		if seen[r.ID] {
			return fmt.Errorf("%w: duplicate mask id %d", ErrInvalidNote, r.ID)
		}
		seen[r.ID] = true
		if r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > 1 || r.Y+r.H > 1 {
			return fmt.Errorf("%w: mask %d is outside the image", ErrInvalidNote, r.ID)
		}
	}
	return nil
}

// Generate returns one card per mask, in the order the masks were drawn —
// their IDs ascend by construction, and keeping the stored order means the
// preview pages through them the way the author laid them out.
func (g occlusionGenerator) Generate(fields []Field, cfg Config) ([]CardSpec, error) {
	if err := g.Validate(fields, cfg); err != nil {
		return nil, err
	}
	specs := []CardSpec{}
	for _, r := range cfg.Occlusion.Rects {
		specs = append(specs, CardSpec{Template: occTemplate(r.ID)})
	}
	return specs, nil
}

// occFragmentPayload is what the client reads back out of "#occ=".
type occFragmentPayload struct {
	Mode  string          `json:"mode"`
	Side  string          `json:"side"` // "q" or "a"
	Test  int             `json:"test"` // the mask this card asks about
	Rects []OcclusionRect `json:"rects"`
}

// Render rewrites the front's first image URL to carry the mask set in its
// fragment, once for each side. What differs between the sides is only
// side:"q" vs side:"a" — how much to cover is the client's rendering rule,
// kept in one place there.
func (g occlusionGenerator) Render(fields []Field, cfg Config, template string) (Rendered, error) {
	raw, ok := strings.CutPrefix(template, occTemplatePrefix)
	if !ok {
		return Rendered{}, fmt.Errorf("%w: template %q", ErrInvalidNote, template)
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return Rendered{}, fmt.Errorf("%w: template %q", ErrInvalidNote, template)
	}

	occ := cfg.Occlusion
	if occ == nil {
		return Rendered{}, fmt.Errorf("%w: note has no masks", ErrInvalidNote)
	}
	found := false
	for _, r := range occ.Rects {
		if r.ID == id {
			found = true
		}
	}
	if !found {
		return Rendered{}, fmt.Errorf("%w: template %q", ErrInvalidNote, template)
	}

	mode := occ.Mode
	if mode == "" {
		mode = OcclusionHideOne
	}

	front, _ := fieldValue(fields, FieldFront)
	question, err := withOcclusionFragment(front, occFragmentPayload{
		Mode: mode, Side: "q", Test: id, Rects: occ.Rects,
	})
	if err != nil {
		return Rendered{}, err
	}
	answer, err := withOcclusionFragment(front, occFragmentPayload{
		Mode: mode, Side: "a", Test: id, Rects: occ.Rects,
	})
	if err != nil {
		return Rendered{}, err
	}

	if back, _ := fieldValue(fields, FieldBack); strings.TrimSpace(back) != "" {
		answer += "\n\n" + back
	}
	return Rendered{Question: question, Answer: answer}, nil
}

// withOcclusionFragment appends "#occ=<payload>" to the URL of the first
// image in the markdown.
func withOcclusionFragment(markdown string, payload occFragmentPayload) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode occlusion: %w", err)
	}
	fragment := "#occ=" + url.PathEscape(string(encoded))

	replaced := false
	out := markdownImage.ReplaceAllStringFunc(markdown, func(m string) string {
		if replaced {
			return m
		}
		replaced = true
		// Splice the fragment onto the URL, inside the closing paren.
		return m[:len(m)-1] + fragment + ")"
	})
	return out, nil
}
