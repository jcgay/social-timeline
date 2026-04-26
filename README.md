# social-timeline

A Go CLI that aggregates your Bluesky and Mastodon home timelines into a single
Markdown document, intended to be piped into an LLM for ranking or summarization.

## Build

```bash
make build
# or
go build -o social-timeline ./cmd/social-timeline
```

## Credentials

Credentials are supplied via environment variables. You only need to set the
variables for the platforms you want to fetch.

| Variable | Platform | Description |
|---|---|---|
| `BLUESKY_HANDLE` | Bluesky | Your Bluesky handle, e.g. `alice.bsky.social` |
| `BLUESKY_APP_PASSWORD` | Bluesky | An [App Password](https://bsky.app/settings/app-passwords) (not your login password) |
| `MASTODON_INSTANCE_URL` | Mastodon | Base URL of your instance, e.g. `https://mastodon.social` |
| `MASTODON_ACCESS_TOKEN` | Mastodon | A user access token with `read:statuses` scope |

> **v1 note:** credentials are read from environment variables only; config-file
> or keychain support is out of scope for v1.

## Flags

| Flag | Default | Description |
|---|---|---|
| `--since` | *(required)* | Fetch posts newer than this point in time. Accepts a duration (`1d`, `6h`, `30m`, `2w`, `30s`) or an ISO-8601 date/datetime (`2026-04-25`, `2026-04-25T10:00:00Z`). |
| `--output`, `-o` | stdout | Write Markdown to a file instead of stdout. |
| `--max` | `0` | Maximum posts to return per platform. `0` means no limit. |
| `--platform` | *(all)* | Comma-separated list of platforms to include: `bluesky`, `mastodon`. |
| `--verbose`, `-v` | false | Print progress and debug info to stderr. |
| `--version` | — | Print version string and exit. |

## Usage

```bash
export BLUESKY_HANDLE=alice.bsky.social
export BLUESKY_APP_PASSWORD=xxxx-xxxx-xxxx-xxxx
export MASTODON_INSTANCE_URL=https://mastodon.social
export MASTODON_ACCESS_TOKEN=xxxx

# Last 24 hours, printed to stdout
./social-timeline --since 1d

# Last 6 hours, Bluesky only, max 50 posts, saved to a file
./social-timeline --since 6h --platform bluesky --max 50 --output timeline.md

# Since a specific date
./social-timeline --since 2026-04-25
```

## Pipe into an LLM

```bash
./social-timeline --since 1d | llm "pick the 5 most important posts and explain why"
```

Any LLM CLI that reads stdin works — [llm](https://llm.datasette.io),
[aichat](https://github.com/sigoden/aichat), etc.

## Output format

Each post is a `##` heading block:

```
## @alice.bsky.social — 2026-04-26 14:23 UTC [bluesky]
Hello world, this is my post text.
📎 https://cdn.bsky.app/img/photo.jpg
🔗 https://bsky.app/profile/alice.bsky.social/post/abc123

## @bob@mastodon.social — 2026-04-26 14:30 UTC [mastodon] (boost of @carol@piaille.fr)
The boosted post body text.
🔗 https://piaille.fr/@carol/999

## @dave.bsky.social — 2026-04-26 14:35 UTC [bluesky] (reply to @eve.bsky.social)
This is a reply post body.
🔗 https://bsky.app/profile/dave.bsky.social/post/xyz
```

Posts are sorted chronologically (oldest first). Each block ends with a blank line separator.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | All requested platforms fetched successfully. |
| `1` | Fatal error — no output produced. |
| `2` | Partial success — at least one platform failed but output for the remaining platform(s) was written. |

## Design

See `docs/superpowers/specs/2026-04-26-social-timeline-design.md` for the full
design spec.
