// Package mirror provides one-way ticket mirroring to external issue trackers
// (Linear, Jira). Warrant is the source of truth; updates flow out.
// Two-way sync is an explicit non-goal for v1.
package mirror

import "context"

// Adapter is the contract that external issue tracker integrations implement.
// Each adapter handles creating, updating, and closing mirrored tickets in
// its respective system. Implementations must be safe for concurrent use.
//
// Each method receives the project-level Config so adapters can access their
// provider-specific settings (team ID, project key, labels, etc.) without
// needing to store per-project state internally.
type Adapter interface {
	// Name returns the adapter identifier (e.g. "linear", "jira").
	Name() string

	// CreateTicket creates a new issue in the external system mirroring the
	// given ticket data. Returns the external issue ID/key for reference.
	CreateTicket(ctx context.Context, cfg *Config, data TicketData) (externalID string, err error)

	// UpdateState updates the external issue's workflow state based on
	// our ticket state transition.
	UpdateState(ctx context.Context, cfg *Config, externalID string, newState string, data TicketData) error

	// UpdateFields updates custom fields on the external issue from our
	// structured ticket data (risk, environment, linked services).
	UpdateFields(ctx context.Context, cfg *Config, externalID string, fields CustomFields) error

	// CloseTicket closes the external issue with a link back to our full context.
	CloseTicket(ctx context.Context, cfg *Config, externalID string, contextURL string, data TicketData) error
}

// TicketData is the subset of our ticket data relevant for mirroring.
// Adapters use this to populate fields in the external system.
type TicketData struct {
	ID          string
	ProjectID   string
	Title       string
	Type        string // task, bug, spike, review
	Priority    int    // 0=P0, 3=P3
	State       string
	Description string
	Environment string
	AssignedTo  string
	URL         string // link back to warrant ticket
}

// CustomFields holds structured ticket data that maps to custom fields
// in external issue trackers.
type CustomFields struct {
	RiskClassification string   // e.g. "low", "medium", "high"
	EnvironmentTarget  string   // e.g. "production", "staging", "development"
	LinkedServices     []string // service IDs or names relevant to this ticket
}
