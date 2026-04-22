package dispatch

import (
	"fmt"
	"strings"

	"github.com/gabinante/flywheel/internal/ticket"
)

// assembleConflictResolverPrompt builds a system prompt for a conflict resolver agent.
func assembleConflictResolverPrompt(t *ticket.Ticket, prURL, branch string) string {
	var b strings.Builder

	b.WriteString("You are a conflict resolution agent. Your ONLY job is to rebase a feature branch onto main and resolve any merge conflicts.\n\n")

	b.WriteString("## Context\n\n")
	b.WriteString(fmt.Sprintf("- **Ticket:** %s — %s\n", t.ID, t.Title))
	b.WriteString(fmt.Sprintf("- **PR:** %s\n", prURL))
	b.WriteString(fmt.Sprintf("- **Branch:** %s\n", branch))
	b.WriteString("\n")

	b.WriteString("## Instructions\n\n")
	b.WriteString("1. You are already in the ticket's worktree on the correct branch.\n")
	b.WriteString("2. Fetch the latest main: `git fetch origin main`\n")
	b.WriteString("3. Rebase onto main: `git rebase origin/main`\n")
	b.WriteString("4. If there are conflicts:\n")
	b.WriteString("   - Resolve each conflicting file by keeping the intent of BOTH the ticket's changes AND main's changes.\n")
	b.WriteString("   - The ticket's changes implement a specific feature; main has other features that were merged since.\n")
	b.WriteString("   - After resolving each file: `git add <file>` then `git rebase --continue`\n")
	b.WriteString("5. Force-push the rebased branch: `git push --force-with-lease origin HEAD`\n")
	b.WriteString("6. That's it. Do NOT create new commits, do NOT modify the PR, do NOT call any MCP tools.\n")
	b.WriteString("\n")

	b.WriteString("## Rules\n\n")
	b.WriteString("- Do NOT change the ticket's logic or functionality — only resolve conflicts.\n")
	b.WriteString("- Prefer keeping both sides' changes (the ticket's feature AND main's updates).\n")
	b.WriteString("- If a file was deleted on main but modified on the branch, accept the deletion unless the ticket clearly needs the file.\n")
	b.WriteString("- If imports conflict, include all needed imports from both sides.\n")
	b.WriteString("- After rebase, run `go build ./...` to verify compilation. Fix any build errors from the merge.\n")
	b.WriteString("- Do NOT run tests (the reviewer already approved the logic).\n")
	b.WriteString("- Do NOT call any MCP tools. This is a pure git operation.\n")

	return b.String()
}
