package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gabinante/flywheel/internal/cost"
	"github.com/gabinante/flywheel/internal/dispatch"
	"github.com/gabinante/flywheel/internal/project"
)

var (
	ErrMessageContentRequired = errors.New("message content is required")
	ErrWorkerNotConfigured    = errors.New("orchestrator worker is not configured")
	ErrOrchestratorRunFailed  = errors.New("orchestrator run failed")
	ErrRunNotActive           = errors.New("run is not active")
)

type ConversationStore interface {
	CreateMessage(ctx context.Context, msg *Message) error
	ListMessagesByProjectID(ctx context.Context, projectID string, limit int) ([]Message, error)
	CreateRun(ctx context.Context, run *Run) error
	UpdateRun(ctx context.Context, run *Run) error
	AppendRunEvent(ctx context.Context, event *RunEvent) error
	ListRunsByProjectID(ctx context.Context, projectID string, limit int) ([]Run, error)
	ListRunEventsByRunIDs(ctx context.Context, runIDs []string, limitPerRun int) (map[string][]RunEvent, error)
}

type ProjectGetter interface {
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

type Worker interface {
	Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*dispatch.WorkerResult, error)
}

type Config struct {
	Enabled       bool
	RepoDir       string
	ServerURL     string
	AgentID       string
	HistoryLimit  int
	RunLimit      int
	RunEventLimit int
	CostSvc       *cost.Service
	AgentRunner   string
	AgentDriver   string
	AgentModel    string
	WorkerConfig  dispatch.Config
}

type eventSubscriber struct {
	ch        chan RunEvent
	projectID string
}

type Service struct {
	store    ConversationStore
	projects ProjectGetter
	worker   Worker
	router   *dispatch.ProjectWorkerRouter
	cfg      Config
	playbook Playbook

	serverCtx context.Context
	activeRuns sync.Map // runID → context.CancelFunc

	// SSE pub/sub
	subsMu      sync.Mutex
	subscribers map[string][]*eventSubscriber // projectID → subscribers

	// run → project mapping for SSE fan-out
	runProjects sync.Map // runID → projectID

	seMu             sync.Mutex
	seenSystemEvents map[string][]string // projectID → bounded ring of content hashes
}

func NewService(serverCtx context.Context, store ConversationStore, projects ProjectGetter, worker Worker, cfg Config) *Service {
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = 200
	}
	if cfg.RunLimit <= 0 {
		cfg.RunLimit = 20
	}
	if cfg.RunEventLimit <= 0 {
		cfg.RunEventLimit = 200
	}
	var router *dispatch.ProjectWorkerRouter
	if cfg.Enabled {
		router = dispatch.NewProjectWorkerRouter(cfg.WorkerConfig)
	}
	return &Service{
		store:            store,
		projects:         projects,
		worker:           worker,
		router:           router,
		cfg:              cfg,
		playbook:         DefaultPlaybook(),
		serverCtx:        serverCtx,
		subscribers:      make(map[string][]*eventSubscriber),
		seenSystemEvents: make(map[string][]string),
	}
}

func (s *Service) GetThread(ctx context.Context, projectID string) (*Thread, error) {
	msgs, err := s.store.ListMessagesByProjectID(ctx, projectID, s.cfg.HistoryLimit)
	if err != nil {
		return nil, err
	}
	if msgs == nil {
		msgs = []Message{}
	}
	runs, err := s.store.ListRunsByProjectID(ctx, projectID, s.cfg.RunLimit)
	if err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []Run{}
	}
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.ID)
	}
	eventsByRun, err := s.store.ListRunEventsByRunIDs(ctx, runIDs, s.cfg.RunEventLimit)
	if err != nil {
		return nil, err
	}
	for index := range runs {
		runs[index].Events = eventsByRun[runs[index].ID]
		if runs[index].Events == nil {
			runs[index].Events = []RunEvent{}
		}
	}
	return &Thread{
		ProjectID: projectID,
		Messages:  msgs,
		Runs:      runs,
		Playbook:  s.playbook,
	}, nil
}

