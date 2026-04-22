package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/events"
)

// Thresholds for trend analysis.
const (
	// GoodTrendMaxRollbackRate: rollback rate must be below this to propose broadening.
	GoodTrendMaxRollbackRate = 0.05
	// GoodTrendMaxIncidentRate: incident rate must be zero for broadening proposals.
	GoodTrendMaxIncidentRate = 0.0
	// GoodTrendMinSuccessRate: success rate must be above this for broadening.
	GoodTrendMinSuccessRate = 0.90
	// BadTrendMinRollbackRate: rollback rate above this triggers review flagging.
	BadTrendMinRollbackRate = 0.15
	// BadTrendMinIncidentRate: any incident rate above zero triggers review flagging.
	BadTrendMinIncidentRate = 0.01
)

// Service provides policy calibration operations.
type Service struct {
	store *Store
	bus   events.Bus
}

// NewService returns a new policy Service.
func NewService(store *Store, bus events.Bus) *Service {
	svc := &Service{store: store, bus: bus}

	// Subscribe to ticket lifecycle events to record outcomes.
	bus.Subscribe(events.EventTicketClosed, func(ctx context.Context, ev events.Event) {
		ticketID, _ := ev.Payload["ticket_id"].(string)
		if ticketID != "" {
			_ = store.RecordOutcomeByTicket(ctx, ticketID, OutcomeSuccess)
		}
	})

	return svc
}

// CreatePolicy creates a new policy and records a change event.
func (s *Service) CreatePolicy(ctx context.Context, projectID, name, description, actorID string, rules Rules, minSample int) (*Policy, error) {
	if name == "" {
		return nil, fmt.Errorf("policy name required")
	}
	if minSample <= 0 {
		minSample = 20
	}
	now := time.Now().UTC()
	p := &Policy{
		ProjectID:   projectID,
		Name:        name,
		Description: description,
		Rules:       rules,
		Enabled:     true,
		MinSample:   minSample,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.CreatePolicy(ctx, p); err != nil {
		return nil, err
	}

	// Record change event
	_ = s.store.RecordChangeEvent(ctx, &PolicyChangeEvent{
		PolicyID:   p.ID,
		ActorID:    actorID,
		ChangeType: ChangeCreated,
		NewRules:   &rules,
		Notes:      "Policy created",
		CreatedAt:  now,
	})

	// Emit event
	_ = s.bus.Publish(ctx, events.Event{
		Type: EventPolicyCreated,
		Payload: map[string]any{
			"policy_id":  p.ID,
			"project_id": projectID,
			"actor_id":   actorID,
		},
	})

	return p, nil
}

// GetPolicy returns a policy by ID.
func (s *Service) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	return s.store.GetPolicy(ctx, id)
}

// ListPolicies returns all policies for a project.
func (s *Service) ListPolicies(ctx context.Context, projectID string, enabledOnly bool) ([]Policy, error) {
	return s.store.ListPolicies(ctx, projectID, enabledOnly)
}

// UpdatePolicy updates a policy and records the change event.
func (s *Service) UpdatePolicy(ctx context.Context, id, name, description, actorID string, rules Rules, enabled bool, minSample int) error {
	prev, err := s.store.GetPolicy(ctx, id)
	if err != nil {
		return err
	}

	if err := s.store.UpdatePolicy(ctx, id, name, description, rules, enabled, minSample); err != nil {
		return err
	}

	// Determine change type
	changeType := ChangeUpdated
	if prev.Enabled && !enabled {
		changeType = ChangeDisabled
	} else if !prev.Enabled && enabled {
		changeType = ChangeEnabled
	}

	_ = s.store.RecordChangeEvent(ctx, &PolicyChangeEvent{
		PolicyID:   id,
		ActorID:    actorID,
		ChangeType: changeType,
		PrevRules:  &prev.Rules,
		NewRules:   &rules,
		Notes:      "Policy updated",
		CreatedAt:  time.Now().UTC(),
	})

	_ = s.bus.Publish(ctx, events.Event{
		Type: EventPolicyUpdated,
		Payload: map[string]any{
			"policy_id":   id,
			"project_id":  prev.ProjectID,
			"actor_id":    actorID,
			"change_type": string(changeType),
		},
	})

	return nil
}

