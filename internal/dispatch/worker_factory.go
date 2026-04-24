package dispatch

import "log"

// NewWorker builds a standalone worker using the same driver and runner
// selection logic as the main dispatcher.
func NewWorker(cfg Config) Worker {
	driverName := cfg.AgentDriver
	if driverName == "" {
		driverName = "claude"
	}
	cliPath := cfg.AgentCLIPath
	if cliPath == "" && driverName == "claude" {
		cliPath = cfg.ClaudePath
	}
	driver, err := LookupDriver(driverName, DriverConfig{
		CLIPath:   cliPath,
		ExtraArgs: cfg.AgentArgs,
	})
	if err != nil {
		log.Printf("dispatch: %v, falling back to claude driver", err)
		driver = NewClaudeDriver(DriverConfig{CLIPath: cliPath})
	}

	worker, workerErr := newWorker(cfg, driver)
	if workerErr != nil {
		log.Printf("dispatch: %v, falling back to cli runner", workerErr)
		worker = &CLIWorker{
			Driver:      driver,
			APIKey:      cfg.APIKey,
			AgentAPIKey: cfg.AgentAPIKey,
		}
	}
	return worker
}
