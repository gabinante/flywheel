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
	WorkerID      string `json:"worker_id,omitempty"`
	Harness       string `json:"harness,omitempty"`
	Model         string `json:"model,omitempty"`
	Effort        string `json:"effort,omitempty"`
	Role          string `json:"role"`                     // executor, planner, validator, deployer, investigator
	Goal          string `json:"goal"`                     // freeform objective
	Prompt        string `json:"prompt,omitempty"`         // custom system prompt supplement
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

// GateCondition describes a single structured condition for a gate phase.
type GateCondition struct {
	Type   string         `json:"type"`             // github_checks | human_approval | webhook | http_check
	Config map[string]any `json:"config,omitempty"` // type-specific params
}

// validGateConditionTypes is the set of recognized condition types.
var validGateConditionTypes = map[string]bool{
	"github_checks":  true,
	"human_approval": true,
	"webhook":        true,
	"http_check":     true,
}

// GatePhaseConfig is the typed config for a gate phase (blocks until conditions are met).
type GatePhaseConfig struct {
	Conditions   []GateCondition `json:"conditions,omitempty"`
	RequiredRole string          `json:"required_role,omitempty"`
	// Deprecated: use Conditions instead. Kept for backward compatibility.
	Prompt       string   `json:"prompt,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

// EffectiveConditions returns the conditions to evaluate, falling back to
// converting legacy Requirements into GateCondition entries.
func (g *GatePhaseConfig) EffectiveConditions() []GateCondition {
	if len(g.Conditions) > 0 {
		return g.Conditions
	}
	var out []GateCondition
	for _, r := range g.Requirements {
		out = append(out, GateCondition{Type: r})
	}
	return out
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

// ValidRoles are the recognized agent phase roles.
var ValidRoles = map[string]bool{
	"executor": true, "planner": true, "validator": true, "deployer": true,
	"investigator": true, "operator": true, "decomposer": true,
	"fast-executor": true, "conflict-resolver": true,
}

// ValidatePhaseConfig checks a phase's config for unknown keys, type errors,
// and semantic issues. Returns a slice of error strings (empty = valid).
// Legacy phase types are skipped.
func ValidatePhaseConfig(pt PhaseType, config map[string]any) []string {
	switch pt {
	case PhaseAgent:
		return validateAgentPhaseConfig(config)
	case PhaseExternal:
		return validateExternalPhaseConfig(config)
	case PhaseGate:
		return validateGatePhaseConfig(config)
	case PhaseAction:
		return validateActionPhaseConfig(config)
	default:
		// Legacy types: skip validation.
		return nil
	}
}

func validateAgentPhaseConfig(config map[string]any) []string {
	var errs []string
	cfg, err := ParseAgentConfig(config)
	if err != nil {
		return []string{fmt.Sprintf("invalid agent config: %v", err)}
	}
	errs = append(errs, detectUnknownKeys(config, "role", "goal", "prompt", "auto_advance", "max_iterations", "harness", "model", "effort", "worker_id")...)
	if cfg.Role != "" && !ValidRoles[cfg.Role] {
		errs = append(errs, fmt.Sprintf("unknown agent role %q; known roles: executor, planner, validator, deployer, investigator, operator, decomposer, fast-executor", cfg.Role))
	}
	if cfg.Harness != "" && cfg.Harness != "codex" && cfg.Harness != "claude" {
		errs = append(errs, "harness must be claude or codex")
	}
	if cfg.MaxIterations < 0 {
		errs = append(errs, "max_iterations must be >= 0")
	}
	return errs
}

func validateExternalPhaseConfig(config map[string]any) []string {
	var errs []string
	cfg, err := ParseExternalConfig(config)
	if err != nil {
		return []string{fmt.Sprintf("invalid external config: %v", err)}
	}
	errs = append(errs, detectUnknownKeys(config, "mode", "url", "method", "headers", "body_template",
		"success_status", "poll_url", "poll_interval", "poll_timeout", "poll_success_condition")...)
	validModes := map[string]bool{"sync": true, "async": true, "poll": true}
	if !validModes[cfg.Mode] {
		errs = append(errs, fmt.Sprintf("invalid external mode %q; must be sync, async, or poll", cfg.Mode))
	}
	if cfg.URL == "" && (cfg.Mode != "poll" || cfg.PollURL == "") {
		errs = append(errs, "external phase requires a URL")
	}
	if cfg.PollInterval != "" {
		if _, parseErr := time.ParseDuration(cfg.PollInterval); parseErr != nil {
			errs = append(errs, fmt.Sprintf("poll_interval %q is not a valid duration", cfg.PollInterval))
		}
	}
	if cfg.PollTimeout != "" {
		if _, parseErr := time.ParseDuration(cfg.PollTimeout); parseErr != nil {
			errs = append(errs, fmt.Sprintf("poll_timeout %q is not a valid duration", cfg.PollTimeout))
		}
	}
	return errs
}

func validateGatePhaseConfig(config map[string]any) []string {
	var errs []string
	cfg, err := ParseGateConfig(config)
	if err != nil {
		return []string{fmt.Sprintf("invalid gate config: %v", err)}
	}
	errs = append(errs, detectUnknownKeys(config, "prompt", "required_role", "requirements", "conditions")...)
	// Validate structured conditions if present.
	for i, c := range cfg.Conditions {
		if !validGateConditionTypes[c.Type] {
			errs = append(errs, fmt.Sprintf("conditions[%d]: unknown type %q", i, c.Type))
		}
		if c.Type == "http_check" {
			if u, _ := c.Config["url"].(string); u == "" {
				errs = append(errs, fmt.Sprintf("conditions[%d]: http_check requires a 'url' in config", i))
			}
		}
	}
	// No longer require prompt — conditions are the primary mechanism.
	return errs
}

func validateActionPhaseConfig(config map[string]any) []string {
	var errs []string
	cfg, err := ParseActionConfig(config)
	if err != nil {
		return []string{fmt.Sprintf("invalid action config: %v", err)}
	}
	errs = append(errs, detectUnknownKeys(config, "action", "params")...)
	if cfg.Action == "" {
		errs = append(errs, "action phase requires a non-empty 'action'")
	} else if cfg.Action != "merge_pr" {
		errs = append(errs, "unsupported action; use merge_pr or an external phase with a URL")
	}
	return errs
}

// detectUnknownKeys returns errors for any keys in config not in the known set.
func detectUnknownKeys(config map[string]any, known ...string) []string {
	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		knownSet[k] = true
	}
	var errs []string
	for k := range config {
		if !knownSet[k] {
			errs = append(errs, fmt.Sprintf("unknown config key %q", k))
		}
	}
	return errs
}

// ActionPhaseConfig is the typed config for an action phase (inline Go function).
type ActionPhaseConfig struct {
	Action string         `json:"action"`           // registered handler name
	Params map[string]any `json:"params,omitempty"` // static parameters
}

// ParseActionConfig extracts typed config from a phase's generic Config map.
func ParseActionConfig(config map[string]any) (*ActionPhaseConfig, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var cfg ActionPhaseConfig
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

// PollOnce performs a read-only status probe. With no JSON condition, configured
// success HTTP statuses satisfy it. A condition is a dotted JSON path with a
// boolean true value (for example deployment.ready).
func (e *ExternalExecutor) PollOnce(ctx context.Context, cfg *ExternalPhaseConfig) (bool, error) {
	url := cfg.PollURL
	if url == "" {
		url = cfg.URL
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	codes := cfg.SuccessStatus
	if len(codes) == 0 {
		codes = []int{200}
	}
	success := false
	for _, code := range codes {
		if resp.StatusCode == code {
			success = true
		}
	}
	if !success {
		return false, nil
	}
	if cfg.PollSuccessCondition == "" {
		return true, nil
	}
	var value any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&value); err != nil {
		return false, err
	}
	for _, part := range strings.Split(cfg.PollSuccessCondition, ".") {
		m, ok := value.(map[string]any)
		if !ok {
			return false, nil
		}
		value = m[part]
	}
	ready, _ := value.(bool)
	return ready, nil
}

func (e *ExternalExecutor) GateCallback(ctx context.Context, ticketID, workflowID, phaseID string) (string, error) {
	if e.callback == nil {
		return "", fmt.Errorf("callbacks not configured")
	}
	token, err := e.callback.GenerateToken(ctx, ticketID, workflowID, phaseID, 24*time.Hour)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/gate/callback/%s", e.baseURL, token), nil
}
