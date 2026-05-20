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

// Load reads configuration from environment with sensible defaults.
// If a .env file exists in the current directory, it is loaded first (values already in env are not overwritten).
//
// Environment variables should be populated by varlock before the server starts:
//
//	varlock run -- ./warrant          (production / Docker)
//	varlock run -- go run ./cmd/server (local dev)
//
// varlock validates variables against .env.schema and ensures sensitive values
// are never logged. See scripts/varlock and .env.schema for details.
func Load() *Config {
	loadEnvFile(".env")
	port := getEnv("PORT", "8080")
	baseURL := getEnv("BASE_URL", "http://localhost:"+port)
	agentRunner := getEnv("DISPATCH_AGENT_RUNNER", "")
	dockerEnabled := getEnvBool("DISPATCH_DOCKER_ENABLED", false)
	if agentRunner == "" {
		if dockerEnabled {
			agentRunner = dispatch.RunnerDocker
		} else {
			agentRunner = dispatch.RunnerCLI
		}
	}
	agentDriver := getEnv("DISPATCH_AGENT_DRIVER", "claude")
	agentAPIKey := resolveDispatchAgentAPIKey(agentRunner, agentDriver)
	agentModel := getEnv("DISPATCH_AGENT_MODEL", "")
	if agentModel == "" && agentRunner == dispatch.RunnerOpenAIResponses {
		agentModel = "gpt-5.2-codex"
	}
	orchestratorRunner := getEnv("ORCHESTRATOR_AGENT_RUNNER", agentRunner)
	orchestratorDriver := getEnv("ORCHESTRATOR_AGENT_DRIVER", agentDriver)
	orchestratorAPIKey := resolveAgentAPIKey("ORCHESTRATOR_AGENT_API_KEY", orchestratorRunner, orchestratorDriver)
	orchestratorModel := getEnv("ORCHESTRATOR_AGENT_MODEL", agentModel)
	if orchestratorModel == "" && orchestratorRunner == dispatch.RunnerOpenAIResponses {
		orchestratorModel = "gpt-5.2-codex"
	}
	orchestratorReasoning := getEnv("ORCHESTRATOR_AGENT_REASONING_EFFORT", "")
	if orchestratorReasoning == "" && orchestratorRunner == dispatch.RunnerOpenAIResponses {
		orchestratorReasoning = "xhigh"
	}
	cfg := &Config{
		Policy: PolicyConfig{
			DefaultPosture:   getEnv("POLICY_DEFAULT_POSTURE", "plan-only"),
			AutoApplyDefault: getEnvBool("POLICY_AUTO_APPLY_DEFAULT", true),
		},
		Mirror: MirrorConfig{
			Enabled:      getEnvBool("MIRROR_ENABLED", false),
			LinearAPIKey: getEnv("MIRROR_LINEAR_API_KEY", ""),
			JiraBaseURL:  getEnv("MIRROR_JIRA_BASE_URL", ""),
			JiraEmail:    getEnv("MIRROR_JIRA_EMAIL", ""),
			JiraAPIToken: getEnv("MIRROR_JIRA_API_TOKEN", ""),
		},
		Notification: NotificationConfig{
			Enabled:         getEnvBool("NOTIFICATION_ENABLED", true),
			SlackWebhookURL: getEnv("NOTIFICATION_SLACK_WEBHOOK_URL", ""),
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
			AgentAPIBaseURL:             getEnv("DISPATCH_AGENT_API_BASE_URL", "https://api.openai.com/v1"),
			DockerEnabled:               dockerEnabled,
			DockerImage:                 getEnv("DISPATCH_DOCKER_IMAGE", "flywheel-worker"),
			DockerMemory:                getEnv("DISPATCH_DOCKER_MEMORY", "4g"),
			DockerCPUs:                  getEnv("DISPATCH_DOCKER_CPUS", "2"),
			DockerFirewall:              getEnvBool("DISPATCH_DOCKER_FIREWALL", true),
			AgentAPIKey:                 agentAPIKey,
			ReconcileInterval:           getEnvDuration("DISPATCH_RECONCILE_INTERVAL", 60*time.Second),
		},
		Orchestrator: OrchestratorConfig{
			Enabled:              getEnvBool("ORCHESTRATOR_ENABLED", true),
			AgentRunner:          orchestratorRunner,
			AgentDriver:          orchestratorDriver,
			AgentCLIPath:         getEnv("ORCHESTRATOR_AGENT_CMD", getEnv("DISPATCH_AGENT_CMD", "")),
			AgentModel:           orchestratorModel,
			AgentReasoningEffort: orchestratorReasoning,
			AgentAPIBaseURL:      getEnv("ORCHESTRATOR_AGENT_API_BASE_URL", getEnv("DISPATCH_AGENT_API_BASE_URL", "https://api.openai.com/v1")),
			AgentAPIKey:          orchestratorAPIKey,
			HistoryLimit:         getEnvInt("ORCHESTRATOR_HISTORY_LIMIT", 200),
		},
		Cost: CostConfig{
			Enabled:                     getEnvBool("COST_TRACKING_ENABLED", true),
			DefaultMonthlyBudgetDollars: getEnvFloat("COST_DEFAULT_MONTHLY_BUDGET", 0),
			DefaultTicketBudgetDollars:  getEnvFloat("COST_DEFAULT_TICKET_BUDGET", 0),
			WarnAtFraction:              getEnvFloat("COST_WARN_AT_FRACTION", 0.8),
			FlagshipProvider:            getEnv("COST_FLAGSHIP_PROVIDER", "anthropic"),
			FlagshipModel:               getEnv("COST_FLAGSHIP_MODEL", "claude-opus-4-20250514"),
			MidProvider:                 getEnv("COST_MID_PROVIDER", "anthropic"),
			MidModel:                    getEnv("COST_MID_MODEL", "claude-sonnet-4-20250514"),
			FastProvider:                getEnv("COST_FAST_PROVIDER", "anthropic"),
			FastModel:                   getEnv("COST_FAST_MODEL", "claude-haiku-3-20250307"),
		},
		Server: ServerConfig{
			Port:           port,
			WebDist:        getEnv("WEB_DIST", "web/dist"),
			WebDevProxyURL: getEnv("WEB_DEV_PROXY_URL", ""),
		},
		DB: DBConfig{
			URL: getEnv("DATABASE_URL", "postgres://flywheel:flywheel@localhost:5433/flywheel?sslmode=disable"),
		},
		Redis: RedisConfig{
			URL: getEnv("REDIS_URL", "redis://localhost:6379/0"),
		},
		Queue: QueueConfig{
			LeaseTTLMinutes: getEnvInt("LEASE_TTL_MINUTES", 10),
		},
		RunAcceptanceTestOnSubmit: getEnvBool("RUN_ACCEPTANCE_TEST_ON_SUBMIT", false),
		Auth: AuthConfig{
			GitHubClientID:     getEnv("GITHUB_CLIENT_ID", ""),
			GitHubClientSecret: getEnv("GITHUB_CLIENT_SECRET", ""),
			BaseURL:            baseURL,
			SuccessRedirectURL: getEnv("AUTH_SUCCESS_REDIRECT_URL", ""),
			JWTSecret:          getEnv("JWT_SECRET", ""),
		},
		Findings: FindingsConfig{
			WeaviateURL:        getEnv("WEAVIATE_URL", ""),
			WeaviateAPIKey:     getEnv("WEAVIATE_API_KEY", ""),
			WeaviateVectorizer: getEnv("WEAVIATE_VECTORIZER", "text2vec-openai"),
		},
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
	runnerName := c.Dispatch.AgentRunner
	if runnerName == "" {
		if c.Dispatch.DockerEnabled {
			runnerName = dispatch.RunnerDocker
		} else {
			runnerName = dispatch.RunnerCLI
		}
	}

	if c.Dispatch.Enabled {
		if err := dispatchValidateRunner(runnerName); err != nil {
			warnings = append(warnings, "DISPATCH_ENABLED=true but "+err.Error())
		}
	}

	if c.Dispatch.Enabled && (runnerName == dispatch.RunnerOpenAIResponses || runnerName == dispatch.RunnerOpenAICompatible) && c.Dispatch.AgentAPIKey == "" {
		warnings = append(warnings,
			fmt.Sprintf("DISPATCH_ENABLED=true and DISPATCH_AGENT_RUNNER=%s but no API key was found; ", runnerName)+
				"set DISPATCH_AGENT_API_KEY or OPENAI_API_KEY")
	}
	if c.Orchestrator.Enabled {
		if err := dispatchValidateRunner(c.Orchestrator.AgentRunner); err != nil {
			warnings = append(warnings, "ORCHESTRATOR_ENABLED=true but "+err.Error())
		}
		if (c.Orchestrator.AgentRunner == dispatch.RunnerOpenAIResponses || c.Orchestrator.AgentRunner == dispatch.RunnerOpenAICompatible) && c.Orchestrator.AgentAPIKey == "" {
			warnings = append(warnings,
				fmt.Sprintf("ORCHESTRATOR_ENABLED=true and ORCHESTRATOR_AGENT_RUNNER=%s but no API key was found; ", c.Orchestrator.AgentRunner)+
					"set ORCHESTRATOR_AGENT_API_KEY or OPENAI_API_KEY")
		}
		if c.Orchestrator.AgentRunner == dispatch.RunnerCLI {
			driverName := c.Orchestrator.AgentDriver
			if driverName == "" {
				driverName = "claude"
			}
			cliPath := c.Orchestrator.AgentCLIPath
			sourceVar := "ORCHESTRATOR_AGENT_CMD"
			if cliPath == "" && driverName == "claude" {
				cliPath = c.Dispatch.ClaudePath
				sourceVar = "DISPATCH_CLAUDE_PATH"
			}
			driver, err := dispatch.LookupDriver(driverName, dispatch.DriverConfig{CLIPath: cliPath})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("ORCHESTRATOR_ENABLED=true but ORCHESTRATOR_AGENT_DRIVER=%q is invalid: %v", driverName, err))
			} else if _, err := exec.LookPath(driver.Executable()); err != nil {
				warnings = append(warnings, fmt.Sprintf(
					"ORCHESTRATOR_ENABLED=true but %s=%q not found on PATH for driver %q: %v",
					sourceVar, driver.Executable(), driver.Name(), err))
			}
		}
	}

	// When dispatch is enabled in host mode, the selected agent CLI must be
	// reachable. Skip the check when Docker isolation is active because the
	// binary lives inside the container image, not on the host PATH.
	if c.Dispatch.Enabled && runnerName == dispatch.RunnerCLI {
		driverName := c.Dispatch.AgentDriver
		if driverName == "" {
			driverName = "claude"
		}
		cliPath := c.Dispatch.AgentCLIPath
		sourceVar := "DISPATCH_AGENT_CMD"
		if cliPath == "" && driverName == "claude" {
			cliPath = c.Dispatch.ClaudePath
			sourceVar = "DISPATCH_CLAUDE_PATH"
		}
		driver, err := dispatch.LookupDriver(driverName, dispatch.DriverConfig{CLIPath: cliPath})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("DISPATCH_ENABLED=true but DISPATCH_AGENT_DRIVER=%q is invalid: %v", driverName, err))
		} else if _, err := exec.LookPath(driver.Executable()); err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"DISPATCH_ENABLED=true but %s=%q not found on PATH for driver %q: %v",
				sourceVar, driver.Executable(), driver.Name(), err))
		}
	}

	if c.Dispatch.Enabled && runnerName == dispatch.RunnerDocker && !c.Dispatch.DockerEnabled {
		warnings = append(warnings,
			"DISPATCH_AGENT_RUNNER=docker but DISPATCH_DOCKER_ENABLED=false; runner selection now controls docker usage")
	}
	if c.Dispatch.Enabled && c.Dispatch.DockerEnabled && runnerName != dispatch.RunnerDocker {
		warnings = append(warnings,
			"DISPATCH_DOCKER_ENABLED=true but DISPATCH_AGENT_RUNNER is not docker; DISPATCH_AGENT_RUNNER now takes precedence")
	}

	// Auto-approving on acceptance pass requires acceptance tests to actually run.
	if c.Dispatch.AutoApproveOnAcceptancePass && !c.RunAcceptanceTestOnSubmit {
		warnings = append(warnings,
			"AUTO_APPROVE_ON_ACCEPTANCE_PASS=true but RUN_ACCEPTANCE_TEST_ON_SUBMIT=false; "+
				"tickets cannot be auto-approved because no acceptance test will run")
	}

	return warnings
}

