package dispatch

import "time"

// Status represents the current state of the dispatcher.
type Status struct {
	Enabled        bool      `json:"enabled"`
	ActiveWorkers  int       `json:"active_workers"`
	MaxWorkers     int       `json:"max_workers"`
	ActiveTicketIDs []string `json:"active_ticket_ids"`
	Timestamp      time.Time `json:"timestamp"`
}

// GetStatus returns the current dispatcher status.
func (d *Dispatcher) GetStatus() Status {
	d.mu.Lock()
	ids := make([]string, 0, len(d.active))
	for id := range d.active {
		ids = append(ids, id)
	}
	d.mu.Unlock()

	return Status{
		Enabled:        true,
		ActiveWorkers:  len(ids),
		MaxWorkers:     d.cfg.MaxWorkers,
		ActiveTicketIDs: ids,
		Timestamp:      time.Now().UTC(),
	}
}
