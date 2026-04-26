# social-timeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI named `social-timeline` that aggregates Bluesky and Mastodon home timelines over a time window and writes a single Markdown document to stdout, optimized for downstream LLM consumption.

**Architecture:** Single Go module. A neutral `internal/timeline.Post` type sits at the center. Two platform packages (`internal/bluesky`, `internal/mastodon`) implement a `timeline.Client` interface that returns `[]Post`; platform-specific API shapes never leak. A `render` package emits Markdown. `cmd/social-timeline/main.go` parses flags, reads env credentials, runs the configured clients in parallel via `errgroup`, merges/sorts, and writes the output.

**Tech Stack:** Go 1.22+, standard library only for HTTP/JSON, `golang.org/x/sync/errgroup` for parallel fetches, `golang.org/x/net/html` for Mastodon HTML stripping, `golangci-lint` for linting.

**Reference spec:** `docs/superpowers/specs/2026-04-26-social-timeline-design.md`

---

## File Structure

```
go.mod
go.sum
Makefile
README.md
.gitignore
.golangci.yml
cmd/social-timeline/
  main.go                  CLI entrypoint: flag parsing, env config, orchestration, output writing
  main_test.go             integration tests for the CLI surface (flag parsing, env-only config, exit codes)
internal/timeline/
  post.go                  Post struct, PostType constants, Client interface
  aggregate.go             MergeSort([]Post...) []Post — deterministic merge across platforms
  aggregate_test.go
internal/timeparse/
  timeparse.go             ParseSince(s string, now time.Time) (time.Time, error) — duration or ISO date
  timeparse_test.go
internal/htmltext/
  htmltext.go              FromHTML(s string) string — Mastodon HTML to plain text
  htmltext_test.go
internal/render/
  render.go                Markdown(posts []timeline.Post, w io.Writer) error
  render_test.go
internal/mastodon/
  client.go                Client implementing timeline.Client; auth via bearer token
  parse.go                 JSON status struct + ToPost conversion + acct fully-qualifying
  client_test.go           httptest fixtures: original, boost, reply, image, empty, paginated, end-of-feed
  testdata/
internal/bluesky/
  client.go                Client implementing timeline.Client; createSession + getTimeline
  parse.go                 feed item struct + ToPost conversion + permalink construction + media extraction
  client_test.go           httptest fixtures: original, repost, reply, image, empty, paginated, end-of-feed
  testdata/
docs/
  superpowers/
    specs/2026-04-26-social-timeline-design.md   (existing)
    plans/2026-04-26-social-timeline.md          (this file)
```

**Decomposition rationale:**
- `internal/timeline` owns the cross-package types (`Post`, `Client`) so platform clients import `timeline`, never the other way around (no import cycle).
- `parse.go` is split out from `client.go` in each platform package so the JSON-to-`Post` mapping can be unit-tested without the HTTP layer.
- `htmltext` and `timeparse` are pure helpers, fully unit-tested.
- `cmd/social-timeline/main.go` is the only place that touches `os.Stdout`/`os.Stderr`/`os.Getenv`/flags.

---

## Task 1: Bootstrap module, tooling, repo hygiene

**Files:**
- Create: `go.mod`, `Makefile`, `.gitignore`, `.golangci.yml`, `README.md`

- [ ] **Step 1: Initialize module**

Run:
```bash
cd /Volumes/sourcecode/social-timeline
go mod init github.com/jcgay/social-timeline
go get golang.org/x/sync/errgroup@latest
go get golang.org/x/net/html@latest
go mod tidy
```

Expected: `go.mod` and `go.sum` created and tidy.

Note: `make lint` requires `golangci-lint` on `PATH`. Install with
`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
or skip the lint target.

- [ ] **Step 2: Create `.gitignore`**

```gitignore
/social-timeline
/dist/
*.test
*.out
.env
.envrc
```

- [ ] **Step 3: Create `Makefile`**

```makefile
.PHONY: build test lint fmt tidy clean

build:
	go build -o social-timeline ./cmd/social-timeline

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -f social-timeline
```

- [ ] **Step 4: Create minimal `.golangci.yml`**

```yaml
run:
  timeout: 2m
linters:
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - goimports
```

- [ ] **Step 5: Create skeleton `README.md`**

```markdown
# social-timeline

A Go CLI that aggregates your Bluesky and Mastodon home timelines into a single
Markdown document, intended to be piped into an LLM for ranking or summarization.

See `docs/superpowers/specs/2026-04-26-social-timeline-design.md` for the
design spec.

## Build

    make build

## Usage

    export BLUESKY_HANDLE=alice.bsky.social
    export BLUESKY_APP_PASSWORD=xxxx-xxxx-xxxx-xxxx
    export MASTODON_INSTANCE_URL=https://mastodon.social
    export MASTODON_ACCESS_TOKEN=xxxx
    ./social-timeline --since 1d
```

- [ ] **Step 6: Verify the toolchain**

Run: `go version && go build ./... && go test ./...`
Expected: builds with no targets, tests pass with no targets ("no Go files" or "ok" for empty packages is fine).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: bootstrap go module, makefile, lint config

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 2: `internal/timeparse` — `--since` parser

**Files:**
- Create: `internal/timeparse/timeparse.go`, `internal/timeparse/timeparse_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/timeparse/timeparse_test.go`:

```go
package timeparse