type Config struct {
	Server                    ServerConfig
	DB                        DBConfig
	Redis                     RedisConfig
	Queue                     QueueConfig
	Auth                      AuthConfig
	Dispatch                  DispatchConfig
	Orchestrator              OrchestratorConfig
	Policy                    PolicyConfig
	Cost                      CostConfig
	Mirror                    MirrorConfig
	Notification              NotificationConfig
	Findings                  FindingsConfig
	RunAcceptanceTestOnSubmit bool
}

// PolicyConfig controls the policy layer behavior.
type PolicyConfig struct {
	// DefaultPosture is the posture applied to new projects on first run.
	// Valid values: plan-only, sandbox, prod-gate, graduated-risk, paranoid-service.
	// Default: plan-only (most conservative).
	DefaultPosture string
	// AutoApplyDefault automatically applies the default posture to projects without an active policy.
	// Default: true.
	AutoApplyDefault bool
}

// FindingsConfig holds configuration for the findings layer (Layer 4).
type FindingsConfig struct {
	WeaviateURL        string // Weaviate server URL. Empty = use in-memory fallback.
	WeaviateAPIKey     string // Weaviate API key for authentication (optional).
	WeaviateVectorizer string // Vectorizer module name (default: "text2vec-openai").
}

// CostConfig holds cost and rate-limit management settings.
type CostConfig struct {
	Enabled                     bool    // enable cost tracking (default: true)
	DefaultMonthlyBudgetDollars float64 // 0 = no default budget
	DefaultTicketBudgetDollars  float64 // 0 = no default budget
	WarnAtFraction              float64 // fraction (0-1) at which to warn (default: 0.8)
	FlagshipProvider            string  // provider for flagship tier (e.g. "anthropic")
	FlagshipModel               string  // model name for flagship tier
	MidProvider                 string  // provider for mid tier
	MidModel                    string  // model name for mid tier
	FastProvider                string  // provider for fast/cheap tier
	FastModel                   string  // model name for fast tier
}

