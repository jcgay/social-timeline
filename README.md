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
