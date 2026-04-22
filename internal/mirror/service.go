package mirror

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/ticket"
)

// TicketGetter retrieves ticket data. Implemented by ticket.Service.
type TicketGetter interface {
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)
}

// ProjectGetter retrieves project data. Implemented by project.Service.
type ProjectGetter interface {
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

// Service orchestrates one-way ticket mirroring to external issue trackers.
// It subscribes to ticket lifecycle events and dispatches mirror operations
// to the appropriate adapter based on project configuration.
type Service struct {
	tickets  TicketGetter
	projects ProjectGetter
	bus      events.Bus
	adapters map[string]Adapter

	// mu protects externalIDs map (in-memory mapping for v1; could be persisted later)
	mu          sync.RWMutex
	externalIDs map[string]string // ticket_id -> external_issue_id
}

// NewService creates a new mirror service and subscribes to ticket events.
func NewService(tickets TicketGetter, projects ProjectGetter, bus events.Bus) *Service {
	s := &Service{
		tickets:     tickets,
		projects:    projects,
		bus:         bus,
		adapters:    make(map[string]Adapter),
		externalIDs: make(map[string]string),
	}
	s.subscribeToEvents()
	return s
}

// RegisterAdapter registers an adapter for the given provider name.
func (s *Service) RegisterAdapter(name string, adapter Adapter) {
	s.adapters[name] = adapter
}

// SetExternalID stores the external ID mapping for a ticket (used for recovery/restart).
func (s *Service) SetExternalID(ticketID, externalID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.externalIDs[ticketID] = externalID
}

// GetExternalID returns the external ID for a mirrored ticket, if any.
func (s *Service) GetExternalID(ticketID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.externalIDs[ticketID]
	return id, ok
}

// subscribeToEvents registers handlers for all ticket lifecycle events that
// should trigger mirror updates.
func (s *Service) subscribeToEvents() {
	// Creation event - create mirror ticket
	s.bus.Subscribe(events.EventTicketCreated, s.handleTicketCreated)

	// State transition events - update mirror state
	stateEvents := []string{
		events.EventTicketSpecced,
		events.EventTicketPlanning,
		events.EventTicketClaimed,
		events.EventTicketStarted,
		events.EventTicketSubmitted,
		events.EventTicketValidated,
		events.EventTicketDeploying,
		events.EventTicketObserving,
		events.EventTicketApproved,
		events.EventTicketRejected,
		events.EventTicketEscalated,
		events.EventTicketReplanned,
		events.EventTicketInvalidated,
		events.EventTicketAwaitingInput,
		events.EventTicketInputProvided,
		events.EventTicketFailed,
		events.EventTicketReopened,
	}
	for _, eventType := range stateEvents {
		s.bus.Subscribe(eventType, s.handleStateTransition)
	}

	// Close/cancel events - close mirror ticket
	s.bus.Subscribe(events.EventTicketClosed, s.handleTicketClosed)
	s.bus.Subscribe(events.EventTicketCancelled, s.handleTicketClosed)
}

// handleTicketCreated creates a new issue in the external tracker.
func (s *Service) handleTicketCreated(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	cfg, adapter, err := s.resolveAdapter(ctx, ticketID)
	if err != nil || adapter == nil {
		return // not configured or error (already logged)
	}

	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		log.Printf("mirror: failed to get ticket %s: %v", ticketID, err)
		return
	}

	data := ticketToData(t, cfg)
	externalID, err := adapter.CreateTicket(ctx, cfg, data)
	if err != nil {
		log.Printf("mirror: failed to create external ticket for %s: %v", ticketID, err)
		return
	}

	s.mu.Lock()
	s.externalIDs[ticketID] = externalID
	s.mu.Unlock()

	log.Printf("mirror: created %s issue %s for ticket %s", adapter.Name(), externalID, ticketID)
}

