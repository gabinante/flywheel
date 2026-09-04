package linear

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// Provider is the external_provider value for Linear projections.
const Provider = "linear"

// Config controls the syncer.
type Config struct {
	Enabled        bool
	ProjectIDs     []string      // explicit Linear project IDs; empty = projects the viewer leads
	Interval       time.Duration // inbound poll interval (default 60s)
	DefaultTeamKey string        // team for issues filed into multi-team projects
	Overlap        time.Duration // re-read window to survive clock skew (default 2m)
}

// Status is the syncer's health summary.
type Status struct {
	Enabled        bool       `json:"enabled"`
	ViewerName     string     `json:"viewer_name,omitempty"`
	ViewerEmail    string     `json:"viewer_email,omitempty"`
	ProjectsLinked int        `json:"projects_linked"`
	TicketsLinked  int        `json:"tickets_linked"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	LastDurationMS int64      `json:"last_duration_ms"`
}

// OutputPatcher merges keys into a ticket's outputs (implemented by the ticket store).
type OutputPatcher interface {
	PatchOutputs(ctx context.Context, id string, patch map[string]any) error
}

// Syncer keeps Flywheel projects/tickets in step with Linear in both directions.
type Syncer struct {
	client   *Client
	store    *Store
	tickets  *ticket.Service
	projects *project.Service
	bus      events.Bus
	cfg      Config
	outputs  OutputPatcher // optional: records that a PR was attached so it is not re-posted

	mu       sync.Mutex
	status   Status
	viewer   *Viewer
	running  int32
	states   map[string]teamStatesCache
	statesMu sync.Mutex

	cfgMu      sync.RWMutex // guards client, cfg
	loopOnce   sync.Once
	subscribed bool
	loopCtx    context.Context
}

// cl returns the current API client (nil when Linear is not configured).
func (s *Syncer) cl() *Client {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.client
}

// conf returns a snapshot of the current config.
func (s *Syncer) conf() Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

// Reconfigure swaps the API key and config at runtime. Sync starts (or stops) on the
// next tick; the loop is created on first enablement if Start ran while disabled.
func (s *Syncer) Reconfigure(apiKey string, cfg Config) {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.Overlap <= 0 {
		cfg.Overlap = 2 * time.Minute
	}
	cfg.Enabled = cfg.Enabled && apiKey != ""
	s.cfgMu.Lock()
	if apiKey == "" {
		s.client = nil
	} else if s.client == nil || s.client.apiKey != apiKey {
		s.client = NewClient(apiKey, "")
	}
	s.cfg = cfg
	s.viewer = nil
	s.cfgMu.Unlock()
	s.mu.Lock()
	s.status.Enabled = cfg.Enabled
	s.status.ViewerName, s.status.ViewerEmail, s.status.LastError = "", "", ""
	s.mu.Unlock()
	if cfg.Enabled && s.loopCtx != nil {
		s.startLoop(s.loopCtx)
		go func() {
			if err := s.RunOnce(s.loopCtx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("linear: sync after reconfigure failed", "error", err)
			}
		}()
	}
	slog.Info("linear: reconfigured", "enabled", cfg.Enabled, "interval", cfg.Interval)
}

type teamStatesCache struct {
	states  []WorkflowState
	fetched time.Time
}

// NewSyncer builds a Syncer. client may be nil when Linear is not configured; the
// syncer then only serves status and lookups.
func NewSyncer(client *Client, store *Store, tickets *ticket.Service, projects *project.Service, bus events.Bus, cfg Config) *Syncer {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.Overlap <= 0 {
		cfg.Overlap = 2 * time.Minute
	}
	if client == nil {
		cfg.Enabled = false
	}
	return &Syncer{client: client, store: store, tickets: tickets, projects: projects, bus: bus, cfg: cfg,
		status: Status{Enabled: cfg.Enabled}, states: map[string]teamStatesCache{}}
}

// SetOutputPatcher lets the syncer remember which PR it already attached.
func (s *Syncer) SetOutputPatcher(p OutputPatcher) { s.outputs = p }

// Enabled reports whether Linear sync is active.
func (s *Syncer) Enabled() bool { return s.conf().Enabled && s.cl() != nil }

// Start runs the inbound poll loop and subscribes to ticket events for outbound sync.
func (s *Syncer) Start(ctx context.Context) {
	s.loopCtx = ctx
	if !s.subscribed {
		s.subscribe(ctx)
		s.subscribed = true
	}
	cfg := s.conf()
	if !cfg.Enabled {
		slog.Info("linear: sync disabled until an API key is set in settings")
		return
	}
	s.startLoop(ctx)
	slog.Info("linear: sync started", "interval", cfg.Interval, "explicit_projects", len(cfg.ProjectIDs))
}

// startLoop runs the poll loop once per process; each tick re-reads the config.
func (s *Syncer) startLoop(ctx context.Context) {
	s.loopOnce.Do(func() {
		go func() {
			if err := s.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("linear: initial sync failed", "error", err)
			}
			interval := s.conf().Interval
			t := time.NewTicker(interval)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if next := s.conf().Interval; next != interval && next > 0 {
						interval = next
						t.Reset(interval)
					}
					if err := s.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
						slog.Warn("linear: sync failed", "error", err)
					}
				}
			}
		}()
	})
}

// RunOnce discovers projects and pulls updated issues. Concurrent calls are coalesced.
func (s *Syncer) RunOnce(ctx context.Context) error {
	if !s.conf().Enabled || s.cl() == nil {
		return nil
	}
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil
	}
	defer atomic.StoreInt32(&s.running, 0)
	start := time.Now()
	var errs []error
	if s.viewer == nil {
		v, err := s.cl().Viewer(ctx)
		if err != nil {
			s.finish(start, err)
			return err
		}
		s.mu.Lock()
		s.viewer = v
		s.status.ViewerName, s.status.ViewerEmail = v.Name, v.Email
		s.mu.Unlock()
	}
	links, err := s.discover(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("discover: %w", err))
	}
	for _, l := range links {
		if ctx.Err() != nil {
			break
		}
		if err := s.syncProject(ctx, l); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", l.LinearProjectName, err))
			if errors.Is(err, ErrRateLimited) {
				break
			}
		}
	}
	err = errors.Join(errs...)
	s.finish(start, err)
	return err
}

func (s *Syncer) finish(start time.Time, err error) {
	now := time.Now()
	s.mu.Lock()
	s.status.LastRunAt = &now
	s.status.LastDurationMS = time.Since(start).Milliseconds()
	if err != nil {
		s.status.LastError = err.Error()
	} else {
		s.status.LastError = ""
	}
	s.mu.Unlock()
}

// discover ensures a Flywheel project exists for every in-scope Linear project.
func (s *Syncer) discover(ctx context.Context) ([]*ProjectLink, error) {
	var lps []Project
	if len(s.conf().ProjectIDs) > 0 {
		for _, id := range s.conf().ProjectIDs {
			p, err := s.cl().ProjectByID(ctx, id)
			if err != nil {
				return nil, err
			}
			lps = append(lps, *p)
		}
	} else {
		var err error
		lps, err = s.cl().LedProjects(ctx)
		if err != nil {
			return nil, err
		}
	}
	var links []*ProjectLink
	covered := map[string]bool{}
	// Manually linked projects first (PUT /projects/{id}/linear): resolve slug ids to UUIDs so the
	// led-project pass below finds the existing link instead of creating a duplicate project.
	if stored, err := s.store.ListLinks(ctx); err == nil {
		for _, l := range stored {
			lp, err := s.resolveProjectRef(ctx, l.LinearProjectID)
			if err != nil {
				slog.Warn("linear: stored link unresolved", "project", l.ProjectID, "linear_project", l.LinearProjectID, "error", err)
				covered[l.LinearProjectID] = true
				links = append(links, l)
				continue
			}
			l.LinearProjectID, l.LinearProjectName, l.LinearProjectURL = lp.ID, lp.Name, lp.URL
			l.TeamIDs, l.TeamKeys = nil, nil
			for _, t := range lp.Teams {
				l.TeamIDs = append(l.TeamIDs, t.ID)
				l.TeamKeys = append(l.TeamKeys, t.Key)
			}
			_ = s.store.UpsertLink(ctx, l)
			covered[lp.ID] = true
			links = append(links, l)
		}
	}
	for _, lp := range lps {
		if covered[lp.ID] {
			continue
		}
		link, err := s.ensureLink(ctx, lp)
		if err != nil {
			slog.Warn("linear: ensure project link failed", "project", lp.Name, "error", err)
			continue
		}
		covered[link.LinearProjectID] = true
		links = append(links, link)
	}
	return links, nil
}

var linearProjectURLRe = regexp.MustCompile(`linear\.app/[^/]+/project/(?:[a-z0-9-]*?-)?([0-9a-f]{8,})(?:/|$|\?)`)

// ParseProjectRef extracts a Linear project id (UUID or URL slug id) from a URL or raw id.
func ParseProjectRef(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if m := linearProjectURLRe.FindStringSubmatch(ref); m != nil {
		return m[1], true
	}
	if uuidLike(ref) || (len(ref) >= 8 && isHex(ref)) {
		return ref, true
	}
	return "", false
}

func uuidLike(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

func isHex(s string) bool {
	for _, r := range strings.ToLower(s) {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return s != ""
}

// resolveProjectRef fetches a project by UUID, falling back to slug-id lookup.
func (s *Syncer) resolveProjectRef(ctx context.Context, id string) (*Project, error) {
	if s.cl() == nil {
		return nil, errors.New("linear: not configured")
	}
	if uuidLike(id) {
		return s.cl().ProjectByID(ctx, id)
	}
	return s.cl().ProjectBySlugID(ctx, id)
}

// LinkProject links a Flywheel project to a Linear project given a URL or id. When
// Linear is configured the project is resolved immediately and its issues sync on the
// next pass; otherwise the link is stored and resolved once a key is present.
func (s *Syncer) LinkProject(ctx context.Context, projectID, ref string) (*ProjectLink, error) {
	id, ok := ParseProjectRef(ref)
	if !ok {
		return nil, fmt.Errorf("not a Linear project URL or id: %q", ref)
	}
	if existing, err := s.store.LinkByLinearProject(ctx, id); err == nil && existing != nil && existing.ProjectID != projectID {
		return nil, fmt.Errorf("Linear project is already linked to Flywheel project %s", existing.ProjectID)
	}
	link := &ProjectLink{ProjectID: projectID, LinearProjectID: id}
	if strings.Contains(ref, "linear.app") {
		link.LinearProjectURL = strings.TrimSpace(ref)
	}
	if s.cl() != nil {
		lp, err := s.resolveProjectRef(ctx, id)
		if err != nil {
			return nil, err
		}
		link.LinearProjectID, link.LinearProjectName, link.LinearProjectURL = lp.ID, lp.Name, lp.URL
		for _, t := range lp.Teams {
			link.TeamIDs = append(link.TeamIDs, t.ID)
			link.TeamKeys = append(link.TeamKeys, t.Key)
		}
	}
	if err := s.store.UpsertLink(ctx, link); err != nil {
		return nil, err
	}
	if s.conf().Enabled {
		go func() {
			if err := s.syncProject(context.WithoutCancel(ctx), link); err != nil {
				slog.Warn("linear: initial sync after manual link failed", "project", projectID, "error", err)
			}
		}()
	}
	return link, nil
}

// UnlinkProject removes the Linear link for a project.
func (s *Syncer) UnlinkProject(ctx context.Context, projectID string) error {
	return s.store.DeleteLink(ctx, projectID)
}

// ensureLink creates the Flywheel project for a Linear project on first sight.
func (s *Syncer) ensureLink(ctx context.Context, lp Project) (*ProjectLink, error) {
	link, err := s.store.LinkByLinearProject(ctx, lp.ID)
	if err != nil {
		return nil, err
	}
	teamIDs := make([]string, 0, len(lp.Teams))
	teamKeys := make([]string, 0, len(lp.Teams))
	for _, t := range lp.Teams {
		teamIDs = append(teamIDs, t.ID)
		teamKeys = append(teamKeys, t.Key)
	}
	if link == nil {
		orgID, err := s.store.DefaultOrgID(ctx)
		if err != nil {
			return nil, err
		}
		if orgID == "" {
			return nil, errors.New("no organization exists yet; sign in once so the operator org is created")
		}
		slug := Slugify(lp.Name)
		for i := 2; ; i++ {
			existing, err := s.projects.GetBySlug(ctx, orgID, slug)
			if err != nil || existing == nil {
				break
			}
			slug = fmt.Sprintf("%s-%d", Slugify(lp.Name), i)
		}
		p, err := s.projects.CreateProject(ctx, orgID, lp.Name, slug, "", nil)
		if err != nil {
			return nil, fmt.Errorf("create project: %w", err)
		}
		// Imported issues must not trigger workers until the operator opts in.
		_ = s.projects.UpdateDispatchEnabled(ctx, p.ID, false)
		_ = s.projects.UpdateDescription(ctx, p.ID, "Mirrors Linear project "+lp.URL)
		link = &ProjectLink{ProjectID: p.ID}
		slog.Info("linear: created project for Linear project", "project", p.Slug, "linear_project", lp.Name)
	}
	link.LinearProjectID = lp.ID
	link.LinearProjectName = lp.Name
	link.LinearProjectURL = lp.URL
	link.TeamIDs = teamIDs
	link.TeamKeys = teamKeys
	if err := s.store.UpsertLink(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

// syncProject pulls issues updated since the last pass.
func (s *Syncer) syncProject(ctx context.Context, link *ProjectLink) error {
	start := time.Now()
	var since time.Time
	if link.SyncedAt != nil {
		since = link.SyncedAt.Add(-s.conf().Overlap)
	}
	n := 0
	err := s.cl().IssuesUpdatedSince(ctx, link.LinearProjectID, since, func(is Issue) error {
		n++
		return s.upsertIssue(ctx, link, is)
	})
	if err != nil {
		_ = s.store.MarkSynced(ctx, link.ProjectID, start, err.Error())
		return err
	}
	if n > 0 {
		slog.Info("linear: synced issues", "project", link.LinearProjectName, "issues", n)
	}
	return s.store.MarkSynced(ctx, link.ProjectID, start, "")
}

func refFromIssue(is Issue) ticket.ExternalRef {
	ref := ticket.ExternalRef{
		Provider:   Provider,
		ExternalID: is.ID,
		Identifier: is.Identifier,
		URL:        is.URL,
		StateName:  is.State.Name,
		StateType:  is.State.Type,
		TeamKey:    is.Team.Key,
		Priority:   is.Priority,
		Labels:     is.Labels,
		BranchName: is.BranchName,
	}
	if is.Assignee != nil {
		ref.Assignee = firstNonEmpty(is.Assignee.Name, is.Assignee.Email)
	}
	if !is.UpdatedAt.IsZero() {
		u := is.UpdatedAt
		ref.UpdatedAt = &u
	}
	return ref
}

// upsertIssue projects one Linear issue onto a ticket.
func (s *Syncer) upsertIssue(ctx context.Context, link *ProjectLink, is Issue) error {
	ticketID, prev, err := s.store.RefByExternalID(ctx, Provider, is.ID)
	if err != nil {
		return err
	}
	mapped := MapState(is.State.Type, is.State.Name)
	ref := refFromIssue(is)
	if ticketID == "" {
		t, err := s.tickets.ImportExternal(ctx, ticket.ExternalImport{
			ProjectID:  link.ProjectID,
			Title:      is.Title,
			Type:       MapType(is.Labels),
			Priority:   MapPriority(is.Priority),
			State:      mapped,
			Objective:  ticket.Objective{Description: is.Description},
			Provider:   Provider,
			Identifier: is.Identifier,
			CreatedAt:  is.CreatedAt,
		})
		if err != nil {
			return fmt.Errorf("import %s: %w", is.Identifier, err)
		}
		return s.store.UpsertRef(ctx, t.ID, ref)
	}
	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	if t.Title != is.Title || t.Objective.Description != is.Description {
		obj := t.Objective
		obj.Description = is.Description
		if err := s.tickets.UpdateTitleAndObjective(ctx, t.ID, is.Title, obj); err != nil {
			return err
		}
	}
	// Forward moves always apply; backward moves only when Linear itself changed
	// state type, so Flywheel-side progress is not clobbered by a stale column.
	if t.State != mapped && (stateRank(mapped) > stateRank(t.State) || prev == nil || prev.StateType != is.State.Type) {
		if err := s.tickets.SetStateFromExternal(ctx, t.ID, mapped, Provider); err != nil {
			slog.Warn("linear: apply state failed", "ticket", t.ID, "state", mapped, "error", err)
		}
	}
	return s.store.UpsertRef(ctx, t.ID, ref)
}

// ---- outbound -------------------------------------------------------------

func (s *Syncer) subscribe(ctx context.Context) {
	if s.bus == nil {
		return
	}
	s.bus.Subscribe(events.EventTicketCreated, func(_ context.Context, e events.Event) {
		if payloadString(e, "external_provider") != "" {
			return
		}
		go s.fileIssue(ctx, payloadString(e, "ticket_id"))
	})
	stateEvents := map[string]ticket.State{
		events.EventTicketPlanning:  ticket.StatePlanning,
		events.EventTicketStarted:   ticket.StateExecuting,
		events.EventTicketSubmitted: ticket.StateAwaitingValidation,
		events.EventTicketApproved:  ticket.StateValidated,
		events.EventTicketValidated: ticket.StateValidated,
		events.EventTicketClosed:    ticket.StateClosed,
		events.EventTicketReopened:  ticket.StateDraft,
	}
	for evt, st := range stateEvents {
		st := st
		s.bus.Subscribe(evt, func(_ context.Context, e events.Event) {
			if payloadString(e, "external_provider") != "" {
				return
			}
			go s.pushState(ctx, payloadString(e, "ticket_id"), st, "")
		})
	}
	// A submitted ticket carries its PR: attach it to the Linear issue and leave the Abstract comment
	// the operator's hook used to post by hand.
	s.bus.Subscribe(events.EventTicketSubmitted, func(_ context.Context, e events.Event) {
		if payloadString(e, "external_provider") != "" {
			return
		}
		go s.attachSubmittedPR(ctx, payloadString(e, "ticket_id"))
	})
	s.bus.Subscribe(events.EventTicketCancelled, func(_ context.Context, e events.Event) {
		if payloadString(e, "external_provider") != "" {
			return
		}
		go s.pushState(ctx, payloadString(e, "ticket_id"), ticket.StateClosed, "canceled")
	})
}

func payloadString(e events.Event, key string) string {
	if e.Payload == nil {
		return ""
	}
	if v, ok := e.Payload[key].(string); ok {
		return v
	}
	return ""
}

// fileIssue creates the Linear issue for a Flywheel-originated ticket in a linked project.
func (s *Syncer) fileIssue(ctx context.Context, ticketID string) {
	if ticketID == "" {
		return
	}
	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil || t == nil || t.External != nil {
		return
	}
	link, err := s.store.LinkByProject(ctx, t.ProjectID)
	if err != nil || link == nil {
		return
	}
	teamID := s.teamForLink(link, "")
	if teamID == "" {
		slog.Warn("linear: cannot file issue, project has no team", "ticket", t.ID)
		return
	}
	is, err := s.cl().CreateIssue(ctx, CreateIssueInput{
		TeamID:      teamID,
		ProjectID:   link.LinearProjectID,
		Title:       t.Title,
		Description: AbstractBody(t),
		Priority:    ToLinearPriority(t.Priority),
	})
	if err != nil {
		slog.Warn("linear: file issue failed", "ticket", t.ID, "error", err)
		return
	}
	if err := s.store.UpsertRef(ctx, t.ID, refFromIssue(*is)); err != nil {
		slog.Warn("linear: store ref failed", "ticket", t.ID, "error", err)
		return
	}
	slog.Info("linear: filed issue", "ticket", t.ID, "issue", is.Identifier)
	if t.State != ticket.StateDraft {
		s.pushState(ctx, t.ID, t.State, "")
	}
}

// teamForLink picks the team to file into: the configured default, the team
// matching preferKey, or the project's first team.
func (s *Syncer) teamForLink(link *ProjectLink, preferKey string) string {
	for _, key := range []string{preferKey, s.conf().DefaultTeamKey} {
		if key == "" {
			continue
		}
		for i, k := range link.TeamKeys {
			if strings.EqualFold(k, key) && i < len(link.TeamIDs) {
				return link.TeamIDs[i]
			}
		}
	}
	if len(link.TeamIDs) > 0 {
		return link.TeamIDs[0]
	}
	return ""
}

func (s *Syncer) teamStates(ctx context.Context, teamID string) ([]WorkflowState, error) {
	s.statesMu.Lock()
	c, ok := s.states[teamID]
	s.statesMu.Unlock()
	if ok && time.Since(c.fetched) < 30*time.Minute {
		return c.states, nil
	}
	states, err := s.cl().TeamStates(ctx, teamID)
	if err != nil {
		return nil, err
	}
	s.statesMu.Lock()
	s.states[teamID] = teamStatesCache{states: states, fetched: time.Now()}
	s.statesMu.Unlock()
	return states, nil
}

// pushState moves the linked Linear issue to the column matching a Flywheel state.
// overrideType forces a Linear state type (used for cancellation).
func (s *Syncer) pushState(ctx context.Context, ticketID string, st ticket.State, overrideType string) {
	if ticketID == "" || s.cl() == nil {
		return
	}
	ref, err := s.store.RefByTicketID(ctx, ticketID)
	if err != nil || ref == nil {
		return
	}
	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil || t == nil {
		return
	}
	link, err := s.store.LinkByProject(ctx, t.ProjectID)
	if err != nil || link == nil {
		return
	}
	teamID := s.teamForLink(link, ref.TeamKey)
	if teamID == "" {
		return
	}
	stateType, names := DesiredStateType(st)
	if overrideType != "" {
		stateType, names = overrideType, []string{"canceled", "cancelled", "won't do"}
	}
	if stateType == "" {
		return
	}
	states, err := s.teamStates(ctx, teamID)
	if err != nil {
		slog.Warn("linear: team states failed", "team", teamID, "error", err)
		return
	}
	target := PickState(states, stateType, names)
	if target == nil || target.Name == ref.StateName {
		return
	}
	is, err := s.cl().UpdateIssueState(ctx, ref.ExternalID, target.ID)
	if err != nil {
		slog.Warn("linear: push state failed", "ticket", ticketID, "state", target.Name, "error", err)
		return
	}
	_ = s.store.UpsertRef(ctx, ticketID, refFromIssue(*is))
	slog.Info("linear: moved issue", "issue", ref.Identifier, "state", target.Name)
}

// attachSubmittedPR links the ticket's PR to its Linear issue and comments once.
func (s *Syncer) attachSubmittedPR(ctx context.Context, ticketID string) {
	if ticketID == "" || s.cl() == nil {
		return
	}
	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil || t == nil || t.External == nil {
		return
	}
	prURL, _ := t.Outputs["pr_url"].(string)
	if prURL == "" {
		return
	}
	if attached, _ := t.Outputs["_linear_pr_attached"].(string); attached == prURL {
		return
	}
	ref, err := s.store.RefByTicketID(ctx, ticketID)
	if err != nil || ref == nil {
		return
	}
	if err := s.cl().AttachPullRequest(ctx, ref.ExternalID, prURL); err != nil {
		slog.Warn("linear: attach PR failed", "ticket", ticketID, "pr", prURL, "error", err)
	}
	body := AbstractBody(t) + "\n\nPull request: " + prURL
	if _, err := s.cl().CreateComment(ctx, ref.ExternalID, body); err != nil {
		slog.Warn("linear: PR comment failed", "ticket", ticketID, "error", err)
		return
	}
	if s.outputs != nil {
		_ = s.outputs.PatchOutputs(ctx, ticketID, map[string]any{"_linear_pr_attached": prURL})
	}
	slog.Info("linear: attached PR to issue", "issue", ref.Identifier, "pr", prURL)
}

// Comment posts a comment on the Linear issue linked to a ticket.
func (s *Syncer) Comment(ctx context.Context, ticketID, body string) error {
	if s.cl() == nil {
		return errors.New("linear: not configured")
	}
	ref, err := s.store.RefByTicketID(ctx, ticketID)
	if err != nil {
		return err
	}
	if ref == nil {
		return fmt.Errorf("ticket %s has no Linear issue", ticketID)
	}
	_, err = s.cl().CreateComment(ctx, ref.ExternalID, body)
	return err
}

// AttachPullRequest links a PR to the Linear issue behind a ticket.
func (s *Syncer) AttachPullRequest(ctx context.Context, ticketID, prURL string) error {
	if s.cl() == nil {
		return errors.New("linear: not configured")
	}
	ref, err := s.store.RefByTicketID(ctx, ticketID)
	if err != nil {
		return err
	}
	if ref == nil {
		return fmt.Errorf("ticket %s has no Linear issue", ticketID)
	}
	return s.cl().AttachPullRequest(ctx, ref.ExternalID, prURL)
}

// SyncProject runs an inbound pass for one Flywheel project now.
func (s *Syncer) SyncProject(ctx context.Context, projectID string) (*ProjectLink, error) {
	link, err := s.store.LinkByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, nil
	}
	if !s.conf().Enabled {
		return link, errors.New("linear: not configured")
	}
	err = s.syncProject(ctx, link)
	fresh, _ := s.store.LinkByProject(ctx, projectID)
	if fresh != nil {
		link = fresh
	}
	return link, err
}

// Link returns the project link, or nil.
func (s *Syncer) Link(ctx context.Context, projectID string) (*ProjectLink, error) {
	return s.store.LinkByProject(ctx, projectID)
}

// Links returns all project links.
func (s *Syncer) Links(ctx context.Context) ([]*ProjectLink, error) { return s.store.ListLinks(ctx) }

// TicketCount returns how many tickets in a project mirror Linear issues.
func (s *Syncer) TicketCount(ctx context.Context, projectID string) (int, error) {
	return s.store.CountRefsByProject(ctx, projectID)
}

// Status returns the syncer's health plus counts.
func (s *Syncer) Status(ctx context.Context) Status {
	s.mu.Lock()
	st := s.status
	s.mu.Unlock()
	if links, err := s.store.ListLinks(ctx); err == nil {
		st.ProjectsLinked = len(links)
	}
	if n, err := s.store.CountRefs(ctx); err == nil {
		st.TicketsLinked = n
	}
	return st
}

// AbstractBody renders a ticket as a Linear issue description in the operator's
// required shape: Abstract, Components, Before, After.
func AbstractBody(t *ticket.Ticket) string {
	desc := strings.TrimSpace(t.Objective.Description)
	if desc == "" {
		desc = t.Title
	}
	components := t.TargetRepo
	if components == "" {
		components = "See project repositories"
	}
	after := "Done when: " + t.Title
	if len(t.Objective.SuccessCriteria) > 0 {
		var b strings.Builder
		for _, c := range t.Objective.SuccessCriteria {
			b.WriteString("\n- ")
			b.WriteString(c)
		}
		after = b.String()
	}
	return fmt.Sprintf("## Abstract\n%s\n\n**Components:** %s\n\n**Before:** Not started; tracked as Flywheel ticket `%s`.\n\n**After:** %s\n",
		desc, components, t.ID, after)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// PostProjectUpdate posts a status update on the Linear project linked to a Flywheel project.
func (s *Syncer) PostProjectUpdate(ctx context.Context, projectID, body, health string) (string, error) {
	if s.cl() == nil {
		return "", errors.New("linear: not configured")
	}
	link, err := s.store.LinkByProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	if link == nil {
		return "", fmt.Errorf("project %s is not linked to Linear", projectID)
	}
	return s.cl().CreateProjectUpdate(ctx, link.LinearProjectID, body, health)
}

// Client exposes the underlying API client (nil when Linear is not configured).
func (s *Syncer) Client() *Client { return s.cl() }