// MirrorConfig holds configuration for the ticket mirroring service.
// The mirror service itself is opt-in per project (via project ContextPack.Extra),
// but global API credentials are configured here.
type MirrorConfig struct {
	Enabled      bool   // master switch: enable the mirror service
	LinearAPIKey string // Linear API key (global, or per-project via varlock)
	JiraBaseURL  string // Jira instance base URL
	JiraEmail    string // Jira API user email
	JiraAPIToken string // Jira API token
}

// NotificationConfig holds configuration for the notification push layer.
// Channel adapters (Slack, email, SMS) are pluggable — defaults are registered
// when their credentials are configured. Per-project settings are stored in the
// notification_preferences table.
type NotificationConfig struct {
	Enabled         bool   // master switch: enable the notification service
	SlackWebhookURL string // default Slack incoming webhook URL (per-project overrides via preferences)
}

type DispatchConfig struct {
	Enabled                     bool
	MaxWorkers                  int
	ClaudePath                  string // path to claude CLI binary (host mode, backward compat)
	WorktreeDir                 string // base directory for git worktrees (host mode)
	APIKey                      string // Flywheel API key for worker MCP authentication
	ProjectID                   string // only dispatch tickets for this project (empty = all)
	AutoApproveOnAcceptancePass bool
	AgentRunner                 string // execution backend: cli, docker, openai-responses, or openai-compatible
	// Agent driver settings.
	AgentDriver          string // driver name: "claude" (default), "generic", or custom registered driver
	AgentCLIPath         string // override CLI path for the agent binary (DISPATCH_AGENT_CMD)
	AgentModel           string // API-native model name (used by API runners)
	AgentReasoningEffort string // API-native reasoning effort
	AgentAPIBaseURL      string // base URL for API-native runners
	// Docker isolation settings.
	DockerEnabled     bool
	DockerImage       string        // worker image name (default: "flywheel-worker")
	DockerMemory      string        // memory limit per worker (default: "4g")
	DockerCPUs        string        // CPU limit per worker (default: "2")
	DockerFirewall    bool          // enable default-deny firewall with allowlist
	AgentAPIKey       string        // provider credential from DISPATCH_AGENT_API_KEY or provider-specific fallbacks
	ReconcileInterval time.Duration // periodic reconciliation interval (default: 60s)
}

