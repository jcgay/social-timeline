package timeline

import (
	"testing"
	"time"
)

func TestMergeSort_chronologicalAcrossPlatforms(t *testing.T) {
	t1 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 1, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	b1 := Post{Platform: "bluesky", CreatedAt: t1, URL: "https://bsky.app/1"}
	m2 := Post{Platform: "mastodon", CreatedAt: t2, URL: "https://mastodon.social/2"}
	b3 := Post{Platform: "bluesky", CreatedAt: t3, URL: "https://bsky.app/3"}

	got := MergeSort([]Post{b1, b3}, []Post{m2})
	if len(got) != 3 {
		t.Fatalf("want 3 posts got %d", len(got))
	}
	if got[0].URL != b1.URL || got[1].URL != m2.URL || got[2].URL != b3.URL {
		t.Errorf("wrong order: %v %v %v", got[0].URL, got[1].URL, got[2].URL)
	}
}

func TestMergeSort_tieBreakByPlatformThenURL(t *testing.T) {
	ts := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	posts := []Post{
		{Platform: "mastodon", CreatedAt: ts, URL: "https://mastodon.social/z"},
		{Platform: "bluesky", CreatedAt: ts, URL: "https://bsky.app/b"},
		{Platform: "mastodon", CreatedAt: ts, URL: "https://mastodon.social/a"},
		{Platform: "bluesky", CreatedAt: ts, URL: "https://bsky.app/a"},
	}
	got := MergeSort(posts)
	want := []string{
		"https://bsky.app/a",
		"https://bsky.app/b",
		"https://mastodon.social/a",
		"https://mastodon.social/z",
	}
	for i, w := range want {
		if got[i].URL != w {
			t.Errorf("[%d] got %s want %s", i, got[i].URL, w)
		}
	}
}

func TestMergeSort_emptyInputs(t *testing.T) {
	if got := MergeSort(); len(got) != 0 {
		t.Errorf("want empty got %v", got)
	}
	if got := MergeSort(nil, nil); len(got) != 0 {
		t.Errorf("want empty got %v", got)
	}
}
