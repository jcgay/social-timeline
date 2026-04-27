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

// doWithRetry executes an HTTP request, retrying up to 3 times on 5xx responses
// with exponential backoff (1s, 2s, 4s). Network errors and 4xx responses are
// not retried. bodyBytes may be nil for requests without a body.
func (c *Client) doWithRetry(ctx context.Context, method, url string, bodyBytes []byte, headers map[string]string) (*http.Response, error) {
	const maxRetries = 3
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	for attempt := 0; ; attempt++ {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 5 || attempt >= maxRetries {
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delays[attempt]):
		}
	}
}

func (c *Client) createSession(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]string{"identifier": c.handle, "password": c.password})
	if err != nil {
		return "", fmt.Errorf("bluesky: marshal session request: %w", err)
	}
	resp, err := c.doWithRetry(ctx, http.MethodPost,
		c.baseURL+"/xrpc/com.atproto.server.createSession", body,
		map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
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
	resp, err := c.doWithRetry(ctx, http.MethodGet,
		c.baseURL+"/xrpc/app.bsky.feed.getTimeline?"+q.Encode(), nil,
		map[string]string{"Authorization": "Bearer " + jwt})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
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
