package notes

import (
	"regexp"
	"strings"
)

// imagePattern matches a markdown image: ![alt](url) with an optional title.
//
// The leading (^|[^\\]) guard keeps an escaped \![...] from matching, and the
// alt group stops at the first ] so nested brackets do not run away.
var imagePattern = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)

// PreviewMarkdown rewrites card markdown for a text preview, replacing each
// image with a description of it.
//
// The point is to be able to read a card the way it will be asked — and to
// see at a glance which images carry no alt text, since an undescribed image
// is one nobody can review from a screen reader or a text-only glance.
func PreviewMarkdown(md string) string {
	return imagePattern.ReplaceAllStringFunc(md, func(match string) string {
		groups := imagePattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}
		alt := strings.TrimSpace(groups[1])
		if alt == "" {
			return "*image:* no alt text"
		}
		return `*image:* "` + alt + `"`
	})
}
