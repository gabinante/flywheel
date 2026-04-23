package dispatch

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	RunnerCLI             = "cli"
	RunnerDocker          = "docker"
	RunnerOpenAIResponses = "openai-responses"
)

// AvailableRunners returns the supported worker execution backends.
func AvailableRunners() []string {
	names := []string{RunnerCLI, RunnerDocker, RunnerOpenAIResponses}
	sort.Strings(names)
	return names
}

func resolveRunnerType(cfg Config) string {
	if cfg.AgentRunner != "" {
		return cfg.AgentRunner
	}
	if cfg.DockerEnabled {
		return RunnerDocker
	}
	return RunnerCLI
}

func validateRunnerType(name string) error {
	for _, runner := range AvailableRunners() {
		if name == runner {
			return nil
		}
	}
	return fmt.Errorf("unknown agent runner %q (available: %s)", name, strings.Join(AvailableRunners(), ", "))
}

func newWorker(cfg Config, driver AgentDriver) (Worker, error) {
	switch resolveRunnerType(cfg) {
	case RunnerCLI:
		return &CLIWorker{
			Driver: driver,
			APIKey: cfg.APIKey,
		}, nil
	case RunnerDocker:
		return &DockerWorker{
			Driver:      driver,
			Image:       cfg.DockerImage,
			APIKey:      cfg.APIKey,
			RepoDir:     cfg.RepoDir,
			AgentAPIKey: cfg.AgentAPIKey,
			Memory:      cfg.DockerMemory,
			CPUs:        cfg.DockerCPUs,
			Firewall:    cfg.DockerFirewall,
		}, nil
	case RunnerOpenAIResponses:
		return &OpenAIResponsesWorker{
			Config: OpenAIResponsesConfig{
				APIBaseURL:      cfg.AgentAPIBaseURL,
				APIKey:          cfg.AgentAPIKey,
				Model:           cfg.AgentModel,
				ReasoningEffort: cfg.AgentReasoningEffort,
				PollInterval:    2 * time.Second,
			},
			APIKey: cfg.APIKey,
		}, nil
	default:
		return nil, validateRunnerType(resolveRunnerType(cfg))
	}
}
