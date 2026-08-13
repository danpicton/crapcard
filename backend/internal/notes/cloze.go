package notes

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Cloze markers use Anki's syntax: {{c1::answer}} or {{c1::answer::hint}}.
// Nesting is not supported — the scan stops at the first "}}" — and c0 is not
// a deletion, matching Anki, where numbering starts at 1.
var clozeMarker = regexp.MustCompile(`\{\{c([1-9][0-9]*)::(.*?)\}\}`)

// clozeSpan is one parsed marker.
type clozeSpan struct {
	Number int
	Answer string
	Hint   string // "" when the marker carries none
}

// parseClozeSpans returns every well-formed marker in the text, in order.
func parseClozeSpans(text string) []clozeSpan {
	var spans []clozeSpan
	for _, m := range clozeMarker.FindAllStringSubmatch(text, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		answer, hint := m[2], ""
		if i := strings.Index(answer, "::"); i >= 0 {
			answer, hint = answer[:i], answer[i+2:]
		}
		spans = append(spans, clozeSpan{Number: n, Answer: answer, Hint: hint})
	}
	return spans
}

// clozeNumbers lists the distinct deletion numbers in the text, ascending.
// Each becomes one card.
func clozeNumbers(text string) []int {
	seen := map[int]bool{}
	for _, s := range parseClozeSpans(text) {
		seen[s.Number] = true
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]int, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// renderClozeText renders the text for the card testing one deletion number.
// On the question the tested deletions become blanks — "[...]", or "[hint]"
// when the marker carries one — and every other marker is revealed as plain
// text. On the answer everything is revealed, with the tested text bolded so
// the eye lands on what was asked.
func renderClozeText(text string, test int) Rendered {
	question := clozeMarker.ReplaceAllStringFunc(text, func(marker string) string {
		s := parseOneSpan(marker)
		if s.Number != test {
			return s.Answer
		}
		if s.Hint != "" {
			return "[" + s.Hint + "]"
		}
		return "[...]"
	})
	answer := clozeMarker.ReplaceAllStringFunc(text, func(marker string) string {
		s := parseOneSpan(marker)
		if s.Number != test {
			return s.Answer
		}
		return "**" + s.Answer + "**"
	})
	return Rendered{Question: question, Answer: answer}
}

// parseOneSpan parses a string already known to match clozeMarker.
func parseOneSpan(marker string) clozeSpan {
	spans := parseClozeSpans(marker)
	if len(spans) == 0 {
		return clozeSpan{Answer: marker}
	}
	return spans[0]
}

// ── Cloze note type ──────────────────────────────────────────────────────────

// clozeTemplatePrefix starts every cloze card template: "cloze:1", "cloze:2"…
const clozeTemplatePrefix = "cloze:"

// clozeTemplate names the card testing one deletion number.
func clozeTemplate(n int) string {
	return clozeTemplatePrefix + strconv.Itoa(n)
}

// clozeGenerator expands a cloze note into one card per deletion number. It
// reuses the basic note's front/back field names so a note can switch between
// basic and cloze without its content moving anywhere: the front holds the
// cloze text, the back is optional extra context for the answer.
type clozeGenerator struct{}

func (clozeGenerator) Fields() []string { return []string{FieldFront, FieldBack} }

// Validate requires a front with at least one well-formed deletion. The back
// is optional — it is extra context, not a side of its own.
func (g clozeGenerator) Validate(fields []Field, _ Config) error {
	for _, f := range fields {
		if f.Name != FieldFront && f.Name != FieldBack {
			return fmt.Errorf("%w: unknown field %q", ErrInvalidNote, f.Name)
		}
	}
	front, _ := fieldValue(fields, FieldFront)
	if strings.TrimSpace(front) == "" {
		return fmt.Errorf("%w: front is required", ErrInvalidNote)
	}
	if len(clozeNumbers(front)) == 0 {
		return fmt.Errorf("%w: a cloze note needs at least one {{c1::…}} deletion", ErrInvalidNote)
	}
	return nil
}

// Generate returns one card per distinct deletion number, ascending, so a
// card's template — and with it the review history — survives edits that
// leave its number in place.
func (g clozeGenerator) Generate(fields []Field, cfg Config) ([]CardSpec, error) {
	if err := g.Validate(fields, cfg); err != nil {
		return nil, err
	}
	front, _ := fieldValue(fields, FieldFront)
	specs := []CardSpec{}
	for _, n := range clozeNumbers(front) {
		specs = append(specs, CardSpec{Template: clozeTemplate(n)})
	}
	return specs, nil
}

// Render blanks the tested deletion on the question and reveals everything on
// the answer, with the back — extra context — appended below it.
func (g clozeGenerator) Render(fields []Field, _ Config, template string) (Rendered, error) {
	front, _ := fieldValue(fields, FieldFront)

	n, err := clozeTemplateNumber(template)
	if err != nil {
		return Rendered{}, err
	}
	tested := false
	for _, have := range clozeNumbers(front) {
		if have == n {
			tested = true
		}
	}
	if !tested {
		return Rendered{}, fmt.Errorf("%w: template %q", ErrInvalidNote, template)
	}

	r := renderClozeText(front, n)
	if back, _ := fieldValue(fields, FieldBack); strings.TrimSpace(back) != "" {
		r.Answer += "\n\n" + back
	}
	return r, nil
}

// clozeTemplateNumber extracts N from "cloze:N".
func clozeTemplateNumber(template string) (int, error) {
	raw, ok := strings.CutPrefix(template, clozeTemplatePrefix)
	if !ok {
		return 0, fmt.Errorf("%w: template %q", ErrInvalidNote, template)
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%w: template %q", ErrInvalidNote, template)
	}
	return n, nil
}
