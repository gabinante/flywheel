package events

import "testing"

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern   string
		eventType string
		want      bool
	}{
		// Exact match
		{"ticket.created", "ticket.created", true},
		{"ticket.created", "ticket.closed", false},
		{"ticket.created", "project.created", false},

		// Single wildcard
		{"ticket.*", "ticket.created", true},
		{"ticket.*", "ticket.closed", true},
		{"ticket.*", "ticket.state.changed", false}, // * matches exactly one segment
		{"ticket.*", "project.created", false},
		{"*.created", "ticket.created", true},
		{"*.created", "project.created", true},
		{"*.created", "ticket.closed", false},

		// Double wildcard
		{"ticket.**", "ticket.created", true},
		{"ticket.**", "ticket.state.changed", true},
		{"ticket.**", "project.created", false},
		{"**", "ticket.created", true},
		{"**", "a.b.c.d", true},

		// Mixed
		{"ticket.*.done", "ticket.abc.done", true},
		{"ticket.*.done", "ticket.abc.pending", false},
		{"*.*.created", "org.project.created", true},
		{"*.*.created", "ticket.created", false},

		// Edge cases
		{"*", "ticket", true},
		{"*", "ticket.created", false},
		{"", "", true},
		{"ticket", "ticket", true},
		{"ticket", "ticket.created", false},

		// Double wildcard in middle
		{"ticket.**.closed", "ticket.closed", true},
		{"ticket.**.closed", "ticket.state.closed", true},
		{"ticket.**.closed", "ticket.a.b.closed", true},
		{"ticket.**.closed", "ticket.a.b.opened", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.eventType, func(t *testing.T) {
			got := MatchPattern(tt.pattern, tt.eventType)
			if got != tt.want {
				t.Errorf("MatchPattern(%q, %q) = %v, want %v", tt.pattern, tt.eventType, got, tt.want)
			}
		})
	}
}
