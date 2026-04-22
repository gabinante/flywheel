// Package plan — freshness.go implements plan freshness stamps and re-plan-before-apply
// mechanics. Plans carry rich freshness stamps (commit SHA, state-index snapshot,
// claims-registry snapshot, observed entity versions). Before apply, the dispatcher
// checks whether stamped state has changed. If a re-generated plan is structurally
// identical, it proceeds automatically. If it differs, it routes to review.
// Per-environment staleness thresholds are configurable.
package plan

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// FreshnessData is the rich freshness stamp attached to a plan. It captures the
// exact state of the world at plan-generation time so the dispatcher can detect drift
// before apply. Stored as JSONB alongside the legacy freshness_stamp TIMESTAMPTZ.
type FreshnessData struct {
	// CapturedAt is when this snapshot was taken.
	CapturedAt time.Time `json:"captured_at"`

	// CommitSHA is the git commit hash the plan was generated against.
	CommitSHA string `json:"commit_sha,omitempty"`

	// StateIndexTimestamp is the high-water mark of the state index when the plan
	// was generated. If the state index has advanced past this, state has changed.
	StateIndexTimestamp *time.Time `json:"state_index_timestamp,omitempty"`

	// ClaimsRegistryTimestamp is the high-water mark of the claims registry.
	// If new claims have been filed since, the plan may be operating on stale intent.
	ClaimsRegistryTimestamp *time.Time `json:"claims_registry_timestamp,omitempty"`

	// ObservedEntityVersions maps entity IDs to their observed versions at plan time.
	// Any entity whose current version exceeds the stamped version indicates drift.
	ObservedEntityVersions map[string]EntityVersion `json:"observed_entity_versions,omitempty"`
}

