package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gabinante/flywheel/internal/cost"
)

// Worker is the interface for spawning subagent processes.
// This matches the dispatch.Worker interface to reuse existing infrastructure.
type Worker interface {
	Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error)
}

// WorkerResult mirrors dispatch.WorkerResult.
type WorkerResult struct {
	Success bool
	Output  string
	Error   string
}

// WorkDirResolver provides the working directory for investigations.
type WorkDirResolver interface {
	// ResolveWorkDir returns the directory to run investigations in.
	// Typically the repo root or a worktree.
	ResolveWorkDir(projectID string) string
}

// Config holds service configuration.
type Config struct {
	// ServerURL is the Flywheel server URL (for MCP access — investigations are read-only).
	ServerURL string

	// DefaultTimeout is the maximum duration for an investigation.
	// Default: 2 minutes.
	DefaultTimeout time.Duration

	// WorkDir is the fallback working directory for investigations.
	WorkDir string

	// CostSvc records estimated usage for investigation sessions.
	CostSvc *cost.Service
	// Agent identity used for provider/model attribution.
	AgentRunner string
	AgentDriver string
	AgentModel  string
}

// Service dispatches investigations to subagent workers.
type Service struct {
	cfg    Config
	worker Worker
}

// NewService creates an investigation service with the given worker and config.
func NewService(worker Worker, cfg Config) *Service {
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = 2 * time.Minute
	}
	return &Service{
		cfg:    cfg,
		worker: worker,
	}
}

// Dispatch runs a scoped investigation and returns structured findings.
// This is synchronous — the coordinator waits for the result.
// One level deep: the investigation subagent does NOT have access to dispatch_investigation.
func (s *Service) Dispatch(ctx context.Context, req *Request) (*Response, error) {
	if req == nil {
		return nil, fmt.Errorf("investigation request is nil")
	}
	if req.Question == "" {
		return nil, fmt.Errorf("investigation question is required")
	}
	if req.ProjectID == "" {
		return nil, fmt.Errorf("investigation project_id is required")
	}

	// Apply token budget defaults and limits.
	tokenBudget := req.TokenBudget
	if tokenBudget <= 0 {
		tokenBudget = DefaultTokenBudget
	}
	if tokenBudget > MaxTokenBudget {
		tokenBudget = MaxTokenBudget
	}

	// Build the investigation system prompt.
	systemPrompt := buildInvestigationPrompt(req, tokenBudget)

	// Build the task message.
	taskMessage := buildTaskMessage(req)

	// Apply timeout.
	timeout := s.cfg.DefaultTimeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine working directory.
	workDir := s.cfg.WorkDir
	if workDir == "" {
		workDir = "."
	}

	// Generate a unique investigation ID for tracking.
	investigationID := fmt.Sprintf("inv-%s-%d", req.ParentTicketID, time.Now().UnixMilli())

	start := time.Now()

	// Spawn the investigation subagent.
	// The subagent uses the same worker infrastructure but with a read-only prompt.
	// It does NOT have access to dispatch_investigation (one-level-deep constraint).
	result, err := s.worker.Spawn(ctx, investigationID, req.ProjectID, systemPrompt, taskMessage, workDir, s.cfg.ServerURL)
	duration := time.Since(start)
	s.recordUsage(ctx, req.ProjectID, investigationID, systemPrompt, taskMessage, result)

	if err != nil {
		// Context timeout or spawn failure.
		if ctx.Err() == context.DeadlineExceeded {
			return &Response{
				Status:   StatusTimeout,
				Question: req.Question,
				Duration: duration,
				Error:    "investigation exceeded time limit",
			}, nil
		}
		return &Response{
			Status:   StatusFailed,
			Question: req.Question,
			Duration: duration,
			Error:    err.Error(),
		}, nil
	}

	if !result.Success {
		return &Response{
			Status:   StatusFailed,
			Question: req.Question,
			Duration: duration,
			Error:    result.Error,
		}, nil
	}

	// Parse the structured output from the subagent.
	response, parseErr := parseInvestigationOutput(result.Output, req.Question, tokenBudget)
	if parseErr != nil {
		// If we can't parse structured output, wrap raw output as a single claim.
		return &Response{
			Status:   StatusPartial,
			Question: req.Question,
			Claims: []Claim{
				{
					Statement:  "Raw investigation output (structured parsing failed)",
					Confidence: "low",
					Citations:  nil,
				},
			},
			NegativeSpace: []string{"Structured output parsing failed: " + parseErr.Error()},
			OpenQuestions: []string{"Re-run investigation with clearer scope"},
			TokensUsed:    estimateTokens(result.Output),
			Duration:      duration,
		}, nil
	}

	response.Duration = duration
	return response, nil
}

