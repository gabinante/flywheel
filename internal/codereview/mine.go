package codereview

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gabinante/flywheel/internal/sessions"
)

// PRCard is a cross-project pull request row: GitHub facts plus what Flywheel knows
// about it (review queue entry, feedback rounds, sessions, Linear references).
type PRCard struct {
	PRDetail
	MyReviewState         string          `json:"my_review_state"`
	ReviewRequestedFromMe bool            `json:"review_requested_from_me"`
	LinearRefs            []string        `json:"linear_refs"`
	Sessions              int             `json:"sessions"`
	Review                *Request        `json:"review,omitempty"`
	Feedback              *FeedbackDigest `json:"feedback,omitempty"`
}

// FeedbackDigest summarizes feedback rounds observed on a PR.
type FeedbackDigest struct {
	New          int        `json:"new"`
	Dispatched   int        `json:"dispatched"`
	Addressed    int        `json:"addressed"`
	LastReviewer string     `json:"last_reviewer"`
	LastState    string     `json:"last_state"`
	LastAt       *time.Time `json:"last_at,omitempty"`
	LatestID     string     `json:"latest_id"`
}

// MyPullRequests is the operator's PR overview.
type MyPullRequests struct {
	Login     string    `json:"login"`
	FetchedAt time.Time `json:"fetched_at"`
	Open      []PRCard  `json:"open"`
	Merged    []PRCard  `json:"merged"` // merged within the last 7 days
}

// MyReviews is the operator's reviewing overview.
type MyReviews struct {
	Login     string    `json:"login"`
	FetchedAt time.Time `json:"fetched_at"`
	Requested []PRCard  `json:"requested"` // open PRs asking for the operator's review
	Reviewed  []PRCard  `json:"reviewed"`  // open PRs the operator has reviewed
}

const mineCacheTTL = 45 * time.Second

type mineCache struct {
	mu      sync.Mutex
	prs     *MyPullRequests
	reviews *MyReviews
	prsAt   time.Time
	revAt   time.Time
}

