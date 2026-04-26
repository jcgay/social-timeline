package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

func ts(h, m int) time.Time {
	return time.Date(2026, 4, 26, h, m, 0, 0, time.UTC)
}

func TestMarkdown_singleOriginal(t *testing.T) {
	posts := []timeline.Post{{
		Platform: "bluesky", Author: "alice.bsky.social", CreatedAt: ts(14, 23),
		Text: "hello world", URL: "https://bsky.app/profile/alice.bsky.social/post/xyz",
		Type: timeline.PostOriginal,
	}}
	var buf bytes.Buffer
	if err := Markdown(posts, &buf); err != nil {
		t.Fatal(err)
	}
	want := "## @alice.bsky.social — 2026-04-26 14:23 UTC [bluesky]\nhello world\n🔗 https://bsky.app/profile/alice.bsky.social/post/xyz\n"
	if buf.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestMarkdown_repostMarker(t *testing.T) {
	posts := []timeline.Post{{
		Platform: "mastodon", Author: "bob@mastodon.social", CreatedAt: ts(14, 25),
		Text: "boosted body", URL: "https://piaille.fr/@carol/123456",
		Type: timeline.PostRepost, OriginalAuthor: "carol@piaille.fr",
	}}
	var buf bytes.Buffer
	if err := Markdown(posts, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[mastodon] (boost of @carol@piaille.fr)") {
		t.Fatalf("missing boost marker: %s", buf.String())
	}
}

func TestMarkdown_blueskyRepostUsesRepostMarker(t *testing.T) {
	posts := []timeline.Post{{
		Platform: "bluesky", Author: "a.bsky.social", CreatedAt: ts(14, 25),
		Text: "x", URL: "u", Type: timeline.PostRepost, OriginalAuthor: "b.bsky.social",
	}}
	var buf bytes.Buffer
	_ = Markdown(posts, &buf)
	if !strings.Contains(buf.String(), "(repost of @b.bsky.social)") {
		t.Fatalf("missing repost marker: %s", buf.String())
	}
}

func TestMarkdown_replyMarkerAndMedia(t *testing.T) {
	posts := []timeline.Post{{
		Platform: "bluesky", Author: "dave.bsky.social", CreatedAt: ts(14, 30),
		Text: "reply body", URL: "https://bsky.app/profile/dave.bsky.social/post/abc",
		Type: timeline.PostReply, OriginalAuthor: "eve.bsky.social",
		MediaURLs: []string{"https://cdn/img1.jpg", "https://cdn/img2.jpg"},
	}}
	var buf bytes.Buffer
	_ = Markdown(posts, &buf)
	got := buf.String()
	for _, want := range []string{
		"(reply to @eve.bsky.social)",
		"📎 https://cdn/img1.jpg",
		"📎 https://cdn/img2.jpg",
		"🔗 https://bsky.app/profile/dave.bsky.social/post/abc",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestMarkdown_emptyBody(t *testing.T) {
	posts := []timeline.Post{{
		Platform: "bluesky", Author: "a.bsky.social", CreatedAt: ts(10, 0),
		Text: "", URL: "u", Type: timeline.PostOriginal,
		MediaURLs: []string{"https://cdn/x.jpg"},
	}}
	var buf bytes.Buffer
	_ = Markdown(posts, &buf)
	want := "## @a.bsky.social — 2026-04-26 10:00 UTC [bluesky]\n\n📎 https://cdn/x.jpg\n🔗 u\n"
	if buf.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestMarkdown_blocksSeparatedByBlankLine(t *testing.T) {
	posts := []timeline.Post{
		{Platform: "bluesky", Author: "a", CreatedAt: ts(10, 0), Text: "1", URL: "u1"},
		{Platform: "bluesky", Author: "b", CreatedAt: ts(11, 0), Text: "2", URL: "u2"},
	}
	var buf bytes.Buffer
	_ = Markdown(posts, &buf)
	if !strings.Contains(buf.String(), "🔗 u1\n\n## @b") {
		t.Fatalf("posts not separated by blank line:\n%s", buf.String())
	}
}

func TestMarkdown_emptyInput(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(nil, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty output, got %q", buf.String())
	}
}
