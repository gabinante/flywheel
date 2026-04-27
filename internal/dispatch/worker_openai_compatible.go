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

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OpenAICompatibleConfig configures the chat-completions based runner used for
// OpenAI-compatible APIs such as LiteLLM or vLLM.
type OpenAICompatibleConfig struct {
	APIBaseURL       string
	APIKey           string
	Model            string
	ReasoningEffort  string
	MaxToolRounds    int
	HTTPClient       *http.Client
	MCPBridgeFactory func(context.Context, mcpConnection) (openAIToolBridge, error)
}

// OpenAICompatibleWorker uses the standard OpenAI chat completions API with a
// local tool bridge for workspace functions and Flywheel MCP tools.
type OpenAICompatibleWorker struct {
	Config OpenAICompatibleConfig
	APIKey string // Flywheel API key for MCP authentication
}

func (w *OpenAICompatibleWorker) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
	return w.SpawnStream(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL, nil)
}

func (w *OpenAICompatibleWorker) SpawnStream(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string, onOutput WorkerOutputHandler) (*WorkerResult, error) {
	cfg := w.Config
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.openai.com/v1"
	}
	if cfg.MaxToolRounds <= 0 {
		cfg.MaxToolRounds = 32
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return &WorkerResult{
			Success: false,
			Error:   "missing model for openai-compatible runner",
		}, nil
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return &WorkerResult{
			Success: false,
			Error:   "missing API key for openai-compatible runner",
		}, nil
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	mcpConn := buildMCPConnection(serverURL, w.APIKey)
	bridgeFactory := cfg.MCPBridgeFactory
	if bridgeFactory == nil {
		bridgeFactory = newOpenAIMCPToolBridge
	}
	bridge, err := bridgeFactory(ctx, mcpConn)
	if err != nil {
		return nil, fmt.Errorf("connect flywheel mcp: %w", err)
	}
	defer bridge.Close()

	mcpTools, err := bridge.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("list flywheel mcp tools: %w", err)
	}

	tools := openAIChatFunctionTools(openAIWorkspaceToolDefinitions())
	tools = append(tools, openAIChatMCPTools(mcpTools)...)

	messages := []openAIChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: taskMessage},
	}
	toolLog := make([]string, 0, 8)

	for round := 0; round < cfg.MaxToolRounds; round++ {
		resp, err := w.createCompletion(ctx, client, cfg, openAIChatCompletionRequest{
			Model:      cfg.Model,
			Messages:   messages,
			Tools:      tools,
			ToolChoice: "auto",
		})
		if err != nil {
			return nil, err
		}
		choice := resp.firstChoice()
		if choice == nil {
			return &WorkerResult{
				Success: false,
				Error:   "chat completion returned no choices",
			}, nil
		}

		assistant := choice.Message
		if assistant.Role == "" {
			assistant.Role = "assistant"
		}
		messages = append(messages, assistant)

		if len(assistant.ToolCalls) == 0 {
			if strings.EqualFold(strings.TrimSpace(choice.FinishReason), "length") {
				return &WorkerResult{
					Success: false,
					Error:   "incomplete: length",
				}, nil
			}
			output := strings.TrimSpace(assistant.textContent())
			if output == "" {
				output = strings.TrimSpace(strings.Join(toolLog, "\n"))
			}
			return &WorkerResult{
				Success: true,
				Output:  output,
			}, nil
		}

		for _, toolCall := range assistant.ToolCalls {
			output := executeOpenAIChatToolCall(ctx, workDir, bridge, toolCall)
			summary := summarizeOpenAIToolCall(toolCall.Function.Name, output)
			toolLog = append(toolLog, summary)
			if onOutput != nil && strings.TrimSpace(summary) != "" {
				onOutput("stdout", summary)
			}
			messages = append(messages, openAIChatMessage{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    output,
			})
		}
	}

	return &WorkerResult{
		Success: false,
		Error:   "chat completion exceeded tool round limit",
	}, nil
}

type openAIToolBridge interface {
	ListTools(ctx context.Context) ([]*mcp.Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error)
	Close() error
}

type openAIMCPToolBridge struct {
	session *mcp.ClientSession
}

func newOpenAIMCPToolBridge(ctx context.Context, conn mcpConnection) (openAIToolBridge, error) {
	session, err := connectOpenAIMCP(ctx, conn)
	if err != nil {
		return nil, err
	}
	return &openAIMCPToolBridge{session: session}, nil
}

func (b *openAIMCPToolBridge) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	return listOpenAIMCPTools(ctx, b.session)
}

func (b *openAIMCPToolBridge) CallTool(ctx context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	return b.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
}

func (b *openAIMCPToolBridge) Close() error {
	return b.session.Close()
}

func (w *OpenAICompatibleWorker) createCompletion(ctx context.Context, client *http.Client, cfg OpenAICompatibleConfig, payload openAIChatCompletionRequest) (*openAIChatCompletionResponse, error) {
	return doOpenAIChatJSON(ctx, client, cfg, http.MethodPost, strings.TrimRight(cfg.APIBaseURL, "/")+"/chat/completions", payload)
}

func doOpenAIChatJSON(ctx context.Context, client *http.Client, cfg OpenAICompatibleConfig, method, url string, payload any) (*openAIChatCompletionResponse, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal openai-compatible request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build openai-compatible request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai-compatible response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr openAIErrorResponse
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("openai-compatible api: %s", apiErr.Error.Message)
		}
		return nil, fmt.Errorf("openai-compatible api: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed openAIChatCompletionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode openai-compatible response body: %w", err)
	}
	return &parsed, nil
}