// handleStateTransition updates the external issue's state.
func (s *Service) handleStateTransition(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	newState, _ := event.Payload["state"].(string)
	if ticketID == "" || newState == "" {
		return
	}

	cfg, adapter, err := s.resolveAdapter(ctx, ticketID)
	if err != nil || adapter == nil {
		return
	}

	externalID, ok := s.GetExternalID(ticketID)
	if !ok {
		// Ticket may have been created before mirroring was enabled.
		// Try to create it now.
		s.handleTicketCreated(ctx, event)
		return
	}

	// Map our state to the external system's state.
	mappedState := cfg.MapState(newState)
	if mappedState == "" {
		// No mapping for this state - skip update.
		return
	}

	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		log.Printf("mirror: failed to get ticket %s for state update: %v", ticketID, err)
		return
	}

	data := ticketToData(t, cfg)
	if err := adapter.UpdateState(ctx, cfg, externalID, mappedState, data); err != nil {
		log.Printf("mirror: failed to update state for %s (external: %s): %v", ticketID, externalID, err)
		return
	}

	// Also update custom fields on state transitions (data may have changed).
	fields := ticketToCustomFields(t)
	if err := adapter.UpdateFields(ctx, cfg, externalID, fields); err != nil {
		log.Printf("mirror: failed to update fields for %s (external: %s): %v", ticketID, externalID, err)
	}
}

// handleTicketClosed closes the external issue with a link back to our context.
func (s *Service) handleTicketClosed(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	if ticketID == "" {
		return
	}

	cfg, adapter, err := s.resolveAdapter(ctx, ticketID)
	if err != nil || adapter == nil {
		return
	}

	externalID, ok := s.GetExternalID(ticketID)
	if !ok {
		return // never mirrored
	}

	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		log.Printf("mirror: failed to get ticket %s for close: %v", ticketID, err)
		return
	}

	data := ticketToData(t, cfg)
	contextURL := cfg.TicketURL(ticketID)
	if err := adapter.CloseTicket(ctx, cfg, externalID, contextURL, data); err != nil {
		log.Printf("mirror: failed to close external ticket for %s (external: %s): %v", ticketID, externalID, err)
	}

	log.Printf("mirror: closed %s issue %s for ticket %s", adapter.Name(), externalID, ticketID)
}

// resolveAdapter loads the mirror config for a ticket's project and returns
// the appropriate adapter. Returns (nil, nil, nil) if mirroring is not enabled.
func (s *Service) resolveAdapter(ctx context.Context, ticketID string) (*Config, Adapter, error) {
	t, err := s.tickets.GetTicket(ctx, ticketID)
	if err != nil {
		log.Printf("mirror: failed to get ticket %s: %v", ticketID, err)
		return nil, nil, err
	}

	p, err := s.projects.GetProject(ctx, t.ProjectID)
	if err != nil {
		log.Printf("mirror: failed to get project %s: %v", t.ProjectID, err)
		return nil, nil, err
	}

	cfg, err := ParseConfig(p.ContextPack.Extra)
	if err != nil {
		log.Printf("mirror: invalid config for project %s: %v", t.ProjectID, err)
		return nil, nil, err
	}
	if cfg == nil || !cfg.Enabled {
		return nil, nil, nil
	}

	adapter, ok := s.adapters[cfg.Provider]
	if !ok {
		log.Printf("mirror: no adapter registered for provider %q (project %s)", cfg.Provider, t.ProjectID)
		return nil, nil, fmt.Errorf("no adapter for provider %q", cfg.Provider)
	}

	return cfg, adapter, nil
}

// ticketToData converts a ticket to the mirror data struct.
func ticketToData(t *ticket.Ticket, cfg *Config) TicketData {
	return TicketData{
		ID:          t.ID,
		ProjectID:   t.ProjectID,
		Title:       t.Title,
		Type:        string(t.Type),
		Priority:    int(t.Priority),
		State:       string(t.State),
		Description: t.Objective.Description,
		Environment: string(t.Environment),
		AssignedTo:  t.AssignedTo,
		URL:         cfg.TicketURL(t.ID),
	}
}

// ticketToCustomFields extracts custom fields from a ticket for mirroring.
func ticketToCustomFields(t *ticket.Ticket) CustomFields {
	fields := CustomFields{
		EnvironmentTarget: string(t.Environment),
	}

	// Extract risk classification from inputs if present.
	if risk, ok := t.Inputs["risk_classification"].(string); ok {
		fields.RiskClassification = risk
	}

	// Extract linked services from inputs if present.
	if services, ok := t.Inputs["linked_services"].([]any); ok {
		for _, svc := range services {
			if s, ok := svc.(string); ok {
				fields.LinkedServices = append(fields.LinkedServices, s)
			}
		}
	}

	return fields
}
