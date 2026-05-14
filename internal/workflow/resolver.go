package workflow

import "context"

// Resolver implements ticket.WorkflowResolver using the workflow engine.
type Resolver struct {
	engine *Engine
}

// NewResolver returns a new Resolver wrapping the workflow engine.
func NewResolver(engine *Engine) *Resolver {
	return &Resolver{engine: engine}
}

// ResolveForProject returns the effective workflow ID, version, and first phase ID for a project.
// Returns empty strings if no active workflow exists.
func (r *Resolver) ResolveForProject(ctx context.Context, orgID, projectID string) (workflowID string, version int, firstPhaseID string, err error) {
	def, err := r.engine.Resolve(ctx, orgID, projectID)
	if err != nil {
		return "", 0, "", err
	}
	if def == nil || len(def.Phases) == 0 {
		return "", 0, "", nil
	}
	return def.ID, def.Version, def.Phases[0].ID, nil
}
