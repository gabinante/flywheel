package observation

import (
	"context"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
)

func TestOpenAndCloseWindow(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	scope := Scope{
		Files:    []string{"pkg/api/handler.go", "pkg/api/routes.go"},
		Services: []string{"api-gateway"},
		Tags:     []string{"api"},
	}

	window, err := svc.OpenWindow(ctx, "ticket-1", "project-1", scope)
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	if window.ID == "" {
		t.Fatal("expected window ID")
	}
	if !window.IsOpen() {
		t.Fatal("expected window to be open")
	}

	// Verify it appears in open windows.
	open, err := svc.GetOpenWindows(ctx, "project-1")
	if err != nil {
		t.Fatalf("GetOpenWindows: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open window, got %d", len(open))
	}

	// Close the window.
	if err := svc.CloseWindow(ctx, window.ID); err != nil {
		t.Fatalf("CloseWindow: %v", err)
	}

	// Verify it's no longer open.
	open, err = svc.GetOpenWindows(ctx, "project-1")
	if err != nil {
		t.Fatalf("GetOpenWindows: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("expected 0 open windows after close, got %d", len(open))
	}
}

func TestAttributionSingleCandidate_HighConfidence(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Open a single window with specific scope.
	scope := Scope{
		Files:    []string{"pkg/api/handler.go"},
		Services: []string{"api-gateway"},
	}
	_, err := svc.OpenWindow(ctx, "ticket-1", "project-1", scope)
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	// Ingest a signal that matches the window's scope exactly.
	signal := Signal{
		ProjectID:  "project-1",
		Source:     "prometheus",
		SignalType: SignalRegression,
		Severity:   SeverityHigh,
		Title:      "Latency spike in api-gateway",
		Scope: Scope{
			Services: []string{"api-gateway"},
			Files:    []string{"pkg/api/handler.go"},
		},
		OccurredAt: time.Now().UTC(),
	}

	attr, err := svc.IngestSignal(ctx, signal)
	if err != nil {
		t.Fatalf("IngestSignal: %v", err)
	}
	if attr == nil {
		t.Fatal("expected attribution")
	}
	if len(attr.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(attr.Candidates))
	}
	if attr.Confidence < 0.7 {
		t.Fatalf("expected high confidence (>0.7), got %f", attr.Confidence)
	}
	if attr.Candidates[0].TicketID != "ticket-1" {
		t.Fatalf("expected ticket-1, got %s", attr.Candidates[0].TicketID)
	}
	if attr.Retroactive {
		t.Fatal("expected non-retroactive attribution")
	}
}

func TestAttributionMultipleCandidates_LowConfidence(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Track low-confidence events.
	var lowConfEvents []events.Event
	bus.Subscribe(EventLowConfidence, func(_ context.Context, event events.Event) {
		lowConfEvents = append(lowConfEvents, event)
	})

	// Open multiple windows with overlapping scope.
	svc.OpenWindow(ctx, "ticket-1", "project-1", Scope{
		Services: []string{"api-gateway"},
		Packages: []string{"pkg/api"},
	})
	svc.OpenWindow(ctx, "ticket-2", "project-1", Scope{
		Services: []string{"api-gateway", "auth-service"},
		Packages: []string{"pkg/api", "pkg/auth"},
	})
	svc.OpenWindow(ctx, "ticket-3", "project-1", Scope{
		Services: []string{"api-gateway"},
		Tags:     []string{"api"},
	})

	// Signal matching all three windows.
	signal := Signal{
		ProjectID:  "project-1",
		Source:     "datadog",
		SignalType: SignalRegression,
		Severity:   SeverityMedium,
		Title:      "Error rate spike in api-gateway",
		Scope: Scope{
			Services: []string{"api-gateway"},
		},
		OccurredAt: time.Now().UTC(),
	}

	attr, err := svc.IngestSignal(ctx, signal)
	if err != nil {
		t.Fatalf("IngestSignal: %v", err)
	}
	if attr == nil {
		t.Fatal("expected attribution")
	}
	if len(attr.Candidates) < 2 {
		t.Fatalf("expected multiple candidates, got %d", len(attr.Candidates))
	}
	// With multiple candidates sharing scope, should be low confidence.
	if !attr.IsLowConfidence() {
		t.Fatalf("expected low confidence with multiple candidates, got %.2f", attr.Confidence)
	}

	// Low-confidence event should be emitted.
	if len(lowConfEvents) != 1 {
		t.Fatalf("expected 1 low_confidence event, got %d", len(lowConfEvents))
	}
}

