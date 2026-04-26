# social-timeline — Design

## 1. Overview

`social-timeline` is a Go command-line tool that aggregates the home
timelines of a user's Bluesky and Mastodon accounts over a given time
window and emits a single Markdown document on stdout. The output is
optimized for downstream consumption by an LLM that selects the most
important posts.

Typical usage:

```bash
social-timeline --since 1d | llm "pick the 10 most important posts"
social-timeline --since 2026-04-25 -o digest.md
```

## 2. Goals and non-goals

**Goals**

- Fetch the authenticated user's home timeline from Bluesky and Mastodon
  for a configurable time window.
- Produce clean, structured Markdown suitable for an LLM pipeline.
- Ship as a single static binary with no runtime dependency.

**Non-goals (for v1)**

- No persistence, deduplication, or incremental sync between runs.
- No thread reconstruction (each post is rendered standalone).
- No support for notifications, lists, hashtags, or non-home feeds.
- No publishing, replying, or any write operation.
- No secret management beyond environment variables (no keychain, no
  config file).
- No `--until` upper bound; the window is always `[since, now]`.

## 3. Command-line interface

```
social-timeline [flags]

Flags:
  --since STRING     Required. Either a relative duration (1d, 6h, 2w, 30m)
                     or an ISO-8601 date/datetime (2026-04-25 or
                     2026-04-25T10:00:00Z). Naive dates are interpreted as
                     UTC.
  -o, --output PATH  Output file. Defaults to stdout.
  --max N            Maximum number of posts per platform. No limit by
                     default.
  --platform LIST    Comma-separated list of platforms to query
                     (bluesky, mastodon). Defaults to every platform
                     that has complete credentials configured.
  -v, --verbose      Emit progress and pagination logs to stderr.
  --version          Print the version and exit.
  -h, --help         Print help and exit.
```

**Exit codes**

- `0` — full success: every requested platform returned its window.
- `1` — fatal error: invalid configuration, invalid `--since`, or no
  platform could be queried at all.
- `2` — partial success: at least one platform was queried successfully
  and at least one platform failed. Output still contains the posts that
  were retrieved.

## 4. Configuration

Credentials and endpoints are read exclusively from environment
variables in v1.

```
BLUESKY_HANDLE           e.g. alice.bsky.social
BLUESKY_APP_PASSWORD     app password generated in Bluesky settings

MASTODON_INSTANCE_URL    e.g. https://mastodon.social
MASTODON_ACCESS_TOKEN    access token created via a Mastodon application
```

Rules:

- A platform is considered "configured" only when **all** of its
  variables are non-empty.
- If no platform is configured: exit code 1.
- If only one platform is configured: query only that one. In verbose
  mode, log a notice on stderr explaining why the other platform is
  skipped.
- `--platform` may further restrict the set, but cannot enable a
  platform that is not configured.

## 5. Output format

Posts from both platforms are merged and sorted by `created_at`
ascending (oldest first). Ties are broken by platform name (`bluesky`
before `mastodon`) and then by `URL`, so output is fully deterministic.
Posts are then rendered as a sequence of `##` blocks.

```markdown
## @alice.bsky.social — 2026-04-26 14:23 UTC [bluesky]
Original post body. Multiple lines are preserved.
Inline links are kept as-is: https://example.com
🔗 https://bsky.app/profile/alice.bsky.social/post/xyz

## @bob@mastodon.social — 2026-04-26 14:25 UTC [mastodon] (boost of @carol@piaille.fr)
Body of the boosted post.
🔗 https://piaille.fr/@carol/123456

## @dave.bsky.social — 2026-04-26 14:30 UTC [bluesky] (reply to @eve.bsky.social)
Body of the reply.
📎 https://cdn.bsky.app/img/feed_thumbnail/abc.jpg
🔗 https://bsky.app/profile/dave.bsky.social/post/abc
```

**Rules**

- Heading line: `## @author — YYYY-MM-DD HH:MM UTC [platform]` followed
  by an optional type marker in parentheses.
- Type markers: `(repost of @x)` or `(boost of @x)` for Bluesky reposts
  and Mastodon boosts; `(reply to @x)` for replies. Original posts have
  no marker.
- For a repost/boost the body shown is the **original post's body**, not
  any commentary by the reposter (Bluesky reposts have no commentary;
  Mastodon boosts likewise carry no extra text).
- Mastodon HTML content is converted to plain text: paragraphs become
  blank lines, `<br>` becomes a newline, `<a href="...">label</a>`
  becomes `label (url)` when the label differs from the URL, otherwise
  just the URL.
- Bluesky text is already plain; facets (link/mention spans) are kept as
  inline plain text. Mention facets render as `@handle`.
- Media: image and video URLs are listed at the bottom of the post as
  `📎 <url>`, one per line, before the permalink.
- Permalink line is always last: `🔗 <url>`.
- Empty bodies (e.g. media-only posts) render with an empty body line
  followed by the media and permalink lines.
- Blocks are separated by a single blank line.
- If no posts match the window, the tool emits an empty document (no
  error) and exits 0.

## 6. Time window semantics

- `--since` accepts either a duration or an ISO-8601 timestamp.
- Supported duration units: `s`, `m`, `h`, `d`, `w`. A leading integer
  is required (`1d`, `6h`, `2w`, `30m`). No fractional values. `d` and
  `w` are not standard `time.ParseDuration` units, so parsing is
  implemented in `internal/timeparse`. `1d` means exactly 24 hours and
  `1w` means exactly 7×24 hours; calendar-aware semantics are out of
  scope.
