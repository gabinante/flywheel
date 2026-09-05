package queue

import (
	"context"
	"errors"
	"time"

	"github.com/gabinante/flywheel/internal/ticket"
)

var ErrNoTicketAvailable = errors.New("no ticket available to claim")

// TicketTransitioner performs state transitions (ticket.Service).
type TicketTransitioner interface {
	TransitionTicket(ctx context.Context, id string, trigger string, actor ticket.Actor, payload map[string]any) error
}

// TicketListerForQueue lists pending tickets and fetches tickets by ID (ticket.Service).
type TicketListerForQueue interface {
	ListByState(ctx context.Context, projectID string, state ticket.State) ([]*ticket.Ticket, error)
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
	GetTicketsByIDs(ctx context.Context, ids []string) ([]*ticket.Ticket, error)
}

// Service provides queue operations (claim, renew, release).
type Service struct {
	ticketSvc  TicketTransitioner
	ticketList TicketListerForQueue
	leases     LeaseStore
}

// NewService returns a new queue Service. The leases parameter accepts any LeaseStore
// implementation (RedisStore for production, miniredis-backed for embedded, etc.).
func NewService(ticketSvc TicketTransitioner, ticketList TicketListerForQueue, leases LeaseStore) *Service {
	return &Service{
		ticketSvc:  ticketSvc,
		ticketList: ticketList,
		leases:     leases,
	}
}

// ClaimTicket finds the highest-priority unblocked pending ticket, creates a lease in Redis, and transitions to claimed.
// If idempotencyKey is non-empty: (1) if this agent already has a valid lease on the ticket from a previous claim with this key, renew and return it; (2) else if the previously claimed ticket is pending again, re-claim that ticket; (3) else claim the next available ticket and record the key.
func (s *Service) ClaimTicket(ctx context.Context, agentID, projectID string, priority *int, idempotencyKey string) (*ticket.Ticket, *Lease, error) {
	if idempotencyKey != "" {
		if prevID, _ := s.leases.GetClaimIdempotencyTicketID(ctx, projectID, agentID, idempotencyKey); prevID != "" {
			t, err := s.ticketList.GetTicket(ctx, prevID)
			if err != nil || t == nil || t.ProjectID != projectID {
				// Stale or wrong project; fall through to normal claim
			} else {
				leaseData, err := s.leases.GetLease(ctx, prevID)
				if err == nil && leaseData != nil && leaseData.AgentID == agentID {
					// Same agent still has lease: renew and return
					newExpiresAt, err := s.leases.RenewLease(ctx, prevID, leaseData.Token, s.leases.TTL())
					if err == nil {
						t, _ = s.ticketList.GetTicket(ctx, prevID)
						lease := &Lease{TicketID: prevID, AgentID: agentID, Token: leaseData.Token, ExpiresAt: newExpiresAt, Renewable: true}
						return t, lease, nil
					}
				}
				// Ticket may be pending again (lease expired); try to claim this specific ticket
				if t.State == ticket.StateDraft {
					deps, _ := s.ticketList.GetTicketsByIDs(ctx, t.DependsOn)
					if ticket.IsUnblocked(t, deps) && (priority == nil || int(t.Priority) == *priority) {
						t, lease, err := s.claimTicketByID(ctx, projectID, prevID, agentID)
						if err == nil {
							_ = s.leases.SetClaimIdempotency(ctx, projectID, agentID, idempotencyKey, prevID)
							return t, lease, nil
						}
					}
				}
			}
		}
	}

	t, lease, err := s.claimNextTicket(ctx, agentID, projectID, priority)
	if err != nil {
		return nil, nil, err
	}
	if idempotencyKey != "" && t != nil {
		_ = s.leases.SetClaimIdempotency(ctx, projectID, agentID, idempotencyKey, t.ID)
	}
	return t, lease, nil
}

// claimTicketByID claims a specific ticket if it is pending and unblocked. Caller must ensure project and priority match.
func (s *Service) claimTicketByID(ctx context.Context, projectID, ticketID, agentID string) (*ticket.Ticket, *Lease, error) {
	t, err := s.ticketList.GetTicket(ctx, ticketID)
	if err != nil || t == nil || t.ProjectID != projectID || t.State != ticket.StateDraft {
		return nil, nil, ErrNoTicketAvailable
	}
	deps, err := s.ticketList.GetTicketsByIDs(ctx, t.DependsOn)
	if err != nil || !ticket.IsUnblocked(t, deps) {
		return nil, nil, ErrNoTicketAvailable
	}
	token, expiresAt, err := s.leases.CreateLease(ctx, t.ID, agentID)
	if err != nil {
		return nil, nil, err
	}
	actor := ticket.Actor{ID: agentID, Type: ticket.ActorAgent}
	payload := map[string]any{"agent_id": agentID}
	if err := s.ticketSvc.TransitionTicket(ctx, t.ID, ticket.TriggerClaim, actor, payload); err != nil {
		_ = s.leases.ReleaseLease(ctx, t.ID, token)
		return nil, nil, err
	}
	t, _ = s.ticketList.GetTicket(ctx, t.ID)
	lease := &Lease{TicketID: t.ID, AgentID: agentID, Token: token, ExpiresAt: expiresAt, Renewable: true}
	return t, lease, nil
}

