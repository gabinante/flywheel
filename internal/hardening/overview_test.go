package hardening

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gabinante/flywheel/internal/overview"
	"github.com/gabinante/flywheel/internal/runstatus"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workflow"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestGlobalOverviewAndLiveSessions(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	_, first := fixture(t, p)
	_, second := fixture(t, p)
	ws := workflow.NewStore(p)
	ts := ticket.NewStore(p)
	definition := &workflow.Definition{Scope: "project", ScopeID: first, Name: "Pinned approval", Version: 1, Phases: []workflow.Phase{{ID: "approve", Name: "Operator approval", Type: workflow.PhaseGate, Config: map[string]any{"conditions": []any{map[string]any{"type": "human_approval"}}}}}}
	if err := ws.Create(ctx, definition); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	create := func(projectID, state, status string) *ticket.Ticket {
		t.Helper()
		task := &ticket.Ticket{ID: uuid.NewString(), ProjectID: projectID, Title: "Global tray test", DependsOn: []string{}, Type: ticket.TypeTask, State: ticket.State(state), WorkflowID: definition.ID, WorkflowVersion: 1, WorkflowPhase: "approve", WorkflowPhaseStatus: status, WorkflowPhaseEnteredAt: &now, CreatedBy: "test", CreatedAt: now, UpdatedAt: now}
		if err := ts.Create(ctx, task); err != nil {
			t.Fatal(err)
		}
		return task
	}
	approval := create(first, "awaiting_validation", "blocked")
	failed := create(second, "executing", "failed")
	running := create(second, "executing", "running")
	if _, err := p.Exec(ctx, `UPDATE tickets SET outputs='[]'::jsonb WHERE id=$1`, failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO ticket_external_refs(ticket_id,external_id,identifier)VALUES($1,$1,'LOCAL-123')`, running.ID); err != nil {
		t.Fatal(err)
	}
	// The current definition changes; the tray must still honor the ticket's pinned version.
	if _, err := p.Exec(ctx, `INSERT INTO workflow_definition_versions(workflow_id,version,name,phases)SELECT id,version,name,phases FROM workflow_definitions WHERE id=$1 ON CONFLICT DO NOTHING`, definition.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE workflow_definitions SET version=2,phases='[{"id":"approve","type":"gate","name":"CI","config":{"conditions":[{"type":"webhook"}]}}]' WHERE id=$1`, definition.ID); err != nil {
		t.Fatal(err)
	}
	registry := runstatus.New()
	recorder := sessions.NewService(sessions.NewStore(p), sessions.Config{})
	registry.SetObserver(recorder.RecordRun)
	runctx, finish := registry.Begin(ctx, runstatus.Run{Kind: "executor", Harness: "codex", ProjectID: second, TicketID: running.ID, Title: running.Title})
	runstatus.Running(runctx)
	runstatus.Session(runctx, "native-"+uuid.NewString())
	runstatus.ParseOutput(runctx, []byte(`{"type":"item.started","item":{"type":"command_execution"}}`))
	runstatus.ParseOutput(runctx, []byte(`{"type":"turn.completed","usage":{"input_tokens":50,"output_tokens":7}}`))
	live := registry.Snapshot()
	if len(live) != 1 || live[0].SessionID == "" {
		t.Fatalf("live=%v", live)
	}
	sess, err := recorder.Get(ctx, live[0].SessionID)
	if err != nil || sess == nil {
		t.Fatalf("session=%v err=%v", sess, err)
	}
	if sess.Status(now.Add(time.Hour)) != sessions.StatusActive {
		t.Fatal("managed process became idle just because it was quiet")
	}
	if _, err := p.Exec(ctx, `UPDATE agent_sessions SET last_activity_at=now()-interval '2 hours' WHERE id=$1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	for _, status := range []sessions.Status{sessions.StatusActive, sessions.StatusIdle, sessions.StatusEnded} {
		_, total, err := recorder.List(ctx, sessions.Filter{Status: status, Ref: running.ID})
		want := 0
		if status == sessions.StatusActive {
			want = 1
		}
		if err != nil || total != want {
			t.Fatalf("quiet managed run filtered as %s: total=%d want=%d err=%v", status, total, want, err)
		}
	}
	store := overview.NewStore(p)
	snapshot, err := store.Get(ctx, live, []string{running.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, item := range snapshot.Attention {
		actions[item.TicketID] = item.Action
	}
	if actions[approval.ID] != "Approve phase" || actions[failed.ID] != "Retry phase" || actions[running.ID] != "" {
		t.Fatalf("actions=%v", actions)
	}
	var entry overview.Item
	for _, item := range snapshot.InFlight {
		if item.TicketID == running.ID {
			entry = item
		}
	}
	if entry.Ref != "LOCAL-123" || entry.SessionHref != "/sessions/"+sess.ID || entry.ProjectID != second {
		t.Fatalf("entry=%+v", entry)
	}
	value, _ := json.Marshal(map[string]string{"_human_approval_approve": now.Format(time.RFC3339Nano)})
	if _, err := p.Exec(ctx, `UPDATE tickets SET outputs=$2::jsonb WHERE id=$1`, approval.ID, value); err != nil {
		t.Fatal(err)
	}
	finish()
	snapshot, err = store.Get(ctx, registry.Snapshot(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snapshot.InFlight {
		if item.TicketID == running.ID {
			t.Fatal("completed process remained in flight")
		}
	}
	for _, item := range snapshot.Attention {
		if item.TicketID == approval.ID {
			t.Fatal("resolved approval remained in attention")
		}
	}
	sess, err = recorder.Get(ctx, sess.ID)
	if err != nil || sess.Status(time.Now()) != sessions.StatusEnded {
		t.Fatalf("ended session=%v err=%v", sess, err)
	}
	progress, _ := sess.Metadata["flywheel_progress"].(map[string]any)
	if progress["worker_state"] != "exited" || progress["tool_calls"] != float64(1) || sess.TokensIn != 50 || sess.TokensOut != 7 {
		t.Fatalf("final progress was not retained: progress=%v tokens=%d/%d", progress, sess.TokensIn, sess.TokensOut)
	}
}

func TestReviewOwnershipDistinguishesStaleStateFromServiceWork(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	id := uuid.NewString()
	t.Cleanup(func() { p.Exec(ctx, `DELETE FROM code_review_requests WHERE id=$1`, id) })
	if _, err := p.Exec(ctx, `INSERT INTO code_review_requests(id,repo,number,title,state,updated_at) VALUES($1,$1,1,'Orphaned review','reviewing',now()-interval '2 hours')`, id); err != nil {
		t.Fatal(err)
	}
	store := overview.NewStore(p)
	snapshot, err := store.Get(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range snapshot.InFlight {
		if item.ReviewID == id {
			t.Fatal("stale review appeared to have a running worker")
		}
	}
	for _, item := range snapshot.Attention {
		if item.ReviewID == id {
			found = item.Progress != nil && item.Progress.Health == "untracked"
		}
	}
	if !found {
		t.Fatal("missing worker was not surfaced")
	}
	snapshot, err = store.Get(ctx, nil, nil, []string{id})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range snapshot.InFlight {
		if item.ReviewID == id {
			found = item.Progress != nil && item.Progress.WorkerState == "preparing"
		}
	}
	if !found {
		t.Fatal("review service working between harness turns was marked orphaned")
	}
}

func TestOverviewKeepsEachReviewerThreadAndItsPublicationSession(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	repo := "test/" + uuid.NewString()
	store := overview.NewStore(p)
	var runs []runstatus.Run
	var reviewIDs []string
	for number := 1; number <= 2; number++ {
		id, sessionID := uuid.NewString(), uuid.NewString()
		t.Cleanup(func() { p.Exec(ctx, `DELETE FROM code_review_requests WHERE id=$1`, id) })
		reviewIDs = append(reviewIDs, id)
		if _, err := p.Exec(ctx, `INSERT INTO code_review_requests(id,repo,number,title,state,harness,model,session_id) VALUES($1,$2,$3,'Concurrent review','reviewing','codex','review-model',$4)`, id, repo, number, sessionID); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, runstatus.Run{ID: uuid.NewString(), Kind: "code_review", ReviewID: id, Ref: fmt.Sprintf("%s#%d", repo, number), Title: "Concurrent review", Harness: "codex", Model: "review-model", SessionID: sessionID, State: "running", StartedAt: time.Now()})
	}
	assertThreads := func(live []runstatus.Run) {
		t.Helper()
		snapshot, err := store.Get(ctx, live, nil, reviewIDs)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			found := 0
			for _, item := range snapshot.InFlight {
				if item.ReviewID != run.ReviewID {
					continue
				}
				found++
				if item.Ref != run.Ref || item.SessionHref != "/sessions/"+run.SessionID || item.Href != "/code-reviews/"+run.ReviewID || item.Model != run.Model || item.Harness != run.Harness {
					t.Fatalf("reviewer lost its PR, session, or harness identity: %+v", item)
				}
			}
			if found != 1 {
				t.Fatalf("review %s has %d entries; want one per live reviewer", run.ReviewID, found)
			}
		}
	}
	assertThreads(runs)
	// The harness has finished, but the review service is still publishing its result.
	if _, err := p.Exec(ctx, `UPDATE code_review_requests SET state='publishing' WHERE id=ANY($1)`, reviewIDs); err != nil {
		t.Fatal(err)
	}
	assertThreads(nil)
	if _, err := p.Exec(ctx, `UPDATE code_review_requests SET state='watching' WHERE id=ANY($1)`, reviewIDs); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Get(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range snapshot.InFlight {
		for _, id := range reviewIDs {
			if item.ReviewID == id {
				t.Fatal("completed review still appeared in flight")
			}
		}
	}
}
