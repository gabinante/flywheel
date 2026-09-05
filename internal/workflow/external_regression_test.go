package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExternalSyncAndPollResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/failed":
			w.WriteHeader(503)
		case "/pending":
			w.Write([]byte(`{"deployment":{"ready":false}}`))
		default:
			w.Write([]byte(`{"deployment":{"ready":true}}`))
		}
	}))
	defer server.Close()
	executor := NewExternalExecutor(nil, "")
	ctx := context.Background()
	for _, tc := range []struct{ path, outcome string }{{"/ready", "success"}, {"/failed", "failed"}} {
		outcome, _, err := executor.ExecuteSync(ctx, &ExternalPhaseConfig{URL: server.URL + tc.path, Method: "POST"}, "t", "wf", "phase")
		if err != nil || outcome != tc.outcome {
			t.Fatalf("%s: %s %v", tc.path, outcome, err)
		}
	}
	for _, tc := range []struct {
		path  string
		ready bool
	}{{"/ready", true}, {"/pending", false}, {"/failed", false}} {
		ready, err := executor.PollOnce(ctx, &ExternalPhaseConfig{PollURL: server.URL + tc.path, PollSuccessCondition: "deployment.ready"})
		if err != nil || ready != tc.ready {
			t.Fatalf("%s: %v %v", tc.path, ready, err)
		}
	}
}

func TestUnsupportedWorkflowContractsRejected(t *testing.T) {
	for _, tc := range []struct {
		kind   PhaseType
		config map[string]any
	}{
		{PhaseExternal, map[string]any{"mode": "poll"}},
		{PhaseExternal, map[string]any{"mode": "sync"}},
		{PhaseAction, map[string]any{"action": "unregistered"}},
	} {
		if len(ValidatePhaseConfig(tc.kind, tc.config)) == 0 {
			t.Fatalf("accepted unsupported config: %v", tc.config)
		}
	}
}
