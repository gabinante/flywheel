package ticket

import (
	"errors"
	"fmt"
)

// Trigger constants for state transitions (spec v0.2).
const (
	// --- Core lifecycle triggers ---
	TriggerSpec     = "spec"     // draft → specced
	TriggerPlan     = "plan"     // specced → planning
	TriggerStart    = "start"    // planning → executing
	TriggerSubmit   = "submit"   // executing → awaiting_validation
	TriggerValidate = "validate" // awaiting_validation → validated
	TriggerDeploy   = "deploy"   // validated → deploying
	TriggerObserve  = "observe"  // deploying → observing
	TriggerClose    = "close"    // observing → closed

	// --- Input / escalation triggers ---
	TriggerRequestInput = "request_input" // planning|executing → awaiting_input
	TriggerProvideInput = "provide_input" // awaiting_input → planning or executing
	TriggerEscalate     = "escalate"      // executing → awaiting_input (agent needs help)

	// --- Failure arcs ---
	TriggerReplan     = "replan"     // executing → planning
	TriggerInvalidate = "invalidate" // validated → planning

	// --- Review / approval triggers ---
	TriggerApprove = "approve" // awaiting_validation → validated (also resolves awaiting_input)
	TriggerReject  = "reject"  // awaiting_validation → executing

	// --- Rollback trigger ---
	TriggerRollback = "rollback" // executing|awaiting_validation|validated|deploying|observing → draft

	// --- Operational triggers ---
	TriggerClaim        = "claim"         // draft → planning (agent claims work)
	TriggerCancel       = "cancel"        // any early state → closed
	TriggerLeaseExpired = "lease_expired" // planning|executing → draft
	TriggerFail         = "fail"          // executing → draft (unrecoverable for this attempt)
	TriggerReopen       = "reopen"        // closed → draft

	// Deprecated trigger alias for backward compat.
	TriggerReopenReview = TriggerReopen // was done → awaiting_review, now closed → draft
)

// ActorType is who is performing the action.
type ActorType string

