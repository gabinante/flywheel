package stream

import (
	"context"
	"strings"
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
			"event_id":          event.ID,
			"project_id":        event.ProjectID,
			"change_type":       event.ChangeType,
			"affected_entities": event.AffectedEntities,
			"source":            event.Source,
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

// SubscribeToTicketEvents subscribes to user-visible workflow events on the bus
// and records them in the project change stream.
func (s *Service) SubscribeToTicketEvents(projectIDLookup func(ticketID string) string) {
	activityEvents := []string{
		events.EventTicketCreated,
		events.EventTicketUpdated,
		events.EventTicketPlanning,
		events.EventTicketAwaitingInput,
		events.EventTicketInputProvided,
		events.EventTicketEscalated,
		events.EventTicketReplanned,
		events.EventTicketInvalidated,
		events.EventTicketRolledBack,
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
		events.EventLeaseExpired,
		events.EventTestsPassed,
		events.EventTestsFailed,
		events.EventPlanCreated,
		events.EventPlanSubmitted,
		events.EventPlanClassified,
		events.EventPlanApproved,
		events.EventPlanApplied,
		events.EventPlanRejected,
		events.EventPlanSuperseded,
		events.EventPlanRePlanIdentical,
		events.EventPlanRePlanDiverged,
		events.EventWorkStreamCompleted,
	}
	for _, eventType := range activityEvents {
		et := eventType // capture loop variable
		s.bus.Subscribe(et, func(ctx context.Context, event events.Event) {
			if changeEvent := activityChangeEvent(et, event.Payload, projectIDLookup); changeEvent != nil {
				_ = s.AppendChangeEvent(ctx, changeEvent)
			}
		})
	}
}

func activityChangeEvent(eventType string, payload map[string]any, projectIDLookup func(ticketID string) string) *ChangeEvent {
	ticketID := payloadString(payload, "ticket_id")
	projectID := payloadString(payload, "project_id")
	if projectID == "" && ticketID != "" && projectIDLookup != nil {
		projectID = projectIDLookup(ticketID)
	}
	if projectID == "" {
		return nil
	}

	changeEvent := &ChangeEvent{
		ProjectID:        projectID,
		ChangeType:       strings.ReplaceAll(eventType, ".", "_"),
		AffectedEntities: activityEntities(payload),
		InitiatorID:      activityInitiatorID(payload),
		InitiatorType:    activityInitiatorType(payload),
		Environment:      payloadString(payload, "environment"),
		Source:           activitySource(eventType),
		Metadata:         payload,
	}
	if state := payloadString(payload, "state"); state != "" {
		changeEvent.AfterState = map[string]any{"state": state}
		if ticketID != "" {
			changeEvent.AfterState["ticket_id"] = ticketID
		}
	}
	return changeEvent
}

func activityEntities(payload map[string]any) []string {
	keys := []string{
		"ticket_id",
		"plan_id",
		"work_stream_id",
		"window_id",
		"signal_id",
		"attribution_id",
		"environment_id",
		"entity_id",
	}
	seen := make(map[string]struct{}, len(keys))
	entities := make([]string, 0, len(keys))
	for _, key := range keys {
		value := payloadString(payload, key)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		entities = append(entities, value)
	}
	return entities
}

func activityInitiatorID(payload map[string]any) string {
	for _, key := range []string{"agent_id", "actor_id", "reviewer_id", "resolved_by", "created_by"} {
		if value := payloadString(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func activityInitiatorType(payload map[string]any) InitiatorType {
	switch payloadString(payload, "actor_type") {
	case "human":
		return InitiatorHuman
	case "agent":
		return InitiatorAgent
	case "system":
		return InitiatorSystem
	}
	if payloadString(payload, "agent_id") != "" {
		return InitiatorAgent
	}
	if payloadString(payload, "reviewer_id") != "" || payloadString(payload, "resolved_by") != "" {
		return InitiatorHuman
	}
	return InitiatorSystem
}

func activitySource(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "ticket."):
		return "ticket_lifecycle"
	case strings.HasPrefix(eventType, "plan."):
		return "plan_lifecycle"
	case strings.HasPrefix(eventType, "work_stream."):
		return "work_stream"
	default:
		if prefix, _, ok := strings.Cut(eventType, "."); ok {
			return prefix
		}
		return "system"
	}
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}
