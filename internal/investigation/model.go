// Package investigation provides subagent investigation dispatch for coordinators.
// Investigations are bounded research tasks that return structured findings:
// claims with file:line citations, verified invariants, explicit negative space,
// and open questions. One level deep — subagents do not spawn subagents.
package investigation

import "time"

// Request defines a scoped investigation to dispatch to a subagent.
type Request struct {
	// ProjectID is the project this investigation belongs to.
	ProjectID string `json:"project_id"`

	// Question is the specific research question to answer.
	Question string `json:"question"`

	// Scope constrains the investigation boundaries.
	Scope Scope `json:"scope"`

	// TokenBudget is the maximum tokens the investigation response may use.
	// Enforced at response time — the subagent is instructed to stay within budget.
	// Default: 4000 tokens if zero.
	TokenBudget int `json:"token_budget,omitempty"`

	// ParentTicketID is the ticket that triggered this investigation (for tracing).
	ParentTicketID string `json:"parent_ticket_id,omitempty"`

	// RequestedBy is the agent ID of the coordinator requesting the investigation.
	RequestedBy string `json:"requested_by,omitempty"`
}

// Scope defines the boundaries of an investigation.
type Scope struct {
	// Files limits investigation to these file patterns (glob).
	// Empty means unrestricted within the repo.
	Files []string `json:"files,omitempty"`

	// Symbols limits investigation to these symbol names (functions, types, etc.).
	Symbols []string `json:"symbols,omitempty"`

	// Packages limits investigation to these Go packages or directories.
	Packages []string `json:"packages,omitempty"`

	// ExcludeFiles excludes these file patterns from investigation.
	ExcludeFiles []string `json:"exclude_files,omitempty"`

	// Constraints are natural language constraints for the subagent.
	Constraints []string `json:"constraints,omitempty"`
}

// Response is the structured result of an investigation.
type Response struct {
	// Status indicates whether the investigation completed successfully.
	Status Status `json:"status"`

	// Question echoes the original question for context.
	Question string `json:"question"`

	// Claims are verified assertions with evidence.
	Claims []Claim `json:"claims"`

	// NegativeSpace documents what was explicitly NOT found or confirmed.
	NegativeSpace []string `json:"negative_space,omitempty"`

	// OpenQuestions are unresolved items that need further investigation.
	OpenQuestions []string `json:"open_questions,omitempty"`

	// TokensUsed is the approximate token count of this response.
	TokensUsed int `json:"tokens_used"`

	// Duration is how long the investigation took.
	Duration time.Duration `json:"duration"`

	// Error contains error details if status is StatusFailed.
	Error string `json:"error,omitempty"`
}

// Status represents the outcome of an investigation.
type Status string

const (
	StatusComplete Status = "complete"
	StatusPartial  Status = "partial"  // Hit token budget before finishing
	StatusFailed   Status = "failed"   // Subagent error
	StatusTimeout  Status = "timeout"  // Investigation exceeded time limit
)

// Claim is a verified assertion with supporting evidence.
type Claim struct {
	// Statement is the claim being made.
	Statement string `json:"statement"`

	// Confidence is the subagent's confidence level: high, medium, low.
	Confidence string `json:"confidence"`

	// Citations are file:line references supporting this claim.
	Citations []Citation `json:"citations"`

	// Invariants are conditions that must hold for this claim to remain true.
	Invariants []string `json:"invariants,omitempty"`
}

// Citation is a file:line reference to supporting evidence.
type Citation struct {
	// File is the relative file path.
	File string `json:"file"`

	// Line is the line number (1-based). Zero means file-level citation.
	Line int `json:"line,omitempty"`

	// EndLine is the end line for multi-line citations.
	EndLine int `json:"end_line,omitempty"`

	// Snippet is the relevant code or text at this location.
	Snippet string `json:"snippet,omitempty"`
}

// DefaultTokenBudget is applied when Request.TokenBudget is zero.
const DefaultTokenBudget = 4000

// MaxTokenBudget is the hard upper limit for token budgets.
const MaxTokenBudget = 16000