// SendUserMessage creates the user message and run record, then launches the
// worker in a background goroutine. It returns the thread immediately (run in
// "running" state) so the HTTP request is not blocked for the worker duration.
func (s *Service) SendUserMessage(ctx context.Context, projectID, content string) (*Thread, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrMessageContentRequired
	}
	if s.worker == nil && !s.cfg.Enabled {
		return nil, ErrWorkerNotConfigured
	}

	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	userMsg := Message{
		ID:        uuid.Must(uuid.NewV7()).String(),
		ProjectID: projectID,
		Role:      RoleUser,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateMessage(ctx, &userMsg); err != nil {
		return nil, err
	}

	run := Run{
		ID:            "orch-" + uuid.Must(uuid.NewV7()).String(),
		ProjectID:     projectID,
		UserMessageID: userMsg.ID,
		Status:        RunStatusRunning,
		Phase:         PhaseQueued,
		StartedAt:     time.Now().UTC(),
	}
	if err := s.store.CreateRun(ctx, &run); err != nil {
		return nil, err
	}
	s.runProjects.Store(run.ID, projectID)
	s.appendRunEvent(ctx, run.ID, RunEventKindStatus, map[string]any{
		"message": "Planner run queued.",
		"status":  string(run.Status),
	})
	s.appendRunEvent(ctx, run.ID, RunEventKindPhaseChange, map[string]any{
		"phase": string(PhaseQueued),
	})

	thread, err := s.GetThread(ctx, projectID)
	if err != nil {
		return nil, err
	}

	systemPrompt := strings.TrimSpace(dispatch.AssembleCoordinatorPrompt(proj, s.cfg.ServerURL, s.cfg.AgentID) + "\n\n" + s.playbook.PromptAppendix())
	taskMessage := buildConversationTask(proj, thread.Messages)
	workDir := strings.TrimSpace(s.cfg.RepoDir)
	if workDir == "" {
		workDir = strings.TrimSpace(proj.RepoURL)
	}
	if workDir == "" {
		workDir = "."
	}

	// Launch worker in background goroutine detached from the HTTP context.
	runCopy := run
	go s.executeRun(proj, &runCopy, projectID, systemPrompt, taskMessage, workDir)

	return s.GetThread(ctx, projectID)
}

