package report

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/codereview"
	"github.com/gabinante/flywheel/internal/linear"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/gabinante/flywheel/internal/ticket"
)

// Deps are the data sources reports are rendered from. Any may be nil except Store, Tickets, Projects.
type Deps struct {
	Store    *Store
	Tickets  *ticket.Service
	Projects *project.Service
	Repos    *project.RepositoryService
	Linear   *linear.Syncer
	GitHub   *codereview.GitHub
	Reviews  *codereview.Service
	Sessions *sessions.Service
}

// Config controls scheduling and destinations.
type Config struct {
	ProjectUpdatesEnabled bool
	ProjectUpdateInterval time.Duration
	WeeklyEnabled         bool
	WeeklyDay             time.Weekday
	WeeklyHour            int
	RoundupDocumentID     string
	RoundupProjectID      string
	DefaultHealth         string
}

// Service composes and posts reports.
type Service struct {
	d   Deps
	cfg Config
}

// New builds a Service.
func New(d Deps, cfg Config) *Service {
	if cfg.ProjectUpdateInterval <= 0 {
		cfg.ProjectUpdateInterval = 48 * time.Hour
	}
	if cfg.DefaultHealth == "" {
		cfg.DefaultHealth = HealthOnTrack
	}
	return &Service{d: d, cfg: cfg}
}

// ParseWeekday accepts names like "friday" or "Fri".
func ParseWeekday(s string) time.Weekday {
	s = strings.ToLower(strings.TrimSpace(s))
	for d := time.Sunday; d <= time.Saturday; d++ {
		name := strings.ToLower(d.String())
		if s == name || (len(s) >= 3 && strings.HasPrefix(name, s[:3])) {
			return d
		}
	}
	return time.Friday
}

// Start runs the scheduler when automatic posting is enabled.
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.ProjectUpdatesEnabled && !s.cfg.WeeklyEnabled {
		slog.Info("report: automatic posting disabled (previews available)")
		return
	}
	go func() {
		t := time.NewTicker(15 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.tick(ctx, time.Now())
			}
		}
	}()
	slog.Info("report: scheduler started", "project_updates", s.cfg.ProjectUpdatesEnabled, "weekly", s.cfg.WeeklyEnabled)
}

func (s *Service) tick(ctx context.Context, now time.Time) {
	if s.cfg.ProjectUpdatesEnabled && s.d.Linear != nil && s.d.Linear.Enabled() && now.Weekday() != time.Saturday && now.Weekday() != time.Sunday {
		links, err := s.d.Linear.Links(ctx)
		if err == nil {
			for _, l := range links {
				last, err := s.d.Store.LastPosted(ctx, l.ProjectID, KindProjectUpdate)
				if err != nil {
					continue
				}
				if last == nil || now.Sub(*last) >= s.cfg.ProjectUpdateInterval {
					if _, err := s.PostProjectUpdate(ctx, l.ProjectID, "", ""); err != nil {
						slog.Warn("report: automatic project update failed", "project", l.ProjectID, "error", err)
					}
				}
			}
		}
	}
	if s.cfg.WeeklyEnabled && now.Weekday() == s.cfg.WeeklyDay && now.Hour() >= s.cfg.WeeklyHour {
		start, _ := WeekWindow(now)
		if done, err := s.d.Store.WeeklyPosted(ctx, start); err == nil && !done {
			if _, err := s.PostWeeklyRoundup(ctx, now, ""); err != nil {
				slog.Warn("report: automatic weekly roundup failed", "error", err)
			}
		}
	}
}

