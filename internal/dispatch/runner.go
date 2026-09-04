package dispatch

import (
	"fmt"
	"sort"
	"strings"
)

const (
	RunnerCLI = "cli"
)

// AvailableRunners returns the supported worker execution backends.
// Flywheel runs local CLI harnesses (Claude Code, Codex); API-native and
// Docker runners were removed in the local-first revival.
func AvailableRunners() []string {
	names := []string{RunnerCLI}
	sort.Strings(names)
	return names
}

func resolveRunnerType(cfg Config) string {
	if cfg.AgentRunner != "" {
		return cfg.AgentRunner
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
			Driver:      driver,
			APIKey:      cfg.APIKey,
			AgentAPIKey: cfg.AgentAPIKey,
		}, nil
	default:
		return nil, validateRunnerType(resolveRunnerType(cfg))
	}
}