// RecordDecision records a policy decision on a ticket.
func (s *Service) RecordDecision(ctx context.Context, policyID, ticketID string, decision Decision, reason string) (*PolicyDecision, error) {
	d := &PolicyDecision{
		PolicyID:  policyID,
		TicketID:  ticketID,
		Decision:  decision,
		Reason:    reason,
		DecidedAt: time.Now().UTC(),
	}
	if err := s.store.RecordDecision(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// RecordOutcome records the outcome for a specific decision.
func (s *Service) RecordOutcome(ctx context.Context, decisionID string, outcome Outcome) error {
	return s.store.RecordOutcome(ctx, decisionID, outcome)
}

// RecordOutcomeByTicket records the outcome for all decisions on a ticket.
func (s *Service) RecordOutcomeByTicket(ctx context.Context, ticketID string, outcome Outcome) error {
	return s.store.RecordOutcomeByTicket(ctx, ticketID, outcome)
}

// GetMetrics computes current metrics for a policy.
func (s *Service) GetMetrics(ctx context.Context, policyID string) (*Metrics, error) {
	p, err := s.store.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	return s.store.ComputeMetrics(ctx, policyID, p.MinSample)
}

// GetPolicyHealth returns the full health view for a policy.
func (s *Service) GetPolicyHealth(ctx context.Context, policyID string) (*PolicyHealth, error) {
	p, err := s.store.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	metrics, err := s.store.ComputeMetrics(ctx, policyID, p.MinSample)
	if err != nil {
		return nil, err
	}
	proposals, err := s.store.ListProposals(ctx, policyID, true)
	if err != nil {
		return nil, err
	}
	history, err := s.store.ListChangeEvents(ctx, policyID, 20)
	if err != nil {
		return nil, err
	}
	return &PolicyHealth{
		Policy:    *p,
		Metrics:   *metrics,
		Proposals: proposals,
		History:   history,
	}, nil
}

// GetProjectHealth returns health views for all active policies in a project.
func (s *Service) GetProjectHealth(ctx context.Context, projectID string) ([]PolicyHealth, error) {
	policies, err := s.store.ListPolicies(ctx, projectID, false)
	if err != nil {
		return nil, err
	}
	var health []PolicyHealth
	for _, p := range policies {
		metrics, err := s.store.ComputeMetrics(ctx, p.ID, p.MinSample)
		if err != nil {
			continue
		}
		proposals, err := s.store.ListProposals(ctx, p.ID, true)
		if err != nil {
			proposals = nil
		}
		history, err := s.store.ListChangeEvents(ctx, p.ID, 5)
		if err != nil {
			history = nil
		}
		health = append(health, PolicyHealth{
			Policy:    p,
			Metrics:   *metrics,
			Proposals: proposals,
			History:   history,
		})
	}
	return health, nil
}

// RunCalibration checks all active policies in a project and generates proposals.
// This should be called periodically (e.g., daily or on outcome recording).
func (s *Service) RunCalibration(ctx context.Context, projectID string) ([]PolicyProposal, error) {
	policies, err := s.store.ListPolicies(ctx, projectID, true)
	if err != nil {
		return nil, err
	}
	var generated []PolicyProposal
	for _, p := range policies {
		metrics, err := s.store.ComputeMetrics(ctx, p.ID, p.MinSample)
		if err != nil {
			continue
		}
		if !metrics.SampleSizeSufficient {
			continue
		}
		proposal := s.analyzeMetrics(&p, metrics)
		if proposal != nil {
			if err := s.store.CreateProposal(ctx, proposal); err == nil {
				generated = append(generated, *proposal)
				_ = s.bus.Publish(ctx, events.Event{
					Type: EventPolicyProposalCreated,
					Payload: map[string]any{
						"policy_id":     p.ID,
						"project_id":    projectID,
						"proposal_type": string(proposal.ProposalType),
						"proposal_id":   proposal.ID,
					},
				})
			}
		}
	}
	return generated, nil
}

// analyzeMetrics checks whether outcomes are trending well or badly and generates appropriate proposal.
func (s *Service) analyzeMetrics(p *Policy, m *Metrics) *PolicyProposal {
	// Good trend: low rollback, no incidents, high success over sufficient sample
	if m.RollbackRate <= GoodTrendMaxRollbackRate &&
		m.IncidentRate <= GoodTrendMaxIncidentRate &&
		m.SuccessRate >= GoodTrendMinSuccessRate {
		return &PolicyProposal{
			PolicyID:     p.ID,
			ProposalType: ProposalBroaden,
			Suggestion: map[string]any{
				"recommendation": "Consider broadening auto-approval scope",
				"rationale":      fmt.Sprintf("%.0f%% success rate with %.1f%% rollback rate over %d decisions", m.SuccessRate*100, m.RollbackRate*100, m.TotalDecisions),
				"current_auto_approve_conditions": p.Rules.AutoApproveConditions,
			},
			Statistics: map[string]any{
				"total_decisions":    m.TotalDecisions,
				"auto_approval_rate": m.AutoApprovalRate,
				"rollback_rate":      m.RollbackRate,
				"incident_rate":      m.IncidentRate,
				"success_rate":       m.SuccessRate,
			},
			Status:    ProposalPending,
			CreatedAt: time.Now().UTC(),
		}
	}

	// Bad trend: high rollback or incidents
	if m.RollbackRate >= BadTrendMinRollbackRate || m.IncidentRate >= BadTrendMinIncidentRate {
		return &PolicyProposal{
			PolicyID:     p.ID,
			ProposalType: ProposalReview,
			Suggestion: map[string]any{
				"recommendation": "Policy requires review - outcomes trending badly",
				"concerns":       buildConcerns(m),
			},
			Statistics: map[string]any{
				"total_decisions":    m.TotalDecisions,
				"auto_approval_rate": m.AutoApprovalRate,
				"rollback_rate":      m.RollbackRate,
				"incident_rate":      m.IncidentRate,
				"success_rate":       m.SuccessRate,
			},
			Status:    ProposalPending,
			CreatedAt: time.Now().UTC(),
		}
	}

	return nil
}

func buildConcerns(m *Metrics) []string {
	var concerns []string
	if m.RollbackRate >= BadTrendMinRollbackRate {
		concerns = append(concerns, fmt.Sprintf("Rollback rate %.1f%% exceeds threshold %.1f%%", m.RollbackRate*100, BadTrendMinRollbackRate*100))
	}
	if m.IncidentRate >= BadTrendMinIncidentRate {
		concerns = append(concerns, fmt.Sprintf("Incident attribution rate %.1f%% above zero tolerance", m.IncidentRate*100))
	}
	return concerns
}

// ResolveProposal resolves a pending proposal.
func (s *Service) ResolveProposal(ctx context.Context, proposalID string, status ProposalStatus, resolvedBy string) error {
	return s.store.ResolveProposal(ctx, proposalID, status, resolvedBy)
}

// SimulateRuleChange runs candidate rules against the last N days of historical tickets.
func (s *Service) SimulateRuleChange(ctx context.Context, policyID string, candidateRules Rules, days int) (*SimulationResult, error) {
	if days <= 0 {
		days = 30
	}
	p, err := s.store.GetPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}

	// Get historical decisions for this policy
	decisions, err := s.store.ListRecentDecisionsByProject(ctx, p.ProjectID, days)
	if err != nil {
		return nil, err
	}

	// Filter to this policy's decisions
	var policyDecisions []PolicyDecision
	for _, d := range decisions {
		if d.PolicyID == policyID {
			policyDecisions = append(policyDecisions, d)
		}
	}

	result := &SimulationResult{
		PolicyID:         policyID,
		CandidateRules:   candidateRules,
		TicketsSimulated: len(policyDecisions),
	}

	var autoApprove, requireReview, block, changed int
	for _, d := range policyDecisions {
		// Simulate: evaluate candidate rules against this ticket's context
		newDecision := simulateDecision(candidateRules, d)
		sd := SimulationDecision{
			TicketID:        d.TicketID,
			CurrentDecision: d.Decision,
			NewDecision:     newDecision,
			Changed:         d.Decision != newDecision,
		}
		result.Results = append(result.Results, sd)
		switch newDecision {
		case DecisionAutoApproved:
			autoApprove++
		case DecisionRequiredReview:
			requireReview++
		case DecisionBlocked:
			block++
		}
		if sd.Changed {
			changed++
		}
	}

	total := len(policyDecisions)
	changeRate := 0.0
	if total > 0 {
		changeRate = float64(changed) / float64(total)
	}
	result.Summary = SimulationSummary{
		TotalTickets:       total,
		WouldAutoApprove:   autoApprove,
		WouldRequireReview: requireReview,
		WouldBlock:         block,
		ChangedDecisions:   changed,
		ChangeRate:         changeRate,
	}

	return result, nil
}

// simulateDecision evaluates candidate rules against a historical decision.
// In a full implementation this would re-evaluate conditions against ticket metadata;
// for now it uses rule count heuristics (more auto-approve conditions → more auto-approvals).
func simulateDecision(rules Rules, d PolicyDecision) Decision {
	// If candidate has more restrictive block conditions and the original was auto_approved,
	// simulate as blocked. If candidate has more permissive auto-approve conditions and
	// original was required_review, simulate as auto_approved. Otherwise keep the same.
	if len(rules.BlockConditions) > 0 && d.Decision == DecisionAutoApproved {
		return DecisionBlocked
	}
	if len(rules.AutoApproveConditions) > 0 && d.Decision == DecisionRequiredReview {
		return DecisionAutoApproved
	}
	return d.Decision
}

// ListDecisions returns recent decisions for a policy.
func (s *Service) ListDecisions(ctx context.Context, policyID string, limit int) ([]PolicyDecision, error) {
	return s.store.ListDecisions(ctx, policyID, limit)
}

// ListChangeEvents returns the change history for a policy.
func (s *Service) ListChangeEvents(ctx context.Context, policyID string, limit int) ([]PolicyChangeEvent, error) {
	return s.store.ListChangeEvents(ctx, policyID, limit)
}
