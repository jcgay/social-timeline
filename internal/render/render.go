// Package render writes a slice of timeline.Post values as a Markdown
// document. The format is documented in
// docs/superpowers/specs/2026-04-26-social-timeline-design.md §5.
package render

import (
	"fmt"
	"io"

	"github.com/jcgay/social-timeline/internal/timeline"
)

// Markdown writes one ## block per post, separated by a blank line.
// Posts are emitted in the order received; callers are responsible for
// sorting (see timeline.MergeSort).
func Markdown(posts []timeline.Post, w io.Writer) error {
	for i, p := range posts {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if err := writeOne(w, p); err != nil {
			return err
		}
	}
	return nil
}

func writeOne(w io.Writer, p timeline.Post) error {
	marker := typeMarker(p)
	if _, err := fmt.Fprintf(w, "## @%s — %s [%s]%s\n",
		p.Author,
		p.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		p.Platform,
		marker,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "%s\n", p.Text); err != nil {
		return err
	}
	for _, m := range p.MediaURLs {
		if _, err := fmt.Fprintf(w, "📎 %s\n", m); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "🔗 %s\n", p.URL); err != nil {
		return err
	}
	return nil
}

func typeMarker(p timeline.Post) string {
	switch p.Type {
	case timeline.PostRepost:
		verb := "repost"
		if p.Platform == "mastodon" {
			verb = "boost"
		}
		return fmt.Sprintf(" (%s of @%s)", verb, p.OriginalAuthor)
	case timeline.PostReply:
		return fmt.Sprintf(" (reply to @%s)", p.OriginalAuthor)
	}
	return ""
}
