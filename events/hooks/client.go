package hooks

import (
	"context"
	"fmt"

	"github.com/gabinante/flywheel/events"
)

// Client publishes change events to the event bus. Create one with NewClient
// and reuse it — it is safe for concurrent use.
//
// Three lines to publish a change:
//
//	client := hooks.NewClient(bus)                                                        // 1
//	err := client.Publish(ctx, hooks.Deploy("api", "v2.1").By("ci").In("production"))     // 2
//	// (line 3 is your error handling)                                                    // 3
type Client struct {
	bus      events.Bus
	defaults Defaults
}

// Defaults are applied to every change event published through this client
// unless the change already has the field set.
type Defaults struct {
	// Initiator default (e.g. "deploy-pipeline", "cron-scheduler").
	Initiator string

	// Environment default (e.g. "production").
	Environment string

	// ProjectID default.
	ProjectID string
}

// NewClient creates a hook client that publishes to the given bus.
//
//	client := hooks.NewClient(bus)
//	client := hooks.NewClient(bus, hooks.WithEnvironment("production"))
func NewClient(bus events.Bus, opts ...Option) *Client {
	c := &Client{bus: bus}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Publish publishes a change event onto the bus. Missing fields are filled
// from client defaults. ID and timestamp are auto-generated when empty.
func (c *Client) Publish(ctx context.Context, change Change) error {
	c.applyDefaults(&change)
	change.ensureDefaults()

	if change.ChangeType == "" {
		return fmt.Errorf("hooks: change_type is required")
	}
	if change.EntityID == "" {
		return fmt.Errorf("hooks: entity_id is required")
	}

	return c.bus.Publish(ctx, events.Event{
		Type:    events.EventChangePublished,
		Payload: change.toPayload(),
	})
}

// applyDefaults fills in fields from client defaults when the change doesn't
// already have them set.
func (c *Client) applyDefaults(change *Change) {
	if change.Initiator == "" && c.defaults.Initiator != "" {
		change.Initiator = c.defaults.Initiator
	}
	if change.Environment == "" && c.defaults.Environment != "" {
		change.Environment = c.defaults.Environment
	}
	if change.ProjectID == "" && c.defaults.ProjectID != "" {
		change.ProjectID = c.defaults.ProjectID
	}
}

// --- Package-level convenience (optional singleton pattern) ---

var defaultClient *Client

// Init sets a package-level default client. After calling Init, you can use
// the package-level Publish function for even less boilerplate:
//
//	hooks.Init(bus, hooks.WithEnvironment("production"))
//	hooks.Publish(ctx, hooks.Deploy("api", "v2"))
func Init(bus events.Bus, opts ...Option) {
	defaultClient = NewClient(bus, opts...)
}

// Publish publishes a change using the package-level client (set via Init).
// Panics if Init has not been called.
func Publish(ctx context.Context, change Change) error {
	if defaultClient == nil {
		return fmt.Errorf("hooks: Init() has not been called; create a Client with NewClient() or call Init() first")
	}
	return defaultClient.Publish(ctx, change)
}
