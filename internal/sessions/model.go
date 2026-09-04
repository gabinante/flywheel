// Package sessions tracks every Claude Code and Codex session the operator runs —
// interactive, dispatched, automation, or subagent — by ingesting the harnesses'
// own local stores, and links each session to the PRs and Linear issues it touched.
package sessions

import "time"

// Harness identifies the coding agent that produced a session.
type Harness string

const (
	HarnessClaudeCode Harness = "claude_code"
	HarnessCodex      Harness = "codex"
)

// Origin says how a session was started.
type Origin string

const (
	OriginInteractive Origin = "interactive"
	OriginDispatched  Origin = "dispatched"
	OriginAutomation  Origin = "automation"
	OriginSubagent    Origin = "subagent"
)

// Status is derived from recency, since neither harness records an explicit end
// for interactive sessions.
type Status string

const (
	StatusActive Status = "active" // activity within the last 5 minutes
	StatusIdle   Status = "idle"   // activity within the last hour
	StatusEnded  Status = "ended"
)

const (
	activeWindow = 5 * time.Minute
	idleWindow   = time.Hour
)

// Link kinds and sources.
const (
	LinkPR          = "pr"
	LinkLinearIssue = "linear_issue"
	LinkTicket      = "ticket"
	LinkReview      = "review"

	LinkSourceInferred = "inferred"
	LinkSourceExplicit = "explicit"
	LinkSourceDispatch = "dispatch"
)

// Session is one tracked harness session.
type Session struct {
	ID               string         `json:"id"`
	Harness          Harness        `json:"harness"`
	ExternalID       string         `json:"external_id"`
	Origin           Origin         `json:"origin"`
	ParentSessionID  string         `json:"parent_session_id,omitempty"`
	ParentExternalID string         `json:"parent_external_id,omitempty"`
	CWD              string         `json:"cwd"`
	Repo             string         `json:"repo"`
	Branch           string         `json:"branch"`
	Model            string         `json:"model"`
	ReasoningEffort  string         `json:"reasoning_effort"`
	Title            string         `json:"title"`
	FirstPrompt      string         `json:"first_prompt"`
	TranscriptPath   string         `json:"transcript_path"`
	TokensIn         int64          `json:"tokens_in"`
	TokensOut        int64          `json:"tokens_out"`
	PromptCount      int            `json:"prompt_count"`
	ToolCallCount    int            `json:"tool_call_count"`
	StartedAt        time.Time      `json:"started_at"`
	LastActivityAt   time.Time      `json:"last_activity_at"`
	EndedAt          *time.Time     `json:"ended_at,omitempty"`
	IngestOffset     int64          `json:"-"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Links            []Link         `json:"links,omitempty"`
}

// Status derives the session status at time now.
func (s *Session) Status(now time.Time) Status {
	if s.EndedAt != nil {
		return StatusEnded
	}
	age := now.Sub(s.LastActivityAt)
	switch {
	case age < activeWindow:
		return StatusActive
	case age < idleWindow:
		return StatusIdle
	default:
		return StatusEnded
	}
}

// Link records that a session touched an external object.
type Link struct {
	SessionID string    `json:"session_id"`
	Kind      string    `json:"kind"`
	Ref       string    `json:"ref"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// Prompt is one operator prompt (or notable agent message) in a session.
type Prompt struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Seq       int       `json:"seq"`
	Role      string    `json:"role"`
	Text      string    `json:"text"`
	TS        time.Time `json:"ts"`
}

// Filter narrows a session listing.
type Filter struct {
	Harness          Harness
	Origin           Origin
	Repo             string
	Branch           string
	Status           Status
	Query            string // full-text over prompts, title, first prompt
	Ref              string // only sessions linked to this ref (owner/repo#N, KEY-N, ticket id)
	Since            *time.Time
	IncludeSubagents bool
	Limit            int
	Offset           int
}

// CollectorStatus reports the ingestion loop's health.
type CollectorStatus struct {
	Enabled        bool           `json:"enabled"`
	ClaudeDir      string         `json:"claude_dir"`
	CodexDir       string         `json:"codex_dir"`
	LastRunAt      *time.Time     `json:"last_run_at,omitempty"`
	LastError      string         `json:"last_error,omitempty"`
	LastDurationMS int64          `json:"last_duration_ms"`
	SessionsTotal  int            `json:"sessions_total"`
	ByHarness      map[string]int `json:"by_harness"`
}
