package settings

import "github.com/gabinante/flywheel/internal/project"

// Materialize the former hidden defaults without enrolling them in custom-worker
// routing. Existing definitions always win, including their names and instructions.
func (s *Settings) ensureBuiltinWorkers(orchestrator project.DispatchWorkerProfile) {
	s.Normalize()
	defaults := []project.DispatchWorkerProfile{
		{ID: project.BuiltinDispatch, Name: "Implementation worker", Enabled: true, Driver: s.Dispatch.Driver, Model: s.Dispatch.Model, ReasoningEffort: s.Dispatch.ReasoningEffort},
		{ID: project.BuiltinReview, Name: "PR reviewer", Enabled: true, Driver: s.Review.Harness, Model: s.Review.Model, ReasoningEffort: s.Review.ReasoningEffort},
		{ID: project.BuiltinFeedback, Name: "Feedback worker", Enabled: true, Driver: s.Feedback.Harness, Model: s.Feedback.Model, ReasoningEffort: s.Feedback.ReasoningEffort},
	}
	if orchestrator.ID != "" {
		defaults = append(defaults, orchestrator)
	}
	for _, candidate := range defaults {
		found := false
		for _, w := range s.Workers.Workers {
			if w.ID == candidate.ID {
				found = true
				break
			}
		}
		if !found {
			s.Workers.Workers = append(s.Workers.Workers, candidate)
		}
	}
}

func (s Settings) builtin(id string) project.DispatchWorkerProfile {
	for _, w := range s.Workers.Workers {
		if w.ID == id {
			return w
		}
	}
	return project.DispatchWorkerProfile{}
}

func (s Settings) defaultAssignment(id, role, builtin string) string {
	if id == "" && role == "" && s.builtin(builtin).ID != "" {
		return builtin
	}
	return id
}

// Keep legacy settings clients functional. Worker edits win when both surfaces
// change; otherwise edits through an old task-settings form update its definition.
func (s *Settings) syncBuiltinSettings(previous Settings) {
	for i := range s.Workers.Workers {
		w := &s.Workers.Workers[i]
		old := previous.builtin(w.ID)
		var driver, model, effort *string
		var oldDriver, oldModel, oldEffort string
		switch w.ID {
		case project.BuiltinDispatch:
			driver, model, effort = &s.Dispatch.Driver, &s.Dispatch.Model, &s.Dispatch.ReasoningEffort
			oldDriver, oldModel, oldEffort = previous.Dispatch.Driver, previous.Dispatch.Model, previous.Dispatch.ReasoningEffort
		case project.BuiltinReview:
			driver, model, effort = &s.Review.Harness, &s.Review.Model, &s.Review.ReasoningEffort
			oldDriver, oldModel, oldEffort = previous.Review.Harness, previous.Review.Model, previous.Review.ReasoningEffort
		case project.BuiltinFeedback:
			driver, model, effort = &s.Feedback.Harness, &s.Feedback.Model, &s.Feedback.ReasoningEffort
			oldDriver, oldModel, oldEffort = previous.Feedback.Harness, previous.Feedback.Model, previous.Feedback.ReasoningEffort
		default:
			continue
		}
		if old.ID != "" && w.Driver == old.Driver && *driver != oldDriver {
			w.Driver = *driver
		}
		if old.ID != "" && w.Model == old.Model && *model != oldModel {
			w.Model = *model
		}
		if old.ID != "" && w.ReasoningEffort == old.ReasoningEffort && *effort != oldEffort {
			w.ReasoningEffort = *effort
		}
		*driver, *model, *effort = w.Driver, w.Model, w.ReasoningEffort
	}
}
