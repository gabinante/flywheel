package dispatch

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

// OpenAIResponsesConfig configures the API-native OpenAI runner.
type OpenAIResponsesConfig struct {
	APIBaseURL      string
	APIKey          string
	Model           string
	ReasoningEffort string
	PollInterval    time.Duration
	HTTPClient      *http.Client
}

// OpenAIResponsesWorker uses the OpenAI Responses API with a remote MCP tool.
// This is an API-native execution backend rather than a CLI harness.
type OpenAIResponsesWorker struct {
	Config OpenAIResponsesConfig
	APIKey string // Flywheel API key for MCP authentication
}

func (w *OpenAIResponsesWorker) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
	cfg := w.Config
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-5.2-codex"
	}
	if cfg.ReasoningEffort == "" {
		cfg.ReasoningEffort = "medium"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.APIKey == "" {
		return &WorkerResult{
			Success: false,
			Error:   "missing OpenAI API key",
		}, nil
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	mcpConn := buildMCPConnection(serverURL, w.APIKey)
	tools := append([]openAIResponsesTool{
		{
			Type:            "mcp",
			ServerLabel:     mcpConn.Name,
			ServerURL:       mcpConn.HTTPURL,
			Headers:         cloneStringMap(mcpConn.Headers),
			RequireApproval: "never",
		},
	}, openAIWorkspaceTools()...)
	reqBody := openAIResponsesRequest{
		Model:        cfg.Model,
		Instructions: systemPrompt,
		Input:        taskMessage,
		Background:   true,
		Tools:        tools,
		ToolChoice:   "auto",
		Metadata: map[string]string{
			"ticket_id":  ticketID,
			"project_id": projectID,
			"source":     "flywheel-dispatch",
		},
	}
	if cfg.ReasoningEffort != "" {
		reqBody.Reasoning = &openAIResponsesReasoning{Effort: cfg.ReasoningEffort}
	}

	resp, err := w.createResponse(ctx, client, cfg, reqBody)
	if err != nil {
		return nil, err
	}
	resp, err = w.waitForResponse(ctx, client, cfg, resp)
	if err != nil {
		return nil, err
	}

	for round := 0; round < 32; round++ {
		calls := resp.functionCalls()
		if len(calls) == 0 {
			break
		}
		nextInput := make([]openAIResponsesFunctionCallOutput, 0, len(calls))
		for _, call := range calls {
			nextInput = append(nextInput, openAIResponsesFunctionCallOutput{
				Type:   "function_call_output",
				CallID: call.CallID,
				Output: executeOpenAIWorkspaceTool(ctx, workDir, call),
			})
		}

		resp, err = w.createResponse(ctx, client, cfg, openAIResponsesRequest{
			Model:              cfg.Model,
			Instructions:       systemPrompt,
			PreviousResponseID: resp.ID,
			Input:              nextInput,
			Background:         true,
			Tools:              tools,
			ToolChoice:         "auto",
			Metadata: map[string]string{
				"ticket_id":  ticketID,
				"project_id": projectID,
				"source":     "flywheel-dispatch",
			},
			Reasoning: reqBody.Reasoning,
		})
		if err != nil {
			return nil, err
		}
		resp, err = w.waitForResponse(ctx, client, cfg, resp)
		if err != nil {
			return nil, err
		}
	}
	if len(resp.functionCalls()) > 0 {
		return &WorkerResult{
			Success: false,
			Error:   "response exceeded function call round limit",
		}, nil
	}

	result := &WorkerResult{
		Success: resp.Status == "completed",
		Output:  resp.outputText(),
	}
	if result.Output == "" {
		result.Output = resp.toolLog()
	}
	if result.Success {
		return result, nil
	}
	result.Error = resp.failureReason()
	if result.Error == "" {
		result.Error = "response did not complete successfully"
	}
	return result, nil
}

func (w *OpenAIResponsesWorker) waitForResponse(ctx context.Context, client *http.Client, cfg OpenAIResponsesConfig, resp *openAIResponsesResponse) (*openAIResponsesResponse, error) {
	for resp.Status == "in_progress" || resp.Status == "queued" {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(cfg.PollInterval):
		}
		next, err := w.retrieveResponse(ctx, client, cfg, resp.ID)
		if err != nil {
			return nil, err
		}
		resp = next
	}
	return resp, nil
}

func (w *OpenAIResponsesWorker) createResponse(ctx context.Context, client *http.Client, cfg OpenAIResponsesConfig, payload openAIResponsesRequest) (*openAIResponsesResponse, error) {
	return w.doJSON(ctx, client, cfg, http.MethodPost, strings.TrimRight(cfg.APIBaseURL, "/")+"/responses", payload)
}

