package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// Event type constants for plan lifecycle.
const (
	EventPlanCreated    = "plan.created"
	EventPlanSubmitted  = "plan.submitted"
	EventPlanClassified = "plan.classified"
	EventPlanApproved   = "plan.approved"
	EventPlanApplied    = "plan.applied"
	EventPlanRejected   = "plan.rejected"
	EventPlanSuperseded = "plan.superseded"
)

// Service provides plan operations with content validation and lifecycle management.
type Service struct {
	store *Store
	bus   events.Bus
}

// NewService returns a new plan Service.
func NewService(store *Store, bus events.Bus) *Service {
	return &Service{store: store, bus: bus}
}

// CreatePlan creates a new plan for a ticket. Content is validated at schema level
// before persistence — invalid content (e.g., unparseable DDL) fails immediately.
// If a previous non-terminal plan exists for the same ticket+backend, it is superseded.
func (s *Service) CreatePlan(ctx context.Context, ticketID string, backend Backend, content Content, createdBy string, freshnessStamp time.Time, expiresAt *time.Time) (*Plan, error) {
	// Validate backend
	if !IsValidBackend(backend) {
		return nil, fmt.Errorf("invalid backend: %s", backend)
	}

	// Validate content against backend sub-schema — this is the hard constraint.
	// Agent cannot produce a plan that bypasses classification.
	if err := ValidateContent(backend, content); err != nil {
		return nil, err
	}

	// Generate UUID v7 for the plan
	id := uuid.Must(uuid.NewV7()).String()
	now := time.Now().UTC()

	p := &Plan{
		ID:             id,
		TicketID:       ticketID,
		Backend:        backend,
		State:          StateDraft,
		Version:        1,
		Content:        content,
		FreshnessStamp: freshnessStamp,
		ExpiresAt:      expiresAt,
		CreatedBy:      createdBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Supersede previous plans for same ticket+backend
	if err := s.store.SupersedeByTicketAndBackend(ctx, ticketID, backend, id); err != nil {
		return nil, fmt.Errorf("supersede previous plans: %w", err)
	}

	if err := s.store.Create(ctx, p); err != nil {
		return nil, err
	}

	// Store initial version
	versionID := uuid.Must(uuid.NewV7()).String()
	v := &PlanVersion{
		ID:        versionID,
		PlanID:    id,
		Version:   1,
		Content:   content,
		CreatedBy: createdBy,
		CreatedAt: now,
	}
	if err := s.store.CreateVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("create version: %w", err)
	}

	_ = s.bus.Publish(ctx, events.Event{Type: EventPlanCreated, Payload: map[string]any{
		"plan_id": id, "ticket_id": ticketID, "backend": string(backend),
	}})

	return p, nil
}

// GetPlan returns a plan by ID.
func (s *Service) GetPlan(ctx context.Context, id string) (*Plan, error) {
	return s.store.GetByID(ctx, id)
}

// ListPlansByTicket returns all plans for a ticket.
func (s *Service) ListPlansByTicket(ctx context.Context, ticketID string) ([]*Plan, error) {
	return s.store.ListByTicket(ctx, ticketID)
}

// ListPlansByTicketAndBackend returns plans for a ticket filtered by backend.
func (s *Service) ListPlansByTicketAndBackend(ctx context.Context, ticketID string, backend Backend) ([]*Plan, error) {
	if !IsValidBackend(backend) {
		return nil, fmt.Errorf("invalid backend: %s", backend)
	}
	return s.store.ListByTicketAndBackend(ctx, ticketID, backend)
}

// SubmitPlan transitions a plan from draft to submitted. This triggers content
// validation again to ensure nothing changed between creation and submission.
func (s *Service) SubmitPlan(ctx context.Context, id string) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.State != StateDraft {
		return fmt.Errorf("plan must be in draft state to submit (current: %s)", p.State)
	}

	// Re-validate content at submission
	if err := ValidateContent(p.Backend, p.Content); err != nil {
		return err
	}

	if err := s.store.UpdateState(ctx, id, p.Version, StateSubmitted); err != nil {
		return err
	}

	_ = s.bus.Publish(ctx, events.Event{Type: EventPlanSubmitted, Payload: map[string]any{
		"plan_id": id, "ticket_id": p.TicketID, "backend": string(p.Backend),
	}})
	return nil
}

