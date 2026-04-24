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
	base    Config
	counter uint64
}

func NewProjectWorkerRouter(base Config) *ProjectWorkerRouter {
	return &ProjectWorkerRouter{base: base}
}

func (r *ProjectWorkerRouter) Candidates(proj *project.Project, role string) []RoutedWorker {
	cfg := project.DispatchConfig{}.Normalized()
	if proj != nil {
		cfg = proj.DispatchConfig.Normalized()
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
	if profile.Driver != "" {
		cfg.AgentDriver = profile.Driver
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
	case resolveRunnerType(cfg) == RunnerOpenAIResponses, resolveRunnerType(cfg) == RunnerOpenAICompatible:
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	case cfg.AgentDriver == "codex":
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
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
