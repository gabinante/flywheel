package codereview

import (
	"encoding/json"
	"fmt"
	"github.com/gabinante/flywheel/internal/prompts"
	"sort"
	"strings"
)

// FindingsSchema is the structured output the reviewing harness must produce.
var FindingsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "summary": {"type": "string", "description": "One or two short, conversational sentences: the verdict and any concrete blocker. A clean review can simply say: Looks good to me."},
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "severity": {"type": "string", "enum": ["P0", "P1", "P2", "P3"]},
          "title": {"type": "string", "description": "Imperative, one line"},
          "path": {"type": "string", "description": "Repo-relative file path from the diff"},
          "line": {"type": "integer", "description": "Line number on the new side of the diff"},
          "body": {"type": "string", "description": "One short paragraph to a colleague: the affected scenario, what breaks, and a practical fix"}
        },
        "required": ["severity", "title", "path", "line", "body"],
        "additionalProperties": false
      }
    }
  },
  "required": ["summary", "findings"],
  "additionalProperties": false
}`)

// ReviewSystemPrompt captures the operator's review recipe: in-depth, defect-first,
// conversational, and gated on P0/P1.
const ReviewSystemPrompt = `You are reviewing a pull request the way a senior engineer on the team would: in depth, defect-first, and in a
conversational tone. You are running read-only in a checkout of the PR head; the unified diff against the base
branch is in the file named in the task. Read the whole diff, then read enough surrounding code, tests, and call
sites to confirm each finding is real. Follow any AGENTS.md or CLAUDE.md in the repository.

Flag an issue only when all of these hold: it affects correctness, security, data integrity, performance, or
maintainability in a meaningful way; it is discrete and actionable; it was introduced by this change; you can
demonstrate the affected scenario from the code; and the author would fix it if they knew. Do not flag speculative
concerns, pre-existing problems, intentional behavior changes, or style nits. Continue through the whole diff after
the first issue.

Severity: P0 = release blocker or critical failure; P1 = urgent defect to fix before merge; P2 = ordinary defect
worth fixing; P3 = low impact but still worth a comment. Cite the file path and a line number that is part of the
diff on the new side. Write each body as the inline comment itself — friendly, direct, specific, with a concrete
fix — not as a report about a comment.

Keep the posted review short, conversational, and pragmatic. Use one or two sentences for the summary;
"Looks good to me." is enough when there are no findings. Give each inline comment one short paragraph
that explains the affected scenario, impact, and a practical fix. Skip change recaps, generic praise, review
process narration, and repeated finding lists. Do not add a signature, branding, or a "reviewed by" tagline.

Approval policy: recommend approval in the summary when there are no P0 or P1 findings, including when the
review contains only P2 or P3 findings. P2 and P3 findings are nonblocking comments. Recommend changes only
when a P0 or P1 finding remains. Flywheel derives the approval verdict from the finding severities and handles
posting the review; do not post a review yourself.

Return the structured output only: a summary and the findings array. If nothing qualifies, return an empty
findings array and say so in the summary; never invent a finding.`

// BuildReviewPrompt renders the task for one PR.
func BuildReviewPrompt(pr *PR, diffPath string, files []string, priorFindings []Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review pull request %s#%d: %s\n", pr.Repo, pr.Number, pr.Title)
	fmt.Fprintf(&b, "Author: %s. Base: %s. Head: %s (%s).\nURL: %s\n\n", pr.Author, pr.BaseRef, pr.HeadRef, short(pr.HeadSHA), pr.URL)
	if body := strings.TrimSpace(pr.Body); body != "" {
		fmt.Fprintf(&b, "PR description:\n%s\n\n", truncateStr(body, 6000))
	}
	fmt.Fprintf(&b, "The unified diff (base...head) is at: %s\nThe working directory is the PR head checkout.\n\n", diffPath)
	if len(files) > 0 {
		sort.Strings(files)
		fmt.Fprintf(&b, "Changed files (%d):\n", len(files))
		for i, f := range files {
			if i >= 80 {
				fmt.Fprintf(&b, "  … and %d more\n", len(files)-80)
				break
			}
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("\n")
	}
	if len(priorFindings) > 0 {
		b.WriteString("This is a re-review after new commits. Findings from the previous round (check whether each is addressed; do not repeat ones that are fixed):\n")
		for _, f := range priorFindings {
			fmt.Fprintf(&b, "  - [%s] %s — %s:%d\n", f.Severity, f.Title, f.Path, f.Line)
		}
		b.WriteString("\n")
	}
	b.WriteString("Produce the structured review now.")
	return b.String()
}

// ReviewOutput is the parsed structured output.
type ReviewOutput struct {
	Summary  string `json:"summary"`
	Findings []struct {
		Severity string `json:"severity"`
		Title    string `json:"title"`
		Path     string `json:"path"`
		Line     int    `json:"line"`
		Body     string `json:"body"`
	} `json:"findings"`
}

// ParseReviewOutput decodes the harness output into findings.
func ParseReviewOutput(raw []byte) (string, []Finding, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	// Tolerate a fenced code block around the JSON.
	if s := string(raw); strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "```json"), "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		raw = []byte(s)
	}
	var out ReviewOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", nil, fmt.Errorf("review output is not the expected JSON: %w", err)
	}
	findings := make([]Finding, 0, len(out.Findings))
	for _, f := range out.Findings {
		sev := strings.ToUpper(strings.TrimSpace(f.Severity))
		switch sev {
		case "P0", "P1", "P2", "P3":
		default:
			sev = "P2"
		}
		findings = append(findings, Finding{
			Severity: sev,
			Title:    strings.TrimSpace(f.Title),
			Path:     strings.TrimPrefix(strings.TrimSpace(f.Path), "./"),
			Line:     f.Line,
			Side:     "RIGHT",
			Body:     strings.TrimSpace(f.Body),
			Status:   "pending",
		})
	}
	return strings.TrimSpace(out.Summary), findings, nil
}

// Verdict applies the recipe: any P0/P1 requests changes, otherwise approve.
func Verdict(findings []Finding) string {
	for _, f := range findings {
		if f.Blocking() {
			return VerdictRequestChanges
		}
	}
	return VerdictApprove
}

// ComposeReview splits findings into inline comments (lines present in the diff)
// and body bullets (everything else), and renders the review body.
func ComposeReview(summary string, findings []Finding, diff *DiffIndex) (body string, inline []int, inBody []int) {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(summary))
	for i, f := range findings {
		if diff.Contains(f.Path, f.Line) {
			inline = append(inline, i)
		} else {
			inBody = append(inBody, i)
		}
	}
	if len(inBody) > 0 {
		b.WriteString("\n")
		for _, i := range inBody {
			f := findings[i]
			loc := f.Path
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
			}
			fmt.Fprintf(&b, "\n- **[%s] %s** — `%s`\n  %s\n", f.Severity, f.Title, loc, strings.ReplaceAll(f.Body, "\n", "\n  "))
		}
	}
	return strings.TrimSpace(b.String()), inline, inBody
}

// InlineCommentBody renders one finding as its inline comment.
func InlineCommentBody(f Finding) string {
	return fmt.Sprintf("**[%s] %s**\n\n%s", f.Severity, f.Title, f.Body)
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func init() {
	prompts.Register(prompts.Prompt{ID: "code_review", Name: "Code reviewer", Order: 10,
		Description: "System prompt for PR-keyed code reviews: what to flag, severities, tone. The per-PR task (diff path, files, prior findings) is appended automatically.",
		UsedBy:      "Code review", Default: ReviewSystemPrompt})
}
