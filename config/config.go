package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Load reads configuration from environment with sensible defaults.
// If a .env file exists in the current directory, it is loaded first (values already in env are not overwritten).
func Load() *Config {
	loadEnvFile(".env")
	port := getEnv("PORT", "8080")
	baseURL := getEnv("BASE_URL", "http://localhost:"+port)
	return &Config{
		Dispatch: DispatchConfig{
			Enabled:      getEnvBool("DISPATCH_ENABLED", false),
			MaxWorkers:   getEnvInt("DISPATCH_MAX_WORKERS", 4),
			ClaudePath:   getEnv("DISPATCH_CLAUDE_PATH", "claude"),
			WorktreeDir:  getEnv("DISPATCH_WORKTREE_DIR", "/tmp/warrant-worktrees"),
			APIKey:       getEnv("DISPATCH_API_KEY", ""),
			ProjectID:    getEnv("DISPATCH_PROJECT_ID", ""),
			AutoApproveOnAcceptancePass: getEnvBool("AUTO_APPROVE_ON_ACCEPTANCE_PASS", false),
			DockerEnabled:  getEnvBool("DISPATCH_DOCKER_ENABLED", false),
			DockerImage:    getEnv("DISPATCH_DOCKER_IMAGE", "warrant-worker"),
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
			URL: getEnv("DATABASE_URL", "postgres://warrant:warrant@localhost:5433/warrant?sslmode=disable"),
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
}

type Config struct {
	Server                    ServerConfig
	DB                        DBConfig
	Redis                     RedisConfig
	Queue                     QueueConfig
	Auth                      AuthConfig
	Dispatch                  DispatchConfig
	Cost                      CostConfig
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

type DispatchConfig struct {
	Enabled      bool
	MaxWorkers   int
	ClaudePath   string   // path to claude CLI binary (host mode)
	WorktreeDir  string   // base directory for git worktrees (host mode)
	APIKey       string   // warrant API key for worker MCP authentication
	ProjectID    string   // only dispatch tickets for this project (empty = all)
	AutoApproveOnAcceptancePass bool
	// Docker isolation settings.
	DockerEnabled  bool
	DockerImage    string // worker image name (default: "warrant-worker")
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
