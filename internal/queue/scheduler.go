package queue

import (
	"context"
	"log"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/ticket"
)

// Scheduler runs background jobs: expire leases and react to ticket.done for unblocked.
// Supports both legacy Bus and DurableEventBus for at-least-once event delivery.
type Scheduler struct {
	redis       *RedisStore
	ticketSvc   TicketTransitioner
	ticketList  TicketListerForQueue
	bus         events.Bus
	durableBus  events.DurableEventBus // nil if bus doesn't support durability
	pollInterval time.Duration
	batchSize   int64
}

// NewScheduler returns a new Scheduler.
func NewScheduler(redis *RedisStore, ticketSvc TicketTransitioner, ticketList TicketListerForQueue, bus events.Bus, pollInterval time.Duration) *Scheduler {
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}
	s := &Scheduler{
		redis:        redis,
		ticketSvc:     ticketSvc,
		ticketList:    ticketList,
		bus:           bus,
		pollInterval:  pollInterval,
		batchSize:    50,
	}
	// Detect if bus supports durable event delivery.
	if durable, ok := bus.(events.DurableEventBus); ok {
		s.durableBus = durable
	}
	return s
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
		}
	}
}

func (s *Scheduler) expireLeases(ctx context.Context) {
	ids, err := s.redis.GetExpiredLeaseTicketIDs(ctx, s.batchSize)
	if err != nil {
		log.Printf("queue/scheduler: get expired leases: %v", err)
		return
	}
	actor := ticket.Actor{ID: "system", Type: ticket.ActorSystem}
	for _, id := range ids {
		if err := s.ticketSvc.TransitionTicket(ctx, id, ticket.TriggerLeaseExpired, actor, nil); err != nil {
			log.Printf("queue/scheduler: transition lease_expired %s: %v", id, err)
			continue
		}
		if err := s.redis.RemoveExpired(ctx, id); err != nil {
			log.Printf("queue/scheduler: remove expired lease %s: %v", id, err)
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
