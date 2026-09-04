// Package schedule aggregates every recurring or pending automation in Flywheel into
// one operator-facing list — what runs, when it last ran, when it runs next — and
// lets the operator trigger a run now.
package schedule

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/codereview"
	"github.com/gabinante/flywheel/internal/linear"
	"github.com/gabinante/flywheel/internal/report"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/gabinante/flywheel/internal/settings"
)

// Action is one scheduled or pending automation.
type Action struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Kind            string     `json:"kind"` // recurring | scheduled | pending
	Enabled         bool       `json:"enabled"`
	IntervalSeconds int        `json:"interval_seconds,omitempty"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	Detail          string     `json:"detail,omitempty"`
	Count           int        `json:"count,omitempty"`
	ProjectID       string     `json:"project_id,omitempty"`
	Runnable        bool       `json:"runnable"`
	Outward         bool       `json:"outward"` // running it posts to GitHub or Linear
	SettingsSection string     `json:"settings_section,omitempty"`
}

// Deps are the services the schedule reads from.
type Deps struct {
	Linear   *linear.Syncer
	Sessions *sessions.Service
	Reviews  *codereview.Service
	Reports  *report.Service
	Settings *settings.Service
}

// Service builds the schedule view.
type Service struct{ d Deps }

// New returns a Service.
func New(d Deps) *Service { return &Service{d: d} }

// ErrUnknownAction is returned by Run for an unknown id.
var ErrUnknownAction = errors.New("unknown scheduled action")

// List returns every automation with its timing.
func (s *Service) List(ctx context.Context) []Action {
	now := time.Now()
	var out []Action
	var st settings.Settings
	if s.d.Settings != nil {
		st = s.d.Settings.Current()
	}

	if s.d.Linear != nil {
		ls := s.d.Linear.Status(ctx)
		a := Action{ID: "linear_sync", Name: "Linear sync", Kind: "recurring", Enabled: ls.Enabled, Runnable: ls.Enabled,
			Description:     "Mirror the Linear projects you lead into Flywheel and push state changes back.",
			IntervalSeconds: int(s.d.Linear.Interval() / time.Second), LastRunAt: ls.LastRunAt, LastError: ls.LastError, SettingsSection: "linear",
			Detail: fmt.Sprintf("%d projects, %d tickets linked", ls.ProjectsLinked, ls.TicketsLinked)}
		if ls.Enabled && ls.LastRunAt != nil {
			t := ls.LastRunAt.Add(s.d.Linear.Interval())
			a.NextRunAt = &t
		}
		out = append(out, a)
	}
	if s.d.Sessions != nil {
		ss := s.d.Sessions.Status(ctx)
		a := Action{ID: "sessions_ingest", Name: "Session ingestion", Kind: "recurring", Enabled: ss.Enabled, Runnable: true,
			Description:     "Read new Claude Code and Codex sessions from their local stores and link them to PRs and tickets.",
			IntervalSeconds: int(s.d.Sessions.Interval() / time.Second), LastRunAt: ss.LastRunAt, LastError: ss.LastError,
			Detail: fmt.Sprintf("%d sessions tracked", ss.SessionsTotal)}
		if ss.LastRunAt != nil {
			t := ss.LastRunAt.Add(s.d.Sessions.Interval())
			a.NextRunAt = &t
		}
		out = append(out, a)
	}
	if s.d.Reviews != nil {
		rs := s.d.Reviews.Status(ctx)
		iv := s.d.Reviews.PollInterval()
		var next *time.Time
		if rs.LastPollAt != nil {
			t := rs.LastPollAt.Add(iv)
			next = &t
		}
		out = append(out,
			Action{ID: "review_requested_poll", Name: "Watch review-requested:@me", Kind: "recurring", Enabled: rs.Enabled && rs.WatchRequested, Runnable: rs.Enabled,
				Description:     "Queue a review for PRs that request yours; re-review watched PRs on new commits or dismissal.",
				IntervalSeconds: int(iv / time.Second), LastRunAt: rs.LastPollAt, NextRunAt: next, LastError: rs.LastError, SettingsSection: "review",
				Detail: fmt.Sprintf("%d watching", rs.Watching), Count: rs.Watching, Outward: rs.Publish},
			Action{ID: "feedback_poll", Name: "Watch feedback on my PRs", Kind: "recurring", Enabled: rs.Enabled && rs.WatchAuthored, Runnable: rs.Enabled,
				Description:     "Detect actionable reviews landing on PRs you authored.",
				IntervalSeconds: int(iv / time.Second), LastRunAt: rs.LastPollAt, NextRunAt: next, SettingsSection: "review",
				Detail: fmt.Sprintf("%d new feedback rounds", rs.NewFeedback), Count: rs.NewFeedback},
			Action{ID: "review_queue", Name: "Code review queue", Kind: "pending", Enabled: rs.Enabled, Runnable: rs.Enabled && rs.Queued > 0,
				Description: "Queued PR reviews waiting for a free slot.", LastRunAt: rs.LastQueueRunAt, SettingsSection: "review",
				Detail: fmt.Sprintf("%d queued, %d active of %d", rs.Queued, rs.Active, rs.MaxConcurrent), Count: rs.Queued, Outward: rs.Publish},
			Action{ID: "feedback_auto_address", Name: "Address PR feedback automatically", Kind: "pending", Enabled: st.Feedback.AutoAddress, Runnable: false,
				Description: "New feedback rounds are dispatched to a harness as they arrive.", SettingsSection: "feedback",
				Detail: fmt.Sprintf("%d awaiting", rs.NewFeedback), Count: rs.NewFeedback, Outward: true},
		)
	}
	if s.d.Reports != nil {
		sc := s.d.Reports.Schedule(ctx, now)
		for _, p := range sc.Projects {
			next := p.NextDue
			a := Action{ID: "project_update:" + p.ProjectID, Name: "Project status update · " + p.ProjectName, Kind: "scheduled",
				Enabled: sc.ProjectUpdatesEnabled && sc.LinearConnected, Runnable: sc.LinearConnected, Outward: true,
				Description: "Post a delta status update to the linked Linear project.", IntervalSeconds: int(sc.Interval / time.Second),
				LastRunAt: p.LastPosted, NextRunAt: &next, ProjectID: p.ProjectID, SettingsSection: "reports"}
			if p.LastPosted == nil {
				a.Detail = "never posted"
			}
			out = append(out, a)
		}
		nw := sc.NextWeekly
		a := Action{ID: "weekly_roundup", Name: "Weekly roundup", Kind: "scheduled", Enabled: sc.WeeklyEnabled && sc.LinearConnected, Runnable: sc.LinearConnected, Outward: true,
			Description: fmt.Sprintf("Post the weekly roundup every %s at %02d:00.", sc.WeeklyDay, sc.WeeklyHour), NextRunAt: &nw, SettingsSection: "reports"}
		if sc.WeeklyPostedThisWeek {
			a.Detail = "posted this week"
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ki, kj := kindRank(out[i].Kind), kindRank(out[j].Kind)
		if ki != kj {
			return ki < kj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func kindRank(k string) int {
	switch k {
	case "pending":
		return 0
	case "scheduled":
		return 1
	}
	return 2
}

// Run triggers an action now. Outward actions post to Linear/GitHub — the operator asked.
func (s *Service) Run(ctx context.Context, id string) (*Action, error) {
	var err error
	switch {
	case id == "linear_sync" && s.d.Linear != nil:
		err = s.d.Linear.RunOnce(ctx)
	case id == "sessions_ingest" && s.d.Sessions != nil:
		err = s.d.Sessions.RunOnce(ctx)
	case (id == "review_requested_poll" || id == "feedback_poll") && s.d.Reviews != nil:
		err = s.d.Reviews.PollNow(ctx)
	case id == "review_queue" && s.d.Reviews != nil:
		s.d.Reviews.DrainNow(ctx)
	case strings.HasPrefix(id, "project_update:") && s.d.Reports != nil:
		_, err = s.d.Reports.PostProjectUpdate(ctx, strings.TrimPrefix(id, "project_update:"), "", "")
	case id == "weekly_roundup" && s.d.Reports != nil:
		_, err = s.d.Reports.PostWeeklyRoundup(ctx, time.Now(), "")
	default:
		return nil, ErrUnknownAction
	}
	if err != nil {
		return nil, err
	}
	for _, a := range s.List(ctx) {
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, ErrUnknownAction
}
