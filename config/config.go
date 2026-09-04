package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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
			BaseURL:            baseURL,
			SuccessRedirectURL: getEnv("AUTH_SUCCESS_REDIRECT_URL", ""),
			JWTSecret:          getEnv("JWT_SECRET", ""),
		},
		Dispatch: DispatchConfig{
			Enabled:                     getEnvBool("DISPATCH_ENABLED", false),
			MaxWorkers:                  getEnvInt("DISPATCH_MAX_WORKERS", 4),
			ClaudePath:                  getEnv("DISPATCH_CLAUDE_PATH", "claude"),
			WorktreeDir:                 getEnv("DISPATCH_WORKTREE_DIR", "/tmp/flywheel-worktrees"),
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
	RunAcceptanceTestOnSubmit bool
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

// AuthConfig configures local operator sign-in. There is no external identity
// provider: GET /auth/login issues a JWT for the single local operator.
type AuthConfig struct {
	BaseURL            string
	SuccessRedirectURL string
	JWTSecret          string // empty = generated per process (UI sessions do not survive restarts)
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
