package ticket

import "testing"

func TestEnvironmentQualifiedState(t *testing.T) {
	tests := []struct {
		state   State
		envSlug string
		want    string
	}{
		{StateExecuting, "dev", "executing-dev"},
		{StateValidated, "staging", "validated-staging"},
		{StateClosed, "prod", "closed-prod"},
		{StatePlanning, "custom-env", "planning-custom-env"},
		{StateExecuting, "", "executing"},
	}
	for _, tt := range tests {
		got := EnvironmentQualifiedState(tt.state, tt.envSlug)
		if got != tt.want {
			t.Errorf("EnvironmentQualifiedState(%q, %q) = %q, want %q", tt.state, tt.envSlug, got, tt.want)
		}
	}
}

func TestParseQualifiedState(t *testing.T) {
	tests := []struct {
		input     string
		wantState State
		wantSlug  string
	}{
		{"executing-dev", StateExecuting, "dev"},
		{"validated-staging", StateValidated, "staging"},
		{"closed-prod", StateClosed, "prod"},
		{"planning-custom-env", StatePlanning, "custom-env"},
		{"awaiting_validation-staging", StateAwaitingValidation, "staging"},
		{"awaiting_input-dev", StateAwaitingInput, "dev"},
		// Unqualified state (no environment)
		{"executing", State("executing"), ""},
		{"draft", State("draft"), ""},
	}
	for _, tt := range tests {
		state, slug := ParseQualifiedState(tt.input)
		if state != tt.wantState || slug != tt.wantSlug {
			t.Errorf("ParseQualifiedState(%q) = (%q, %q), want (%q, %q)",
				tt.input, state, slug, tt.wantState, tt.wantSlug)
		}
	}
}
