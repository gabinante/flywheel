package mcp

import (
	"context"
	"fmt"
	"github.com/gabinante/flywheel/internal/auth"
)

func authorizeRun(ctx context.Context, name string, args map[string]any) error {
	g, ok := auth.RunFromContext(ctx)
	if !ok {
		if (name == "approve_ticket" || name == "reject_ticket") && !auth.IsOperator(ctx) {
			return fmt.Errorf("operator or validator run required")
		}
		return nil
	}
	if id, _ := args["project_id"].(string); id != "" && id != g.ProjectID {
		return fmt.Errorf("run is restricted to project %s", g.ProjectID)
	}
	if g.Role == "orchestrator" {
		planningWrites := map[string]bool{"create_ticket": true, "update_ticket": true, "create_work_stream": true, "update_work_stream": true, "update_work_stream_plan": true, "update_project_context": true}
		if GetToolScope(name) != ToolScopeRead && !planningWrites[name] {
			return fmt.Errorf("planner cannot execute %s", name)
		}
		return nil
	}
	if id, _ := args["ticket_id"].(string); id != "" && id != g.TicketID {
		return fmt.Errorf("run is restricted to ticket %s", g.TicketID)
	}
	if name == "claim_ticket" {
		args["ticket_id"] = g.TicketID
		args["project_id"] = g.ProjectID
	}
	if GetToolScope(name) == ToolScopeRead {
		return nil
	}
	allowed := map[string]bool{"log_step": true, "renew_lease": true}
	switch g.Role {
	case "validator":
		allowed["approve_ticket"] = true
		allowed["reject_ticket"] = true
	case "decomposer":
		allowed["create_ticket"] = true
		fallthrough
	case "investigator":
		fallthrough
	default:
		for _, tool := range []string{"claim_ticket", "start_ticket", "submit_ticket", "escalate_ticket", "update_ticket_context"} {
			allowed[tool] = true
		}
	}
	if !allowed[name] {
		return fmt.Errorf("%s run cannot execute %s", g.Role, name)
	}
	return nil
}

// Resolve indirect subjects before allowing a project planner to mutate them.
func authorizeRunSubject(ctx context.Context, b *Backend, args map[string]any) error {
	g, ok := auth.RunFromContext(ctx)
	if !ok || g.Role != "orchestrator" {
		return nil
	}
	if id, _ := args["ticket_id"].(string); id != "" {
		t, err := b.Ticket.GetTicket(ctx, id)
		if err != nil {
			return err
		}
		if t.ProjectID != g.ProjectID {
			return fmt.Errorf("ticket is outside the planner project")
		}
	}
	if id, _ := args["work_stream_id"].(string); id != "" {
		stream, err := b.WorkStream.GetWorkStream(ctx, id)
		if err != nil {
			return err
		}
		if stream.ProjectID != g.ProjectID {
			return fmt.Errorf("work stream is outside the planner project")
		}
	}
	return nil
}
