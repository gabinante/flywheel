package dispatch

import (
	"strings"
	"testing"
)

func TestCodexDriverBuildCLIArgs(t *testing.T) {
	d := NewCodexDriver(DriverConfig{Model: "gpt-5.6-sol", ReasoningEffort: "xhigh"})
	conn := mcpConnection{Name: "flywheel", HTTPURL: "http://localhost:8090/mcp", Headers: map[string]string{"X-API-Key": "k1"}}
	args := d.BuildCLIArgs("You are a worker.", "Implement ticket X.", conn, "/tmp/unused.json")
	joined := strings.Join(args, " ")
	for _, want := range []string{"exec --json", "-s workspace-write", "approval_policy=never", "mcp_servers={}",
		`mcp_servers.flywheel.url="http://localhost:8090/mcp"`, `mcp_servers.flywheel.http_headers={ "X-API-Key" = "k1" }`,
		"-m gpt-5.6-sol", `model_reasoning_effort="xhigh"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q:\n%s", want, joined)
		}
	}
	last := args[len(args)-1]
	if !strings.HasPrefix(last, "You are a worker.") || !strings.Contains(last, "# Task") || !strings.HasSuffix(last, "Implement ticket X.") {
		t.Errorf("prompt preamble wrong: %q", last)
	}
	if d.Executable() != "codex" || d.Name() != "codex" {
		t.Errorf("identity wrong")
	}
	if _, err := LookupDriver("codex", DriverConfig{}); err != nil {
		t.Errorf("codex driver not registered: %v", err)
	}
}
