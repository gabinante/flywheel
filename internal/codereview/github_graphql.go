package codereview

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PRDetail is a rich search row from the GraphQL search API — enough to render a
// cross-repo PR list without one request per PR.
type PRDetail struct {
	Repo               string
	Number             int
	Title              string
	URL                string
	Author             string
	IsDraft            bool
	State              string // OPEN, CLOSED, MERGED
	HeadRef            string
	BaseRef            string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MergedAt           *time.Time
	ReviewDecision     string // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Checks             string // SUCCESS, FAILURE, PENDING, ERROR, EXPECTED, ""
	Mergeable          string // not fetched (expensive); kept for the API shape
	Additions          int    // not fetched (expensive); kept for the API shape
	Deletions          int
	ChangedFiles       int
	Labels             []string
	Reviews            []ReviewerState // latest review per reviewer
	RequestedReviewers []string
}

// ReviewerState is the latest review by one reviewer.
type ReviewerState struct {
	Login       string
	State       string
	SubmittedAt *time.Time
}

const prSearchQuery = `query($q: String!, $n: Int!) {
  search(query: $q, type: ISSUE, first: $n) {
    nodes {
      ... on PullRequest {
        number title url isDraft state createdAt updatedAt mergedAt headRefName baseRefName
        reviewDecision
        author { login }
        repository { nameWithOwner }
        labels(first: 10) { nodes { name } }
        commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
        latestReviews(first: 20) { nodes { author { login } state submittedAt } }
        reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } ... on Team { name } } } }
      }
    }
  }
}`

type gqlPRNode struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	IsDraft        bool   `json:"isDraft"`
	State          string `json:"state"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	MergedAt       string `json:"mergedAt"`
	HeadRefName    string `json:"headRefName"`
	BaseRefName    string `json:"baseRefName"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	ChangedFiles   int    `json:"changedFiles"`
	Mergeable      string `json:"mergeable"`
	ReviewDecision string `json:"reviewDecision"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	LatestReviews struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			State       string `json:"state"`
			SubmittedAt string `json:"submittedAt"`
		} `json:"nodes"`
	} `json:"latestReviews"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
}

// SearchPRs runs a GitHub search (e.g. "is:pr is:open author:@me") through the GraphQL
// API and returns rich rows. limit is capped at 100 by the API.
func (g *GitHub) SearchPRs(ctx context.Context, query string, limit int) ([]PRDetail, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var out []byte
	var err error
	for attempt := 0; attempt < 3; attempt++ { // GitHub's search backend 502s now and then
		out, err = g.run(ctx, nil, "api", "graphql", "-f", "query="+prSearchQuery, "-f", "q="+query, "-F", "n="+strconv.Itoa(limit))
		if err == nil || ctx.Err() != nil || !strings.Contains(err.Error(), "HTTP 5") {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 1500 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Search struct {
				Nodes []gqlPRNode `json:"nodes"`
			} `json:"search"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("gh api graphql: decode: %w", err)
	}
	if len(resp.Errors) > 0 && len(resp.Data.Search.Nodes) == 0 {
		return nil, fmt.Errorf("gh api graphql: %s", resp.Errors[0].Message)
	}
	res := make([]PRDetail, 0, len(resp.Data.Search.Nodes))
	for _, n := range resp.Data.Search.Nodes {
		if n.Number == 0 || n.Repository.NameWithOwner == "" {
			continue
		}
		d := PRDetail{
			Repo: n.Repository.NameWithOwner, Number: n.Number, Title: n.Title, URL: n.URL, Author: n.Author.Login,
			IsDraft: n.IsDraft, State: n.State, HeadRef: n.HeadRefName, BaseRef: n.BaseRefName,
			CreatedAt: parseTime(n.CreatedAt), UpdatedAt: parseTime(n.UpdatedAt), ReviewDecision: n.ReviewDecision,
			Mergeable: n.Mergeable, Additions: n.Additions, Deletions: n.Deletions, ChangedFiles: n.ChangedFiles,
		}
		if n.MergedAt != "" {
			t := parseTime(n.MergedAt)
			d.MergedAt = &t
		}
		for _, l := range n.Labels.Nodes {
			d.Labels = append(d.Labels, l.Name)
		}
		if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			d.Checks = n.Commits.Nodes[0].Commit.StatusCheckRollup.State
		}
		for _, r := range n.LatestReviews.Nodes {
			rs := ReviewerState{Login: r.Author.Login, State: r.State}
			if r.SubmittedAt != "" {
				t := parseTime(r.SubmittedAt)
				rs.SubmittedAt = &t
			}
			d.Reviews = append(d.Reviews, rs)
		}
		for _, r := range n.ReviewRequests.Nodes {
			if name := firstNonEmpty(r.RequestedReviewer.Login, r.RequestedReviewer.Name); name != "" {
				d.RequestedReviewers = append(d.RequestedReviewers, name)
			}
		}
		res = append(res, d)
	}
	return res, nil
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, strings.TrimSpace(s))
	return t
}
