package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// HeadlessConfig provides non-interactive bootstrap configuration,
// used when stdin is not a TTY (e.g., Docker without -it, CI/CD).
type HeadlessConfig struct {
	RepoPath     string // defaults to cwd
	AnthropicKey string // from ANTHROPIC_API_KEY env
	AutonomyMode string // from AUTONOMY_MODE env, default "sandbox"
	DataDir      string // from WARRANT_DATA_DIR env
}

// HeadlessConfigFromEnv builds a HeadlessConfig from environment variables.
func HeadlessConfigFromEnv() *HeadlessConfig {
	cwd, _ := os.Getwd()
	repoPath := os.Getenv("WARRANT_REPO_PATH")
	if repoPath == "" {
		repoPath = cwd
	}
	autonomy := os.Getenv("AUTONOMY_MODE")
	if autonomy == "" {
		autonomy = "sandbox"
	}
	return &HeadlessConfig{
		RepoPath:     repoPath,
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		AutonomyMode: autonomy,
		DataDir:      os.Getenv("WARRANT_DATA_DIR"),
	}
}

// RunHeadless performs non-interactive bootstrap: scans the repo, creates
// default org/project/agent, and writes config. Used when no TTY is available.
func RunHeadless(ctx context.Context, cfg *HeadlessConfig, orgSvc OrgCreator, projectSvc ProjectCreator, agentSvc AgentCreator) (*WizardResult, error) {
	result := &WizardResult{
		AutonomyMode: cfg.AutonomyMode,
		AnthropicKey: cfg.AnthropicKey,
	}

	// Resolve repo path.
	repoPath := cfg.RepoPath
	if repoPath == "" {
		repoPath, _ = os.Getwd()
	}
	repoPath, _ = filepath.Abs(repoPath)
	result.RepoPath = repoPath

	// Resolve data directory.
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	result.DataDir = dataDir

	// Scan repo.
	log.Printf("bootstrap: scanning %s", repoPath)
	pm, err := ScanRepo(repoPath)
	if err != nil {
		return nil, fmt.Errorf("scan repo: %w", err)
	}
	result.ProjectMap = pm
	log.Printf("bootstrap: detected %s", pm.Summary)

	// Generate secrets.
	result.JWTSecret = generateSecret(32)

	// Create org.
	o, err := orgSvc.CreateOrg(ctx, "default", "default")
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}
	result.OrgID = o.ID

	// Create project.
	projectName := filepath.Base(repoPath)
	p, err := projectSvc.CreateProject(ctx, o.ID, projectName, "", repoPath, pm.TechStack)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	result.ProjectID = p.ID

	// Register agent.
	a, apiKey, err := agentSvc.RegisterAgent(ctx, "bootstrap-agent", "claude")
	if err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}
	result.AgentID = a.ID
	result.AgentAPIKey = apiKey

	// Write config.
	configPath := filepath.Join(result.DataDir, "config.env")
	if err := writeConfigFile(configPath, result); err != nil {
		log.Printf("bootstrap: warning: could not write config file: %v", err)
	}

	log.Printf("bootstrap: setup complete — org=%s project=%s agent-key=%s", o.ID, p.ID, apiKey)
	return result, nil
}

// IsTTY returns true if stdin is connected to a terminal.
func IsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