func TestRetroactiveAttribution(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Create a recently-closed window.
	closedAt := time.Now().UTC().Add(-30 * time.Minute)
	window := &ObservationWindow{
		ID:        "window-closed-1",
		TicketID:  "ticket-old",
		ProjectID: "project-1",
		Scope: Scope{
			Files:    []string{"pkg/db/migrations.go"},
			Services: []string{"postgres"},
		},
		StartedAt: closedAt.Add(-2 * time.Hour),
		EndsAt:    closedAt,
		ClosedAt:  &closedAt,
		State:     WindowClosed,
	}
	store.CreateWindow(ctx, window)

	// Signal arrives after window closed, but scope matches.
	signal := Signal{
		ProjectID:  "project-1",
		Source:     "sentry",
		SignalType: SignalRegression,
		Severity:   SeverityHigh,
		Title:      "Database connection timeout",
		Scope: Scope{
			Services: []string{"postgres"},
			Files:    []string{"pkg/db/migrations.go"},
		},
		OccurredAt: time.Now().UTC(),
	}

	attr, err := svc.IngestSignal(ctx, signal)
	if err != nil {
		t.Fatalf("IngestSignal: %v", err)
	}
	if attr == nil {
		t.Fatal("expected retroactive attribution")
	}
	if !attr.Retroactive {
		t.Fatal("expected retroactive flag to be true")
	}
	if len(attr.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(attr.Candidates))
	}
	if attr.Candidates[0].TicketID != "ticket-old" {
		t.Fatalf("expected ticket-old, got %s", attr.Candidates[0].TicketID)
	}
}

func TestIngestSignal_NoMatchingScope(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Open a window with specific scope.
	_, err := svc.OpenWindow(ctx, "ticket-1", "project-1", Scope{
		Services: []string{"api"},
		Files:    []string{"internal/api/handler.go"},
	})
	if err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}

	// Ingest a signal for a completely different scope.
	signal := Signal{
		ProjectID:  "project-1",
		Source:     "prometheus",
		SignalType: SignalAlert,
		Severity:   SeverityMedium,
		Title:      "Worker memory usage high",
		Scope: Scope{
			Services: []string{"worker"},
			Files:    []string{"internal/worker/pool.go"},
		},
		OccurredAt: time.Now().UTC(),
	}

	attr, err := svc.IngestSignal(ctx, signal)
	if err != nil {
		t.Fatalf("IngestSignal: %v", err)
	}
	if attr != nil {
		t.Fatalf("expected nil attribution for non-matching scope, got confidence %.2f", attr.Confidence)
	}
}

