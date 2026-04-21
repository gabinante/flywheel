package hooks

// Option configures a Client.
type Option func(*Client)

// WithInitiator sets the default initiator for all events.
//
//	hooks.NewClient(bus, hooks.WithInitiator("deploy-pipeline"))
func WithInitiator(initiator string) Option {
	return func(c *Client) {
		c.defaults.Initiator = initiator
	}
}

// WithEnvironment sets the default environment for all events.
//
//	hooks.NewClient(bus, hooks.WithEnvironment("production"))
func WithEnvironment(env string) Option {
	return func(c *Client) {
		c.defaults.Environment = env
	}
}

// WithProjectID sets the default project ID for all events.
//
//	hooks.NewClient(bus, hooks.WithProjectID("proj-123"))
func WithProjectID(projectID string) Option {
	return func(c *Client) {
		c.defaults.ProjectID = projectID
	}
}

// WithDefaults sets all defaults at once.
func WithDefaults(d Defaults) Option {
	return func(c *Client) {
		c.defaults = d
	}
}
