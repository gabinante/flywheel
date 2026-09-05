package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/dispatch"
)

// Load reads configuration from the environment with local-first defaults.
// If a .env file exists in the current directory it is loaded first (values
// already in the environment win). varlock validates the file against
// .env.schema before the server starts: `varlock run -- go run ./cmd/server`.
func Load() *Config {
	loadEnvFile(".env")
	port := getEnv("PORT", "8090")
	baseURL := getEnv("BASE_URL", "http://localhost:"+port)
	agentRunner := getEnv("DISPATCH_AGENT_RUNNER", dispatch.RunnerCLI)
	agentDriver := getEnv("DISPATCH_AGENT_DRIVER", "claude")
	agentModel := getEnv("DISPATCH_AGENT_MODEL", "")
	orchestratorRunner := getEnv("ORCHESTRATOR_AGENT_RUNNER", agentRunner)
	orchestratorDriver := getEnv("ORCHESTRATOR_AGENT_DRIVER", agentDriver)

	cfg := &Config{
		Server: ServerConfig{
			Port:           port,
			WebDist:        getEnv("WEB_DIST", "web/dist"),
			WebDevProxyURL: getEnv("WEB_DEV_PROXY_URL", ""),
		},
		DB: DBConfig{
			URL: getEnv("DATABASE_URL", "postgres://flywheel:flywheel@localhost:5439/flywheel?sslmode=disable"),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", "redis://localhost:6389/0"),
		},
		Queue: QueueConfig{
			LeaseTTLMinutes: getEnvInt("LEASE_TTL_MINUTES", 10),
		},
		Auth: AuthConfig{
			BaseURL:   baseURL,
			JWTSecret: getEnv("JWT_SECRET", ""),
		},
		Dispatch: DispatchConfig{
			Enabled:                     getEnvBool("DISPATCH_ENABLED", false),
			MaxWorkers:                  getEnvInt("DISPATCH_MAX_WORKERS", 4),
			ClaudePath:                  getEnv("DISPATCH_CLAUDE_PATH", "claude"),
			WorktreeDir:                 getEnv("DISPATCH_WORKTREE_DIR", filepath.Join(homeDir(), "git")),
			APIKey:                      getEnv("DISPATCH_API_KEY", ""),
			ProjectID:                   getEnv("DISPATCH_PROJECT_ID", ""),
			AutoApproveOnAcceptancePass: getEnvBool("AUTO_APPROVE_ON_ACCEPTANCE_PASS", false),
			AgentRunner:                 agentRunner,
			AgentDriver:                 agentDriver,
			AgentCLIPath:                getEnv("DISPATCH_AGENT_CMD", ""),
			AgentModel:                  agentModel,
			AgentReasoningEffort:        getEnv("DISPATCH_AGENT_REASONING_EFFORT", ""),
			AgentAPIKey:                 resolveAgentAPIKey("DISPATCH_AGENT_API_KEY", agentDriver),
			ReconcileInterval:           getEnvDuration("DISPATCH_RECONCILE_INTERVAL", 60*time.Second),
		},
		Orchestrator: OrchestratorConfig{
			Enabled:              getEnvBool("ORCHESTRATOR_ENABLED", true),
			AgentRunner:          orchestratorRunner,
			AgentDriver:          orchestratorDriver,
			AgentCLIPath:         getEnv("ORCHESTRATOR_AGENT_CMD", getEnv("DISPATCH_AGENT_CMD", "")),
			AgentModel:           getEnv("ORCHESTRATOR_AGENT_MODEL", agentModel),
			AgentReasoningEffort: getEnv("ORCHESTRATOR_AGENT_REASONING_EFFORT", ""),
			AgentAPIKey:          resolveAgentAPIKey("ORCHESTRATOR_AGENT_API_KEY", orchestratorDriver),
			HistoryLimit:         getEnvInt("ORCHESTRATOR_HISTORY_LIMIT", 200),
		},
		Review: ReviewConfig{
			Enabled:        getEnvBool("REVIEW_ENABLED", true),
			Harness:        getEnv("REVIEW_HARNESS", "codex"),
			Model:          getEnv("REVIEW_MODEL", ""),
			Effort:         getEnv("REVIEW_REASONING_EFFORT", ""),
			Publish:        getEnvBool("REVIEW_PUBLISH", false),
			PollInterval:   getEnvDuration("REVIEW_POLL_INTERVAL", 2*time.Minute),
			MaxConcurrent:  getEnvInt("REVIEW_MAX_CONCURRENT", 2),
			RepoRoot:       getEnv("REVIEW_REPO_ROOT", filepath.Join(homeDir(), "git")),
			WatchRequested: getEnvBool("REVIEW_WATCH_REQUESTED", true),
			WatchAuthored:  getEnvBool("REVIEW_WATCH_AUTHORED", true),
			Timeout:        getEnvDuration("REVIEW_TIMEOUT", 30*time.Minute),
			SkipDrafts:     getEnvBool("REVIEW_SKIP_DRAFTS", true),
		},
		Feedback: FeedbackConfig{
			Harness:     getEnv("FEEDBACK_HARNESS", "claude"),
			Model:       getEnv("FEEDBACK_MODEL", ""),
			Effort:      getEnv("FEEDBACK_REASONING_EFFORT", ""),
			AutoAddress: getEnvBool("FEEDBACK_AUTO_ADDRESS", false),
			Timeout:     getEnvDuration("FEEDBACK_TIMEOUT", 45*time.Minute),
		},
		Report: ReportConfig{
			ProjectUpdatesEnabled: getEnvBool("REPORT_PROJECT_UPDATES_ENABLED", false),
			ProjectUpdateInterval: getEnvDuration("REPORT_PROJECT_UPDATE_INTERVAL", 48*time.Hour),
			WeeklyEnabled:         getEnvBool("REPORT_WEEKLY_ENABLED", false),
			WeeklyDay:             getEnv("REPORT_WEEKLY_DAY", "Friday"),
			WeeklyHour:            getEnvInt("REPORT_WEEKLY_HOUR", 16),
			RoundupDocumentID:     getEnv("REPORT_ROUNDUP_DOCUMENT_ID", ""),
			RoundupProjectID:      getEnv("REPORT_ROUNDUP_PROJECT_ID", ""),
			DefaultHealth:         getEnv("REPORT_HEALTH_DEFAULT", "onTrack"),
		},
		Linear: LinearConfig{
			APIKey:         getEnv("LINEAR_API_KEY", ""),
			Enabled:        getEnvBool("LINEAR_SYNC_ENABLED", getEnv("LINEAR_API_KEY", "") != ""),
			ProjectIDs:     splitCSV(getEnv("LINEAR_PROJECT_IDS", "")),
			Interval:       getEnvDuration("LINEAR_SYNC_INTERVAL", 60*time.Second),
			DefaultTeamKey: getEnv("LINEAR_DEFAULT_TEAM_KEY", ""),
		},
		Sessions: SessionsConfig{
			Enabled:   getEnvBool("SESSIONS_ENABLED", true),
			ClaudeDir: getEnv("SESSIONS_CLAUDE_DIR", filepath.Join(homeDir(), ".claude", "projects")),
			CodexDir:  getEnv("SESSIONS_CODEX_DIR", filepath.Join(homeDir(), ".codex")),
			Interval:  getEnvDuration("SESSIONS_POLL_INTERVAL", 10*time.Second),
		},
		RunAcceptanceTestOnSubmit: getEnvBool("RUN_ACCEPTANCE_TEST_ON_SUBMIT", false),
	}

	for _, w := range cfg.Validate() {
		slog.Warn("config warning", "message", w)
	}

	return cfg
}

