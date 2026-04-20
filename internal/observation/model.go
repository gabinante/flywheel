package observation

import (
	"errors"
	"time"
)

// Sentinel errors.
var (
	ErrWindowNotFound      = errors.New("observation window not found")
	ErrSignalNotFound      = errors.New("signal not found")
	ErrAttributionNotFound = errors.New("attribution not found")
)

// ObservationWindow represents the monitoring period after a ticket's change is deployed.
// During this window, production signals are tracked and attributed to changes.
type ObservationWindow struct {
	ID        string      `json:"id"`
	TicketID  string      `json:"ticket_id"`
	ProjectID string      `json:"project_id"`
	Scope     Scope       `json:"scope"`
	StartedAt time.Time   `json:"started_at"`
	EndsAt    time.Time   `json:"ends_at"`
	ClosedAt  *time.Time  `json:"closed_at,omitempty"`
	State     WindowState `json:"state"`
}

// IsOpen returns true if the window is still active (not closed and not past end time).
func (w *ObservationWindow) IsOpen() bool {
	if w.ClosedAt != nil {
		return false
	}
	return w.State == WindowActive || w.State == WindowExtended
}

// Overlaps returns true if this window's active period overlaps with [start, end].
func (w *ObservationWindow) Overlaps(start, end time.Time) bool {
	effectiveEnd := w.EndsAt
	if w.ClosedAt != nil {
		effectiveEnd = *w.ClosedAt
	}
	return !w.StartedAt.After(end) && !effectiveEnd.Before(start)
}

// WindowState represents the state of an observation window.
type WindowState string

const (
	WindowActive   WindowState = "active"
	WindowClosed   WindowState = "closed"
	WindowExtended WindowState = "extended"
)

// Scope defines what a change touches — used for attribution matching.
// Each dimension is weighted differently in scope-match scoring.
type Scope struct {
	// Files changed by the ticket's implementation (highest specificity).
	Files []string `json:"files,omitempty"`
	// Services affected (e.g. "api", "worker", "frontend").
	Services []string `json:"services,omitempty"`
	// Packages or modules touched.
	Packages []string `json:"packages,omitempty"`
	// Tags are free-form labels for matching (e.g. "auth", "billing").
	Tags []string `json:"tags,omitempty"`
}

// IsEmpty returns true if the scope has no entries.
func (s Scope) IsEmpty() bool {
	return len(s.Files) == 0 && len(s.Services) == 0 && len(s.Packages) == 0 && len(s.Tags) == 0
}

// Signal represents a production observation (regression, anomaly, incident).
type Signal struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Source     string            `json:"source"`
	SignalType SignalType         `json:"signal_type"`
	Severity   Severity          `json:"severity"`
	Title      string            `json:"title"`
	Detail     string            `json:"detail,omitempty"`
	Scope      Scope             `json:"scope"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	OccurredAt time.Time         `json:"occurred_at"`
	CreatedAt  time.Time         `json:"created_at"`
}

// SignalType classifies the kind of production signal.
type SignalType string

const (
	SignalRegression SignalType = "regression"
	SignalAnomaly    SignalType = "anomaly"
	SignalIncident   SignalType = "incident"
	SignalAlert      SignalType = "alert"
)

// Severity of the signal.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// Attribution links a signal to candidate tickets with a confidence score.
type Attribution struct {
	ID          string    `json:"id"`
	SignalID    string    `json:"signal_id"`
	ProjectID   string    `json:"project_id"`
	Candidates  []Candidate `json:"candidates"`
	Confidence  float64   `json:"confidence"`
	Rationale   string    `json:"rationale"`
	Retroactive bool      `json:"retroactive"`
	Resolved    bool      `json:"resolved"`
	ResolvedBy  string    `json:"resolved_by,omitempty"`
	Resolution  string    `json:"resolution,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// IsLowConfidence returns true if this attribution has low confidence (ambiguous).
func (a *Attribution) IsLowConfidence() bool {
	return a.Confidence < 0.5 || len(a.Candidates) > 1
}

// Candidate is a ticket that may have caused a regression.
type Candidate struct {
	TicketID   string  `json:"ticket_id"`
	WindowID   string  `json:"window_id"`
	ScopeMatch float64 `json:"scope_match"` // 0.0 to 1.0
	Rationale  string  `json:"rationale"`
}

// AmbiguityMetrics tracks attribution ambiguity over time as a meta-signal
// for release cadence density.
type AmbiguityMetrics struct {
	ProjectID              string    `json:"project_id"`
	WindowStart            time.Time `json:"window_start"`
	WindowEnd              time.Time `json:"window_end"`
	TotalAttributions      int       `json:"total_attributions"`
	LowConfidenceCount     int       `json:"low_confidence_count"`
	AmbiguityRate          float64   `json:"ambiguity_rate"`
	AvgCandidatesPerSignal float64   `json:"avg_candidates_per_signal"`
}

// SignalAction describes what to do when a signal is received.
type SignalAction string

const (
	ActionCreateTicket   SignalAction = "create_ticket"
	ActionAnnotateTicket SignalAction = "annotate_ticket"
	ActionSurfaceHuman   SignalAction = "surface_human"
	ActionIgnore         SignalAction = "ignore"
)

// SignalRule defines how a signal maps to a ticket action in the signal-to-ticket pipeline.
type SignalRule struct {
	ID          string      `json:"id"`
	ProjectID   string      `json:"project_id"`
	Source      string      `json:"source"`
	SignalType  SignalType   `json:"signal_type"`
	MinSeverity Severity    `json:"min_severity"`
	Action      SignalAction `json:"action"`
	Enabled     bool        `json:"enabled"`
}
