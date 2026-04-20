package policy

import (
	"context"
	"strings"
)

// TicketForPolicy is the subset of ticket data needed for policy evaluation.
// Matches the ticket.Ticket structure to avoid circular imports.
type TicketForPolicy struct {
	ID          string
	ProjectID   string
	State       string
	Environment string
	Type        string
	Priority    int
	Services    []string
}

// TicketPolicyAdapter adapts policy.Service to the ticket.PolicyEvaluator interface.
// It bridges the policy and ticket packages without circular imports.
type TicketPolicyAdapter struct {
	svc *Service
}

// NewTicketPolicyAdapter returns a new adapter.
func NewTicketPolicyAdapter(svc *Service) *TicketPolicyAdapter {
	return &TicketPolicyAdapter{svc: svc}
}

// EvaluateForTicketData evaluates the active policy for a ticket against the given trigger.
// This method accepts the raw ticket fields to avoid importing the ticket package.
func (a *TicketPolicyAdapter) EvaluateForTicketData(ctx context.Context, projectID, environment, ticketType string, priority int, services []string, trigger string) (*PolicyDecision, error) {
	tctx := TransitionContext{
		Environment:    environment,
		TicketType:     ticketType,
		TicketPriority: priority,
		Services:       services,
		Transition:     trigger,
	}

	return a.svc.EvaluateTransition(ctx, projectID, tctx)
}

// ExtractServicesFromConstraints extracts service names from ticket constraints.
// Convention: constraints containing "service:" prefix are treated as service identifiers.
func ExtractServicesFromConstraints(constraints []string) []string {
	var services []string
	for _, c := range constraints {
		if strings.HasPrefix(c, "service:") {
			svc := strings.TrimPrefix(c, "service:")
			svc = strings.TrimSpace(svc)
			if svc != "" {
				services = append(services, svc)
			}
		}
	}
	return services
}
