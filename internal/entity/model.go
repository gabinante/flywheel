// Package entity implements the entity identity system (spec v0.2 Layer 0, section 2.1).
// Every real thing in the system has a stable UUID used across all layers.
package entity

import "time"

// Type represents the kind of entity.
type Type string

const (
	TypeService     Type = "service"
	TypeDatastore   Type = "datastore"
	TypeIntegration Type = "integration"
	TypeTicket      Type = "ticket"
	TypeFinding     Type = "finding"
)

// AllTypes returns all valid entity types.
func AllTypes() []Type {
	return []Type{TypeService, TypeDatastore, TypeIntegration, TypeTicket, TypeFinding}
}

// IsValidType checks whether t is a valid entity type.
func IsValidType(t Type) bool {
	for _, v := range AllTypes() {
		if t == v {
			return true
		}
	}
	return false
}

// Entity is the core identity: a stable UUID referencing a real thing.
type Entity struct {
	ID          string         `json:"id"`
	Type        Type           `json:"type"`
	LogicalName string         `json:"logical_name"`
	ProjectID   string         `json:"project_id"`
	Description string         `json:"description,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	RetiredAt   *time.Time     `json:"retired_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// IsRetired returns true if the entity has been retired.
func (e *Entity) IsRetired() bool {
	return e.RetiredAt != nil
}

// Instance is an environment-qualified entity instance.
// For example, OrderService@prod and OrderService@dev are two instances of the same entity.
type Instance struct {
	ID          string         `json:"id"`
	EntityID    string         `json:"entity_id"`
	Environment string         `json:"environment"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	RetiredAt   *time.Time     `json:"retired_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// IsRetired returns true if the instance has been retired.
func (i *Instance) IsRetired() bool {
	return i.RetiredAt != nil
}

// StreamEvent is an append-only event in the entity lifecycle stream.
type StreamEvent struct {
	ID        string         `json:"id"`
	EntityID  string         `json:"entity_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload,omitempty"`
	Actor     string         `json:"actor"`
	CreatedAt time.Time      `json:"created_at"`
}

// Stream event types.
const (
	EventCreated          = "created"
	EventRenamed          = "renamed"
	EventRetired          = "retired"
	EventInstanceCreated  = "instance_created"
	EventInstanceRetired  = "instance_retired"
	EventAttributeChanged = "attribute_changed"
)
