package bootstrap

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
)

// WizardResult is the output of the first-run wizard.
type WizardResult struct {
	RepoPath           string
	DispatchCredential string
	AutonomyMode       string // "sandbox", "supervised", "autonomous"
	ProjectMap         *ProjectMap
	OrgID              string
	ProjectID          string
	AgentID            string
	AgentAPIKey        string
	JWTSecret          string
	DataDir            string
}

// OrgCreator can create orgs and add members (org.Service).
type OrgCreator interface {
	CreateOrg(ctx context.Context, name, slug string) (*org.Org, error)
	AddMember(ctx context.Context, orgID, userID string, role org.Role) error
}

// ProjectCreator can create projects (project.Service).
type ProjectCreator interface {
	CreateProject(ctx context.Context, orgID, name, slug, repoURL string, techStack []string) (*project.Project, error)
	UpdateContextPack(ctx context.Context, projectID string, pack project.ContextPack) error
}

// AgentCreator can register agents (agent.Service).
type AgentCreator interface {
	RegisterAgent(ctx context.Context, name string, typ agent.Type) (*agent.Agent, string, error)
}

// RunWizard runs the interactive first-run setup wizard.
// It prompts for repo path, API key, and autonomy posture,
// then performs a bootstrap scan and creates the default org, project, and agent.
func RunWizard(ctx context.Context, orgSvc OrgCreator, projectSvc ProjectCreator, agentSvc AgentCreator) (*WizardResult, error) {
	reader := bufio.NewReader(os.Stdin)
	result := &WizardResult{}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║           Warrant — First-Run Setup                 ║")
	fmt.Println("║    Adderall for coding agents. Let's get started.   ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	// 1. Repo path
	cwd, _ := os.Getwd()
	fmt.Printf("Repository path [%s]: ", cwd)
	repoPath := readLine(reader)
	if repoPath == "" {
		repoPath = cwd
	}
	repoPath, _ = filepath.Abs(repoPath)
	result.RepoPath = repoPath

	// 2. Optional provider credential for dispatched agents.
	fmt.Print("Dispatch agent API key (optional) [skip]: ")
	apiKey := readLine(reader)
	result.DispatchCredential = apiKey

	// 3. Autonomy posture
	fmt.Println()
	fmt.Println("Autonomy posture — how much freedom do agents get?")
	fmt.Println("  1) sandbox    — agents run in containers, default-deny network (recommended)")
	fmt.Println("  2) supervised — agents run on host, require approval for risky actions")
	fmt.Println("  3) autonomous — agents run on host, auto-approve when tests pass")
	fmt.Print("Choose [1]: ")
	choice := readLine(reader)
	switch choice {
	case "2":
		result.AutonomyMode = "supervised"
	case "3":
		result.AutonomyMode = "autonomous"
	default:
		result.AutonomyMode = "sandbox"
	}

	// 4. Bootstrap scan
	fmt.Println()
	fmt.Printf("Scanning %s ...\n", repoPath)
	pm, err := ScanRepo(repoPath)
	if err != nil {
		return nil, fmt.Errorf("scan repo: %w", err)
	}
	result.ProjectMap = pm

	fmt.Println()
	fmt.Println("── Draft Project Map ──────────────────────────────────")
	printProjectMap(pm)
	fmt.Println("───────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Print("Accept this project map? [Y/n]: ")
	accept := readLine(reader)
	if accept != "" && strings.ToLower(accept)[0] != 'y' {
		fmt.Println("You can edit the project configuration later via the UI or API.")
	}

	// 5. Generate secrets
	result.JWTSecret = generateSecret(32)
	result.DataDir = defaultDataDir()

	// 6. Create org, project, agent
	fmt.Println()
	fmt.Println("Setting up default organization and project...")

	// Derive project name from repo directory.
	projectName := filepath.Base(repoPath)
	orgName := "default"

	o, err := orgSvc.CreateOrg(ctx, orgName, "default")
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}
	result.OrgID = o.ID

	// Create a bootstrap user for the org membership.
	bootstrapUserID := uuid.Must(uuid.NewV7()).String()
	if err := orgSvc.AddMember(ctx, o.ID, bootstrapUserID, org.RoleOwner); err != nil {
		return nil, fmt.Errorf("add org member: %w", err)
	}

	p, err := projectSvc.CreateProject(ctx, o.ID, projectName, "", repoPath, pm.TechStack)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	result.ProjectID = p.ID

	// Set context pack with key files from scan.
	if len(pm.KeyFiles) > 0 {
		var keyFileRefs []project.FileRef
		for _, kf := range pm.KeyFiles {
			keyFileRefs = append(keyFileRefs, project.FileRef{Path: kf})
		}
		pack := project.ContextPack{
			Conventions: fmt.Sprintf("Auto-detected: %s", pm.Summary),
			KeyFiles:    keyFileRefs,
		}
		_ = projectSvc.UpdateContextPack(ctx, p.ID, pack)
	}

	a, apiKey, err := agentSvc.RegisterAgent(ctx, "bootstrap-agent", agent.TypeClaude)
	if err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}
	result.AgentID = a.ID
	result.AgentAPIKey = apiKey

	// 7. Write config file
	configPath := filepath.Join(result.DataDir, "config.env")
	if err := writeConfigFile(configPath, result); err != nil {
		fmt.Printf("Warning: could not write config file: %v\n", err)
	}

	// 8. Summary
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║                 Setup Complete!                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Organization:  %s (ID: %s)\n", orgName, o.ID)
	fmt.Printf("  Project:       %s (ID: %s)\n", projectName, p.ID)
	fmt.Printf("  Agent API Key: %s\n", apiKey)
	fmt.Printf("  Data dir:      %s\n", result.DataDir)
	fmt.Printf("  Autonomy:      %s\n", result.AutonomyMode)
	fmt.Println()
	fmt.Println("  Server is starting on :8090 ...")
	fmt.Println("  Connect Claude Code:  claude --mcp-server warrant=http://localhost:8090/mcp")
	fmt.Println("  Connect Cursor:       copy .flywheel-mcp-config.json to your IDE MCP config")
	fmt.Printf("  Set API key header:   Authorization: Bearer %s\n", apiKey)
	fmt.Println()

	return result, nil
}

