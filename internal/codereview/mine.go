package codereview

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	RequestKind           string          `json:"request_kind"` // direct | team | ""
	LinearRefs            []string        `json:"linear_refs"`
	Sessions              int             `json:"sessions"`
	Review                *Request        `json:"review,omitempty"`
	Feedback              *FeedbackDigest `json:"feedback,omitempty"`
	TicketID              string          `json:"ticket_id,omitempty"` // set by project views
	TicketTitle           string          `json:"ticket_title,omitempty"`
	TicketIdentifier      string          `json:"ticket_identifier,omitempty"`
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

// Overviews are served from the last snapshot (memory, then Postgres) and refreshed in
// the background: a stale answer now beats a 10s wait on GitHub search.
const (
	mineCacheTTL     = 3 * time.Minute // how old a snapshot may be before a refresh is kicked off
	mineRefreshEvery = 3 * time.Minute // background refresh cadence for My PRs / My Reviews
)

type mineCache struct {
	mu         sync.Mutex
	prs        *MyPullRequests
	reviews    *MyReviews
	prsAt      time.Time
	revAt      time.Time
	projects   map[string]projectPRsEntry // key: sorted repo list
	refreshing map[string]bool            // in-flight background refreshes by key
	loaded     bool                       // snapshots restored from Postgres
}

type projectPRsEntry struct {
	at    time.Time
	login string
	cards []PRCard
}

// ProjectPRs returns PRs in the given repos: every open PR plus anything updated in
// the last 14 days (so freshly merged work still shows next to its ticket). Callers
// decide which of them matter (the operator's own, or ones linked to a ticket).
func (s *Service) ProjectPRs(ctx context.Context, repos []string, force bool) (string, []PRCard, error) {
	if len(repos) == 0 {
		return "", []PRCard{}, nil
	}
	sorted := append([]string{}, repos...)
	sort.Strings(sorted)
	key := strings.Join(sorted, ",")
	if !force {
		s.mine.mu.Lock()
		e, ok := s.mine.projects[key]
		s.mine.mu.Unlock()
		if !ok {
			if raw, at, err := s.store.LoadOverview(ctx, "project:"+key); err == nil && raw != nil {
				var v projectPRsSnapshot
				if json.Unmarshal(raw, &v) == nil {
					e, ok = projectPRsEntry{at: at, login: v.Login, cards: v.Cards}, true
					s.mine.mu.Lock()
					if s.mine.projects == nil {
						s.mine.projects = map[string]projectPRsEntry{}
					}
					s.mine.projects[key] = e
					s.mine.mu.Unlock()
				}
			}
		}
		if ok {
			if time.Since(e.at) >= mineCacheTTL && s.beginRefresh("project:"+key) {
				go func() {
					defer s.endRefresh("project:" + key)
					if _, _, err := s.ProjectPRs(context.WithoutCancel(ctx), repos, true); err != nil {
						slog.Warn("codereview: refresh project prs failed", "error", err)
					}
				}()
			}
			return e.login, e.cards, nil
		}
	}

	login, _ := s.gh.Login(ctx)
	var scope strings.Builder
	for _, r := range sorted {
		scope.WriteString(" repo:" + r)
	}
	since := time.Now().AddDate(0, 0, -14).Format("2006-01-02")
	var open, recent []PRDetail
	var openErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		open, openErr = s.gh.SearchPRs(ctx, "is:pr is:open archived:false sort:updated-desc"+scope.String(), 100)
	}()
	go func() {
		defer wg.Done()
		recent, _ = s.gh.SearchPRs(ctx, fmt.Sprintf("is:pr archived:false updated:>=%s sort:updated-desc%s", since, scope.String()), 100) // best effort
	}()
	wg.Wait()
	if openErr != nil {
		return login, nil, openErr
	}
	seen := map[string]bool{}
	var all []PRDetail
	for _, p := range append(open, recent...) {
		if k := prKey(p.Repo, p.Number); !seen[k] {
			seen[k] = true
			all = append(all, p)
		}
	}
	cards := s.enrich(ctx, login, all)
	s.mine.mu.Lock()
	if s.mine.projects == nil {
		s.mine.projects = map[string]projectPRsEntry{}
	}
	now := time.Now()
	s.mine.projects[key] = projectPRsEntry{at: now, login: login, cards: cards}
	s.mine.mu.Unlock()
	s.persistOverview(ctx, "project:"+key, projectPRsSnapshot{Login: login, Cards: cards}, now)
	return login, cards, nil
}

type projectPRsSnapshot struct {
	Login string   `json:"login"`
	Cards []PRCard `json:"cards"`
}

// LinearRefsIn extracts Linear-style identifiers (KEY-123) from text.
func LinearRefsIn(text string) []string { return linearRefs(text) }

