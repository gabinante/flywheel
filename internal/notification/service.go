package notification

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gabinante/flywheel/events"
	"github.com/google/uuid"
)

// Service orchestrates notification delivery with policy-driven routing.
// It subscribes to events, determines urgency/channel/routing, and dispatches
// to the appropriate channel adapter.
type Service struct {
	store    Store
	bus      events.Bus
	adapters map[Channel]ChannelAdapter
	mu       sync.RWMutex
}

// NewService creates a notification service and subscribes to relevant events.
func NewService(store Store, bus events.Bus) *Service {
	s := &Service{
		store:    store,
		bus:      bus,
		adapters: make(map[Channel]ChannelAdapter),
	}
	s.subscribeToEvents()
	return s
}

// RegisterAdapter registers a channel adapter for the given channel type.
func (s *Service) RegisterAdapter(ch Channel, adapter ChannelAdapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapters[ch] = adapter
}

// getAdapter returns the adapter for a channel, if registered.
func (s *Service) getAdapter(ch Channel) (ChannelAdapter, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.adapters[ch]
	return a, ok
}

// Notify creates and routes a notification based on project preferences.
// This is the main entry point for programmatic notification creation.
func (s *Service) Notify(ctx context.Context, n *Notification) error {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	// Load preferences for routing decisions.
	prefs, err := s.store.GetPreferences(ctx, n.ProjectID)
	if err != nil {
		// No preferences set — use defaults.
		prefs = DefaultPreferences(n.ProjectID)
	}

	// Determine channel and routing based on preferences.
	n.Channel = prefs.ChannelForUrgency(n.Urgency)
	if prefs.ShouldPush(n.Urgency) {
		n.Routing = RoutingPush
	} else if prefs.DigestEnabled {
		n.Routing = RoutingDigest
	} else {
		n.Routing = RoutingPush // fallback: push if digest disabled
	}

	n.Status = StatusPending
	if err := s.store.CreateNotification(ctx, n); err != nil {
		return err
	}

	// Track send count for classifier tuning.
	if n.Classifier != "" {
		if err := s.store.IncrementSent(ctx, n.Classifier, n.ProjectID); err != nil {
			slog.Error("notification: failed to increment sent count", "classifier", n.Classifier, "error", err)
		}
	}

	// Publish event.
	_ = s.bus.Publish(ctx, events.Event{
		Type: events.EventNotificationCreated,
		Payload: map[string]any{
			"notification_id": n.ID,
			"project_id":      n.ProjectID,
			"category":        string(n.Category),
			"urgency":         string(n.Urgency),
			"channel":         string(n.Channel),
			"routing":         string(n.Routing),
		},
	})

	// Push immediately if routing says so.
	if n.Routing == RoutingPush {
		s.deliver(ctx, n, prefs)
	}

	return nil
}

// DismissNotification marks a notification as dismissed and updates classifier stats.
func (s *Service) DismissNotification(ctx context.Context, id string) error {
	return s.store.DismissNotification(ctx, id)
}

