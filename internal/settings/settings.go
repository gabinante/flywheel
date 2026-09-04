// Package settings holds the operator-editable runtime configuration — Linear
// credentials and sync, code review, feedback, and reporting — persisted in
// Postgres and applied to the running services without a restart. Environment
// variables seed the defaults for a fresh install.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gabinante/flywheel/config"
	"github.com/gabinante/flywheel/internal/codereview"
	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/harness"
	"github.com/gabinante/flywheel/internal/linear"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/report"
)

// Settings is the complete operator configuration.
type Settings struct {
	Linear   LinearSettings         `json:"linear"`
	Review   ReviewSettings         `json:"review"`
	Feedback FeedbackSettings       `json:"feedback"`
	Report   ReportSettings         `json:"report"`
	Dispatch DispatchSettings       `json:"dispatch"`
	Harness  HarnessSettings        `json:"harnesses"`
	Workers  project.DispatchConfig `json:"workers"` // shared worker profiles, roles and routing policies
	Prompts  map[string]string      `json:"prompts"` // overrides of built-in prompts by id (see internal/prompts)
	Layout   LayoutSettings         `json:"layout"`  // UI arrangement (not shown on the settings page)
}

// ResolvedWorker is what a role resolves to for a single-turn service (review, feedback).
type ResolvedWorker struct {
	WorkerID     string
	WorkerName   string
	Harness      string // claude | codex
	Model        string
	Effort       string
	SystemPrompt string
}

