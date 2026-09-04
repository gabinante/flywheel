// Package report reproduces the operator's Linear reporting habits inside Flywheel:
// a delta status update per Linear-linked project ("what merged and closed since the
// last update, what is in review, what is blocked") and a weekly roundup across
// projects. Previews render from Flywheel's own data plus the gh CLI; posting goes to
// Linear project updates and a rolling Linear document.
package report

import "time"

// Kinds of report.
const (
	KindProjectUpdate = "project_update"
	KindWeeklyRoundup = "weekly_roundup"
)

// Health values accepted by Linear project updates.
const (
	HealthOnTrack  = "onTrack"
	HealthAtRisk   = "atRisk"
	HealthOffTrack = "offTrack"
)

// Report is a composed (and possibly posted) report.
type Report struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	ProjectID   string    `json:"project_id,omitempty"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Body        string    `json:"body"`
	Health      string    `json:"health,omitempty"`
	Posted      bool      `json:"posted"`
	URL         string    `json:"url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// TicketLine is one ticket in a report section.
type TicketLine struct {
	Identifier string
	Title      string
	URL        string
	State      string
}

// PRLine is one pull request in a report section.
type PRLine struct {
	Repo   string
	Number int
	Title  string
	URL    string
	When   time.Time
}

// HarnessLine is agent activity for one harness.
type HarnessLine struct {
	Harness   string
	Sessions  int
	TokensIn  int64
	TokensOut int64
}

// ProjectData is everything a project update is rendered from.
type ProjectData struct {
	ProjectName string
	LinearURL   string
	Since       time.Time
	Until       time.Time
	Done        []TicketLine
	InReview    []TicketLine
	InProgress  []TicketLine
	Blocked     []TicketLine
	New         []TicketLine
	MergedPRs   []PRLine
	OpenPRs     []PRLine
	Feedback    []PRLine // landed reviews awaiting a response
	Reviews     []PRLine // reviews Flywheel posted for the operator
	Activity    []HarnessLine
}

// WeekData is everything the weekly roundup is rendered from.
type WeekData struct {
	Start    time.Time
	End      time.Time
	Projects []ProjectData
	MergedPRs []PRLine // across all repos, de-duplicated
	Activity []HarnessLine
}