// executeRun runs the orchestrator worker in the background. It uses the server
// context (not the HTTP context) so it survives after the POST returns.
func (s *Service) executeRun(proj *project.Project, run *Run, projectID, systemPrompt, taskMessage, workDir string) {
	ctx, cancel := context.WithCancel(s.serverCtx)
	s.activeRuns.Store(run.ID, cancel)
	defer func() {
		cancel()
		s.activeRuns.Delete(run.ID)
		s.runProjects.Delete(run.ID)
		if r := recover(); r != nil {
			run.Status = RunStatusFailed
			run.Error = fmt.Sprintf("panic: %v", r)
			now := time.Now().UTC()
			run.CompletedAt = &now
			run.Phase = PhaseFailed
			_ = s.store.UpdateRun(context.Background(), run)
			s.appendRunEvent(context.Background(), run.ID, RunEventKindError, map[string]any{
				"message": run.Error,
			})
		}
	}()

	s.updatePhase(ctx, run, PhaseConnecting)

	result, selected, err := s.runOrchestratorWorker(ctx, proj, run, projectID, systemPrompt, taskMessage, workDir)
	s.recordUsage(ctx, selected.Config, projectID, run.ID, systemPrompt, taskMessage, result)

	if ctx.Err() == context.Canceled {
		run.Status = RunStatusCancelled
		run.Phase = PhaseCancelled
		now := time.Now().UTC()
		run.CompletedAt = &now
		run.Error = "run cancelled by user"
		_ = s.store.UpdateRun(context.Background(), run)
		s.appendRunEvent(context.Background(), run.ID, RunEventKindStatus, map[string]any{
			"message": "Run cancelled.",
			"status":  string(run.Status),
		})
		s.appendRunEvent(context.Background(), run.ID, RunEventKindPhaseChange, map[string]any{
			"phase": string(PhaseCancelled),
		})
		return
	}

	if err != nil {
		s.failRun(ctx, run, selected, result, err)
		return
	}
	if !result.Success {
		runErr := ErrOrchestratorRunFailed
		if strings.TrimSpace(result.Error) != "" {
			runErr = fmt.Errorf("%w: %s", ErrOrchestratorRunFailed, strings.TrimSpace(result.Error))
		}
		s.failRun(ctx, run, selected, result, runErr)
		return
	}

	s.updatePhase(ctx, run, PhaseComposing)

	reply := strings.TrimSpace(result.Output)
	if reply == "" {
		reply = "No response generated."
	}
	assistantMsg := Message{
		ID:        uuid.Must(uuid.NewV7()).String(),
		ProjectID: projectID,
		Role:      RoleAssistant,
		Content:   reply,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.CreateMessage(ctx, &assistantMsg); err != nil {
		s.failRun(ctx, run, selected, result, err)
		return
	}
	run.AssistantMessageID = assistantMsg.ID
	run.Status = RunStatusCompleted
	run.Phase = PhaseComplete
	run.Error = ""
	run.CompletedAt = ptrTime(assistantMsg.CreatedAt)
	if err := s.store.UpdateRun(ctx, run); err != nil {
		return
	}
	s.appendRunEvent(ctx, run.ID, RunEventKindStatus, map[string]any{
		"message": "Planner completed.",
		"status":  string(run.Status),
	})
	s.appendRunEvent(ctx, run.ID, RunEventKindPhaseChange, map[string]any{
		"phase": string(PhaseComplete),
	})
}

// CancelRun cancels an active orchestrator run. It validates the run belongs
// to the given project before allowing cancellation.
func (s *Service) CancelRun(ctx context.Context, projectID, runID string) error {
	storedProjectID, ok := s.runProjects.Load(runID)
	if !ok || storedProjectID.(string) != projectID {
		return ErrRunNotActive
	}
	cancelFn, ok := s.activeRuns.Load(runID)
	if !ok {
		return ErrRunNotActive
	}
	cancelFn.(context.CancelFunc)()
	return nil
}

// SubscribeRunEvents returns a channel that receives run events for a project
// in real-time. The channel is closed when the context is cancelled.
func (s *Service) SubscribeRunEvents(ctx context.Context, projectID string) <-chan RunEvent {
	ch := make(chan RunEvent, 64)
	sub := &eventSubscriber{ch: ch, projectID: projectID}
	s.subsMu.Lock()
	s.subscribers[projectID] = append(s.subscribers[projectID], sub)
	s.subsMu.Unlock()
	go func() {
		<-ctx.Done()
		s.removeSub(projectID, ch)
		close(ch)
	}()
	return ch
}

func (s *Service) removeSub(projectID string, ch chan RunEvent) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	subs := s.subscribers[projectID]
	for i, sub := range subs {
		if sub.ch == ch {
			s.subscribers[projectID] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

func (s *Service) notifySubscribers(projectID string, event RunEvent) {
	s.subsMu.Lock()
	subs := make([]*eventSubscriber, len(s.subscribers[projectID]))
	copy(subs, s.subscribers[projectID])
	s.subsMu.Unlock()
	for _, sub := range subs {
		select {
		case sub.ch <- event:
		default: // drop if subscriber is slow
		}
	}
}

func (s *Service) updatePhase(ctx context.Context, run *Run, phase OrchestratorPhase) {
	run.Phase = phase
	_ = s.store.UpdateRun(ctx, run)
	s.appendRunEvent(ctx, run.ID, RunEventKindPhaseChange, map[string]any{
		"phase": string(phase),
	})
}

func (s *Service) runOrchestratorWorker(ctx context.Context, proj *project.Project, run *Run, projectID, systemPrompt, taskMessage, workDir string) (*dispatch.WorkerResult, dispatch.RoutedWorker, error) {
	candidates := []dispatch.RoutedWorker{{
		ID:         dispatch.DefaultProjectWorkerID,
		Name:       "Default server worker",
		Config:     s.cfg.WorkerConfig,
		UseDefault: true,
	}}
	if s.router != nil {
		candidates = s.router.Candidates(proj, dispatch.WorkerRoleOrchestrator)
	}

	var lastResult *dispatch.WorkerResult
	var lastErr error
	var lastWorker dispatch.RoutedWorker
	for index, candidate := range candidates {
		worker := s.resolveWorker(candidate)
		if worker == nil {
			lastErr = ErrWorkerNotConfigured
			lastWorker = candidate
			continue
		}
		run.WorkerID = candidate.ID
		run.WorkerName = candidate.Name
		run.Runner = runnerNameFromConfig(candidate.Config)
		run.Driver = candidate.Config.AgentDriver
		run.Model = candidate.Config.AgentModel
		if err := s.store.UpdateRun(ctx, run); err != nil {
			return nil, candidate, err
		}
		s.appendRunEvent(ctx, run.ID, RunEventKindStatus, map[string]any{
			"message": "Planner started.",
			"status":  string(run.Status),
			"worker":  candidate.Name,
			"runner":  run.Runner,
			"driver":  run.Driver,
			"model":   run.Model,
		})

		var (
			result *dispatch.WorkerResult
			err    error
		)
		if streamable, ok := worker.(dispatch.StreamableWorker); ok {
			result, err = streamable.SpawnStream(
				ctx,
				run.ID,
				projectID,
				systemPrompt,
				taskMessage,
				workDir,
				s.cfg.ServerURL,
				s.traceRunOutput(run.ID, run),
			)
		} else {
			result, err = worker.Spawn(ctx, run.ID, projectID, systemPrompt, taskMessage, workDir, s.cfg.ServerURL)
		}
		if err == nil && result != nil && result.Success {
			return result, candidate, nil
		}
		lastResult = result
		lastErr = err
		lastWorker = candidate
		if index < len(candidates)-1 && dispatch.ShouldFailoverToNextWorker(err, result) {
			s.appendRunEvent(ctx, run.ID, RunEventKindStatus, map[string]any{
				"message": "Planner failed over to the next worker candidate.",
				"worker":  candidate.Name,
			})
			continue
		}
		if err != nil {
			return nil, candidate, err
		}
		return result, candidate, nil
	}
	if lastErr != nil {
		return nil, lastWorker, lastErr
	}
	return lastResult, lastWorker, nil
}

func (s *Service) resolveWorker(candidate dispatch.RoutedWorker) Worker {
	if candidate.UseDefault {
		if s.worker != nil {
			return s.worker
		}
		if !s.cfg.Enabled {
			return nil
		}
		return dispatch.NewWorker(s.cfg.WorkerConfig)
	}
	return dispatch.NewWorker(candidate.Config)
}

// knownToolNames are Flywheel MCP tools that appear in worker output.
var knownToolNames = []string{
	"create_ticket", "update_ticket", "create_work_stream", "update_work_stream",
	"update_work_stream_plan", "get_project_context", "list_tickets", "get_ticket",
	"list_work_streams", "get_work_stream", "list_orgs", "list_projects",
	"update_project_context",
}

// readToolNames are tools that only read data (used for phase inference).
var readToolNames = map[string]bool{
	"get_project_context": true,
	"list_tickets":        true,
	"get_ticket":          true,
	"list_work_streams":   true,
	"get_work_stream":     true,
	"list_orgs":           true,
	"list_projects":       true,
}

func classifyWorkerOutput(stream, text string) (RunEventKind, map[string]any) {
	trimmed := strings.TrimSpace(text)
	// Semantic stream markers from OpenAI runners (fast path).
	switch stream {
	case "tool_call":
		tool := trimmed
		if tool == "" {
			tool = "unknown"
		}
		return RunEventKindToolCall, map[string]any{"tool": tool}
	case "tool_result":
		tool, summary, status := parseToolResult(trimmed)
		return RunEventKindToolResult, map[string]any{
			"tool": tool, "summary": truncateText(summary, 500), "status": status,
		}
	}
	// Fallback heuristic for CLI/Docker workers.
	for _, tool := range knownToolNames {
		if strings.Contains(trimmed, tool) {
			return RunEventKindToolCall, map[string]any{
				"stream": stream, "text": trimmed, "tool": tool,
			}
		}
	}
	return RunEventKindWorkerOutput, map[string]any{
		"stream": stream, "text": trimmed,
	}
}

// parseToolResult splits "name: output" and detects "name failed: error" pattern.
func parseToolResult(text string) (tool, summary, status string) {
	status = "success"
	if idx := strings.Index(text, " failed: "); idx >= 0 {
		tool = strings.TrimSpace(text[:idx])
		summary = strings.TrimSpace(text[idx+len(" failed: "):])
		status = "error"
		return
	}
	if idx := strings.Index(text, ": "); idx >= 0 {
		tool = strings.TrimSpace(text[:idx])
		summary = strings.TrimSpace(text[idx+2:])
		return
	}
	tool = text
	return
}

func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (s *Service) traceRunOutput(runID string, run *Run) dispatch.WorkerOutputHandler {
	return func(stream, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		kind, payload := classifyWorkerOutput(stream, text)
		s.appendRunEvent(context.Background(), runID, kind, payload)

		// Phase inference from tool calls and results.
		if kind == RunEventKindToolCall || kind == RunEventKindToolResult {
			toolName, _ := payload["tool"].(string)
			if readToolNames[toolName] {
				if run.Phase == PhaseConnecting || run.Phase == PhaseQueued {
					s.updatePhase(context.Background(), run, PhaseInvestigating)
				}
			} else {
				if run.Phase != PhaseAuthoring {
					s.updatePhase(context.Background(), run, PhaseAuthoring)
				}
			}
		}
	}
}

func (s *Service) failRun(ctx context.Context, run *Run, selected dispatch.RoutedWorker, result *dispatch.WorkerResult, err error) {
	run.Status = RunStatusFailed
	run.Phase = PhaseFailed
	now := time.Now().UTC()
	run.CompletedAt = &now
	run.Error = strings.TrimSpace(err.Error())
	if run.WorkerID == "" {
		run.WorkerID = selected.ID
		run.WorkerName = selected.Name
		run.Runner = runnerNameFromConfig(selected.Config)
		run.Driver = selected.Config.AgentDriver
		run.Model = selected.Config.AgentModel
	}
	if updateErr := s.store.UpdateRun(ctx, run); updateErr != nil {
		return
	}
	if result != nil && strings.TrimSpace(result.Output) != "" {
		s.appendRunEvent(ctx, run.ID, RunEventKindWorkerOutput, map[string]any{
			"stream": "summary",
			"text":   strings.TrimSpace(result.Output),
		})
	}
	s.appendRunEvent(ctx, run.ID, RunEventKindError, map[string]any{
		"message": strings.TrimSpace(err.Error()),
	})
	s.appendRunEvent(ctx, run.ID, RunEventKindPhaseChange, map[string]any{
		"phase": string(PhaseFailed),
	})
}

func (s *Service) appendRunEvent(ctx context.Context, runID string, kind RunEventKind, payload map[string]any) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	event := &RunEvent{
		ID:        uuid.Must(uuid.NewV7()).String(),
		RunID:     runID,
		Kind:      kind,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
	_ = s.store.AppendRunEvent(ctx, event)

	// Fan out to SSE subscribers.
	if projectID, ok := s.runProjects.Load(runID); ok {
		s.notifySubscribers(projectID.(string), *event)
	}
}

func (s *Service) recordUsage(ctx context.Context, workerCfg dispatch.Config, projectID, ticketID, systemPrompt, taskMessage string, result *dispatch.WorkerResult) {
	if s.cfg.CostSvc == nil || result == nil {
		return
	}
	provider, model := cost.InferProviderModel(workerCfg.AgentRunner, workerCfg.AgentDriver, workerCfg.AgentModel)
	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = strings.TrimSpace(result.Error)
	}
	_, _ = s.cfg.CostSvc.RecordAndCheck(ctx, &cost.LLMCallRecord{
		ProjectID:     projectID,
		TicketID:      ticketID,
		WorkerRole:    "orchestrator",
		Provider:      provider,
		Model:         model,
		OperationType: cost.OpPlanning,
		InputTokens:   cost.EstimateTokens(systemPrompt, taskMessage),
		OutputTokens:  cost.EstimateTokens(output),
	})
}

// InjectSystemEvent creates a system-role message in the command center thread.
// This surfaces lifecycle events (escalations, failures, gate blocks, completions)
// so the orchestrator agent sees them as context on its next invocation.
func (s *Service) InjectSystemEvent(ctx context.Context, projectID, category, summary string) error {
	if projectID == "" || summary == "" {
		return nil
	}
	content := fmt.Sprintf("[%s] %s", category, summary)

	const maxSeen = 50
	s.seMu.Lock()
	seen := s.seenSystemEvents[projectID]
	for _, prev := range seen {
		if prev == content {
			s.seMu.Unlock()
			return nil
		}
	}
	seen = append(seen, content)
	if len(seen) > maxSeen {
		seen = seen[len(seen)-maxSeen:]
	}
	s.seenSystemEvents[projectID] = seen
	s.seMu.Unlock()

	msg := Message{
		ID:        uuid.Must(uuid.NewV7()).String(),
		ProjectID: projectID,
		Role:      RoleSystem,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	return s.store.CreateMessage(ctx, &msg)
}

func buildConversationTask(proj *project.Project, messages []Message) string {
	var b strings.Builder
	b.WriteString("You are operating inside the Flywheel command center.\n\n")
	b.WriteString("Your job is to converse with the human, inspect the project, and create or update work streams and tickets when enough clarity exists.\n")
	b.WriteString("If the request is still ambiguous, ask the shortest set of clarification questions that will unblock ticket authoring.\n")
	b.WriteString("If enough clarity exists, create the work in Flywheel during this turn instead of only describing a plan.\n")
	b.WriteString("Do not execute coding work yourself. Stay at the orchestration layer.\n\n")
	b.WriteString(fmt.Sprintf("Project: %s (%s)\n\n", proj.Name, proj.ID))
	b.WriteString("Conversation so far (oldest first):\n\n")
	for _, msg := range messages {
		var role string
		switch msg.Role {
		case RoleAssistant:
			role = "Assistant"
		case RoleSystem:
			role = "System"
		default:
			role = "User"
		}
		b.WriteString(role + ": " + msg.Content + "\n\n")
	}
	b.WriteString("Respond to the latest user message. When you create tickets or a work stream, summarize exactly what you created and why.")
	return strings.TrimSpace(b.String())
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func runnerNameFromConfig(cfg dispatch.Config) string {
	if strings.TrimSpace(cfg.AgentRunner) != "" {
		return strings.TrimSpace(cfg.AgentRunner)
	}
	if cfg.DockerEnabled {
		return "docker"
	}
	return "cli"
}