// ResolveRole picks the worker a role routes to in the shared library: the role's
// policy in order, else the first enabled worker. ok is false when nothing matches.
func (s Settings) ResolveRole(roleID string) (ResolvedWorker, bool) {
	roleID = strings.ToLower(strings.TrimSpace(roleID))
	if roleID == "" {
		return ResolvedWorker{}, false
	}
	cfg := s.Workers.Normalized()
	byID := map[string]project.DispatchWorkerProfile{}
	for _, w := range cfg.Workers {
		if w.Enabled {
			byID[w.ID] = w
		}
	}
	var candidates []project.DispatchWorkerProfile
	for key, pol := range cfg.Policies {
		if strings.ToLower(strings.TrimSpace(key)) != roleID {
			continue
		}
		for _, id := range pol.WorkerIDs {
			if w, ok := byID[strings.TrimSpace(id)]; ok {
				candidates = append(candidates, w)
			}
		}
	}
	if len(candidates) == 0 {
		for _, w := range cfg.Workers {
			if w.Enabled {
				candidates = append(candidates, w)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return ResolvedWorker{}, false
	}
	w := candidates[0]
	h := strings.ToLower(w.Driver)
	if h != "claude" && h != "codex" {
		h = ""
	}
	return ResolvedWorker{WorkerID: w.ID, WorkerName: w.Name, Harness: h, Model: w.Model, Effort: w.ReasoningEffort, SystemPrompt: w.SystemPrompt}, true
}

// HarnessSettings holds per-harness binaries and default model/effort. Review,
// feedback and dispatch pick a harness and only override these when set.
type HarnessSettings struct {
	Claude HarnessDefaults `json:"claude"`
	Codex  HarnessDefaults `json:"codex"`
}

// HarnessDefaults is one harness's configuration.
type HarnessDefaults struct {
	Bin             string `json:"bin"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
}

// Defaults returns the defaults for a harness name ("claude"/"claude_code" or "codex").
func (h HarnessSettings) Defaults(name string) HarnessDefaults {
	switch strings.ToLower(name) {
	case "codex":
		return h.Codex
	default:
		return h.Claude
	}
}

// Resolve fills a blank model/effort from the harness defaults.
func (h HarnessSettings) Resolve(name, model, effort string) (string, string) {
	d := h.Defaults(name)
	if model == "" {
		model = d.Model
	}
	if effort == "" {
		effort = d.ReasoningEffort
	}
	return model, effort
}

// RunnerConfig converts to the harness runner's config.
func (s Settings) RunnerConfig() harness.Config {
	return harness.Config{ClaudeBin: s.Harness.Claude.Bin, CodexBin: s.Harness.Codex.Bin}
}

// DispatchSettings configures the ticket dispatcher (implementation workers).
type DispatchSettings struct {
	Enabled         bool   `json:"enabled"`
	MaxWorkers      int    `json:"max_workers"`
	Driver          string `json:"driver"` // claude | codex | generic
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	WorktreeDir     string `json:"worktree_dir"`
	WorkerAPIKey    string `json:"worker_api_key"` // Flywheel key workers use for MCP; minted on first boot, never returned
}

// LayoutSettings holds the operator's UI arrangement.
type LayoutSettings struct {
	ProjectSections []ProjectSection `json:"project_sections"`
}

// ProjectSection is a named, ordered group of projects on the projects page.
type ProjectSection struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ProjectIDs []string `json:"project_ids"`
	Collapsed  bool     `json:"collapsed"`
}

// LinearSettings configures the ticket store.
type LinearSettings struct {
	APIKey              string   `json:"api_key"` // stored; never returned in full
	Enabled             bool     `json:"enabled"`
	ProjectIDs          []string `json:"project_ids"`
	DefaultTeamKey      string   `json:"default_team_key"`
	SyncIntervalSeconds int      `json:"sync_interval_seconds"`
}

// ReviewSettings configures PR-keyed code review.
type ReviewSettings struct {
	Enabled             bool   `json:"enabled"`
	RoleID              string `json:"role_id"` // worker role from the shared library; empty = use harness/model/effort below
	Harness             string `json:"harness"`
	Model               string `json:"model"`
	ReasoningEffort     string `json:"reasoning_effort"`
	Publish             bool   `json:"publish"`
	WatchRequested      bool   `json:"watch_requested"`
	WatchAuthored       bool   `json:"watch_authored"`
	SkipDrafts          bool   `json:"skip_drafts"`
	MaxConcurrent       int    `json:"max_concurrent"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	RepoRoot            string `json:"repo_root"`
}

// FeedbackSettings configures the address-feedback workflow.
type FeedbackSettings struct {
	RoleID          string `json:"role_id"` // worker role from the shared library; empty = use harness/model/effort below
	Harness         string `json:"harness"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	AutoAddress     bool   `json:"auto_address"`
}

// ReportSettings configures Linear reporting.
type ReportSettings struct {
	ProjectUpdatesEnabled      bool   `json:"project_updates_enabled"`
	ProjectUpdateIntervalHours int    `json:"project_update_interval_hours"`
	WeeklyEnabled              bool   `json:"weekly_enabled"`
	WeeklyDay                  string `json:"weekly_day"`
	WeeklyHour                 int    `json:"weekly_hour"`
	RoundupDocumentID          string `json:"roundup_document_id"`
	RoundupProjectID           string `json:"roundup_project_id"`
	DefaultHealth              string `json:"default_health"`
}

// FromConfig seeds settings from the environment-derived config.
func FromConfig(cfg *config.Config) Settings {
	return Settings{
		Linear: LinearSettings{
			APIKey: cfg.Linear.APIKey, Enabled: cfg.Linear.Enabled || cfg.Linear.APIKey == "", ProjectIDs: cfg.Linear.ProjectIDs,
			DefaultTeamKey: cfg.Linear.DefaultTeamKey, SyncIntervalSeconds: int(cfg.Linear.Interval / time.Second),
		},
		Review: ReviewSettings{
			Enabled: cfg.Review.Enabled, Harness: cfg.Review.Harness, Model: cfg.Review.Model, ReasoningEffort: cfg.Review.Effort,
			Publish: cfg.Review.Publish, WatchRequested: cfg.Review.WatchRequested, WatchAuthored: cfg.Review.WatchAuthored,
			SkipDrafts: cfg.Review.SkipDrafts, MaxConcurrent: cfg.Review.MaxConcurrent, PollIntervalSeconds: int(cfg.Review.PollInterval / time.Second),
			RepoRoot: cfg.Review.RepoRoot,
		},
		Feedback: FeedbackSettings{Harness: cfg.Feedback.Harness, Model: cfg.Feedback.Model, ReasoningEffort: cfg.Feedback.Effort, AutoAddress: cfg.Feedback.AutoAddress},
		Dispatch: DispatchSettings{
			Enabled: cfg.Dispatch.Enabled, MaxWorkers: cfg.Dispatch.MaxWorkers, Driver: cfg.Dispatch.AgentDriver, Model: cfg.Dispatch.AgentModel,
			ReasoningEffort: cfg.Dispatch.AgentReasoningEffort, WorktreeDir: cfg.Dispatch.WorktreeDir, WorkerAPIKey: cfg.Dispatch.APIKey,
		},
		Harness: harnessDefaultsFromConfig(cfg),
		Report: ReportSettings{
			ProjectUpdatesEnabled: cfg.Report.ProjectUpdatesEnabled, ProjectUpdateIntervalHours: int(cfg.Report.ProjectUpdateInterval / time.Hour),
			WeeklyEnabled: cfg.Report.WeeklyEnabled, WeeklyDay: cfg.Report.WeeklyDay, WeeklyHour: cfg.Report.WeeklyHour,
			RoundupDocumentID: cfg.Report.RoundupDocumentID, RoundupProjectID: cfg.Report.RoundupProjectID, DefaultHealth: cfg.Report.DefaultHealth,
		},
	}
}

func harnessDefaultsFromConfig(cfg *config.Config) HarnessSettings {
	h := HarnessSettings{Claude: HarnessDefaults{Bin: cfg.Dispatch.ClaudePath}, Codex: HarnessDefaults{Bin: "codex"}}
	if cfg.Dispatch.AgentDriver == "codex" && cfg.Dispatch.AgentCLIPath != "" {
		h.Codex.Bin = cfg.Dispatch.AgentCLIPath
	}
	return h
}

// Normalize fills invalid or empty values with safe defaults.
func (s *Settings) Normalize() {
	if s.Linear.SyncIntervalSeconds < 15 {
		s.Linear.SyncIntervalSeconds = 60
	}
	if s.Review.Harness == "" {
		s.Review.Harness = "codex"
	}
	if s.Review.MaxConcurrent <= 0 {
		s.Review.MaxConcurrent = 2
	}
	if s.Review.PollIntervalSeconds < 30 {
		s.Review.PollIntervalSeconds = 120
	}
	if s.Feedback.Harness == "" {
		s.Feedback.Harness = "claude"
	}
	if s.Report.ProjectUpdateIntervalHours <= 0 {
		s.Report.ProjectUpdateIntervalHours = 48
	}
	if s.Report.WeeklyDay == "" {
		s.Report.WeeklyDay = "Friday"
	}
	if s.Report.WeeklyHour < 0 || s.Report.WeeklyHour > 23 {
		s.Report.WeeklyHour = 16
	}
	if s.Report.DefaultHealth == "" {
		s.Report.DefaultHealth = report.HealthOnTrack
	}
	if s.Linear.ProjectIDs == nil {
		s.Linear.ProjectIDs = []string{}
	}
	if s.Dispatch.MaxWorkers <= 0 {
		s.Dispatch.MaxWorkers = 4
	}
	if s.Harness.Claude.Bin == "" {
		s.Harness.Claude.Bin = "claude"
	}
	if s.Harness.Codex.Bin == "" {
		s.Harness.Codex.Bin = "codex"
	}
	if s.Dispatch.Driver == "" {
		s.Dispatch.Driver = "claude"
	}
	s.Workers = s.Workers.Normalized()
	if s.Prompts == nil {
		s.Prompts = map[string]string{}
	}
	if s.Layout.ProjectSections == nil {
		s.Layout.ProjectSections = []ProjectSection{}
	}
	for i := range s.Layout.ProjectSections {
		if s.Layout.ProjectSections[i].ProjectIDs == nil {
			s.Layout.ProjectSections[i].ProjectIDs = []string{}
		}
	}
}

// LinearConfig converts to the syncer's config and API key.
func (s Settings) LinearConfig() (string, linear.Config) {
	return s.Linear.APIKey, linear.Config{
		Enabled:        s.Linear.Enabled && s.Linear.APIKey != "",
		ProjectIDs:     s.Linear.ProjectIDs,
		Interval:       time.Duration(s.Linear.SyncIntervalSeconds) * time.Second,
		DefaultTeamKey: s.Linear.DefaultTeamKey,
	}
}

// ReviewConfig converts to the code review service's configs, filling blank
// model/effort from the harness defaults.
func (s Settings) ReviewConfig() (codereview.Config, codereview.FeedbackConfig) {
	rh, rmodel, reffort, rprompt := s.Review.Harness, s.Review.Model, s.Review.ReasoningEffort, ""
	if w, ok := s.ResolveRole(s.Review.RoleID); ok {
		if w.Harness != "" {
			rh = w.Harness
		}
		rmodel, reffort, rprompt = w.Model, w.Effort, w.SystemPrompt
	}
	fh, fmodel, feffort, fprompt := s.Feedback.Harness, s.Feedback.Model, s.Feedback.ReasoningEffort, ""
	if w, ok := s.ResolveRole(s.Feedback.RoleID); ok {
		if w.Harness != "" {
			fh = w.Harness
		}
		fmodel, feffort, fprompt = w.Model, w.Effort, w.SystemPrompt
	}
	rm, re := s.Harness.Resolve(rh, rmodel, reffort)
	fm, fe := s.Harness.Resolve(fh, fmodel, feffort)
	return codereview.Config{
			Enabled: s.Review.Enabled, Harness: rh, Model: rm, Effort: re, PromptPrefix: rprompt,
			Publish: s.Review.Publish, PollInterval: time.Duration(s.Review.PollIntervalSeconds) * time.Second,
			MaxConcurrent: s.Review.MaxConcurrent, RepoRoot: s.Review.RepoRoot, WatchRequested: s.Review.WatchRequested,
			WatchAuthored: s.Review.WatchAuthored, SkipDrafts: s.Review.SkipDrafts,
		}, codereview.FeedbackConfig{
			Harness: fh, Model: fm, Effort: fe, AutoAddress: s.Feedback.AutoAddress, PromptPrefix: fprompt,
		}
}

// DispatchRuntime converts to the dispatcher's runtime configuration, filling blank
// model/effort from the harness defaults for the chosen driver.
func (s Settings) DispatchRuntime() dispatch.Runtime {
	m, e := s.Harness.Resolve(s.Dispatch.Driver, s.Dispatch.Model, s.Dispatch.ReasoningEffort)
	return dispatch.Runtime{
		Enabled: s.Dispatch.Enabled, MaxWorkers: s.Dispatch.MaxWorkers, Driver: s.Dispatch.Driver, Model: m, Effort: e,
		ClaudePath: s.Harness.Claude.Bin, CodexPath: s.Harness.Codex.Bin, Workers: s.Workers.Normalized(),
	}
}

// ReportConfig converts to the report service's config.
func (s Settings) ReportConfig() report.Config {
	return report.Config{
		ProjectUpdatesEnabled: s.Report.ProjectUpdatesEnabled,
		ProjectUpdateInterval: time.Duration(s.Report.ProjectUpdateIntervalHours) * time.Hour,
		WeeklyEnabled:         s.Report.WeeklyEnabled,
		WeeklyDay:             report.ParseWeekday(s.Report.WeeklyDay),
		WeeklyHour:            s.Report.WeeklyHour,
		RoundupDocumentID:     s.Report.RoundupDocumentID,
		RoundupProjectID:      s.Report.RoundupProjectID,
		DefaultHealth:         s.Report.DefaultHealth,
	}
}

// Store persists the single settings row.
type Store struct{ pool *pgxpool.Pool }

// NewStore returns a Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Load returns the stored settings, or nil when none were saved yet.
func (st *Store) Load(ctx context.Context) (*Settings, error) {
	var raw []byte
	err := st.pool.QueryRow(ctx, `SELECT data FROM operator_settings WHERE id = 1`).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save upserts the settings row.
func (st *Store) Save(ctx context.Context, s Settings) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = st.pool.Exec(ctx, `INSERT INTO operator_settings (id, data) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`, raw)
	return err
}

// Service owns the current settings and notifies listeners on change.
type Service struct {
	store     *Store
	mu        sync.RWMutex
	current   Settings
	listeners []func(Settings)
	saved     bool
}

// Load builds a Service: stored settings when present, else the config defaults.
func Load(ctx context.Context, store *Store, defaults Settings) (*Service, error) {
	s := &Service{store: store, current: defaults}
	if store != nil {
		stored, err := store.Load(ctx)
		if err != nil {
			return nil, err
		}
		if stored != nil {
			s.current = *stored
			s.saved = true
			// A key set in the environment still applies when the saved row has none.
			if s.current.Linear.APIKey == "" && defaults.Linear.APIKey != "" {
				s.current.Linear.APIKey = defaults.Linear.APIKey
			}
			if s.current.Dispatch.WorkerAPIKey == "" {
				s.current.Dispatch.WorkerAPIKey = defaults.Dispatch.WorkerAPIKey
			}
			if s.current.Dispatch.WorktreeDir == "" {
				s.current.Dispatch.WorktreeDir = defaults.Dispatch.WorktreeDir
			}
			if s.current.Dispatch.MaxWorkers == 0 && s.current.Dispatch.Driver == "" {
				// Row predates dispatch settings: seed from the environment.
				s.current.Dispatch = defaults.Dispatch
			}
		}
	}
	s.current.Normalize()
	return s, nil
}

// Current returns a copy of the effective settings.
func (s *Service) Current() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Saved reports whether settings have ever been saved from the UI.
func (s *Service) Saved() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.saved
}

// OnChange registers a listener called (synchronously) after every successful Update.
func (s *Service) OnChange(fn func(Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// Update replaces the settings (the caller sends the complete struct; the API layer
// handles secret masking), persists them, and notifies listeners.
func (s *Service) Update(ctx context.Context, next Settings) (Settings, error) {
	next.Normalize()
	if err := validate(next); err != nil {
		return Settings{}, err
	}
	if s.store != nil {
		if err := s.store.Save(ctx, next); err != nil {
			return Settings{}, err
		}
	}
	s.mu.Lock()
	s.current = next
	s.saved = true
	listeners := append([]func(Settings){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("settings: listener panicked", "panic", r)
				}
			}()
			fn(next)
		}()
	}
	slog.Info("settings: updated", "linear_enabled", next.Linear.Enabled && next.Linear.APIKey != "", "review_publish", next.Review.Publish,
		"review_watch_requested", next.Review.WatchRequested, "feedback_auto", next.Feedback.AutoAddress)
	return next, nil
}

// UpdatePrompt sets (or clears, when text is empty) the override for one built-in prompt.
func (s *Service) UpdatePrompt(ctx context.Context, id, text string) (Settings, error) {
	next := s.Current()
	m := map[string]string{}
	for k, v := range next.Prompts {
		m[k] = v
	}
	if strings.TrimSpace(text) == "" {
		delete(m, id)
	} else {
		m[id] = text
	}
	next.Prompts = m
	return s.Update(ctx, next)
}

// UpdateLayout replaces only the UI layout, leaving the operational settings untouched.
func (s *Service) UpdateLayout(ctx context.Context, layout LayoutSettings) (LayoutSettings, error) {
	next := s.Current()
	next.Layout = layout
	saved, err := s.Update(ctx, next)
	if err != nil {
		return LayoutSettings{}, err
	}
	return saved.Layout, nil
}

func validate(s Settings) error {
	switch strings.ToLower(s.Review.Harness) {
	case "codex", "claude":
	default:
		return errors.New("review.harness must be codex or claude")
	}
	switch strings.ToLower(s.Feedback.Harness) {
	case "codex", "claude":
	default:
		return errors.New("feedback.harness must be codex or claude")
	}
	switch s.Report.DefaultHealth {
	case report.HealthOnTrack, report.HealthAtRisk, report.HealthOffTrack:
	default:
		return errors.New("report.default_health must be onTrack, atRisk, or offTrack")
	}
	switch strings.ToLower(s.Dispatch.Driver) {
	case "claude", "codex", "generic":
	default:
		return errors.New("dispatch.driver must be claude, codex, or generic")
	}
	if s.Linear.APIKey != "" && !strings.HasPrefix(s.Linear.APIKey, "lin_") {
		return errors.New("linear.api_key does not look like a Linear personal API key (lin_api_…)")
	}
	return nil
}

// KeyHint returns a safe suffix of a secret for display.
func KeyHint(key string) string {
	if len(key) <= 6 {
		return ""
	}
	return "…" + key[len(key)-4:]
}
