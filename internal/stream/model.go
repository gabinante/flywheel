// Package stream implements the three foundational append-only streams
// from spec v0.2 section 2.2: Entity, State, and Change streams.
// All streams share the common change event shape from section 2.3.
package stream

import (
	"errors"
	"time"
)

// Sentinel errors.
var (
	ErrEventNotFound = errors.New("stream event not found")
	ErrAppendOnly    = errors.New("stream tables are append-only; updates and deletes are not allowed")
)

// --- Common change event shape (spec 2.3) ---

// InitiatorType identifies who caused a change.
type InitiatorType string

const (
	InitiatorHuman  InitiatorType = "human"
	InitiatorAgent  InitiatorType = "agent"
	InitiatorSystem InitiatorType = "system"
)

// --- Entity Stream ---

// EntityChangeType enumerates entity lifecycle events.
type EntityChangeType string

const (
	EntityCreated EntityChangeType = "created"
	EntityRenamed EntityChangeType = "renamed"
	EntityRetired EntityChangeType = "retired"
	EntityUpdated EntityChangeType = "updated"
)

// EntityType classifies the kind of entity.
type EntityType string

const (
	EntityService        EntityType = "service"
	EntityDatastore      EntityType = "datastore"
	EntityIntegration    EntityType = "integration"
	EntityTicket         EntityType = "ticket"
	EntityFinding        EntityType = "finding"
	EntityInfrastructure EntityType = "infrastructure"
	EntityOther          EntityType = "other"
)

// EntityEvent records an entity lifecycle event (creation, renaming, retirement).
type EntityEvent struct {
	// Common shape
	ID                string           `json:"id"`
	ProjectID         string           `json:"project_id"`
	ChangeType        EntityChangeType `json:"change_type"`
	AffectedEntities  []string         `json:"affected_entities"`
	BeforeState       map[string]any   `json:"before_state,omitempty"`
	AfterState        map[string]any   `json:"after_state,omitempty"`
	InitiatorID       string           `json:"initiator_id"`
	InitiatorType     InitiatorType    `json:"initiator_type"`
	Environment       string           `json:"environment,omitempty"`
	Metadata          map[string]any   `json:"metadata,omitempty"`
	Timestamp         time.Time        `json:"timestamp"`
	CreatedAt         time.Time        `json:"created_at"`
	// Entity-specific
	EntityID   string     `json:"entity_id"`
	EntityType EntityType `json:"entity_type"`
	EntityName string     `json:"entity_name"`
}

// --- State Stream ---

// StateChangeType enumerates state observation events.
type StateChangeType string

const (
	StateObserved   StateChangeType = "observed"
	StateUpdated    StateChangeType = "updated"
	StateDrifted    StateChangeType = "drifted"
	StateReconciled StateChangeType = "reconciled"
)

// StateEvent records a state observation (current values from APIs, K8s, databases).
type StateEvent struct {
	// Common shape
	ID               string          `json:"id"`
	ProjectID        string          `json:"project_id"`
	ChangeType       StateChangeType `json:"change_type"`
	AffectedEntities []string        `json:"affected_entities"`
	BeforeState      map[string]any  `json:"before_state,omitempty"`
	AfterState       map[string]any  `json:"after_state,omitempty"`
	InitiatorID      string          `json:"initiator_id"`
	InitiatorType    InitiatorType   `json:"initiator_type"`
	Environment      string          `json:"environment,omitempty"`
	Metadata         map[string]any  `json:"metadata,omitempty"`
	Timestamp        time.Time       `json:"timestamp"`
	CreatedAt        time.Time       `json:"created_at"`
	// State-specific
	EntityID   string    `json:"entity_id"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
}

// --- Change Stream ---

// ChangeEvent records what happened — deploys, config updates, failovers, ticket events.
// change_type is free-form (not constrained to an enum) for extensibility.
type ChangeEvent struct {
	// Common shape
	ID               string         `json:"id"`
	ProjectID        string         `json:"project_id"`
	ChangeType       string         `json:"change_type"`
	AffectedEntities []string       `json:"affected_entities"`
	BeforeState      map[string]any `json:"before_state,omitempty"`
	AfterState       map[string]any `json:"after_state,omitempty"`
	InitiatorID      string         `json:"initiator_id"`
	InitiatorType    InitiatorType  `json:"initiator_type"`
	Environment      string         `json:"environment,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Timestamp        time.Time      `json:"timestamp"`
	CreatedAt        time.Time      `json:"created_at"`
	// Change-specific
	Source string `json:"source"`
}

// --- Query filters ---

// StreamQuery filters for listing stream events.
type StreamQuery struct {
	ProjectID   string
	EntityID    string
	ChangeType  string
	Environment string
	InitiatorID string
	Source      string // state_stream and change_stream only
	Since       time.Time
	Until       time.Time
	Limit       int
	Offset      int
}

// StreamPage is a paginated list of events.
type StreamPage[T any] struct {
	Events []*T `json:"events"`
	Total  int  `json:"total"`
	Limit  int  `json:"limit"`
	Offset int  `json:"offset"`
}
