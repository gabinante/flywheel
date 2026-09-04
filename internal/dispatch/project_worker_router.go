package dispatch

import (
	"os"
	"strings"
	"sync/atomic"

	"github.com/gabinante/flywheel/internal/project"
)

const DefaultProjectWorkerID = "default"

const (
	WorkerRoleOrchestrator     = "orchestrator"
	WorkerRoleConflictResolver = "conflict_resolver"
)

type RoutedWorker struct {
	ID         string
	Name       string
	Config     Config
	UseDefault bool
}

type ProjectWorkerRouter struct {
	global  project.DispatchConfig // shared library (operator settings); projects overlay it
	base    Config
	counter uint64
}

func NewProjectWorkerRouter(base Config) *ProjectWorkerRouter {
	return &ProjectWorkerRouter{base: base}
}

// NewProjectWorkerRouterWithGlobal returns a router whose candidates come from the
// shared worker library overlaid with each project's own dispatch config.
func NewProjectWorkerRouterWithGlobal(base Config, global project.DispatchConfig) *ProjectWorkerRouter {
	return &ProjectWorkerRouter{base: base, global: global.Normalized()}
}

// MergeDispatchConfig overlays a project's dispatch config on the shared library:
// workers and roles by id (project wins), policies by role key (project wins when set).
func MergeDispatchConfig(global, proj project.DispatchConfig) project.DispatchConfig {
	g, p := global.Normalized(), proj.Normalized()
	out := project.DispatchConfig{MaxActiveWorkers: p.MaxActiveWorkers, GitPolicy: p.GitPolicy}
	seen := map[string]int{}
	for _, w := range g.Workers {
		seen[w.ID] = len(out.Workers)
		out.Workers = append(out.Workers, w)
	}
	for _, w := range p.Workers {
		if i, ok := seen[w.ID]; ok {
			out.Workers[i] = w
		} else {
			seen[w.ID] = len(out.Workers)
			out.Workers = append(out.Workers, w)
		}
	}
	roleSeen := map[string]int{}
	for _, r := range g.Roles {
		roleSeen[r.ID] = len(out.Roles)
		out.Roles = append(out.Roles, r)
	}
	for _, r := range p.Roles {
		if i, ok := roleSeen[r.ID]; ok {
			out.Roles[i] = r
		} else {
			out.Roles = append(out.Roles, r)
		}
	}
	out.Policies = map[string]project.DispatchRolePolicy{}
	for k, v := range g.Policies {
		out.Policies[k] = v
	}
	for k, v := range p.Policies {
		if len(v.WorkerIDs) > 0 {
			out.Policies[k] = v
		}
	}
	return out.Normalized()
}

func (r *ProjectWorkerRouter) Candidates(proj *project.Project, role string) []RoutedWorker {
	cfg := r.global.Normalized()
	if proj != nil {
		cfg = MergeDispatchConfig(r.global, proj.DispatchConfig)
	}

	policies := make(map[string]project.DispatchRolePolicy, len(cfg.Policies))
	for key, policy := range cfg.Policies {
		policies[normalizePolicyRole(key)] = policy
	}

	enabled := make([]project.DispatchWorkerProfile, 0, len(cfg.Workers))
	workersByID := make(map[string]project.DispatchWorkerProfile, len(cfg.Workers))
	for _, worker := range cfg.Workers {
		if !worker.Enabled {
			continue
		}
		enabled = append(enabled, worker)
		workersByID[worker.ID] = worker
	}

	policy := policies[normalizePolicyRole(role)]
	candidates := make([]RoutedWorker, 0, len(enabled)+1)
	if len(policy.WorkerIDs) > 0 {
		for _, id := range policy.WorkerIDs {
			id = strings.TrimSpace(id)
			switch id {
			case "":
				continue
			case DefaultProjectWorkerID:
				candidates = append(candidates, r.defaultWorker())
			default:
				if worker, ok := workersByID[id]; ok {
					candidates = append(candidates, r.profileWorker(worker))
				}
			}
		}
	} else {
		for _, worker := range enabled {
			candidates = append(candidates, r.profileWorker(worker))
		}
	}

	if len(candidates) == 0 {
		candidates = append(candidates, r.defaultWorker())
	}
	if policy.SelectionMode == "any" && len(candidates) > 1 {
		start := int(atomic.AddUint64(&r.counter, 1)-1) % len(candidates)
		rotated := append([]RoutedWorker{}, candidates[start:]...)
		rotated = append(rotated, candidates[:start]...)
		return rotated
	}
	return candidates
}

