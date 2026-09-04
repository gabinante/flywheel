package dispatch

import "log/slog"

// NewWorker builds a standalone worker using the same driver and runner
// selection logic as the main dispatcher.
func NewWorker(cfg Config) Worker {
	driverName := cfg.AgentDriver
	if driverName == "" {
		driverName = "claude"
	}
	cliPath := cfg.AgentCLIPath
	if cliPath == "" {
		switch driverName {
		case "claude":
			cliPath = cfg.ClaudePath
		case "codex":
			cliPath = cfg.CodexPath
			if cliPath == "" {
				cliPath = "codex"
			}
		}
	}
	driver, err := LookupDriver(driverName, DriverConfig{
		CLIPath:         cliPath,
		ExtraArgs:       cfg.AgentArgs,
		Model:           cfg.AgentModel,
		ReasoningEffort: cfg.AgentReasoningEffort,
	})
	if err != nil {
		slog.Warn("driver lookup failed, falling back to claude driver", "error", err)
		driver = NewClaudeDriver(DriverConfig{CLIPath: cliPath})
	}

	worker, workerErr := newWorker(cfg, driver)
	if workerErr != nil {
		slog.Warn("worker creation failed, falling back to cli runner", "error", workerErr)
		worker = &CLIWorker{
			Driver:      driver,
			APIKey:      cfg.APIKey,
			AgentAPIKey: cfg.AgentAPIKey,
		}
	}
	return worker
}
