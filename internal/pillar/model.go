// Package pillar implements Layer 15: the pillar and strategy layer.
// Seven fixed pillars (Observability, Mutability, Scalability, Availability,
// Security, Resiliency, Cost) with per-entity strategy statements, structured
// claims citing project-map entities, known gaps, and continuous evaluation.
package pillar

import (
	"errors"
	"time"
)

// Sentinel errors.
var (
	ErrEntryNotFound      = errors.New("pillar entry not found")
	ErrClaimNotFound      = errors.New("pillar claim not found")
	ErrEvaluationNotFound = errors.New("evaluation not found")
	ErrInvalidPillarType  = errors.New("invalid pillar type")
	ErrInvalidClaimStatus = errors.New("invalid claim status")
	ErrVersionConflict    = errors.New("pillar entry version conflict")
)

// PillarType is one of the seven fixed architectural pillars.
type PillarType string

const (
	PillarObservability PillarType = "observability"
	PillarMutability    PillarType = "mutability"
	PillarScalability   PillarType = "scalability"
	PillarAvailability  PillarType = "availability"
	PillarSecurity      PillarType = "security"
	PillarResiliency    PillarType = "resiliency"
	PillarCost          PillarType = "cost"
)

// AllPillarTypes returns the complete set of seven pillars.
func AllPillarTypes() []PillarType {
	return []PillarType{
		PillarObservability, PillarMutability, PillarScalability,
		PillarAvailability, PillarSecurity, PillarResiliency, PillarCost,
	}
}

// IsValidPillarType returns true if t is one of the seven fixed pillars.
func IsValidPillarType(t string) bool {
	for _, v := range AllPillarTypes() {
		if string(v) == t {
			return true
		}
	}
	return false
}

// ClaimStatus represents the verification state of a claim.
type ClaimStatus string

const (
	ClaimVerified   ClaimStatus = "verified"
	ClaimUnverified ClaimStatus = "unverified"
	ClaimStale      ClaimStatus = "stale"
	ClaimInvalid    ClaimStatus = "invalid"
)

// AllClaimStatuses returns all valid claim statuses.
func AllClaimStatuses() []ClaimStatus {
	return []ClaimStatus{ClaimVerified, ClaimUnverified, ClaimStale, ClaimInvalid}
}

// IsValidClaimStatus returns true if s is a known claim status.
func IsValidClaimStatus(s string) bool {
	for _, v := range AllClaimStatuses() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// EvalCheckType identifies what aspect of a claim the evaluation examines.
type EvalCheckType string

const (
	EvalEvidenceIntact      EvalCheckType = "evidence_intact"
	EvalClaimMatchesReality EvalCheckType = "claim_matches_reality"
	EvalStrategyAppropriate EvalCheckType = "strategy_appropriate"
)

// EvalOutcome is the result of a single evaluation check.
type EvalOutcome string

const (
	EvalPass EvalOutcome = "pass"
	EvalWarn EvalOutcome = "warn"
	EvalFail EvalOutcome = "fail"
)

// ReviewCadence defines how often a pillar entry should be reviewed.
type ReviewCadence string

const (
	CadenceWeekly    ReviewCadence = "weekly"
	CadenceBiweekly  ReviewCadence = "biweekly"
	CadenceMonthly   ReviewCadence = "monthly"
	CadenceQuarterly ReviewCadence = "quarterly"
)

// AllCadences returns all valid review cadences.
func AllCadences() []ReviewCadence {
	return []ReviewCadence{CadenceWeekly, CadenceBiweekly, CadenceMonthly, CadenceQuarterly}
}

// IsValidCadence returns true if c is a known review cadence.
func IsValidCadence(c string) bool {
	for _, v := range AllCadences() {
		if string(v) == c {
			return true
		}
	}
	return false
}

// Entry is a per-entity pillar record: strategy, claims, gaps, and review cadence
// for one of the seven pillars applied to a specific project-map entity.
type Entry struct {
	ID             string        `json:"id"`
	ProjectID      string        `json:"project_id"`
	EntityID       string        `json:"entity_id"`        // references catalog entity by ID
	EntityType     string        `json:"entity_type"`      // informational: "service", "datastore", etc.
	PillarType     PillarType    `json:"pillar_type"`      // one of seven fixed pillars
	Strategy       string        `json:"strategy"`         // strategy statement
	Gaps           []Gap         `json:"gaps,omitempty"`   // known gaps
	ReviewCadence  ReviewCadence `json:"review_cadence"`   // how often to review
	LastReviewedAt *time.Time    `json:"last_reviewed_at,omitempty"`
	NextReviewAt   *time.Time    `json:"next_review_at,omitempty"`
	Version        int           `json:"version"`
	CreatedBy      string        `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// Gap describes a known gap in a pillar's coverage for an entity.
type Gap struct {
	Description string `json:"description"`
	Severity    string `json:"severity"` // "low", "medium", "high", "critical"
	Mitigation  string `json:"mitigation,omitempty"`
}

// Claim is a structured assertion about a pillar, citing a specific map entity as evidence.
type Claim struct {
	ID              string      `json:"id"`
	PillarEntryID   string      `json:"pillar_entry_id"`
	Statement       string      `json:"statement"`          // what is being claimed
	EntityRefID     string      `json:"entity_ref_id"`      // catalog entity ID cited as evidence
	EntityRefType   string      `json:"entity_ref_type"`    // type of the referenced entity
	Evidence        string      `json:"evidence"`           // supporting evidence/proof
	Status          ClaimStatus `json:"status"`             // verification state
	LastEvaluatedAt *time.Time  `json:"last_evaluated_at,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// Evaluation records the result of a continuous evaluation check on a pillar entry.
type Evaluation struct {
	ID            string        `json:"id"`
	PillarEntryID string        `json:"pillar_entry_id"`
	CheckType     EvalCheckType `json:"check_type"` // what was checked
	Outcome       EvalOutcome   `json:"outcome"`    // pass / warn / fail
	Details       string        `json:"details"`    // human-readable explanation
	EvaluatedAt   time.Time     `json:"evaluated_at"`
}
