package config

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Load reads configuration from environment variables with sensible defaults.
//
// Environment variables should be populated by varlock before the server starts:
//
//	varlock run -- ./warrant          (production / Docker)
//	varlock run -- go run ./cmd/server (local dev)
//
// varlock validates variables against .env.schema and ensures sensitive values
// are never logged. See scripts/varlock and .env.schema for details.
func Load() *Config {
	port := getEnv("PORT", "8080")
	baseURL := getEnv("BASE_URL", "http://localhost:"+port)
	cfg := &Config{
		Mirror: MirrorConfig{
			Enabled:      getEnvBool("MIRROR_ENABLED", false),
			LinearAPIKey: getEnv("MIRROR_LINEAR_API_KEY", ""),
			JiraBaseURL:  getEnv("MIRROR_JIRA_BASE_URL", ""),
			JiraEmail:    getEnv("MIRROR_JIRA_EMAIL", ""),
			JiraAPIToken: getEnv("MIRROR_JIRA_API_TOKEN", ""),
		},
		Dispatch: DispatchConfig{
			Enabled:      getEnvBool("DISPATCH_ENABLED", false),
			MaxWorkers:   getEnvInt("DISPATCH_MAX_WORKERS", 4),
			ClaudePath:   getEnv("DISPATCH_CLAUDE_PATH", "claude"),
			WorktreeDir:  getEnv("DISPATCH_WORKTREE_DIR", "/tmp/flywheel-worktrees"),
			APIKey:       getEnv("DISPATCH_API_KEY", ""),
			ProjectID:    getEnv("DISPATCH_PROJECT_ID", ""),
			AutoApproveOnAcceptancePass: getEnvBool("AUTO_APPROVE_ON_ACCEPTANCE_PASS", false),
			AgentDriver:   getEnv("DISPATCH_AGENT_DRIVER", "claude"),
			AgentCLIPath:  getEnv("DISPATCH_AGENT_CMD", ""),
			DockerEnabled:  getEnvBool("DISPATCH_DOCKER_ENABLED", false),
			DockerImage:    getEnv("DISPATCH_DOCKER_IMAGE", "flywheel-worker"),
			DockerMemory:   getEnv("DISPATCH_DOCKER_MEMORY", "4g"),
			DockerCPUs:     getEnv("DISPATCH_DOCKER_CPUS", "2"),
			DockerFirewall: getEnvBool("DISPATCH_DOCKER_FIREWALL", true),
			AnthropicKey:   getEnv("ANTHROPIC_API_KEY", ""),
		},
		Cost: CostConfig{
			Enabled:                    getEnvBool("COST_TRACKING_ENABLED", true),
			DefaultMonthlyBudgetDollars: getEnvFloat("COST_DEFAULT_MONTHLY_BUDGET", 0),
			DefaultTicketBudgetDollars:  getEnvFloat("COST_DEFAULT_TICKET_BUDGET", 0),
			WarnAtFraction:             getEnvFloat("COST_WARN_AT_FRACTION", 0.8),
			FlagshipProvider:           getEnv("COST_FLAGSHIP_PROVIDER", "anthropic"),
			FlagshipModel:              getEnv("COST_FLAGSHIP_MODEL", "claude-opus-4-20250514"),
			MidProvider:                getEnv("COST_MID_PROVIDER", "anthropic"),
			MidModel:                   getEnv("COST_MID_MODEL", "claude-sonnet-4-20250514"),
			FastProvider:               getEnv("COST_FAST_PROVIDER", "anthropic"),
			FastModel:                  getEnv("COST_FAST_MODEL", "claude-haiku-3-20250307"),
		},
		Server: ServerConfig{
			Port:    port,
			WebDist: getEnv("WEB_DIST", "web/dist"),
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
	}

	for _, w := range cfg.Validate() {
		log.Printf("config warning: %s", w)
	}

	return cfg
}

// Validate checks for conflicting or potentially misconfigured settings
// and returns a list of warning messages. Called automatically by Load().
func (c *Config) Validate() []string {
	var warnings []string

	// When dispatch is enabled in host mode, the claude CLI must be reachable.
	// Skip the check when Docker isolation is active because the binary lives
	// inside the container image, not on the host PATH.
	if c.Dispatch.Enabled && !c.Dispatch.DockerEnabled {
		if _, err := exec.LookPath(c.Dispatch.ClaudePath); err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"DISPATCH_ENABLED=true but DISPATCH_CLAUDE_PATH=%q not found on PATH: %v",
				c.Dispatch.ClaudePath, err))
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

type Config struct {
	Server                    ServerConfig
	DB                        DBConfig
	Redis                     RedisConfig
	Queue                     QueueConfig
	Auth                      AuthConfig
	Dispatch                  DispatchConfig
	Cost                      CostConfig
	Mirror                    MirrorConfig
	RunAcceptanceTestOnSubmit bool
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

type DispatchConfig struct {
	Enabled      bool
	MaxWorkers   int
	ClaudePath   string   // path to claude CLI binary (host mode, backward compat)
	WorktreeDir  string   // base directory for git worktrees (host mode)
	APIKey       string   // flywheel API key for worker MCP authentication
	ProjectID    string   // only dispatch tickets for this project (empty = all)
	AutoApproveOnAcceptancePass bool
	// Agent driver settings.
	AgentDriver  string   // driver name: "claude" (default), "generic", or custom registered driver
	AgentCLIPath string   // override CLI path for the agent binary (DISPATCH_AGENT_CMD)
	// Docker isolation settings.
	DockerEnabled  bool
	DockerImage    string // worker image name (default: "flywheel-worker")
	DockerMemory   string // memory limit per worker (default: "4g")
	DockerCPUs     string // CPU limit per worker (default: "2")
	DockerFirewall bool   // enable default-deny firewall with allowlist
	AnthropicKey   string // ANTHROPIC_API_KEY passed to docker workers
}

type ServerConfig struct {
	Port    string
	WebDist string // directory with Vite build (index.html, assets/). Empty disables SPA routes.
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
