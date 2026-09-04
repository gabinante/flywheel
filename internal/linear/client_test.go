package linear

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestIssuesUpdatedSincePaginates checks request shape, auth header, and cursor paging.
func TestIssuesUpdatedSincePaginates(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "lin_api_test" {
			t.Errorf("missing api key header")
		}
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls++
		filter, _ := body.Variables["filter"].(map[string]any)
		if _, ok := filter["updatedAt"]; !ok {
			t.Errorf("expected updatedAt filter, got %v", filter)
		}
		if calls == 1 {
			if _, has := body.Variables["after"]; has {
				t.Errorf("first page must not send a cursor")
			}
			_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i1","identifier":"RLETD-1","title":"a","priority":2,
				"state":{"id":"s","name":"In Review","type":"started"},"team":{"id":"t","key":"RLETD"},
				"labels":{"nodes":[{"name":"bug"}]},"attachments":{"nodes":[]}}],
				"pageInfo":{"hasNextPage":true,"endCursor":"c1"}}}}`))
			return
		}
		if body.Variables["after"] != "c1" {
			t.Errorf("second page cursor = %v, want c1", body.Variables["after"])
		}
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i2","identifier":"RLETD-2","title":"b","priority":0,
			"state":{"id":"s2","name":"Done","type":"completed"},"team":{"id":"t","key":"RLETD"},
			"labels":{"nodes":[]},"attachments":{"nodes":[]}}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`))
	}))
	defer srv.Close()

	c := NewClient("lin_api_test", srv.URL)
	var got []Issue
	since := mustTime("2026-09-01T00:00:00Z")
	if err := c.IssuesUpdatedSince(context.Background(), "proj", since, func(is Issue) error {
		got = append(got, is)
		return nil
	}); err != nil {
		t.Fatalf("IssuesUpdatedSince: %v", err)
	}
	if len(got) != 2 || got[0].Identifier != "RLETD-1" || got[1].Identifier != "RLETD-2" {
		t.Fatalf("got %+v", got)
	}
	if len(got[0].Labels) != 1 || got[0].Labels[0] != "bug" {
		t.Errorf("labels not flattened: %+v", got[0].Labels)
	}
	if calls != 2 {
		t.Errorf("expected 2 page requests, got %d", calls)
	}
}

func TestQueryGraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errors":[{"message":"Entity not found"}]}`))
	}))
	defer srv.Close()
	c := NewClient("k", srv.URL)
	if _, err := c.Viewer(context.Background()); err == nil {
		t.Fatal("expected GraphQL error")
	}
}

func TestQueryRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := NewClient("k", srv.URL)
	err := c.Query(context.Background(), `query { viewer { id } }`, nil, nil)
	if err == nil || !errorsIs(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}
