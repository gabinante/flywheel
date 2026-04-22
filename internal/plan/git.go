package plan

import (
	"fmt"
	"strings"
)

// GitPolicy defines the git integration policy for code plan execution.
// Git is the substrate, not a bypass: branches per ticket, commits tagged,
// PRs from structured content, merge under policy.
type GitPolicy struct {
	// BranchPrefix is the prefix for ticket branches (default: "ticket/").
	BranchPrefix string `json:"branch_prefix"`
	// BaseBranch is the target branch for PRs (default: "main").
	BaseBranch string `json:"base_branch"`
	// RequirePR indicates whether a PR is required before merge (default: true).
	RequirePR bool `json:"require_pr"`
	// RequireReview indicates whether PR review is required (default: true).
	RequireReview bool `json:"require_review"`
	// RequireCI indicates whether CI must pass before merge (default: true).
	RequireCI bool `json:"require_ci"`
	// AutoMerge indicates whether to auto-merge when all checks pass (default: false).
	AutoMerge bool `json:"auto_merge"`
	// CommitTagPattern is the pattern for commit tags (default: "ticket/{ticket_id}").
	CommitTagPattern string `json:"commit_tag_pattern"`
}

// DefaultGitPolicy returns conservative git policy defaults.
func DefaultGitPolicy() *GitPolicy {
	return &GitPolicy{
		BranchPrefix:     "ticket/",
		BaseBranch:       "main",
		RequirePR:        true,
		RequireReview:    true,
		RequireCI:        true,
		AutoMerge:        false,
		CommitTagPattern: "ticket/{ticket_id}",
	}
}

// BranchNameForTicket generates the branch name for a given ticket ID.
func (gp *GitPolicy) BranchNameForTicket(ticketID string) string {
	prefix := gp.BranchPrefix
	if prefix == "" {
		prefix = "ticket/"
	}
	return prefix + ticketID
}

// CommitTagForTicket generates the commit tag for a given ticket ID.
func (gp *GitPolicy) CommitTagForTicket(ticketID string) string {
	pattern := gp.CommitTagPattern
	if pattern == "" {
		pattern = "ticket/{ticket_id}"
	}
	return strings.Replace(pattern, "{ticket_id}", ticketID, 1)
}

// FormatCommitMessage creates a structured commit message from a code plan.
// The message includes ticket ID, plan summary, and affected files.
func FormatCommitMessage(codePlan *CodePlan, ticketID, title string) string {
	var b strings.Builder

	// Subject line: ticket ID + title.
	b.WriteString(fmt.Sprintf("%s: %s\n\n", ticketID, title))

	// Body: affected files.
	if len(codePlan.Diffs) > 0 {
		b.WriteString("Files changed:\n")
		for _, diff := range codePlan.Diffs {
			operation := "modified"
			for _, te := range codePlan.TargetEntities {
				if te.ID == diff.FilePath {
					operation = te.OperationType
					break
				}
			}
			b.WriteString(fmt.Sprintf("  - %s (%s)\n", diff.FilePath, operation))
		}
		b.WriteString("\n")
	}

	// Symbol changes summary.
	if len(codePlan.SymbolSnapshots) > 0 {
		b.WriteString("Symbols affected:\n")
		for _, ss := range codePlan.SymbolSnapshots {
			action := inferSymbolAction(ss)
			exported := ""
			if ss.Exported {
				exported = " [exported]"
			}
			b.WriteString(fmt.Sprintf("  - %s %s (%s)%s\n", action, ss.SymbolID, ss.Kind, exported))
		}
		b.WriteString("\n")
	}

	// Plan ticket reference.
	b.WriteString(fmt.Sprintf("Plan-Ticket: %s\n", ticketID))

	return b.String()
}

