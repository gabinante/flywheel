package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// mockWorker implements Worker for testing.
type mockWorker struct {
	spawnFn func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error)
}

func (m *mockWorker) Spawn(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
	if m.spawnFn != nil {
		return m.spawnFn(ctx, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL)
	}
	return &WorkerResult{Success: true, Output: ""}, nil
}

func TestDispatch_ValidRequest(t *testing.T) {
	findings := map[string]any{
		"status": "complete",
		"claims": []map[string]any{
			{
				"statement":  "The Worker interface has a Spawn method",
				"confidence": "high",
				"citations": []map[string]any{
					{"file": "internal/dispatch/worker.go", "line": 47, "snippet": "Spawn(ctx context.Context, ...)"},
				},
			},
		},
		"negative_space": []string{"No existing investigation package found"},
		"open_questions": []string{},
	}
	findingsJSON, _ := json.Marshal(findings)
	output := fmt.Sprintf("Working...\n\n```json\n%s\n```\n", string(findingsJSON))

	worker := &mockWorker{
		spawnFn: func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
			// Verify one-level-deep: system prompt should NOT mention dispatch_investigation
			if contains(systemPrompt, "dispatch_investigation") {
				t.Error("system prompt should not expose dispatch_investigation (one-level-deep constraint)")
			}
			// Verify read-only constraint
			if !contains(systemPrompt, "Read-only") {
				t.Error("system prompt should enforce read-only constraint")
			}
			// Verify token budget is mentioned
			if !contains(systemPrompt, "4000") {
				t.Error("system prompt should mention default token budget")
			}
			return &WorkerResult{Success: true, Output: output}, nil
		},
	}

	svc := NewService(worker, Config{
		ServerURL:      "http://localhost:8080",
		DefaultTimeout: 30 * time.Second,
		WorkDir:        "/tmp/test",
	})

	resp, err := svc.Dispatch(context.Background(), &Request{
		ProjectID:      "test-project",
		Question:       "What is the Worker interface?",
		ParentTicketID: "warrant-99",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusComplete {
		t.Errorf("expected status complete, got %s", resp.Status)
	}
	if len(resp.Claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(resp.Claims))
	}
	if resp.Claims[0].Statement != "The Worker interface has a Spawn method" {
		t.Errorf("unexpected claim statement: %s", resp.Claims[0].Statement)
	}
	if len(resp.Claims[0].Citations) != 1 {
		t.Fatalf("expected 1 citation, got %d", len(resp.Claims[0].Citations))
	}
	if resp.Claims[0].Citations[0].File != "internal/dispatch/worker.go" {
		t.Errorf("unexpected citation file: %s", resp.Claims[0].Citations[0].File)
	}
	if resp.Claims[0].Citations[0].Line != 47 {
		t.Errorf("unexpected citation line: %d", resp.Claims[0].Citations[0].Line)
	}
	if len(resp.NegativeSpace) != 1 {
		t.Errorf("expected 1 negative space entry, got %d", len(resp.NegativeSpace))
	}
}

