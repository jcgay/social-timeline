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

// Client is a Bluesky timeline.Client. Use New to construct it.
type Client struct {
	baseURL  string
	handle   string
	password string
	http     *http.Client
}

// New constructs a Client. baseURL must include scheme (default: https://bsky.social).
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
	body, err := json.Marshal(map[string]string{"identifier": c.handle, "password": c.password})
	if err != nil {
		return "", fmt.Errorf("bluesky: marshal session request: %w", err)
	}
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