func (r *ProjectWorkerRouter) defaultWorker() RoutedWorker {
	cfg := cloneDispatchConfig(r.base)
	return RoutedWorker{
		ID:         DefaultProjectWorkerID,
		Name:       "Default server worker",
		Config:     cfg,
		UseDefault: true,
	}
}

func (r *ProjectWorkerRouter) profileWorker(profile project.DispatchWorkerProfile) RoutedWorker {
	cfg := cloneDispatchConfig(r.base)
	if profile.Runner != "" {
		cfg.AgentRunner = profile.Runner
	}
	if profile.Driver != "" && profile.Driver != cfg.AgentDriver {
		// Switching harness: the base driver's model, effort and executable do not carry over.
		cfg.AgentDriver = profile.Driver
		cfg.AgentCLIPath = ""
		cfg.AgentModel, cfg.AgentReasoningEffort = "", ""
		if d, ok := cfg.DriverDefaults[profile.Driver]; ok {
			cfg.AgentModel, cfg.AgentReasoningEffort = d.Model, d.Effort
		}
	}
	if profile.CLIPath != "" {
		cfg.AgentCLIPath = profile.CLIPath
	}
	if profile.Model != "" {
		cfg.AgentModel = profile.Model
	}
	if profile.ReasoningEffort != "" {
		cfg.AgentReasoningEffort = profile.ReasoningEffort
	}
	if profile.APIBaseURL != "" {
		cfg.AgentAPIBaseURL = profile.APIBaseURL
	}
	if len(profile.Args) > 0 {
		cfg.AgentArgs = append([]string{}, profile.Args...)
	}
	if strings.TrimSpace(profile.SystemPrompt) != "" {
		cfg.AgentSystemPrompt = strings.TrimSpace(profile.SystemPrompt)
	}
	if resolvedKey := resolveWorkerCredential(cfg, profile.CredentialEnvVar); resolvedKey != "" {
		cfg.AgentAPIKey = resolvedKey
	}
	return RoutedWorker{
		ID:     profile.ID,
		Name:   profile.Name,
		Config: cfg,
	}
}

func cloneDispatchConfig(cfg Config) Config {
	out := cfg
	if cfg.AgentArgs != nil {
		out.AgentArgs = append([]string{}, cfg.AgentArgs...)
	}
	return out
}

func resolveWorkerCredential(cfg Config, envVar string) string {
	if envVar = strings.TrimSpace(envVar); envVar != "" {
		return strings.TrimSpace(os.Getenv(envVar))
	}
	if key := strings.TrimSpace(cfg.AgentAPIKey); key != "" {
		return key
	}
	switch {
	case cfg.AgentDriver == "claude":
		return strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	default:
		return ""
	}
}

func normalizePolicyRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	switch role {
	case "planning":
		return string(WorkerTypePlanner)
	case "implementation":
		return string(WorkerTypeExecutor)
	case "review", "validation":
		return string(WorkerTypeValidator)
	case "deploy", "deployment":
		return string(WorkerTypeDeployer)
	case "investigation":
		return string(WorkerTypeInvestigator)
	case "orchestration":
		return WorkerRoleOrchestrator
	case "conflict_resolution":
		return WorkerRoleConflictResolver
	default:
		return role
	}
}

func ShouldFailoverToNextWorker(err error, result *WorkerResult) bool {
	if err != nil {
		return true
	}
	if result == nil || result.Success {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(result.Error + "\n" + result.Output))
	for _, marker := range []string{
		"out of extra usage",
		"rate limit",
		"too many requests",
		"status 429",
		"quota",
		"credits",
		"status 401",
		"unauthorized",
		"authentication",
		"invalid api key",
		// Claude Code's own wording when its API key or login is rejected.
		"api key is invalid",
		"failed to authenticate",
		"missing openai api key",
		"missing api key for openai-compatible runner",
		"missing model for openai-compatible runner",
		"missing anthropic api key",
		"executable file not found",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
