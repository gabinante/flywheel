package observation

import (
	"context"
	"time"
)

// SignalSource is the pluggable interface for production signal providers.
// Implementations wrap external observability platforms (Prometheus, Datadog, Sentry, etc.)
// and normalize their alerts/events into the Signal model.
//
// The contract: a source is registered with the service and either:
//   - Pushes signals via the service's IngestSignal method (webhook-driven), or
//   - Is polled periodically via Poll (pull-driven).
//
// Both modes produce Signal values that feed into the attribution engine.
type SignalSource interface {
	// Name returns the source identifier (e.g. "prometheus", "datadog", "sentry").
	Name() string

	// Poll checks for new signals since the given checkpoint.
	// Returns signals found and a new checkpoint for the next poll.
	// Implementations should return nil slice if no new signals.
	Poll(ctx context.Context, projectID string, checkpoint string) ([]Signal, string, error)
}

// WebhookSource extends SignalSource for sources that push via HTTP webhooks.
// The service exposes an HTTP endpoint that routes incoming payloads to the
// appropriate WebhookSource for parsing.
type WebhookSource interface {
	SignalSource

	// ParseWebhook converts a raw webhook payload into zero or more signals.
	// Returns nil if the payload is not relevant (e.g. resolved alert).
	ParseWebhook(ctx context.Context, projectID string, payload []byte) ([]Signal, error)
}

// SourceRegistry manages registered signal sources and their polling state.
type SourceRegistry struct {
	sources     map[string]SignalSource
	checkpoints map[string]string // source_name:project_id → checkpoint
}

// NewSourceRegistry creates a new source registry.
func NewSourceRegistry() *SourceRegistry {
	return &SourceRegistry{
		sources:     make(map[string]SignalSource),
		checkpoints: make(map[string]string),
	}
}

// Register adds a signal source to the registry.
func (r *SourceRegistry) Register(source SignalSource) {
	r.sources[source.Name()] = source
}

// Get returns a source by name, or nil if not found.
func (r *SourceRegistry) Get(name string) SignalSource {
	return r.sources[name]
}

// Sources returns all registered sources.
func (r *SourceRegistry) Sources() []SignalSource {
	result := make([]SignalSource, 0, len(r.sources))
	for _, s := range r.sources {
		result = append(result, s)
	}
	return result
}

// GetCheckpoint returns the last checkpoint for a source+project pair.
func (r *SourceRegistry) GetCheckpoint(sourceName, projectID string) string {
	return r.checkpoints[sourceName+":"+projectID]
}

// SetCheckpoint stores the checkpoint for a source+project pair.
func (r *SourceRegistry) SetCheckpoint(sourceName, projectID, checkpoint string) {
	r.checkpoints[sourceName+":"+projectID] = checkpoint
}

// PollAll polls all registered sources for new signals.
func (r *SourceRegistry) PollAll(ctx context.Context, projectID string) ([]Signal, error) {
	var allSignals []Signal
	for _, source := range r.sources {
		checkpoint := r.GetCheckpoint(source.Name(), projectID)
		signals, newCheckpoint, err := source.Poll(ctx, projectID, checkpoint)
		if err != nil {
			// Log and continue — one source failure shouldn't block others.
			continue
		}
		r.SetCheckpoint(source.Name(), projectID, newCheckpoint)
		allSignals = append(allSignals, signals...)
	}
	return allSignals, nil
}

// NoopSource is a signal source that never returns signals (used as a default/placeholder).
type NoopSource struct {
	name string
}

// NewNoopSource creates a noop source with the given name.
func NewNoopSource(name string) *NoopSource {
	return &NoopSource{name: name}
}

func (n *NoopSource) Name() string { return n.name }

func (n *NoopSource) Poll(_ context.Context, _ string, checkpoint string) ([]Signal, string, error) {
	return nil, checkpoint, nil
}

// ManualSource allows signals to be pushed manually (e.g., from webhooks or CLI).
type ManualSource struct {
	name    string
	signals []Signal
}

// NewManualSource creates a source for manually-pushed signals.
func NewManualSource(name string) *ManualSource {
	return &ManualSource{name: name, signals: nil}
}

func (m *ManualSource) Name() string { return m.name }

// Push adds a signal to be returned on next Poll.
func (m *ManualSource) Push(signal Signal) {
	signal.Source = m.name
	if signal.CreatedAt.IsZero() {
		signal.CreatedAt = time.Now().UTC()
	}
	m.signals = append(m.signals, signal)
}

func (m *ManualSource) Poll(_ context.Context, _ string, checkpoint string) ([]Signal, string, error) {
	signals := m.signals
	m.signals = nil
	newCheckpoint := checkpoint
	if len(signals) > 0 {
		newCheckpoint = signals[len(signals)-1].ID
	}
	return signals, newCheckpoint, nil
}
