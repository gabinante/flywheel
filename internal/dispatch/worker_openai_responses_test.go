package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenAIResponsesWorkerSpawn(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	filePath := filepath.Join(workDir, "README.md")
	if err := os.WriteFile(filePath, []byte("workspace content"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	var requests []openAIResponsesRequest
	requestCount := 0
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			switch req.URL.Path {
			case "/v1/responses":
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST /responses, got %s", req.Method)
				}
				if got := req.Header.Get("Authorization"); got != "Bearer sk-openai" {
					t.Fatalf("expected bearer auth, got %q", got)
				}
				var decoded openAIResponsesRequest
				if err := json.NewDecoder(req.Body).Decode(&decoded); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				requests = append(requests, decoded)
				if decoded.PreviousResponseID == "" {
					return jsonHTTPResponse(http.StatusOK, openAIResponsesResponse{
						ID:     "resp_123",
						Status: "in_progress",
					}), nil
				}
				if decoded.PreviousResponseID != "resp_123" {
					t.Fatalf("expected previous_response_id resp_123, got %q", decoded.PreviousResponseID)
				}
				return jsonHTTPResponse(http.StatusOK, openAIResponsesResponse{
					ID:     "resp_456",
					Status: "in_progress",
				}), nil
			case "/v1/responses/resp_123":
				return jsonHTTPResponse(http.StatusOK, openAIResponsesResponse{
					ID:     "resp_123",
					Status: "completed",
					Output: []openAIResponsesOutputItem{
						{
							Type:      "function_call",
							Name:      "workspace_read_file",
							CallID:    "call_readme",
							Arguments: `{"path":"README.md"}`,
						},
					},
				}), nil
			case "/v1/responses/resp_456":
				return jsonHTTPResponse(http.StatusOK, openAIResponsesResponse{
					ID:     "resp_456",
					Status: "completed",
					Output: []openAIResponsesOutputItem{
						{
							Type:   "message",
							Role:   "assistant",
							Status: "completed",
							Content: []openAIResponsesOutputContent{
								{Type: "output_text", Text: "done"},
							},
						},
					},
				}), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	worker := &OpenAIResponsesWorker{
		Config: OpenAIResponsesConfig{
			APIBaseURL:      "https://api.openai.test/v1",
			APIKey:          "sk-openai",
			Model:           "gpt-5.2-codex",
			ReasoningEffort: "medium",
			PollInterval:    time.Millisecond,
			HTTPClient:      client,
		},
		APIKey: "wf-test",
	}

	result, err := worker.Spawn(context.Background(), "ticket-1", "project-1", "system prompt", "task prompt", workDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.Output != "done" {
		t.Fatalf("expected output 'done', got %q", result.Output)
	}
	if requestCount != 4 {
		t.Fatalf("expected create + poll + followup create + poll, got %d requests", requestCount)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 create requests, got %d", len(requests))
	}
	createReq := requests[0]
	followupReq := requests[1]
	if createReq.Model != "gpt-5.2-codex" {
		t.Fatalf("expected model gpt-5.2-codex, got %q", createReq.Model)
	}
	if createReq.Background != true {
		t.Fatal("expected background mode enabled")
	}
	if createReq.Tools[0].Type != "mcp" {
		t.Fatalf("expected MCP tool, got %+v", createReq.Tools)
	}
	if createReq.Tools[0].ServerURL != "http://localhost:8080/mcp" {
		t.Fatalf("expected MCP URL http://localhost:8080/mcp, got %q", createReq.Tools[0].ServerURL)
	}
	if createReq.Tools[0].Headers["X-API-Key"] != "wf-test" {
		t.Fatalf("expected Flywheel API key header, got %+v", createReq.Tools[0].Headers)
	}
	if createReq.Tools[0].RequireApproval != "never" {
		t.Fatalf("expected require_approval=never, got %q", createReq.Tools[0].RequireApproval)
	}
	if !hasTool(createReq.Tools, "workspace_read_file") {
		t.Fatalf("expected workspace_read_file tool, got %+v", createReq.Tools)
	}
	if createReq.Reasoning == nil || createReq.Reasoning.Effort != "medium" {
		t.Fatalf("expected reasoning effort medium, got %+v", createReq.Reasoning)
	}
	inputItems, ok := followupReq.Input.([]any)
	if !ok || len(inputItems) != 1 {
		t.Fatalf("expected one function_call_output input item, got %#v", followupReq.Input)
	}
	outputItem, ok := inputItems[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map input item, got %#v", inputItems[0])
	}
	if outputItem["type"] != "function_call_output" {
		t.Fatalf("expected function_call_output, got %#v", outputItem)
	}
	if outputItem["call_id"] != "call_readme" {
		t.Fatalf("expected call_readme output, got %#v", outputItem)
	}
	outputText, _ := outputItem["output"].(string)
	if !strings.Contains(outputText, "workspace content") {
		t.Fatalf("expected file contents in tool output, got %q", outputText)
	}
}

func TestOpenAIResponsesWorkerSpawnIncomplete(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/responses" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			return jsonHTTPResponse(http.StatusOK, openAIResponsesResponse{
				ID:     "resp_bad",
				Status: "incomplete",
				IncompleteDetails: &openAIResponsesIncompleteReason{
					Reason: "max_output_tokens",
				},
			}), nil
		}),
	}

	worker := &OpenAIResponsesWorker{
		Config: OpenAIResponsesConfig{
			APIBaseURL: "https://api.openai.test/v1",
			APIKey:     "sk-openai",
			HTTPClient: client,
		},
	}

	result, err := worker.Spawn(context.Background(), "ticket-2", "project-2", "system", "task", t.TempDir(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure result, got %+v", result)
	}
	if result.Error != "incomplete: max_output_tokens" {
		t.Fatalf("unexpected error: %q", result.Error)
	}
}

func TestOpenAIResponsesWorkerSpawnAPIError(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return rawHTTPResponse(http.StatusUnauthorized, `{"error":{"message":"bad api key"}}`), nil
		}),
	}

	worker := &OpenAIResponsesWorker{
		Config: OpenAIResponsesConfig{
			APIBaseURL: "https://api.openai.test/v1",
			APIKey:     "sk-openai",
			HTTPClient: client,
		},
	}

	_, err := worker.Spawn(context.Background(), "ticket-3", "project-3", "system", "task", t.TempDir(), "http://localhost:8080")
	if err == nil || !strings.Contains(err.Error(), "bad api key") {
		t.Fatalf("expected API error containing bad api key, got %v", err)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonHTTPResponse(status int, payload any) *http.Response {
	b, _ := json.Marshal(payload)
	return rawHTTPResponse(status, string(b))
}

func rawHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func hasTool(tools []openAIResponsesTool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}
