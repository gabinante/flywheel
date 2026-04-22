package pillar

import (
	"fmt"
	"time"
)

// evaluateEvidenceIntact checks that all claims reference entities and have non-empty evidence.
// This is a structural check — it does not verify the entity actually exists in the catalog
// (that would require a catalog dependency). It checks that references are populated.
func evaluateEvidenceIntact(entry *Entry, claims []*Claim) *Evaluation {
	if len(claims) == 0 {
		return &Evaluation{
			CheckType: EvalEvidenceIntact,
			Outcome:   EvalWarn,
			Details:   "No claims defined for this pillar entry; cannot verify evidence.",
		}
	}

	missingRef := 0
	missingEvidence := 0
	for _, c := range claims {
		if c.EntityRefID == "" {
			missingRef++
		}
		if c.Evidence == "" {
			missingEvidence++
		}
	}

	if missingRef > 0 || missingEvidence > 0 {
		return &Evaluation{
			CheckType: EvalEvidenceIntact,
			Outcome:   EvalFail,
			Details: fmt.Sprintf("%d/%d claims missing entity reference, %d/%d missing evidence",
				missingRef, len(claims), missingEvidence, len(claims)),
		}
	}

	return &Evaluation{
		CheckType: EvalEvidenceIntact,
		Outcome:   EvalPass,
		Details:   fmt.Sprintf("All %d claims have entity references and evidence.", len(claims)),
	}
}

// evaluateClaimMatchesReality checks that claims are in a verified state and not stale.
// A claim is considered stale if it hasn't been evaluated in the last 30 days.
func evaluateClaimMatchesReality(claims []*Claim) *Evaluation {
	if len(claims) == 0 {
		return &Evaluation{
			CheckType: EvalClaimMatchesReality,
			Outcome:   EvalWarn,
			Details:   "No claims to evaluate.",
		}
	}

	staleThreshold := time.Now().UTC().AddDate(0, 0, -30)
	verified := 0
	stale := 0
	invalid := 0
	unverified := 0

	for _, c := range claims {
		switch c.Status {
		case ClaimVerified:
			if c.LastEvaluatedAt != nil && c.LastEvaluatedAt.Before(staleThreshold) {
				stale++
			} else {
				verified++
			}
		case ClaimStale:
			stale++
		case ClaimInvalid:
			invalid++
		case ClaimUnverified:
			unverified++
		}
	}

	if invalid > 0 {
		return &Evaluation{
			CheckType: EvalClaimMatchesReality,
			Outcome:   EvalFail,
			Details: fmt.Sprintf("%d invalid, %d stale, %d unverified, %d verified out of %d claims",
				invalid, stale, unverified, verified, len(claims)),
		}
	}

	if stale > 0 || unverified > 0 {
		return &Evaluation{
			CheckType: EvalClaimMatchesReality,
			Outcome:   EvalWarn,
			Details: fmt.Sprintf("%d stale, %d unverified, %d verified out of %d claims",
				stale, unverified, verified, len(claims)),
		}
	}

	return &Evaluation{
		CheckType: EvalClaimMatchesReality,
		Outcome:   EvalPass,
		Details:   fmt.Sprintf("All %d claims verified and current.", len(claims)),
	}
}

// evaluateStrategyAppropriate checks whether the strategy is adequate given gaps and claims.
// Fails if there are critical gaps without mitigation; warns if high-severity gaps exist.
func evaluateStrategyAppropriate(entry *Entry, claims []*Claim) *Evaluation {
	if entry.Strategy == "" {
		return &Evaluation{
			CheckType: EvalStrategyAppropriate,
			Outcome:   EvalFail,
			Details:   "No strategy statement defined.",
		}
	}

	criticalUnmitigated := 0
	highSeverity := 0
	for _, g := range entry.Gaps {
		if g.Severity == "critical" && g.Mitigation == "" {
			criticalUnmitigated++
		}
		if g.Severity == "high" {
			highSeverity++
		}
	}

	if criticalUnmitigated > 0 {
		return &Evaluation{
			CheckType: EvalStrategyAppropriate,
			Outcome:   EvalFail,
			Details: fmt.Sprintf("Strategy has %d critical unmitigated gaps and %d high-severity gaps.",
				criticalUnmitigated, highSeverity),
		}
	}

	if highSeverity > 0 {
		return &Evaluation{
			CheckType: EvalStrategyAppropriate,
			Outcome:   EvalWarn,
			Details:   fmt.Sprintf("Strategy has %d high-severity gaps to address.", highSeverity),
		}
	}

	if len(claims) == 0 {
		return &Evaluation{
			CheckType: EvalStrategyAppropriate,
			Outcome:   EvalWarn,
			Details:   "Strategy defined but no claims support it yet.",
		}
	}

	return &Evaluation{
		CheckType: EvalStrategyAppropriate,
		Outcome:   EvalPass,
		Details: fmt.Sprintf("Strategy adequate: %d claims, %d gaps (none critical unmitigated).",
			len(claims), len(entry.Gaps)),
	}
}
