package codereview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/harness"
	"github.com/gabinante/flywheel/internal/sessions"
)

// Event types published on the bus.
const (
	EventReviewCompleted = "codereview.completed"
	EventFeedbackLanded  = "pr.feedback_landed"
)

// Config controls the review service.
type Config struct {
	Enabled          bool
	Harness          string // codex | claude
	Model            string
	Effort           string
	Publish          bool          // false = review everything as a dry run (nothing posted to GitHub)
	PollInterval     time.Duration // GitHub pollers (default 2m)
	QueueInterval    time.Duration // queue scan (default 5s)
	MaxConcurrent    int           // concurrent reviews (default 2)
	RepoRoot         string        // where clones and worktrees live (default ~/git)
	WatchRequested   bool          // pick up review-requested PRs
	WatchAuthored    bool          // watch the operator's PRs for landed reviews
	ReviewTimeout    time.Duration // per review (default 30m)
	SkipDrafts       bool
	FeedbackLookback time.Duration // reviews older than this at first sight are recorded as ignored (default 48h)
}

// Service runs the review queue and the GitHub watchers.
type Service struct {
	store    *Store
	gh       *GitHub
	ws       *Workspaces
	runner   harness.Runner
	sessions *sessions.Service
	bus      events.Bus
	cfg      Config

	fb FeedbackConfig

	mu        sync.Mutex
	status    PollerStatus
	active    int32
	startedAt time.Time
}

// SetFeedbackConfig configures the address-feedback workflow.
func (s *Service) SetFeedbackConfig(cfg FeedbackConfig) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 45 * time.Minute
	}
	if cfg.Harness == "" {
		cfg.Harness = "claude"
	}
	s.fb = cfg
}

// New builds a Service. sessions and bus may be nil.
func New(store *Store, gh *GitHub, runner harness.Runner, sess *sessions.Service, bus events.Bus, cfg Config) *Service {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Minute
	}
	if cfg.QueueInterval <= 0 {
		cfg.QueueInterval = 5 * time.Second
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 2
	}
	if cfg.ReviewTimeout <= 0 {
		cfg.ReviewTimeout = 30 * time.Minute
	}
	if cfg.FeedbackLookback <= 0 {
		cfg.FeedbackLookback = 48 * time.Hour
	}
	if cfg.Harness == "" {
		cfg.Harness = "codex"
	}
	if cfg.RepoRoot == "" {
		if h, err := os.UserHomeDir(); err == nil {
			cfg.RepoRoot = filepath.Join(h, "git")
		}
	}
	return &Service{store: store, gh: gh, ws: &Workspaces{Root: cfg.RepoRoot}, runner: runner, sessions: sess, bus: bus, cfg: cfg,
		status: PollerStatus{Enabled: cfg.Enabled, Harness: cfg.Harness, Publish: cfg.Publish, WatchRequested: cfg.WatchRequested,
			WatchAuthored: cfg.WatchAuthored, MaxConcurrent: cfg.MaxConcurrent, RepoRoot: cfg.RepoRoot, WorktreeRootFmt: filepath.Join(cfg.RepoRoot, "<repo>-worktrees", "review-<n>")}}
}

// Start launches the queue and poller loops.
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		slog.Info("codereview: disabled")
		return
	}
	s.startedAt = time.Now()
	go func() {
		if login, err := s.gh.Login(ctx); err == nil {
			s.mu.Lock()
			s.status.Login = login
			s.mu.Unlock()
		} else {
			slog.Warn("codereview: gh auth check failed", "error", err)
		}
	}()
	go s.queueLoop(ctx)
	go s.pollLoop(ctx)
	slog.Info("codereview: started", "harness", s.cfg.Harness, "publish", s.cfg.Publish, "watch_requested", s.cfg.WatchRequested, "watch_authored", s.cfg.WatchAuthored)
}

func (s *Service) queueLoop(ctx context.Context) {
	t := time.NewTicker(s.cfg.QueueInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.drainQueue(ctx)
		}
	}
}

