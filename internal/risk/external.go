package risk

import "strings"

// ExternalAPIClassifier classifies operations against external APIs using
// per-integration operation semantics tables.
//
// Classification rules:
//   - GET, HEAD, OPTIONS → safe
//   - POST to create/additive endpoints → low
//   - PUT/PATCH to update endpoints → reversible
//   - DELETE endpoints → high or destructive depending on resource
//   - Deploy/release operations → high
//   - Per-integration overrides (e.g. Stripe charge = high, GitHub delete repo = destructive)
//
// The classifier inspects:
//   - Operation.Action: HTTP method or operation name (e.g. "GET", "POST", "deploy")
//   - Operation.Target: API endpoint or integration name
//   - Operation.Details["integration"]: integration identifier (e.g. "stripe", "github", "slack")
//   - Operation.Details["operation"]: specific operation name (e.g. "create_charge", "delete_repo")
type ExternalAPIClassifier struct {
	// IntegrationTables maps integration names to their operation semantics.
	// Operators can register custom tables for their integrations.
	IntegrationTables map[string]OperationTable
}

// OperationTable maps operation names to their risk levels for a specific integration.
type OperationTable map[string]OperationSemantic

// OperationSemantic describes the risk semantics of a specific API operation.
type OperationSemantic struct {
	Level       RiskLevel
	Description string
}

// DefaultIntegrationTables returns the built-in operation semantics for common integrations.
func DefaultIntegrationTables() map[string]OperationTable {
	return map[string]OperationTable{
		"stripe": {
			"create_charge":       {RiskHigh, "Creates a financial charge — real money movement"},
			"create_refund":       {RiskHigh, "Creates a refund — real money movement"},
			"delete_customer":     {RiskDestructive, "Deletes a Stripe customer — irreversible data loss"},
			"create_subscription": {RiskReversible, "Creates a subscription — can be cancelled"},
			"cancel_subscription": {RiskHigh, "Cancels a subscription — may affect revenue"},
			"update_customer":     {RiskReversible, "Updates customer data — reversible"},
			"list_charges":        {RiskSafe, "Lists charges — read only"},
			"get_customer":        {RiskSafe, "Gets customer data — read only"},
		},
		"github": {
			"delete_repo":     {RiskDestructive, "Deletes a GitHub repository — irreversible"},
			"delete_branch":   {RiskHigh, "Deletes a branch — can be recovered from reflog"},
			"force_push":      {RiskDestructive, "Force pushes — may overwrite history"},
			"create_repo":     {RiskLow, "Creates a repository — additive"},
			"create_branch":   {RiskLow, "Creates a branch — additive"},
			"create_pr":       {RiskLow, "Creates a pull request — additive"},
			"merge_pr":        {RiskReversible, "Merges a pull request — reversible via revert"},
			"update_branch_protection": {RiskHigh, "Changes branch protection — affects repo security"},
			"list_repos":      {RiskSafe, "Lists repositories — read only"},
			"get_pr":          {RiskSafe, "Gets pull request — read only"},
		},
		"slack": {
			"post_message":    {RiskLow, "Posts a message — visible but low impact"},
			"delete_message":  {RiskReversible, "Deletes a message — data loss but recoverable from logs"},
			"update_message":  {RiskLow, "Updates a message — reversible"},
			"create_channel":  {RiskLow, "Creates a channel — additive"},
			"archive_channel": {RiskReversible, "Archives a channel — reversible"},
			"delete_channel":  {RiskHigh, "Deletes a channel — difficult to recover"},
			"list_channels":   {RiskSafe, "Lists channels — read only"},
		},
		"pagerduty": {
			"create_incident":   {RiskHigh, "Creates an incident — triggers on-call alerts"},
			"resolve_incident":  {RiskReversible, "Resolves an incident — can be reopened"},
			"acknowledge_incident": {RiskLow, "Acknowledges an incident — low impact"},
			"list_incidents":    {RiskSafe, "Lists incidents — read only"},
		},
		"datadog": {
			"create_monitor":  {RiskLow, "Creates a monitor — additive"},
			"delete_monitor":  {RiskReversible, "Deletes a monitor — can be recreated"},
			"mute_monitor":    {RiskHigh, "Mutes a monitor — silences alerts, may miss issues"},
			"create_downtime": {RiskReversible, "Creates downtime — can be cancelled"},
			"query_metrics":   {RiskSafe, "Queries metrics — read only"},
		},
	}
}

