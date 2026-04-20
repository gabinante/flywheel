package observation

import (
	"context"
	"sync"
	"time"
)

// Store is the persistence interface for observation data.
// Implementations can use Postgres, Redis, or in-memory storage.
type Store interface {
	// Windows
	CreateWindow(ctx context.Context, window *ObservationWindow) error
	CloseWindow(ctx context.Context, windowID string, closedAt time.Time) error
	GetWindow(ctx context.Context, windowID string) (*ObservationWindow, error)
	GetWindowByTicket(ctx context.Context, ticketID string) (*ObservationWindow, error)
	GetOpenWindows(ctx context.Context, projectID string) ([]*ObservationWindow, error)
	GetOverlappingWindows(ctx context.Context, projectID string, start, end time.Time) ([]*ObservationWindow, error)
	// GetRecentlyClosedWindows returns windows that closed within the lookback duration.
	// Used for retroactive attribution.
	GetRecentlyClosedWindows(ctx context.Context, projectID string, lookback time.Duration) ([]*ObservationWindow, error)

	// Signals
	CreateSignal(ctx context.Context, signal *Signal) error
	GetSignal(ctx context.Context, signalID string) (*Signal, error)
	ListSignals(ctx context.Context, projectID string, since time.Time, limit int) ([]*Signal, error)

	// Attributions
	CreateAttribution(ctx context.Context, attr *Attribution) error
	GetAttribution(ctx context.Context, attrID string) (*Attribution, error)
	GetAttributionBySignal(ctx context.Context, signalID string) (*Attribution, error)
	ListUnresolvedAttributions(ctx context.Context, projectID string) ([]*Attribution, error)
	ResolveAttribution(ctx context.Context, attrID, resolvedBy, resolution string) error

	// Metrics
	GetAmbiguityMetrics(ctx context.Context, projectID string, start, end time.Time) (*AmbiguityMetrics, error)
}

// MemoryStore is an in-memory implementation of Store for testing and development.
type MemoryStore struct {
	mu           sync.RWMutex
	windows      map[string]*ObservationWindow
	signals      map[string]*Signal
	attributions map[string]*Attribution
}

// NewMemoryStore returns a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		windows:      make(map[string]*ObservationWindow),
		signals:      make(map[string]*Signal),
		attributions: make(map[string]*Attribution),
	}
}

func (m *MemoryStore) CreateWindow(_ context.Context, window *ObservationWindow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.windows[window.ID] = window
	return nil
}

func (m *MemoryStore) CloseWindow(_ context.Context, windowID string, closedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.windows[windowID]
	if !ok {
		return ErrWindowNotFound
	}
	w.ClosedAt = &closedAt
	w.State = WindowClosed
	return nil
}

func (m *MemoryStore) GetWindow(_ context.Context, windowID string) (*ObservationWindow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w, ok := m.windows[windowID]
	if !ok {
		return nil, ErrWindowNotFound
	}
	return w, nil
}

func (m *MemoryStore) GetWindowByTicket(_ context.Context, ticketID string) (*ObservationWindow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.windows {
		if w.TicketID == ticketID {
			return w, nil
		}
	}
	return nil, ErrWindowNotFound
}

func (m *MemoryStore) GetOpenWindows(_ context.Context, projectID string) ([]*ObservationWindow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*ObservationWindow
	for _, w := range m.windows {
		if w.ProjectID == projectID && w.IsOpen() {
			result = append(result, w)
		}
	}
	return result, nil
}

func (m *MemoryStore) GetOverlappingWindows(_ context.Context, projectID string, start, end time.Time) ([]*ObservationWindow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*ObservationWindow
	for _, w := range m.windows {
		if w.ProjectID == projectID && w.Overlaps(start, end) {
			result = append(result, w)
		}
	}
	return result, nil
}

func (m *MemoryStore) GetRecentlyClosedWindows(_ context.Context, projectID string, lookback time.Duration) ([]*ObservationWindow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cutoff := time.Now().UTC().Add(-lookback)
	var result []*ObservationWindow
	for _, w := range m.windows {
		if w.ProjectID == projectID && w.State == WindowClosed && w.ClosedAt != nil && w.ClosedAt.After(cutoff) {
			result = append(result, w)
		}
	}
	return result, nil
}

func (m *MemoryStore) CreateSignal(_ context.Context, signal *Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals[signal.ID] = signal
	return nil
}

func (m *MemoryStore) GetSignal(_ context.Context, signalID string) (*Signal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.signals[signalID]
	if !ok {
		return nil, ErrSignalNotFound
	}
	return s, nil
}

func (m *MemoryStore) ListSignals(_ context.Context, projectID string, since time.Time, limit int) ([]*Signal, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Signal
	for _, s := range m.signals {
		if s.ProjectID == projectID && !s.OccurredAt.Before(since) {
			result = append(result, s)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *MemoryStore) CreateAttribution(_ context.Context, attr *Attribution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attributions[attr.ID] = attr
	return nil
}

func (m *MemoryStore) GetAttribution(_ context.Context, attrID string) (*Attribution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.attributions[attrID]
	if !ok {
		return nil, ErrAttributionNotFound
	}
	return a, nil
}

func (m *MemoryStore) GetAttributionBySignal(_ context.Context, signalID string) (*Attribution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, a := range m.attributions {
		if a.SignalID == signalID {
			return a, nil
		}
	}
	return nil, ErrAttributionNotFound
}

func (m *MemoryStore) ListUnresolvedAttributions(_ context.Context, projectID string) ([]*Attribution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Attribution
	for _, a := range m.attributions {
		if a.ProjectID == projectID && !a.Resolved {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *MemoryStore) ResolveAttribution(_ context.Context, attrID, resolvedBy, resolution string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.attributions[attrID]
	if !ok {
		return ErrAttributionNotFound
	}
	now := time.Now().UTC()
	a.Resolved = true
	a.ResolvedBy = resolvedBy
	a.Resolution = resolution
	a.UpdatedAt = now
	return nil
}

func (m *MemoryStore) GetAmbiguityMetrics(_ context.Context, projectID string, start, end time.Time) (*AmbiguityMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	metrics := &AmbiguityMetrics{
		ProjectID:   projectID,
		WindowStart: start,
		WindowEnd:   end,
	}
	totalCandidates := 0
	for _, a := range m.attributions {
		if a.ProjectID != projectID {
			continue
		}
		if a.CreatedAt.Before(start) || a.CreatedAt.After(end) {
			continue
		}
		metrics.TotalAttributions++
		if a.IsLowConfidence() {
			metrics.LowConfidenceCount++
		}
		totalCandidates += len(a.Candidates)
	}
	if metrics.TotalAttributions > 0 {
		metrics.AmbiguityRate = float64(metrics.LowConfidenceCount) / float64(metrics.TotalAttributions)
		metrics.AvgCandidatesPerSignal = float64(totalCandidates) / float64(metrics.TotalAttributions)
	}
	return metrics, nil
}
