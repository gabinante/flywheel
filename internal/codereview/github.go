package codereview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitHub talks to GitHub through the operator's authenticated `gh` CLI, exactly
// as the operator does by hand today. No tokens are stored by Flywheel.
type GitHub struct {
	bin   string
	mu    sync.Mutex
	login string
}

// NewGitHub returns a client using the gh binary on PATH (or bin when set).
func NewGitHub(bin string) *GitHub {
	if bin == "" {
		bin = "gh"
	}
	return &GitHub{bin: bin}
}

func (g *GitHub) run(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, g.bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("gh %s: %s", strings.Join(args[:min(len(args), 3)], " "), truncateStr(msg, 500))
	}
	return out, nil
}

// Login returns the authenticated GitHub login (cached).
func (g *GitHub) Login(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.login != "" {
		return g.login, nil
	}
	out, err := g.run(ctx, nil, "api", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	g.login = strings.TrimSpace(string(out))
	return g.login, nil
}

// PRSummary is a search result row.
type PRSummary struct {
	Repo      string
	Number    int
	Title     string
	URL       string
	Author    string
	IsDraft   bool
	UpdatedAt time.Time
}

type searchRow struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	IsDraft    bool   `json:"isDraft"`
	UpdatedAt  string `json:"updatedAt"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

func (g *GitHub) search(ctx context.Context, extra ...string) ([]PRSummary, error) {
	args := append([]string{"search", "prs", "--state=open", "--limit", "100", "--json", "number,repository,title,url,author,updatedAt,isDraft"}, extra...)
	out, err := g.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	var rows []searchRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("gh search: decode: %w", err)
	}
	res := make([]PRSummary, 0, len(rows))
	for _, r := range rows {
		ts, _ := time.Parse(time.RFC3339, r.UpdatedAt)
		res = append(res, PRSummary{Repo: r.Repository.NameWithOwner, Number: r.Number, Title: r.Title, URL: r.URL, Author: r.Author.Login, IsDraft: r.IsDraft, UpdatedAt: ts})
	}
	return res, nil
}

// SearchReviewRequested lists open PRs where the operator's review is requested.
func (g *GitHub) SearchReviewRequested(ctx context.Context) ([]PRSummary, error) {
	return g.search(ctx, "--review-requested=@me")
}

// SearchReviewRequestedDirect lists open PRs where the operator personally (not via a
// team) is requested for review.
func (g *GitHub) SearchReviewRequestedDirect(ctx context.Context) ([]PRSummary, error) {
	return g.search(ctx, "user-review-requested:@me")
}

// SearchAuthoredOpen lists the operator's open PRs.
func (g *GitHub) SearchAuthoredOpen(ctx context.Context) ([]PRSummary, error) {
	return g.search(ctx, "--author=@me")
}

// PR is the detail view of a pull request.
type PR struct {
	Repo            string
	Number          int
	Title           string
	Body            string
	Author          string
	BaseRef         string
	HeadRef         string
	HeadSHA         string
	URL             string
	State           string // OPEN, CLOSED, MERGED
	IsDraft         bool
	ReviewDecision  string
	MyReviewState   string    // latest review by the operator: APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, or ""
	HeadCommittedAt time.Time // time of the newest commit on the branch (zero when unknown)
}

// ViewPR fetches PR details. login is used to find the operator's latest review state.
func (g *GitHub) ViewPR(ctx context.Context, repo string, number int, login string) (*PR, error) {
	out, err := g.run(ctx, nil, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json",
		"number,title,body,author,baseRefName,headRefName,headRefOid,url,state,isDraft,reviewDecision,latestReviews,commits")
	if err != nil {
		return nil, err
	}
	var v struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		Body        string `json:"body"`
		BaseRefName string `json:"baseRefName"`
		HeadRefName string `json:"headRefName"`
		HeadRefOid  string `json:"headRefOid"`
		URL         string `json:"url"`
		State       string `json:"state"`
		IsDraft     bool   `json:"isDraft"`
		ReviewDec   string `json:"reviewDecision"`
		Commits     []struct {
			CommittedDate string `json:"committedDate"`
		} `json:"commits"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		LatestReviews []struct {
			State  string `json:"state"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
		} `json:"latestReviews"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("gh pr view: decode: %w", err)
	}
	pr := &PR{Repo: repo, Number: v.Number, Title: v.Title, Body: v.Body, Author: v.Author.Login, BaseRef: v.BaseRefName,
		HeadRef: v.HeadRefName, HeadSHA: v.HeadRefOid, URL: v.URL, State: v.State, IsDraft: v.IsDraft, ReviewDecision: v.ReviewDec}
	for _, r := range v.LatestReviews {
		if strings.EqualFold(r.Author.Login, login) {
			pr.MyReviewState = r.State
		}
	}
	for _, c := range v.Commits {
		if t, err := time.Parse(time.RFC3339, c.CommittedDate); err == nil && t.After(pr.HeadCommittedAt) {
			pr.HeadCommittedAt = t
		}
	}
	return pr, nil
}