func TestDispatch_NilRequest(t *testing.T) {
	svc := NewService(&mockWorker{}, Config{WorkDir: "/tmp"})
	_, err := svc.Dispatch(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
}

func TestDispatch_EmptyQuestion(t *testing.T) {
	svc := NewService(&mockWorker{}, Config{WorkDir: "/tmp"})
	_, err := svc.Dispatch(context.Background(), &Request{ProjectID: "p"})
	if err == nil {
		t.Error("expected error for empty question")
	}
}

func TestDispatch_EmptyProjectID(t *testing.T) {
	svc := NewService(&mockWorker{}, Config{WorkDir: "/tmp"})
	_, err := svc.Dispatch(context.Background(), &Request{Question: "q"})
	if err == nil {
		t.Error("expected error for empty project_id")
	}
}

func TestDispatch_WorkerFailure(t *testing.T) {
	worker := &mockWorker{
		spawnFn: func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
			return &WorkerResult{Success: false, Error: "agent crashed"}, nil
		},
	}

	svc := NewService(worker, Config{WorkDir: "/tmp"})
	resp, err := svc.Dispatch(context.Background(), &Request{
		ProjectID: "p",
		Question:  "test question",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusFailed {
		t.Errorf("expected status failed, got %s", resp.Status)
	}
	if resp.Error != "agent crashed" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func TestDispatch_SpawnError(t *testing.T) {
	worker := &mockWorker{
		spawnFn: func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
			return nil, fmt.Errorf("spawn failed: binary not found")
		},
	}

	svc := NewService(worker, Config{WorkDir: "/tmp"})
	resp, err := svc.Dispatch(context.Background(), &Request{
		ProjectID: "p",
		Question:  "test question",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusFailed {
		t.Errorf("expected status failed, got %s", resp.Status)
	}
}

func TestDispatch_Timeout(t *testing.T) {
	worker := &mockWorker{
		spawnFn: func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
			// Simulate timeout by waiting for context cancellation.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	svc := NewService(worker, Config{
		WorkDir:        "/tmp",
		DefaultTimeout: 50 * time.Millisecond,
	})

	resp, err := svc.Dispatch(context.Background(), &Request{
		ProjectID: "p",
		Question:  "slow question",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != StatusTimeout {
		t.Errorf("expected status timeout, got %s", resp.Status)
	}
}

func TestDispatch_TokenBudgetDefaults(t *testing.T) {
	var capturedPrompt string
	worker := &mockWorker{
		spawnFn: func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
			capturedPrompt = systemPrompt
			return &WorkerResult{Success: true, Output: `{"status":"complete","claims":[]}`}, nil
		},
	}

	svc := NewService(worker, Config{WorkDir: "/tmp"})

	// Default budget
	_, _ = svc.Dispatch(context.Background(), &Request{
		ProjectID: "p",
		Question:  "q",
	})
	if !contains(capturedPrompt, "4000") {
		t.Error("expected default token budget of 4000 in prompt")
	}

	// Custom budget
	_, _ = svc.Dispatch(context.Background(), &Request{
		ProjectID:   "p",
		Question:    "q",
		TokenBudget: 8000,
	})
	if !contains(capturedPrompt, "8000") {
		t.Error("expected custom token budget of 8000 in prompt")
	}

	// Budget capped at max
	_, _ = svc.Dispatch(context.Background(), &Request{
		ProjectID:   "p",
		Question:    "q",
		TokenBudget: 50000,
	})
	if !contains(capturedPrompt, "16000") {
		t.Error("expected max token budget of 16000 in prompt")
	}
}

func TestDispatch_ScopeInPrompt(t *testing.T) {
	var capturedPrompt string
	worker := &mockWorker{
		spawnFn: func(ctx context.Context, ticketID, projectID, systemPrompt, taskMessage, workDir, serverURL string) (*WorkerResult, error) {
			capturedPrompt = systemPrompt
			return &WorkerResult{Success: true, Output: `{"status":"complete","claims":[]}`}, nil
		},
	}

	svc := NewService(worker, Config{WorkDir: "/tmp"})
	_, _ = svc.Dispatch(context.Background(), &Request{
		ProjectID: "p",
		Question:  "q",
		Scope: Scope{
			Files:    []string{"internal/dispatch/*.go"},
			Symbols:  []string{"Dispatcher", "Worker"},
			Packages: []string{"internal/dispatch"},
		},
	})

	if !contains(capturedPrompt, "internal/dispatch/*.go") {
		t.Error("expected file scope in prompt")
	}
	if !contains(capturedPrompt, "Dispatcher") {
		t.Error("expected symbol scope in prompt")
	}
	if !contains(capturedPrompt, "internal/dispatch") {
		t.Error("expected package scope in prompt")
	}
}

func TestExtractJSONBlock_Fenced(t *testing.T) {
	input := "Some working notes...\n\n```json\n{\"status\":\"complete\",\"claims\":[]}\n```\n\nDone."
	got := extractJSONBlock(input)
	if got != `{"status":"complete","claims":[]}` {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestExtractJSONBlock_Raw(t *testing.T) {
	input := `Some text before {"status":"complete","claims":[]} after`
	got := extractJSONBlock(input)
	if got != `{"status":"complete","claims":[]}` {
		t.Errorf("unexpected result: %q", got)
	}
}

func TestExtractJSONBlock_Nested(t *testing.T) {
	input := `{"status":"complete","claims":[{"statement":"foo","citations":[{"file":"a.go","line":1}]}]}`
	got := extractJSONBlock(input)
	if got == "" {
		t.Error("expected to find nested JSON")
	}
	// Verify it's valid JSON.
	var js json.RawMessage
	if err := json.Unmarshal([]byte(got), &js); err != nil {
		t.Errorf("extracted JSON is invalid: %v", err)
	}
}

func TestExtractJSONBlock_Empty(t *testing.T) {
	got := extractJSONBlock("no json here at all")
	if got != "" {
		t.Errorf("expected empty result, got: %q", got)
	}
}

func TestEstimateTokens(t *testing.T) {
	// 100 chars → ~25 tokens
	input := "aaaaaaaaaa" // 10 chars
	got := estimateTokens(input + input + input + input + input +
		input + input + input + input + input) // 100 chars
	if got != 25 {
		t.Errorf("expected 25 tokens for 100 chars, got %d", got)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && fmt.Sprintf("%s", s) != "" && // avoid import "strings" conflict
		len(s) >= len(substr) && searchContains(s, substr)
}

func searchContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