// Validate checks for conflicting or potentially misconfigured settings
// and returns a list of warning messages. Called automatically by Load().
func (c *Config) Validate() []string {
	var warnings []string

	if c.Dispatch.Enabled {
		if err := dispatchValidateRunner(c.Dispatch.AgentRunner); err != nil {
			warnings = append(warnings, "DISPATCH_ENABLED=true but "+err.Error())
		} else if w := validateCLIDriver("DISPATCH", c.Dispatch.AgentDriver, c.Dispatch.AgentCLIPath, c.Dispatch.ClaudePath); w != "" {
			warnings = append(warnings, w)
		}
	}
	if c.Orchestrator.Enabled {
		if err := dispatchValidateRunner(c.Orchestrator.AgentRunner); err != nil {
			warnings = append(warnings, "ORCHESTRATOR_ENABLED=true but "+err.Error())
		} else if w := validateCLIDriver("ORCHESTRATOR", c.Orchestrator.AgentDriver, c.Orchestrator.AgentCLIPath, c.Dispatch.ClaudePath); w != "" {
			warnings = append(warnings, w)
		}
	}

	// Auto-approving on acceptance pass requires acceptance tests to actually run.
	if c.Dispatch.AutoApproveOnAcceptancePass && !c.RunAcceptanceTestOnSubmit {
		warnings = append(warnings,
			"AUTO_APPROVE_ON_ACCEPTANCE_PASS=true but RUN_ACCEPTANCE_TEST_ON_SUBMIT=false; "+
				"tickets cannot be auto-approved because no acceptance test will run")
	}

	return warnings
}