// Review is a submitted PR review.
type Review struct {
	ID          int64     `json:"id"`
	User        string    `json:"-"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	CommitID    string    `json:"commit_id"`
	HTMLURL     string    `json:"html_url"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// ListReviews returns all reviews on a PR, oldest first.
func (g *GitHub) ListReviews(ctx context.Context, repo string, number int) ([]Review, error) {
	out, err := g.run(ctx, nil, "api", "--paginate", fmt.Sprintf("repos/%s/pulls/%d/reviews", repo, number))
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Review
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	// --paginate concatenates JSON arrays; normalize "][" joins.
	norm := strings.ReplaceAll(string(out), "][", ",")
	if err := json.Unmarshal([]byte(norm), &raw); err != nil {
		return nil, fmt.Errorf("gh reviews: decode: %w", err)
	}
	res := make([]Review, 0, len(raw))
	for _, r := range raw {
		rv := r.Review
		rv.User = r.User.Login
		res = append(res, rv)
	}
	return res, nil
}

// CountReviewComments returns the number of inline comments attached to a review.
func (g *GitHub) CountReviewComments(ctx context.Context, repo string, number int, reviewID int64) int {
	out, err := g.run(ctx, nil, "api", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/comments", repo, number, reviewID), "--jq", "length")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

// Diff returns the PR's unified diff.
func (g *GitHub) Diff(ctx context.Context, repo string, number int) (string, error) {
	out, err := g.run(ctx, nil, "pr", "diff", strconv.Itoa(number), "--repo", repo)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ReviewComment is an inline comment in a review submission.
type ReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
}

// ReviewInput is a review submission.
type ReviewInput struct {
	CommitID string          `json:"commit_id,omitempty"`
	Event    string          `json:"event"` // APPROVE | REQUEST_CHANGES | COMMENT
	Body     string          `json:"body"`
	Comments []ReviewComment `json:"comments,omitempty"`
}

// PostedReview is the response to a review submission.
type PostedReview struct {
	ID      int64  `json:"id"`
	State   string `json:"state"`
	HTMLURL string `json:"html_url"`
}

// CreateReview submits one review with inline comments.
func (g *GitHub) CreateReview(ctx context.Context, repo string, number int, in ReviewInput) (*PostedReview, error) {
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	out, err := g.run(ctx, payload, "api", "-X", "POST", fmt.Sprintf("repos/%s/pulls/%d/reviews", repo, number), "--input", "-")
	if err != nil {
		return nil, err
	}
	var pr PostedReview
	if err := json.Unmarshal(out, &pr); err != nil {
		return nil, fmt.Errorf("gh review: decode: %w", err)
	}
	return &pr, nil
}

// ListReviewCommentIDs returns inline comment ids for a review, in order.
func (g *GitHub) ListReviewCommentIDs(ctx context.Context, repo string, number int, reviewID int64) ([]int64, error) {
	out, err := g.run(ctx, nil, "api", fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/comments", repo, number, reviewID), "--jq", "[.[].id]")
	if err != nil {
		return nil, err
	}
	var ids []int64
	if err := json.Unmarshal(out, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// MergedPR is a merged pull request authored by the operator.
type MergedPR struct {
	Repo     string
	Number   int
	Title    string
	URL      string
	MergedAt time.Time
}

// SearchMergedSince lists the operator's PRs merged since the given time, optionally in one repo.
func (g *GitHub) SearchMergedSince(ctx context.Context, since time.Time, repo string) ([]MergedPR, error) {
	args := []string{"search", "prs", "--author=@me", "--merged", "--merged-at", ">=" + since.UTC().Format("2006-01-02"), "--limit", "100",
		"--json", "number,repository,title,url,closedAt"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := g.run(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Number     int    `json:"number"`
		Title      string `json:"title"`
		URL        string `json:"url"`
		ClosedAt   string `json:"closedAt"`
		Repository struct {
			NameWithOwner string `json:"nameWithOwner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("gh search merged: decode: %w", err)
	}
	res := make([]MergedPR, 0, len(rows))
	for _, r := range rows {
		ts, _ := time.Parse(time.RFC3339, r.ClosedAt)
		if !ts.IsZero() && ts.Before(since) {
			continue
		}
		res = append(res, MergedPR{Repo: r.Repository.NameWithOwner, Number: r.Number, Title: r.Title, URL: r.URL, MergedAt: ts})
	}
	return res, nil
}

// SearchOpenAuthored lists the operator's open PRs, optionally in one repo.
func (g *GitHub) SearchOpenAuthored(ctx context.Context, repo string) ([]PRSummary, error) {
	if repo == "" {
		return g.search(ctx, "--author=@me")
	}
	return g.search(ctx, "--author=@me", "--repo", repo)
}
