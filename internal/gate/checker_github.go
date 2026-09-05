package gate

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

type GitHubChecksChecker struct{}

func (c *GitHubChecksChecker) Check(ctx context.Context, req GateRequirement, rctx CheckContext) GateRequirementStatus {
	status, reason := ChecksUnknown, "no PR URL available"
	if rctx.PRURL != "" {
		status, reason = ParseGHPRChecksContext(ctx, rctx.PRURL)
	}
	return GateRequirementStatus{Requirement: req, Satisfied: status == ChecksPassed, Failed: status == ChecksFailed, Reason: reason, CheckedAt: time.Now().UTC()}
}

type ChecksStatus int

const (
	ChecksPassed ChecksStatus = iota
	ChecksFailed
	ChecksPending
	ChecksUnknown
)

func ParseGHPRChecks(prURL string) (ChecksStatus, string) {
	return ParseGHPRChecksContext(context.Background(), prURL)
}
func ParseGHPRChecksContext(ctx context.Context, prURL string) (ChecksStatus, string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "pr", "checks", prURL, "--json", "name,bucket").Output()
	var checks []struct {
		Name   string `json:"name"`
		Bucket string `json:"bucket"`
	}
	if json.Unmarshal(out, &checks) != nil || len(checks) == 0 {
		return ChecksUnknown, "CI checks unavailable or no checks configured; operator approval required"
	}
	pending := false
	for _, check := range checks {
		switch check.Bucket {
		case "fail", "cancel":
			return ChecksFailed, "CI check failed: " + check.Name
		case "pending":
			pending = true
		case "pass", "skipping":
		default:
			return ChecksUnknown, "unrecognized CI status: " + check.Bucket
		}
	}
	if pending {
		return ChecksPending, "CI checks still pending"
	}
	if err != nil {
		return ChecksUnknown, "unable to confirm CI checks"
	}
	return ChecksPassed, "all CI checks passed"
}
func truncateOutput(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