func (s *Service) drainQueue(ctx context.Context) {
	free := s.cfg.MaxConcurrent - int(atomic.LoadInt32(&s.active))
	if free <= 0 {
		return
	}
	reqs, err := s.store.ClaimQueued(ctx, free)
	if err != nil {
		s.setError(err)
		return
	}
	now := time.Now()
	s.mu.Lock()
	s.status.LastQueueRunAt = &now
	s.mu.Unlock()
	for _, r := range reqs {
		atomic.AddInt32(&s.active, 1)
		go func(r *Request) {
			defer atomic.AddInt32(&s.active, -1)
			s.process(ctx, r)
		}(r)
	}
}

func (s *Service) pollLoop(ctx context.Context) {
	run := func() {
		if s.cfg.WatchRequested {
			if err := s.pollRequested(ctx); err != nil {
				s.setError(fmt.Errorf("review-requested: %w", err))
			}
		}
		if err := s.pollWatching(ctx); err != nil {
			s.setError(fmt.Errorf("watching: %w", err))
		}
		if s.cfg.WatchAuthored {
			if err := s.pollFeedback(ctx); err != nil {
				s.setError(fmt.Errorf("feedback: %w", err))
			}
		}
		now := time.Now()
		s.mu.Lock()
		s.status.LastPollAt = &now
		s.mu.Unlock()
	}
	run()
	t := time.NewTicker(s.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

func (s *Service) setError(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	slog.Warn("codereview: "+err.Error())
	s.mu.Lock()
	s.status.LastError = err.Error()
	s.mu.Unlock()
}

// EnqueueOptions tune a pasted/MCP request.
type EnqueueOptions struct {
	DryRun  *bool
	Watch   *bool
	Harness string
}

// Enqueue parses PR references from text and queues a review for each.
// Already-reviewed PRs are re-queued; PRs currently in flight are left alone.
func (s *Service) Enqueue(ctx context.Context, text string, origin Origin, opts EnqueueOptions) ([]*Request, error) {
	refs := ParsePRRefs(text)
	if len(refs) == 0 {
		return nil, errors.New("no pull request references found (expected GitHub PR URLs or owner/repo#N)")
	}
	var out []*Request
	for _, ref := range refs {
		existing, err := s.store.GetByRepoNumber(ctx, ref.Repo, ref.Number)
		if err != nil {
			return out, err
		}
		if existing != nil {
			switch existing.State {
			case StateQueued, StateFetching, StateReviewing, StatePublishing:
				out = append(out, existing)
				continue
			}
			if opts.DryRun != nil {
				existing.DryRun = *opts.DryRun
			}
			if opts.Watch != nil {
				existing.Watch = *opts.Watch
			}
			if opts.Harness != "" {
				existing.Harness = opts.Harness
			}
			_ = s.store.Update(ctx, existing)
			if err := s.store.Requeue(ctx, existing.ID, origin); err != nil {
				return out, err
			}
			fresh, _ := s.store.Get(ctx, existing.ID)
			if fresh != nil {
				existing = fresh
			}
			out = append(out, existing)
			continue
		}
		req := &Request{Repo: ref.Repo, Number: ref.Number, URL: PRURL(ref.Repo, ref.Number), Origin: origin, Harness: s.cfg.Harness,
			Model: s.cfg.Model, ReasoningEffort: s.cfg.Effort, Watch: true, DryRun: !s.cfg.Publish}
		if opts.DryRun != nil {
			req.DryRun = *opts.DryRun || !s.cfg.Publish
		}
		if opts.Watch != nil {
			req.Watch = *opts.Watch
		}
		if opts.Harness != "" {
			req.Harness = opts.Harness
		}
		if err := s.store.Create(ctx, req); err != nil {
			return out, err
		}
		out = append(out, req)
	}
	return out, nil
}

// process runs one review attempt end to end.
func (s *Service) process(ctx context.Context, req *Request) {
	fail := func(err error) {
		req.State = StateFailed
		req.Error = err.Error()
		_ = s.store.Update(ctx, req)
		slog.Warn("codereview: failed", "pr", req.Ref(), "error", err)
	}
	login, _ := s.gh.Login(ctx)
	pr, err := s.gh.ViewPR(ctx, req.Repo, req.Number, login)
	if err != nil {
		fail(err)
		return
	}
	req.Title, req.Author, req.BaseRef, req.HeadRef, req.HeadSHA, req.URL = pr.Title, pr.Author, pr.BaseRef, pr.HeadRef, pr.HeadSHA, pr.URL
	req.MyReviewState = pr.MyReviewState
	now := time.Now()
	req.LastCheckedAt = &now
	if pr.State != "OPEN" {
		req.State = StateClosed
		_ = s.store.Update(ctx, req)
		return
	}
	if pr.IsDraft && s.cfg.SkipDrafts {
		req.State = StateWatching
		req.Error = "draft PR; will review when it is marked ready"
		_ = s.store.Update(ctx, req)
		return
	}
	req.Error = ""
	_ = s.store.Update(ctx, req)

	wt, sha, err := s.ws.Prepare(ctx, req.Repo, req.Number)
	if err != nil {
		fail(fmt.Errorf("prepare worktree: %w", err))
		return
	}
	defer s.ws.Remove(ctx, req.Repo, wt)
	req.WorktreePath = wt
	if sha != "" {
		req.HeadSHA = sha
	}
	diff, err := s.gh.Diff(ctx, req.Repo, req.Number)
	if err != nil {
		fail(fmt.Errorf("diff: %w", err))
		return
	}
	diffIdx := ParseUnifiedDiff(diff)
	diffDir, err := os.MkdirTemp("", "flywheel-review-")
	if err != nil {
		fail(err)
		return
	}
	defer os.RemoveAll(diffDir)
	diffPath := filepath.Join(diffDir, fmt.Sprintf("pr-%d.diff", req.Number))
	if err := os.WriteFile(diffPath, []byte(diff), 0o600); err != nil {
		fail(err)
		return
	}

	var prior []Finding
	if req.Attempt > 1 {
		prior, _ = s.store.ListFindings(ctx, req.ID, req.Attempt-1)
	}
	req.State = StateReviewing
	_ = s.store.Update(ctx, req)

	kind, err := harness.ParseKind(req.Harness)
	if err != nil {
		fail(err)
		return
	}
	prompt := BuildReviewPrompt(pr, diffPath, diffIdx.Files(), prior)
	started := time.Now()
	res, runErr := s.runner.Run(ctx, harness.Spec{
		Harness: kind, Model: req.Model, Effort: req.ReasoningEffort, WorkDir: wt,
		SystemPrompt: ReviewSystemPrompt, Prompt: prompt, OutputSchema: FindingsSchema,
		Sandbox: harness.SandboxReadOnly, Timeout: s.cfg.ReviewTimeout,
	})
	s.recordSession(ctx, req, pr, kind, res, prompt, started)
	if runErr != nil {
		if res != nil && res.Stderr != "" {
			runErr = fmt.Errorf("%w; stderr: %s", runErr, truncateStr(res.Stderr, 300))
		}
		fail(runErr)
		return
	}
	raw := res.Structured
	if len(raw) == 0 {
		raw = []byte(res.Output)
	}
	summary, findings, err := ParseReviewOutput(raw)
	if err != nil {
		fail(fmt.Errorf("%w: %s", err, truncateStr(res.Output, 300)))
		return
	}
	for i := range findings {
		findings[i].RequestID = req.ID
		findings[i].Attempt = req.Attempt
	}
	req.Summary = summary
	req.Verdict = Verdict(findings)
	req.State = StatePublishing
	_ = s.store.Update(ctx, req)
	if err := s.store.ReplaceFindings(ctx, req.ID, req.Attempt, findings); err != nil {
		fail(err)
		return
	}

	body, inline, inBody := ComposeReview(summary, findings, diffIdx)
	reviewedAt := time.Now()
	req.ReviewedAt = &reviewedAt
	req.LastReviewedHeadSHA = req.HeadSHA
	if req.DryRun || !s.cfg.Publish {
		for _, f := range findings {
			_ = s.store.UpdateFindingStatus(ctx, f.ID, "withheld", 0)
		}
		req.State = StateCommented
		if req.Watch {
			req.State = StateWatching
		}
		req.Error = ""
		_ = s.store.Update(ctx, req)
		s.publishCompleted(ctx, req, "")
		slog.Info("codereview: dry run complete", "pr", req.Ref(), "verdict", req.Verdict, "findings", len(findings))
		return
	}

	event := "APPROVE"
	if req.Verdict == VerdictRequestChanges {
		event = "REQUEST_CHANGES"
	}
	if strings.EqualFold(pr.Author, login) {
		event = "COMMENT" // GitHub forbids approving your own PR
		req.Verdict = VerdictComment
	}
	in := ReviewInput{CommitID: req.HeadSHA, Event: event, Body: body}
	for _, i := range inline {
		f := findings[i]
		in.Comments = append(in.Comments, ReviewComment{Path: f.Path, Line: f.Line, Side: "RIGHT", Body: InlineCommentBody(f)})
	}
	posted, err := s.gh.CreateReview(ctx, req.Repo, req.Number, in)
	if err != nil && len(in.Comments) > 0 {
		// Some line was not commentable after all; fall back to everything in the body.
		body2, _, _ := ComposeReview(summary, findings, &DiffIndex{files: map[string]map[int]bool{}})
		in.Comments = nil
		in.Body = body2
		inBody = allIndexes(findings)
		inline = nil
		posted, err = s.gh.CreateReview(ctx, req.Repo, req.Number, in)
	}
	if err != nil {
		fail(fmt.Errorf("post review: %w", err))
		return
	}
	ids, _ := s.gh.ListReviewCommentIDs(ctx, req.Repo, req.Number, posted.ID)
	for k, i := range inline {
		var cid int64
		if k < len(ids) {
			cid = ids[k]
		}
		_ = s.store.UpdateFindingStatus(ctx, findings[i].ID, "posted", cid)
	}
	for _, i := range inBody {
		_ = s.store.UpdateFindingStatus(ctx, findings[i].ID, "in_body", 0)
	}
	req.ReviewURL = posted.HTMLURL
	req.MyReviewID = posted.ID
	req.MyReviewState = posted.State
	switch req.Verdict {
	case VerdictRequestChanges:
		req.State = StateChangesRequested
	case VerdictComment:
		req.State = StateCommented
	default:
		req.State = StateApproved
	}
	if req.Watch {
		req.State = StateWatching
	}
	req.Error = ""
	_ = s.store.Update(ctx, req)
	s.publishCompleted(ctx, req, posted.HTMLURL)
	slog.Info("codereview: posted review", "pr", req.Ref(), "verdict", req.Verdict, "inline", len(inline), "in_body", len(inBody), "url", posted.HTMLURL)
}

func allIndexes(fs []Finding) []int {
	out := make([]int, len(fs))
	for i := range fs {
		out[i] = i
	}
	return out
}

func (s *Service) publishCompleted(ctx context.Context, req *Request, url string) {
	if s.bus == nil {
		return
	}
	_ = s.bus.Publish(ctx, events.NewEvent(EventReviewCompleted, map[string]any{
		"request_id": req.ID, "repo": req.Repo, "number": req.Number, "verdict": req.Verdict, "review_url": url, "dry_run": req.DryRun,
	}).WithEntityKey("codereview:"+req.ID))
}

// recordSession stores the reviewing run as a dispatched session linked to the PR and the request.
func (s *Service) recordSession(ctx context.Context, req *Request, pr *PR, kind harness.Kind, res *harness.Result, prompt string, started time.Time) {
	if s.sessions == nil || res == nil || res.ExternalSessionID == "" {
		return
	}
	h := sessions.HarnessCodex
	if kind == harness.ClaudeCode {
		h = sessions.HarnessClaudeCode
	}
	ended := time.Now()
	sess := &sessions.Session{
		Harness: h, ExternalID: res.ExternalSessionID, Origin: sessions.OriginDispatched, CWD: req.WorktreePath, Repo: req.Repo,
		Branch: pr.HeadRef, Model: firstNonEmpty(res.Model, req.Model), ReasoningEffort: req.ReasoningEffort,
		Title: fmt.Sprintf("Review %s: %s", req.Ref(), pr.Title), FirstPrompt: truncateStr(prompt, 500),
		TokensIn: res.TokensIn, TokensOut: res.TokensOut, PromptCount: 1, StartedAt: started, LastActivityAt: ended, EndedAt: &ended,
		Metadata: map[string]any{"flywheel_role": "reviewer", "review_request_id": req.ID, "attempt": req.Attempt, "cost_usd": res.CostUSD},
	}
	if err := s.sessions.Record(ctx, sess); err != nil {
		slog.Warn("codereview: record session failed", "error", err)
		return
	}
	req.SessionID = sess.ID
	req.SessionExternalID = res.ExternalSessionID
	_ = s.sessions.Link(ctx, sess.ID, sessions.LinkPR, req.Ref(), sessions.LinkSourceDispatch)
	_ = s.sessions.Link(ctx, sess.ID, sessions.LinkReview, req.ID, sessions.LinkSourceDispatch)
}

// pollRequested queues PRs where the operator's review is requested.
func (s *Service) pollRequested(ctx context.Context) error {
	prs, err := s.gh.SearchReviewRequested(ctx)
	if err != nil {
		return err
	}
	for _, p := range prs {
		if p.IsDraft && s.cfg.SkipDrafts {
			continue
		}
		existing, err := s.store.GetByRepoNumber(ctx, p.Repo, p.Number)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		req := &Request{Repo: p.Repo, Number: p.Number, URL: p.URL, Title: p.Title, Author: p.Author, Origin: OriginReviewRequested,
			Harness: s.cfg.Harness, Model: s.cfg.Model, ReasoningEffort: s.cfg.Effort, Watch: true, DryRun: !s.cfg.Publish}
		if err := s.store.Create(ctx, req); err != nil {
			return err
		}
		slog.Info("codereview: queued review-requested PR", "pr", req.Ref(), "title", p.Title)
	}
	return nil
}

// pollWatching re-queues reviewed PRs whose head moved or whose review was dismissed, and closes merged/closed ones.
func (s *Service) pollWatching(ctx context.Context) error {
	reqs, err := s.store.ListByState(ctx, StateWatching)
	if err != nil {
		return err
	}
	login, _ := s.gh.Login(ctx)
	for _, r := range reqs {
		pr, err := s.gh.ViewPR(ctx, r.Repo, r.Number, login)
		if err != nil {
			slog.Warn("codereview: watch check failed", "pr", r.Ref(), "error", err)
			continue
		}
		now := time.Now()
		r.LastCheckedAt = &now
		if pr.State != "OPEN" {
			r.State = StateClosed
			_ = s.store.Update(ctx, r)
			continue
		}
		requeue := ""
		switch {
		case r.LastReviewedHeadSHA == "" && !pr.IsDraft:
			requeue = "ready for review"
		case r.LastReviewedHeadSHA != "" && pr.HeadSHA != r.LastReviewedHeadSHA:
			requeue = "new commits"
		case r.MyReviewState != "" && pr.MyReviewState == "DISMISSED":
			requeue = "review dismissed"
		}
		r.MyReviewState = pr.MyReviewState
		r.HeadSHA = pr.HeadSHA
		_ = s.store.Update(ctx, r)
		if requeue != "" && !(pr.IsDraft && s.cfg.SkipDrafts) {
			slog.Info("codereview: re-queuing", "pr", r.Ref(), "reason", requeue)
			_ = s.store.Requeue(ctx, r.ID, OriginReReview)
		}
	}
	return nil
}

// pollFeedback records reviews that landed on the operator's own open PRs.
func (s *Service) pollFeedback(ctx context.Context) error {
	login, err := s.gh.Login(ctx)
	if err != nil {
		return err
	}
	prs, err := s.gh.SearchAuthoredOpen(ctx)
	if err != nil {
		return err
	}
	cutoff := s.startedAt.Add(-s.cfg.FeedbackLookback)
	for _, p := range prs {
		maxSeen, err := s.store.MaxFeedbackReviewID(ctx, p.Repo, p.Number)
		if err != nil {
			return err
		}
		reviews, err := s.gh.ListReviews(ctx, p.Repo, p.Number)
		if err != nil {
			slog.Warn("codereview: list reviews failed", "pr", p.Repo, "error", err)
			continue
		}
		for _, rv := range reviews {
			if rv.ID <= maxSeen || strings.EqualFold(rv.User, login) || rv.State == "PENDING" {
				continue
			}
			round := &FeedbackRound{Repo: p.Repo, Number: p.Number, URL: p.URL, Title: p.Title, HeadSHA: rv.CommitID, Reviewer: rv.User,
				ReviewState: rv.State, ReviewID: rv.ID, Body: truncateStr(rv.Body, 4000), State: "new"}
			if !rv.SubmittedAt.IsZero() {
				t := rv.SubmittedAt
				round.SubmittedAt = &t
				if maxSeen == 0 && t.Before(cutoff) {
					round.State = "ignored" // historical review from before Flywheel started watching
				}
			}
			if round.State == "new" {
				round.CommentCount = s.gh.CountReviewComments(ctx, p.Repo, p.Number, rv.ID)
			}
			created, err := s.store.InsertFeedbackRound(ctx, round)
			if err != nil {
				return err
			}
			if created && round.State == "new" {
				slog.Info("codereview: review landed on your PR", "pr", round.Ref(), "reviewer", rv.User, "state", rv.State, "comments", round.CommentCount)
				if s.fb.AutoAddress && (rv.State == "CHANGES_REQUESTED" || round.CommentCount > 0) {
					if _, err := s.AddressFeedback(ctx, round.ID); err != nil {
						slog.Warn("codereview: auto-address failed", "pr", round.Ref(), "error", err)
					}
				}
				if s.bus != nil {
					_ = s.bus.Publish(ctx, events.NewEvent(EventFeedbackLanded, map[string]any{
						"round_id": round.ID, "repo": round.Repo, "number": round.Number, "url": round.URL, "title": round.Title,
						"reviewer": rv.User, "review_state": rv.State, "comment_count": round.CommentCount, "head_sha": rv.CommitID,
					}).WithEntityKey("pr:"+round.Ref()))
				}
			}
		}
	}
	return nil
}

// ---- queries ----------------------------------------------------------------

// List returns requests.
func (s *Service) List(ctx context.Context, f Filter) ([]*Request, int, error) { return s.store.List(ctx, f) }

// Get returns one request with findings, or nil.
func (s *Service) Get(ctx context.Context, id string) (*Request, error) { return s.store.Get(ctx, id) }

// Rerun queues another attempt.
func (s *Service) Rerun(ctx context.Context, id string) (*Request, error) {
	r, err := s.store.Get(ctx, id)
	if err != nil || r == nil {
		return r, err
	}
	switch r.State {
	case StateQueued, StateFetching, StateReviewing, StatePublishing:
		return r, nil
	}
	if err := s.store.Requeue(ctx, id, OriginPaste); err != nil {
		return nil, err
	}
	return s.store.Get(ctx, id)
}

// Close stops watching a PR.
func (s *Service) Close(ctx context.Context, id string) (*Request, error) {
	r, err := s.store.Get(ctx, id)
	if err != nil || r == nil {
		return r, err
	}
	r.Watch = false
	if r.State == StateWatching || r.State == StateQueued {
		r.State = StateClosed
	}
	if err := s.store.Update(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// FeedbackRounds lists landed reviews on the operator's PRs.
func (s *Service) FeedbackRounds(ctx context.Context, state string, limit int) ([]*FeedbackRound, error) {
	return s.store.ListFeedbackRounds(ctx, state, limit)
}

// FeedbackRound returns one round.
func (s *Service) FeedbackRound(ctx context.Context, id string) (*FeedbackRound, error) {
	return s.store.GetFeedbackRound(ctx, id)
}

// SetFeedbackState updates a round's state (ignored, addressed, dispatched).
func (s *Service) SetFeedbackState(ctx context.Context, id, state, ticketID, sessionID string) (*FeedbackRound, error) {
	if err := s.store.UpdateFeedbackRound(ctx, id, state, ticketID, sessionID); err != nil {
		return nil, err
	}
	return s.store.GetFeedbackRound(ctx, id)
}

// Status reports the loops and queue depth.
func (s *Service) Status(ctx context.Context) PollerStatus {
	s.mu.Lock()
	st := s.status
	s.mu.Unlock()
	st.Active = int(atomic.LoadInt32(&s.active))
	if counts, err := s.store.CountsByState(ctx); err == nil {
		st.Queued = counts[StateQueued]
		st.Watching = counts[StateWatching]
	}
	if fb, err := s.store.CountFeedbackByState(ctx); err == nil {
		st.NewFeedback = fb["new"]
	}
	if n, err := s.store.CountPostedReviews(ctx); err == nil {
		st.ReviewsPosted = n
	}
	return st
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