// validateCLIDriver checks that the selected driver exists and its binary is on PATH.
// prefix is "DISPATCH" or "ORCHESTRATOR" and only affects the warning text.
func validateCLIDriver(prefix, driverName, cliPath, claudePath string) string {
	if driverName == "" {
		driverName = "claude"
	}
	sourceVar := prefix + "_AGENT_CMD"
	if cliPath == "" && driverName == "claude" {
		cliPath = claudePath
		sourceVar = "DISPATCH_CLAUDE_PATH"
	}
	driver, err := dispatch.LookupDriver(driverName, dispatch.DriverConfig{CLIPath: cliPath})
	if err != nil {
		return fmt.Sprintf("%s_ENABLED=true but %s_AGENT_DRIVER=%q is invalid: %v", prefix, prefix, driverName, err)
	}
	if _, err := exec.LookPath(driver.Executable()); err != nil {
		return fmt.Sprintf("%s_ENABLED=true but %s=%q not found on PATH for driver %q: %v",
			prefix, sourceVar, driver.Executable(), driver.Name(), err)
	}
	return ""
}

type Config struct {
	Server                    ServerConfig
	DB                        DBConfig
	Redis                     RedisConfig
	Queue                     QueueConfig
	Auth                      AuthConfig
	Dispatch                  DispatchConfig
	Orchestrator              OrchestratorConfig
	Sessions                  SessionsConfig
	Linear                    LinearConfig
	Review                    ReviewConfig
	Feedback                  FeedbackConfig
	Report                    ReportConfig
	RunAcceptanceTestOnSubmit bool
}

// ReportConfig controls Linear reporting parity with the operator's project-status and
// weekly-roundup skills. Previews are always available; posting is opt-in.
type ReportConfig struct {
	ProjectUpdatesEnabled bool
	ProjectUpdateInterval time.Duration
	WeeklyEnabled         bool
	WeeklyDay             string // weekday name
	WeeklyHour            int    // local hour (0-23)
	RoundupDocumentID     string // rolling Linear document that receives each week's roundup
	RoundupProjectID      string // Flywheel project whose Linear project receives the weekly accomplishment update
	DefaultHealth         string // onTrack | atRisk | offTrack
}

// FeedbackConfig controls the address-feedback workflow: when a review lands on one
// of the operator's PRs, a harness (Claude Code by default) can resolve the comments
// on the PR branch. AutoAddress defaults to false; the operator triggers it from the UI.
type FeedbackConfig struct {
	Harness     string
	Model       string
	Effort      string
	AutoAddress bool
	Timeout     time.Duration
}

