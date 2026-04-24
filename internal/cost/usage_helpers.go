package cost

import "strings"

// EstimateTokens returns a rough token estimate for free-form text.
// CLI harnesses do not expose exact provider usage, so we approximate at ~4 chars/token.
func EstimateTokens(parts ...string) int64 {
	var chars int64
	for _, part := range parts {
		chars += int64(len(strings.TrimSpace(part)))
	}
	if chars <= 0 {
		return 0
	}
	// Round up so very short prompts still count as at least one token.
	return (chars + 3) / 4
}

// InferProviderModel maps runner/driver configuration to a reporting identity.
func InferProviderModel(runner, driver, configuredModel string) (provider, model string) {
	model = strings.TrimSpace(configuredModel)
	driver = strings.ToLower(strings.TrimSpace(driver))
	runner = strings.ToLower(strings.TrimSpace(runner))
	if model != "" {
		return inferProviderFromModel(model, driver, runner), model
	}

	switch {
	case runner == "openai-responses":
		return "openai", "gpt-5.2-codex"
	case runner == "openai-compatible":
		return "openai-compatible", "api-compatible"
	case driver == "claude":
		return "anthropic", "claude-cli"
	case driver == "codex":
		return "openai", "codex-cli"
	case driver == "generic":
		return "generic", "generic-cli"
	case driver != "":
		return driver, driver + "-cli"
	default:
		return "unknown", "unknown-cli"
	}
}

// APILabel normalizes providers and models into a user-facing API family.
func APILabel(provider, model string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(model, "claude"):
		return "claude"
	case strings.Contains(model, "codex"):
		return "codex"
	case strings.Contains(model, "gpt"):
		return "openai"
	case strings.Contains(model, "gemini"):
		return "gemini"
	case provider == "openai-compatible":
		return "openai-compatible"
	case provider == "anthropic":
		return "claude"
	case provider == "openai":
		return "openai"
	case provider == "google":
		return "gemini"
	case provider != "":
		return provider
	default:
		return "unknown"
	}
}

func inferProviderFromModel(model, driver, runner string) string {
	lower := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(lower, "claude"):
		return "anthropic"
	case strings.Contains(lower, "codex"), strings.Contains(lower, "gpt"), strings.Contains(lower, "o1"), runner == "openai-responses":
		return "openai"
	case runner == "openai-compatible":
		return "openai-compatible"
	case strings.Contains(lower, "gemini"):
		return "google"
	case driver == "claude":
		return "anthropic"
	case driver == "codex":
		return "openai"
	case driver != "":
		return driver
	default:
		return "unknown"
	}
}
