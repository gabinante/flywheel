package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// StaleTicketLister finds tickets stuck in active states beyond a threshold.
// Implemented by ticket.Service (backed by Postgres or SQLite).
type StaleTicketLister interface {
	ListStaleTickets(ctx context.Context, states []ticket.State, threshold time.Duration) ([]*ticket.Ticket, error)
}

// Scheduler runs background jobs: expire leases and react to ticket.done for unblocked.
// Supports both legacy Bus and DurableEventBus for at-least-once event delivery.
type Scheduler struct {
	leases             LeaseStore
	ticketSvc          TicketTransitioner
	ticketList         TicketListerForQueue
	bus                events.Bus
	durableBus         events.DurableEventBus // nil if bus doesn't support durability
	pollInterval       time.Duration
	batchSize          int64
	staleLister        StaleTicketLister
	stalenessThreshold time.Duration
}

// NewScheduler returns a new Scheduler. The leases parameter accepts any LeaseStore implementation.
func NewScheduler(leases LeaseStore, ticketSvc TicketTransitioner, ticketList TicketListerForQueue, bus events.Bus, pollInterval time.Duration) *Scheduler {
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}
	s := &Scheduler{
		leases:       leases,
		ticketSvc:    ticketSvc,
		ticketList:   ticketList,
		bus:          bus,
		pollInterval: pollInterval,
		batchSize:    50,
	}
	// Detect if bus supports durable event delivery.
	if durable, ok := bus.(events.DurableEventBus); ok {
		s.durableBus = durable
	}
	return s
}

// EnableStalenessSwitch enables the DB staleness sweep (Layer 3 recovery).
// The sweep periodically scans for tickets stuck in planning/executing state with
// updated_at older than the staleness threshold, and transitions them back to draft
// via TriggerLeaseExpired.
//
// This is the last-resort backstop for cases where both the dispatcher (Layer 1) and
// Redis lease expiry (Layer 2) fail — e.g. Redis restart loses lease data while the
// server is also down. The sweep is grounded in the database, not ephemeral Redis state.
//
// The stalenessThreshold should be conservative (default: 2x lease TTL) to avoid
// reclaiming tickets that are actively being worked on.
func (s *Scheduler) EnableStalenessSweep(lister StaleTicketLister, stalenessThreshold time.Duration) {
	if stalenessThreshold <= 0 {
		stalenessThreshold = 2 * s.leases.TTL()
	}
	s.staleLister = lister
	s.stalenessThreshold = stalenessThreshold
}

// Run starts the scheduler (blocking). Call in a goroutine.
func (s *Scheduler) Run(ctx context.Context) {
	s.subscribeTicketDone(ctx)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.expireLeases(ctx)
			s.sweepStaleTickets(ctx)
		}
	}
}

func (s *Scheduler) expireLeases(ctx context.Context) {
	ids, err := s.leases.GetExpiredLeaseTicketIDs(ctx, s.batchSize)
	if err != nil {
		slog.Error("queue/scheduler: get expired leases failed", "error", err)
		return
	}
	actor := ticket.Actor{ID: "system", Type: ticket.ActorSystem}
	for _, id := range ids {
		if err := s.ticketSvc.TransitionTicket(ctx, id, ticket.TriggerLeaseExpired, actor, nil); err != nil {
			slog.Error("queue/scheduler: transition lease_expired failed", "ticket", id, "error", err)
			continue
		}
		if err := s.leases.RemoveExpired(ctx, id); err != nil {
			slog.Error("queue/scheduler: remove expired lease failed", "ticket", id, "error", err)
		}
	}
}

// sweepStaleTickets is the Layer 3 DB staleness sweep. It queries Postgres for tickets
// stuck in planning or executing state with updated_at older than the staleness threshold,
// then transitions them back to draft via TriggerLeaseExpired.
//
// Idempotent: running twice on the same stale ticket is safe because the first run
// transitions it to draft, and the second run won't find it (it's no longer in
// planning/executing state). If the transition fails (e.g. version conflict), the error
// is logged and the ticket is retried on the next sweep cycle.
func (s *Scheduler) sweepStaleTickets(ctx context.Context) {
	if s.staleLister == nil {
		return
	}
	staleStates := []ticket.State{ticket.StatePlanning, ticket.StateExecuting}
	stale, err := s.staleLister.ListStaleTickets(ctx, staleStates, s.stalenessThreshold)
	if err != nil {
		slog.Error("queue/scheduler: staleness sweep query failed", "error", err)
		return
	}
	if len(stale) == 0 {
		return
	}
	actor := ticket.Actor{ID: "staleness-sweep", Type: ticket.ActorSystem}
	for _, t := range stale {
		slog.Warn("queue/scheduler: staleness sweep recovering zombie ticket", "ticket", t.ID, "state", string(t.State), "updated_at", t.UpdatedAt.Format(time.RFC3339), "threshold", s.stalenessThreshold.String())
		if err := s.ticketSvc.TransitionTicket(ctx, t.ID, ticket.TriggerLeaseExpired, actor, nil); err != nil {
			slog.Error("queue/scheduler: staleness sweep transition failed", "ticket", t.ID, "error", err)
			continue
		}
		// Also clean up any stale Redis lease data for this ticket.
		if err := s.leases.RemoveExpired(ctx, t.ID); err != nil {
			slog.Error("queue/scheduler: staleness sweep remove lease failed", "ticket", t.ID, "error", err)
		}
	}
}

func (s *Scheduler) subscribeTicketDone(ctx context.Context) {
	handler := func(ctx context.Context, ev events.Event) {
		ticketID, _ := ev.Payload["ticket_id"].(string)
		if ticketID == "" {
			return
		}
		// Find tickets that depend on this one and are pending; if now unblocked, emit ticket.unblocked
		t, err := s.ticketList.GetTicket(ctx, ticketID)
		if err != nil {
			return
		}
		// We need to find all tickets in any project that have ticketID in DependsOn. We don't have a global index.
		// So we subscribe to ticket.done and have project_id in payload, then list pending for that project and check deps.
		projectID, _ := ev.Payload["project_id"].(string)
		if projectID == "" && t != nil {
			projectID = t.ProjectID
		}
		if projectID == "" {
			return
		}
		pending, err := s.ticketList.ListByState(ctx, projectID, ticket.StatePending)
		if err != nil {
			return
		}
		for _, p := range pending {
			hasDep := false
			for _, d := range p.DependsOn {
				if d == ticketID {
					hasDep = true
					break
				}
			}
			if !hasDep {
				continue
			}
			deps, err := s.ticketList.GetTicketsByIDs(ctx, p.DependsOn)
			if err != nil {
				continue
			}
			if ticket.IsUnblocked(p, deps) {
				_ = s.bus.Publish(ctx, events.NewEvent(events.EventTicketUnblocked, map[string]any{"ticket_id": p.ID}).WithEntityKey("ticket:"+p.ID))
			}
		}
	}

	// Use pattern subscription if durable bus available; otherwise exact match.
	if s.durableBus != nil {
		_ = s.durableBus.SubscribePattern("ticket.closed", "scheduler:unblock", handler)
	} else {
		s.bus.Subscribe(events.EventTicketDone, handler)
	}
}
