package ticket

import (
	"testing"
)

func TestStateMachine_Transition(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		name       string
		from       State
		trigger    string
		actor      Actor
		assignedTo string // ticket.AssignedTo (for leaseholder checks)
		payload    map[string]any
		deps       []*Ticket
		wantState  State
		wantErr    bool
	}{
		// === Happy path ===
		{"planning->executing", StatePlanning, TriggerStart, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, StateExecuting, false},
		{"executing->awaiting_validation", StateExecuting, TriggerSubmit, Actor{ID: "agent1", Type: ActorAgent}, "agent1", map[string]any{"outputs": map[string]any{"x": 1}}, nil, StateAwaitingValidation, false},
		{"awaiting_validation->validated (validate)", StateAwaitingValidation, TriggerValidate, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateValidated, false},
		{"validated->closed (close)", StateValidated, TriggerClose, Actor{ID: "system", Type: ActorSystem}, "agent1", nil, nil, StateClosed, false},

		// === Claim ===
		{"draft->planning (claim)", StateDraft, TriggerClaim, Actor{ID: "agent1", Type: ActorAgent}, "", nil, nil, StatePlanning, false},
		{"draft->planning (claim) with deps done", StateDraft, TriggerClaim, Actor{ID: "a", Type: ActorAgent}, "", map[string]any{"agent_id": "a"}, []*Ticket{{ID: "x", State: StateClosed}}, StatePlanning, false},
		{"draft->planning (claim) dep not done", StateDraft, TriggerClaim, Actor{ID: "a", Type: ActorAgent}, "", nil, []*Ticket{{ID: "x", State: StateExecuting}}, "", true},

		// === Leaseholder guards ===
		{"planning->executing wrong agent", StatePlanning, TriggerStart, Actor{ID: "other", Type: ActorAgent}, "agent1", nil, nil, "", true},
		{"executing->submit no outputs", StateExecuting, TriggerSubmit, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, "", true},

		// === Input / escalation ===
		{"executing->awaiting_input (escalate)", StateExecuting, TriggerEscalate, Actor{ID: "agent1", Type: ActorAgent}, "agent1", map[string]any{"reason": "stuck", "question": "?"}, nil, StateAwaitingInput, false},
		{"executing->escalate no reason", StateExecuting, TriggerEscalate, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, "", true},
		{"planning->awaiting_input (request_input)", StatePlanning, TriggerRequestInput, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, StateAwaitingInput, false},
		{"executing->awaiting_input (request_input)", StateExecuting, TriggerRequestInput, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, StateAwaitingInput, false},
		{"awaiting_input->planning (provide_input)", StateAwaitingInput, TriggerProvideInput, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StatePlanning, false},
		{"awaiting_input->executing (provide_input+resume)", StateAwaitingInput, TriggerProvideInput, Actor{ID: "human1", Type: ActorHuman}, "agent1", map[string]any{"resume_state": "executing"}, nil, StateExecuting, false},
		{"awaiting_input->executing (approve)", StateAwaitingInput, TriggerApprove, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateExecuting, false},

		// === Failure arcs ===
		{"executing->planning (replan)", StateExecuting, TriggerReplan, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, StatePlanning, false},
		{"validated->planning (invalidate)", StateValidated, TriggerInvalidate, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StatePlanning, false},

		// === Rejection / approval ===
		{"awaiting_validation->validated (approve)", StateAwaitingValidation, TriggerApprove, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateValidated, false},
		{"awaiting_validation->executing (reject)", StateAwaitingValidation, TriggerReject, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateExecuting, false},

		// === Cancel ===
		{"draft->closed (cancel)", StateDraft, TriggerCancel, Actor{ID: "human1", Type: ActorHuman}, "", nil, nil, StateClosed, false},
		{"planning->closed (cancel)", StatePlanning, TriggerCancel, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateClosed, false},
		{"executing->closed (cancel)", StateExecuting, TriggerCancel, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, StateClosed, false},

		// === Lease expiry ===
		{"planning->draft (lease_expired)", StatePlanning, TriggerLeaseExpired, Actor{ID: "system", Type: ActorSystem}, "agent1", nil, nil, StateDraft, false},
		{"executing->draft (lease_expired system)", StateExecuting, TriggerLeaseExpired, Actor{ID: "operator", Type: ActorSystem}, "agent1", nil, nil, StateDraft, false},
		{"executing->draft lease_expired non-system rejected", StateExecuting, TriggerLeaseExpired, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, "", true},

		// === Fail ===
		{"executing->draft (fail)", StateExecuting, TriggerFail, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, StateDraft, false},

		// === Reopen ===
		{"closed->draft (reopen)", StateClosed, TriggerReopen, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateDraft, false},

		// === Rollback ===
		{"executing->draft (rollback)", StateExecuting, TriggerRollback, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateDraft, false},
		{"awaiting_validation->draft (rollback)", StateAwaitingValidation, TriggerRollback, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateDraft, false},
		{"validated->draft (rollback)", StateValidated, TriggerRollback, Actor{ID: "human1", Type: ActorHuman}, "agent1", nil, nil, StateDraft, false},
		{"draft rollback invalid", StateDraft, TriggerRollback, Actor{ID: "human1", Type: ActorHuman}, "", nil, nil, "", true},
		{"planning rollback invalid", StatePlanning, TriggerRollback, Actor{ID: "agent1", Type: ActorAgent}, "agent1", nil, nil, "", true},

		// === Invalid transitions ===
		{"invalid trigger from draft", StateDraft, "invalid", Actor{}, "", nil, nil, "", true},
		{"already claimed", StateDraft, TriggerClaim, Actor{ID: "agent2", Type: ActorAgent}, "agent1", nil, nil, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket := &Ticket{State: tt.from}
			if tt.assignedTo != "" {
				ticket.AssignedTo = tt.assignedTo
			} else if tt.actor.ID != "" && (tt.from == StatePlanning || tt.from == StateExecuting || tt.from == StateAwaitingValidation || tt.from == StateAwaitingInput) {
				ticket.AssignedTo = tt.actor.ID
			}
			got, err := sm.Transition(ticket, tt.trigger, tt.actor, tt.payload, tt.deps)
			if (err != nil) != tt.wantErr {
				t.Errorf("Transition() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantState {
				t.Errorf("Transition() got state %v, want %v", got, tt.wantState)
			}
		})
	}
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	sm := NewStateMachine()

	// Draft should have claim and cancel transitions
	triggers := sm.ValidTransitions(StateDraft)
	if len(triggers) == 0 {
		t.Fatal("expected valid transitions from draft")
	}
	found := make(map[string]bool)
	for _, tr := range triggers {
		found[tr] = true
	}
	for _, want := range []string{TriggerClaim, TriggerCancel} {
		if !found[want] {
			t.Errorf("expected trigger %q from draft, got triggers: %v", want, triggers)
		}
	}
}

