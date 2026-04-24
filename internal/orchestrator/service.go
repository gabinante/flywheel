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

type MessageStore interface {
	Create(ctx context.Context, msg *Message) error
	ListByProjectID(ctx context.Context, projectID string, limit int) ([]Message, error)
}

type ProjectGetter interface {
	GetProject(ctx context.Context, id string) (*project.Project, error)
}

type Worker interface {
	Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*dispatch.WorkerResult, error)
}

type Config struct {
	Enabled      bool
	RepoDir      string
	ServerURL    string
	AgentID      string
	HistoryLimit int
	CostSvc      *cost.Service
	AgentRunner  string
	AgentDriver  string
	AgentModel   string
	WorkerConfig dispatch.Config
}

type Service struct {
	store    MessageStore
	projects ProjectGetter
	worker   Worker
	router   *dispatch.ProjectWorkerRouter
	cfg      Config
	playbook Playbook
}

func NewService(store MessageStore, projects ProjectGetter, worker Worker, cfg Config) *Service {
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = 200
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
	msgs, err := s.store.ListByProjectID(ctx, projectID, s.cfg.HistoryLimit)
	if err != nil {
		return nil, err
	}
	if msgs == nil {
		msgs = []Message{}
	}
	return &Thread{
		ProjectID: projectID,
		Messages:  msgs,
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
	if err := s.store.Create(ctx, &userMsg); err != nil {
		return nil, err
	}

	thread, err := s.GetThread(ctx, projectID)
	if err != nil {
		return nil, err
	}

	systemPrompt := strings.TrimSpace(dispatch.AssembleCoordinatorPrompt(proj, s.cfg.ServerURL, s.cfg.AgentID) + "\n\n" + s.playbook.PromptAppendix())
	taskMessage := buildConversationTask(proj, thread.Messages)
	runID := "orch-" + uuid.Must(uuid.NewV7()).String()
	workDir := strings.TrimSpace(s.cfg.RepoDir)
	if workDir == "" {
		workDir = strings.TrimSpace(proj.RepoURL)
	}
	if workDir == "" {
		workDir = "."
	}

	result, selected, err := s.runOrchestratorWorker(ctx, proj, runID, projectID, systemPrompt, taskMessage, workDir)
	if err != nil {
		return nil, err
	}
	s.recordUsage(ctx, selected.Config, projectID, runID, systemPrompt, taskMessage, result)
	if !result.Success {
		if strings.TrimSpace(result.Error) != "" {
			return nil, fmt.Errorf("%w: %s", ErrOrchestratorRunFailed, strings.TrimSpace(result.Error))
		}
		return nil, ErrOrchestratorRunFailed
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
	if err := s.store.Create(ctx, &assistantMsg); err != nil {
		return nil, err
	}

	return s.GetThread(ctx, projectID)
}

func (s *Service) runOrchestratorWorker(ctx context.Context, proj *project.Project, runID, projectID, systemPrompt, taskMessage, workDir string) (*dispatch.WorkerResult, dispatch.RoutedWorker, error) {
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
		result, err := worker.Spawn(ctx, runID, projectID, systemPrompt, taskMessage, workDir, s.cfg.ServerURL)
		if err == nil && result != nil && result.Success {
			return result, candidate, nil
		}
		lastResult = result
		lastErr = err
		lastWorker = candidate
		if index < len(candidates)-1 && dispatch.ShouldFailoverToNextWorker(err, result) {
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
		role := "User"
		if msg.Role == RoleAssistant {
			role = "Assistant"
		}
		b.WriteString(role + ": " + msg.Content + "\n\n")
	}
	b.WriteString("Respond to the latest user message. When you create tickets or a work stream, summarize exactly what you created and why.")
	return strings.TrimSpace(b.String())
}
