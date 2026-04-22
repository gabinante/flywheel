package entity

import (
	"context"
	"fmt"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// Bus event types for entities.
const (
	BusEventEntityCreated  = "entity.created"
	BusEventEntityRenamed  = "entity.renamed"
	BusEventEntityRetired  = "entity.retired"
	BusEventInstanceCreated = "entity.instance_created"
	BusEventInstanceRetired = "entity.instance_retired"
)

// Service provides entity operations.
type Service struct {
	store *Store
	bus   events.Bus
}

// NewService returns a new Service.
func NewService(store *Store, bus events.Bus) *Service {
	return &Service{store: store, bus: bus}
}

// CreateEntity creates a new entity with a minted UUID. IDs are never reused.
func (s *Service) CreateEntity(ctx context.Context, projectID string, entityType Type, logicalName, description string, attributes map[string]any, actor string) (*Entity, error) {
	if !IsValidType(entityType) {
		return nil, fmt.Errorf("invalid entity type: %s", entityType)
	}
	if logicalName == "" {
		return nil, fmt.Errorf("logical_name is required")
	}
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if attributes == nil {
		attributes = make(map[string]any)
	}

	now := time.Now().UTC()
	e := &Entity{
		ID:          uuid.New().String(),
		Type:        entityType,
		LogicalName: logicalName,
		ProjectID:   projectID,
		Description: description,
		Attributes:  attributes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateEntity(ctx, e); err != nil {
		return nil, err
	}

	// Append to entity stream
	s.appendEvent(ctx, e.ID, EventCreated, map[string]any{
		"type":         string(entityType),
		"logical_name": logicalName,
		"description":  description,
	}, actor)

	// Publish bus event
	_ = s.bus.Publish(ctx, events.Event{
		Type: BusEventEntityCreated,
		Payload: map[string]any{
			"entity_id":    e.ID,
			"project_id":   projectID,
			"type":         string(entityType),
			"logical_name": logicalName,
		},
	})

	return e, nil
}

// GetEntity returns an entity by ID.
func (s *Service) GetEntity(ctx context.Context, id string) (*Entity, error) {
	return s.store.GetEntityByID(ctx, id)
}

// ListEntities returns entities for a project, optionally filtered by type.
func (s *Service) ListEntities(ctx context.Context, projectID string, entityType Type, includeRetired bool) ([]*Entity, error) {
	return s.store.ListEntities(ctx, projectID, entityType, includeRetired)
}

// RenameEntity changes the logical name. Logical names are mutable; IDs are not.
func (s *Service) RenameEntity(ctx context.Context, id, newName, actor string) error {
	if newName == "" {
		return fmt.Errorf("new logical_name is required")
	}
	e, err := s.store.GetEntityByID(ctx, id)
	if err != nil {
		return err
	}
	oldName := e.LogicalName
	if err := s.store.UpdateLogicalName(ctx, id, newName); err != nil {
		return err
	}

	s.appendEvent(ctx, id, EventRenamed, map[string]any{
		"old_name": oldName,
		"new_name": newName,
	}, actor)

	_ = s.bus.Publish(ctx, events.Event{
		Type: BusEventEntityRenamed,
		Payload: map[string]any{
			"entity_id": id,
			"old_name":  oldName,
			"new_name":  newName,
		},
	})

	return nil
}

// UpdateAttributes merges new attributes into the entity.
// External system IDs (e.g. GitHub repo ID, PagerDuty service ID) go here.
func (s *Service) UpdateAttributes(ctx context.Context, id string, attrs map[string]any, actor string) error {
	if err := s.store.UpdateAttributes(ctx, id, attrs); err != nil {
		return err
	}

	s.appendEvent(ctx, id, EventAttributeChanged, map[string]any{
		"attributes": attrs,
	}, actor)

	return nil
}

// RetireEntity soft-deletes an entity. Retired entities remain queryable but are inactive.
func (s *Service) RetireEntity(ctx context.Context, id, actor string) error {
	if err := s.store.RetireEntity(ctx, id); err != nil {
		return err
	}

	s.appendEvent(ctx, id, EventRetired, nil, actor)

	_ = s.bus.Publish(ctx, events.Event{
		Type: BusEventEntityRetired,
		Payload: map[string]any{
			"entity_id": id,
		},
	})

	return nil
}

// CreateInstance creates an environment-qualified instance of an entity.
func (s *Service) CreateInstance(ctx context.Context, entityID, environment string, attributes map[string]any, actor string) (*Instance, error) {
	if environment == "" {
		return nil, fmt.Errorf("environment is required")
	}
	// Verify entity exists
	if _, err := s.store.GetEntityByID(ctx, entityID); err != nil {
		return nil, fmt.Errorf("entity: %w", err)
	}
	if attributes == nil {
		attributes = make(map[string]any)
	}

	now := time.Now().UTC()
	inst := &Instance{
		ID:          uuid.New().String(),
		EntityID:    entityID,
		Environment: environment,
		Attributes:  attributes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.store.CreateInstance(ctx, inst); err != nil {
		return nil, err
	}

	s.appendEvent(ctx, entityID, EventInstanceCreated, map[string]any{
		"instance_id": inst.ID,
		"environment": environment,
	}, actor)

	_ = s.bus.Publish(ctx, events.Event{
		Type: BusEventInstanceCreated,
		Payload: map[string]any{
			"entity_id":   entityID,
			"instance_id": inst.ID,
			"environment": environment,
		},
	})

	return inst, nil
}

// GetInstance returns an instance by ID.
func (s *Service) GetInstance(ctx context.Context, id string) (*Instance, error) {
	return s.store.GetInstanceByID(ctx, id)
}

// ListInstances returns instances for an entity.
func (s *Service) ListInstances(ctx context.Context, entityID string, includeRetired bool) ([]*Instance, error) {
	return s.store.ListInstances(ctx, entityID, includeRetired)
}

// RetireInstance soft-deletes an instance.
func (s *Service) RetireInstance(ctx context.Context, instanceID, actor string) error {
	inst, err := s.store.GetInstanceByID(ctx, instanceID)
	if err != nil {
		return err
	}
	if err := s.store.RetireInstance(ctx, instanceID); err != nil {
		return err
	}

	s.appendEvent(ctx, inst.EntityID, EventInstanceRetired, map[string]any{
		"instance_id": instanceID,
		"environment": inst.Environment,
	}, actor)

	_ = s.bus.Publish(ctx, events.Event{
		Type: BusEventInstanceRetired,
		Payload: map[string]any{
			"entity_id":   inst.EntityID,
			"instance_id": instanceID,
			"environment": inst.Environment,
		},
	})

	return nil
}

// GetStream returns the append-only event stream for an entity.
func (s *Service) GetStream(ctx context.Context, entityID string, limit int) ([]*StreamEvent, error) {
	return s.store.GetStream(ctx, entityID, limit)
}

// GetStreamSince returns global stream events since a given time.
func (s *Service) GetStreamSince(ctx context.Context, since time.Time, limit int) ([]*StreamEvent, error) {
	return s.store.GetStreamSince(ctx, since, limit)
}

// appendEvent appends a stream event. Errors are silently ignored (stream is best-effort).
func (s *Service) appendEvent(ctx context.Context, entityID, eventType string, payload map[string]any, actor string) {
	if payload == nil {
		payload = make(map[string]any)
	}
	evt := &StreamEvent{
		ID:        uuid.New().String(),
		EntityID:  entityID,
		EventType: eventType,
		Payload:   payload,
		Actor:     actor,
		CreatedAt: time.Now().UTC(),
	}
	_ = s.store.AppendStreamEvent(ctx, evt)
}