// NewExternalAPIClassifier creates a classifier with default integration tables.
// Additional tables can be registered via IntegrationTables.
func NewExternalAPIClassifier() *ExternalAPIClassifier {
	return &ExternalAPIClassifier{
		IntegrationTables: DefaultIntegrationTables(),
	}
}

// Classify classifies an external API operation.
func (c *ExternalAPIClassifier) Classify(op Operation) (RiskLevel, []RuleCitation) {
	integration := ""
	if i, ok := op.Details["integration"].(string); ok {
		integration = strings.ToLower(i)
	}
	operation := ""
	if o, ok := op.Details["operation"].(string); ok {
		operation = strings.ToLower(o)
	}

	// Check per-integration operation tables first.
	if integration != "" && c.IntegrationTables != nil {
		if table, ok := c.IntegrationTables[integration]; ok {
			if semantic, ok := table[operation]; ok {
				return semantic.Level, []RuleCitation{{
					Rule:        "api." + integration + "." + operation,
					Backend:     BackendExternalAPI,
					Level:       semantic.Level,
					Description: semantic.Description,
				}}
			}
			// Known integration but unknown operation → high risk.
			return RiskHigh, []RuleCitation{{
				Rule:        "api." + integration + ".unknown_operation",
				Backend:     BackendExternalAPI,
				Level:       RiskHigh,
				Description: "Unknown operation '" + operation + "' on known integration '" + integration + "' — defaults to high risk",
			}}
		}
	}

	// Fall back to HTTP method classification.
	return c.classifyByHTTPMethod(op)
}

// classifyByHTTPMethod uses the HTTP method (action) for generic API risk classification.
func (c *ExternalAPIClassifier) classifyByHTTPMethod(op Operation) (RiskLevel, []RuleCitation) {
	method := strings.ToUpper(strings.TrimSpace(op.Action))

	switch method {
	case "GET", "HEAD", "OPTIONS":
		return RiskSafe, []RuleCitation{{
			Rule:        "api.http_read",
			Backend:     BackendExternalAPI,
			Level:       RiskSafe,
			Description: "HTTP " + method + " request to '" + op.Target + "' — read only",
		}}

	case "POST":
		return RiskLow, []RuleCitation{{
			Rule:        "api.http_post",
			Backend:     BackendExternalAPI,
			Level:       RiskLow,
			Description: "HTTP POST to '" + op.Target + "' — typically creates resources",
		}}

	case "PUT", "PATCH":
		return RiskReversible, []RuleCitation{{
			Rule:        "api.http_update",
			Backend:     BackendExternalAPI,
			Level:       RiskReversible,
			Description: "HTTP " + method + " to '" + op.Target + "' — updates existing resources",
		}}

	case "DELETE":
		return RiskHigh, []RuleCitation{{
			Rule:        "api.http_delete",
			Backend:     BackendExternalAPI,
			Level:       RiskHigh,
			Description: "HTTP DELETE to '" + op.Target + "' — removes resources",
		}}

	default:
		// Unknown HTTP method or custom action → maximum risk for unknown integrations.
		return RiskDestructive, []RuleCitation{{
			Rule:        "api.unknown_method",
			Backend:     BackendExternalAPI,
			Level:       RiskDestructive,
			Description: "Unknown API method/action '" + op.Action + "' on '" + op.Target + "' — defaults to maximum risk",
		}}
	}
}
