package plan

import (
	"strings"
)

// Classification represents the risk classification of a plan.
// The classifier reads the manifest, not the script body.
type Classification string

const (
	ClassificationReadOnly    Classification = "read_only"
	ClassificationIdempotent  Classification = "idempotent"
	ClassificationMutating    Classification = "mutating"
	ClassificationDestructive Classification = "destructive"
)

// ClassificationOrder defines classification severity (lower index = less severe).
var ClassificationOrder = []Classification{
	ClassificationReadOnly,
	ClassificationIdempotent,
	ClassificationMutating,
	ClassificationDestructive,
}

// ClassifyManifest determines the risk classification of a shell plan based
// solely on its side-effect manifest. The classifier never reads the script body.
//
// Rules:
// - Wildcard file ops (containing ** or *) → destructive
// - Wildcard network endpoints (containing *) → destructive
// - File delete ops → at minimum mutating, destructive if broad patterns
// - File write ops → at minimum mutating
// - Network ops with non-idempotent methods (POST, PUT, DELETE, PATCH) → mutating
// - Network ops with idempotent methods only (GET, HEAD, OPTIONS) → read_only contribution
// - Read-only file ops with no other effects → read_only
// - All ops declared idempotent → idempotent
//
// Each wildcard widens the classification proportionally:
// - Single wildcard in a narrow path (e.g., /app/dist/*.js) → mutating
// - Double wildcard or broad patterns (e.g., /**, *.*, *:*) → destructive
func ClassifyManifest(manifest *SideEffectManifest) Classification {
	if manifest == nil {
		return ClassificationReadOnly
	}

	current := ClassificationReadOnly

	// Classify file ops.
	for _, op := range manifest.FileOps {
		opClass := classifyFileOp(op)
		current = widenClassification(current, opClass)
	}

	// Classify network ops.
	for _, op := range manifest.NetworkOps {
		opClass := classifyNetworkOp(op)
		current = widenClassification(current, opClass)
	}

	// Classify process ops.
	for _, op := range manifest.ProcessOps {
		opClass := classifyProcessOp(op)
		current = widenClassification(current, opClass)
	}

	// Credential access is always at least mutating (secret exposure risk).
	if len(manifest.CredentialOps) > 0 {
		current = widenClassification(current, ClassificationMutating)
	}

	return current
}

// classifyFileOp classifies a single file operation.
func classifyFileOp(op FileOp) Classification {
	wildcardLevel := countWildcardLevel(op.Path)

	switch op.Action {
	case "read":
		if wildcardLevel >= 2 {
			return ClassificationMutating // Broad read access is concerning
		}
		return ClassificationReadOnly
	case "write":
		if wildcardLevel >= 2 {
			return ClassificationDestructive
		}
		if wildcardLevel >= 1 {
			return ClassificationMutating
		}
		return ClassificationMutating
	case "delete":
		if wildcardLevel >= 1 {
			return ClassificationDestructive
		}
		return ClassificationMutating
	default:
		return ClassificationDestructive // Unknown action → most restrictive
	}
}

// classifyNetworkOp classifies a single network operation.
func classifyNetworkOp(op NetworkOp) Classification {
	// Wildcard endpoints are always destructive.
	if containsWildcard(op.Endpoint) {
		return ClassificationDestructive
	}

	// Non-idempotent methods are mutating.
	switch strings.ToUpper(op.Method) {
	case "GET", "HEAD", "OPTIONS":
		if op.Idempotent {
			return ClassificationReadOnly
		}
		return ClassificationIdempotent
	case "POST", "PUT", "DELETE", "PATCH":
		if op.Idempotent {
			return ClassificationIdempotent
		}
		return ClassificationMutating
	default:
		// TCP/UDP raw connections are at least mutating.
		if containsWildcard(op.Endpoint) {
			return ClassificationDestructive
		}
		return ClassificationMutating
	}
}

// classifyProcessOp classifies a single process operation.
func classifyProcessOp(op ProcessOp) Classification {
	// Wildcard binary names are destructive.
	if containsWildcard(op.Binary) {
		return ClassificationDestructive
	}
	// Wildcard args widen classification.
	for _, arg := range op.Args {
		if arg == "*" || strings.Contains(arg, "**") {
			return ClassificationMutating
		}
	}
	return ClassificationIdempotent
}

// widenClassification returns the more severe of two classifications.
func widenClassification(current, candidate Classification) Classification {
	if classificationSeverity(candidate) > classificationSeverity(current) {
		return candidate
	}
	return current
}

// classificationSeverity returns numeric severity (higher = more severe).
func classificationSeverity(c Classification) int {
	for i, level := range ClassificationOrder {
		if c == level {
			return i
		}
	}
	return len(ClassificationOrder) // Unknown = most severe
}

// countWildcardLevel returns the wildcard "breadth" of a glob pattern.
// 0 = no wildcards (exact path), 1 = single wildcard, 2 = double wildcard or multiple.
func countWildcardLevel(pattern string) int {
	if strings.Contains(pattern, "**") {
		return 2
	}
	count := strings.Count(pattern, "*")
	if count > 1 {
		return 2
	}
	return count
}

// containsWildcard returns true if the string contains glob wildcards.
func containsWildcard(s string) bool {
	return strings.Contains(s, "*") || strings.Contains(s, "?")
}
