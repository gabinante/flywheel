package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockStepStore struct {
	appendErr     error
	steps         []Step
	agentIDs      []string
	getStepsErr   error
	agentID       string
	getAgentIDErr error
}

func (m *mockStepStore) AppendStep(ctx context.Context, ticketID, agentID string, step Step) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	m.agentIDs = append(m.agentIDs, agentID)
	m.steps = append(m.steps, step)
	return nil
}

func (m *mockStepStore) GetStepsByTicketID(ctx context.Context, ticketID string) ([]Step, error) {
	if m.getStepsErr != nil {
		return nil, m.getStepsErr
	}
	return m.steps, nil
}

func (m *mockStepStore) GetAgentIDByTicketID(ctx context.Context, ticketID string) (string, error) {
	if m.getAgentIDErr != nil {
		return "", m.getAgentIDErr
	}
	return m.agentID, nil
}

type mockLeaseValidator struct {
	agentID string
	err     error
}

func (m *mockLeaseValidator) ValidateLease(ctx context.Context, ticketID, token string) (string, error) {
	return m.agentID, m.err
}

func TestService_LogStep_ValidLease_SetsCreatedAt(t *testing.T) {
	store := &mockStepStore{}
	validate := &mockLeaseValidator{agentID: "agent1"}
	svc := NewService(store, validate)
	ctx := context.Background()

	step := Step{Type: StepTypeToolCall, Payload: map[string]any{"name": "write"}}
	if step.CreatedAt.IsZero() == false {
		t.Fatal("step.CreatedAt should be zero before LogStep")
	}
	err := svc.LogStep(ctx, "ticket1", "token1", step)
	if err != nil {
		t.Fatalf("LogStep: %v", err)
	}
	if len(store.steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(store.steps))
	}
	got := store.steps[0]
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set by LogStep")
	}
	if got.Type != StepTypeToolCall {
		t.Errorf("Type: got %q", got.Type)
	}
}

func TestService_LogStep_InvalidLease_ReturnsError(t *testing.T) {
	store := &mockStepStore{}
	validate := &mockLeaseValidator{err: errors.New("lease invalid")}
	svc := NewService(store, validate)
	ctx := context.Background()

	err := svc.LogStep(ctx, "ticket1", "bad", Step{Type: StepTypeObservation})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(store.steps) != 0 {
		t.Errorf("expected no step appended, got %d", len(store.steps))
	}
}

func TestService_AppendSystemStep_SetsCreatedAtWithoutLease(t *testing.T) {
	store := &mockStepStore{}
	svc := NewService(store, &mockLeaseValidator{})
	ctx := context.Background()

	step := Step{Type: StepTypeObservation, Payload: map[string]any{"message": "worker output"}}
	if err := svc.AppendSystemStep(ctx, "ticket1", step); err != nil {
		t.Fatalf("AppendSystemStep: %v", err)
	}
	if len(store.steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(store.steps))
	}
	if got := store.steps[0]; got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should be set by AppendSystemStep")
	}
	if len(store.agentIDs) != 1 || store.agentIDs[0] != "" {
		t.Fatalf("expected AppendSystemStep to append with an empty agent id, got %v", store.agentIDs)
	}
}

func TestService_GetTrace_ReturnsStepsInOrder(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStepStore{
		steps: []Step{
			{ID: "1", Type: StepTypeToolCall, Payload: map[string]any{"a": "1"}, CreatedAt: now},
			{ID: "2", Type: StepTypeObservation, Payload: map[string]any{"b": "2"}, CreatedAt: now.Add(time.Second)},
		},
		agentID: "agent1",
	}
	svc := NewService(store, &mockLeaseValidator{})
	ctx := context.Background()

	trace, err := svc.GetTrace(ctx, "ticket1")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if trace.TicketID != "ticket1" {
		t.Errorf("TicketID: got %q", trace.TicketID)
	}
	if trace.AgentID != "agent1" {
		t.Errorf("AgentID: got %q", trace.AgentID)
	}
	if len(trace.Steps) != 2 {
		t.Fatalf("Steps: got %d", len(trace.Steps))
	}
	if trace.Steps[0].ID != "1" || trace.Steps[1].ID != "2" {
		t.Errorf("Steps order: got %v", trace.Steps)
	}
}

func TestService_GetTrace_StoreError(t *testing.T) {
	store := &mockStepStore{getStepsErr: errors.New("db error")}
	svc := NewService(store, &mockLeaseValidator{})
	ctx := context.Background()

	_, err := svc.GetTrace(ctx, "ticket1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestService_SummarizeTrace_AttemptSummaryFromLastStep(t *testing.T) {
	now := time.Now().UTC()
	store := &mockStepStore{
		steps: []Step{
			{Type: StepTypeToolCall, Payload: map[string]any{}, CreatedAt: now},
			{Type: StepTypeObservation, Payload: map[string]any{"summary": "Done refactor"}, CreatedAt: now.Add(time.Second)},
		},
		agentID: "agent1",
	}
	svc := NewService(store, &mockLeaseValidator{})
	ctx := context.Background()

	sum, err := svc.SummarizeTrace(ctx, "ticket1")
	if err != nil {
		t.Fatalf("SummarizeTrace: %v", err)
	}
	if sum.AgentID != "agent1" {
		t.Errorf("AgentID: got %q", sum.AgentID)
	}
	if sum.Outcome != string(StepTypeObservation) {
		t.Errorf("Outcome: got %q", sum.Outcome)
	}
	if sum.Summary != "Done refactor" {
		t.Errorf("Summary: got %q", sum.Summary)
	}
	if sum.CreatedAt != now.Add(time.Second).Format(time.RFC3339) {
		t.Errorf("CreatedAt: got %q", sum.CreatedAt)
	}
}

func TestService_SummarizeTrace_NoSteps_ReturnsNil(t *testing.T) {
	store := &mockStepStore{steps: nil}
	svc := NewService(store, &mockLeaseValidator{})
	ctx := context.Background()

	sum, err := svc.SummarizeTrace(ctx, "ticket1")
	if err != nil {
		t.Fatalf("SummarizeTrace: %v", err)
	}
	if sum != nil {
		t.Errorf("expected nil summary, got %+v", sum)
	}
}

func TestService_SummarizeTrace_StoreError(t *testing.T) {
	store := &mockStepStore{getStepsErr: errors.New("db error")}
	svc := NewService(store, &mockLeaseValidator{})
	ctx := context.Background()

	_, err := svc.SummarizeTrace(ctx, "ticket1")
	if err == nil {
		t.Fatal("expected error")
	}
}
