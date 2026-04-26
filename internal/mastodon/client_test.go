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
	// Boost
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
