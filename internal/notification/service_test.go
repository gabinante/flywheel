package notification

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gabinante/flywheel/events"
)

// --- In-memory store for testing ---

type memStore struct {
	mu             sync.Mutex
	notifications  map[string]*Notification
	preferences    map[string]*Preferences
	dismissalRates map[string]*DismissalRate
}

func newMemStore() *memStore {
	return &memStore{
		notifications:  make(map[string]*Notification),
		preferences:    make(map[string]*Preferences),
		dismissalRates: make(map[string]*DismissalRate),
	}
}

func (s *memStore) CreateNotification(_ context.Context, n *Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifications[n.ID] = n
	return nil
}

func (s *memStore) GetNotification(_ context.Context, id string) (*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notifications[id]
	if !ok {
		return nil, context.DeadlineExceeded // stand-in for not found
	}
	return n, nil
}

func (s *memStore) UpdateNotificationStatus(_ context.Context, id string, status Status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.notifications[id]; ok {
		n.Status = status
		n.ErrorMessage = errMsg
		if status == StatusSent {
			now := time.Now().UTC()
			n.SentAt = &now
		}
	}
	return nil
}

func (s *memStore) ListNotifications(_ context.Context, projectID string, limit, offset int) ([]*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Notification
	for _, n := range s.notifications {
		if n.ProjectID == projectID {
			result = append(result, n)
		}
	}
	if offset >= len(result) {
		return nil, nil
	}
	result = result[offset:]
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *memStore) ListPendingDigest(_ context.Context, projectID string) ([]*Notification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Notification
	for _, n := range s.notifications {
		if n.ProjectID == projectID && n.Routing == RoutingDigest && n.Status == StatusPending {
			result = append(result, n)
		}
	}
	return result, nil
}

func (s *memStore) MarkDigested(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if n, ok := s.notifications[id]; ok {
			n.Status = StatusDigested
		}
	}
	return nil
}

func (s *memStore) DismissNotification(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.notifications[id]; ok {
		n.Dismissed = true
		now := time.Now().UTC()
		n.DismissedAt = &now
	}
	return nil
}

func (s *memStore) GetPreferences(_ context.Context, projectID string) (*Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.preferences[projectID]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return p, nil
}

func (s *memStore) UpsertPreferences(_ context.Context, prefs *Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preferences[prefs.ProjectID] = prefs
	return nil
}

func (s *memStore) IncrementSent(_ context.Context, classifier, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := classifier + ":" + projectID
	d, ok := s.dismissalRates[key]
	if !ok {
		d = &DismissalRate{Classifier: classifier, ProjectID: projectID}
		s.dismissalRates[key] = d
	}
	d.TotalSent++
	d.WindowSent++
	return nil
}

func (s *memStore) IncrementDismissed(_ context.Context, classifier, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := classifier + ":" + projectID
	d, ok := s.dismissalRates[key]
	if !ok {
		d = &DismissalRate{Classifier: classifier, ProjectID: projectID}
		s.dismissalRates[key] = d
	}
	d.TotalDismissed++
	d.WindowDismissed++
	return nil
}

func (s *memStore) GetDismissalRate(_ context.Context, classifier, projectID string) (*DismissalRate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := classifier + ":" + projectID
	d, ok := s.dismissalRates[key]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return d, nil
}

func (s *memStore) ListDismissalRates(_ context.Context, projectID string) ([]*DismissalRate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*DismissalRate
	for _, d := range s.dismissalRates {
		if d.ProjectID == projectID {
			result = append(result, d)
		}
	}
	return result, nil
}

// --- Mock adapter ---

type mockAdapter struct {
	mu   sync.Mutex
	sent []*Notification
	name string
}

func newMockAdapter(name string) *mockAdapter {
	return &mockAdapter{name: name}
}

func (a *mockAdapter) Name() string { return a.name }

func (a *mockAdapter) Send(_ context.Context, n *Notification, _ *Preferences) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = append(a.sent, n)
	return nil
}

func (a *mockAdapter) SendDigest(_ context.Context, notifications []*Notification, _ *Preferences) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = append(a.sent, notifications...)
	return nil
}

func (a *mockAdapter) sentCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.sent)
}

// --- Tests ---