// EntityVersion captures the version of a single observed entity at plan-generation time.
type EntityVersion struct {
	Version   int    `json:"version"`
	Hash      string `json:"hash,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// ContentHash computes a deterministic hash of the FreshnessData for quick equality checks.
func (fd *FreshnessData) ContentHash() string {
	if fd == nil {
		return ""
	}
	h := sha256.New()

	h.Write([]byte(fd.CommitSHA))

	if fd.StateIndexTimestamp != nil {
		h.Write([]byte(fd.StateIndexTimestamp.UTC().Format(time.RFC3339Nano)))
	}
	if fd.ClaimsRegistryTimestamp != nil {
		h.Write([]byte(fd.ClaimsRegistryTimestamp.UTC().Format(time.RFC3339Nano)))
	}

	// Sort entity IDs for determinism.
	if len(fd.ObservedEntityVersions) > 0 {
		ids := make([]string, 0, len(fd.ObservedEntityVersions))
		for id := range fd.ObservedEntityVersions {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			ev := fd.ObservedEntityVersions[id]
			h.Write([]byte(fmt.Sprintf("%s:%d:%s", id, ev.Version, ev.Hash)))
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// --- Staleness thresholds ---

// StalenessThreshold configures how long plans stay fresh per environment.
// Dev environments tolerate staleness; prod always re-plans immediately.
type StalenessThreshold struct {
	// Environment name (e.g., "dev", "staging", "prod").
	Environment string `json:"environment"`

	// MaxAge is how long a plan remains fresh after its freshness stamp was captured.
	// Zero means "never stale" (not recommended for prod).
	MaxAge time.Duration `json:"max_age"`

	// RequireRePlan when true forces re-plan even within MaxAge if underlying state changed.
	// This is the conservative default for prod.
	RequireRePlan bool `json:"require_re_plan"`

	// AllowApplyAnywayClasses lists change classes (e.g., "staging-deploy", "config-update")
	// for which policy permits apply-anyway even when the re-plan differs structurally.
	AllowApplyAnywayClasses []string `json:"allow_apply_anyway_classes,omitempty"`
}

// DefaultStalenessThresholds returns conservative per-environment defaults.
// Prod always requires re-plan. Staging tolerates 30 min. Dev tolerates 4 hours.
func DefaultStalenessThresholds() []StalenessThreshold {
	return []StalenessThreshold{
		{
			Environment:   "prod",
			MaxAge:        0, // always re-plan
			RequireRePlan: true,
		},
		{
			Environment:             "staging",
			MaxAge:                  30 * time.Minute,
			RequireRePlan:           true,
			AllowApplyAnywayClasses: []string{"staging-deploy"},
		},
		{
			Environment:   "dev",
			MaxAge:        4 * time.Hour,
			RequireRePlan: false,
		},
	}
}

// StalenessConfig holds the full set of per-environment thresholds.
type StalenessConfig struct {
	Thresholds []StalenessThreshold `json:"thresholds"`
}

// GetThreshold returns the threshold for a given environment. Returns the prod
// default (conservative) if no environment-specific threshold is configured.
func (sc *StalenessConfig) GetThreshold(environment string) StalenessThreshold {
	for _, t := range sc.Thresholds {
		if t.Environment == environment {
			return t
		}
	}
	// Conservative default: behave like prod.
	return StalenessThreshold{
		Environment:   environment,
		MaxAge:        0,
		RequireRePlan: true,
	}
}

// --- Freshness check ---

// FreshnessCheckResult is the outcome of evaluating a plan's freshness before apply.
type FreshnessCheckResult struct {
	// Fresh is true if no staleness was detected.
	Fresh bool `json:"fresh"`

	// StaleFields lists which stamp fields have drifted (e.g., "commit_sha", "entity:xyz").
	StaleFields []string `json:"stale_fields,omitempty"`

	// RequiresRePlan is true if the plan must be regenerated before apply.
	RequiresRePlan bool `json:"requires_re_plan"`

	// CanApplyAnyway is true if policy permits applying despite staleness.
	CanApplyAnyway bool `json:"can_apply_anyway"`

	// PolicyOverride explains why apply-anyway is permitted (if applicable).
	PolicyOverride string `json:"policy_override,omitempty"`
}

// CheckFreshness evaluates a plan's freshness data against the current state.
// currentState is the freshly-captured state; planStamp is what was captured at plan time.
// threshold configures per-environment staleness rules.
func CheckFreshness(planStamp *FreshnessData, currentState *FreshnessData, threshold StalenessThreshold, changeClass string) FreshnessCheckResult {
	result := FreshnessCheckResult{Fresh: true}

	if planStamp == nil {
		// No freshness data — fall back to legacy time-based check.
		// Consider it fresh (the simple ExpiresAt check handles this case).
		return result
	}

	// Check time-based staleness first.
	if threshold.MaxAge > 0 {
		age := time.Since(planStamp.CapturedAt)
		if age > threshold.MaxAge {
			result.Fresh = false
			result.StaleFields = append(result.StaleFields, fmt.Sprintf("age:%s>%s", age.Round(time.Second), threshold.MaxAge))
		}
	} else if threshold.MaxAge == 0 && threshold.RequireRePlan {
		// MaxAge=0 + RequireRePlan: always stale (prod default).
		// But only if we have current state to compare against.
		if currentState != nil {
			result.Fresh = false
			result.StaleFields = append(result.StaleFields, "always_replan")
		}
	}

	if currentState == nil {
		// No current state to compare. Use time-based result only.
		result.RequiresRePlan = !result.Fresh && threshold.RequireRePlan
		return result
	}

	// Compare commit SHA.
	if planStamp.CommitSHA != "" && currentState.CommitSHA != "" && planStamp.CommitSHA != currentState.CommitSHA {
		result.Fresh = false
		result.StaleFields = append(result.StaleFields, "commit_sha")
	}

	// Compare state index timestamp.
	if planStamp.StateIndexTimestamp != nil && currentState.StateIndexTimestamp != nil {
		if currentState.StateIndexTimestamp.After(*planStamp.StateIndexTimestamp) {
			result.Fresh = false
			result.StaleFields = append(result.StaleFields, "state_index_timestamp")
		}
	}

	// Compare claims registry timestamp.
	if planStamp.ClaimsRegistryTimestamp != nil && currentState.ClaimsRegistryTimestamp != nil {
		if currentState.ClaimsRegistryTimestamp.After(*planStamp.ClaimsRegistryTimestamp) {
			result.Fresh = false
			result.StaleFields = append(result.StaleFields, "claims_registry_timestamp")
		}
	}

	// Compare observed entity versions.
	if len(planStamp.ObservedEntityVersions) > 0 && len(currentState.ObservedEntityVersions) > 0 {
		for entityID, planVer := range planStamp.ObservedEntityVersions {
			if currentVer, ok := currentState.ObservedEntityVersions[entityID]; ok {
				if currentVer.Version > planVer.Version {
					result.Fresh = false
					result.StaleFields = append(result.StaleFields, fmt.Sprintf("entity:%s:v%d>v%d", entityID, currentVer.Version, planVer.Version))
				}
				if planVer.Hash != "" && currentVer.Hash != "" && planVer.Hash != currentVer.Hash {
					result.Fresh = false
					result.StaleFields = append(result.StaleFields, fmt.Sprintf("entity:%s:hash_changed", entityID))
				}
			}
		}
	}

	result.RequiresRePlan = !result.Fresh && threshold.RequireRePlan

	// Check apply-anyway policy.
	if result.RequiresRePlan && changeClass != "" && len(threshold.AllowApplyAnywayClasses) > 0 {
		for _, allowed := range threshold.AllowApplyAnywayClasses {
			if allowed == changeClass {
				result.CanApplyAnyway = true
				result.PolicyOverride = fmt.Sprintf("change class %q permitted for apply-anyway in environment %q", changeClass, threshold.Environment)
				break
			}
		}
	}

	return result
}

// --- Structural plan comparison ---

// PlanDiff captures the result of comparing two plans for structural equality.
type PlanDiff struct {
	// StructurallyIdentical is true when the two plans have the same content
	// (ignoring metadata like timestamps, IDs).
	StructurallyIdentical bool `json:"structurally_identical"`

	// Differences lists the fields that differ between the two plans.
	Differences []PlanDifference `json:"differences,omitempty"`
}

// PlanDifference describes a single field difference between two plan contents.
type PlanDifference struct {
	Field    string `json:"field"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

// ComparePlanContent compares two plan contents for structural equality.
// This is used after re-plan to determine whether the regenerated plan is
// identical (auto-proceed) or different (route to review).
func ComparePlanContent(old, new Content) PlanDiff {
	diff := PlanDiff{StructurallyIdentical: true}

	// Serialize both to canonical JSON for comparison.
	oldJSON, err1 := json.Marshal(old)
	newJSON, err2 := json.Marshal(new)

	if err1 != nil || err2 != nil {
		diff.StructurallyIdentical = false
		diff.Differences = append(diff.Differences, PlanDifference{
			Field:    "content",
			OldValue: fmt.Sprintf("marshal_error:%v", err1),
			NewValue: fmt.Sprintf("marshal_error:%v", err2),
		})
		return diff
	}

	// Quick check: byte-equal JSON means identical.
	if string(oldJSON) == string(newJSON) {
		return diff
	}

	// Not byte-equal — compute field-level differences.
	diff.StructurallyIdentical = false

	// Compare via normalized maps for more granular diffs.
	var oldMap, newMap map[string]any
	_ = json.Unmarshal(oldJSON, &oldMap)
	_ = json.Unmarshal(newJSON, &newMap)

	diff.Differences = diffMaps("content", oldMap, newMap)

	return diff
}

// diffMaps recursively compares two maps and returns differences.
func diffMaps(prefix string, old, new map[string]any) []PlanDifference {
	var diffs []PlanDifference

	allKeys := make(map[string]bool)
	for k := range old {
		allKeys[k] = true
	}
	for k := range new {
		allKeys[k] = true
	}

	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		path := prefix + "." + k
		oldVal, oldOK := old[k]
		newVal, newOK := new[k]

		if !oldOK {
			diffs = append(diffs, PlanDifference{
				Field:    path,
				OldValue: "<absent>",
				NewValue: fmt.Sprintf("%v", newVal),
			})
			continue
		}
		if !newOK {
			diffs = append(diffs, PlanDifference{
				Field:    path,
				OldValue: fmt.Sprintf("%v", oldVal),
				NewValue: "<absent>",
			})
			continue
		}

		// Recurse into sub-maps.
		oldSub, oldIsMap := oldVal.(map[string]any)
		newSub, newIsMap := newVal.(map[string]any)
		if oldIsMap && newIsMap {
			diffs = append(diffs, diffMaps(path, oldSub, newSub)...)
			continue
		}

		// Compare as JSON strings for type-agnostic comparison.
		oldBytes, _ := json.Marshal(oldVal)
		newBytes, _ := json.Marshal(newVal)
		if string(oldBytes) != string(newBytes) {
			diffs = append(diffs, PlanDifference{
				Field:    path,
				OldValue: string(oldBytes),
				NewValue: string(newBytes),
			})
		}
	}

	return diffs
}

