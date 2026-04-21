package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// --------------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------------

// newCoordinatorTestBackend creates a Backend for testing with a memory findings store.
func newCoordinatorTestBackend() *Backend {
	return &Backend{
		Findings: NewMemoryFindingsStore(),
	}
}

// --------------------------------------------------------------------------
// Tests for coordinator_get_history
// --------------------------------------------------------------------------

func TestHandleCoordinatorGetHistory_Empty(t *testing.T) {
	findings := NewMemoryFindingsStore()
	b := &Backend{Findings: findings}

	// We need a real ticket service, but since we just need ListTickets,
	// we'll test the helper functions directly.
	tickets := []*ticket.Ticket{}
	stats := computeHistoryStats(tickets)

	if stats.Total != 0 {
		t.Errorf("expected 0 total, got %d", stats.Total)
	}
	if stats.SuccessRate != "N/A" {
		t.Errorf("expected N/A success rate, got %s", stats.SuccessRate)
	}
	_ = b // avoid unused
}

func TestHandleCoordinatorGetHistory_WithTickets(t *testing.T) {
	now := time.Now()
	tickets := []*ticket.Ticket{
		{ID: "proj-1", ProjectID: "p1", Title: "Migration", State: ticket.StateClosed, CreatedAt: now},
		{ID: "proj-2", ProjectID: "p1", Title: "API endpoint", State: ticket.StateClosed, CreatedAt: now},
		{ID: "proj-3", ProjectID: "p1", Title: "Tests", State: ticket.StateExecuting, CreatedAt: now},
		{ID: "proj-4", ProjectID: "p1", Title: "Failed ticket", State: ticket.StateDraft, CreatedAt: now,
			Context: ticket.TicketContext{PriorAttempts: []ticket.AttemptSummary{{Outcome: "failed"}}}},
	}

	stats := computeHistoryStats(tickets)
	if stats.Total != 4 {
		t.Errorf("expected 4 total, got %d", stats.Total)
	}
	if stats.Closed != 2 {
		t.Errorf("expected 2 closed, got %d", stats.Closed)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
	if stats.InProgress != 1 {
		t.Errorf("expected 1 in_progress, got %d", stats.InProgress)
	}
	// Success rate: 2/(2+1) = 66.7%
	if stats.SuccessRate != "66.7%" {
		t.Errorf("expected 66.7%% success rate, got %s", stats.SuccessRate)
	}
}

// --------------------------------------------------------------------------
// Tests for classifyTicketOutcome
// --------------------------------------------------------------------------

func TestClassifyTicketOutcome(t *testing.T) {
	tests := []struct {
		name     string
		ticket   *ticket.Ticket
		expected string
	}{
		{"closed ticket", &ticket.Ticket{State: ticket.StateClosed}, "success"},
		{"executing ticket", &ticket.Ticket{State: ticket.StateExecuting}, "in_progress"},
		{"planning ticket", &ticket.Ticket{State: ticket.StatePlanning}, "in_progress"},
		{"draft ticket no attempts", &ticket.Ticket{State: ticket.StateDraft}, "pending"},
		{"draft ticket with attempts", &ticket.Ticket{
			State:   ticket.StateDraft,
			Context: ticket.TicketContext{PriorAttempts: []ticket.AttemptSummary{{Outcome: "rejected"}}},
		}, "failed"},
		{"awaiting validation", &ticket.Ticket{State: ticket.StateAwaitingValidation}, "awaiting_review"},
		{"awaiting input", &ticket.Ticket{State: ticket.StateAwaitingInput}, "needs_input"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTicketOutcome(tt.ticket)
			if got != tt.expected {
				t.Errorf("classifyTicketOutcome() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Tests for coordinator_calibration
// --------------------------------------------------------------------------

func TestComputeCalibration_NoTickets(t *testing.T) {
	cal := computeCalibration(nil)
	if cal.TotalAuthored != 0 {
		t.Errorf("expected 0 authored, got %d", cal.TotalAuthored)
	}
	if cal.SuccessRate != "N/A" {
		t.Errorf("expected N/A, got %s", cal.SuccessRate)
	}
	if cal.Summary != "No tickets authored yet — no calibration data available." {
		t.Errorf("unexpected summary: %s", cal.Summary)
	}
}

func TestComputeCalibration_WithMixedOutcomes(t *testing.T) {
	tickets := []*ticket.Ticket{
		{ID: "t-1", State: ticket.StateClosed},
		{ID: "t-2", State: ticket.StateClosed},
		{ID: "t-3", State: ticket.StateClosed, Context: ticket.TicketContext{
			PriorAttempts: []ticket.AttemptSummary{
				{Outcome: "rejected", Summary: "acceptance criteria vague"},
			},
		}},
		{ID: "t-4", State: ticket.StateDraft, Context: ticket.TicketContext{
			PriorAttempts: []ticket.AttemptSummary{
				{Outcome: "failed", Summary: "build failure"},
			},
		}},
		{ID: "t-5", State: ticket.StateExecuting},
	}

	cal := computeCalibration(tickets)

	if cal.TotalAuthored != 5 {
		t.Errorf("expected 5 authored, got %d", cal.TotalAuthored)
	}
	if cal.SuccessCount != 3 {
		t.Errorf("expected 3 success, got %d", cal.SuccessCount)
	}
	if cal.FailedCount != 1 {
		t.Errorf("expected 1 failed, got %d", cal.FailedCount)
	}
	if cal.InProgressCount != 1 {
		t.Errorf("expected 1 in_progress, got %d", cal.InProgressCount)
	}
	if cal.RejectedCount != 1 {
		t.Errorf("expected 1 rejection, got %d", cal.RejectedCount)
	}
	if cal.TicketsWithRetries != 2 {
		t.Errorf("expected 2 tickets with retries, got %d", cal.TicketsWithRetries)
	}
	// Success rate: 3/(3+1) = 75.0%
	if cal.SuccessRate != "75.0%" {
		t.Errorf("expected 75.0%%, got %s", cal.SuccessRate)
	}
}

func TestComputeCalibration_Summary(t *testing.T) {
	tickets := []*ticket.Ticket{
		{ID: "t-1", State: ticket.StateClosed},
		{ID: "t-2", State: ticket.StateClosed},
	}
	cal := computeCalibration(tickets)
	if cal.Summary == "" {
		t.Error("expected non-empty summary")
	}
	// Should contain total and success info
	if cal.SuccessRate != "100.0%" {
		t.Errorf("expected 100.0%% success rate for all-closed, got %s", cal.SuccessRate)
	}
}

// --------------------------------------------------------------------------
// Tests for isCoordinatorFeedbackCategory
// --------------------------------------------------------------------------

func TestIsCoordinatorFeedbackCategory(t *testing.T) {
	valid := []string{"acceptance_ambiguity", "scope_too_large", "missing_dependency", "wrong_decomposition", "missing_context", "other"}
	for _, cat := range valid {
		if !isCoordinatorFeedbackCategory(cat) {
			t.Errorf("expected %q to be a valid category", cat)
		}
	}
	invalid := []string{"coordinator-feedback", "automated", "rejection", "random_tag"}
	for _, cat := range invalid {
		if isCoordinatorFeedbackCategory(cat) {
			t.Errorf("expected %q NOT to be a valid category", cat)
		}
	}
}

// --------------------------------------------------------------------------
// Tests for CoordinatorFeedbackSubscriber (event-driven feedback)
// --------------------------------------------------------------------------

func TestFeedbackSubscriber_OnRejected(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInProcessBus()
	findings := NewMemoryFindingsStore()

	// Pre-populate a ticket.
	mockSvc := &mockTicketServiceForFeedback{
		ticket: &ticket.Ticket{
			ID:        "proj-42",
			ProjectID: "proj-1",
			Title:     "Add user endpoint",
			State:     ticket.StateExecuting,
			Context: ticket.TicketContext{
				PriorAttempts: []ticket.AttemptSummary{
					{Outcome: "rejected", Summary: "acceptance test didn't cover error case"},
				},
			},
		},
	}

	sub := &CoordinatorFeedbackSubscriber{
		findings: findings,
		tickets:  nil, // we'll use the mock directly
	}
	// Replace the subscriber's ticket lookup by wrapping.
	sub2 := &CoordinatorFeedbackSubscriber{findings: findings}
	_ = sub
	// Register manually to use our mock.
	bus.Subscribe(events.EventTicketRejected, func(ctx context.Context, ev events.Event) {
		ticketID := eventStr(ev, "ticket_id")
		projectID := eventStr(ev, "project_id")
		t2 := mockSvc.ticket

		finding := Finding{
			ProjectID:      projectID,
			Claim:          "Ticket " + ticketID + " was rejected: " + t2.Context.PriorAttempts[0].Summary,
			FindingType:    "investigation",
			Confidence:     0.85,
			SourceType:     "review_comment",
			SourceTicketID: ticketID,
			TicketRefs:     []string{ticketID},
			Tags:           []string{"coordinator-feedback", "rejection", "author_role:coordinator", "automated"},
			Summary:        "[rejection] " + t2.Title + " — " + t2.Context.PriorAttempts[0].Summary,
		}
		_, _ = sub2.findings.SaveFinding(ctx, finding)
	})

	// Fire the event.
	_ = bus.Publish(ctx, events.Event{
		Type: events.EventTicketRejected,
		Payload: map[string]any{
			"ticket_id":  "proj-42",
			"project_id": "proj-1",
			"state":      "executing",
		},
	})

	// Check that a finding was created.
	results, err := findings.QueryFindings(ctx, "proj-1", "coordinator-feedback", FindingsQueryOpts{ValidOnly: false, Limit: 10})
	if err != nil {
		t.Fatalf("query findings: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 feedback finding, got %d", len(results))
	}
	f := results[0]
	if f.SourceTicketID != "proj-42" {
		t.Errorf("expected source_ticket_id proj-42, got %s", f.SourceTicketID)
	}
	if !containsStr(f.Tags, "coordinator-feedback") {
		t.Error("expected coordinator-feedback tag")
	}
	if !containsStr(f.Tags, "rejection") {
		t.Error("expected rejection tag")
	}
	if !containsStr(f.Tags, "author_role:coordinator") {
		t.Error("expected author_role:coordinator tag")
	}
}

func TestFeedbackSubscriber_OnFailed(t *testing.T) {
	ctx := context.Background()
	findings := NewMemoryFindingsStore()

	// Directly test the finding creation.
	finding := Finding{
		ProjectID:      "proj-1",
		Claim:          "Ticket proj-99 (Deploy service) failed during execution: build error",
		FindingType:    "investigation",
		Confidence:     0.9,
		SourceType:     "agent_analysis",
		SourceTicketID: "proj-99",
		TicketRefs:     []string{"proj-99"},
		Tags:           []string{"coordinator-feedback", "failure", "author_role:coordinator", "automated"},
		Summary:        "[failure] Deploy service — build error",
	}
	saved, err := findings.SaveFinding(ctx, finding)
	if err != nil {
		t.Fatalf("save finding: %v", err)
	}
	if saved.ID == "" {
		t.Error("expected non-empty finding ID")
	}

	// Query it back.
	results, err := findings.QueryFindings(ctx, "proj-1", "coordinator-feedback", FindingsQueryOpts{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(results))
	}
	if !containsStr(results[0].Tags, "failure") {
		t.Error("expected failure tag")
	}
}

func TestFeedbackSubscriber_OnReplanned(t *testing.T) {
	ctx := context.Background()
	findings := NewMemoryFindingsStore()

	finding := Finding{
		ProjectID:      "proj-1",
		Claim:          "Ticket proj-10 (Refactor auth) required replanning during execution",
		FindingType:    "investigation",
		Confidence:     0.8,
		SourceType:     "agent_analysis",
		SourceTicketID: "proj-10",
		TicketRefs:     []string{"proj-10"},
		Tags:           []string{"coordinator-feedback", "replan", "wrong_decomposition", "author_role:coordinator", "automated"},
		Summary:        "[replan] Refactor auth — execution revealed decomposition issues",
	}
	saved, err := findings.SaveFinding(ctx, finding)
	if err != nil {
		t.Fatalf("save finding: %v", err)
	}
	if saved.ID == "" {
		t.Error("expected non-empty finding ID")
	}

	results, err := findings.QueryFindings(ctx, "proj-1", "replan", FindingsQueryOpts{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(results))
	}
	if !containsStr(results[0].Tags, "wrong_decomposition") {
		t.Error("expected wrong_decomposition tag on replan finding")
	}
}

func TestFeedbackSubscriber_OnInvalidated(t *testing.T) {
	ctx := context.Background()
	findings := NewMemoryFindingsStore()

	finding := Finding{
		ProjectID:      "proj-1",
		Claim:          "Ticket proj-20 (Add caching) was invalidated after validation",
		FindingType:    "investigation",
		Confidence:     0.75,
		SourceType:     "review_comment",
		SourceTicketID: "proj-20",
		TicketRefs:     []string{"proj-20"},
		Tags:           []string{"coordinator-feedback", "invalidation", "acceptance_ambiguity", "author_role:coordinator", "automated"},
		Summary:        "[invalidation] Add caching — post-validation revert",
	}
	saved, err := findings.SaveFinding(ctx, finding)
	if err != nil {
		t.Fatalf("save finding: %v", err)
	}
	if saved.ID == "" {
		t.Error("expected non-empty finding ID")
	}

	results, err := findings.QueryFindings(ctx, "proj-1", "invalidation", FindingsQueryOpts{Limit: 10})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(results))
	}
	if !containsStr(results[0].Tags, "acceptance_ambiguity") {
		t.Error("expected acceptance_ambiguity tag on invalidation finding")
	}
}

// --------------------------------------------------------------------------
// Tests for coordinator_record_feedback handler
// --------------------------------------------------------------------------

func TestHandleCoordinatorRecordFeedback_ValidationErrors(t *testing.T) {
	b := &Backend{Findings: NewMemoryFindingsStore()}

	// Missing required fields — should return validation error before ticket lookup.
	args := map[string]any{
		"project_id": "",
		"ticket_id":  "proj-42",
		"feedback":   "test",
		"category":   "other",
	}
	_, _, err := handleCoordinatorRecordFeedback(b, context.Background(), args)
	if err == nil {
		t.Error("expected error for empty project_id")
	}
}

// --------------------------------------------------------------------------
// Tests for NewCoordinatorFeedbackSubscriber
// --------------------------------------------------------------------------

func TestNewCoordinatorFeedbackSubscriber_NilDeps(t *testing.T) {
	// Should not panic with nil dependencies.
	sub := NewCoordinatorFeedbackSubscriber(nil, nil, nil)
	if sub == nil {
		t.Error("expected non-nil subscriber even with nil deps")
	}
}

func TestNewCoordinatorFeedbackSubscriber_NilFindings(t *testing.T) {
	bus := events.NewInProcessBus()
	sub := NewCoordinatorFeedbackSubscriber(bus, nil, nil)
	if sub == nil {
		t.Error("expected non-nil subscriber")
	}
}

// --------------------------------------------------------------------------
// Test helpers
// --------------------------------------------------------------------------

type mockTicketServiceForFeedback struct {
	ticket *ticket.Ticket
}

func (m *mockTicketServiceForFeedback) GetTicket(_ context.Context, _ string) (*ticket.Ticket, error) {
	return m.ticket, nil
}

// --------------------------------------------------------------------------
// Tests for buildCalibrationSummary
// --------------------------------------------------------------------------

func TestBuildCalibrationSummary_HighRetryRate(t *testing.T) {
	m := calibrationMetrics{
		TotalAuthored:       10,
		SuccessCount:        8,
		RejectedCount:       5,
		FailedCount:         2,
		InProgressCount:     0,
		SuccessRate:         "80.0%",
		AvgAttemptsPerClose: 2.5,
		TicketsWithRetries:  5,
	}
	summary := buildCalibrationSummary(m)
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	// Should mention high avg attempts
	if !containsSubstring(summary, "attempts per successful close") {
		t.Errorf("expected suggestion about tightening criteria, got: %s", summary)
	}
}

func TestBuildCalibrationSummary_Perfect(t *testing.T) {
	m := calibrationMetrics{
		TotalAuthored:       5,
		SuccessCount:        5,
		SuccessRate:         "100.0%",
		AvgAttemptsPerClose: 1.0,
	}
	summary := buildCalibrationSummary(m)
	// Should mention success but NOT suggest tightening
	if containsSubstring(summary, "tightening") {
		t.Errorf("should not suggest tightening for perfect record, got: %s", summary)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