func TestAmbiguityMetrics(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	now := time.Now().UTC()
	// Create attributions with varying confidence.
	store.CreateAttribution(ctx, &Attribution{
		ID:         "attr-1",
		SignalID:   "sig-1",
		ProjectID:  "project-1",
		Confidence: 0.9,
		Candidates: []Candidate{{TicketID: "t1", ScopeMatch: 0.9}},
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	store.CreateAttribution(ctx, &Attribution{
		ID:         "attr-2",
		SignalID:   "sig-2",
		ProjectID:  "project-1",
		Confidence: 0.3,
		Candidates: []Candidate{{TicketID: "t1", ScopeMatch: 0.5}, {TicketID: "t2", ScopeMatch: 0.4}, {TicketID: "t3", ScopeMatch: 0.3}},
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	store.CreateAttribution(ctx, &Attribution{
		ID:         "attr-3",
		SignalID:   "sig-3",
		ProjectID:  "project-1",
		Confidence: 0.2,
		Candidates: []Candidate{{TicketID: "t1", ScopeMatch: 0.3}, {TicketID: "t2", ScopeMatch: 0.3}},
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	metrics, err := svc.GetAmbiguityMetrics(ctx, "project-1", now.Add(-1*time.Hour), now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("GetAmbiguityMetrics: %v", err)
	}
	if metrics.TotalAttributions != 3 {
		t.Fatalf("expected 3 total, got %d", metrics.TotalAttributions)
	}
	if metrics.LowConfidenceCount != 2 {
		t.Fatalf("expected 2 low confidence, got %d", metrics.LowConfidenceCount)
	}
	expectedRate := 2.0 / 3.0
	if metrics.AmbiguityRate < expectedRate-0.01 || metrics.AmbiguityRate > expectedRate+0.01 {
		t.Fatalf("expected ambiguity rate ~%.2f, got %f", expectedRate, metrics.AmbiguityRate)
	}
}

func TestSignalToTicketPipeline(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Add a rule: critical regressions create ticket.
	svc.AddRule(SignalRule{
		ID:          "rule-1",
		ProjectID:   "project-1",
		SignalType:  SignalRegression,
		MinSeverity: SeverityCritical,
		Action:      ActionCreateTicket,
		Enabled:     true,
	})

	// Track create_ticket events.
	var createEvents []events.Event
	bus.Subscribe(EventCreateTicket, func(_ context.Context, event events.Event) {
		createEvents = append(createEvents, event)
	})

	// Critical regression should trigger the rule.
	signal := Signal{
		ProjectID:  "project-1",
		Source:     "sentry",
		SignalType: SignalRegression,
		Severity:   SeverityCritical,
		Title:      "Unhandled exception in payment flow",
		Scope:      Scope{Services: []string{"billing"}},
		OccurredAt: time.Now().UTC(),
	}
	_, err := svc.IngestSignal(ctx, signal)
	if err != nil {
		t.Fatalf("IngestSignal: %v", err)
	}

	if len(createEvents) != 1 {
		t.Fatalf("expected 1 create_ticket event, got %d", len(createEvents))
	}
	if createEvents[0].Payload["action"] != string(ActionCreateTicket) {
		t.Fatalf("expected action=%s, got %v", ActionCreateTicket, createEvents[0].Payload["action"])
	}
}

func TestSignalToTicketPipeline_SeverityFilter(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Rule only triggers for high+ severity.
	svc.AddRule(SignalRule{
		ID:          "rule-1",
		ProjectID:   "project-1",
		SignalType:  SignalRegression,
		MinSeverity: SeverityHigh,
		Action:      ActionCreateTicket,
		Enabled:     true,
	})

	var createEvents []events.Event
	bus.Subscribe(EventCreateTicket, func(_ context.Context, event events.Event) {
		createEvents = append(createEvents, event)
	})

	// Low severity should NOT trigger.
	signal := Signal{
		ProjectID:  "project-1",
		Source:     "prometheus",
		SignalType: SignalRegression,
		Severity:   SeverityLow,
		Title:      "Minor latency increase",
		Scope:      Scope{Services: []string{"api"}},
		OccurredAt: time.Now().UTC(),
	}
	_, _ = svc.IngestSignal(ctx, signal)

	if len(createEvents) != 0 {
		t.Fatalf("expected 0 events for low severity, got %d", len(createEvents))
	}
}

func TestResolveAttribution(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	now := time.Now().UTC()
	store.CreateAttribution(ctx, &Attribution{
		ID:         "attr-1",
		SignalID:   "sig-1",
		ProjectID:  "project-1",
		Confidence: 0.3,
		Candidates: []Candidate{{TicketID: "t1", ScopeMatch: 0.5}},
		CreatedAt:  now,
		UpdatedAt:  now,
	})

	err := svc.ResolveAttribution(ctx, "attr-1", "user-123", "confirmed")
	if err != nil {
		t.Fatalf("ResolveAttribution: %v", err)
	}

	// Verify resolved.
	attr, _ := store.GetAttribution(ctx, "attr-1")
	if !attr.Resolved {
		t.Fatal("expected resolved=true")
	}
	if attr.ResolvedBy != "user-123" {
		t.Fatalf("expected resolved_by user-123, got %s", attr.ResolvedBy)
	}
	if attr.Resolution != "confirmed" {
		t.Fatalf("expected resolution=confirmed, got %s", attr.Resolution)
	}

	// Should no longer appear in unresolved list.
	unresolved, _ := svc.ListUnresolvedAttributions(ctx, "project-1")
	if len(unresolved) != 0 {
		t.Fatalf("expected 0 unresolved, got %d", len(unresolved))
	}
}

func TestPollSources(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	ctx := context.Background()

	// Register a manual source with a pre-loaded signal.
	manual := NewManualSource("test-source")
	manual.Push(Signal{
		ID:         "manual-sig-1",
		SignalType: SignalAlert,
		Severity:   SeverityMedium,
		Title:      "Test alert",
		Scope:      Scope{Tags: []string{"test"}},
		OccurredAt: time.Now().UTC(),
	})
	svc.RegisterSource(manual)

	// Open a matching window.
	_, _ = svc.OpenWindow(ctx, "ticket-1", "project-1", Scope{Tags: []string{"test"}})

	// Poll sources.
	attrs, err := svc.PollSources(ctx, "project-1")
	if err != nil {
		t.Fatalf("PollSources: %v", err)
	}
	if len(attrs) != 1 {
		t.Fatalf("expected 1 attribution from poll, got %d", len(attrs))
	}
}

func TestScopeMatchComputation(t *testing.T) {
	tests := []struct {
		name        string
		ticketScope Scope
		signalScope Scope
		expectMin   float64
		expectMax   float64
	}{
		{
			name:        "exact file match",
			ticketScope: Scope{Files: []string{"pkg/api/handler.go"}},
			signalScope: Scope{Files: []string{"pkg/api/handler.go"}},
			expectMin:   0.9,
			expectMax:   1.0,
		},
		{
			name:        "no overlap",
			ticketScope: Scope{Files: []string{"pkg/api/handler.go"}},
			signalScope: Scope{Files: []string{"pkg/db/store.go"}},
			expectMin:   0.0,
			expectMax:   0.05,
		},
		{
			name:        "prefix match",
			ticketScope: Scope{Files: []string{"pkg/api/handler.go"}},
			signalScope: Scope{Files: []string{"pkg/api"}},
			expectMin:   0.9,
			expectMax:   1.0,
		},
		{
			name:        "service match only",
			ticketScope: Scope{Services: []string{"api-gateway"}},
			signalScope: Scope{Services: []string{"api-gateway"}},
			expectMin:   0.9,
			expectMax:   1.0,
		},
		{
			name:        "empty ticket scope",
			ticketScope: Scope{},
			signalScope: Scope{Files: []string{"pkg/api/handler.go"}},
			expectMin:   0.0,
			expectMax:   0.15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ComputeScopeMatch(tt.ticketScope, tt.signalScope)
			if score < tt.expectMin || score > tt.expectMax {
				t.Errorf("expected scope match in [%.2f, %.2f], got %.2f",
					tt.expectMin, tt.expectMax, score)
			}
		})
	}
}

func TestAttributionConfidence(t *testing.T) {
	tests := []struct {
		name       string
		candidates []Candidate
		expectMin  float64
		expectMax  float64
	}{
		{
			name:       "no candidates",
			candidates: nil,
			expectMin:  0.0,
			expectMax:  0.0,
		},
		{
			name:       "single high-match candidate",
			candidates: []Candidate{{TicketID: "t1", ScopeMatch: 1.0}},
			expectMin:  0.9,
			expectMax:  1.0,
		},
		{
			name:       "single low-match candidate",
			candidates: []Candidate{{TicketID: "t1", ScopeMatch: 0.2}},
			expectMin:  0.5,
			expectMax:  0.7,
		},
		{
			name: "two equal candidates",
			candidates: []Candidate{
				{TicketID: "t1", ScopeMatch: 0.8},
				{TicketID: "t2", ScopeMatch: 0.8},
			},
			expectMin: 0.2,
			expectMax: 0.55,
		},
		{
			name: "many candidates dilute confidence",
			candidates: []Candidate{
				{TicketID: "t1", ScopeMatch: 0.5},
				{TicketID: "t2", ScopeMatch: 0.5},
				{TicketID: "t3", ScopeMatch: 0.5},
				{TicketID: "t4", ScopeMatch: 0.5},
				{TicketID: "t5", ScopeMatch: 0.5},
			},
			expectMin: 0.1,
			expectMax: 0.4,
		},
		{
			name: "one dominant candidate among many",
			candidates: []Candidate{
				{TicketID: "t1", ScopeMatch: 0.95},
				{TicketID: "t2", ScopeMatch: 0.1},
				{TicketID: "t3", ScopeMatch: 0.1},
			},
			expectMin: 0.3,
			expectMax: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence := ComputeAttributionConfidence(tt.candidates)
			if confidence < tt.expectMin || confidence > tt.expectMax {
				t.Errorf("expected confidence in [%.2f, %.2f], got %.2f",
					tt.expectMin, tt.expectMax, confidence)
			}
		})
	}
}

func TestWindowOverlaps(t *testing.T) {
	now := time.Now().UTC()
	endsAt := now.Add(1 * time.Hour)

	window := &ObservationWindow{
		StartedAt: now,
		EndsAt:    endsAt,
		State:     WindowActive,
	}

	// Overlapping range.
	if !window.Overlaps(now.Add(30*time.Minute), now.Add(90*time.Minute)) {
		t.Fatal("expected overlap")
	}
	// Before window.
	if window.Overlaps(now.Add(-2*time.Hour), now.Add(-1*time.Hour)) {
		t.Fatal("expected no overlap before window")
	}
	// After window.
	if window.Overlaps(now.Add(2*time.Hour), now.Add(3*time.Hour)) {
		t.Fatal("expected no overlap after window")
	}
}
