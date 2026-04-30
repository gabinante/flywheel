package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

type Service struct {
	store    ConversationStore
	projects ProjectGetter
	worker   Worker
	router   *dispatch.ProjectWorkerRouter
	cfg      Config
	playbook Playbook
}

func NewService(store ConversationStore, projects ProjectGetter, worker Worker, cfg Config) *Service {
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
		store:    store,
		projects: projects,
		worker:   worker,
		router:   router,
		cfg:      cfg,
		playbook: DefaultPlaybook(),
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
		StartedAt:     time.Now().UTC(),
	}
	if err := s.store.CreateRun(ctx, &run); err != nil {
		return nil, err
	}
	s.appendRunEvent(ctx, run.ID, RunEventKindStatus, map[string]any{
		"message": "Planner run queued.",
		"status":  string(run.Status),
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

	result, selected, err := s.runOrchestratorWorker(ctx, proj, &run, projectID, systemPrompt, taskMessage, workDir)
	s.recordUsage(ctx, selected.Config, projectID, run.ID, systemPrompt, taskMessage, result)
	if err != nil {
		s.failRun(ctx, &run, selected, result, err)
		return nil, err
	}
	if !result.Success {
		runErr := ErrOrchestratorRunFailed
		if strings.TrimSpace(result.Error) != "" {
			runErr = fmt.Errorf("%w: %s", ErrOrchestratorRunFailed, strings.TrimSpace(result.Error))
		}
		s.failRun(ctx, &run, selected, result, runErr)
		return nil, runErr
	}

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
		return nil, err
	}
	run.AssistantMessageID = assistantMsg.ID
	run.Status = RunStatusCompleted
	run.Error = ""
	run.CompletedAt = ptrTime(assistantMsg.CreatedAt)
	if err := s.store.UpdateRun(ctx, &run); err != nil {
		return nil, err
	}
	s.appendRunEvent(ctx, run.ID, RunEventKindStatus, map[string]any{
		"message": "Planner completed.",
		"status":  string(run.Status),
	})

	return s.GetThread(ctx, projectID)
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
				s.traceRunOutput(run.ID),
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

func (s *Service) traceRunOutput(runID string) dispatch.WorkerOutputHandler {
	return func(stream, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		s.appendRunEvent(context.Background(), runID, RunEventKindWorkerOutput, map[string]any{
			"stream": stream,
			"text":   text,
		})
	}
}

func (s *Service) failRun(ctx context.Context, run *Run, selected dispatch.RoutedWorker, result *dispatch.WorkerResult, err error) {
	run.Status = RunStatusFailed
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
	msg := Message{
		ID:        uuid.Must(uuid.NewV7()).String(),
		ProjectID: projectID,
		Role:      RoleSystem,
		Content:   fmt.Sprintf("[%s] %s", category, summary),
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