type OrchestratorConfig struct {
	Enabled              bool
	AgentRunner          string
	AgentDriver          string
	AgentCLIPath         string
	AgentModel           string
	AgentReasoningEffort string
	AgentAPIBaseURL      string
	AgentAPIKey          string
	HistoryLimit         int
}

type ServerConfig struct {
	Port           string
	WebDist        string // directory with Vite build (index.html, assets/). Empty disables SPA routes.
	WebDevProxyURL string // optional Vite dev server URL to reverse proxy for HMR in local development.
}

type AuthConfig struct {
	GitHubClientID     string
	GitHubClientSecret string
	BaseURL            string
	SuccessRedirectURL string
	JWTSecret          string
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

func getEnvFloat(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
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

func resolveDispatchAgentAPIKey(agentRunner, agentDriver string) string {
	return resolveAgentAPIKey("DISPATCH_AGENT_API_KEY", agentRunner, agentDriver)
}

func resolveAgentAPIKey(explicitEnv, agentRunner, agentDriver string) string {
	if explicitEnv != "" {
		if v := getEnv(explicitEnv, ""); v != "" {
			return v
		}
	}
	if agentRunner == dispatch.RunnerOpenAIResponses || agentRunner == dispatch.RunnerOpenAICompatible {
		if v := getEnv("OPENAI_API_KEY", ""); v != "" {
			return v
		}
		return ""
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
