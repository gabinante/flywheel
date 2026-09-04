package linear

import (
	"strings"

	"github.com/gabinante/flywheel/internal/ticket"
)

// MapState converts a Linear workflow state into the Flywheel ticket state.
//
//	triage, backlog, unstarted → draft
//	started                    → executing, or awaiting_validation when the state is a review column
//	completed                  → closed
//	canceled                   → closed
func MapState(stateType, stateName string) ticket.State {
	switch stateType {
	case "started":
		if isReviewState(stateName) {
			return ticket.StateAwaitingValidation
		}
		return ticket.StateExecuting
	case "completed", "canceled":
		return ticket.StateClosed
	default:
		return ticket.StateDraft
	}
}

func isReviewState(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "review") || strings.Contains(n, "qa") || strings.Contains(n, "verify")
}

// stateRank orders Flywheel states so inbound sync only regresses a ticket when
// Linear itself moved backwards.
func stateRank(s ticket.State) int {
	switch s {
	case ticket.StateDraft:
		return 0
	case ticket.StatePlanning:
		return 1
	case ticket.StateAwaitingInput:
		return 2
	case ticket.StateExecuting:
		return 3
	case ticket.StateAwaitingValidation:
		return 4
	case ticket.StateValidated:
		return 5
	case ticket.StateClosed:
		return 6
	}
	return 0
}

// MapPriority converts Linear priority (0 none, 1 urgent, 2 high, 3 normal, 4 low) to P0–P3.
func MapPriority(p int) ticket.Priority {
	switch p {
	case 1:
		return ticket.P0
	case 2:
		return ticket.P1
	case 4:
		return ticket.P3
	default:
		return ticket.P2
	}
}

// ToLinearPriority converts P0–P3 back to Linear's scale.
func ToLinearPriority(p ticket.Priority) int {
	switch p {
	case ticket.P0:
		return 1
	case ticket.P1:
		return 2
	case ticket.P3:
		return 4
	default:
		return 3
	}
}

// MapType infers the ticket type from labels.
func MapType(labels []string) ticket.TicketType {
	for _, l := range labels {
		switch strings.ToLower(l) {
		case "bug", "defect", "incident":
			return ticket.TypeBug
		case "spike", "research", "investigation":
			return ticket.TypeSpike
		}
	}
	return ticket.TypeTask
}

// DesiredStateType maps a Flywheel state to the Linear state type (and preferred
// column names) Flywheel pushes when it changes a ticket.
func DesiredStateType(s ticket.State) (stateType string, preferredNames []string) {
	switch s {
	case ticket.StatePlanning:
		return "unstarted", []string{"todo", "planned", "ready"}
	case ticket.StateExecuting:
		return "started", []string{"in progress", "in development", "doing"}
	case ticket.StateAwaitingValidation:
		return "started", []string{"in review", "review", "code review", "qa"}
	case ticket.StateValidated:
		return "started", []string{"approved", "ready to merge", "in review"}
	case ticket.StateClosed:
		return "completed", []string{"done", "completed", "shipped"}
	default:
		return "", nil
	}
}

// PickState chooses the team workflow state matching a type and preferred names.
func PickState(states []WorkflowState, stateType string, preferredNames []string) *WorkflowState {
	for _, pref := range preferredNames {
		for i := range states {
			if states[i].Type == stateType && strings.EqualFold(states[i].Name, pref) {
				return &states[i]
			}
		}
	}
	for _, pref := range preferredNames {
		for i := range states {
			if states[i].Type == stateType && strings.Contains(strings.ToLower(states[i].Name), pref) {
				return &states[i]
			}
		}
	}
	var best *WorkflowState
	for i := range states {
		if states[i].Type == stateType && (best == nil || states[i].Position < best.Position) {
			best = &states[i]
		}
	}
	return best
}

// Slugify turns a Linear project name into a Flywheel project slug.
func Slugify(name string) string {
	var b strings.Builder
	lastDash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	if s == "" {
		s = "linear-project"
	}
	return s
}