// claimNextTicket finds the next available pending ticket and claims it.
func (s *Service) claimNextTicket(ctx context.Context, agentID, projectID string, priority *int) (*ticket.Ticket, *Lease, error) {
	list, err := s.ticketList.ListByState(ctx, projectID, ticket.StateDraft)
	if err != nil {
		return nil, nil, err
	}
	var candidate *ticket.Ticket
	for _, t := range list {
		deps, err := s.ticketList.GetTicketsByIDs(ctx, t.DependsOn)
		if err != nil {
			continue
		}
		if !ticket.IsUnblocked(t, deps) {
			continue
		}
		if priority != nil && int(t.Priority) != *priority {
			continue
		}
		candidate = t
		break
	}
	if candidate == nil {
		return nil, nil, ErrNoTicketAvailable
	}
	return s.claimTicketByID(ctx, projectID, candidate.ID, agentID)
}

// RenewLease extends the lease TTL. Returns new expiry or error.
func (s *Service) RenewLease(ctx context.Context, ticketID, token string) (newExpiresAt time.Time, err error) {
	return s.leases.RenewLease(ctx, ticketID, token, s.leases.TTL())
}

// ReleaseLease removes the lease and transitions the ticket back to pending.
func (s *Service) ReleaseLease(ctx context.Context, ticketID, token string) error {
	if err := s.leases.ReleaseLease(ctx, ticketID, token); err != nil {
		return err
	}
	actor := ticket.Actor{ID: "system", Type: ticket.ActorSystem}
	return s.ticketSvc.TransitionTicket(ctx, ticketID, ticket.TriggerLeaseExpired, actor, nil)
}

// CleanupLease removes a stale lease entry from Redis without transitioning the ticket.
// Used after submit/escalate where the ticket has already advanced past lease-tracked states.
func (s *Service) CleanupLease(ctx context.Context, ticketID string) error {
	return s.leases.RemoveExpired(ctx, ticketID)
}

// ForceReleaseLease removes the lease from Redis and forces the ticket back to pending (system actor).
// Use when the user has confirmed via elicitation; no lease token required.
func (s *Service) ForceReleaseLease(ctx context.Context, ticketID string) error {
	_ = s.leases.RemoveExpired(ctx, ticketID)
	actor := ticket.Actor{ID: "operator", Type: ticket.ActorSystem}
	return s.ticketSvc.TransitionTicket(ctx, ticketID, ticket.TriggerLeaseExpired, actor, nil)
}

func (s *Service) ClaimTicketByID(ctx context.Context, agentID, projectID, ticketID string) (*ticket.Ticket, *Lease, error) {
	if existing, err := s.leases.GetLease(ctx, ticketID); err == nil && existing != nil && existing.AgentID == agentID {
		t, err := s.ticketList.GetTicket(ctx, ticketID)
		if err != nil || t == nil || t.ProjectID != projectID {
			return nil, nil, ErrNoTicketAvailable
		}
		if t.State == ticket.StateDraft {
			// The next workflow phase needs a fresh claim and lease. A leftover
			// lease from the submitted phase must not return an unclaimed draft.
			if err := s.leases.ReleaseLease(ctx, ticketID, existing.Token); err != nil {
				return nil, nil, err
			}
			return s.claimTicketByID(ctx, projectID, ticketID, agentID)
		}
		if t.State != ticket.StatePlanning && t.State != ticket.StateExecuting {
			return nil, nil, ErrNoTicketAvailable
		}
		expires, err := s.leases.RenewLease(ctx, ticketID, existing.Token, s.leases.TTL())
		if err != nil {
			return nil, nil, err
		}
		return t, &Lease{TicketID: ticketID, AgentID: agentID, Token: existing.Token, ExpiresAt: expires, Renewable: true}, nil
	}
	return s.claimTicketByID(ctx, projectID, ticketID, agentID)
}
