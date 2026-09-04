// Package linear makes Linear the ticket store: it discovers the Linear projects
// the operator leads, projects their issues onto Flywheel tickets, and pushes
// Flywheel-originated tickets, state changes, and comments back to Linear.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const DefaultEndpoint = "https://api.linear.app/graphql"

// ErrRateLimited is returned when Linear answers 429; callers should back off.
var ErrRateLimited = errors.New("linear: rate limited")

// Client is a minimal Linear GraphQL client authenticated with a personal API key.
type Client struct {
	apiKey   string
	endpoint string
	http     *http.Client
}

// NewClient returns a client for the given API key. endpoint may be empty.
func NewClient(apiKey, endpoint string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{apiKey: apiKey, endpoint: endpoint, http: &http.Client{Timeout: 30 * time.Second}}
}

// GraphQLError is a single error entry from a GraphQL response.
type GraphQLError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// Query executes a GraphQL operation and decodes data into out.
func (c *Client) Query(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		retry := resp.Header.Get("Retry-After")
		if secs, err := strconv.Atoi(retry); err == nil && secs > 0 {
			return fmt.Errorf("%w (retry after %ds)", ErrRateLimited, secs)
		}
		return ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("linear: HTTP %d: %s", resp.StatusCode, truncateBytes(raw, 400))
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []GraphQLError  `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("linear: decode response: %w", err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("linear: %s", env.Errors[0].Message)
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