// ClassifyPlan transitions a plan from submitted to classified (classifier confirmed it).
func (s *Service) ClassifyPlan(ctx context.Context, id string) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.State != StateSubmitted {
		return fmt.Errorf("plan must be in submitted state to classify (current: %s)", p.State)
	}

	if err := s.store.UpdateState(ctx, id, p.Version, StateClassified); err != nil {
		return err
	}

	_ = s.bus.Publish(ctx, events.Event{Type: EventPlanClassified, Payload: map[string]any{
		"plan_id": id, "ticket_id": p.TicketID, "backend": string(p.Backend),
	}})
	return nil
}

// ApprovePlan transitions a plan from classified to approved.
func (s *Service) ApprovePlan(ctx context.Context, id string) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.State != StateClassified {
		return fmt.Errorf("plan must be in classified state to approve (current: %s)", p.State)
	}

	// Check freshness — if plan is expired, reject the approval
	if p.ExpiresAt != nil && time.Now().UTC().After(*p.ExpiresAt) {
		return fmt.Errorf("plan has expired (freshness_stamp: %s, expires_at: %s) — re-plan required", p.FreshnessStamp, p.ExpiresAt)
	}

	if err := s.store.UpdateState(ctx, id, p.Version, StateApproved); err != nil {
		return err
	}

	_ = s.bus.Publish(ctx, events.Event{Type: EventPlanApproved, Payload: map[string]any{
		"plan_id": id, "ticket_id": p.TicketID, "backend": string(p.Backend),
	}})
	return nil
}

// ApplyPlan transitions a plan from approved to applied.
func (s *Service) ApplyPlan(ctx context.Context, id string) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.State != StateApproved {
		return fmt.Errorf("plan must be in approved state to apply (current: %s)", p.State)
	}

	// Final freshness check before apply
	if p.ExpiresAt != nil && time.Now().UTC().After(*p.ExpiresAt) {
		return fmt.Errorf("plan has expired before apply — re-plan required (see warrant-45)")
	}

	if err := s.store.UpdateState(ctx, id, p.Version, StateApplied); err != nil {
		return err
	}

	_ = s.bus.Publish(ctx, events.Event{Type: EventPlanApplied, Payload: map[string]any{
		"plan_id": id, "ticket_id": p.TicketID, "backend": string(p.Backend),
	}})
	return nil
}

// RejectPlan transitions a plan to rejected from any non-terminal state.
func (s *Service) RejectPlan(ctx context.Context, id string) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.State == StateApplied || p.State == StateSuperseded || p.State == StateRejected {
		return fmt.Errorf("plan in terminal state %s cannot be rejected", p.State)
	}

	if err := s.store.UpdateState(ctx, id, p.Version, StateRejected); err != nil {
		return err
	}

	_ = s.bus.Publish(ctx, events.Event{Type: EventPlanRejected, Payload: map[string]any{
		"plan_id": id, "ticket_id": p.TicketID, "backend": string(p.Backend),
	}})
	return nil
}

// UpdateContent updates a plan's content (only in draft state). Validates the new content
// against the backend sub-schema and creates a new version record.
func (s *Service) UpdateContent(ctx context.Context, id string, content Content, updatedBy string) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.State != StateDraft {
		return fmt.Errorf("plan content can only be updated in draft state (current: %s)", p.State)
	}

	// Validate new content against backend sub-schema
	if err := ValidateContent(p.Backend, content); err != nil {
		return err
	}

	if err := s.store.UpdateContent(ctx, id, p.Version, content); err != nil {
		return err
	}

	// Create version record
	versionID := uuid.Must(uuid.NewV7()).String()
	v := &PlanVersion{
		ID:        versionID,
		PlanID:    id,
		Version:   p.Version + 1,
		Content:   content,
		CreatedBy: updatedBy,
		CreatedAt: time.Now().UTC(),
	}
	return s.store.CreateVersion(ctx, v)
}

// UpdateFreshness updates the freshness stamp and expiry for a plan.
func (s *Service) UpdateFreshness(ctx context.Context, id string, freshnessStamp time.Time, expiresAt *time.Time) error {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if p.State == StateApplied || p.State == StateSuperseded || p.State == StateRejected {
		return fmt.Errorf("cannot update freshness on terminal plan (state: %s)", p.State)
	}
	return s.store.UpdateFreshness(ctx, id, freshnessStamp, expiresAt)
}

// GetVersions returns the version history for a plan.
func (s *Service) GetVersions(ctx context.Context, planID string) ([]*PlanVersion, error) {
	return s.store.ListVersions(ctx, planID)
}

// IsFresh checks whether a plan's freshness is still valid (not expired).
func (s *Service) IsFresh(ctx context.Context, id string) (bool, error) {
	p, err := s.store.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	if p.ExpiresAt == nil {
		return true, nil // No expiry set — always fresh
	}
	return time.Now().UTC().Before(*p.ExpiresAt), nil
}
