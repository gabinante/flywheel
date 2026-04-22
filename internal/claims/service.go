package claims

import (
	"context"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// Event type constants for claims lifecycle.
const (
	EventClaimRegistered  = "claim.registered"
	EventClaimReleased    = "claim.released"
	EventConflictDetected = "claim.conflict_detected"
	EventConflictResolved = "claim.conflict_resolved"
)

// Service provides claims registry operations: registration, release, and conflict detection.
type Service struct {
	store *Store
	bus   events.Bus
}

// NewService creates a new claims service.
func NewService(store *Store, bus events.Bus) *Service {
	return &Service{
		store: store,
		bus:   bus,
	}
}

// RegisterClaims registers claims for a ticket entering execution.
// Each touch from the plan becomes an active claim.
func (s *Service) RegisterClaims(ctx context.Context, ticketID string, touches []Touch) ([]*Claim, error) {
	now := time.Now().UTC()
	var registered []*Claim

	for _, t := range touches {
		claim := &Claim{
			ID:          uuid.New().String(),
			TicketID:    ticketID,
			EntityID:    t.EntityID,
			Environment: t.Environment,
			ClaimType:   t.ClaimType,
			State:       StateActive,
			Metadata:    t.Metadata,
			ClaimedAt:   now,
		}

		if err := s.store.CreateClaim(ctx, claim); err != nil {
			return registered, err
		}
		registered = append(registered, claim)

		_ = s.bus.Publish(ctx, events.Event{
			Type: EventClaimRegistered,
			Payload: map[string]any{
				"claim_id":    claim.ID,
				"ticket_id":   ticketID,
				"entity_id":   t.EntityID,
				"environment": t.Environment,
				"claim_type":  string(t.ClaimType),
			},
		})
	}

	return registered, nil
}

// ReleaseClaims releases all active claims for a ticket (on completion or abandonment).
func (s *Service) ReleaseClaims(ctx context.Context, ticketID string) (int, error) {
	now := time.Now().UTC()
	count, err := s.store.ReleaseByTicket(ctx, ticketID, now)
	if err != nil {
		return 0, err
	}

	// Resolve any conflicts that were blocked by this ticket.
	resolved, _ := s.store.ResolveConflictsByTicket(ctx, ticketID, now)

	if count > 0 {
		_ = s.bus.Publish(ctx, events.Event{
			Type: EventClaimReleased,
			Payload: map[string]any{
				"ticket_id":          ticketID,
				"claims_released":    count,
				"conflicts_resolved": resolved,
			},
		})
	}

	return count, nil
}

// DetectConflicts checks whether the given touches conflict with any active claims.
// This is called at ticket creation time and again at execution dispatch time.
// It does NOT persist conflicts — use DetectAndRecordConflicts for that.
func (s *Service) DetectConflicts(ctx context.Context, ticketID string, touches []Touch) (*ConflictResult, error) {
	result := &ConflictResult{
		ParallelSafe: true,
	}

	for _, t := range touches {
		activeClaims, err := s.store.GetActiveByEntity(ctx, t.EntityID, t.Environment)
		if err != nil {
			return nil, err
		}

		for _, existing := range activeClaims {
			// Don't conflict with yourself
			if existing.TicketID == ticketID {
				continue
			}

			conflictType, severity := classifyConflict(t, existing)
			if conflictType == ConflictDisjoint {
				continue
			}

			conflict := Conflict{
				ID:             uuid.New().String(),
				TicketID:       ticketID,
				BlockingTicket: existing.TicketID,
				ClaimID:        existing.ID,
				ConflictType:   conflictType,
				Severity:       severity,
				DetectedAt:     time.Now().UTC(),
			}

			result.Conflicts = append(result.Conflicts, conflict)
			result.HasConflicts = true
			result.ParallelSafe = false

			if severity == SeverityHard {
				result.HardCount++
			} else {
				result.SoftCount++
			}
		}
	}

	return result, nil
}

// DetectAndRecordConflicts detects conflicts and persists them to the database.
// Used at execution dispatch time when conflicts should be tracked.
func (s *Service) DetectAndRecordConflicts(ctx context.Context, ticketID string, touches []Touch) (*ConflictResult, error) {
	result, err := s.DetectConflicts(ctx, ticketID, touches)
	if err != nil {
		return nil, err
	}

	// Persist detected conflicts
	for i := range result.Conflicts {
		if err := s.store.CreateConflict(ctx, &result.Conflicts[i]); err != nil {
			return result, err
		}

		_ = s.bus.Publish(ctx, events.Event{
			Type: EventConflictDetected,
			Payload: map[string]any{
				"conflict_id":    result.Conflicts[i].ID,
				"ticket_id":     ticketID,
				"blocking_ticket": result.Conflicts[i].BlockingTicket,
				"conflict_type":  string(result.Conflicts[i].ConflictType),
				"severity":       string(result.Conflicts[i].Severity),
			},
		})
	}

	return result, nil
}

// GetActiveClaims returns all active claims for a ticket.
func (s *Service) GetActiveClaims(ctx context.Context, ticketID string) ([]*Claim, error) {
	return s.store.GetActiveByTicket(ctx, ticketID)
}

// GetActiveClaimsByEntity returns active claims for an entity+environment.
func (s *Service) GetActiveClaimsByEntity(ctx context.Context, entityID, environment string) ([]*Claim, error) {
	return s.store.GetActiveByEntity(ctx, entityID, environment)
}

// GetActiveClaimsByEnvironment returns all active claims in an environment.
func (s *Service) GetActiveClaimsByEnvironment(ctx context.Context, environment string) ([]*Claim, error) {
	return s.store.GetActiveByEnvironment(ctx, environment)
}

// GetUnresolvedConflicts returns unresolved conflicts for a ticket.
func (s *Service) GetUnresolvedConflicts(ctx context.Context, ticketID string) ([]*Conflict, error) {
	return s.store.GetUnresolvedConflicts(ctx, ticketID)
}

// classifyConflict determines the conflict type and severity between a touch and an existing claim.
func classifyConflict(touch Touch, existing *Claim) (ConflictType, Severity) {
	// Same entity, same environment — classify based on claim types
	switch {
	case touch.ClaimType == ClaimFileWrite && existing.ClaimType == ClaimFileWrite:
		// Two tickets writing to the same file — hard conflict
		return ConflictSameFileWrite, SeverityHard

	case touch.ClaimType == ClaimSymbol && existing.ClaimType == ClaimSymbol:
		// Two tickets modifying the same symbol — hard conflict
		return ConflictSameSymbol, SeverityHard

	case touch.ClaimType == ClaimService && existing.ClaimType == ClaimService:
		// Two tickets touching the same service — soft conflict (advisory)
		return ConflictSameService, SeveritySoft

	case touch.ClaimType == ClaimSchema && existing.ClaimType == ClaimSchema:
		// Two tickets modifying the same schema — hard conflict
		return ConflictSameSchema, SeverityHard

	case touch.ClaimType == ClaimDeployTarget && existing.ClaimType == ClaimDeployTarget:
		// Two tickets deploying to the same target — hard conflict
		return ConflictSameFileWrite, SeverityHard

	case touch.ClaimType == ClaimResource && existing.ClaimType == ClaimResource:
		// Two tickets touching the same resource — check if same type
		return ConflictSameService, SeveritySoft

	default:
		// Different claim types on same entity — generally disjoint
		// e.g., one writes a file, another deploys — no direct conflict
		return ConflictDisjoint, SeveritySoft
	}
}
