package workflow

import (
	"context"
	"fmt"
	"sync"
)

// ActionContext carries ticket/phase context into an action handler.
type ActionContext struct {
	TicketID   string
	ProjectID  string
	PhaseID    string
	WorkflowID string
	Params     map[string]any // from ActionPhaseConfig
	Outputs    map[string]any // ticket's current outputs (read-only)
	Inputs     map[string]any // ticket's current inputs (read-only)
}

// ActionResult is the outcome of an inline action execution.
type ActionResult struct {
	Outcome  string         // "success" or "failed"
	Metadata map[string]any // merged into ticket outputs + phase completion
	Error    string         // on failure
}

// ActionHandler is a function that executes inline business logic for a workflow phase.
type ActionHandler func(ctx context.Context, ac ActionContext) ActionResult

// ActionRegistry holds named action handlers for action phases.
type ActionRegistry struct {
	mu       sync.RWMutex
	handlers map[string]ActionHandler
}

// NewActionRegistry creates a new empty action registry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		handlers: make(map[string]ActionHandler),
	}
}

// Register adds a named action handler. Panics on duplicate names.
func (r *ActionRegistry) Register(name string, handler ActionHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[name]; exists {
		panic(fmt.Sprintf("action handler %q already registered", name))
	}
	r.handlers[name] = handler
}

// Execute runs a named action handler with panic recovery.
func (r *ActionRegistry) Execute(ctx context.Context, name string, ac ActionContext) (result ActionResult) {
	r.mu.RLock()
	handler, ok := r.handlers[name]
	r.mu.RUnlock()

	if !ok {
		return ActionResult{
			Outcome: "failed",
			Error:   fmt.Sprintf("unknown action handler: %s", name),
		}
	}

	defer func() {
		if rv := recover(); rv != nil {
			result = ActionResult{
				Outcome: "failed",
				Error:   fmt.Sprintf("action handler panicked: %v", rv),
			}
		}
	}()

	return handler(ctx, ac)
}

// Has returns true if a handler with the given name is registered.
func (r *ActionRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[name]
	return ok
}

// Names returns all registered handler names.
func (r *ActionRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}
