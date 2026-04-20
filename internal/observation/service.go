package observation

import (
	"context"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// Default observation window duration.
const DefaultWindowDuration = 30 * time.Minute

// Event type constants for observation events.
const (
	EventWindowOpened        = "observation.window_opened"
	EventWindowClosed        = "observation.window_closed"
	EventSignalIngested      = "observation.signal_ingested"
	EventAttributionCreated  = "observation.attribution_created"
	EventAttributionResolved = "observation.attribution_resolved"
	EventLowConfidence       = "observation.low_confidence"
	EventCreateTicket        = "observation.create_ticket"
	EventAnnotateTicket      = "observation.annotate_ticket"
)

// Service provides observation window management, attribution, and signal-to-ticket pipeline.
type Service struct {
	store   Store
	bus     events.Bus
	sources *SourceRegistry
	rules   []SignalRule
}

// NewService creates a new observation service.
func NewService(store Store, bus events.Bus) *Service {
	return &Service{
		store:   store,
		bus:     bus,
		sources: NewSourceRegistry(),
	}
}

// RegisterSource adds a signal source to the service.
func (s *Service) RegisterSource(source SignalSource) {
	s.sources.Register(source)
}

// AddRule adds a signal routing rule.
func (s *Service) AddRule(rule SignalRule) {
	s.rules = append(s.rules, rule)
}

// OpenWindow creates an observation window for a ticket entering the observing state.
func (s *Service) OpenWindow(ctx context.Context, ticketID, projectID string, scope Scope) (*ObservationWindow, error) {
	now := time.Now().UTC()
	window := &ObservationWindow{
		ID:        uuid.New().String(),
		TicketID:  ticketID,
		ProjectID: projectID,
		Scope:     scope,
		StartedAt: now,
		EndsAt:    now.Add(DefaultWindowDuration),
		State:     WindowActive,
	}
	if err := s.store.CreateWindow(ctx, window); err != nil {
		return nil, err
	}
	_ = s.bus.Publish(ctx, events.Event{
		Type:    EventWindowOpened,
		Payload: map[string]any{"window_id": window.ID, "ticket_id": ticketID, "project_id": projectID},
	})
	return window, nil
}

// CloseWindow closes an observation window.
func (s *Service) CloseWindow(ctx context.Context, windowID string) error {
	now := time.Now().UTC()
	if err := s.store.CloseWindow(ctx, windowID, now); err != nil {
		return err
	}
	_ = s.bus.Publish(ctx, events.Event{
		Type:    EventWindowClosed,
		Payload: map[string]any{"window_id": windowID},
	})
	return nil
}

// IngestSignal processes a production signal through the attribution and routing pipeline.
// Returns the attribution (may be nil if no matching windows), or an error.
func (s *Service) IngestSignal(ctx context.Context, signal Signal) (*Attribution, error) {
	if signal.ID == "" {
		signal.ID = uuid.New().String()
	}
	if signal.CreatedAt.IsZero() {
		signal.CreatedAt = time.Now().UTC()
	}
	if err := s.store.CreateSignal(ctx, &signal); err != nil {
		return nil, err
	}
	_ = s.bus.Publish(ctx, events.Event{
		Type:    EventSignalIngested,
		Payload: map[string]any{"signal_id": signal.ID, "project_id": signal.ProjectID, "signal_type": string(signal.SignalType)},
	})

	// Perform attribution: find overlapping windows and score candidates.
	attr, err := s.attributeSignal(ctx, &signal)
	if err != nil {
		return nil, err
	}

	// Emit low-confidence event if attribution is ambiguous.
	if attr != nil && attr.IsLowConfidence() && !attr.Resolved {
		_ = s.bus.Publish(ctx, events.Event{
			Type: EventLowConfidence,
			Payload: map[string]any{
				"attribution_id": attr.ID,
				"signal_id":      signal.ID,
				"confidence":     attr.Confidence,
				"candidates":     len(attr.Candidates),
			},
		})
	}

	// Evaluate signal-to-ticket rules.
	s.evaluateRules(ctx, signal)

	return attr, nil
}

// attributeSignal finds overlapping windows and scores candidates.
func (s *Service) attributeSignal(ctx context.Context, signal *Signal) (*Attribution, error) {
	// Use a +/-5 minute buffer for near-boundary signals.
	buffer := 5 * time.Minute
	start := signal.OccurredAt.Add(-buffer)
	end := signal.OccurredAt.Add(buffer)

	windows, err := s.store.GetOverlappingWindows(ctx, signal.ProjectID, start, end)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return s.retroactiveAttribution(ctx, signal)
	}

	candidates := make([]Candidate, 0, len(windows))
	for _, w := range windows {
		scopeMatch := ComputeScopeMatch(w.Scope, signal.Scope)
		if scopeMatch < 0.05 {
			continue
		}
		rationale := BuildCandidateRationale(w.Scope, signal.Scope, scopeMatch)
		candidates = append(candidates, Candidate{
			TicketID:   w.TicketID,
			WindowID:   w.ID,
			ScopeMatch: scopeMatch,
			Rationale:  rationale,
		})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	confidence := ComputeAttributionConfidence(candidates)
	now := time.Now().UTC()
	attr := &Attribution{
		ID:         uuid.New().String(),
		SignalID:   signal.ID,
		ProjectID:  signal.ProjectID,
		Candidates: candidates,
		Confidence: confidence,
		Rationale:  buildAttributionRationale(candidates, confidence),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.store.CreateAttribution(ctx, attr); err != nil {
		return nil, err
	}
	_ = s.bus.Publish(ctx, events.Event{
		Type: EventAttributionCreated,
		Payload: map[string]any{
			"attribution_id": attr.ID,
			"signal_id":      signal.ID,
			"confidence":     confidence,
			"candidates":     len(candidates),
		},
	})
	return attr, nil
}

// retroactiveAttribution finds recently-closed windows whose scope matches.
func (s *Service) retroactiveAttribution(ctx context.Context, signal *Signal) (*Attribution, error) {
	lookback := 24 * time.Hour
	start := signal.OccurredAt.Add(-lookback)
	end := signal.OccurredAt

	windows, err := s.store.GetOverlappingWindows(ctx, signal.ProjectID, start, end)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, nil
	}

	candidates := make([]Candidate, 0)
	for _, w := range windows {
		if w.IsOpen() {
			continue
		}
		scopeMatch := ComputeScopeMatch(w.Scope, signal.Scope)
		if scopeMatch < 0.1 {
			continue
		}
		rationale := "retroactive: " + BuildCandidateRationale(w.Scope, signal.Scope, scopeMatch)
		candidates = append(candidates, Candidate{
			TicketID:   w.TicketID,
			WindowID:   w.ID,
			ScopeMatch: scopeMatch,
			Rationale:  rationale,
		})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Retroactive has inherently lower confidence (0.8x penalty).
	confidence := ComputeAttributionConfidence(candidates) * 0.8
	now := time.Now().UTC()
	attr := &Attribution{
		ID:          uuid.New().String(),
		SignalID:    signal.ID,
		ProjectID:  signal.ProjectID,
		Candidates: candidates,
		Confidence: confidence,
		Rationale:  buildAttributionRationale(candidates, confidence),
		Retroactive: true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.store.CreateAttribution(ctx, attr); err != nil {
		return nil, err
	}
	_ = s.bus.Publish(ctx, events.Event{
		Type: EventAttributionCreated,
		Payload: map[string]any{
			"attribution_id": attr.ID,
			"signal_id":      signal.ID,
			"confidence":     confidence,
			"candidates":     len(candidates),
			"retroactive":    true,
		},
	})
	return attr, nil
}

// evaluateRules checks signal against configured rules and emits events.
func (s *Service) evaluateRules(ctx context.Context, signal Signal) {
	for _, rule := range s.rules {
		if !rule.Enabled {
			continue
		}
		if rule.ProjectID != "" && rule.ProjectID != signal.ProjectID {
			continue
		}
		if rule.Source != "" && rule.Source != signal.Source {
			continue
		}
		if rule.SignalType != "" && rule.SignalType != signal.SignalType {
			continue
		}
		if !severityMeetsThreshold(signal.Severity, rule.MinSeverity) {
			continue
		}
		// Rule matches -- emit the action event.
		switch rule.Action {
		case ActionCreateTicket:
			_ = s.bus.Publish(ctx, events.Event{
				Type: EventCreateTicket,
				Payload: map[string]any{
					"signal_id":  signal.ID,
					"project_id": signal.ProjectID,
					"action":     string(ActionCreateTicket),
					"title":      signal.Title,
				},
			})
		case ActionAnnotateTicket:
			_ = s.bus.Publish(ctx, events.Event{
				Type: EventAnnotateTicket,
				Payload: map[string]any{
					"signal_id":  signal.ID,
					"project_id": signal.ProjectID,
					"action":     string(ActionAnnotateTicket),
					"title":      signal.Title,
				},
			})
		}
	}
}

// ResolveAttribution marks an attribution as resolved by a human.
func (s *Service) ResolveAttribution(ctx context.Context, attrID, resolvedBy, resolution string) error {
	if err := s.store.ResolveAttribution(ctx, attrID, resolvedBy, resolution); err != nil {
		return err
	}
	_ = s.bus.Publish(ctx, events.Event{
		Type:    EventAttributionResolved,
		Payload: map[string]any{"attribution_id": attrID, "resolved_by": resolvedBy},
	})
	return nil
}

// GetAmbiguityMetrics returns attribution ambiguity metrics for a time range.
func (s *Service) GetAmbiguityMetrics(ctx context.Context, projectID string, start, end time.Time) (*AmbiguityMetrics, error) {
	return s.store.GetAmbiguityMetrics(ctx, projectID, start, end)
}

// PollSources polls all registered sources and processes signals.
func (s *Service) PollSources(ctx context.Context, projectID string) ([]*Attribution, error) {
	signals, err := s.sources.PollAll(ctx, projectID)
	if err != nil {
		return nil, err
	}
	var attrs []*Attribution
	for _, sig := range signals {
		sig.ProjectID = projectID
		attr, err := s.IngestSignal(ctx, sig)
		if err != nil {
			continue
		}
		if attr != nil {
			attrs = append(attrs, attr)
		}
	}
	return attrs, nil
}

// GetOpenWindows returns all currently-open observation windows for a project.
func (s *Service) GetOpenWindows(ctx context.Context, projectID string) ([]*ObservationWindow, error) {
	return s.store.GetOpenWindows(ctx, projectID)
}

// ListUnresolvedAttributions returns attributions needing human review.
func (s *Service) ListUnresolvedAttributions(ctx context.Context, projectID string) ([]*Attribution, error) {
	return s.store.ListUnresolvedAttributions(ctx, projectID)
}

// ListSignals returns recent signals for a project.
func (s *Service) ListSignals(ctx context.Context, projectID string, since time.Time, limit int) ([]*Signal, error) {
	return s.store.ListSignals(ctx, projectID, since, limit)
}

// GetSource returns a registered signal source by name, or nil if not found.
func (s *Service) GetSource(name string) SignalSource {
	return s.sources.Get(name)
}

// severityMeetsThreshold returns true if the signal severity is at or above the threshold.
func severityMeetsThreshold(signal, threshold Severity) bool {
	return severityRank(signal) >= severityRank(threshold)
}

// severityRank returns a numeric rank for severity (higher = more severe).
func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}

// buildAttributionRationale summarizes the attribution for human consumption.
func buildAttributionRationale(candidates []Candidate, confidence float64) string {
	if len(candidates) == 1 {
		return candidates[0].Rationale
	}
	if confidence >= 0.5 {
		return "Multiple candidates with distinguishable scope overlap"
	}
	return "Ambiguous attribution -- multiple candidates with similar scope overlap"
}
