package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultGitHubAPIBaseURL = "https://api.github.com"

type GitHubPullRequestProvider struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewGitHubPullRequestProvider(token, baseURL string) *GitHubPullRequestProvider {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultGitHubAPIBaseURL
	}
	return &GitHubPullRequestProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *GitHubPullRequestProvider) Name() string { return "github" }

func (p *GitHubPullRequestProvider) Supports(repoURL string) bool {
	_, _, ok := parseGitHubRepoURL(repoURL)
	return ok
}

func (p *GitHubPullRequestProvider) ListOpenPullRequests(ctx context.Context, repoURL string) (string, []PullRequest, error) {
	owner, repo, ok := parseGitHubRepoURL(repoURL)
	if !ok {
		return "", nil, fmt.Errorf("unsupported repository URL")
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&sort=updated&direction=desc&per_page=20",
		p.baseURL,
		url.PathEscape(owner),
		url.PathEscape(repo),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return owner + "/" + repo, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return owner + "/" + repo, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return owner + "/" + repo, nil, fmt.Errorf("github pull requests: %s", message)
	}

	var raw []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		HTMLURL   string `json:"html_url"`
		State     string `json:"state"`
		Draft     bool   `json:"draft"`
		UpdatedAt string `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return owner + "/" + repo, nil, err
	}

	prs := make([]PullRequest, 0, len(raw))
	for _, item := range raw {
		updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)
		prs = append(prs, PullRequest{
			Number:    item.Number,
			Title:     item.Title,
			URL:       item.HTMLURL,
			State:     item.State,
			Draft:     item.Draft,
			Author:    item.User.Login,
			HeadRef:   item.Head.Ref,
			BaseRef:   item.Base.Ref,
			UpdatedAt: updatedAt,
		})
	}
	return owner + "/" + repo, prs, nil
}

func parseGitHubRepoURL(repoURL string) (string, string, bool) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", "", false
	}
	if strings.HasPrefix(repoURL, "git@github.com:") {
		path := strings.TrimPrefix(repoURL, "git@github.com:")
		return splitGitHubPath(path)
	}
	if strings.HasPrefix(repoURL, "ssh://git@github.com/") {
		path := strings.TrimPrefix(repoURL, "ssh://git@github.com/")
		return splitGitHubPath(path)
	}
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", false
	}
	if !strings.EqualFold(u.Hostname(), "github.com") {
		return "", "", false
	}
	return splitGitHubPath(strings.TrimPrefix(u.Path, "/"))
}

func splitGitHubPath(path string) (string, string, bool) {
	path = strings.TrimSuffix(strings.TrimSpace(path), ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
