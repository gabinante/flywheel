package ticket

import "context"

// DependencyResolver can fetch tickets by IDs for dependency resolution.
type DependencyResolver interface {
	GetByIDs(ctx context.Context, ids []string) ([]*Ticket, error)
}

// ResolveDependencies returns the direct dependency tickets for t.
func ResolveDependencies(store DependencyResolver, ctx context.Context, t *Ticket) ([]*Ticket, error) {
	if len(t.DependsOn) == 0 {
		return nil, nil
	}
	return store.GetByIDs(ctx, t.DependsOn)
}

// IsUnblocked returns true when all dependency tickets are in state closed (done).
func IsUnblocked(t *Ticket, deps []*Ticket) bool {
	done := make(map[string]bool)
	for _, d := range deps {
		done[d.ID] = (d.State == StateClosed)
	}
	for _, id := range t.DependsOn {
		if !done[id] {
			return false
		}
	}
	return true
}

// GetDependencyOutputs collects outputs from closed dependency tickets keyed by dependency ticket ID,
// for injection as inputs. Returns a map from dep ticket ID to its outputs (or nil).
func GetDependencyOutputs(t *Ticket, deps []*Ticket) map[string]any {
	out := make(map[string]any)
	for _, d := range deps {
		if d.State == StateClosed && len(d.Outputs) > 0 {
			out[d.ID] = d.Outputs
		}
	}
	return out
}
