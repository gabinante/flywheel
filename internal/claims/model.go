// Package claims implements the claims registry for concurrency control (spec v0.2 §4.3).
// When a ticket enters execution, its plan's declared touches are registered as claims.
// Claims are keyed by (entity_id, environment) and support conflict classification:
// hard conflicts (serialize), soft conflicts (advisory), and parallel-safe (disjoint).
package claims

import "time"

// ClaimType identifies what kind of resource is being claimed.
type ClaimType string

const (
	ClaimFileWrite    ClaimType = "file_write"
	ClaimSymbol       ClaimType = "symbol"
	ClaimService      ClaimType = "service"
	ClaimSchema       ClaimType = "schema"
	ClaimResource     ClaimType = "resource"
	ClaimDeployTarget ClaimType = "deploy_target"
)

// AllClaimTypes returns all valid claim types.
func AllClaimTypes() []ClaimType {
	return []ClaimType{ClaimFileWrite, ClaimSymbol, ClaimService, ClaimSchema, ClaimResource, ClaimDeployTarget}
}

// IsValidClaimType returns true if ct is a known claim type.
func IsValidClaimType(ct ClaimType) bool {
	for _, valid := range AllClaimTypes() {
		if ct == valid {
			return true
		}
	}
	return false
}

// ClaimState represents the lifecycle of a claim.
type ClaimState string

const (
	StateActive   ClaimState = "active"
	StateReleased ClaimState = "released"
)

// ConflictType classifies how two claims overlap.
type ConflictType string

const (
	ConflictSameFileWrite ConflictType = "same_file_write"
	ConflictSameSymbol    ConflictType = "same_symbol"
	ConflictSameService   ConflictType = "same_service"
	ConflictSameSchema    ConflictType = "same_schema"
	ConflictDisjoint      ConflictType = "disjoint"
)

// Severity classifies whether a conflict blocks execution or is advisory.
type Severity string

const (
	SeverityHard Severity = "hard" // Must serialize: same entity + environment + write
	SeveritySoft Severity = "soft" // Advisory: overlapping scope but not identical target
)

// Claim represents a registered resource claim by a ticket in execution.
type Claim struct {
	ID          string            `json:"id"`
	TicketID    string            `json:"ticket_id"`
	EntityID    string            `json:"entity_id"`
	Environment string            `json:"environment"`
	ClaimType   ClaimType         `json:"claim_type"`
	State       ClaimState        `json:"state"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	ClaimedAt   time.Time         `json:"claimed_at"`
	ReleasedAt  *time.Time        `json:"released_at,omitempty"`
}

// Conflict records a detected conflict between two tickets' claims.
type Conflict struct {
	ID             string       `json:"id"`
	TicketID       string       `json:"ticket_id"`
	BlockingTicket string       `json:"blocking_ticket"`
	ClaimID        string       `json:"claim_id"`
	ConflictType   ConflictType `json:"conflict_type"`
	Severity       Severity     `json:"severity"`
	DetectedAt     time.Time    `json:"detected_at"`
	ResolvedAt     *time.Time   `json:"resolved_at,omitempty"`
}

// Touch represents a declared resource touch from a plan that will become a claim.
// This is the input format — plans declare touches, the registry converts them to claims.
type Touch struct {
	EntityID    string            `json:"entity_id"`
	Environment string            `json:"environment"`
	ClaimType   ClaimType         `json:"claim_type"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
}

// ConflictResult is returned by conflict detection with classification details.
type ConflictResult struct {
	HasConflicts bool         `json:"has_conflicts"`
	HardCount    int          `json:"hard_count"`
	SoftCount    int          `json:"soft_count"`
	Conflicts    []Conflict   `json:"conflicts,omitempty"`
	// ParallelSafe is true when no conflicts were detected.
	ParallelSafe bool         `json:"parallel_safe"`
}
