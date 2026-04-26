// Package timeline defines the platform-agnostic data model shared across
// all social network clients.
package timeline

import (
	"context"
	"time"
)

// PostType classifies a post for rendering purposes.
type PostType int

const (
	PostOriginal PostType = iota
	PostRepost
	PostReply
)

// Post is a platform-neutral representation of a single timeline entry.
type Post struct {
	Platform       string
	Author         string
	CreatedAt      time.Time
	Text           string
	URL            string
	Type           PostType
	OriginalAuthor string
	MediaURLs      []string
}

// Client is the interface that every platform adapter must implement.
type Client interface {
	Name() string
	FetchHomeTimeline(ctx context.Context, since time.Time, maxPosts int) ([]Post, error)
}
