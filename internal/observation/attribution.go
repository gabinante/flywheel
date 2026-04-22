package observation

import (
	"fmt"
	"strings"
)

// ComputeScopeMatch calculates how well a ticket's scope matches a signal's scope.
// Returns a float between 0.0 (no overlap) and 1.0 (exact match).
func ComputeScopeMatch(ticketScope, signalScope Scope) float64 {
	if ticketScope.IsEmpty() || signalScope.IsEmpty() {
		// Empty scope means we can't determine overlap -- return low match.
		return 0.1
	}

	var totalWeight, matchedWeight float64

	// Files: highest weight -- most specific signal.
	if len(signalScope.Files) > 0 {
		totalWeight += 4.0
		matchedWeight += 4.0 * overlapRatio(ticketScope.Files, signalScope.Files)
	}

	// Services: high weight.
	if len(signalScope.Services) > 0 {
		totalWeight += 3.0
		matchedWeight += 3.0 * overlapRatio(ticketScope.Services, signalScope.Services)
	}

	// Packages: medium weight.
	if len(signalScope.Packages) > 0 {
		totalWeight += 2.0
		matchedWeight += 2.0 * overlapRatio(ticketScope.Packages, signalScope.Packages)
	}

	// Tags: lowest weight -- most general.
	if len(signalScope.Tags) > 0 {
		totalWeight += 1.0
		matchedWeight += 1.0 * overlapRatio(ticketScope.Tags, signalScope.Tags)
	}

	if totalWeight == 0 {
		return 0.1
	}
	return matchedWeight / totalWeight
}

// overlapRatio returns the fraction of items in b that appear in a.
// Uses prefix matching for file paths.
func overlapRatio(a, b []string) float64 {
	if len(b) == 0 {
		return 0
	}
	matches := 0
	for _, bItem := range b {
		for _, aItem := range a {
			if matchesScope(aItem, bItem) {
				matches++
				break
			}
		}
	}
	return float64(matches) / float64(len(b))
}

// matchesScope checks if ticketItem matches signalItem.
// Supports exact match and prefix matching for hierarchical scopes.
func matchesScope(ticketItem, signalItem string) bool {
	if ticketItem == signalItem {
		return true
	}
	// Prefix match: ticket changed "pkg/api/handler.go", signal scope is "pkg/api"
	if strings.HasPrefix(ticketItem, signalItem+"/") {
		return true
	}
	// Reverse: signal scope is "pkg/api/handler.go", ticket scope is "pkg/api"
	if strings.HasPrefix(signalItem, ticketItem+"/") {
		return true
	}
	return false
}

// ComputeAttributionConfidence determines overall attribution confidence
// based on the number of candidates and their scope matches.
//
// High confidence (>0.7): single candidate with strong scope match.
// Medium confidence (0.4-0.7): few candidates or mixed scope matches.
// Low confidence (<0.4): many candidates with ambiguous overlap.
func ComputeAttributionConfidence(candidates []Candidate) float64 {
	if len(candidates) == 0 {
		return 0.0
	}
	if len(candidates) == 1 {
		// Single candidate: confidence scales with scope match.
		// Floor at 0.5 because being the only candidate is itself strong signal.
		return 0.5 + 0.5*candidates[0].ScopeMatch
	}

	// Multiple candidates: confidence decreases with count.
	bestMatch := 0.0
	totalMatch := 0.0
	for _, c := range candidates {
		if c.ScopeMatch > bestMatch {
			bestMatch = c.ScopeMatch
		}
		totalMatch += c.ScopeMatch
	}
	avgMatch := totalMatch / float64(len(candidates))

	// Separation: how much better is the best match vs average?
	// High separation = one clear winner = higher confidence.
	separation := 0.0
	if bestMatch > 0 {
		separation = (bestMatch - avgMatch) / bestMatch
	}

	// Confidence formula: inversely proportional to candidate count,
	// boosted by separation and best match quality.
	countFactor := 1.0 / float64(len(candidates))
	confidence := countFactor*0.4 + bestMatch*0.3 + separation*0.3

	// Clamp to [0, 1].
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}
	return confidence
}

// BuildCandidateRationale generates a human-readable explanation for a candidate.
func BuildCandidateRationale(ticketScope, signalScope Scope, scopeMatch float64) string {
	var parts []string

	fileOverlap := overlapRatio(ticketScope.Files, signalScope.Files)
	if fileOverlap > 0 {
		parts = append(parts, fmt.Sprintf("file overlap: %.0f%%", fileOverlap*100))
	}

	svcOverlap := overlapRatio(ticketScope.Services, signalScope.Services)
	if svcOverlap > 0 {
		parts = append(parts, fmt.Sprintf("service overlap: %.0f%%", svcOverlap*100))
	}

	pkgOverlap := overlapRatio(ticketScope.Packages, signalScope.Packages)
	if pkgOverlap > 0 {
		parts = append(parts, fmt.Sprintf("package overlap: %.0f%%", pkgOverlap*100))
	}

	tagOverlap := overlapRatio(ticketScope.Tags, signalScope.Tags)
	if tagOverlap > 0 {
		parts = append(parts, fmt.Sprintf("tag overlap: %.0f%%", tagOverlap*100))
	}

	if len(parts) == 0 {
		return fmt.Sprintf("scope match: %.0f%% (no specific overlap identified)", scopeMatch*100)
	}
	return fmt.Sprintf("scope match: %.0f%% (%s)", scopeMatch*100, strings.Join(parts, ", "))
}
