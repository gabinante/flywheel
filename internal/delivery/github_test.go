package delivery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParseGitHubRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		owner   string
		repo    string
		ok      bool
	}{
		{
			name:    "https",
			repoURL: "https://github.com/gabinante/flywheel",
			owner:   "gabinante",
			repo:    "flywheel",
			ok:      true,
		},
		{
			name:    "https git suffix",
			repoURL: "https://github.com/gabinante/flywheel.git",
			owner:   "gabinante",
			repo:    "flywheel",
			ok:      true,
		},
		{
			name:    "ssh",
			repoURL: "git@github.com:gabinante/flywheel.git",
			owner:   "gabinante",
			repo:    "flywheel",
			ok:      true,
		},
		{
			name:    "unsupported host",
			repoURL: "https://gitlab.com/gabinante/flywheel",
			ok:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubRepoURL(tt.repoURL)
			if ok != tt.ok {
				t.Fatalf("expected ok=%v, got %v", tt.ok, ok)
			}
			if owner != tt.owner || repo != tt.repo {
				t.Fatalf("expected %s/%s, got %s/%s", tt.owner, tt.repo, owner, repo)
			}
		})
	}
}

func TestGitHubPullRequestProvider_ListOpenPullRequests(t *testing.T) {
	provider := NewGitHubPullRequestProvider("test-token", "https://example.test")
	provider.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("expected bearer token, got %q", got)
			}
			if r.URL.Path != "/repos/gabinante/flywheel/pulls" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			body, err := json.Marshal([]map[string]any{
				{
					"number":     42,
					"title":      "Tighten delivery pipeline",
					"html_url":   "https://github.com/gabinante/flywheel/pull/42",
					"state":      "open",
					"draft":      false,
					"updated_at": "2026-04-23T12:00:00Z",
					"user": map[string]any{
						"login": "gabe",
					},
					"head": map[string]any{
						"ref": "ticket/warrant-42",
					},
					"base": map[string]any{
						"ref": "main",
					},
				},
			})
			if err != nil {
				t.Fatalf("marshal response: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil
		}),
	}
	repo, prs, err := provider.ListOpenPullRequests(context.Background(), "https://github.com/gabinante/flywheel")
	if err != nil {
		t.Fatalf("ListOpenPullRequests returned error: %v", err)
	}
	if repo != "gabinante/flywheel" {
		t.Fatalf("unexpected repo: %s", repo)
	}
	if len(prs) != 1 {
		t.Fatalf("expected one PR, got %d", len(prs))
	}
	if prs[0].Number != 42 || prs[0].HeadRef != "ticket/warrant-42" {
		t.Fatalf("unexpected PR payload: %#v", prs[0])
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
