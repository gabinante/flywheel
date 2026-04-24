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
	RepoDir      string
	ServerURL    string
	AgentID      string
	HistoryLimit int
	CostSvc      *cost.Service
	AgentRunner  string
	AgentDriver  string
	AgentModel   string
}

type Service struct {
	store    MessageStore
	projects ProjectGetter
	worker   Worker
	cfg      Config
	playbook Playbook
}

func NewService(store MessageStore, projects ProjectGetter, worker Worker, cfg Config) *Service {
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = 200
	}
	return &Service{
		store:    store,
		projects: projects,
		worker:   worker,
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
	if s.worker == nil {
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

	result, err := s.worker.Spawn(ctx, runID, projectID, systemPrompt, taskMessage, workDir, s.cfg.ServerURL)
	if err != nil {
		return nil, err
	}
	if s.cfg.CostSvc != nil {
		provider, model := cost.InferProviderModel(s.cfg.AgentRunner, s.cfg.AgentDriver, s.cfg.AgentModel)
		output := strings.TrimSpace(result.Output)
		if output == "" {
			output = strings.TrimSpace(result.Error)
		}
		_, _ = s.cfg.CostSvc.RecordAndCheck(ctx, &cost.LLMCallRecord{
			ProjectID:     projectID,
			TicketID:      runID,
			WorkerRole:    "orchestrator",
			Provider:      provider,
			Model:         model,
			OperationType: cost.OpPlanning,
			InputTokens:   cost.EstimateTokens(systemPrompt, taskMessage),
			OutputTokens:  cost.EstimateTokens(output),
		})
	}
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
