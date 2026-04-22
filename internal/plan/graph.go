package plan

import (
	"fmt"
	"strings"
)

// PlanGraph represents a multi-step plan with inter-step dependencies.
// Used for coordinated changes like: schema migration → code write →
// code read behind flag → flag ramp → cleanup.
//
// Each step is a Plan (with its own backend, content, classification).
// Dependencies are explicit edges: step B depends on step A completing.
// The graph is a DAG — cycles are rejected at validation time.
type PlanGraph struct {
	// ID is the unique identifier for this plan graph.
	ID string `json:"id"`
	// TicketID links the graph to the originating ticket.
	TicketID string `json:"ticket_id"`
	// Name is a human-readable name for the coordinated change.
	Name string `json:"name"`
	// Steps is the ordered set of plan steps in this graph.
	Steps []PlanStep `json:"steps"`
	// State is the overall graph state: "pending", "executing", "completed", "failed", "cancelled".
	State PlanGraphState `json:"state"`
}

// PlanGraphState represents the lifecycle state of a plan graph.
type PlanGraphState string

const (
	GraphStatePending   PlanGraphState = "pending"
	GraphStateExecuting PlanGraphState = "executing"
	GraphStateCompleted PlanGraphState = "completed"
	GraphStateFailed    PlanGraphState = "failed"
	GraphStateCancelled PlanGraphState = "cancelled"
)

// PlanStep is a single step within a multi-step plan graph.
type PlanStep struct {
	// StepID is the unique identifier within this graph (e.g., "step-1", "migrate-db").
	StepID string `json:"step_id"`
	// PlanID references the Plan entity for this step.
	PlanID string `json:"plan_id"`
	// Backend is the plan backend for this step.
	Backend Backend `json:"backend"`
	// Name is a human-readable description of this step.
	Name string `json:"name"`
	// DependsOn lists step IDs that must complete before this step can execute.
	DependsOn []string `json:"depends_on,omitempty"`
	// State is the step's execution state.
	State PlanStepState `json:"state"`
	// Ordinal is the execution order hint (steps with same deps run in ordinal order).
	Ordinal int `json:"ordinal"`
}

// PlanStepState represents the lifecycle state of a plan step.
type PlanStepState string

const (
	StepStatePending   PlanStepState = "pending"
	StepStateReady     PlanStepState = "ready"     // All dependencies met.
	StepStateExecuting PlanStepState = "executing"
	StepStateCompleted PlanStepState = "completed"
	StepStateFailed    PlanStepState = "failed"
	StepStateSkipped   PlanStepState = "skipped" // Skipped due to upstream failure.
)

// ValidateGraph validates a PlanGraph for structural correctness.
// Returns an error if:
// - No steps defined
// - Step IDs are not unique
// - Dependencies reference non-existent steps
// - The graph contains cycles
// - Backend types are invalid
func ValidateGraph(g *PlanGraph) error {
	if len(g.Steps) == 0 {
		return fmt.Errorf("plan graph must have at least one step")
	}

	stepIDs := make(map[string]bool)
	for _, step := range g.Steps {
		if strings.TrimSpace(step.StepID) == "" {
			return fmt.Errorf("step_id is required for all steps")
		}
		if stepIDs[step.StepID] {
			return fmt.Errorf("duplicate step_id: %s", step.StepID)
		}
		stepIDs[step.StepID] = true

		if !IsValidBackend(step.Backend) {
			return fmt.Errorf("step %s has invalid backend: %s", step.StepID, step.Backend)
		}
	}

	// Check dependency references.
	for _, step := range g.Steps {
		for _, dep := range step.DependsOn {
			if !stepIDs[dep] {
				return fmt.Errorf("step %s depends on non-existent step: %s", step.StepID, dep)
			}
			if dep == step.StepID {
				return fmt.Errorf("step %s depends on itself", step.StepID)
			}
		}
	}

	// Detect cycles using topological sort (Kahn's algorithm).
	if err := detectCycles(g.Steps); err != nil {
		return err
	}

	return nil
}

