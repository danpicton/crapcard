package notes_test

import (
	"testing"

	"github.com/danpicton/crapcard/internal/notes"
)

func TestPreviewMarkdownLeavesTextAlone(t *testing.T) {
	md := "**ciao**\n\n- one\n- two\n\n`code`"
	if got := notes.PreviewMarkdown(md); got != md {
		t.Fatalf("text was altered:\n got %q\nwant %q", got, md)
	}
}

func TestPreviewMarkdownDescribesImagesByTheirAltText(t *testing.T) {
	// A preview is for checking the card reads correctly, including whether
	// its images are described — so the alt text is what gets shown.
	cases := map[string]struct{ in, want string }{
		"with alt": {
			`![the femur](/api/images/abc)`,
			`*image:* "the femur"`,
		},
		"with alt and width": {
			`![the femur](/api/images/abc?w=400)`,
			`*image:* "the femur"`,
		},
		"empty alt": {
			`![](/api/images/abc)`,
			`*image:* no alt text`,
		},
		"whitespace-only alt": {
			`![   ](/api/images/abc)`,
			`*image:* no alt text`,
		},
		"with a title": {
			`![the femur](/api/images/abc "a title")`,
			`*image:* "the femur"`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := notes.PreviewMarkdown(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreviewMarkdownHandlesImagesAmongText(t *testing.T) {
	in := "The **femur**:\n\n![thigh bone](/api/images/a?w=300)\n\nLongest in the body."
	want := "The **femur**:\n\n*image:* \"thigh bone\"\n\nLongest in the body."

	if got := notes.PreviewMarkdown(in); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPreviewMarkdownHandlesSeveralImages(t *testing.T) {
	in := `![one](/api/images/a) and ![](/api/images/b)`
	want := `*image:* "one" and *image:* no alt text`

	if got := notes.PreviewMarkdown(in); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPreviewMarkdownDoesNotTouchOrdinaryLinks(t *testing.T) {
	// A link is not an image; only the ! prefix makes it one.
	in := `see [the femur](/api/images/abc)`
	if got := notes.PreviewMarkdown(in); got != in {
		t.Fatalf("a link was rewritten: %q", got)
	}
}
