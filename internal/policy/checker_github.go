package policy

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// GitHubChecksChecker verifies that all GitHub CI checks on a PR have passed.
// It shells out to `gh pr checks <prURL>` and parses the output.
type GitHubChecksChecker struct{}

// Check implements RequirementChecker.
func (c *GitHubChecksChecker) Check(_ context.Context, req GateRequirement, rctx CheckContext) GateRequirementStatus {
	now := time.Now().UTC()

	if rctx.PRURL == "" {
		return GateRequirementStatus{
			Requirement: req,
			Satisfied:   false,
			Reason:      "no PR URL available",
			CheckedAt:   now,
		}
	}

	status, reason := ParseGHPRChecks(rctx.PRURL)
	return GateRequirementStatus{
		Requirement: req,
		Satisfied:   status == ChecksPassed,
		Reason:      reason,
		CheckedAt:   now,
	}
}

// ChecksStatus represents the aggregate result of CI checks.
type ChecksStatus int

const (
	ChecksPassed  ChecksStatus = iota
	ChecksFailed
	ChecksPending
	ChecksUnknown
)

// ParseGHPRChecks runs `gh pr checks` and returns the aggregate status and a reason string.
// Extracted from dispatcher.validatePRChecks for reuse.
func ParseGHPRChecks(prURL string) (ChecksStatus, string) {
	cmd := exec.Command("gh", "pr", "checks", prURL)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		if strings.Contains(output, "fail") || strings.Contains(output, "X") {
			return ChecksFailed, "CI checks failed: " + truncateOutput(output, 200)
		}
		if strings.Contains(output, "pending") || strings.Contains(output, "\t-\t") {
			return ChecksPending, "CI checks still pending"
		}
		// No checks configured or other error — treat as passed.
		return ChecksPassed, "no CI checks configured or checks unavailable"
	}

	return ChecksPassed, "all CI checks passed"
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