- A duration `D` is resolved as `now() - D`.
- A naive date `2026-04-25` is interpreted as `2026-04-25T00:00:00Z`.
- A post is included iff `since <= created_at <= now()`. The upper
  bound `now()` is captured once at startup.

## 7. Internal architecture

```
cmd/social-timeline/      main, CLI parsing
internal/timeline/        neutral Post type, aggregation, sort
internal/bluesky/         Bluesky client (com.atproto.* + app.bsky.*)
internal/mastodon/        Mastodon client (REST /api/v1/timelines/home)
internal/render/          Markdown renderer
internal/timeparse/       parser for --since
internal/htmltext/        HTML → plain text helper for Mastodon
```

### 7.1 Neutral domain type

```go
type PostType int

const (
    PostOriginal PostType = iota
    PostRepost            // Bluesky repost or Mastodon boost
    PostReply
)

type Post struct {
    Platform        string    // "bluesky" | "mastodon"
    Author          string    // "alice.bsky.social" or "bob@mastodon.social"
    CreatedAt       time.Time // UTC
    Text            string    // plain text body (already HTML-stripped)
    URL             string    // permalink to the canonical post
    Type            PostType
    OriginalAuthor  string    // set for Repost/Reply, empty otherwise
    MediaURLs       []string
}
```

The `Post` type is the only structure that crosses package boundaries
between platform clients and the renderer. Platform-specific API
structs never leak out of `internal/bluesky` or `internal/mastodon`.

The `Client` interface (see §7.2) is declared in `internal/timeline`
to avoid an import cycle: platform packages import `timeline` to use
`Post` and to satisfy `Client`, never the other way around.

### 7.2 Client contract

Each platform client exposes a single function:

```go
type Client interface {
    FetchHomeTimeline(ctx context.Context, since time.Time, maxPosts int) ([]timeline.Post, error)
}
```

Implementation rules:

- Pagination is internal to the client. The client keeps requesting
  pages until either:
  - the oldest post in the latest page is `< since` (we then drop
    posts older than `since` and stop), or
  - the cumulative number of returned posts reaches `maxPosts`
    (`maxPosts == 0` means no limit), or
  - the platform reports the end of the timeline.
- Reposts/boosts in the feed produce a single `Post` whose `Author` is
  the reposter, `OriginalAuthor` is the original author, `Text`/`URL`/
  `MediaURLs` come from the original post, and `CreatedAt` is the
  **time of the repost action** (so the repost appears at the position
  the user actually saw it in their timeline).
- Replies in the feed are surfaced as `PostReply` with `OriginalAuthor`
  set to the parent post's author.
- HTTP client uses a sane timeout (e.g. 30s per request) and a small
  backoff on 5xx (3 retries max).

### 7.3 Concurrency and error handling

`main` runs the configured clients in parallel via `errgroup` (or
equivalent). Per-platform errors are recorded but do not cancel the
sibling fetches. After both finish:

- If every platform returned an error → exit 1, no output.
- If at least one platform succeeded and at least one failed → render
  the successful posts, log the failures on stderr, exit 2.
- If every platform succeeded → exit 0.

### 7.4 Bluesky specifics

- Auth: `com.atproto.server.createSession` with handle + app password,
  yielding an access JWT used for subsequent requests.
- Feed: `app.bsky.feed.getTimeline` with `cursor` pagination.
- Permalink construction: `https://bsky.app/profile/<handle>/post/<rkey>`
  where `rkey` is the last segment of the post's AT-URI.
- Embeds: pull image URLs from `embed.images[].fullsize`. For video
  embeds (`embed.video`), use the thumbnail URL (still image) rather
  than the HLS playlist, so media URLs are consistently directly
  viewable.

### 7.5 Mastodon specifics

- Auth: bearer token in `Authorization` header.
- Feed: `GET /api/v1/timelines/home` with `max_id` pagination and
  `limit=40`.
- Reblogs (`reblog` field non-null) → `PostRepost`; `in_reply_to_id`
  non-null → `PostReply`; otherwise `PostOriginal`.
- Author handle: `acct` field. For local users `acct` is bare username,
  so we append `@<instance-host>` to keep handles fully-qualified in
  output.
- Media: pull `url` from `media_attachments[]`.

## 8. Testing strategy

- Pure-function unit tests for `timeparse`, `render`, and `htmltext`.
- For each platform client, table-driven tests that feed captured JSON
  fixtures into a `httptest.Server` and assert the resulting
  `[]Post`. Fixtures cover: original post, repost/boost, reply, post
  with images, post with empty body, paginated response, end-of-feed.
- Aggregation tests on `internal/timeline` covering sort order across
  platforms and partial-failure handling.
- No live API tests in CI. A `make smoke` target may invoke the binary
  against real APIs locally when credentials are present.

## 9. Build and distribution

- Single module, Go 1.22+.
- `go build ./cmd/social-timeline` produces a static binary.
- `Makefile` targets: `build`, `test`, `lint` (golangci-lint), `fmt`.
- No release automation in v1; cross-compilation is left to ad-hoc
  `GOOS`/`GOARCH` invocations.

## 10. Open questions deferred to later versions

- Secret storage beyond env vars (config file, keychain).
- Incremental sync / state file to avoid refetching across runs.
- Thread reconstruction for replies.
- Additional sources (notifications, lists, hashtags).
- `--until` upper bound for arbitrary historical windows.