var linearRefRE = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9})-(\d{1,6})\b`)

// MyPRs returns the operator's open PRs (and recent merges) across every repo.
func (s *Service) MyPRs(ctx context.Context, force bool) (*MyPullRequests, error) {
	s.mine.mu.Lock()
	if !force && s.mine.prs != nil && time.Since(s.mine.prsAt) < mineCacheTTL {
		out := *s.mine.prs
		s.mine.mu.Unlock()
		return &out, nil
	}
	s.mine.mu.Unlock()

	login, _ := s.gh.Login(ctx)
	since := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var open, merged []PRDetail
	var openErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		open, openErr = s.gh.SearchPRs(ctx, "is:pr is:open author:@me archived:false sort:updated-desc", 100)
	}()
	go func() {
		defer wg.Done()
		merged, _ = s.gh.SearchPRs(ctx, fmt.Sprintf("is:pr is:merged author:@me archived:false merged:>=%s sort:updated-desc", since), 50) // best effort
	}()
	wg.Wait()
	if openErr != nil {
		return nil, openErr
	}
	res := &MyPullRequests{Login: login, FetchedAt: time.Now(), Open: s.enrich(ctx, login, open), Merged: s.enrich(ctx, login, merged)}
	s.mine.mu.Lock()
	s.mine.prs, s.mine.prsAt = res, time.Now()
	s.mine.mu.Unlock()
	return res, nil
}

// MyReviewsOverview returns open PRs that ask for the operator's review or that the
// operator has reviewed, with Flywheel's queue state attached.
func (s *Service) MyReviewsOverview(ctx context.Context, force bool) (*MyReviews, error) {
	s.mine.mu.Lock()
	if !force && s.mine.reviews != nil && time.Since(s.mine.revAt) < mineCacheTTL {
		out := *s.mine.reviews
		s.mine.mu.Unlock()
		return &out, nil
	}
	s.mine.mu.Unlock()

	login, _ := s.gh.Login(ctx)
	var requested, reviewed []PRDetail
	var reqErr, revErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		requested, reqErr = s.gh.SearchPRs(ctx, "is:pr is:open review-requested:@me archived:false sort:updated-desc", 100)
	}()
	go func() {
		defer wg.Done()
		reviewed, revErr = s.gh.SearchPRs(ctx, "is:pr is:open reviewed-by:@me -author:@me archived:false sort:updated-desc", 50)
	}()
	wg.Wait()
	if reqErr != nil {
		return nil, reqErr
	}
	if revErr != nil {
		return nil, revErr
	}
	inRequested := map[string]bool{}
	for _, p := range requested {
		inRequested[prKey(p.Repo, p.Number)] = true
	}
	var onlyReviewed []PRDetail
	for _, p := range reviewed {
		if !inRequested[prKey(p.Repo, p.Number)] {
			onlyReviewed = append(onlyReviewed, p)
		}
	}
	res := &MyReviews{Login: login, FetchedAt: time.Now(), Requested: s.enrich(ctx, login, requested), Reviewed: s.enrich(ctx, login, onlyReviewed)}
	s.mine.mu.Lock()
	s.mine.reviews, s.mine.revAt = res, time.Now()
	s.mine.mu.Unlock()
	return res, nil
}

// InvalidateMine drops the cached overviews (called after actions that change them).
func (s *Service) InvalidateMine() {
	s.mine.mu.Lock()
	s.mine.prs, s.mine.reviews = nil, nil
	s.mine.mu.Unlock()
}

func prKey(repo string, n int) string { return fmt.Sprintf("%s#%d", repo, n) }

func (s *Service) enrich(ctx context.Context, login string, prs []PRDetail) []PRCard {
	cards := make([]PRCard, 0, len(prs))
	if len(prs) == 0 {
		return cards
	}
	// Feedback rounds, bucketed by PR.
	feedback := map[string]*FeedbackDigest{}
	if rounds, err := s.store.ListFeedbackRounds(ctx, "", 1000); err == nil {
		for _, r := range rounds {
			k := prKey(r.Repo, r.Number)
			d := feedback[k]
			if d == nil {
				d = &FeedbackDigest{}
				feedback[k] = d
			}
			switch r.State {
			case "new":
				d.New++
			case "dispatched":
				d.Dispatched++
			case "addressed":
				d.Addressed++
			}
			at := r.ObservedAt
			if r.SubmittedAt != nil {
				at = *r.SubmittedAt
			}
			if d.LastAt == nil || at.After(*d.LastAt) {
				t := at
				d.LastAt, d.LastReviewer, d.LastState, d.LatestID = &t, r.Reviewer, r.ReviewState, r.ID
			}
		}
	}
	for _, p := range prs {
		c := PRCard{PRDetail: p, LinearRefs: linearRefs(p.HeadRef + " " + p.Title)}
		for _, r := range p.Reviews {
			if login != "" && strings.EqualFold(r.Login, login) {
				c.MyReviewState = r.State
			}
		}
		for _, r := range p.RequestedReviewers {
			if login != "" && strings.EqualFold(r, login) {
				c.ReviewRequestedFromMe = true
			}
		}
		if req, err := s.store.GetByRepoNumber(ctx, p.Repo, p.Number); err == nil && req != nil {
			c.Review = req
		}
		if d := feedback[prKey(p.Repo, p.Number)]; d != nil {
			c.Feedback = d
		}
		if s.sessions != nil {
			if _, total, err := s.sessions.List(ctx, sessions.Filter{Ref: prKey(p.Repo, p.Number), Limit: 1}); err == nil {
				c.Sessions = total
			}
		}
		cards = append(cards, c)
	}
	sort.SliceStable(cards, func(i, j int) bool { return cards[i].UpdatedAt.After(cards[j].UpdatedAt) })
	return cards
}

func linearRefs(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range linearRefRE.FindAllStringSubmatch(strings.ToUpper(text), -1) {
		ref := m[1] + "-" + m[2]
		if seen[ref] || len(m[1]) < 2 {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// PollInterval reports how often the watchers run.
func (s *Service) PollInterval() time.Duration { return s.conf().PollInterval }

// PollNow runs the review-requested and feedback watchers immediately (regardless of
// their enabled flags — this is the operator asking).
func (s *Service) PollNow(ctx context.Context) error {
	var errs []string
	if err := s.pollRequested(ctx); err != nil {
		errs = append(errs, "review-requested: "+err.Error())
	}
	if err := s.pollFeedback(ctx); err != nil {
		errs = append(errs, "feedback: "+err.Error())
	}
	s.InvalidateMine()
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// DrainNow processes queued reviews immediately.
func (s *Service) DrainNow(ctx context.Context) { s.drainQueue(ctx) }
