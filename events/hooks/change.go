// Package hooks provides a lightweight client library for publishing change
// events. Publishing a change should be easier than not publishing it —
// three lines in a cron job, a single HTTP call from an external platform.
//
// Quick start:
//
//	client := hooks.NewClient(bus)
//	err := client.Publish(ctx, hooks.Deploy("my-service", "v1.2.3").By("ci-pipeline").In("production"))
package hooks

import (
	"time"

	"github.com/google/uuid"
)

// Common change types matching spec v0.2 §2.3.
const (
	TypeDeploy       = "deploy"
	TypeConfigUpdate = "config_update"
	TypeFailover     = "failover"
	TypeMigration    = "migration"
	TypeScaleEvent   = "scale"
	TypeRollback     = "rollback"
	TypeSecretRotate = "secret_rotation"
	TypeCustom       = "custom"
)

// Change represents a change event with a fluent builder API.
// Construct with helper functions (Deploy, ConfigUpdate, etc.) and chain
// methods to add context.
type Change struct {
	// ID is auto-generated if empty.
	ID string `json:"id"`

	// ChangeType categorises the change (deploy, config_update, failover, …).
	ChangeType string `json:"change_type"`

	// EntityID is the primary entity affected (service name, resource ID, …).
	EntityID string `json:"entity_id"`

	// EntityType classifies the entity (service, database, config, …).
	EntityType string `json:"entity_type,omitempty"`

	// AffectedEntities lists additional entity IDs impacted by this change.
	AffectedEntities []string `json:"affected_entities,omitempty"`

	// Before is the state before the change (version, config hash, replica count, …).
	Before any `json:"before,omitempty"`

	// After is the state after the change.
	After any `json:"after,omitempty"`

	// Initiator identifies who or what triggered the change (user, pipeline, cron, …).
	Initiator string `json:"initiator,omitempty"`

	// Environment is the target environment (production, staging, …).
	Environment string `json:"environment,omitempty"`

	// ProjectID scopes the change to a project.
	ProjectID string `json:"project_id,omitempty"`

	// Timestamp of the change. Defaults to now if zero.
	Timestamp time.Time `json:"timestamp"`

	// Metadata carries arbitrary key-value context.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// --- Builder helpers: one function per common change type ---

// Deploy creates a deploy change event.
//
//	hooks.Deploy("api-gateway", "v2.1.0")
func Deploy(service, version string) Change {
	return Change{
		ChangeType: TypeDeploy,
		EntityID:   service,
		EntityType: "service",
		After:      version,
	}
}

// ConfigUpdate creates a configuration change event.
//
//	hooks.ConfigUpdate("feature-flags", "enable_dark_mode")
func ConfigUpdate(entity, key string) Change {
	return Change{
		ChangeType: TypeConfigUpdate,
		EntityID:   entity,
		Metadata:   map[string]any{"key": key},
	}
}

// Failover creates a failover change event.
//
//	hooks.Failover("postgres-primary").WithBefore("us-east-1").WithAfter("us-west-2")
func Failover(service string) Change {
	return Change{
		ChangeType: TypeFailover,
		EntityID:   service,
		EntityType: "service",
	}
}

// Migration creates a database migration change event.
//
//	hooks.Migration("users-db", "000042_add_index")
func Migration(database, migration string) Change {
	return Change{
		ChangeType: TypeMigration,
		EntityID:   database,
		EntityType: "database",
		After:      migration,
	}
}

// Scale creates a scale change event.
//
//	hooks.Scale("worker-pool").WithBefore(3).WithAfter(10)
func Scale(entity string) Change {
	return Change{
		ChangeType: TypeScaleEvent,
		EntityID:   entity,
	}
}

// Rollback creates a rollback change event.
//
//	hooks.Rollback("api-gateway", "v2.0.9")
func Rollback(service, targetVersion string) Change {
	return Change{
		ChangeType: TypeRollback,
		EntityID:   service,
		EntityType: "service",
		After:      targetVersion,
	}
}

// Custom creates a change event with a custom type.
//
//	hooks.Custom("dns_update", "cdn.example.com")
func Custom(changeType, entityID string) Change {
	return Change{
		ChangeType: changeType,
		EntityID:   entityID,
	}
}

// --- Fluent setters (return Change by value so they're chainable) ---

// By sets the initiator (who or what triggered the change).
func (c Change) By(initiator string) Change {
	c.Initiator = initiator
	return c
}

// In sets the environment.
func (c Change) In(env string) Change {
	c.Environment = env
	return c
}

// For sets the project ID.
func (c Change) For(projectID string) Change {
	c.ProjectID = projectID
	return c
}

// WithBefore sets the before-state.
func (c Change) WithBefore(before any) Change {
	c.Before = before
	return c
}

// WithAfter sets the after-state.
func (c Change) WithAfter(after any) Change {
	c.After = after
	return c
}

// Affecting adds additional affected entity IDs.
func (c Change) Affecting(entityIDs ...string) Change {
	c.AffectedEntities = append(c.AffectedEntities, entityIDs...)
	return c
}

// WithMeta sets a single metadata key.
func (c Change) WithMeta(key string, value any) Change {
	if c.Metadata == nil {
		c.Metadata = make(map[string]any)
	}
	c.Metadata[key] = value
	return c
}

// At sets the timestamp (defaults to time.Now if not set).
func (c Change) At(t time.Time) Change {
	c.Timestamp = t
	return c
}

// AsType overrides the entity type.
func (c Change) AsType(entityType string) Change {
	c.EntityType = entityType
	return c
}

// ensureDefaults fills in auto-generated fields.
func (c *Change) ensureDefaults() {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	if c.Timestamp.IsZero() {
		c.Timestamp = time.Now().UTC()
	}
}

// toPayload converts the change to the bus payload format.
func (c *Change) toPayload() map[string]any {
	m := map[string]any{
		"id":          c.ID,
		"change_type": c.ChangeType,
		"entity_id":   c.EntityID,
		"timestamp":   c.Timestamp.Format(time.RFC3339Nano),
	}
	if c.EntityType != "" {
		m["entity_type"] = c.EntityType
	}
	if len(c.AffectedEntities) > 0 {
		m["affected_entities"] = c.AffectedEntities
	}
	if c.Before != nil {
		m["before"] = c.Before
	}
	if c.After != nil {
		m["after"] = c.After
	}
	if c.Initiator != "" {
		m["initiator"] = c.Initiator
	}
	if c.Environment != "" {
		m["environment"] = c.Environment
	}
	if c.ProjectID != "" {
		m["project_id"] = c.ProjectID
	}
	if len(c.Metadata) > 0 {
		m["metadata"] = c.Metadata
	}
	return m
}
