package sessions

import (
	"context"
	"errors"
	"github.com/gabinante/flywheel/internal/harness"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config controls the ingestion loop.
type Config struct {
	Enabled   bool
	ClaudeDir string        // ~/.claude/projects
	CodexDir  string        // ~/.codex
	Interval  time.Duration // poll interval (default 10s)
}

// Service owns the collectors and exposes session queries.
type Service struct {
	store  *Store
	cfg    Config
	runner Continuer
	claude *claudeCollector
	codex  *codexCollector

	mu      sync.Mutex
	status  CollectorStatus
	running int32
}

// NewService builds a Service. Collectors are created even when disabled so RunOnce can be called on demand.
func NewService(store *Store, cfg Config) *Service {
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Second
	}
	repos := newRepoResolver()
	return &Service{
		store:  store,
		cfg:    cfg,
		claude: newClaudeCollector(cfg.ClaudeDir, store, repos),
		codex:  newCodexCollector(cfg.CodexDir, store, repos),
		status: CollectorStatus{Enabled: cfg.Enabled, ClaudeDir: cfg.ClaudeDir, CodexDir: cfg.CodexDir, ByHarness: map[string]int{}},
	}
}

// Start runs the ingestion loop until ctx is cancelled. No-op when disabled.
func (s *Service) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		slog.Info("sessions: collector disabled")
		return
	}
	go func() {
		if err := s.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("sessions: initial ingest failed", "error", err)
		}
		t := time.NewTicker(s.cfg.Interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Warn("sessions: ingest failed", "error", err)
				}
			}
		}
	}()
	slog.Info("sessions: collector started", "claude_dir", s.cfg.ClaudeDir, "codex_dir", s.cfg.CodexDir, "interval", s.cfg.Interval)
}

// RunOnce ingests new data from both harnesses. Concurrent calls are coalesced.
func (s *Service) RunOnce(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil
	}
	defer atomic.StoreInt32(&s.running, 0)
	start := time.Now()
	var errs []error
	if n, err := s.claude.run(ctx); err != nil {
		errs = append(errs, err)
	} else if n > 0 {
		slog.Debug("sessions: claude ingested", "sessions", n)
	}
	if n, err := s.codex.run(ctx); err != nil {
		errs = append(errs, err)
	} else if n > 0 {
		slog.Debug("sessions: codex ingested", "sessions", n)
	}
	if _, err := s.store.ResolveParents(ctx); err != nil {
		errs = append(errs, err)
	}
	err := errors.Join(errs...)
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
	return err
}

// Status returns collector health plus current counts.
func (s *Service) Status(ctx context.Context) CollectorStatus {
	s.mu.Lock()
	st := s.status
	s.mu.Unlock()
	by, total, err := s.store.Counts(ctx)
	if err == nil {
		st.ByHarness = by
		st.SessionsTotal = total
	}
	return st
}

// List returns sessions matching f and the total count.
func (s *Service) List(ctx context.Context, f Filter) ([]*Session, int, error) {
	return s.store.List(ctx, f)
}

// Get returns one session with links, or nil.
func (s *Service) Get(ctx context.Context, id string) (*Session, error) { return s.store.Get(ctx, id) }

// GetByExternal returns a session by harness-native id, or nil.
func (s *Service) GetByExternal(ctx context.Context, h Harness, ext string) (*Session, error) {
	return s.store.GetByExternal(ctx, h, ext)
}

// Prompts returns a session's prompts.
func (s *Service) Prompts(ctx context.Context, id string, limit int) ([]Prompt, error) {
	return s.store.ListPrompts(ctx, id, limit)
}

// Children returns subagent sessions of a parent.
func (s *Service) Children(ctx context.Context, id string) ([]*Session, error) {
	return s.store.ListChildren(ctx, id)
}

// ListByLink returns sessions linked to kind/ref.
func (s *Service) ListByLink(ctx context.Context, kind, ref string) ([]*Session, error) {
	return s.store.ListByLink(ctx, kind, ref)
}

// Link records an explicit link on a session.
func (s *Service) Link(ctx context.Context, sessionID, kind, ref, source string) error {
	if source == "" {
		source = LinkSourceExplicit
	}
	return s.store.AddLinks(ctx, sessionID, []Link{{Kind: kind, Ref: ref, Source: source}})
}

// Record upserts a session that Flywheel itself started (dispatched workers).
func (s *Service) Record(ctx context.Context, sess *Session) error {
	return s.store.Upsert(ctx, sess)
}

// StatsSince aggregates sessions by harness for a window, optionally restricted to repos.
func (s *Service) StatsSince(ctx context.Context, since, until time.Time, repos []string) ([]HarnessStats, error) {
	return s.store.StatsSince(ctx, since, until, repos)
}

// Interval reports how often the collectors run.
func (s *Service) Interval() time.Duration { return s.cfg.Interval }

// Enabled reports whether ingestion is on.
func (s *Service) Enabled() bool { return s.cfg.Enabled }

// Continuer runs a harness turn (set by main to the shared harness runner).
type Continuer interface {
	Run(ctx context.Context, spec harness.Spec) (*harness.Result, error)
}

// SetRunner installs the harness runner used to continue sessions.
func (s *Service) SetRunner(r Continuer) { s.runner = r }

// Continue resumes a tracked session with a new message in its working directory and
// returns the agent's reply. The harness appends to its own session store, so the next
// ingestion pass picks up the new turns.
func (s *Service) Continue(ctx context.Context, id, message string) (string, error) {
	if s.runner == nil {
		return "", errors.New("session continuation is not configured")
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("message is required")
	}
	sess, err := s.store.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if sess == nil {
		return "", errors.New("session not found")
	}
	kind := harness.Codex
	if sess.Harness == HarnessClaudeCode {
		kind = harness.ClaudeCode
	}
	workDir := sess.CWD
	if st, err := os.Stat(workDir); err != nil || !st.IsDir() {
		workDir, _ = os.UserHomeDir()
	}
	res, err := s.runner.Run(ctx, harness.Spec{
		Harness: kind, WorkDir: workDir, Prompt: message, Sandbox: harness.SandboxFull, Timeout: 30 * time.Minute, Resume: sess.ExternalID,
	})
	if err != nil && (res == nil || strings.TrimSpace(res.Output) == "") {
		return "", err
	}
	go func() { _ = s.RunOnce(context.WithoutCancel(ctx)) }() // pick up the new turns
	return strings.TrimSpace(res.Output), nil
}
