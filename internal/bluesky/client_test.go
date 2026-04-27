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

	if posts[2].Type != timeline.PostReply || posts[2].OriginalAuthor != "eve.bsky.social" {
		t.Errorf("reply: %+v", posts[2])
	}
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
			_, _ = w.Write(session)
		case "/xrpc/app.bsky.feed.getTimeline":
			if r.Header.Get("Authorization") != "Bearer fake-access-jwt" {
				http.Error(w, "no auth", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("cursor") == "" {
				_, _ = w.Write(page1)
			} else {
				_, _ = w.Write(page2)
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
			_, _ = w.Write(session)
		case "/xrpc/app.bsky.feed.getTimeline":
			_, _ = w.Write(page1)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "alice.bsky.social", "p")
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
