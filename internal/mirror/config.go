package mirror

import (
	"encoding/json"
	"fmt"
)

// Config holds the mirror configuration for a project.
// Stored in the project's ContextPack.Extra under the key "mirror_config".
type Config struct {
	// Enabled controls whether mirroring is active for this project.
	// Opt-in: defaults to false.
	Enabled bool `json:"enabled"`

	// Provider is the adapter to use ("linear" or "jira").
	Provider string `json:"provider"`

	// StateMapping maps our ticket states to the external system's workflow states.
	// Key: our state (e.g. "executing"), Value: their state (e.g. "In Progress").
	StateMapping map[string]string `json:"state_mapping"`

	// Linear-specific configuration.
	Linear *LinearConfig `json:"linear,omitempty"`

	// Jira-specific configuration.
	Jira *JiraConfig `json:"jira,omitempty"`

	// ContextBaseURL is the base URL for generating links back to warrant tickets.
	// e.g. "https://warrant.example.com" -> ticket URL is "{base}/#/tickets/{id}"
	ContextBaseURL string `json:"context_base_url,omitempty"`
}

// LinearConfig holds Linear-specific mirror settings.
type LinearConfig struct {
	// TeamID is the Linear team to create issues in.
	TeamID string `json:"team_id"`

	// APIKeySecret is the secret name (resolved via varlock) for the Linear API key.
	// Never stored as a raw value.
	APIKeySecret string `json:"api_key_secret"`

	// Labels are optional Linear label IDs to apply to mirrored issues.
	Labels []string `json:"labels,omitempty"`
}

// JiraConfig holds Jira-specific mirror settings.
type JiraConfig struct {
	// BaseURL is the Jira instance URL (e.g. "https://company.atlassian.net").
	BaseURL string `json:"base_url"`

	// ProjectKey is the Jira project key (e.g. "ENG").
	ProjectKey string `json:"project_key"`

	// IssueType is the default issue type for mirrored tickets (e.g. "Task", "Story").
	IssueType string `json:"issue_type"`

	// APITokenSecret is the secret name for the Jira API token.
	APITokenSecret string `json:"api_token_secret"`

	// UserEmail is the Jira user email for API authentication.
	UserEmail string `json:"user_email"`
}

// ConfigKey is the ContextPack.Extra key where mirror config is stored.
const ConfigKey = "mirror_config"

// ParseConfig extracts mirror configuration from a project's ContextPack.Extra map.
// Returns nil config (not an error) if mirroring is not configured.
func ParseConfig(extra map[string]string) (*Config, error) {
	raw, ok := extra[ConfigKey]
	if !ok || raw == "" {
		return nil, nil
	}
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("mirror: invalid config: %w", err)
	}
	return &cfg, nil
}

// Validate checks that the configuration is complete for the specified provider.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Provider == "" {
		return fmt.Errorf("mirror: provider is required when enabled")
	}
	if c.StateMapping == nil || len(c.StateMapping) == 0 {
		return fmt.Errorf("mirror: state_mapping is required")
	}
	switch c.Provider {
	case "linear":
		if c.Linear == nil {
			return fmt.Errorf("mirror: linear config is required for provider 'linear'")
		}
		if c.Linear.TeamID == "" {
			return fmt.Errorf("mirror: linear.team_id is required")
		}
		if c.Linear.APIKeySecret == "" {
			return fmt.Errorf("mirror: linear.api_key_secret is required")
		}
	case "jira":
		if c.Jira == nil {
			return fmt.Errorf("mirror: jira config is required for provider 'jira'")
		}
		if c.Jira.BaseURL == "" {
			return fmt.Errorf("mirror: jira.base_url is required")
		}
		if c.Jira.ProjectKey == "" {
			return fmt.Errorf("mirror: jira.project_key is required")
		}
		if c.Jira.APITokenSecret == "" {
			return fmt.Errorf("mirror: jira.api_token_secret is required")
		}
	default:
		return fmt.Errorf("mirror: unsupported provider %q (supported: linear, jira)", c.Provider)
	}
	return nil
}

// MapState translates our ticket state to the external system's workflow state.
// Returns empty string if no mapping is configured for the given state.
func (c *Config) MapState(ourState string) string {
	if c.StateMapping == nil {
		return ""
	}
	return c.StateMapping[ourState]
}

// TicketURL generates the full URL to a ticket in our system.
func (c *Config) TicketURL(ticketID string) string {
	if c.ContextBaseURL == "" {
		return ""
	}
	return c.ContextBaseURL + "/#/tickets/" + ticketID
}
