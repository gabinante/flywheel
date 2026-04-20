package cost

import (
	"context"
	"sync"
)

// MemStore is an in-memory implementation of Store for testing and lightweight deployments.
// In production, this would be backed by Postgres.
type MemStore struct {
	mu      sync.RWMutex
	calls   []*LLMCallRecord
	budgets map[string]*Budget // key: projectID:ticketID:month
	alerts  []*BudgetAlert
	limits  []*RateLimitEvent
}

// NewMemStore creates a new in-memory cost store.
func NewMemStore() *MemStore {
	return &MemStore{
		budgets: make(map[string]*Budget),
	}
}

func budgetKey(projectID, ticketID, month string) string {
	return projectID + ":" + ticketID + ":" + month
}

// RecordCall stores an LLM call record.
func (s *MemStore) RecordCall(_ context.Context, record *LLMCallRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, record)
	return nil
}

// GetCallsByTicket returns all calls for a ticket.
func (s *MemStore) GetCallsByTicket(_ context.Context, ticketID string) ([]*LLMCallRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*LLMCallRecord
	for _, c := range s.calls {
		if c.TicketID == ticketID {
			result = append(result, c)
		}
	}
	return result, nil
}

// GetCallsByProject returns all calls for a project in a given month.
func (s *MemStore) GetCallsByProject(_ context.Context, projectID string, month string) ([]*LLMCallRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*LLMCallRecord
	for _, c := range s.calls {
		if c.ProjectID == projectID {
			if month != "" {
				callMonth := c.Timestamp.Format("2006-01")
				if callMonth != month {
					continue
				}
			}
			result = append(result, c)
		}
	}
	return result, nil
}

// SetBudget creates or updates a budget.
func (s *MemStore) SetBudget(_ context.Context, budget *Budget) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := budgetKey(budget.ProjectID, budget.TicketID, budget.Month)
	s.budgets[key] = budget
	return nil
}

// GetBudget retrieves a budget by scope.
func (s *MemStore) GetBudget(_ context.Context, projectID, ticketID, month string) (*Budget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := budgetKey(projectID, ticketID, month)
	b := s.budgets[key]
	return b, nil
}

// GetBudgetsByProject returns all budgets for a project.
func (s *MemStore) GetBudgetsByProject(_ context.Context, projectID string) ([]*Budget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Budget
	for _, b := range s.budgets {
		if b.ProjectID == projectID {
			result = append(result, b)
		}
	}
	return result, nil
}

// SumByProject returns the total cost for a project in a month.
func (s *MemStore) SumByProject(_ context.Context, projectID string, month string) (Unit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var total Unit
	for _, c := range s.calls {
		if c.ProjectID == projectID {
			if month != "" {
				callMonth := c.Timestamp.Format("2006-01")
				if callMonth != month {
					continue
				}
			}
			total += c.CostMillicent
		}
	}
	return total, nil
}

// SumByTicket returns the total cost for a ticket.
func (s *MemStore) SumByTicket(_ context.Context, ticketID string) (Unit, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var total Unit
	for _, c := range s.calls {
		if c.TicketID == ticketID {
			total += c.CostMillicent
		}
	}
	return total, nil
}

// SumByProjectGrouped returns a detailed cost breakdown.
func (s *MemStore) SumByProjectGrouped(_ context.Context, projectID string, month string) (*CostSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := &CostSummary{
		ProjectID:  projectID,
		Month:      month,
		ByTicket:   make(map[string]Unit),
		ByRole:     make(map[string]Unit),
		ByModel:    make(map[string]Unit),
		ByProvider: make(map[string]Unit),
	}

	for _, c := range s.calls {
		if c.ProjectID != projectID {
			continue
		}
		if month != "" {
			callMonth := c.Timestamp.Format("2006-01")
			if callMonth != month {
				continue
			}
		}
		summary.TotalMilli += c.CostMillicent
		summary.CallCount++
		summary.ByTicket[c.TicketID] += c.CostMillicent
		summary.ByRole[c.WorkerRole] += c.CostMillicent
		summary.ByModel[c.Model] += c.CostMillicent
		summary.ByProvider[c.Provider] += c.CostMillicent
	}

	summary.TotalDollars = summary.TotalMilli.ToDollars()
	return summary, nil
}

// CreateAlert stores a budget alert.
func (s *MemStore) CreateAlert(_ context.Context, alert *BudgetAlert) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts = append(s.alerts, alert)
	return nil
}

// GetActiveAlerts returns unacknowledged alerts for a project.
func (s *MemStore) GetActiveAlerts(_ context.Context, projectID string) ([]*BudgetAlert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*BudgetAlert
	for _, a := range s.alerts {
		if a.ProjectID == projectID && !a.Acknowledged {
			result = append(result, a)
		}
	}
	return result, nil
}

// AcknowledgeAlert marks an alert as acknowledged.
func (s *MemStore) AcknowledgeAlert(_ context.Context, alertID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, a := range s.alerts {
		if a.ID == alertID {
			a.Acknowledged = true
			return nil
		}
	}
	return nil
}

// RecordRateLimitEvent stores a rate limit event.
func (s *MemStore) RecordRateLimitEvent(_ context.Context, event *RateLimitEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limits = append(s.limits, event)
	return nil
}

// MarkRateLimitResumed marks a rate limit event as resumed.
func (s *MemStore) MarkRateLimitResumed(_ context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.limits {
		if e.ID == eventID {
			e.Resumed = true
			return nil
		}
	}
	return nil
}

// GetActiveRateLimits returns non-resumed rate limit events.
func (s *MemStore) GetActiveRateLimits(_ context.Context, projectID string) ([]*RateLimitEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*RateLimitEvent
	for _, e := range s.limits {
		if e.ProjectID == projectID && !e.Resumed {
			result = append(result, e)
		}
	}
	return result, nil
}