import (
	"testing"
	"time"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 4, 26, 18, 0, 0, 0, time.UTC)
	cases := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"30s", now.Add(-30 * time.Second), false},
		{"15m", now.Add(-15 * time.Minute), false},
		{"6h", now.Add(-6 * time.Hour), false},
		{"1d", now.Add(-24 * time.Hour), false},
		{"2w", now.Add(-14 * 24 * time.Hour), false},
		{"2026-04-25", time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), false},
		{"2026-04-25T10:00:00Z", time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC), false},
		{"2026-04-25T10:00:00+02:00", time.Date(2026, 4, 25, 8, 0, 0, 0, time.UTC), false},
		{"", time.Time{}, true},
		{"1.5d", time.Time{}, true},
		{"d", time.Time{}, true},
		{"1y", time.Time{}, true},
		{"-1d", time.Time{}, true},
		{"not-a-date", time.Time{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseSince(tc.in, now)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && !got.Equal(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/timeparse/...`
Expected: compile error (no `ParseSince` defined).

- [ ] **Step 3: Implement `ParseSince`**

Create `internal/timeparse/timeparse.go`:

```go
// Package timeparse parses the --since CLI flag, which accepts either a
// relative duration (e.g. "1d", "6h", "30m", "2w") or an ISO-8601
// date/datetime. Naive dates are interpreted as UTC.
package timeparse

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var durationRe = regexp.MustCompile(`^(\d+)([smhdw])$`)

// ParseSince resolves the --since flag to an absolute UTC timestamp.
// `now` is injected for testability and is treated as the upper bound
// for duration-relative inputs.
func ParseSince(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("--since: empty value")
	}
	if m := durationRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		var unit time.Duration
		switch m[2] {
		case "s":
			unit = time.Second
		case "m":
			unit = time.Minute
		case "h":
			unit = time.Hour
		case "d":
			unit = 24 * time.Hour
		case "w":
			unit = 7 * 24 * time.Hour
		}
		return now.Add(-time.Duration(n) * unit).UTC(), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("--since: %q is not a duration (e.g. 1d, 6h) or an ISO-8601 date", s)
}
```

- [ ] **Step 4: Run tests, verify they pass**

Run: `go test ./internal/timeparse/... -v`
Expected: PASS for every case.

- [ ] **Step 5: Commit**

```bash
git add internal/timeparse/
git commit -m "feat(timeparse): parse --since duration or ISO date

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 3: `internal/timeline` — neutral Post type, Client interface, MergeSort

**Files:**
- Create: `internal/timeline/post.go`, `internal/timeline/aggregate.go`, `internal/timeline/aggregate_test.go`

- [ ] **Step 1: Create `post.go`**

```go
// Package timeline holds the platform-neutral domain types and the
// Client interface that every platform package must implement.
// Importing this package from a platform package is allowed; the
// reverse is not, to avoid an import cycle.
package timeline

import (
	"context"
	"time"
)

type PostType int

const (
	PostOriginal PostType = iota
	PostRepost            // Bluesky repost or Mastodon boost
	PostReply
)

// Post is the only structure that crosses package boundaries between
// platform clients and the renderer. It is platform-agnostic.
type Post struct {
	Platform       string    // "bluesky" | "mastodon"
	Author         string    // "alice.bsky.social" or "bob@mastodon.social" (no leading @)
	CreatedAt      time.Time // UTC
	Text           string    // plain text body, already HTML-stripped where applicable
	URL            string    // canonical permalink
	Type           PostType
	OriginalAuthor string    // set for PostRepost / PostReply, empty otherwise
	MediaURLs      []string
}

// Client is implemented by every platform package. Pagination and
// per-platform max enforcement are internal to the implementation.
type Client interface {
	// Name returns the platform identifier, e.g. "bluesky".
	Name() string
	// FetchHomeTimeline returns posts whose CreatedAt >= since,
	// optionally capped at maxPosts (0 = no limit).
	FetchHomeTimeline(ctx context.Context, since time.Time, maxPosts int) ([]Post, error)
}
```

- [ ] **Step 2: Write failing test for MergeSort**

Create `internal/timeline/aggregate_test.go`:

```go
package timeline

import (
	"testing"
	"time"
)

func TestMergeSort_chronologicalAcrossPlatforms(t *testing.T) {
	t1 := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 26, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	bsky := []Post{{Platform: "bluesky", CreatedAt: t3, URL: "b3"}, {Platform: "bluesky", CreatedAt: t1, URL: "b1"}}
	mast := []Post{{Platform: "mastodon", CreatedAt: t2, URL: "m2"}}
	out := MergeSort(bsky, mast)
	got := []string{out[0].URL, out[1].URL, out[2].URL}
	want := []string{"b1", "m2", "b3"}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %q want %q (full=%v)", i, got[i], want[i], got)
		}
	}
}

func TestMergeSort_tieBreakByPlatformThenURL(t *testing.T) {
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	in := []Post{
		{Platform: "mastodon", CreatedAt: now, URL: "m-z"},
		{Platform: "bluesky", CreatedAt: now, URL: "b-b"},
		{Platform: "bluesky", CreatedAt: now, URL: "b-a"},
		{Platform: "mastodon", CreatedAt: now, URL: "m-a"},
	}
	out := MergeSort(in)
	want := []string{"b-a", "b-b", "m-a", "m-z"}
	for i, p := range out {
		if p.URL != want[i] {
			t.Fatalf("position %d: got %q want %q", i, p.URL, want[i])
		}
	}
}

func TestMergeSort_emptyInputs(t *testing.T) {
	out := MergeSort()
	if len(out) != 0 {
		t.Fatalf("got %d want 0", len(out))
	}
	out = MergeSort(nil, nil)
	if len(out) != 0 {
		t.Fatalf("got %d want 0", len(out))
	}
}
```

- [ ] **Step 3: Run tests, verify failure**

Run: `go test ./internal/timeline/...`
Expected: compile error (no `MergeSort`).

- [ ] **Step 4: Implement `MergeSort`**

Create `internal/timeline/aggregate.go`:

```go
package timeline

import "sort"

// MergeSort flattens any number of per-platform slices into one slice
// sorted by CreatedAt ascending. Ties are broken by platform name and
// then by URL, so output is fully deterministic regardless of input
// order.
func MergeSort(slices ...[]Post) []Post {
	total := 0
	for _, s := range slices {
		total += len(s)
	}
	out := make([]Post, 0, total)
	for _, s := range slices {
		out = append(out, s...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return out[i].URL < out[j].URL
	})
	return out
}
```

- [ ] **Step 5: Run tests, verify pass**

Run: `go test ./internal/timeline/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/timeline/
git commit -m "feat(timeline): neutral Post type, Client interface, MergeSort

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 4: `internal/htmltext` — HTML to plain text for Mastodon

**Files:**
- Create: `internal/htmltext/htmltext.go`, `internal/htmltext/htmltext_test.go`

- [ ] **Step 1: Write failing tests**

```go
package htmltext

import "testing"

func TestFromHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<p>hello world</p>", "hello world"},
		{"<p>line1</p><p>line2</p>", "line1\n\nline2"},
		{"first<br>second", "first\nsecond"},
		{"first<br />second", "first\nsecond"},
		{`see <a href="https://example.com">example</a>`, "see example (https://example.com)"},
		{`<a href="https://example.com">https://example.com</a>`, "https://example.com"},
		{`<p>hi <span class="h-card"><a href="https://m.s/@bob">@<span>bob</span></a></span></p>`, "hi @bob (https://m.s/@bob)"},
		{"&amp;&lt;&gt;&quot;&#39;", `&<>"'`},
		{"", ""},
		{"<p></p>", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := FromHTML(tc.in)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests, verify failure**

Run: `go test ./internal/htmltext/...`
Expected: compile error.

- [ ] **Step 3: Implement `FromHTML`**

```go
// Package htmltext converts the HTML body that Mastodon returns for
// statuses into a clean plain-text rendering suitable for inclusion in
// a Markdown document.
package htmltext

import (
	"strings"

	"golang.org/x/net/html"
)

// FromHTML strips HTML, keeping paragraph breaks and inlining link
// targets. The output never contains HTML tags or entity references.
func FromHTML(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader("<root>" + s + "</root>"))
	if err != nil {
		return s
	}
	var b strings.Builder
	walk(doc, &b)
	out := b.String()
	out = collapseBlankLines(out)
	return strings.TrimSpace(out)
}

func walk(n *html.Node, b *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
		return
	case html.ElementNode:
		switch n.Data {
		case "br":
			b.WriteByte('\n')
			return
		case "p", "div":
			// emit children, then a paragraph break
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, b)
			}
			b.WriteString("\n\n")
			return
		case "a":
			href := attr(n, "href")
			var inner strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, &inner)
			}
			label := strings.TrimSpace(inner.String())
			switch {
			case href == "":
				b.WriteString(label)
			case label == "" || label == href:
				b.WriteString(href)
			default:
				b.WriteString(label)
				b.WriteString(" (")
				b.WriteString(href)
				b.WriteByte(')')
			}
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, b)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/htmltext/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/htmltext/
git commit -m "feat(htmltext): convert Mastodon HTML status to plain text

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 5: `internal/render` — Markdown writer

**Files:**
- Create: `internal/render/render.go`, `internal/render/render_test.go`

- [ ] **Step 1: Write failing tests**

```go
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
```

- [ ] **Step 2: Run tests, verify failure**

Run: `go test ./internal/render/...`
Expected: compile error.

- [ ] **Step 3: Implement `Markdown`**

```go
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
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/render/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/
git commit -m "feat(render): markdown writer for posts

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 6: `internal/mastodon` — Mastodon client

**Files:**
- Create: `internal/mastodon/parse.go`, `internal/mastodon/client.go`, `internal/mastodon/client_test.go`, `internal/mastodon/testdata/page1.json`, `internal/mastodon/testdata/page2.json`

### 6a — Parse layer (pure)

- [ ] **Step 1: Capture realistic JSON fixtures**

Create `internal/mastodon/testdata/page1.json` with two statuses, one boost (`reblog` non-null), one image attachment, and a `Link` header instructing pagination. Real shape (trimmed):

```json
[
  {
    "id": "111",
    "created_at": "2026-04-26T14:25:00.000Z",
    "url": "https://mastodon.social/@alice/111",
    "content": "<p>Original post with a <a href=\"https://example.com\">link</a></p>",
    "in_reply_to_id": null,
    "reblog": null,
    "account": {"acct": "alice"},
    "media_attachments": [
      {"type": "image", "url": "https://files.mastodon.social/img/1.jpg"}
    ]
  },
  {
    "id": "112",
    "created_at": "2026-04-26T14:30:00.000Z",
    "url": "https://mastodon.social/users/bob/statuses/112/activity",
    "content": "",
    "in_reply_to_id": null,
    "account": {"acct": "bob"},
    "media_attachments": [],
    "reblog": {
      "id": "999",
      "created_at": "2026-04-26T10:00:00.000Z",
      "url": "https://piaille.fr/@carol/999",
      "content": "<p>Boosted body</p>",
      "in_reply_to_id": null,
      "account": {"acct": "carol@piaille.fr"},
      "media_attachments": []
    }
  }
]
```

Create `internal/mastodon/testdata/page2.json` with one reply (`in_reply_to_id` non-null, `in_reply_to_account_id` set; we'll resolve the parent author from `mentions[0].acct`):

```json
[
  {
    "id": "113",
    "created_at": "2026-04-26T09:00:00.000Z",
    "url": "https://mastodon.social/@dave/113",
    "content": "<p>reply body</p>",
    "in_reply_to_id": "999",
    "in_reply_to_account_id": "42",
    "mentions": [{"id": "42", "acct": "eve@mastodon.social"}],
    "reblog": null,
    "account": {"acct": "dave"},
    "media_attachments": []
  }
]
```

- [ ] **Step 2: Write failing tests for parse layer**

Create `internal/mastodon/client_test.go`:

```go
package mastodon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

func TestStatusesToPosts_originalBoostReply(t *testing.T) {
	pageBytes, err := os.ReadFile("testdata/page1.json")
	if err != nil {
		t.Fatal(err)
	}
	page2Bytes, err := os.ReadFile("testdata/page2.json")
	if err != nil {
		t.Fatal(err)
	}
	var statuses []status
	if err := json.Unmarshal(pageBytes, &statuses); err != nil {
		t.Fatal(err)
	}
	var more []status
	if err := json.Unmarshal(page2Bytes, &more); err != nil {
		t.Fatal(err)
	}
	statuses = append(statuses, more...)

	posts := statusesToPosts(statuses, "mastodon.social")
	if len(posts) != 3 {
		t.Fatalf("want 3 posts got %d", len(posts))
	}

	// Original
	if posts[0].Type != timeline.PostOriginal || posts[0].Author != "alice@mastodon.social" {
		t.Errorf("original: %+v", posts[0])
	}
	if posts[0].Text == "" || len(posts[0].MediaURLs) != 1 {
		t.Errorf("original text/media: %+v", posts[0])
	}
	// Boost: Author = booster, OriginalAuthor = carol, body & URL from reblog,
	// CreatedAt = time of boost action (the wrapper status), preserving timeline position.
	if posts[1].Type != timeline.PostRepost {
		t.Fatalf("boost type: %v", posts[1].Type)
	}
	if posts[1].Author != "bob@mastodon.social" || posts[1].OriginalAuthor != "carol@piaille.fr" {
		t.Errorf("boost authors: %+v", posts[1])
	}
	if posts[1].URL != "https://piaille.fr/@carol/999" {
		t.Errorf("boost URL: %s", posts[1].URL)
	}
	if !posts[1].CreatedAt.Equal(time.Date(2026, 4, 26, 14, 30, 0, 0, time.UTC)) {
		t.Errorf("boost CreatedAt should be the boost action time: %v", posts[1].CreatedAt)
	}
	// Reply
	if posts[2].Type != timeline.PostReply || posts[2].OriginalAuthor != "eve@mastodon.social" {
		t.Errorf("reply: %+v", posts[2])
	}
}

func TestFetchHomeTimeline_paginatesUntilSinceAndStops(t *testing.T) {
	page1, _ := os.ReadFile("testdata/page1.json")
	page2, _ := os.ReadFile("testdata/page2.json")
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		switch calls {
		case 1:
			w.Header().Set("Content-Type", "application/json")
			w.Write(page1)
		case 2:
			w.Header().Set("Content-Type", "application/json")
			w.Write(page2)
		default:
			w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token")
	since := time.Date(2026, 4, 26, 8, 0, 0, 0, time.UTC)
	posts, err := c.FetchHomeTimeline(context.Background(), since, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 3 {
		t.Fatalf("want 3 posts got %d", len(posts))
	}
}

func TestFetchHomeTimeline_respectsMaxPosts(t *testing.T) {
	page1, _ := os.ReadFile("testdata/page1.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(page1)
	}))
	defer srv.Close()
	c := New(srv.URL, "test-token")
	posts, err := c.FetchHomeTimeline(context.Background(), time.Time{}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("want 1 post got %d", len(posts))
	}
}

func TestFetchHomeTimeline_propagatesAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := New(srv.URL, "wrong")
	_, err := c.FetchHomeTimeline(context.Background(), time.Time{}, 0)
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
```

- [ ] **Step 3: Run tests, verify failure**

Run: `go test ./internal/mastodon/...`
Expected: compile error.

- [ ] **Step 4: Implement `parse.go`**

```go
package mastodon

import (
	"strings"
	"time"

	"github.com/jcgay/social-timeline/internal/htmltext"
	"github.com/jcgay/social-timeline/internal/timeline"
)

type account struct {
	ID   string `json:"id"`
	Acct string `json:"acct"`
}

type mention struct {
	ID   string `json:"id"`
	Acct string `json:"acct"`
}

type mediaAttachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type status struct {
	ID                  string            `json:"id"`
	CreatedAt           time.Time         `json:"created_at"`
	URL                 string            `json:"url"`
	Content             string            `json:"content"`
	InReplyToID         *string           `json:"in_reply_to_id"`
	InReplyToAccountID  *string           `json:"in_reply_to_account_id"`
	Mentions            []mention         `json:"mentions"`
	Account             account           `json:"account"`
	MediaAttachments    []mediaAttachment `json:"media_attachments"`
	Reblog              *status           `json:"reblog"`
}

// statusesToPosts maps a list of API statuses to neutral Posts.
// `instanceHost` is used to fully-qualify bare local handles ("alice"
// becomes "alice@mastodon.social").
func statusesToPosts(in []status, instanceHost string) []timeline.Post {
	out := make([]timeline.Post, 0, len(in))
	for _, s := range in {
		out = append(out, statusToPost(s, instanceHost))
	}
	return out
}

func statusToPost(s status, instanceHost string) timeline.Post {
	wrapperAuthor := qualify(s.Account.Acct, instanceHost)
	if s.Reblog != nil {
		// Mastodon boost: render the inner status's body/url/media but
		// keep the wrapper's CreatedAt so timeline position is correct.
		inner := s.Reblog
		return timeline.Post{
			Platform:       "mastodon",
			Author:         wrapperAuthor,
			CreatedAt:      s.CreatedAt.UTC(),
			Text:           htmltext.FromHTML(inner.Content),
			URL:            inner.URL,
			Type:           timeline.PostRepost,
			OriginalAuthor: qualify(inner.Account.Acct, instanceHost),
			MediaURLs:      mediaURLs(inner.MediaAttachments),
		}
	}
	p := timeline.Post{
		Platform:  "mastodon",
		Author:    wrapperAuthor,
		CreatedAt: s.CreatedAt.UTC(),
		Text:      htmltext.FromHTML(s.Content),
		URL:       s.URL,
		Type:      timeline.PostOriginal,
		MediaURLs: mediaURLs(s.MediaAttachments),
	}
	if s.InReplyToID != nil && *s.InReplyToID != "" {
		p.Type = timeline.PostReply
		p.OriginalAuthor = replyAuthor(s, instanceHost)
	}
	return p
}

func qualify(acct, instanceHost string) string {
	if acct == "" {
		return ""
	}
	if strings.Contains(acct, "@") {
		return acct
	}
	return acct + "@" + instanceHost
}

func mediaURLs(ms []mediaAttachment) []string {
	if len(ms) == 0 {
		return nil
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if m.URL != "" {
			out = append(out, m.URL)
		}
	}
	return out
}

func replyAuthor(s status, instanceHost string) string {
	if s.InReplyToAccountID == nil {
		return ""
	}
	for _, m := range s.Mentions {
		if m.ID == *s.InReplyToAccountID {
			return qualify(m.Acct, instanceHost)
		}
	}
	return ""
}
```

- [ ] **Step 5: Implement `client.go`**

```go
// Package mastodon implements timeline.Client against the Mastodon
// REST API (GET /api/v1/timelines/home).
package mastodon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

const (
	pageLimit  = 40
	httpTimeout = 30 * time.Second
)

// Client is a Mastodon timeline.Client. Use New to construct it.
type Client struct {
	baseURL     string // e.g. https://mastodon.social
	instanceHost string // e.g. mastodon.social
	token       string
	http        *http.Client
}

// New constructs a Client. baseURL must include scheme.
func New(baseURL, token string) *Client {
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil {
		host = u.Host
	}
	return &Client{
		baseURL:      baseURL,
		instanceHost: host,
		token:        token,
		http:         &http.Client{Timeout: httpTimeout},
	}
}

func (c *Client) Name() string { return "mastodon" }

func (c *Client) FetchHomeTimeline(ctx context.Context, since time.Time, maxPosts int) ([]timeline.Post, error) {
	var (
		all     []timeline.Post
		maxID   string
		stopped bool
	)
	for !stopped {
		page, err := c.fetchPage(ctx, maxID)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		converted := statusesToPosts(page, c.instanceHost)
		for _, p := range converted {
			if !since.IsZero() && p.CreatedAt.Before(since) {
				stopped = true
				continue
			}
			all = append(all, p)
			if maxPosts > 0 && len(all) >= maxPosts {
				return all[:maxPosts], nil
			}
		}
		// pagination uses the wrapper id of the oldest item in the page
		oldest := page[len(page)-1]
		if oldest.ID == "" || oldest.ID == maxID {
			break
		}
		maxID = oldest.ID
	}
	return all, nil
}

func (c *Client) fetchPage(ctx context.Context, maxID string) ([]status, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", pageLimit))
	if maxID != "" {
		q.Set("max_id", maxID)
	}
	endpoint := c.baseURL + "/api/v1/timelines/home?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("mastodon: GET timelines/home: status %d: %s", resp.StatusCode, body)
	}
	var page []status
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("mastodon: decode: %w", err)
	}
	return page, nil
}
```

- [ ] **Step 6: Run tests, verify pass**

Run: `go test ./internal/mastodon/... -v`
Expected: PASS for all four tests.

- [ ] **Step 7: Commit**

```bash
git add internal/mastodon/
git commit -m "feat(mastodon): home timeline client with pagination

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 7: `internal/bluesky` — Bluesky client

**Files:**
- Create: `internal/bluesky/parse.go`, `internal/bluesky/client.go`, `internal/bluesky/client_test.go`, `internal/bluesky/testdata/session.json`, `internal/bluesky/testdata/page1.json`, `internal/bluesky/testdata/page2.json`

### 7a — Fixtures

- [ ] **Step 1: Capture session and feed fixtures**

`internal/bluesky/testdata/session.json`:

```json
{
  "accessJwt": "fake-access-jwt",
  "refreshJwt": "fake-refresh-jwt",
  "handle": "alice.bsky.social",
  "did": "did:plc:alice"
}
```

`internal/bluesky/testdata/page1.json` — covers original w/ image, repost, reply, video. The shape mirrors `app.bsky.feed.getTimeline` responses (trimmed):

```json
{
  "cursor": "cursor-page-2",
  "feed": [
    {
      "post": {
        "uri": "at://did:plc:alice/app.bsky.feed.post/abc",
        "cid": "cid1",
        "author": {"handle": "alice.bsky.social"},
        "record": {"$type": "app.bsky.feed.post", "text": "original body", "createdAt": "2026-04-26T14:00:00.000Z"},
        "embed": {
          "$type": "app.bsky.embed.images#view",
          "images": [{"fullsize": "https://cdn.bsky.app/img/full/abc.jpg"}]
        },
        "indexedAt": "2026-04-26T14:00:00.000Z"
      }
    },
    {
      "post": {
        "uri": "at://did:plc:carol/app.bsky.feed.post/xyz",
        "cid": "cid2",
        "author": {"handle": "carol.bsky.social"},
        "record": {"$type": "app.bsky.feed.post", "text": "reposted body", "createdAt": "2026-04-26T10:00:00.000Z"},
        "indexedAt": "2026-04-26T10:00:00.000Z"
      },
      "reason": {
        "$type": "app.bsky.feed.defs#reasonRepost",
        "by": {"handle": "bob.bsky.social"},
        "indexedAt": "2026-04-26T14:30:00.000Z"
      }
    },
    {
      "post": {
        "uri": "at://did:plc:dave/app.bsky.feed.post/rep",
        "cid": "cid3",
        "author": {"handle": "dave.bsky.social"},
        "record": {
          "$type": "app.bsky.feed.post",
          "text": "reply body",
          "createdAt": "2026-04-26T14:25:00.000Z",
          "reply": {"parent": {"uri": "at://did:plc:eve/app.bsky.feed.post/par"}, "root": {"uri": "at://did:plc:eve/app.bsky.feed.post/root"}}
        },
        "embed": {
          "$type": "app.bsky.embed.video#view",
          "thumbnail": "https://cdn.bsky.app/img/video_thumb/rep.jpg",
          "playlist": "https://cdn.bsky.app/video/rep.m3u8"
        },
        "indexedAt": "2026-04-26T14:25:00.000Z"
      },
      "reply": {
        "parent": {"author": {"handle": "eve.bsky.social"}}
      }
    }
  ]
}
```

`internal/bluesky/testdata/page2.json` — empty cursor signals end:

```json
{"feed": []}
```

### 7b — Tests + implementation

- [ ] **Step 2: Write failing tests**

```go
package bluesky

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

func TestFeedToPosts_originalRepostReply(t *testing.T) {
	raw, err := os.ReadFile("testdata/page1.json")
	if err != nil {
		t.Fatal(err)
	}
	var page feedResponse
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	posts := feedToPosts(page.Feed)
	if len(posts) != 3 {
		t.Fatalf("want 3 posts got %d", len(posts))
	}

	if posts[0].Type != timeline.PostOriginal {
		t.Errorf("original type: %v", posts[0].Type)
	}
	if posts[0].URL != "https://bsky.app/profile/alice.bsky.social/post/abc" {
		t.Errorf("permalink: %s", posts[0].URL)
	}
	if len(posts[0].MediaURLs) != 1 {
		t.Errorf("media: %+v", posts[0].MediaURLs)
	}

	// Repost: Author=bob (reposter), OriginalAuthor=carol, Text/URL from inner post,
	// CreatedAt = repost action (reason.indexedAt).
	if posts[1].Type != timeline.PostRepost {
		t.Errorf("repost type: %v", posts[1].Type)
	}
	if posts[1].Author != "bob.bsky.social" || posts[1].OriginalAuthor != "carol.bsky.social" {
		t.Errorf("repost authors: %+v", posts[1])
	}
	if posts[1].URL != "https://bsky.app/profile/carol.bsky.social/post/xyz" {
		t.Errorf("repost URL: %s", posts[1].URL)
	}
	if !posts[1].CreatedAt.Equal(time.Date(2026, 4, 26, 14, 30, 0, 0, time.UTC)) {
		t.Errorf("repost CreatedAt should be reason.indexedAt: %v", posts[1].CreatedAt)
	}

	// Reply
	if posts[2].Type != timeline.PostReply || posts[2].OriginalAuthor != "eve.bsky.social" {
		t.Errorf("reply: %+v", posts[2])
	}
	// video uses thumbnail (not playlist)
	if len(posts[2].MediaURLs) != 1 || !strings.Contains(posts[2].MediaURLs[0], "video_thumb") {
		t.Errorf("video media should be thumbnail: %v", posts[2].MediaURLs)
	}
}

func TestFetchHomeTimeline_authThenPaginate(t *testing.T) {
	session, _ := os.ReadFile("testdata/session.json")
	page1, _ := os.ReadFile("testdata/page1.json")
	page2, _ := os.ReadFile("testdata/page2.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "alice.bsky.social") {
				http.Error(w, "bad creds", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(session)
		case "/xrpc/app.bsky.feed.getTimeline":
			if r.Header.Get("Authorization") != "Bearer fake-access-jwt" {
				http.Error(w, "no auth", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("cursor") == "" {
				w.Write(page1)
			} else {
				w.Write(page2)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "alice.bsky.social", "app-password")
	posts, err := c.FetchHomeTimeline(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 3 {
		t.Fatalf("want 3 got %d", len(posts))
	}
}

func TestFetchHomeTimeline_stopsAtSince(t *testing.T) {
	session, _ := os.ReadFile("testdata/session.json")
	page1, _ := os.ReadFile("testdata/page1.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			w.Write(session)
		case "/xrpc/app.bsky.feed.getTimeline":
			w.Write(page1)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "alice.bsky.social", "p")
	// since after the repost item's CreatedAt (14:30) but before the original (14:00) — wait,
	// chronological: original=14:00, reply=14:25, repost=14:30. since=14:20 keeps reply+repost.
	since := time.Date(2026, 4, 26, 14, 20, 0, 0, time.UTC)
	posts, err := c.FetchHomeTimeline(context.Background(), since, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range posts {
		if p.CreatedAt.Before(since) {
			t.Errorf("post older than since: %v", p)
		}
	}
}
```

- [ ] **Step 3: Run tests, verify failure**

Run: `go test ./internal/bluesky/...`
Expected: compile error.

- [ ] **Step 4: Implement `parse.go`**

```go
package bluesky

import (
	"strings"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

type author struct {
	Handle string `json:"handle"`
}

type postRecord struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	Reply     *struct {
		Parent struct {
			URI string `json:"uri"`
		} `json:"parent"`
	} `json:"reply"`
}

type embedView struct {
	Type      string  `json:"$type"`
	Images    []struct {
		Fullsize string `json:"fullsize"`
	} `json:"images"`
	Thumbnail string `json:"thumbnail"`
}

type feedPost struct {
	URI       string     `json:"uri"`
	Author    author     `json:"author"`
	Record    postRecord `json:"record"`
	Embed     *embedView `json:"embed"`
	IndexedAt time.Time  `json:"indexedAt"`
}

type repostReason struct {
	Type      string    `json:"$type"`
	By        author    `json:"by"`
	IndexedAt time.Time `json:"indexedAt"`
}

type replyContext struct {
	Parent struct {
		Author author `json:"author"`
	} `json:"parent"`
}

type feedItem struct {
	Post   feedPost      `json:"post"`
	Reason *repostReason `json:"reason,omitempty"`
	Reply  *replyContext `json:"reply,omitempty"`
}

type feedResponse struct {
	Cursor string     `json:"cursor"`
	Feed   []feedItem `json:"feed"`
}

func feedToPosts(items []feedItem) []timeline.Post {
	out := make([]timeline.Post, 0, len(items))
	for _, it := range items {
		out = append(out, itemToPost(it))
	}
	return out
}

func itemToPost(it feedItem) timeline.Post {
	innerPermalink := permalink(it.Post.Author.Handle, it.Post.URI)
	media := mediaFromEmbed(it.Post.Embed)

	if it.Reason != nil && strings.HasSuffix(it.Reason.Type, "reasonRepost") {
		// Repost: outer wrapper has reposter (Reason.By); inner post has the original content.
		return timeline.Post{
			Platform:       "bluesky",
			Author:         it.Reason.By.Handle,
			CreatedAt:      it.Reason.IndexedAt.UTC(),
			Text:           it.Post.Record.Text,
			URL:            innerPermalink,
			Type:           timeline.PostRepost,
			OriginalAuthor: it.Post.Author.Handle,
			MediaURLs:      media,
		}
	}
	p := timeline.Post{
		Platform:  "bluesky",
		Author:    it.Post.Author.Handle,
		CreatedAt: it.Post.Record.CreatedAt.UTC(),
		Text:      it.Post.Record.Text,
		URL:       innerPermalink,
		Type:      timeline.PostOriginal,
		MediaURLs: media,
	}
	if it.Post.Record.Reply != nil {
		p.Type = timeline.PostReply
		if it.Reply != nil {
			p.OriginalAuthor = it.Reply.Parent.Author.Handle
		}
	}
	return p
}

// permalink converts an at:// URI to a https://bsky.app/... URL.
// at://did:plc:alice/app.bsky.feed.post/abc → https://bsky.app/profile/alice.bsky.social/post/abc
func permalink(handle, atURI string) string {
	rkey := atURI
	if i := strings.LastIndex(atURI, "/"); i >= 0 {
		rkey = atURI[i+1:]
	}
	return "https://bsky.app/profile/" + handle + "/post/" + rkey
}

func mediaFromEmbed(e *embedView) []string {
	if e == nil {
		return nil
	}
	switch {
	case strings.Contains(e.Type, "embed.images"):
		out := make([]string, 0, len(e.Images))
		for _, img := range e.Images {
			if img.Fullsize != "" {
				out = append(out, img.Fullsize)
			}
		}
		return out
	case strings.Contains(e.Type, "embed.video"):
		if e.Thumbnail != "" {
			return []string{e.Thumbnail}
		}
	}
	return nil
}
```

- [ ] **Step 5: Implement `client.go`**

```go
// Package bluesky implements timeline.Client against the AT Protocol
// XRPC endpoints (com.atproto.server.createSession, app.bsky.feed.getTimeline).
package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

const (
	pageLimit   = 50
	httpTimeout = 30 * time.Second
)

type Client struct {
	baseURL  string // e.g. https://bsky.social
	handle   string
	password string
	http     *http.Client
}

func New(baseURL, handle, appPassword string) *Client {
	return &Client{
		baseURL:  baseURL,
		handle:   handle,
		password: appPassword,
		http:     &http.Client{Timeout: httpTimeout},
	}
}

func (c *Client) Name() string { return "bluesky" }

func (c *Client) FetchHomeTimeline(ctx context.Context, since time.Time, maxPosts int) ([]timeline.Post, error) {
	jwt, err := c.createSession(ctx)
	if err != nil {
		return nil, err
	}
	var (
		all     []timeline.Post
		cursor  string
		stopped bool
	)
	for !stopped {
		page, err := c.getTimeline(ctx, jwt, cursor)
		if err != nil {
			return nil, err
		}
		if len(page.Feed) == 0 {
			break
		}
		converted := feedToPosts(page.Feed)
		for _, p := range converted {
			if !since.IsZero() && p.CreatedAt.Before(since) {
				stopped = true
				continue
			}
			all = append(all, p)
			if maxPosts > 0 && len(all) >= maxPosts {
				return all[:maxPosts], nil
			}
		}
		if page.Cursor == "" || page.Cursor == cursor {
			break
		}
		cursor = page.Cursor
	}
	return all, nil
}

func (c *Client) createSession(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{"identifier": c.handle, "password": c.password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/xrpc/com.atproto.server.createSession", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("bluesky: createSession status %d: %s", resp.StatusCode, b)
	}
	var s struct {
		AccessJwt string `json:"accessJwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return "", fmt.Errorf("bluesky: decode session: %w", err)
	}
	if s.AccessJwt == "" {
		return "", fmt.Errorf("bluesky: createSession returned no accessJwt")
	}
	return s.AccessJwt, nil
}

func (c *Client) getTimeline(ctx context.Context, jwt, cursor string) (*feedResponse, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", pageLimit))
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/xrpc/app.bsky.feed.getTimeline?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("bluesky: getTimeline status %d: %s", resp.StatusCode, b)
	}
	var page feedResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("bluesky: decode feed: %w", err)
	}
	return &page, nil
}
```

- [ ] **Step 6: Run tests, verify pass**

Run: `go test ./internal/bluesky/... -v`
Expected: PASS for all three tests.

- [ ] **Step 7: Commit**

```bash
git add internal/bluesky/
git commit -m "feat(bluesky): home timeline client with session + pagination

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 8: `cmd/social-timeline` — CLI entrypoint

**Files:**
- Create: `cmd/social-timeline/main.go`, `cmd/social-timeline/main_test.go`

- [ ] **Step 1: Write failing CLI tests**

`cmd/social-timeline/main_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jcgay/social-timeline/internal/timeline"
)

type fakeClient struct {
	name  string
	posts []timeline.Post
	err   error
}

func (f *fakeClient) Name() string { return f.name }
func (f *fakeClient) FetchHomeTimeline(_ context.Context, _ time.Time, _ int) ([]timeline.Post, error) {
	return f.posts, f.err
}

func TestRun_mergesAndSortsAcrossClients(t *testing.T) {
	clients := []timeline.Client{
		&fakeClient{name: "bluesky", posts: []timeline.Post{
			{Platform: "bluesky", Author: "a", CreatedAt: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC), Text: "b1", URL: "u-b1"},
		}},
		&fakeClient{name: "mastodon", posts: []timeline.Post{
			{Platform: "mastodon", Author: "b", CreatedAt: time.Date(2026, 4, 26, 11, 0, 0, 0, time.UTC), Text: "m1", URL: "u-m1"},
		}},
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), runOpts{
		since:    time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
		maxPosts: 0,
		clients:  clients,
		stdout:   &stdout,
		stderr:   &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if strings.Index(out, "u-m1") > strings.Index(out, "u-b1") {
		t.Fatalf("posts not chronological:\n%s", out)
	}
}

func TestRun_partialFailureExitTwo(t *testing.T) {
	clients := []timeline.Client{
		&fakeClient{name: "bluesky", posts: []timeline.Post{
			{Platform: "bluesky", Author: "a", CreatedAt: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC), Text: "ok", URL: "u"},
		}},
		&fakeClient{name: "mastodon", err: errFailure("boom")},
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), runOpts{clients: clients, stdout: &stdout, stderr: &stderr})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stdout.String(), "🔗 u") {
		t.Fatalf("expected partial output to contain bluesky post:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "mastodon") {
		t.Fatalf("expected mastodon error on stderr:\n%s", stderr.String())
	}
}

func TestRun_allFailExitOne(t *testing.T) {
	clients := []timeline.Client{
		&fakeClient{name: "bluesky", err: errFailure("a")},
		&fakeClient{name: "mastodon", err: errFailure("b")},
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), runOpts{clients: clients, stdout: &stdout, stderr: &stderr})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

type errFailure string

func (e errFailure) Error() string { return string(e) }
```

- [ ] **Step 2: Run tests, verify failure**

Run: `go test ./cmd/social-timeline/...`
Expected: compile error (no `run`, `runOpts`).

- [ ] **Step 3: Implement `main.go`**

```go
// Command social-timeline aggregates Bluesky and Mastodon home
// timelines for a given window into a Markdown document on stdout.
//
// See docs/superpowers/specs/2026-04-26-social-timeline-design.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/jcgay/social-timeline/internal/bluesky"
	"github.com/jcgay/social-timeline/internal/mastodon"
	"github.com/jcgay/social-timeline/internal/render"
	"github.com/jcgay/social-timeline/internal/timeline"
	"github.com/jcgay/social-timeline/internal/timeparse"
)

const blueskyDefaultBaseURL = "https://bsky.social"

var version = "dev"

type runOpts struct {
	since    time.Time
	maxPosts int
	clients  []timeline.Client
	stdout   io.Writer
	stderr   io.Writer
	verbose  bool
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	fs := flag.NewFlagSet("social-timeline", flag.ContinueOnError)
	var (
		sinceStr   = fs.String("since", "", "Required. Duration (1d, 6h, 30m, 2w) or ISO-8601 date.")
		output     = fs.String("output", "", "Output file (default: stdout)")
		maxPosts   = fs.Int("max", 0, "Max posts per platform (0 = no limit)")
		platforms  = fs.String("platform", "", "Comma-separated platforms (bluesky,mastodon). Default: every configured platform.")
		verbose    = fs.Bool("verbose", false, "Verbose logging on stderr")
		showVer    = fs.Bool("version", false, "Print version and exit")
	)
	fs.StringVar(output, "o", "", "Output file (default: stdout) (shorthand)")
	fs.BoolVar(verbose, "v", false, "Verbose logging on stderr (shorthand)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return 1
	}
	if *showVer {
		fmt.Fprintln(os.Stdout, version)
		return 0
	}
	if *sinceStr == "" {
		fmt.Fprintln(os.Stderr, "error: --since is required")
		fs.SetOutput(os.Stderr)
		fs.Usage()
		return 1
	}
	since, err := timeparse.ParseSince(*sinceStr, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	clients, err := buildClients(*platforms, *verbose, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if len(clients) == 0 {
		fmt.Fprintln(os.Stderr, "error: no platform configured (set BLUESKY_* and/or MASTODON_* env vars)")
		return 1
	}

	var stdout io.Writer = os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: open %s: %v\n", *output, err)
			return 1
		}
		defer f.Close()
		stdout = f
	}

	return run(context.Background(), runOpts{
		since:    since,
		maxPosts: *maxPosts,
		clients:  clients,
		stdout:   stdout,
		stderr:   os.Stderr,
		verbose:  *verbose,
	})
}

// buildClients reads env vars and constructs the clients for every
// configured platform, optionally restricted by the --platform flag.
func buildClients(platformFilter string, verbose bool, stderr io.Writer) ([]timeline.Client, error) {
	wanted := map[string]bool{}
	if platformFilter != "" {
		for _, p := range strings.Split(platformFilter, ",") {
			p = strings.TrimSpace(strings.ToLower(p))
			switch p {
			case "bluesky", "mastodon":
				wanted[p] = true
			default:
				return nil, fmt.Errorf("--platform: unknown %q (allowed: bluesky, mastodon)", p)
			}
		}
	}

	var clients []timeline.Client

	bskyHandle := os.Getenv("BLUESKY_HANDLE")
	bskyPwd := os.Getenv("BLUESKY_APP_PASSWORD")
	if (len(wanted) == 0 || wanted["bluesky"]) && bskyHandle != "" && bskyPwd != "" {
		clients = append(clients, bluesky.New(blueskyDefaultBaseURL, bskyHandle, bskyPwd))
	} else if (len(wanted) == 0 || wanted["bluesky"]) && verbose {
		fmt.Fprintln(stderr, "skipping bluesky: BLUESKY_HANDLE / BLUESKY_APP_PASSWORD not set")
	}

	mastoURL := os.Getenv("MASTODON_INSTANCE_URL")
	mastoTok := os.Getenv("MASTODON_ACCESS_TOKEN")
	if (len(wanted) == 0 || wanted["mastodon"]) && mastoURL != "" && mastoTok != "" {
		clients = append(clients, mastodon.New(mastoURL, mastoTok))
	} else if (len(wanted) == 0 || wanted["mastodon"]) && verbose {
		fmt.Fprintln(stderr, "skipping mastodon: MASTODON_INSTANCE_URL / MASTODON_ACCESS_TOKEN not set")
	}

	return clients, nil
}

// run is the testable core: it fans out to clients, merges results,
// and writes Markdown. Returns the process exit code.
func run(ctx context.Context, opts runOpts) int {
	type result struct {
		name  string
		posts []timeline.Post
		err   error
	}
	results := make([]result, len(opts.clients))

	g, gctx := errgroup.WithContext(ctx)
	for i, c := range opts.clients {
		i, c := i, c
		g.Go(func() error {
			posts, err := c.FetchHomeTimeline(gctx, opts.since, opts.maxPosts)
			results[i] = result{name: c.Name(), posts: posts, err: err}
			return nil // never propagate, we want all clients to run
		})
	}
	_ = g.Wait()

	var (
		all      []timeline.Post
		failures []string
	)
	successes := 0
	for _, r := range results {
		if r.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", r.name, r.err))
			continue
		}
		successes++
		all = append(all, r.posts...)
	}
	for _, f := range failures {
		fmt.Fprintln(opts.stderr, "warning:", f)
	}
	if successes == 0 {
		return 1
	}
	merged := timeline.MergeSort(all)
	if err := render.Markdown(merged, opts.stdout); err != nil {
		fmt.Fprintf(opts.stderr, "error: write markdown: %v\n", err)
		return 1
	}
	if len(failures) > 0 {
		return 2
	}
	return 0
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./cmd/social-timeline/... -v`
Expected: PASS for all three tests.

- [ ] **Step 5: Run the full test suite + lint + build**

```bash
make test
make lint   # if golangci-lint is installed; otherwise skip
make build
./social-timeline --version
./social-timeline --help
```

Expected: tests pass, binary built, `--version` prints `dev`, `--help` lists every flag.

- [ ] **Step 6: Commit**

```bash
git add cmd/
git commit -m "feat(cli): main entrypoint, env config, parallel fetch, exit codes

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Task 9: README polish + smoke verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Expand `README.md`**

Replace the placeholder README with a complete one covering: install, env vars, every flag, exit codes, output format example (≈ §5 of spec), and a piping-into-LLM example. Mention that v1 supports only env-var credentials and that thread reconstruction is out of scope.

- [ ] **Step 2: (Optional) Smoke test with real credentials**

If you have credentials handy, export them and run:

```bash
./social-timeline --since 1h -v | head -50
```

Expected: real posts in the documented Markdown shape, exit 0.

- [ ] **Step 3: Final commit**

```bash
git add README.md
git commit -m "docs: complete README with usage and examples

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

## Test plan summary

By the end, the following are exercised:

| Concern                                 | Where                                        |
| --------------------------------------- | -------------------------------------------- |
| `--since` parsing (durations, ISO)      | `internal/timeparse/timeparse_test.go`       |
| Cross-platform sort + tie-break         | `internal/timeline/aggregate_test.go`        |
| HTML → plain text                       | `internal/htmltext/htmltext_test.go`         |
| Markdown rendering (all post types)     | `internal/render/render_test.go`             |
| Mastodon parse (original/boost/reply)   | `internal/mastodon/client_test.go`           |
| Mastodon HTTP, pagination, max, errors  | `internal/mastodon/client_test.go`           |
| Bluesky parse (original/repost/reply)   | `internal/bluesky/client_test.go`            |
| Bluesky session + pagination + since    | `internal/bluesky/client_test.go`            |
| CLI orchestration (sort, exit codes)    | `cmd/social-timeline/main_test.go`           |