// --- Re-plan decision ---

// RePlanDecision captures the outcome of the re-plan-before-apply evaluation.
type RePlanDecision string

const (
	// RePlanNotNeeded means the plan is still fresh — proceed with apply.
	RePlanNotNeeded RePlanDecision = "not_needed"

	// RePlanIdentical means a re-plan was triggered and produced an identical plan — auto-proceed.
	RePlanIdentical RePlanDecision = "identical"

	// RePlanDiverged means a re-plan was triggered and produced a different plan — route to review.
	RePlanDiverged RePlanDecision = "diverged"

	// RePlanApplyAnyway means the plan diverged but policy permits applying anyway.
	RePlanApplyAnyway RePlanDecision = "apply_anyway"
)

// RePlanResult captures the full re-plan evaluation for the dispatcher.
type RePlanResult struct {
	// Decision is the action the dispatcher should take.
	Decision RePlanDecision `json:"decision"`

	// FreshnessCheck is the detailed freshness evaluation.
	FreshnessCheck FreshnessCheckResult `json:"freshness_check"`

	// Diff is the structural comparison (only set if re-plan was triggered).
	Diff *PlanDiff `json:"diff,omitempty"`

	// Message is a human-readable explanation.
	Message string `json:"message"`
}

// EvaluateRePlan performs the full re-plan-before-apply evaluation.
// It checks freshness, and if a re-plan was triggered, compares the old and new content.
// This is the main entry point for the dispatcher's pre-apply check.
func EvaluateRePlan(
	originalPlan *Plan,
	currentState *FreshnessData,
	newContent *Content,
	threshold StalenessThreshold,
	changeClass string,
) RePlanResult {
	// Step 1: Check freshness.
	freshnessCheck := CheckFreshness(originalPlan.FreshnessData, currentState, threshold, changeClass)

	if freshnessCheck.Fresh {
		return RePlanResult{
			Decision:       RePlanNotNeeded,
			FreshnessCheck: freshnessCheck,
			Message:        "plan is fresh — proceeding with apply",
		}
	}

	if !freshnessCheck.RequiresRePlan {
		// Stale but threshold doesn't require re-plan (e.g., dev environment).
		return RePlanResult{
			Decision:       RePlanNotNeeded,
			FreshnessCheck: freshnessCheck,
			Message:        fmt.Sprintf("plan is stale (%v) but re-plan not required for this environment", freshnessCheck.StaleFields),
		}
	}

	// Step 2: Re-plan was triggered. Compare content if new content is provided.
	if newContent == nil {
		// No new content available yet — signal that re-plan is required.
		return RePlanResult{
			Decision:       RePlanDiverged,
			FreshnessCheck: freshnessCheck,
			Message:        fmt.Sprintf("plan is stale (%v) — re-plan required before apply", freshnessCheck.StaleFields),
		}
	}

	diff := ComparePlanContent(originalPlan.Content, *newContent)

	// Step 3: If structurally identical, auto-proceed.
	if diff.StructurallyIdentical {
		return RePlanResult{
			Decision:       RePlanIdentical,
			FreshnessCheck: freshnessCheck,
			Diff:           &diff,
			Message:        "re-plan produced identical content — auto-proceeding",
		}
	}

	// Step 4: Structural differences. Check policy for apply-anyway.
	if freshnessCheck.CanApplyAnyway {
		return RePlanResult{
			Decision:       RePlanApplyAnyway,
			FreshnessCheck: freshnessCheck,
			Diff:           &diff,
			Message:        fmt.Sprintf("re-plan diverged but apply-anyway permitted: %s", freshnessCheck.PolicyOverride),
		}
	}

	// Step 5: Conservative default — route to review.
	return RePlanResult{
		Decision:       RePlanDiverged,
		FreshnessCheck: freshnessCheck,
		Diff:           &diff,
		Message:        fmt.Sprintf("re-plan diverged (%d differences) — routing to review", len(diff.Differences)),
	}
}
