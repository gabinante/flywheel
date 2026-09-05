package mcp

import (
	"context"
	"github.com/gabinante/flywheel/internal/auth"
	"testing"
)

func TestRunCapabilities(t *testing.T) {
	for _, tc := range []struct {
		role, tool string
		args       map[string]any
		want       bool
	}{
		{"executor", "approve_ticket", map[string]any{"ticket_id": "t"}, false},
		{"orchestrator", "approve_ticket", map[string]any{"ticket_id": "t"}, false},
		{"orchestrator", "claim_ticket", map[string]any{"project_id": "p"}, false},
		{"orchestrator", "submit_ticket", map[string]any{"ticket_id": "t"}, false},
		{"orchestrator", "force_release_lease", map[string]any{"ticket_id": "t"}, false},
		{"orchestrator", "create_ticket", map[string]any{"project_id": "p"}, true},
		{"validator", "approve_ticket", map[string]any{"ticket_id": "t"}, true},
		{"validator", "approve_ticket", map[string]any{"ticket_id": "other"}, false},
		{"executor", "submit_ticket", map[string]any{"ticket_id": "t", "project_id": "other"}, false},
		{"executor", "submit_ticket", map[string]any{"ticket_id": "t"}, true},
	} {
		t.Run(tc.role+tc.tool, func(t *testing.T) {
			ctx := auth.WithRun(context.Background(), auth.RunGrant{Role: tc.role, TicketID: "t", ProjectID: "p"})
			err := authorizeRun(ctx, tc.tool, tc.args)
			if (err == nil) != tc.want {
				t.Fatalf("allowed=%v err=%v", tc.want, err)
			}
		})
	}
	args := map[string]any{"project_id": "p"}
	ctx := auth.WithRun(context.Background(), auth.RunGrant{Role: "executor", TicketID: "exact", ProjectID: "p"})
	if err := authorizeRun(ctx, "claim_ticket", args); err != nil {
		t.Fatal(err)
	}
	if args["ticket_id"] != "exact" {
		t.Fatal("claim escaped run's ticket")
	}
	if err := authorizeRun(context.Background(), "approve_ticket", nil); err == nil {
		t.Fatal("unscoped key approved")
	}
}
func TestRunCredentialRevocation(t *testing.T) {
	key, revoke, err := auth.IssueRun(auth.RunGrant{TicketID: "t", ProjectID: "p", Role: "executor", ParentKey: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	if grant, ok := auth.LookupRun(key); !ok || grant.ParentKey != "parent" {
		t.Fatal("missing grant")
	}
	revoke()
	if _, ok := auth.LookupRun(key); ok {
		t.Fatal("grant survived worker exit")
	}
}