func (w *OpenAIResponsesWorker) retrieveResponse(ctx context.Context, client *http.Client, cfg OpenAIResponsesConfig, responseID string) (*openAIResponsesResponse, error) {
	return w.doJSON(ctx, client, cfg, http.MethodGet, strings.TrimRight(cfg.APIBaseURL, "/")+"/responses/"+responseID, nil)
}

func (w *OpenAIResponsesWorker) doJSON(ctx context.Context, client *http.Client, cfg OpenAIResponsesConfig, method, url string, payload any) (*openAIResponsesResponse, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal openai responses request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build openai responses request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai responses request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai responses body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr openAIErrorResponse
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("openai responses api: %s", apiErr.Error.Message)
		}
		return nil, fmt.Errorf("openai responses api: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed openAIResponsesResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode openai responses body: %w", err)
	}
	return &parsed, nil
}

type openAIResponsesRequest struct {
	Model              string                    `json:"model"`
	Instructions       string                    `json:"instructions,omitempty"`
	PreviousResponseID string                    `json:"previous_response_id,omitempty"`
	Input              any                       `json:"input"`
	Background         bool                      `json:"background"`
	Reasoning          *openAIResponsesReasoning `json:"reasoning,omitempty"`
	Tools              []openAIResponsesTool     `json:"tools,omitempty"`
	ToolChoice         string                    `json:"tool_choice,omitempty"`
	Metadata           map[string]string         `json:"metadata,omitempty"`
}

type openAIResponsesFunctionCallOutput struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Output string `json:"output"`
}

type openAIResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type openAIResponsesTool struct {
	Type            string            `json:"type"`
	ServerLabel     string            `json:"server_label,omitempty"`
	ServerURL       string            `json:"server_url,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	RequireApproval string            `json:"require_approval,omitempty"`
	Name            string            `json:"name,omitempty"`
	Description     string            `json:"description,omitempty"`
	Parameters      map[string]any    `json:"parameters,omitempty"`
	Strict          bool              `json:"strict,omitempty"`
}

type openAIResponsesResponse struct {
	ID                string                           `json:"id"`
	Status            string                           `json:"status"`
	Error             *openAIResponsesFailure          `json:"error,omitempty"`
	IncompleteDetails *openAIResponsesIncompleteReason `json:"incomplete_details,omitempty"`
	Output            []openAIResponsesOutputItem      `json:"output,omitempty"`
}

type openAIResponsesFailure struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type openAIResponsesIncompleteReason struct {
	Reason string `json:"reason,omitempty"`
}

type openAIResponsesOutputItem struct {
	Type        string                         `json:"type"`
	Role        string                         `json:"role,omitempty"`
	Name        string                         `json:"name,omitempty"`
	CallID      string                         `json:"call_id,omitempty"`
	Arguments   string                         `json:"arguments,omitempty"`
	Status      string                         `json:"status,omitempty"`
	ServerLabel string                         `json:"server_label,omitempty"`
	Error       string                         `json:"error,omitempty"`
	Output      string                         `json:"output,omitempty"`
	Content     []openAIResponsesOutputContent `json:"content,omitempty"`
}

type openAIResponsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func (r *openAIResponsesResponse) outputText() string {
	var parts []string
	for _, item := range r.Output {
		if item.Type != "message" || item.Role != "assistant" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				parts = append(parts, strings.TrimSpace(content.Text))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (r *openAIResponsesResponse) toolLog() string {
	var parts []string
	for _, item := range r.Output {
		if item.Type != "mcp_call" {
			continue
		}
		name := item.Name
		if name == "" {
			name = "tool"
		}
		switch {
		case item.Error != "":
			parts = append(parts, fmt.Sprintf("%s failed: %s", name, item.Error))
		case strings.TrimSpace(item.Output) != "":
			parts = append(parts, fmt.Sprintf("%s: %s", name, strings.TrimSpace(item.Output)))
		default:
			parts = append(parts, fmt.Sprintf("%s status=%s", name, item.Status))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func (r *openAIResponsesResponse) functionCalls() []openAIResponsesOutputItem {
	var calls []openAIResponsesOutputItem
	for _, item := range r.Output {
		if item.Type == "function_call" && item.CallID != "" && item.Name != "" {
			calls = append(calls, item)
		}
	}
	return calls
}

func (r *openAIResponsesResponse) failureReason() string {
	switch {
	case r.Error != nil && r.Error.Message != "":
		return r.Error.Message
	case r.IncompleteDetails != nil && r.IncompleteDetails.Reason != "":
		return "incomplete: " + r.IncompleteDetails.Reason
	case r.Status != "":
		return "status=" + r.Status
	default:
		return ""
	}
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
