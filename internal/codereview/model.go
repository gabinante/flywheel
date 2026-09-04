// Package codereview implements PR-keyed code review: pasted or review-requested
// pull requests are reviewed in depth by a local harness (Codex by default), the
// findings are posted as one GitHub review with inline conversational comments,
// and the PR is approved unless a P0/P1 finding blocks it. Reviewed PRs are watched
// for new commits or dismissed reviews; the operator's own PRs are watched for
// landed reviews so an agent can address the comments.
package codereview

import "time"

// State of a review request.
type State string

const (
	StateQueued           State = "queued"
	StateFetching         State = "fetching"
	StateReviewing        State = "reviewing"
	StatePublishing       State = "publishing"
	StateApproved         State = "approved"
	StateChangesRequested State = "changes_requested"
	StateCommented        State = "commented" // dry run or comment-only outcome
	StateWatching         State = "watching"  // reviewed; waiting for new commits or a dismissal
	StateSuperseded       State = "superseded"
	StateClosed           State = "closed"
	StateFailed           State = "failed"
)

// Origin says how a request entered the queue.
type Origin string

const (
	OriginPaste           Origin = "paste"
	OriginReviewRequested Origin = "review_requested"
	OriginReReview        Origin = "re_review"
	OriginMCP             Origin = "mcp"
)

// RecipeInlineP1Gate is the single supported recipe: in-depth review, inline
// conversational comments, P0/P1 → request changes, otherwise approve.
const RecipeInlineP1Gate = "inline_conversational_p1_gate"

// Verdicts.
const (
	VerdictApprove        = "approve"
	VerdictRequestChanges = "request_changes"
	VerdictComment        = "comment"
)

// Request is one PR under review.
type Request struct {
	ID                  string     `json:"id"`
	Repo                string     `json:"repo"` // owner/name
	Number              int        `json:"number"`
	URL                 string     `json:"url"`
	Title               string     `json:"title"`
	Author              string     `json:"author"`
	BaseRef             string     `json:"base_ref"`
	HeadRef             string     `json:"head_ref"`
	HeadSHA             string     `json:"head_sha"`
	Origin              Origin     `json:"origin"`
	Recipe              string     `json:"recipe"`
	Harness             string     `json:"harness"`
	Model               string     `json:"model"`
	ReasoningEffort     string     `json:"reasoning_effort"`
	State               State      `json:"state"`
	Attempt             int        `json:"attempt"`
	Watch               bool       `json:"watch"`
	DryRun              bool       `json:"dry_run"`
	Verdict             string     `json:"verdict"`
	Summary             string     `json:"summary"`
	ReviewURL           string     `json:"review_url"`
	MyReviewState       string     `json:"my_review_state"`
	MyReviewID          int64      `json:"my_review_id"`
	LastReviewedHeadSHA string     `json:"last_reviewed_head_sha"`
	SessionID           string     `json:"session_id"`
	SessionExternalID   string     `json:"session_external_id"`
	WorktreePath        string     `json:"worktree_path"`
	TicketID            string     `json:"ticket_id"`
	Error               string     `json:"error"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	ReviewedAt          *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	Findings            []Finding  `json:"findings,omitempty"`
}

// Ref returns owner/name#N.
func (r *Request) Ref() string { return r.Repo + "#" + itoa(r.Number) }

// Finding is one review finding.
type Finding struct {
	ID              string    `json:"id"`
	RequestID       string    `json:"request_id"`
	Attempt         int       `json:"attempt"`
	Severity        string    `json:"severity"` // P0..P3
	Path            string    `json:"path"`
	Line            int       `json:"line"`
	Side            string    `json:"side"`
	Title           string    `json:"title"`
	Body            string    `json:"body"`
	GitHubCommentID int64     `json:"github_comment_id"`
	Status          string    `json:"status"` // pending | posted | in_body | withheld | resolved | outdated
	CreatedAt       time.Time `json:"created_at"`
}

// Blocking reports whether the finding should block approval.
func (f Finding) Blocking() bool { return f.Severity == "P0" || f.Severity == "P1" }

// FeedbackRound is a review someone else submitted on one of the operator's PRs.
type FeedbackRound struct {
	ID           string     `json:"id"`
	Repo         string     `json:"repo"`
	Number       int        `json:"number"`
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	HeadSHA      string     `json:"head_sha"`
	Reviewer     string     `json:"reviewer"`
	ReviewState  string     `json:"review_state"`
	ReviewID     int64      `json:"review_id"`
	CommentCount int        `json:"comment_count"`
	Body         string     `json:"body"`
	State        string     `json:"state"` // new | dispatched | addressed | ignored
	TicketID     string     `json:"ticket_id"`
	SessionID    string     `json:"session_id"`
	ObservedAt   time.Time  `json:"observed_at"`
	SubmittedAt  *time.Time `json:"submitted_at,omitempty"`
}

// Ref returns owner/name#N.
func (f *FeedbackRound) Ref() string { return f.Repo + "#" + itoa(f.Number) }

// Filter narrows request listings.
type Filter struct {
	State  State
	Repo   string
	Limit  int
	Offset int
}

// PollerStatus reports the background loops.
type PollerStatus struct {
	Enabled         bool       `json:"enabled"`
	Login           string     `json:"login,omitempty"`
	Harness         string     `json:"harness"`
	Publish         bool       `json:"publish"`
	WatchRequested  bool       `json:"watch_requested"`
	WatchAuthored   bool       `json:"watch_authored"`
	LastQueueRunAt  *time.Time `json:"last_queue_run_at,omitempty"`
	LastPollAt      *time.Time `json:"last_poll_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	Active          int        `json:"active"`
	Queued          int        `json:"queued"`
	Watching        int        `json:"watching"`
	NewFeedback     int        `json:"new_feedback"`
	ReviewsPosted   int        `json:"reviews_posted"`
	MaxConcurrent   int        `json:"max_concurrent"`
	RepoRoot        string     `json:"repo_root"`
	WorktreeRootFmt string     `json:"worktree_root_fmt"`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
