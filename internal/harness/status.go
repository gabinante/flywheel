package harness

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// Status describes whether a harness CLI is installed and signed in.
type Status struct {
	Harness      string `json:"harness"` // claude | codex
	Bin          string `json:"bin"`
	ResolvedPath string `json:"resolved_path,omitempty"`
	Installed    bool   `json:"installed"`
	Version      string `json:"version,omitempty"`
	LoggedIn     bool   `json:"logged_in"`
	Account      string `json:"account,omitempty"`
	Detail       string `json:"detail,omitempty"`
	LoginCommand string `json:"login_command"`
	CheckedAt    string `json:"checked_at"`
}

// Statuses probes both harnesses. Each probe is bounded so a hung CLI cannot stall the UI.
func (r *CLIRunner) Statuses(ctx context.Context) []Status {
	cfg := r.conf()
	return []Status{probeClaude(ctx, cfg.ClaudeBin), probeCodex(ctx, cfg.CodexBin)}
}

func probe(ctx context.Context, bin string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func probeClaude(ctx context.Context, bin string) Status {
	st := Status{Harness: "claude", Bin: bin, LoginCommand: bin + " login", CheckedAt: time.Now().Format(time.RFC3339)}
	path, err := exec.LookPath(bin)
	if err != nil {
		st.Detail = "executable not found on PATH"
		return st
	}
	st.Installed, st.ResolvedPath = true, path
	if v, err := probe(ctx, bin, "--version"); err == nil {
		st.Version = firstLine(v)
	}
	out, err := probe(ctx, bin, "auth", "status")
	if err != nil && out == "" {
		st.Detail = "could not query auth status: " + err.Error()
		return st
	}
	var v struct {
		LoggedIn   bool   `json:"loggedIn"`
		AuthMethod string `json:"authMethod"`
		Email      string `json:"email"`
		Org        string `json:"orgName"`
	}
	if i := strings.Index(out, "{"); i >= 0 && json.Unmarshal([]byte(out[i:]), &v) == nil {
		st.LoggedIn = v.LoggedIn
		st.Account = strings.TrimSpace(strings.Join(nonEmpty(v.Email, v.Org), " · "))
		if v.AuthMethod != "" {
			st.Detail = "via " + v.AuthMethod
		}
		if !st.LoggedIn {
			st.Detail = "not signed in"
		}
		return st
	}
	st.LoggedIn = strings.Contains(strings.ToLower(out), "logged in") && !strings.Contains(strings.ToLower(out), "not logged in")
	st.Detail = firstLine(out)
	return st
}

func probeCodex(ctx context.Context, bin string) Status {
	st := Status{Harness: "codex", Bin: bin, LoginCommand: bin + " login", CheckedAt: time.Now().Format(time.RFC3339)}
	path, err := exec.LookPath(bin)
	if err != nil {
		st.Detail = "executable not found on PATH"
		return st
	}
	st.Installed, st.ResolvedPath = true, path
	if v, err := probe(ctx, bin, "--version"); err == nil {
		st.Version = firstLine(v)
	}
	out, _ := probe(ctx, bin, "login", "status")
	low := strings.ToLower(out)
	st.LoggedIn = strings.Contains(low, "logged in") && !strings.Contains(low, "not logged in")
	st.Detail = firstLine(out)
	if strings.Contains(low, "chatgpt") {
		st.Account = "ChatGPT account"
	} else if strings.Contains(low, "api key") {
		st.Account = "API key"
	}
	return st
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, strings.TrimSpace(v))
		}
	}
	return out
}
