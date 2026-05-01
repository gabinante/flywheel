package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ExternalExecutor fires HTTP requests for external phases.
type ExternalExecutor struct {
	client   *http.Client
	callback *CallbackHandler
	baseURL  string // server base URL for generating callback URLs
}

// NewExternalExecutor creates a new external phase executor.
func NewExternalExecutor(callback *CallbackHandler, baseURL string) *ExternalExecutor {
	return &ExternalExecutor{
		client:   &http.Client{Timeout: 30 * time.Second},
		callback: callback,
		baseURL:  strings.TrimRight(baseURL, "/"),
	}
}

// ExternalPhaseConfig is the typed config for an external phase.
type ExternalPhaseConfig struct {
	Mode                 string            `json:"mode"` // sync, async, poll
	URL                  string            `json:"url"`
	Method               string            `json:"method"` // GET, POST (default POST)
	Headers              map[string]string `json:"headers,omitempty"`
	BodyTemplate         string            `json:"body_template,omitempty"`
	SuccessStatus        []int             `json:"success_status,omitempty"` // sync mode
	PollURL              string            `json:"poll_url,omitempty"`
	PollInterval         string            `json:"poll_interval,omitempty"` // e.g. "30s"
	PollTimeout          string            `json:"poll_timeout,omitempty"`  // e.g. "30m"
	PollSuccessCondition string            `json:"poll_success_condition,omitempty"`
}

// ParseExternalConfig extracts typed config from a phase's generic Config map.
func ParseExternalConfig(config map[string]any) (*ExternalPhaseConfig, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var cfg ExternalPhaseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if cfg.Mode == "" {
		cfg.Mode = "sync"
	}
	if cfg.Method == "" {
		cfg.Method = "POST"
	}
	return &cfg, nil
}

// AgentPhaseConfig is the typed config for an agent phase.
type AgentPhaseConfig struct {
	Role          string `json:"role"`                     // executor, planner, validator, deployer, investigator
	Goal          string `json:"goal"`                     // freeform objective
	Prompt        string `json:"prompt,omitempty"`          // custom system prompt supplement
	AutoAdvance   *bool  `json:"auto_advance"`             // advance on successful submit (default true)
	MaxIterations int    `json:"max_iterations,omitempty"` // 0 = unlimited; for review loops
}

// ParseAgentConfig extracts typed config from a phase's generic Config map.
func ParseAgentConfig(config map[string]any) (*AgentPhaseConfig, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var cfg AgentPhaseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if cfg.Role == "" {
		cfg.Role = "executor"
	}
	return &cfg, nil
}

// GatePhaseConfig is the typed config for a gate phase (blocks until requirements are met).
type GatePhaseConfig struct {
	Prompt       string   `json:"prompt"`
	RequiredRole string   `json:"required_role,omitempty"`
	Requirements []string `json:"requirements,omitempty"` // e.g. ["github_checks"]
}

// ParseGateConfig extracts typed config from a phase's generic Config map.
func ParseGateConfig(config map[string]any) (*GatePhaseConfig, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var cfg GatePhaseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// templateVars are the variables available in body templates.
type templateVars struct {
	TicketID    string `json:"ticket_id"`
	PhaseID     string `json:"phase_id"`
	WorkflowID  string `json:"workflow_id"`
	CallbackURL string `json:"callback_url"`
}

// ExecuteSync fires a synchronous HTTP request and returns success/failed based on status code.
func (e *ExternalExecutor) ExecuteSync(ctx context.Context, cfg *ExternalPhaseConfig, ticketID, workflowID, phaseID string) (outcome string, metadata map[string]any, err error) {
	body, err := e.renderBody(cfg.BodyTemplate, ticketID, workflowID, phaseID, "")
	if err != nil {
		return "failed", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return "failed", nil, fmt.Errorf("create request: %w", err)
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return "failed", map[string]any{"error": err.Error()}, nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	meta := map[string]any{
		"status_code": resp.StatusCode,
		"response":    string(respBody),
	}

	successCodes := cfg.SuccessStatus
	if len(successCodes) == 0 {
		successCodes = []int{200, 201, 204}
	}
	for _, code := range successCodes {
		if resp.StatusCode == code {
			return "success", meta, nil
		}
	}
	return "failed", meta, nil
}

// InitiateAsync fires the initial HTTP request for an async phase and returns the callback token.
func (e *ExternalExecutor) InitiateAsync(ctx context.Context, cfg *ExternalPhaseConfig, ticketID, workflowID, phaseID string) (callbackToken string, err error) {
	if e.callback == nil {
		return "", fmt.Errorf("callback handler not configured")
	}

	ttl := 24 * time.Hour
	token, err := e.callback.GenerateToken(ctx, ticketID, workflowID, phaseID, ttl)
	if err != nil {
		return "", err
	}

	callbackURL := fmt.Sprintf("%s/api/v1/workflow/callback/%s", e.baseURL, token)

	body, err := e.renderBody(cfg.BodyTemplate, ticketID, workflowID, phaseID, callbackURL)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("async request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("async initiate failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return token, nil
}

func (e *ExternalExecutor) renderBody(template, ticketID, workflowID, phaseID, callbackURL string) ([]byte, error) {
	if template == "" {
		vars := templateVars{
			TicketID:    ticketID,
			PhaseID:     phaseID,
			WorkflowID:  workflowID,
			CallbackURL: callbackURL,
		}
		return json.Marshal(vars)
	}
	// Simple variable substitution
	result := template
	result = strings.ReplaceAll(result, "{{ticket_id}}", ticketID)
	result = strings.ReplaceAll(result, "{{phase_id}}", phaseID)
	result = strings.ReplaceAll(result, "{{workflow_id}}", workflowID)
	result = strings.ReplaceAll(result, "{{callback_url}}", callbackURL)
	return []byte(result), nil
}
