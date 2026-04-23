package stream

import (
	"context"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// Service provides business logic for the three foundational streams.
type Service struct {
	store Store
	bus   events.Bus
}

// NewService creates a new stream service.
func NewService(store Store, bus events.Bus) *Service {
	return &Service{store: store, bus: bus}
}

// --- Entity Stream ---

// AppendEntityEvent appends an event to the entity stream and publishes to the bus.
func (s *Service) AppendEntityEvent(ctx context.Context, event *EntityEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}
	event.CreatedAt = now
	if event.AffectedEntities == nil {
		event.AffectedEntities = []string{event.EntityID}
	}

	if err := s.store.AppendEntity(ctx, event); err != nil {
		return err
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.EventEntityStreamAppended,
		Payload: map[string]any{
			"event_id":    event.ID,
			"project_id":  event.ProjectID,
			"change_type": string(event.ChangeType),
			"entity_id":   event.EntityID,
			"entity_type": string(event.EntityType),
			"entity_name": event.EntityName,
		},
	})
	return nil
}

// GetEntityEvent returns a single entity stream event by ID.
func (s *Service) GetEntityEvent(ctx context.Context, id string) (*EntityEvent, error) {
	return s.store.GetEntity(ctx, id)
}

// ListEntityEvents queries the entity stream with filters.
func (s *Service) ListEntityEvents(ctx context.Context, q StreamQuery) (*StreamPage[EntityEvent], error) {
	return s.store.ListEntity(ctx, q)
}

// --- State Stream ---

// AppendStateEvent appends an event to the state stream and publishes to the bus.
func (s *Service) AppendStateEvent(ctx context.Context, event *StateEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}
	if event.ObservedAt.IsZero() {
		event.ObservedAt = now
	}
	event.CreatedAt = now
	if event.AffectedEntities == nil {
		event.AffectedEntities = []string{event.EntityID}
	}

	if err := s.store.AppendState(ctx, event); err != nil {
		return err
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.EventStateStreamAppended,
		Payload: map[string]any{
			"event_id":    event.ID,
			"project_id":  event.ProjectID,
			"change_type": string(event.ChangeType),
			"entity_id":   event.EntityID,
			"source":      event.Source,
		},
	})
	return nil
}

// GetStateEvent returns a single state stream event by ID.
func (s *Service) GetStateEvent(ctx context.Context, id string) (*StateEvent, error) {
	return s.store.GetState(ctx, id)
}

// ListStateEvents queries the state stream with filters.
func (s *Service) ListStateEvents(ctx context.Context, q StreamQuery) (*StreamPage[StateEvent], error) {
	return s.store.ListState(ctx, q)
}

// --- Change Stream ---

// AppendChangeEvent appends an event to the change stream and publishes to the bus.
func (s *Service) AppendChangeEvent(ctx context.Context, event *ChangeEvent) error {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}
	event.CreatedAt = now
	if event.AffectedEntities == nil {
		event.AffectedEntities = []string{}
	}

	if err := s.store.AppendChange(ctx, event); err != nil {
		return err
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.EventChangeStreamAppended,
		Payload: map[string]any{
			"event_id":           event.ID,
			"project_id":         event.ProjectID,
			"change_type":        event.ChangeType,
			"affected_entities":  event.AffectedEntities,
			"source":             event.Source,
		},
	})
	return nil
}

// GetChangeEvent returns a single change stream event by ID.
func (s *Service) GetChangeEvent(ctx context.Context, id string) (*ChangeEvent, error) {
	return s.store.GetChange(ctx, id)
}

// ListChangeEvents queries the change stream with filters.
func (s *Service) ListChangeEvents(ctx context.Context, q StreamQuery) (*StreamPage[ChangeEvent], error) {
	return s.store.ListChange(ctx, q)
}

// --- Ticket event bridge ---

// SubscribeToTicketEvents subscribes to ticket lifecycle events on the bus
// and publishes them to the change stream. This bridges existing ticket events
// into the change stream per the success criteria.
func (s *Service) SubscribeToTicketEvents(projectIDLookup func(ticketID string) string) {
	ticketEvents := []string{
		events.EventTicketCreated,
		events.EventTicketStarted,
		events.EventTicketSubmitted,
		events.EventTicketValidated,
		events.EventTicketClosed,
		events.EventTicketClaimed,
		events.EventTicketFailed,
		events.EventTicketRejected,
		events.EventTicketApproved,
		events.EventTicketCancelled,
		events.EventTicketReopened,
		events.EventTicketDeploying,
	}
	for _, eventType := range ticketEvents {
		et := eventType // capture loop variable
		s.bus.Subscribe(et, func(ctx context.Context, event events.Event) {
			ticketID, _ := event.Payload["ticket_id"].(string)
			projectID, _ := event.Payload["project_id"].(string)
			state, _ := event.Payload["state"].(string)

			// Try to resolve project_id from payload or lookup
			if projectID == "" && ticketID != "" && projectIDLookup != nil {
				projectID = projectIDLookup(ticketID)
			}
			if projectID == "" {
				return // can't record without project context
			}

			changeEvent := &ChangeEvent{
				ProjectID:        projectID,
				ChangeType:       "ticket_" + stripPrefix(et, "ticket."),
				AffectedEntities: []string{ticketID},
				AfterState: map[string]any{
					"state":     state,
					"ticket_id": ticketID,
				},
				InitiatorType: InitiatorSystem,
				Source:         "ticket_lifecycle",
				Metadata:       event.Payload,
			}

			// Extract initiator from payload if available
			if agentID, ok := event.Payload["agent_id"].(string); ok && agentID != "" {
				changeEvent.InitiatorID = agentID
				changeEvent.InitiatorType = InitiatorAgent
			}

			_ = s.AppendChangeEvent(ctx, changeEvent)
		})
	}
}

// stripPrefix removes the prefix from s if present.
func stripPrefix(s, prefix string) string {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}