// FlushDigest delivers all pending digest notifications for a project as a batch.
func (s *Service) FlushDigest(ctx context.Context, projectID string) error {
	pending, err := s.store.ListPendingDigest(ctx, projectID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	prefs, err := s.store.GetPreferences(ctx, projectID)
	if err != nil {
		prefs = DefaultPreferences(projectID)
	}

	// Group by channel.
	byChannel := make(map[Channel][]*Notification)
	for _, n := range pending {
		byChannel[n.Channel] = append(byChannel[n.Channel], n)
	}

	var ids []string
	for ch, batch := range byChannel {
		adapter, ok := s.getAdapter(ch)
		if !ok {
			slog.Warn("notification: no adapter for channel, skipping digest", "channel", string(ch))
			continue
		}
		if err := adapter.SendDigest(ctx, batch, prefs); err != nil {
			slog.Error("notification: digest send failed", "channel", string(ch), "error", err)
			continue
		}
		for _, n := range batch {
			ids = append(ids, n.ID)
		}
	}

	if len(ids) > 0 {
		if err := s.store.MarkDigested(ctx, ids); err != nil {
			slog.Error("notification: failed to mark digested", "error", err)
			return err
		}

		_ = s.bus.Publish(ctx, events.Event{
			Type: events.EventNotificationDigestSent,
			Payload: map[string]any{
				"project_id": projectID,
				"count":      len(ids),
			},
		})
	}

	return nil
}

// GetPreferences returns notification preferences for a project (or defaults).
func (s *Service) GetPreferences(ctx context.Context, projectID string) (*Preferences, error) {
	prefs, err := s.store.GetPreferences(ctx, projectID)
	if err != nil {
		return DefaultPreferences(projectID), nil
	}
	return prefs, nil
}

// SetPreferences updates notification preferences for a project.
func (s *Service) SetPreferences(ctx context.Context, prefs *Preferences) error {
	if prefs.ID == "" {
		prefs.ID = uuid.New().String()
	}
	prefs.UpdatedAt = time.Now().UTC()
	if prefs.CreatedAt.IsZero() {
		prefs.CreatedAt = prefs.UpdatedAt
	}
	return s.store.UpsertPreferences(ctx, prefs)
}

// ListNotifications returns notifications for a project.
func (s *Service) ListNotifications(ctx context.Context, projectID string, limit, offset int) ([]*Notification, error) {
	return s.store.ListNotifications(ctx, projectID, limit, offset)
}

// GetDismissalRates returns dismissal rate stats for all classifiers in a project.
func (s *Service) GetDismissalRates(ctx context.Context, projectID string) ([]*DismissalRate, error) {
	return s.store.ListDismissalRates(ctx, projectID)
}

// deliver sends a single notification via its channel adapter.
func (s *Service) deliver(ctx context.Context, n *Notification, prefs *Preferences) {
	adapter, ok := s.getAdapter(n.Channel)
	if !ok {
		slog.Warn("notification: no adapter for channel, marking failed", "channel", string(n.Channel))
		_ = s.store.UpdateNotificationStatus(ctx, n.ID, StatusFailed, "no adapter registered for channel")
		return
	}

	if err := adapter.Send(ctx, n, prefs); err != nil {
		slog.Error("notification: send failed", "notification_id", n.ID, "channel", string(n.Channel), "error", err)
		_ = s.store.UpdateNotificationStatus(ctx, n.ID, StatusFailed, err.Error())

		_ = s.bus.Publish(ctx, events.Event{
			Type: events.EventNotificationFailed,
			Payload: map[string]any{
				"notification_id": n.ID,
				"channel":         string(n.Channel),
				"error":           err.Error(),
			},
		})
		return
	}

	_ = s.store.UpdateNotificationStatus(ctx, n.ID, StatusSent, "")

	_ = s.bus.Publish(ctx, events.Event{
		Type: events.EventNotificationSent,
		Payload: map[string]any{
			"notification_id": n.ID,
			"project_id":      n.ProjectID,
			"channel":         string(n.Channel),
		},
	})
}

// subscribeToEvents registers handlers for events that should trigger notifications.
func (s *Service) subscribeToEvents() {
	// Escalation events → urgent decision notification.
	s.bus.Subscribe(events.EventTicketEscalated, s.handleEscalation)

	// Budget alerts → anomaly alert notification.
	s.bus.Subscribe(events.EventBudgetAlert, s.handleBudgetAlert)

	// Ticket submitted for review → calibration review notification.
	s.bus.Subscribe(events.EventTicketSubmitted, s.handleTicketSubmitted)

	// Policy calibration events → calibration review notification.
	s.bus.Subscribe(events.EventPlanApproved, s.handlePlanApproved)
	s.bus.Subscribe(events.EventPlanRejected, s.handlePlanRejected)
}

func (s *Service) handleEscalation(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)
	reason, _ := event.Payload["reason"].(string)

	if projectID == "" || ticketID == "" {
		return
	}

	_ = s.Notify(ctx, &Notification{
		ProjectID:  projectID,
		TicketID:   ticketID,
		Category:   CategoryUrgentDecision,
		Urgency:    UrgencyCritical,
		Title:      "Escalation: agent needs human decision",
		Body:       reason,
		Classifier: "escalation",
		DecisionMeta: map[string]any{
			"ticket_id": ticketID,
			"reason":    reason,
		},
	})
}

func (s *Service) handleBudgetAlert(ctx context.Context, event events.Event) {
	projectID, _ := event.Payload["project_id"].(string)
	message, _ := event.Payload["message"].(string)
	alertType, _ := event.Payload["alert_type"].(string)

	if projectID == "" {
		return
	}

	urgency := UrgencyMedium
	if alertType == "exceeded" {
		urgency = UrgencyHigh
	}

	_ = s.Notify(ctx, &Notification{
		ProjectID:  projectID,
		Category:   CategoryAnomalyAlert,
		Urgency:    urgency,
		Title:      "Budget alert",
		Body:       message,
		Classifier: "budget_alert",
		Context:    event.Payload,
	})
}

func (s *Service) handleTicketSubmitted(ctx context.Context, event events.Event) {
	ticketID, _ := event.Payload["ticket_id"].(string)
	projectID, _ := event.Payload["project_id"].(string)

	if projectID == "" || ticketID == "" {
		return
	}

	_ = s.Notify(ctx, &Notification{
		ProjectID:  projectID,
		TicketID:   ticketID,
		Category:   CategoryCalibrationReview,
		Urgency:    UrgencyMedium,
		Title:      "Ticket ready for review",
		Body:       "Ticket " + ticketID + " has been submitted and awaits review.",
		Classifier: "ticket_review",
	})
}

func (s *Service) handlePlanApproved(ctx context.Context, event events.Event) {
	projectID, _ := event.Payload["project_id"].(string)
	planID, _ := event.Payload["plan_id"].(string)

	if projectID == "" {
		return
	}

	_ = s.Notify(ctx, &Notification{
		ProjectID:  projectID,
		Category:   CategoryAutonomousAction,
		Urgency:    UrgencyLow,
		Title:      "Plan approved and executing",
		Body:       "Plan " + planID + " was approved and is now being applied.",
		Classifier: "plan_lifecycle",
	})
}

func (s *Service) handlePlanRejected(ctx context.Context, event events.Event) {
	projectID, _ := event.Payload["project_id"].(string)
	planID, _ := event.Payload["plan_id"].(string)

	if projectID == "" {
		return
	}

	_ = s.Notify(ctx, &Notification{
		ProjectID:  projectID,
		Category:   CategoryCalibrationReview,
		Urgency:    UrgencyMedium,
		Title:      "Plan rejected",
		Body:       "Plan " + planID + " was rejected. Review the feedback and resubmit.",
		Classifier: "plan_lifecycle",
	})
}