// detectCycles checks for cycles in the step dependency graph using Kahn's algorithm.
func detectCycles(steps []PlanStep) error {
	// Build adjacency list and in-degree map.
	inDegree := make(map[string]int)
	successors := make(map[string][]string)

	for _, step := range steps {
		if _, ok := inDegree[step.StepID]; !ok {
			inDegree[step.StepID] = 0
		}
		for _, dep := range step.DependsOn {
			successors[dep] = append(successors[dep], step.StepID)
			inDegree[step.StepID]++
		}
	}

	// Queue starts with nodes that have no incoming edges.
	var queue []string
	for _, step := range steps {
		if inDegree[step.StepID] == 0 {
			queue = append(queue, step.StepID)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++

		for _, succ := range successors[node] {
			inDegree[succ]--
			if inDegree[succ] == 0 {
				queue = append(queue, succ)
			}
		}
	}

	if visited != len(steps) {
		return fmt.Errorf("plan graph contains a cycle — %d of %d steps could not be ordered", len(steps)-visited, len(steps))
	}

	return nil
}

// ReadySteps returns all steps whose dependencies are satisfied (all deps completed).
func ReadySteps(g *PlanGraph) []PlanStep {
	completed := make(map[string]bool)
	for _, step := range g.Steps {
		if step.State == StepStateCompleted {
			completed[step.StepID] = true
		}
	}

	var ready []PlanStep
	for _, step := range g.Steps {
		if step.State != StepStatePending {
			continue
		}
		allDepsMet := true
		for _, dep := range step.DependsOn {
			if !completed[dep] {
				allDepsMet = false
				break
			}
		}
		if allDepsMet {
			ready = append(ready, step)
		}
	}
	return ready
}

// TopologicalOrder returns the steps in a valid execution order.
// Steps with no dependencies come first; then their dependents; etc.
func TopologicalOrder(g *PlanGraph) ([]PlanStep, error) {
	if err := ValidateGraph(g); err != nil {
		return nil, err
	}

	inDegree := make(map[string]int)
	successors := make(map[string][]string)
	stepByID := make(map[string]PlanStep)

	for _, step := range g.Steps {
		stepByID[step.StepID] = step
		if _, ok := inDegree[step.StepID]; !ok {
			inDegree[step.StepID] = 0
		}
		for _, dep := range step.DependsOn {
			successors[dep] = append(successors[dep], step.StepID)
			inDegree[step.StepID]++
		}
	}

	var queue []string
	for _, step := range g.Steps {
		if inDegree[step.StepID] == 0 {
			queue = append(queue, step.StepID)
		}
	}

	var result []PlanStep
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		result = append(result, stepByID[node])

		for _, succ := range successors[node] {
			inDegree[succ]--
			if inDegree[succ] == 0 {
				queue = append(queue, succ)
			}
		}
	}

	return result, nil
}

// FailStep marks a step as failed and cascades to skip all downstream dependents.
func FailStep(g *PlanGraph, failedStepID string) {
	// Mark the step as failed.
	for i, step := range g.Steps {
		if step.StepID == failedStepID {
			g.Steps[i].State = StepStateFailed
			break
		}
	}

	// BFS to find all transitive dependents and skip them.
	failed := map[string]bool{failedStepID: true}
	changed := true
	for changed {
		changed = false
		for i, step := range g.Steps {
			if step.State == StepStatePending || step.State == StepStateReady {
				for _, dep := range step.DependsOn {
					if failed[dep] {
						g.Steps[i].State = StepStateSkipped
						failed[step.StepID] = true
						changed = true
						break
					}
				}
			}
		}
	}

	// Update graph state.
	g.State = GraphStateFailed
}

// IsComplete returns true if all steps are in a terminal state (completed, failed, skipped).
func IsComplete(g *PlanGraph) bool {
	for _, step := range g.Steps {
		switch step.State {
		case StepStatePending, StepStateReady, StepStateExecuting:
			return false
		}
	}
	return true
}
