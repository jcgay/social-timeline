// Package mastodon implements timeline.Client against the Mastodon
// REST API (GET /api/v1/timelines/home).
package mastodon

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
	pageLimit   = 40
	httpTimeout = 30 * time.Second
)

// Client is a Mastodon timeline.Client. Use New to construct it.
type Client struct {
	baseURL      string
	instanceHost string
	token        string
	http         *http.Client
}

// New constructs a Client. baseURL must include scheme.
func New(baseURL, token string) *Client {
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil {
		host = u.Host
	}
	return &Client{
		baseURL:      baseURL,
		instanceHost: host,
		token:        token,
		http:         &http.Client{Timeout: httpTimeout},
	}
}

func (c *Client) Name() string { return "mastodon" }

func (c *Client) FetchHomeTimeline(ctx context.Context, since time.Time, maxPosts int) ([]timeline.Post, error) {
	var (
		all     []timeline.Post
		maxID   string
		stopped bool
	)
	for !stopped {
		page, err := c.fetchPage(ctx, maxID)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		converted := statusesToPosts(page, c.instanceHost)
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
		oldest := page[len(page)-1]
		if oldest.ID == "" || oldest.ID == maxID {
			break
		}
		maxID = oldest.ID
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
		resp.Body.Close()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delays[attempt]):
		}
	}
}

func (c *Client) fetchPage(ctx context.Context, maxID string) ([]status, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", pageLimit))
	if maxID != "" {
		q.Set("max_id", maxID)
	}
	endpoint := c.baseURL + "/api/v1/timelines/home?" + q.Encode()
	resp, err := c.doWithRetry(ctx, http.MethodGet, endpoint, nil,
		map[string]string{
			"Authorization": "Bearer " + c.token,
			"Accept":        "application/json",
		})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("mastodon: GET timelines/home: status %d: %s", resp.StatusCode, body)
	}
	var page []status
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("mastodon: decode: %w", err)
	}
	return page, nil
}