const (
	ActorHuman  ActorType = "human"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

// Actor identifies who is performing the transition.
type Actor struct {
	ID   string
	Type ActorType
}

// GuardFn returns an error if the transition is not allowed.
type GuardFn func(t *Ticket, actor Actor, payload map[string]any, deps []*Ticket) error

// Transition defines a valid state transition.
type Transition struct {
	From    State
	To      State
	Trigger string
	Guards  []GuardFn
}

// StateMachine holds all valid transitions and applies them.
type StateMachine struct {
	transitions []Transition
}

// NewStateMachine returns a state machine with all spec v0.2 transitions.
func NewStateMachine() *StateMachine {
	sm := &StateMachine{}
	sm.transitions = []Transition{
		// === Happy path (full SDLC lifecycle) ===
		{StateDraft, StateSpecced, TriggerSpec, []GuardFn{}},
		{StateSpecced, StatePlanning, TriggerPlan, []GuardFn{}},
		{StatePlanning, StateExecuting, TriggerStart, []GuardFn{guardIsLeaseholder}},
		{StateExecuting, StateAwaitingValidation, TriggerSubmit, []GuardFn{guardIsLeaseholder, guardOutputsPresent}},
		{StateAwaitingValidation, StateValidated, TriggerValidate, []GuardFn{guardIsHuman}},
		{StateValidated, StateDeploying, TriggerDeploy, []GuardFn{}},
		{StateDeploying, StateObserving, TriggerObserve, []GuardFn{}},
		{StateObserving, StateClosed, TriggerClose, []GuardFn{}},

		// === Claim shortcut (draft → planning, skipping specced for pre-specced tickets) ===
		{StateDraft, StatePlanning, TriggerClaim, []GuardFn{guardDependenciesMet, guardNoActiveLease}},
		// Also allow claim from specced → planning.
		{StateSpecced, StatePlanning, TriggerClaim, []GuardFn{guardDependenciesMet, guardNoActiveLease}},

		// === Input / escalation ===
		{StatePlanning, StateAwaitingInput, TriggerRequestInput, []GuardFn{guardIsLeaseholder}},
		{StateExecuting, StateAwaitingInput, TriggerRequestInput, []GuardFn{guardIsLeaseholder}},
		{StateExecuting, StateAwaitingInput, TriggerEscalate, []GuardFn{guardIsLeaseholder, guardEscalationReasonPresent}},
		// Resolve awaiting_input: back to executing (with resume_state payload) or planning (default).
		{StateAwaitingInput, StateExecuting, TriggerProvideInput, []GuardFn{guardIsHuman, guardResumeExec}},
		{StateAwaitingInput, StatePlanning, TriggerProvideInput, []GuardFn{guardIsHuman}},
		// Approve also resolves awaiting_input → executing (backward compat with escalation flow).
		{StateAwaitingInput, StateExecuting, TriggerApprove, []GuardFn{guardIsHuman}},

		// === Failure arcs ===
		{StateExecuting, StatePlanning, TriggerReplan, []GuardFn{guardIsLeaseholder}},
		{StateValidated, StatePlanning, TriggerInvalidate, []GuardFn{guardIsHuman}},

		// === Rejection ===
		{StateAwaitingValidation, StateExecuting, TriggerReject, []GuardFn{guardIsHuman}},

		// === Approval (also works as validate alias) ===
		{StateAwaitingValidation, StateValidated, TriggerApprove, []GuardFn{guardIsHuman}},

		// === Rollback (stage-specific rollback to draft; side effects handled by rollback service) ===
		{StateExecuting, StateDraft, TriggerRollback, []GuardFn{}},
		{StateAwaitingValidation, StateDraft, TriggerRollback, []GuardFn{}},
		{StateValidated, StateDraft, TriggerRollback, []GuardFn{}},
		{StateDeploying, StateDraft, TriggerRollback, []GuardFn{}},
		{StateObserving, StateDraft, TriggerRollback, []GuardFn{}},

		// === Cancel (can cancel from early states) ===
		{StateDraft, StateClosed, TriggerCancel, []GuardFn{}},
		{StateSpecced, StateClosed, TriggerCancel, []GuardFn{}},
		{StatePlanning, StateClosed, TriggerCancel, []GuardFn{}},
		{StateExecuting, StateClosed, TriggerCancel, []GuardFn{}},

		// === Lease expiry (returns to draft for re-claiming) ===
		{StatePlanning, StateDraft, TriggerLeaseExpired, []GuardFn{}},
		{StateExecuting, StateDraft, TriggerLeaseExpired, []GuardFn{guardSystemOnly}},

		// === Fail (executing → draft, can be retried) ===
		{StateExecuting, StateDraft, TriggerFail, []GuardFn{}},

		// === Reopen (closed → draft, human only) ===
		{StateClosed, StateDraft, TriggerReopen, []GuardFn{guardIsHuman}},
	}
	return sm
}

// Transition finds the matching transition, runs guards, and returns the new state or error.
// When multiple transitions share the same From+Trigger, it tries each in order until
// one passes all guards (first-match with fallthrough on guard failure).
func (sm *StateMachine) Transition(t *Ticket, trigger string, actor Actor, payload map[string]any, deps []*Ticket) (State, error) {
	if payload == nil {
		payload = make(map[string]any)
	}
	var lastErr error
	matched := false
	for _, tr := range sm.transitions {
		if tr.From == t.State && tr.Trigger == trigger {
			matched = true
			guardFailed := false
			for _, guard := range tr.Guards {
				if err := guard(t, actor, payload, deps); err != nil {
					lastErr = fmt.Errorf("guard: %w", err)
					guardFailed = true
					break
				}
			}
			if !guardFailed {
				return tr.To, nil
			}
		}
	}
	if !matched {
		return "", fmt.Errorf("no valid transition from %s with trigger %s", t.State, trigger)
	}
	return "", lastErr
}

// ValidTransitions returns all valid triggers from the given state.
func (sm *StateMachine) ValidTransitions(from State) []string {
	var triggers []string
	seen := make(map[string]bool)
	for _, tr := range sm.transitions {
		if tr.From == from && !seen[tr.Trigger] {
			seen[tr.Trigger] = true
			triggers = append(triggers, tr.Trigger)
		}
	}
	return triggers
}

// --- Guards ---

func guardDependenciesMet(_ *Ticket, _ Actor, _ map[string]any, deps []*Ticket) error {
	for _, d := range deps {
		if d.State != StateClosed {
			return errors.New("dependency not done")
		}
	}
	return nil
}

func guardNoActiveLease(t *Ticket, _ Actor, _ map[string]any, _ []*Ticket) error {
	if t.AssignedTo != "" {
		return errors.New("ticket already claimed")
	}
	return nil
}

func guardIsLeaseholder(t *Ticket, actor Actor, _ map[string]any, _ []*Ticket) error {
	if t.AssignedTo != actor.ID {
		return errors.New("actor is not the leaseholder")
	}
	return nil
}

func guardOutputsPresent(_ *Ticket, _ Actor, payload map[string]any, _ []*Ticket) error {
	if payload["outputs"] == nil {
		return errors.New("outputs required for submit")
	}
	return nil
}

func guardEscalationReasonPresent(_ *Ticket, _ Actor, payload map[string]any, _ []*Ticket) error {
	if payload["reason"] == nil && payload["question"] == nil {
		return errors.New("reason or question required for escalate")
	}
	return nil
}

func guardIsHuman(_ *Ticket, _ Actor, _ map[string]any, _ []*Ticket) error {
	// Approve/reject/resolve must be done by a human.
	// For now we allow any actor; restrict to ActorHuman when auth is wired.
	return nil
}

func guardSystemOnly(_ *Ticket, actor Actor, _ map[string]any, _ []*Ticket) error {
	if actor.Type != ActorSystem {
		return errors.New("only system can force lease_expired from executing")
	}
	return nil
}

// guardResumeExec checks that the payload indicates resuming execution (not going back to planning).
func guardResumeExec(_ *Ticket, _ Actor, payload map[string]any, _ []*Ticket) error {
	if payload["resume_state"] != "executing" {
		return errors.New("resume_state must be 'executing' to resume execution from awaiting_input")
	}
	return nil
}
