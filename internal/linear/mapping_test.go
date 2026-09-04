package linear

import (
	"testing"

	"github.com/gabinante/flywheel/internal/ticket"
)

func TestMapState(t *testing.T) {
	cases := []struct {
		typ, name string
		want      ticket.State
	}{
		{"triage", "Triage", ticket.StateDraft},
		{"backlog", "Backlog", ticket.StateDraft},
		{"unstarted", "Todo", ticket.StateDraft},
		{"started", "In Progress", ticket.StateExecuting},
		{"started", "In Review", ticket.StateAwaitingValidation},
		{"started", "QA", ticket.StateAwaitingValidation},
		{"completed", "Done", ticket.StateClosed},
		{"canceled", "Canceled", ticket.StateClosed},
	}
	for _, c := range cases {
		if got := MapState(c.typ, c.name); got != c.want {
			t.Errorf("MapState(%q,%q) = %s, want %s", c.typ, c.name, got, c.want)
		}
	}
}

func TestPriorityRoundTrip(t *testing.T) {
	for _, p := range []int{1, 2, 3, 4} {
		if got := ToLinearPriority(MapPriority(p)); got != p {
			t.Errorf("priority %d round-tripped to %d", p, got)
		}
	}
	if MapPriority(0) != ticket.P2 {
		t.Errorf("no priority should map to P2")
	}
}

func TestMapType(t *testing.T) {
	if MapType([]string{"Bug"}) != ticket.TypeBug || MapType([]string{"research"}) != ticket.TypeSpike || MapType(nil) != ticket.TypeTask {
		t.Fatal("label → type mapping wrong")
	}
}

func TestPickState(t *testing.T) {
	states := []WorkflowState{
		{ID: "a", Name: "Backlog", Type: "backlog", Position: 0},
		{ID: "b", Name: "Todo", Type: "unstarted", Position: 1},
		{ID: "c", Name: "In Progress", Type: "started", Position: 2},
		{ID: "d", Name: "In Review", Type: "started", Position: 3},
		{ID: "e", Name: "Done", Type: "completed", Position: 4},
	}
	typ, names := DesiredStateType(ticket.StateAwaitingValidation)
	if got := PickState(states, typ, names); got == nil || got.ID != "d" {
		t.Fatalf("awaiting_validation → %+v, want In Review", got)
	}
	typ, names = DesiredStateType(ticket.StateExecuting)
	if got := PickState(states, typ, names); got == nil || got.ID != "c" {
		t.Fatalf("executing → %+v, want In Progress", got)
	}
	if got := PickState(states, "canceled", []string{"canceled"}); got != nil {
		t.Fatalf("missing type should return nil, got %+v", got)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Synthetic task generation for Buckeye": "synthetic-task-generation-for-buckeye",
		"  RLE Tools & Data — bugs ":            "rle-tools-data-bugs",
		"":                                      "linear-project",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAbstractBody(t *testing.T) {
	tk := &ticket.Ticket{ID: "buckeye-12", Title: "Add brief toggle", TargetRepo: "joinera",
		Objective: ticket.Objective{Description: "Expose the auto-brief flag per environment.", SuccessCriteria: []string{"toggle visible", "flag persisted"}}}
	body := AbstractBody(tk)
	for _, want := range []string{"## Abstract\nExpose the auto-brief flag", "**Components:** joinera", "**Before:**", "**After:**", "- toggle visible"} {
		if !contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool { return indexOf(s, sub) >= 0 })()
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestParseProjectRef(t *testing.T) {
	cases := map[string]string{
		"https://linear.app/joinhandshake/project/synthetic-task-generation-for-buckeye-71c32f62d775":          "71c32f62d775",
		"https://linear.app/joinhandshake/project/synthetic-task-generation-for-buckeye-71c32f62d775/overview": "71c32f62d775",
		"973951f8-6a52-4663-a75f-32b46d93858c": "973951f8-6a52-4663-a75f-32b46d93858c",
		"71c32f62d775":                         "71c32f62d775",
		"not a project":                        "",
	}
	for in, want := range cases {
		got, ok := ParseProjectRef(in)
		if want == "" {
			if ok {
				t.Errorf("ParseProjectRef(%q) accepted %q", in, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("ParseProjectRef(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
}