// ReviewConfig controls PR-keyed code review. Publish defaults to false so a fresh
// install reviews as a dry run until the operator opts into posting to GitHub.
type ReviewConfig struct {
	Enabled        bool
	Harness        string // codex | claude
	Model          string
	Effort         string
	Publish        bool
	PollInterval   time.Duration
	MaxConcurrent  int
	RepoRoot       string
	WatchRequested bool
	WatchAuthored  bool
	Timeout        time.Duration
	SkipDrafts     bool
}

// LinearConfig makes Linear the ticket store. When APIKey is set, Flywheel
// mirrors the Linear projects the operator leads (or ProjectIDs) into projects
// and tickets, and pushes Flywheel-originated changes back.
type LinearConfig struct {
	APIKey         string
	Enabled        bool
	ProjectIDs     []string      // explicit Linear project IDs; empty = projects the API key's user leads
	Interval       time.Duration // poll interval (default 60s)
	DefaultTeamKey string        // team used when creating issues in a multi-team project
}

// SessionsConfig controls ingestion of Claude Code and Codex sessions from their local stores.
type SessionsConfig struct {
	Enabled   bool
	ClaudeDir string        // Claude Code transcripts (default ~/.claude/projects)
	CodexDir  string        // CODEX_HOME (default ~/.codex)
	Interval  time.Duration // poll interval (default 10s)
}

type DispatchConfig struct {
	Enabled                     bool
	MaxWorkers                  int
	ClaudePath                  string // path to claude CLI binary (DISPATCH_CLAUDE_PATH)
	WorktreeDir                 string // base directory for git worktrees
	APIKey                      string // Flywheel API key for worker MCP authentication
	ProjectID                   string // only dispatch tickets for this project (empty = all)
	AutoApproveOnAcceptancePass bool
	AgentRunner                 string // execution backend; only "cli" is supported
	AgentDriver                 string // driver name: "claude" (default), "codex", "generic"
	AgentCLIPath                string // override CLI path for the agent binary (DISPATCH_AGENT_CMD)
	AgentModel                  string // model passed to the harness when it accepts one
	AgentReasoningEffort        string // reasoning effort passed to the harness when it accepts one
	AgentAPIKey                 string // provider credential from DISPATCH_AGENT_API_KEY or provider fallbacks
	ReconcileInterval           time.Duration
}

type OrchestratorConfig struct {
	Enabled              bool
	AgentRunner          string
	AgentDriver          string
	AgentCLIPath         string
	AgentModel           string
	AgentReasoningEffort string
	AgentAPIKey          string
	HistoryLimit         int
}

type ServerConfig struct {
	Port           string
	WebDist        string // directory with Vite build (index.html, assets/). Empty disables SPA routes.
	WebDevProxyURL string // optional Vite dev server URL to reverse proxy for HMR in local development.
}

// AuthConfig configures callback URLs and signing. The local UI needs no login.
type AuthConfig struct {
	BaseURL   string
	JWTSecret string // empty = generated per process; used for callbacks and legacy MCP credentials
}

type DBConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type QueueConfig struct {
	LeaseTTLMinutes int
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// loadEnvFile sets env vars from a file (KEY=VALUE per line). Only sets vars not already in os.Environ().
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if key == "" {
			continue
		}
		if os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, val)
	}
}

func getEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes":
			return true
		case "0", "false", "no":
			return false
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return defaultVal
}

// resolveAgentAPIKey returns the explicit credential for a harness, falling back
// to ANTHROPIC_API_KEY for the claude driver. Local CLI harnesses normally use
// their own login sessions, so this is usually empty.
func resolveAgentAPIKey(explicitEnv, agentDriver string) string {
	if v := getEnv(explicitEnv, ""); v != "" {
		return v
	}
	if agentDriver == "" || agentDriver == "claude" {
		if v := getEnv("ANTHROPIC_API_KEY", ""); v != "" {
			return v
		}
	}
	return ""
}

func dispatchValidateRunner(name string) error {
	if name == "" {
		name = dispatch.RunnerCLI
	}
	for _, runner := range dispatch.AvailableRunners() {
		if runner == name {
			return nil
		}
	}
	return fmt.Errorf("unknown agent runner %q (available: %s)", name, strings.Join(dispatch.AvailableRunners(), ", "))
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