func connectOpenAIMCP(ctx context.Context, conn mcpConnection) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "flywheel-dispatch",
		Version: "0.2.0",
	}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   conn.HTTPURL,
		HTTPClient: newOpenAIMCPHTTPClient(conn.Headers),
	}, nil)
}

func listOpenAIMCPTools(ctx context.Context, session *mcp.ClientSession) ([]*mcp.Tool, error) {
	var (
		cursor string
		tools  []*mcp.Tool
	)
	for {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		tools = append(tools, result.Tools...)
		cursor = strings.TrimSpace(result.NextCursor)
		if cursor == "" {
			return tools, nil
		}
	}
}

func openAIChatMCPTools(tools []*mcp.Tool) []openAIChatTool {
	out := make([]openAIChatTool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		parameters := tool.InputSchema
		if parameters == nil {
			parameters = map[string]any{"type": "object"}
		}
		out = append(out, openAIChatTool{
			Type: "function",
			Function: openAIChatToolFunction{
				Name:        strings.TrimSpace(tool.Name),
				Description: strings.TrimSpace(tool.Description),
				Parameters:  parameters,
			},
		})
	}
	return out
}

func executeOpenAIChatToolCall(ctx context.Context, workDir string, bridge openAIToolBridge, call openAIChatToolCall) string {
	name := strings.TrimSpace(call.Function.Name)
	if name == "" {
		return jsonToolError(fmt.Errorf("tool call is missing a function name"))
	}
	if isOpenAIWorkspaceTool(name) {
		return executeOpenAIFunctionTool(ctx, workDir, name, call.Function.Arguments)
	}
	return executeOpenAIMCPTool(ctx, bridge, name, call.Function.Arguments)
}

func executeOpenAIMCPTool(ctx context.Context, bridge openAIToolBridge, name, rawArguments string) string {
	args, err := parseOpenAIToolArguments(rawArguments)
	if err != nil {
		return jsonToolError(err)
	}

	result, err := bridge.CallTool(ctx, name, args)
	if err != nil {
		return jsonToolError(err)
	}

	payload := map[string]any{
		"ok": !result.IsError,
	}
	if len(result.Content) > 0 {
		content := make([]any, 0, len(result.Content))
		for _, item := range result.Content {
			content = append(content, openAIMCPContentValue(item))
		}
		payload["content"] = content
	}
	if result.StructuredContent != nil {
		payload["structured_content"] = result.StructuredContent
	}
	return workspaceMustJSON(payload)
}

func parseOpenAIToolArguments(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, err
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	obj, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tool arguments must be a JSON object")
	}
	return obj, nil
}

func openAIMCPContentValue(content mcp.Content) any {
	switch value := content.(type) {
	case *mcp.TextContent:
		return map[string]any{
			"type": "text",
			"text": value.Text,
		}
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return map[string]any{
				"type":  "unsupported",
				"error": err.Error(),
			}
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return map[string]any{
				"type": "unsupported",
				"raw":  string(raw),
			}
		}
		return decoded
	}
}

func summarizeOpenAIToolCall(name, output string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool"
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return name
	}
	return name + ": " + output
}

func newOpenAIMCPHTTPClient(headers map[string]string) *http.Client {
	return &http.Client{
		Transport: &openAIHeaderTransport{
			Base:    http.DefaultTransport,
			Headers: cloneStringMap(headers),
		},
	}
}

type openAIHeaderTransport struct {
	Base    http.RoundTripper
	Headers map[string]string
}

func (t *openAIHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	clone := req.Clone(req.Context())
	clone.Header = clone.Header.Clone()
	for key, value := range t.Headers {
		if strings.TrimSpace(value) != "" {
			clone.Header.Set(key, value)
		}
	}
	return base.RoundTrip(clone)
}

type openAIChatCompletionRequest struct {
	Model      string              `json:"model"`
	Messages   []openAIChatMessage `json:"messages"`
	Tools      []openAIChatTool    `json:"tools,omitempty"`
	ToolChoice any                 `json:"tool_choice,omitempty"`
}

type openAIChatCompletionResponse struct {
	ID      string                 `json:"id,omitempty"`
	Choices []openAIChatChoice     `json:"choices,omitempty"`
	Usage   map[string]any         `json:"usage,omitempty"`
	Error   map[string]any         `json:"error,omitempty"`
	Object  string                 `json:"object,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Extra   map[string]interface{} `json:"-"`
}

func (r *openAIChatCompletionResponse) firstChoice() *openAIChatChoice {
	if r == nil || len(r.Choices) == 0 {
		return nil
	}
	return &r.Choices[0]
}

type openAIChatChoice struct {
	Index        int               `json:"index,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
	Message      openAIChatMessage `json:"message"`
}

type openAIChatMessage struct {
	Role       string               `json:"role"`
	Content    any                  `json:"content,omitempty"`
	Name       string               `json:"name,omitempty"`
	ToolCalls  []openAIChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

func (m openAIChatMessage) textContent() string {
	return openAIChatContentText(m.Content)
}

type openAIChatTool struct {
	Type     string                 `json:"type"`
	Function openAIChatToolFunction `json:"function"`
}

type openAIChatToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type openAIChatToolCall struct {
	ID       string                     `json:"id,omitempty"`
	Type     string                     `json:"type,omitempty"`
	Function openAIChatToolCallFunction `json:"function"`
}

type openAIChatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func openAIChatContentText(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if text := openAIChatContentText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n\n"))
	case map[string]any:
		if text, _ := value["text"].(string); strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		if inner, ok := value["text"].(map[string]any); ok {
			if text, _ := inner["value"].(string); strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
		if outputText, _ := value["output_text"].(string); strings.TrimSpace(outputText) != "" {
			return strings.TrimSpace(outputText)
		}
		return ""
	default:
		return ""
	}
}
