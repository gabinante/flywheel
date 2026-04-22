package stream

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Store is the persistence interface for append-only streams.
type Store interface {
	// Entity stream
	AppendEntity(ctx context.Context, event *EntityEvent) error
	GetEntity(ctx context.Context, id string) (*EntityEvent, error)
	ListEntity(ctx context.Context, q StreamQuery) (*StreamPage[EntityEvent], error)

	// State stream
	AppendState(ctx context.Context, event *StateEvent) error
	GetState(ctx context.Context, id string) (*StateEvent, error)
	ListState(ctx context.Context, q StreamQuery) (*StreamPage[StateEvent], error)

	// Change stream
	AppendChange(ctx context.Context, event *ChangeEvent) error
	GetChange(ctx context.Context, id string) (*ChangeEvent, error)
	ListChange(ctx context.Context, q StreamQuery) (*StreamPage[ChangeEvent], error)
}

// MemoryStore is an in-memory implementation of Store for testing.
type MemoryStore struct {
	mu       sync.RWMutex
	entities []*EntityEvent
	states   []*StateEvent
	changes  []*ChangeEvent
}

// NewMemoryStore returns a new in-memory stream store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

// --- Entity stream (in-memory) ---

func (m *MemoryStore) AppendEntity(_ context.Context, event *EntityEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entities = append(m.entities, event)
	return nil
}

func (m *MemoryStore) GetEntity(_ context.Context, id string) (*EntityEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.entities {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, ErrEventNotFound
}

func (m *MemoryStore) ListEntity(_ context.Context, q StreamQuery) (*StreamPage[EntityEvent], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []*EntityEvent
	for _, e := range m.entities {
		if q.ProjectID != "" && e.ProjectID != q.ProjectID {
			continue
		}
		if q.EntityID != "" && e.EntityID != q.EntityID {
			continue
		}
		if q.ChangeType != "" && string(e.ChangeType) != q.ChangeType {
			continue
		}
		if q.Environment != "" && e.Environment != q.Environment {
			continue
		}
		if q.InitiatorID != "" && e.InitiatorID != q.InitiatorID {
			continue
		}
		if !q.Since.IsZero() && e.Timestamp.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && e.Timestamp.After(q.Until) {
			continue
		}
		filtered = append(filtered, e)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	total := len(filtered)
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := q.Offset
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return &StreamPage[EntityEvent]{
		Events: filtered[offset:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// --- State stream (in-memory) ---

func (m *MemoryStore) AppendState(_ context.Context, event *StateEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, event)
	return nil
}

func (m *MemoryStore) GetState(_ context.Context, id string) (*StateEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.states {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, ErrEventNotFound
}

func (m *MemoryStore) ListState(_ context.Context, q StreamQuery) (*StreamPage[StateEvent], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []*StateEvent
	for _, e := range m.states {
		if q.ProjectID != "" && e.ProjectID != q.ProjectID {
			continue
		}
		if q.EntityID != "" && e.EntityID != q.EntityID {
			continue
		}
		if q.ChangeType != "" && string(e.ChangeType) != q.ChangeType {
			continue
		}
		if q.Environment != "" && e.Environment != q.Environment {
			continue
		}
		if q.InitiatorID != "" && e.InitiatorID != q.InitiatorID {
			continue
		}
		if q.Source != "" && e.Source != q.Source {
			continue
		}
		if !q.Since.IsZero() && e.Timestamp.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && e.Timestamp.After(q.Until) {
			continue
		}
		filtered = append(filtered, e)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	total := len(filtered)
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := q.Offset
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return &StreamPage[StateEvent]{
		Events: filtered[offset:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// --- Change stream (in-memory) ---

func (m *MemoryStore) AppendChange(_ context.Context, event *ChangeEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.changes = append(m.changes, event)
	return nil
}

func (m *MemoryStore) GetChange(_ context.Context, id string) (*ChangeEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, e := range m.changes {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, ErrEventNotFound
}

func (m *MemoryStore) ListChange(_ context.Context, q StreamQuery) (*StreamPage[ChangeEvent], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []*ChangeEvent
	for _, e := range m.changes {
		if q.ProjectID != "" && e.ProjectID != q.ProjectID {
			continue
		}
		if q.ChangeType != "" && e.ChangeType != q.ChangeType {
			continue
		}
		if q.Environment != "" && e.Environment != q.Environment {
			continue
		}
		if q.InitiatorID != "" && e.InitiatorID != q.InitiatorID {
			continue
		}
		if q.Source != "" && e.Source != q.Source {
			continue
		}
		if !q.Since.IsZero() && e.Timestamp.Before(q.Since) {
			continue
		}
		if !q.Until.IsZero() && e.Timestamp.After(q.Until) {
			continue
		}
		// Check affected_entities filter
		if q.EntityID != "" {
			found := false
			for _, eid := range e.AffectedEntities {
				if eid == q.EntityID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	total := len(filtered)
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := q.Offset
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return &StreamPage[ChangeEvent]{
		Events: filtered[offset:end],
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// compile-time interface check
var _ Store = (*MemoryStore)(nil)

// generateID creates a time-ordered unique ID.
func generateID() string {
	now := time.Now().UTC()
	return now.Format("20060102150405") + "-" + randomSuffix(8)
}

func randomSuffix(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return string(b)
}