var linearRefRE = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9})-(\d{1,6})\b`)

// restoreOverviews loads the last snapshots from Postgres once per process.
func (s *Service) restoreOverviews(ctx context.Context) {
	s.mine.mu.Lock()
	if s.mine.loaded {
		s.mine.mu.Unlock()
		return
	}
	s.mine.loaded = true
	s.mine.mu.Unlock()
	if raw, at, err := s.store.LoadOverview(ctx, "my_prs"); err == nil && raw != nil {
		var v MyPullRequests
		if json.Unmarshal(raw, &v) == nil {
			s.mine.mu.Lock()
			if s.mine.prs == nil {
				s.mine.prs, s.mine.prsAt = &v, at
			}
			s.mine.mu.Unlock()
		}
	}
	if raw, at, err := s.store.LoadOverview(ctx, "my_reviews"); err == nil && raw != nil {
		var v MyReviews
		if json.Unmarshal(raw, &v) == nil {
			s.mine.mu.Lock()
			if s.mine.reviews == nil {
				s.mine.reviews, s.mine.revAt = &v, at
			}
			s.mine.mu.Unlock()
		}
	}
}

func (s *Service) persistOverview(ctx context.Context, key string, v any, at time.Time) {
	if raw, err := json.Marshal(v); err == nil {
		if err := s.store.SaveOverview(ctx, key, raw, at); err != nil {
			slog.Warn("codereview: persist overview failed", "key", key, "error", err)
		}
	}
}

// beginRefresh marks a background refresh for key; false when one is already running.
func (s *Service) beginRefresh(key string) bool {
	s.mine.mu.Lock()
	defer s.mine.mu.Unlock()
	if s.mine.refreshing == nil {
		s.mine.refreshing = map[string]bool{}
	}
	if s.mine.refreshing[key] {
		return false
	}
	s.mine.refreshing[key] = true
	return true
}

func (s *Service) endRefresh(key string) {
	s.mine.mu.Lock()
	delete(s.mine.refreshing, key)
	s.mine.mu.Unlock()
}

// StartOverviewRefresh keeps My PRs and My Reviews warm in the background.
func (s *Service) StartOverviewRefresh(ctx context.Context) {
	go func() {
		s.restoreOverviews(ctx)
		refresh := func() {
			if _, err := s.fetchMyPRs(ctx); err != nil {
				slog.Warn("codereview: refresh my prs failed", "error", err)
			}
			if _, err := s.fetchMyReviews(ctx); err != nil {
				slog.Warn("codereview: refresh my reviews failed", "error", err)
			}
		}
		// Warm up shortly after boot, then on a cadence.
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
		refresh()
		t := time.NewTicker(mineRefreshEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				refresh()
			}
		}
	}()
}

// MyPRs returns the operator's open PRs (and recent merges) across every repo. It
// answers from the last snapshot and refreshes in the background when that is stale;
// force waits for a fresh fetch.
func (s *Service) MyPRs(ctx context.Context, force bool) (*MyPullRequests, error) {
	s.restoreOverviews(ctx)
	if !force {
		s.mine.mu.Lock()
		cached, at := s.mine.prs, s.mine.prsAt
		s.mine.mu.Unlock()
		if cached != nil {
			if time.Since(at) >= mineCacheTTL && s.beginRefresh("my_prs") {
				go func() {
					defer s.endRefresh("my_prs")
					if _, err := s.fetchMyPRs(context.WithoutCancel(ctx)); err != nil {
						slog.Warn("codereview: refresh my prs failed", "error", err)
					}
				}()
			}
			out := *cached
			return &out, nil
		}
	}
	return s.fetchMyPRs(ctx)
}

// fetchMyPRs asks GitHub now and stores the snapshot.
func (s *Service) fetchMyPRs(ctx context.Context) (*MyPullRequests, error) {
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
	s.mine.prs, s.mine.prsAt = res, res.FetchedAt
	s.mine.mu.Unlock()
	s.persistOverview(ctx, "my_prs", res, res.FetchedAt)
	out := *res
	return &out, nil
}

// MyReviewsOverview returns open PRs that ask for the operator's review or that the
// operator has reviewed, with Flywheel's queue state attached. Same snapshot semantics
// as MyPRs.
func (s *Service) MyReviewsOverview(ctx context.Context, force bool) (*MyReviews, error) {
	s.restoreOverviews(ctx)
	if !force {
		s.mine.mu.Lock()
		cached, at := s.mine.reviews, s.mine.revAt
		s.mine.mu.Unlock()
		if cached != nil {
			if time.Since(at) >= mineCacheTTL && s.beginRefresh("my_reviews") {
				go func() {
					defer s.endRefresh("my_reviews")
					if _, err := s.fetchMyReviews(context.WithoutCancel(ctx)); err != nil {
						slog.Warn("codereview: refresh my reviews failed", "error", err)
					}
				}()
			}
			out := *cached
			return &out, nil
		}
	}
	return s.fetchMyReviews(ctx)
}

func (s *Service) fetchMyReviews(ctx context.Context) (*MyReviews, error) {
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
	s.mine.reviews, s.mine.revAt = res, res.FetchedAt
	s.mine.mu.Unlock()
	s.persistOverview(ctx, "my_reviews", res, res.FetchedAt)
	out := *res
	return &out, nil
}

// InvalidateMine drops the cached overviews (called after actions that change them).
func (s *Service) InvalidateMine() {
	s.mine.mu.Lock()
	s.mine.prs, s.mine.reviews, s.mine.projects = nil, nil, nil
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
		switch {
		case c.ReviewRequestedFromMe:
			c.RequestKind = "direct"
		case len(p.RequestedTeams) > 0:
			c.RequestKind = "team"
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
