package hardening

import (
	"context"
	"errors"
	"github.com/gabinante/flywheel/db"
	"github.com/gabinante/flywheel/events"
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/auth"
	"github.com/gabinante/flywheel/internal/codereview"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/user"
	"github.com/gabinante/flywheel/internal/workflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("FLYWHEEL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set FLYWHEEL_TEST_DATABASE_URL to an isolated migrated database")
	}
	p, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}
func fixture(t *testing.T, p *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	org, proj := uuid.NewString(), uuid.NewString()
	if _, err := p.Exec(ctx, "INSERT INTO orgs(id,name,slug)VALUES($1,'Test',$1)", org); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO projects(id,org_id,name,slug,repo_url)VALUES($1,$2,'Test',$1,'https://github.com/test/repo.with.dots.git')", proj, org); err != nil {
		t.Fatal(err)
	}
	return org, proj
}
func TestAtomicWorkflowAttemptAndCallback(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	_, project := fixture(t, p)
	ts := ticket.NewStore(p)
	ws := workflow.NewStore(p)
	engine := workflow.NewEngine(ws, ts)
	def := &workflow.Definition{Scope: "project", ScopeID: project, Name: "Attempt test", Version: 1, Phases: []workflow.Phase{{ID: "run", Name: "Run", Type: workflow.PhaseAgent}, {ID: "gate", Name: "Gate", Type: workflow.PhaseGate}}}
	if err := ws.Create(ctx, def); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	id := "test-" + uuid.NewString()
	task := &ticket.Ticket{ID: id, ProjectID: project, Title: "Test", Type: ticket.TypeTask, State: ticket.StateDraft, DependsOn: []string{}, WorkflowID: def.ID, WorkflowVersion: 1, WorkflowPhase: "run", WorkflowPhaseStatus: "running", WorkflowPhaseEnteredAt: &now, CreatedBy: "test", CreatedAt: now, UpdatedAt: now}
	if err := ts.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	cb := workflow.NewCallbackHandler([]byte("test"), workflow.NewPostgresCallbackStore(p), engine)
	token, err := cb.GenerateToken(ctx, id, def.ID, "run", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := engine.AdvancePhase(ctx, id, def.ID, "run", "success", map[string]any{"phase_entered_at": now.Format(time.RFC3339Nano)}, 1); err != nil {
				t.Log(err)
			}
		}()
	}
	wg.Wait()
	completions, err := ws.ListCompletions(ctx, id)
	if err != nil || len(completions) != 1 {
		t.Fatalf("history=%v err=%v", completions, err)
	}
	if err := ts.UpdateWorkflowPhase(ctx, id, "run"); err != nil {
		t.Fatal(err)
	}
	if err := cb.HandleCallback(ctx, token, "success", nil); err == nil {
		t.Fatal("old callback advanced a new attempt of the same phase")
	}
	current, _ := ts.GetWorkflowPhase(ctx, id)
	if current != "run" {
		t.Fatal(current)
	}
}
func TestSessionIngestionRollback(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	store := sessions.NewStore(p)
	now := time.Now()
	sess := &sessions.Session{Harness: sessions.HarnessCodex, ExternalID: uuid.NewString(), Origin: sessions.OriginDispatched, StartedAt: now, LastActivityAt: now, IngestOffset: 123, Metadata: map[string]any{}}
	// A null byte is invalid PostgreSQL text. Cursor must roll back with this prompt.
	err := store.Ingest(ctx, sess, []sessions.Prompt{{Seq: 1, Role: "user", Text: "invalid\x00text", TS: now}}, nil)
	if err == nil {
		t.Fatal("expected invalid prompt to fail")
	}
	saved, err := store.GetByExternal(ctx, sessions.HarnessCodex, sess.ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	if saved != nil {
		t.Fatal("cursor committed before prompts")
	}
}

func TestRetryAndTerminalCompletionAreAtomic(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	_, project := fixture(t, p)
	ts, ws := ticket.NewStore(p), workflow.NewStore(p)
	engine := workflow.NewEngine(ws, ts)
	def := &workflow.Definition{Scope: "project", ScopeID: project, Name: "Final phase", Version: 1, Phases: []workflow.Phase{{ID: "run", Name: "Run", Type: workflow.PhaseAgent}}}
	if err := ws.Create(ctx, def); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	task := &ticket.Ticket{ID: uuid.NewString(), ProjectID: project, Title: "Atomic close", DependsOn: []string{}, Type: ticket.TypeTask, State: ticket.StateAwaitingValidation, WorkflowID: def.ID, WorkflowVersion: 1, WorkflowPhase: "run", WorkflowPhaseStatus: "failed", WorkflowPhaseEnteredAt: &now, CreatedBy: "test", CreatedAt: now, UpdatedAt: now}
	if err := ts.Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := ts.UpdateWorkflow(ctx, task.ID, def.ID, 1, "run"); err != nil {
		t.Fatal(err)
	}
	fresh, err := ts.GetByID(ctx, task.ID)
	if err != nil || fresh.WorkflowPhaseStatus != "ready" || fresh.WorkflowPhaseEnteredAt.Equal(now) {
		t.Fatalf("retry failed: %#v %v", fresh, err)
	}
	engine.SetCompletion(func(context.Context, string) error { return errors.New("close failed") })
	callbacks := workflow.NewCallbackHandler([]byte("test"), workflow.NewPostgresCallbackStore(p), engine)
	token, err := callbacks.GenerateToken(ctx, task.ID, def.ID, "run", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := callbacks.HandleCallback(ctx, token, "success", nil); err == nil {
		t.Fatal("expected close failure")
	}
	fresh, _ = ts.GetByID(ctx, task.ID)
	history, _ := ws.ListCompletions(ctx, task.ID)
	if fresh.WorkflowPhase != "run" || len(history) != 0 {
		t.Fatal("failed close committed phase advancement")
	}
	// Editing the definition must not change an existing callback's pinned version.
	def.Phases = append(def.Phases, workflow.Phase{ID: "later", Name: "Later", Type: workflow.PhaseAgent})
	if err := ws.Update(ctx, def); err != nil {
		t.Fatal(err)
	}
	svc := ticket.NewService(ts, events.NewPostgresBus(p, events.PostgresBusConfig{}), nil)
	engine.SetCompletion(func(ctx context.Context, id string) error {
		return svc.TransitionTicket(ctx, id, ticket.TriggerWorkflowComplete, ticket.Actor{ID: "test", Type: ticket.ActorSystem}, nil)
	})
	if err := callbacks.HandleCallback(ctx, token, "success", nil); err != nil {
		t.Fatal(err)
	}
	fresh, _ = ts.GetByID(ctx, task.ID)
	if fresh.State != ticket.StateClosed || fresh.WorkflowPhase != "" {
		t.Fatalf("completion not closed: %#v", fresh)
	}
	if err := callbacks.HandleCallback(ctx, token, "success", nil); err == nil {
		t.Fatal("callback replay succeeded")
	}
}

func TestIgnoredPublishErrorRollsBackDomainChange(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	_, project := fixture(t, p)
	bus := events.NewPostgresBus(p, events.PostgresBusConfig{})
	err := db.Transaction(ctx, p, func(ctx context.Context) error {
		if _, err := db.Executor(ctx, p).Exec(ctx, "UPDATE projects SET name='should roll back' WHERE id=$1", project); err != nil {
			return err
		}
		_ = bus.Publish(ctx, events.NewEvent("invalid", map[string]any{"bad": make(chan int)}))
		return nil
	})
	if err == nil {
		t.Fatal("ignored outbox error committed")
	}
	var name string
	if err := p.QueryRow(ctx, "SELECT name FROM projects WHERE id=$1", project).Scan(&name); err != nil || name != "Test" {
		t.Fatalf("name=%q err=%v", name, err)
	}
}

func TestFeedbackBatchClaimsSerializeAndExcludeNewRounds(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	s := codereview.NewStore(p)
	repo := "test/" + uuid.NewString()
	a := &codereview.FeedbackRound{Repo: repo, Number: 1, ReviewID: 1}
	if _, err := s.InsertFeedbackRound(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := uuid.NewString()
	if err := s.ClaimFeedback(ctx, repo, 1, a.ID, run); err != nil {
		t.Fatal(err)
	}
	b := &codereview.FeedbackRound{Repo: repo, Number: 1, ReviewID: 2}
	if _, err := s.InsertFeedbackRound(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimFeedback(ctx, repo, 1, b.ID, uuid.NewString()); err == nil {
		t.Fatal("second writer claimed active PR")
	}
	if err := s.FinishFeedback(ctx, run, "addressed", ""); err != nil {
		t.Fatal(err)
	}
	first, _ := s.GetFeedbackRound(ctx, a.ID)
	second, _ := s.GetFeedbackRound(ctx, b.ID)
	if first.State != "addressed" || second.State != "new" {
		t.Fatalf("states=%s,%s", first.State, second.State)
	}
	if err := s.ClaimFeedback(ctx, repo, 1, b.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}
func TestProjectFilteringBeforePaginationAndCurrentFindings(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	_, project := fixture(t, p)
	store := codereview.NewStore(p)
	for i := 0; i < 3; i++ {
		repo := "elsewhere/repo"
		if i == 0 {
			repo = "test/repo.with.dots"
		}
		req := &codereview.Request{ID: uuid.NewString(), Repo: repo, Number: int(time.Now().UnixNano()%1000000) + i, URL: "https://github.com/" + repo + "/pull/1", Origin: codereview.Origin("manual"), Harness: "codex", State: codereview.StateQueued, Attempt: 2}
		if err := store.Create(ctx, req); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			_, err := p.Exec(ctx, `INSERT INTO code_review_findings(id,request_id,attempt,severity,path,line,side,title,body,status) VALUES($1,$2,1,'P1','old.go',1,'RIGHT','old','old','open')`, uuid.NewString(), req.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	list, total, err := store.List(ctx, codereview.Filter{ProjectID: project, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if total < 1 || len(list) != 1 || list[0].Repo != "test/repo.with.dots" {
		t.Fatalf("scope=%v total=%d", list, total)
	}
	if len(list[0].Findings) != 0 {
		t.Fatal("old attempt findings leaked into clean current attempt")
	}
}
func TestDurableBacklogRetriesAndTransactions(t *testing.T) {
	p := pool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kind := "test." + uuid.NewString()
	sub := uuid.NewString()
	entity := uuid.NewString()
	publisher := events.NewPostgresBus(p, events.PostgresBusConfig{})
	if _, err := publisher.PublishDurable(ctx, events.NewEvent(kind, map[string]any{"n": 1}).WithEntityKey(entity)); err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.PublishDurable(ctx, events.NewEvent(kind, map[string]any{"n": 2}).WithEntityKey(entity)); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	var mu sync.Mutex
	var got []int
	consumer := events.NewPostgresBus(p, events.PostgresBusConfig{PollInterval: 20 * time.Millisecond})
	consumer.SubscribePattern(kind, sub, func(ctx context.Context, e events.Event) {
		n := int(e.Payload["n"].(float64))
		if n == 1 && attempts.Add(1) == 1 {
			events.Retry(ctx, errors.New("transient"))
			return
		}
		mu.Lock()
		got = append(got, n)
		mu.Unlock()
	})
	if err := consumer.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer consumer.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := len(got) == 2
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	consumer.Stop()
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("delivery order=%v attempts=%d", got, attempts.Load())
	}
	rollbackKind := "rollback." + uuid.NewString()
	err := db.Transaction(context.Background(), p, func(ctx context.Context) error {
		if _, err := publisher.PublishDurable(ctx, events.NewEvent(rollbackKind, nil)); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	var count int
	if err := p.QueryRow(context.Background(), "SELECT count(*) FROM event_outbox WHERE event_type=$1", rollbackKind).Scan(&count); err != nil || count != 0 {
		t.Fatalf("outbox survived rollback count=%d err=%v", count, err)
	}
}

func TestLocalOperatorBootstrap(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	users, agents := user.NewStore(p), agent.NewStore(p)
	provisioner := &auth.Provisioner{UserStore: users, AgentStore: agents}
	// Reproduce a partial legacy provision: the user exists but its agent does not.
	identity := &user.User{ID: uuid.NewString(), GitHubID: -900001, Login: "bootstrap-test", Email: "bootstrap-test@localhost", CreatedAt: time.Now().UTC()}
	if err := users.Create(ctx, identity); err != nil {
		t.Fatal(err)
	}
	localIdentity := &auth.Identity{ID: identity.GitHubID, Login: identity.Login, Email: identity.Email}
	u, a, err := provisioner.Provision(ctx, localIdentity)
	if err != nil {
		t.Fatal(err)
	}
	orgs := org.NewService(org.NewStore(p))
	if err := orgs.EnsureDefaultOrgForUser(ctx, u.ID, u.Email); err != nil {
		t.Fatal(err)
	}
	before, err := orgs.ListOrgIDsForUser(ctx, u.ID)
	if err != nil || len(before) != 1 {
		t.Fatalf("workspace=%v err=%v", before, err)
	}
	worker, key, err := agent.NewService(agents).RegisterAgentForUser(ctx, "manual-harness", agent.TypeCustom, u.ID)
	if err != nil || worker.UserID != u.ID || key == "" {
		t.Fatalf("worker=%v err=%v", worker, err)
	}
	// Repeated startup reuses the operator, not the newly registered MCP agent.
	nextUser, nextAgent, err := provisioner.Provision(ctx, localIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if nextUser.ID != u.ID || nextAgent.ID != a.ID {
		t.Fatal("restart changed local operator identity")
	}
	if err := orgs.EnsureDefaultOrgForUser(ctx, nextUser.ID, nextUser.Email); err != nil {
		t.Fatal(err)
	}
	after, err := orgs.ListOrgIDsForUser(ctx, nextUser.ID)
	if err != nil || len(after) != 1 || after[0] != before[0] {
		t.Fatalf("restart workspace=%v err=%v", after, err)
	}
}
