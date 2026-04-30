package ticket

import (
	"testing"
)

func TestIsUnblocked(t *testing.T) {
	tests := []struct {
		name   string
		ticket *Ticket
		deps   []*Ticket
		want   bool
	}{
		{"no deps", &Ticket{DependsOn: []string{}}, nil, true},
		{"one dep done", &Ticket{DependsOn: []string{"a"}}, []*Ticket{{ID: "a", State: StateClosed}}, true},
		{"one dep not done", &Ticket{DependsOn: []string{"a"}}, []*Ticket{{ID: "a", State: StateExecuting}}, false},
		{"two deps both done", &Ticket{DependsOn: []string{"a", "b"}}, []*Ticket{{ID: "a", State: StateClosed}, {ID: "b", State: StateClosed}}, true},
		{"two deps one not done", &Ticket{DependsOn: []string{"a", "b"}}, []*Ticket{{ID: "a", State: StateClosed}, {ID: "b", State: StateDraft}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUnblocked(tt.ticket, tt.deps); got != tt.want {
				t.Errorf("IsUnblocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDependencyOutputs(t *testing.T) {
	ticket := &Ticket{DependsOn: []string{"a", "b"}}
	deps := []*Ticket{
		{ID: "a", State: StateClosed, Outputs: map[string]any{"result": "x"}},
		{ID: "b", State: StateDraft, Outputs: map[string]any{"y": 1}},
	}
	got := GetDependencyOutputs(ticket, deps)
	if len(got) != 1 {
		t.Fatalf("expected 1 dep output, got %d", len(got))
	}
	if got["a"].(map[string]any)["result"] != "x" {
		t.Errorf("expected a.result = x, got %v", got["a"])
	}
}