func TestNotifyPushRouting(t *testing.T) {
	store := newMemStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	adapter := newMockAdapter("slack")
	svc.RegisterAdapter(ChannelSlack, adapter)

	// Set preferences with push threshold = high.
	err := svc.SetPreferences(context.Background(), &Preferences{
		ProjectID:       "proj-1",
		CriticalChannel: ChannelSlack,
		HighChannel:     ChannelSlack,
		PushThreshold:   UrgencyHigh,
		DigestEnabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Critical urgency should be pushed immediately.
	err = svc.Notify(context.Background(), &Notification{
		ProjectID:  "proj-1",
		Category:   CategoryUrgentDecision,
		Urgency:    UrgencyCritical,
		Title:      "Critical alert",
		Classifier: "test_classifier",
	})
	if err != nil {
		t.Fatal(err)
	}

	if adapter.sentCount() != 1 {
		t.Errorf("expected 1 push, got %d", adapter.sentCount())
	}

	// Check that notification was created in store.
	notifications, err := svc.ListNotifications(context.Background(), "proj-1", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 {
		t.Errorf("expected 1 notification in store, got %d", len(notifications))
	}
	if notifications[0].Routing != RoutingPush {
		t.Errorf("expected push routing, got %s", notifications[0].Routing)
	}
}

func TestNotifyDigestRouting(t *testing.T) {
	store := newMemStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	adapter := newMockAdapter("slack")
	svc.RegisterAdapter(ChannelSlack, adapter)

	// Set preferences: push only critical.
	err := svc.SetPreferences(context.Background(), &Preferences{
		ProjectID:     "proj-1",
		LowChannel:    ChannelSlack,
		PushThreshold: UrgencyCritical,
		DigestEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Low urgency should be routed to digest.
	err = svc.Notify(context.Background(), &Notification{
		ProjectID: "proj-1",
		Category:  CategoryAutonomousAction,
		Urgency:   UrgencyLow,
		Title:     "Low priority info",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Adapter should NOT have been called yet (digest, not push).
	if adapter.sentCount() != 0 {
		t.Errorf("expected 0 pushes for digest notification, got %d", adapter.sentCount())
	}

	// Now flush digest.
	err = svc.FlushDigest(context.Background(), "proj-1")
	if err != nil {
		t.Fatal(err)
	}

	if adapter.sentCount() != 1 {
		t.Errorf("expected 1 digest delivery after flush, got %d", adapter.sentCount())
	}
}

func TestDismissalRateTracking(t *testing.T) {
	store := newMemStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	adapter := newMockAdapter("slack")
	svc.RegisterAdapter(ChannelSlack, adapter)

	ctx := context.Background()

	// Send a few notifications with a classifier.
	for i := 0; i < 5; i++ {
		_ = svc.Notify(ctx, &Notification{
			ProjectID:  "proj-1",
			Category:   CategoryAnomalyAlert,
			Urgency:    UrgencyHigh,
			Title:      "Alert",
			Classifier: "test_classifier",
		})
	}

	// Check dismissal rate.
	rates, err := svc.GetDismissalRates(ctx, "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rates) != 1 {
		t.Fatalf("expected 1 rate entry, got %d", len(rates))
	}
	if rates[0].TotalSent != 5 {
		t.Errorf("expected 5 sent, got %d", rates[0].TotalSent)
	}
	if rates[0].Rate() != 0 {
		t.Errorf("expected 0%% dismissal rate, got %f", rates[0].Rate())
	}
}

func TestPreferencesDefaults(t *testing.T) {
	store := newMemStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)

	// No preferences set — should get defaults.
	prefs, err := svc.GetPreferences(context.Background(), "proj-new")
	if err != nil {
		t.Fatal(err)
	}
	if prefs.PushThreshold != UrgencyHigh {
		t.Errorf("expected default push threshold high, got %s", prefs.PushThreshold)
	}
	if prefs.CriticalChannel != ChannelSlack {
		t.Errorf("expected default critical channel slack, got %s", prefs.CriticalChannel)
	}
}

func TestUrgencyRanking(t *testing.T) {
	if UrgencyRank(UrgencyCritical) >= UrgencyRank(UrgencyHigh) {
		t.Error("critical should rank higher (lower number) than high")
	}
	if UrgencyRank(UrgencyHigh) >= UrgencyRank(UrgencyMedium) {
		t.Error("high should rank higher than medium")
	}
	if UrgencyRank(UrgencyMedium) >= UrgencyRank(UrgencyLow) {
		t.Error("medium should rank higher than low")
	}
}

func TestPreferencesShouldPush(t *testing.T) {
	prefs := &Preferences{PushThreshold: UrgencyHigh}

	if !prefs.ShouldPush(UrgencyCritical) {
		t.Error("critical should push when threshold is high")
	}
	if !prefs.ShouldPush(UrgencyHigh) {
		t.Error("high should push when threshold is high")
	}
	if prefs.ShouldPush(UrgencyMedium) {
		t.Error("medium should NOT push when threshold is high")
	}
	if prefs.ShouldPush(UrgencyLow) {
		t.Error("low should NOT push when threshold is high")
	}
}

func TestEventDrivenEscalation(t *testing.T) {
	store := newMemStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	adapter := newMockAdapter("slack")
	svc.RegisterAdapter(ChannelSlack, adapter)

	// Simulate an escalation event.
	_ = bus.Publish(context.Background(), events.Event{
		Type: events.EventTicketEscalated,
		Payload: map[string]any{
			"ticket_id":  "ticket-1",
			"project_id": "proj-1",
			"reason":     "agent needs help",
		},
	})

	// The service should have created a critical urgent_decision notification.
	if adapter.sentCount() != 1 {
		t.Errorf("expected 1 notification from escalation event, got %d", adapter.sentCount())
	}
}

func TestNoAdapterFallback(t *testing.T) {
	store := newMemStore()
	bus := events.NewInProcessBus()
	svc := NewService(store, bus)
	// No adapter registered.

	err := svc.Notify(context.Background(), &Notification{
		ProjectID: "proj-1",
		Category:  CategoryAnomalyAlert,
		Urgency:   UrgencyCritical,
		Title:     "Test alert",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Should have been created but marked failed.
	notifications, _ := svc.ListNotifications(context.Background(), "proj-1", 10, 0)
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].Status != StatusFailed {
		t.Errorf("expected failed status, got %s", notifications[0].Status)
	}
}