// FormatPRBody creates a structured PR body from a code plan.
// PRs are populated from ticket content — no manual PR description needed.
func FormatPRBody(codePlan *CodePlan, ticketID, objective string) string {
	var b strings.Builder

	b.WriteString("## Summary\n\n")
	if objective != "" {
		b.WriteString(objective + "\n\n")
	}

	// Files changed.
	if len(codePlan.Diffs) > 0 {
		b.WriteString("## Changes\n\n")
		for _, diff := range codePlan.Diffs {
			hunkCount := len(diff.Hunks)
			b.WriteString(fmt.Sprintf("- `%s` — %d hunk(s)\n", diff.FilePath, hunkCount))
		}
		b.WriteString("\n")
	}

	// Symbol impact.
	exportedChanges := 0
	for _, ss := range codePlan.SymbolSnapshots {
		if ss.Exported && ss.BeforeSignature != ss.AfterSignature {
			exportedChanges++
		}
	}
	if exportedChanges > 0 {
		b.WriteString(fmt.Sprintf("⚠️ **%d exported symbol(s) changed** — review for API compatibility.\n\n", exportedChanges))
	}

	// Test expectations.
	if codePlan.TestExpectations != nil {
		b.WriteString("## Test Plan\n\n")
		for _, cmd := range codePlan.TestExpectations.TestCommands {
			b.WriteString(fmt.Sprintf("- [ ] `%s`\n", cmd))
		}
		b.WriteString("\n")
	}

	// Rollback plan.
	if codePlan.RollbackPlan != nil {
		b.WriteString("## Rollback\n\n")
		b.WriteString(fmt.Sprintf("Strategy: `%s`\n", codePlan.RollbackPlan.Strategy))
		if codePlan.RollbackPlan.RevertCommitRef != "" {
			b.WriteString(fmt.Sprintf("Revert to: `%s`\n", codePlan.RollbackPlan.RevertCommitRef))
		}
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("---\nPlan-Ticket: %s\n", ticketID))

	return b.String()
}

// GitInstruction describes a git operation the executor should perform.
// These are generated from the plan's GitContext and GitPolicy.
type GitInstruction struct {
	// Operation is the git operation: "create_branch", "checkout", "commit", "push", "create_pr", "merge".
	Operation string `json:"operation"`
	// Args contains operation-specific arguments.
	Args map[string]string `json:"args"`
	// Description is a human-readable explanation of the instruction.
	Description string `json:"description"`
}

// GenerateGitInstructions produces the sequence of git operations for a code plan.
func GenerateGitInstructions(codePlan *CodePlan, ticketID, title string, policy *GitPolicy) []GitInstruction {
	if policy == nil {
		policy = DefaultGitPolicy()
	}

	var instructions []GitInstruction
	branch := policy.BranchNameForTicket(ticketID)

	// 1. Create or checkout branch.
	if codePlan.GitContext != nil && codePlan.GitContext.Branch != "" {
		instructions = append(instructions, GitInstruction{
			Operation:   "checkout",
			Args:        map[string]string{"branch": codePlan.GitContext.Branch},
			Description: fmt.Sprintf("Checkout existing branch %s", codePlan.GitContext.Branch),
		})
	} else {
		instructions = append(instructions, GitInstruction{
			Operation:   "create_branch",
			Args:        map[string]string{"branch": branch, "base": policy.BaseBranch},
			Description: fmt.Sprintf("Create branch %s from %s", branch, policy.BaseBranch),
		})
	}

	// 2. Commit with structured message.
	commitMsg := FormatCommitMessage(codePlan, ticketID, title)
	instructions = append(instructions, GitInstruction{
		Operation:   "commit",
		Args:        map[string]string{"message": commitMsg, "tag": policy.CommitTagForTicket(ticketID)},
		Description: fmt.Sprintf("Commit changes tagged with %s", ticketID),
	})

	// 3. Push branch.
	instructions = append(instructions, GitInstruction{
		Operation:   "push",
		Args:        map[string]string{"branch": branch},
		Description: fmt.Sprintf("Push branch %s to remote", branch),
	})

	// 4. Create PR if policy requires it.
	if policy.RequirePR {
		prBody := FormatPRBody(codePlan, ticketID, title)
		instructions = append(instructions, GitInstruction{
			Operation: "create_pr",
			Args: map[string]string{
				"title":  fmt.Sprintf("%s: %s", ticketID, title),
				"body":   prBody,
				"base":   policy.BaseBranch,
				"head":   branch,
				"labels": "plan-managed",
			},
			Description: fmt.Sprintf("Create PR from %s to %s", branch, policy.BaseBranch),
		})
	}

	return instructions
}