// IsFirstRun returns true if the data directory does not exist or has no database.
func IsFirstRun(dataDir string) bool {
	if dataDir == "" {
		dataDir = defaultDataDir()
	}
	dbPath := filepath.Join(dataDir, "warrant.db")
	_, err := os.Stat(dbPath)
	return os.IsNotExist(err)
}

func defaultDataDir() string {
	if d := os.Getenv("WARRANT_DATA_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".warrant", "data")
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func generateSecret(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func printProjectMap(pm *ProjectMap) {
	if len(pm.Languages) > 0 {
		fmt.Printf("  Languages:   %s\n", strings.Join(pm.Languages, ", "))
	}
	if len(pm.Frameworks) > 0 {
		fmt.Printf("  Frameworks:  %s\n", strings.Join(pm.Frameworks, ", "))
	}
	if pm.HasDocker {
		fmt.Println("  Docker:      yes")
	}
	if pm.HasK8s {
		fmt.Println("  Kubernetes:  yes")
	}
	if pm.HasTerraform {
		fmt.Println("  Terraform:   yes")
	}
	if pm.HasCI {
		fmt.Printf("  CI/CD:       %s\n", pm.CISystem)
	}
	if pm.HasEnvFile {
		fmt.Println("  .env file:   found (will offer migration)")
	}
	if len(pm.KeyFiles) > 0 {
		fmt.Printf("  Key files:   %s\n", strings.Join(pm.KeyFiles, ", "))
	}
	fmt.Printf("  Summary:     %s\n", pm.Summary)
}

func writeConfigFile(path string, result *WizardResult) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var lines []string
	lines = append(lines, "# Warrant embedded mode configuration")
	lines = append(lines, fmt.Sprintf("# Generated %s", time.Now().UTC().Format(time.RFC3339)))
	lines = append(lines, "STORAGE_MODE=embedded")
	lines = append(lines, fmt.Sprintf("WARRANT_DATA_DIR=%s", result.DataDir))
	lines = append(lines, fmt.Sprintf("JWT_SECRET=%s", result.JWTSecret))
	lines = append(lines, fmt.Sprintf("DISPATCH_PROJECT_ID=%s", result.ProjectID))
	lines = append(lines, fmt.Sprintf("DISPATCH_API_KEY=%s", result.AgentAPIKey))
	if result.DispatchCredential != "" {
		lines = append(lines, fmt.Sprintf("DISPATCH_AGENT_API_KEY=%s", result.DispatchCredential))
	}
	lines = append(lines, fmt.Sprintf("AUTONOMY_MODE=%s", result.AutonomyMode))
	lines = append(lines, "")

	// Also write project map as JSON.
	pmPath := filepath.Join(dir, "project-map.json")
	pmJSON, _ := json.MarshalIndent(result.ProjectMap, "", "  ")
	_ = os.WriteFile(pmPath, pmJSON, 0o644)

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}