// projectRepos returns owner/name repos for a project (from project_repositories and repo_url).
func (s *Service) projectRepos(ctx context.Context, proj *project.Project) []string {
	seen := map[string]bool{}
	var out []string
	add := func(u string) {
		if r := sessions.RepoFromOriginURL(u); r != "" && strings.Contains(r, "/") && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	add(proj.RepoURL)
	if s.d.Repos != nil {
		if repos, err := s.d.Repos.ListRepositories(ctx, proj.ID); err == nil {
			for _, r := range repos {
				add(r.RepoURL)
			}
		}
	}
	return out
}

func ticketLine(t *ticket.Ticket) TicketLine {
	l := TicketLine{Identifier: t.ID, Title: t.Title, State: string(t.State)}
	if t.External != nil {
		l.Identifier = t.External.Identifier
		l.URL = t.External.URL
		if t.External.StateName != "" {
			l.State = t.External.StateName
		}
	}
	return l
}

// gather builds ProjectData for a project over [since, until).
func (s *Service) gather(ctx context.Context, proj *project.Project, since, until time.Time) (ProjectData, error) {
	d := ProjectData{ProjectName: proj.Name, Since: since, Until: until}
	if s.d.Linear != nil {
		if link, err := s.d.Linear.Link(ctx, proj.ID); err == nil && link != nil {
			d.LinearURL = link.LinearProjectURL
		}
	}
	tickets, err := s.d.Tickets.ListTickets(ctx, proj.ID, "", "")
	if err != nil {
		return d, err
	}
	for _, t := range tickets {
		changed := !t.UpdatedAt.Before(since) && t.UpdatedAt.Before(until)
		line := ticketLine(t)
		switch t.State {
		case ticket.StateClosed:
			if changed {
				d.Done = append(d.Done, line)
			}
		case ticket.StateAwaitingValidation, ticket.StateValidated:
			d.InReview = append(d.InReview, line)
		case ticket.StateExecuting, ticket.StatePlanning:
			d.InProgress = append(d.InProgress, line)
		case ticket.StateAwaitingInput:
			d.Blocked = append(d.Blocked, line)
		default:
			if !t.CreatedAt.Before(since) && t.CreatedAt.Before(until) {
				d.New = append(d.New, line)
			}
		}
	}
	repos := s.projectRepos(ctx, proj)
	repoSet := map[string]bool{}
	for _, r := range repos {
		repoSet[strings.ToLower(r)] = true
	}
	if s.d.GitHub != nil {
		for _, r := range repos {
			if merged, err := s.d.GitHub.SearchMergedSince(ctx, since, r); err == nil {
				for _, m := range merged {
					if m.MergedAt.IsZero() || m.MergedAt.Before(until) {
						d.MergedPRs = append(d.MergedPRs, PRLine{Repo: m.Repo, Number: m.Number, Title: m.Title, URL: m.URL, When: m.MergedAt})
					}
				}
			}
			if open, err := s.d.GitHub.SearchOpenAuthored(ctx, r); err == nil {
				for _, o := range open {
					d.OpenPRs = append(d.OpenPRs, PRLine{Repo: o.Repo, Number: o.Number, Title: o.Title, URL: o.URL, When: o.UpdatedAt})
				}
			}
		}
	}
	if s.d.Reviews != nil {
		if rounds, err := s.d.Reviews.FeedbackRounds(ctx, "new", 200); err == nil {
			for _, r := range rounds {
				if repoSet[strings.ToLower(r.Repo)] {
					d.Feedback = append(d.Feedback, PRLine{Repo: r.Repo, Number: r.Number, Title: fmt.Sprintf("%s — %s by %s", r.Title, strings.ToLower(strings.ReplaceAll(r.ReviewState, "_", " ")), r.Reviewer), URL: r.URL, When: r.ObservedAt})
				}
			}
		}
		if reqs, _, err := s.d.Reviews.List(ctx, codereview.Filter{Limit: 200}); err == nil {
			for _, r := range reqs {
				if r.ReviewedAt != nil && !r.ReviewedAt.Before(since) && r.ReviewedAt.Before(until) && repoSet[strings.ToLower(r.Repo)] && r.ReviewURL != "" {
					d.Reviews = append(d.Reviews, PRLine{Repo: r.Repo, Number: r.Number, Title: r.Title, URL: r.ReviewURL, When: *r.ReviewedAt})
				}
			}
		}
	}
	if s.d.Sessions != nil && len(repos) > 0 {
		if stats, err := s.d.Sessions.StatsSince(ctx, since, until, repos); err == nil {
			for _, st := range stats {
				d.Activity = append(d.Activity, HarnessLine{Harness: st.Harness, Sessions: st.Sessions, TokensIn: st.TokensIn, TokensOut: st.TokensOut})
			}
		}
	}
	sortPRs(d.MergedPRs)
	sortPRs(d.OpenPRs)
	return d, nil
}

// ComposeProjectUpdate renders the delta update since the last posted one.
func (s *Service) ComposeProjectUpdate(ctx context.Context, projectID string) (*Report, error) {
	proj, err := s.d.Projects.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	until := time.Now()
	last, err := s.d.Store.LastPosted(ctx, projectID, KindProjectUpdate)
	if err != nil {
		return nil, err
	}
	since := until.Add(-s.cfg.ProjectUpdateInterval)
	if last != nil {
		since = *last
	}
	data, err := s.gather(ctx, proj, since, until)
	if err != nil {
		return nil, err
	}
	return &Report{Kind: KindProjectUpdate, ProjectID: projectID, WindowStart: since, WindowEnd: until,
		Body: RenderProjectUpdate(data), Health: SuggestHealth(data, s.cfg.DefaultHealth)}, nil
}

// PostProjectUpdate composes (or takes an edited body) and posts to the linked Linear project.
func (s *Service) PostProjectUpdate(ctx context.Context, projectID, bodyOverride, healthOverride string) (*Report, error) {
	if s.d.Linear == nil || !s.d.Linear.Enabled() {
		return nil, errors.New("Linear is not configured (set LINEAR_API_KEY)")
	}
	r, err := s.ComposeProjectUpdate(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bodyOverride) != "" {
		r.Body = bodyOverride
	}
	if healthOverride != "" {
		r.Health = healthOverride
	}
	url, err := s.d.Linear.PostProjectUpdate(ctx, projectID, r.Body, r.Health)
	if err != nil {
		return nil, err
	}
	r.Posted = true
	r.URL = url
	if err := s.d.Store.Insert(ctx, r); err != nil {
		return nil, err
	}
	slog.Info("report: posted project update", "project", projectID, "health", r.Health, "url", url)
	return r, nil
}

// ComposeWeeklyRoundup renders the roundup for the week containing weekOf.
func (s *Service) ComposeWeeklyRoundup(ctx context.Context, weekOf time.Time) (*Report, error) {
	start, end := WeekWindow(weekOf)
	w := WeekData{Start: start, End: end}
	var repos []string
	seen := map[string]bool{}
	if s.d.Linear != nil {
		if links, err := s.d.Linear.Links(ctx); err == nil {
			for _, l := range links {
				proj, err := s.d.Projects.GetProject(ctx, l.ProjectID)
				if err != nil {
					continue
				}
				pd, err := s.gather(ctx, proj, start, end)
				if err != nil {
					continue
				}
				w.Projects = append(w.Projects, pd)
				for _, r := range s.projectRepos(ctx, proj) {
					if !seen[r] {
						seen[r] = true
						repos = append(repos, r)
					}
				}
			}
		}
	}
	if s.d.GitHub != nil {
		if merged, err := s.d.GitHub.SearchMergedSince(ctx, start, ""); err == nil {
			for _, m := range merged {
				if !m.MergedAt.IsZero() && !m.MergedAt.Before(end) {
					continue
				}
				w.MergedPRs = append(w.MergedPRs, PRLine{Repo: m.Repo, Number: m.Number, Title: m.Title, URL: m.URL, When: m.MergedAt})
			}
		}
	}
	sortPRs(w.MergedPRs)
	if s.d.Sessions != nil {
		if stats, err := s.d.Sessions.StatsSince(ctx, start, end, nil); err == nil {
			for _, st := range stats {
				w.Activity = append(w.Activity, HarnessLine{Harness: st.Harness, Sessions: st.Sessions, TokensIn: st.TokensIn, TokensOut: st.TokensOut})
			}
		}
	}
	return &Report{Kind: KindWeeklyRoundup, WindowStart: start, WindowEnd: end, Body: RenderWeeklyRoundup(w), Health: HealthOnTrack}, nil
}

// PostWeeklyRoundup prepends the week to the rolling Linear document and, when configured,
// posts the same text as an update on the roundup project.
func (s *Service) PostWeeklyRoundup(ctx context.Context, weekOf time.Time, bodyOverride string) (*Report, error) {
	if s.d.Linear == nil || !s.d.Linear.Enabled() {
		return nil, errors.New("Linear is not configured (set LINEAR_API_KEY)")
	}
	if s.cfg.RoundupDocumentID == "" && s.cfg.RoundupProjectID == "" {
		return nil, errors.New("set REPORT_ROUNDUP_DOCUMENT_ID and/or REPORT_ROUNDUP_PROJECT_ID to post the weekly roundup")
	}
	r, err := s.ComposeWeeklyRoundup(ctx, weekOf)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bodyOverride) != "" {
		r.Body = bodyOverride
	}
	client := s.d.Linear.Client()
	if s.cfg.RoundupDocumentID != "" {
		doc, err := client.GetDocument(ctx, s.cfg.RoundupDocumentID)
		if err != nil {
			return nil, fmt.Errorf("roundup document: %w", err)
		}
		updated, err := client.UpdateDocumentContent(ctx, doc.ID, PrependSection(doc.Content, r.Body))
		if err != nil {
			return nil, fmt.Errorf("update roundup document: %w", err)
		}
		r.URL = updated.URL
	}
	if s.cfg.RoundupProjectID != "" {
		if url, err := s.d.Linear.PostProjectUpdate(ctx, s.cfg.RoundupProjectID, r.Body, HealthOnTrack); err != nil {
			slog.Warn("report: weekly project update failed", "error", err)
		} else if r.URL == "" {
			r.URL = url
		}
	}
	r.Posted = true
	if err := s.d.Store.Insert(ctx, r); err != nil {
		return nil, err
	}
	slog.Info("report: posted weekly roundup", "week", r.WindowStart.Format("2006-01-02"), "url", r.URL)
	return r, nil
}

// List returns stored reports.
func (s *Service) List(ctx context.Context, projectID, kind string, limit int) ([]*Report, error) {
	return s.d.Store.List(ctx, projectID, kind, limit)
}