func (s *Service) recordUsage(ctx context.Context, projectID, ticketID, systemPrompt, taskMessage string, result *WorkerResult) {
	if s.cfg.CostSvc == nil || result == nil {
		return
	}
	provider, model := cost.InferProviderModel(s.cfg.AgentRunner, s.cfg.AgentDriver, s.cfg.AgentModel)
	output := strings.TrimSpace(result.Output)
	if output == "" {
		output = strings.TrimSpace(result.Error)
	}
	_, _ = s.cfg.CostSvc.RecordAndCheck(ctx, &cost.LLMCallRecord{
		ProjectID:     projectID,
		TicketID:      ticketID,
		WorkerRole:    "investigation",
		Provider:      provider,
		Model:         model,
		OperationType: cost.OpStructuralQuery,
		InputTokens:   cost.EstimateTokens(systemPrompt, taskMessage),
		OutputTokens:  cost.EstimateTokens(output),
	})
}

// parseInvestigationOutput extracts structured findings from the subagent's output.
// The subagent is instructed to output JSON in a specific format.
func parseInvestigationOutput(output, question string, tokenBudget int) (*Response, error) {
	// Look for the JSON block in the output (subagent outputs markdown with a JSON fence).
	jsonStr := extractJSONBlock(output)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON findings block found in output")
	}

	var raw struct {
		Status        string   `json:"status"`
		Claims        []Claim  `json:"claims"`
		NegativeSpace []string `json:"negative_space"`
		OpenQuestions []string `json:"open_questions"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse findings JSON: %w", err)
	}

	status := StatusComplete
	if raw.Status != "" {
		status = Status(raw.Status)
	}

	tokensUsed := estimateTokens(jsonStr)
	if tokensUsed > tokenBudget {
		status = StatusPartial
	}

	return &Response{
		Status:        status,
		Question:      question,
		Claims:        raw.Claims,
		NegativeSpace: raw.NegativeSpace,
		OpenQuestions: raw.OpenQuestions,
		TokensUsed:    tokensUsed,
	}, nil
}

// extractJSONBlock finds the first ```json ... ``` block or raw JSON object in output.
func extractJSONBlock(output string) string {
	// Try fenced code block first.
	const jsonFenceStart = "```json"
	const fenceEnd = "```"

	startIdx := strings.Index(output, jsonFenceStart)
	if startIdx >= 0 {
		contentStart := startIdx + len(jsonFenceStart)
		// Find closing fence.
		endIdx := strings.Index(output[contentStart:], fenceEnd)
		if endIdx >= 0 {
			return strings.TrimSpace(output[contentStart : contentStart+endIdx])
		}
	}

	// Try raw JSON object.
	braceStart := strings.Index(output, "{")
	if braceStart >= 0 {
		// Find matching close brace by counting depth.
		depth := 0
		for i := braceStart; i < len(output); i++ {
			switch output[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := output[braceStart : i+1]
					// Verify it's valid JSON.
					var js json.RawMessage
					if json.Unmarshal([]byte(candidate), &js) == nil {
						return candidate
					}
				}
			}
		}
	}

	return ""
}

// estimateTokens gives a rough token count (≈4 chars per token).
func estimateTokens(s string) int {
	return len(s) / 4
}
