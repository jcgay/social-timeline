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
