package policy

import (
	"testing"
)

func TestAnalyzeMetrics_GoodTrend(t *testing.T) {
	svc := &Service{}
	p := &Policy{ID: "pol-1", MinSample: 20}
	m := &Metrics{
		PolicyID:             "pol-1",
		TotalDecisions:       50,
		AutoApprovalRate:     0.6,
		RollbackRate:         0.02,
		IncidentRate:         0.0,
		SuccessRate:          0.95,
		SampleSizeSufficient: true,
	}
	proposal := svc.analyzeMetrics(p, m)
	if proposal == nil {
		t.Fatal("expected broadening proposal for good metrics")
	}
	if proposal.ProposalType != ProposalBroaden {
		t.Errorf("expected ProposalBroaden, got %s", proposal.ProposalType)
	}
	if proposal.Status != ProposalPending {
		t.Errorf("expected pending status, got %s", proposal.Status)
	}
}

func TestAnalyzeMetrics_BadTrend(t *testing.T) {
	svc := &Service{}
	p := &Policy{ID: "pol-1", MinSample: 20}
	m := &Metrics{
		PolicyID:             "pol-1",
		TotalDecisions:       30,
		AutoApprovalRate:     0.8,
		RollbackRate:         0.20,
		IncidentRate:         0.05,
		SuccessRate:          0.75,
		SampleSizeSufficient: true,
	}
	proposal := svc.analyzeMetrics(p, m)
	if proposal == nil {
		t.Fatal("expected review proposal for bad metrics")
	}
	if proposal.ProposalType != ProposalReview {
		t.Errorf("expected ProposalReview, got %s", proposal.ProposalType)
	}
}

func TestAnalyzeMetrics_NeutralTrend(t *testing.T) {
	svc := &Service{}
	p := &Policy{ID: "pol-1", MinSample: 20}
	m := &Metrics{
		PolicyID:             "pol-1",
		TotalDecisions:       25,
		AutoApprovalRate:     0.5,
		RollbackRate:         0.08,
		IncidentRate:         0.0,
		SuccessRate:          0.80,
		SampleSizeSufficient: true,
	}
	proposal := svc.analyzeMetrics(p, m)
	if proposal != nil {
		t.Errorf("expected no proposal for neutral metrics, got %s", proposal.ProposalType)
	}
}

func TestAnalyzeMetrics_InsufficientSample(t *testing.T) {
	svc := &Service{}
	p := &Policy{ID: "pol-1", MinSample: 20}
	m := &Metrics{
		PolicyID:             "pol-1",
		TotalDecisions:       5,
		AutoApprovalRate:     1.0,
		RollbackRate:         0.0,
		IncidentRate:         0.0,
		SuccessRate:          1.0,
		SampleSizeSufficient: false,
	}
	// Even with perfect metrics, insufficient sample means no proposal in RunCalibration.
	// The method analyzeMetrics itself doesn't check sample - RunCalibration does.
	proposal := svc.analyzeMetrics(p, m)
	if proposal == nil {
		t.Fatal("analyzeMetrics itself doesn't check sample size")
	}
}

func TestSimulateDecision(t *testing.T) {
	tests := []struct {
		name     string
		rules    Rules
		decision PolicyDecision
		expected Decision
	}{
		{
			name:     "block conditions override auto_approved",
			rules:    Rules{BlockConditions: []Condition{{Field: "test", Operator: "eq", Value: true}}},
			decision: PolicyDecision{Decision: DecisionAutoApproved},
			expected: DecisionBlocked,
		},
		{
			name:     "auto-approve conditions upgrade required_review",
			rules:    Rules{AutoApproveConditions: []Condition{{Field: "test", Operator: "eq", Value: true}}},
			decision: PolicyDecision{Decision: DecisionRequiredReview},
			expected: DecisionAutoApproved,
		},
		{
			name:     "no matching rules keeps same decision",
			rules:    Rules{},
			decision: PolicyDecision{Decision: DecisionRequiredReview},
			expected: DecisionRequiredReview,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := simulateDecision(tt.rules, tt.decision)
			if got != tt.expected {
				t.Errorf("got %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestBuildConcerns(t *testing.T) {
	m := &Metrics{RollbackRate: 0.20, IncidentRate: 0.03}
	concerns := buildConcerns(m)
	if len(concerns) != 2 {
		t.Errorf("expected 2 concerns, got %d", len(concerns))
	}
}

func TestSimulationSummary(t *testing.T) {
	rules := Rules{
		AutoApproveConditions: []Condition{{Field: "priority", Operator: "gt", Value: 2}},
	}
	decisions := []PolicyDecision{
		{Decision: DecisionRequiredReview},
		{Decision: DecisionAutoApproved},
		{Decision: DecisionRequiredReview},
	}
	var autoApprove, changed int
	for _, d := range decisions {
		newD := simulateDecision(rules, d)
		if newD == DecisionAutoApproved {
			autoApprove++
		}
		if newD != d.Decision {
			changed++
		}
	}
	if autoApprove != 3 {
		t.Errorf("expected 3 auto-approved, got %d", autoApprove)
	}
	if changed != 2 {
		t.Errorf("expected 2 changed, got %d", changed)
	}
}