func TestAllStates(t *testing.T) {
	states := AllStates()
	if len(states) != 7 {
		t.Errorf("expected 7 states, got %d: %v", len(states), states)
	}
}

func TestIsValidState(t *testing.T) {
	for _, s := range AllStates() {
		if !IsValidState(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	if IsValidState("invalid_state") {
		t.Error("expected 'invalid_state' to be invalid")
	}
}

func TestRollbackAvailableAtEachStage(t *testing.T) {
	sm := NewStateMachine()

	// Rollback should be available from these states.
	rollbackStates := []State{
		StateExecuting,
		StateAwaitingValidation,
		StateValidated,
	}
	for _, state := range rollbackStates {
		triggers := sm.ValidTransitions(state)
		found := false
		for _, tr := range triggers {
			if tr == TriggerRollback {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rollback trigger should be available from %q, got triggers: %v", state, triggers)
		}
	}

	// Rollback should NOT be available from these states.
	noRollbackStates := []State{
		StateDraft,
		StatePlanning,
		StateAwaitingInput,
		StateClosed,
	}
	for _, state := range noRollbackStates {
		triggers := sm.ValidTransitions(state)
		for _, tr := range triggers {
			if tr == TriggerRollback {
				t.Errorf("rollback trigger should NOT be available from %q", state)
			}
		}
	}
}

func TestFailureArcs(t *testing.T) {
	sm := NewStateMachine()

	// executing → planning (replan)
	ticket := &Ticket{State: StateExecuting, AssignedTo: "agent1"}
	got, err := sm.Transition(ticket, TriggerReplan, Actor{ID: "agent1", Type: ActorAgent}, nil, nil)
	if err != nil {
		t.Fatalf("replan: %v", err)
	}
	if got != StatePlanning {
		t.Errorf("replan: got %v, want %v", got, StatePlanning)
	}

	// validated → planning (invalidate)
	ticket = &Ticket{State: StateValidated, AssignedTo: "agent1"}
	got, err = sm.Transition(ticket, TriggerInvalidate, Actor{ID: "human1", Type: ActorHuman}, nil, nil)
	if err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if got != StatePlanning {
		t.Errorf("invalidate: got %v, want %v", got, StatePlanning)
	}
}
