package investigation

import (
	"fmt"
	"strings"
)

// buildInvestigationPrompt creates the system prompt for an investigation subagent.
// The prompt constrains the agent to read-only research and structured output.
func buildInvestigationPrompt(req *Request, tokenBudget int) string {
	var b strings.Builder

	b.WriteString("You are an investigation subagent. Your role is bounded research: ")
	b.WriteString("answer the given question with structured findings.\n\n")

	b.WriteString("## Constraints\n\n")
	b.WriteString("- **Read-only.** You do NOT write code, create files, commit, or mutate any state.\n")
	b.WriteString("- **One level deep.** You do NOT dispatch further investigations or spawn subagents.\n")
	b.WriteString("- **Scoped.** Stay within the defined investigation scope.\n")
	b.WriteString("- **Structured output.** Your final output MUST be a JSON block with findings.\n")
	b.WriteString(fmt.Sprintf("- **Token budget.** Keep your findings JSON under %d tokens (≈%d characters).\n", tokenBudget, tokenBudget*4))
	b.WriteString("- **Evidence-based.** Every claim must have file:line citations.\n")
	b.WriteString("- **Explicit negative space.** Document what you looked for but did NOT find.\n\n")

	// Scope
	if hasScope(req.Scope) {
		b.WriteString("## Investigation scope\n\n")
		if len(req.Scope.Files) > 0 {
			b.WriteString("**Files to investigate:**\n")
			for _, f := range req.Scope.Files {
				b.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
			b.WriteString("\n")
		}
		if len(req.Scope.Symbols) > 0 {
			b.WriteString("**Symbols to investigate:**\n")
			for _, s := range req.Scope.Symbols {
				b.WriteString(fmt.Sprintf("- `%s`\n", s))
			}
			b.WriteString("\n")
		}
		if len(req.Scope.Packages) > 0 {
			b.WriteString("**Packages to investigate:**\n")
			for _, p := range req.Scope.Packages {
				b.WriteString(fmt.Sprintf("- `%s`\n", p))
			}
			b.WriteString("\n")
		}
		if len(req.Scope.ExcludeFiles) > 0 {
			b.WriteString("**Excluded files (do not read):**\n")
			for _, f := range req.Scope.ExcludeFiles {
				b.WriteString(fmt.Sprintf("- `%s`\n", f))
			}
			b.WriteString("\n")
		}
		if len(req.Scope.Constraints) > 0 {
			b.WriteString("**Additional constraints:**\n")
			for _, c := range req.Scope.Constraints {
				b.WriteString(fmt.Sprintf("- %s\n", c))
			}
			b.WriteString("\n")
		}
	}

	// Output format
	b.WriteString("## Required output format\n\n")
	b.WriteString("After completing your research, output your findings as a single JSON block:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{
  "status": "complete",
  "claims": [
    {
      "statement": "The Dispatcher struct uses a sync.Mutex for concurrency control",
      "confidence": "high",
      "citations": [
        {"file": "internal/dispatch/dispatcher.go", "line": 69, "snippet": "mu sync.Mutex"}
      ],
      "invariants": ["The mu field is always locked before accessing the active map"]
    }
  ],
  "negative_space": [
    "No recursive dispatch was found in the codebase",
    "No existing investigation model exists"
  ],
  "open_questions": [
    "Whether the Worker interface should be extended or composed"
  ]
}`)
	b.WriteString("\n```\n\n")

	b.WriteString("## Field definitions\n\n")
	b.WriteString("- **status**: `complete` (answered fully), `partial` (hit budget/scope limits)\n")
	b.WriteString("- **claims**: Verified assertions. Each needs:\n")
	b.WriteString("  - `statement`: What you're asserting\n")
	b.WriteString("  - `confidence`: `high` (directly verified), `medium` (inferred), `low` (uncertain)\n")
	b.WriteString("  - `citations`: Array of `{file, line, end_line?, snippet?}` — evidence\n")
	b.WriteString("  - `invariants`: (optional) Conditions that must hold for claim to remain true\n")
	b.WriteString("- **negative_space**: Things you explicitly checked for and did NOT find\n")
	b.WriteString("- **open_questions**: Unresolved items needing further investigation\n\n")

	b.WriteString("## Process\n\n")
	b.WriteString("1. Read relevant files within scope\n")
	b.WriteString("2. Search for patterns, usages, and relationships\n")
	b.WriteString("3. Verify your findings with direct file:line evidence\n")
	b.WriteString("4. Document what you looked for but didn't find (negative space)\n")
	b.WriteString("5. Note any questions that remain unresolved\n")
	b.WriteString("6. Output the JSON findings block as your final response\n\n")

	b.WriteString("**IMPORTANT:** Your FINAL output must contain exactly one ```json block with your findings. ")
	b.WriteString("Everything before it is your working notes. The JSON block is what gets parsed.\n")

	return b.String()
}

// buildTaskMessage creates the task message for the investigation subagent.
func buildTaskMessage(req *Request) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Investigate the following question:\n\n**%s**\n\n", req.Question))

	if req.ParentTicketID != "" {
		b.WriteString(fmt.Sprintf("This investigation was requested by ticket %s.\n\n", req.ParentTicketID))
	}

	b.WriteString("Read the relevant code, gather evidence, and output your structured findings as JSON. ")
	b.WriteString("Remember: you are read-only. Do not modify any files or create commits.")

	return b.String()
}

// hasScope returns true if the scope has any non-empty field.
func hasScope(s Scope) bool {
	return len(s.Files) > 0 || len(s.Symbols) > 0 || len(s.Packages) > 0 ||
		len(s.ExcludeFiles) > 0 || len(s.Constraints) > 0
}
