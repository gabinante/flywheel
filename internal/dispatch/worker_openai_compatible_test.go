package dispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOpenAICompatibleWorkerSpawn(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	filePath := filepath.Join(workDir, "README.md")
	if err := os.WriteFile(filePath, []byte("workspace content"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	bridge := &fakeOpenAIToolBridge{
		tools: []*mcp.Tool{
			{
				Name:        "flywheel_echo",
				Description: "Echo a message back to the caller.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"message": map[string]any{"type": "string"},
					},
					"required":             []string{"message"},
					"additionalProperties": false,
				},
			},
		},
		callFn: func(_ context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "echo: " + arguments["message"].(string)},
				},
			}, nil
		},
	}

	requestCount := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected chat completion path: %s", req.URL.Path)
			}
			if got := req.Header.Get("Authorization"); got != "Bearer sk-compatible" {
				t.Fatalf("expected bearer auth, got %q", got)
			}
			requestCount++

			var decoded openAIChatCompletionRequest
			if err := json.NewDecoder(req.Body).Decode(&decoded); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			switch requestCount {
			case 1:
				if decoded.Model != "qwen2.5-coder" {
					t.Fatalf("expected model qwen2.5-coder, got %q", decoded.Model)
				}
				if !hasChatTool(decoded.Tools, "workspace_read_file") {
					t.Fatalf("expected workspace_read_file tool, got %#v", decoded.Tools)
				}
				if !hasChatTool(decoded.Tools, "flywheel_echo") {
					t.Fatalf("expected flywheel_echo tool, got %#v", decoded.Tools)
				}
				if len(decoded.Messages) != 2 {
					t.Fatalf("expected system and user messages, got %d", len(decoded.Messages))
				}
				return jsonHTTPResponse(http.StatusOK, openAIChatCompletionResponse{
					Choices: []openAIChatChoice{
						{
							FinishReason: "tool_calls",
							Message: openAIChatMessage{
								Role: "assistant",
								ToolCalls: []openAIChatToolCall{
									{
										ID:   "call_workspace",
										Type: "function",
										Function: openAIChatToolCallFunction{
											Name:      "workspace_read_file",
											Arguments: `{"path":"README.md"}`,
										},
									},
									{
										ID:   "call_echo",
										Type: "function",
										Function: openAIChatToolCallFunction{
											Name:      "flywheel_echo",
											Arguments: `{"message":"hello from mcp"}`,
										},
									},
								},
							},
						},
					},
				}), nil
			case 2:
				if len(decoded.Messages) != 5 {
					t.Fatalf("expected system, user, assistant, and two tool messages; got %d", len(decoded.Messages))
				}
				toolMessage1 := decoded.Messages[3]
				toolMessage2 := decoded.Messages[4]
				if toolMessage1.Role != "tool" || toolMessage1.ToolCallID != "call_workspace" {
					t.Fatalf("unexpected first tool message: %#v", toolMessage1)
				}
				if toolMessage2.Role != "tool" || toolMessage2.ToolCallID != "call_echo" {
					t.Fatalf("unexpected second tool message: %#v", toolMessage2)
				}
				if content, _ := toolMessage1.Content.(string); !strings.Contains(content, "workspace content") {
					t.Fatalf("expected workspace file contents in first tool message, got %#v", toolMessage1.Content)
				}
				if content, _ := toolMessage2.Content.(string); !strings.Contains(content, "hello from mcp") {
					t.Fatalf("expected MCP tool output in second tool message, got %#v", toolMessage2.Content)
				}
				return jsonHTTPResponse(http.StatusOK, openAIChatCompletionResponse{
					Choices: []openAIChatChoice{
						{
							FinishReason: "stop",
							Message: openAIChatMessage{
								Role:    "assistant",
								Content: "done",
							},
						},
					},
				}), nil
			default:
				t.Fatalf("unexpected request count %d", requestCount)
				return nil, nil
			}
		}),
	}

	worker := &OpenAICompatibleWorker{
		Config: OpenAICompatibleConfig{
			APIBaseURL:    "https://compat.example.test/v1",
			APIKey:        "sk-compatible",
			Model:         "qwen2.5-coder",
			MaxToolRounds: 8,
			HTTPClient:    client,
			MCPBridgeFactory: func(ctx context.Context, conn mcpConnection) (openAIToolBridge, error) {
				if conn.HTTPURL != "http://flywheel.test/mcp" {
					t.Fatalf("expected MCP URL http://flywheel.test/mcp, got %q", conn.HTTPURL)
				}
				if conn.Headers["X-API-Key"] != "wf-test" {
					t.Fatalf("expected Flywheel API key header, got %#v", conn.Headers)
				}
				return bridge, nil
			},
		},
		APIKey: "wf-test",
	}

	result, err := worker.Spawn(context.Background(), "ticket-1", "project-1", "system prompt", "task prompt", workDir, "http://flywheel.test")
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.Output != "done" {
		t.Fatalf("expected output 'done', got %q", result.Output)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 chat completion requests, got %d", requestCount)
	}
	if len(bridge.calls) != 1 || bridge.calls[0].name != "flywheel_echo" {
		t.Fatalf("expected one MCP tool call to flywheel_echo, got %#v", bridge.calls)
	}
	if bridge.closed != 1 {
		t.Fatalf("expected bridge to close once, got %d", bridge.closed)
	}
}

func TestOpenAICompatibleWorkerRequiresModel(t *testing.T) {
	t.Parallel()

	worker := &OpenAICompatibleWorker{
		Config: OpenAICompatibleConfig{
			APIKey: "sk-compatible",
		},
	}

	result, err := worker.Spawn(context.Background(), "ticket-1", "project-1", "system", "task", t.TempDir(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure result, got %+v", result)
	}
	if result.Error != "missing model for openai-compatible runner" {
		t.Fatalf("unexpected error: %q", result.Error)
	}
}

func hasChatTool(tools []openAIChatTool, name string) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

type fakeOpenAIToolBridge struct {
	tools  []*mcp.Tool
	callFn func(context.Context, string, map[string]any) (*mcp.CallToolResult, error)
	calls  []fakeOpenAIToolCall
	closed int
}

type fakeOpenAIToolCall struct {
	name      string
	arguments map[string]any
}

func (b *fakeOpenAIToolBridge) ListTools(context.Context) ([]*mcp.Tool, error) {
	return b.tools, nil
}

func (b *fakeOpenAIToolBridge) CallTool(ctx context.Context, name string, arguments map[string]any) (*mcp.CallToolResult, error) {
	b.calls = append(b.calls, fakeOpenAIToolCall{name: name, arguments: arguments})
	if b.callFn != nil {
		return b.callFn(ctx, name, arguments)
	}
	return &mcp.CallToolResult{}, nil
}

func (b *fakeOpenAIToolBridge) Close() error {
	b.closed++
	return nil
}
