package rest

import (
	"context"
	"encoding/json"
	"github.com/gabinante/flywheel/api/generated"
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/settings"
	"github.com/gabinante/flywheel/internal/ticket"
	"testing"
)

type reviewAgentStore struct{ agent.AgentStore }

func (reviewAgentStore) GetByID(context.Context, string) (*agent.Agent, error) {
	return &agent.Agent{ID: "operator"}, nil
}

func TestReviewSettingsSavePreservesPromptOverrides(t *testing.T) {
	svc, err := settings.Load(context.Background(), nil, settings.Settings{Prompts: map[string]string{"code_review": "custom instructions"}})
	if err != nil {
		t.Fatal(err)
	}
	// The settings UI submits its public settings model; prompts have a separate endpoint.
	raw, err := json.Marshal(settingsToGen(svc.Current(), true))
	if err != nil {
		t.Fatal(err)
	}
	var req generated.UpdateOperatorSettingsRequestObject
	if err := json.Unmarshal(raw, &req.Body); err != nil {
		t.Fatal(err)
	}
	server := &StrictServer{SettingsSvc: svc, AgentStore: reviewAgentStore{}}
	ctx := context.WithValue(context.Background(), ContextKeyAgentID, "operator")
	response, err := server.UpdateOperatorSettings(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(generated.UpdateOperatorSettings200JSONResponse); !ok {
		t.Fatalf("unexpected response: %#v", response)
	}
	if svc.Current().Prompts["code_review"] != "custom instructions" {
		t.Fatal("ordinary settings save deleted separately configured prompt overrides")
	}
}

func TestTicketResponseIncludesWorkflow(t *testing.T) {
	got := ticketToGen(&ticket.Ticket{ID: "t", WorkflowID: "wf", WorkflowPhase: "approval", TargetRepo: "secondary"})
	if got.WorkflowId == nil || *got.WorkflowId != "wf" || got.WorkflowPhase == nil || *got.WorkflowPhase != "approval" || got.TargetRepo == nil {
		t.Fatal("ticket response lost execution context")
	}
}
