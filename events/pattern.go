package events

import "strings"

// MatchPattern checks if an event type matches a subscription pattern.
// Pattern syntax:
//   - Exact match: "ticket.created" matches only "ticket.created"
//   - Single wildcard: "ticket.*" matches "ticket.created", "ticket.closed", etc.
//   - Double wildcard: "ticket.**" matches "ticket.created", "ticket.state.changed", etc.
//   - Leading wildcard: "*.created" matches "ticket.created", "project.created", etc.
//
// Segments are separated by '.'.
func MatchPattern(pattern, eventType string) bool {
	if pattern == eventType {
		return true
	}
	if pattern == "**" {
		return true
	}

	patternParts := strings.Split(pattern, ".")
	typeParts := strings.Split(eventType, ".")

	return matchParts(patternParts, typeParts)
}

func matchParts(pattern, subject []string) bool {
	pi, si := 0, 0

	for pi < len(pattern) && si < len(subject) {
		switch pattern[pi] {
		case "**":
			// ** matches zero or more segments.
			// If it's the last pattern segment, match everything remaining.
			if pi == len(pattern)-1 {
				return true
			}
			// Try matching ** against zero through all remaining subject segments.
			for k := si; k <= len(subject); k++ {
				if matchParts(pattern[pi+1:], subject[k:]) {
					return true
				}
			}
			return false
		case "*":
			// * matches exactly one segment (any value).
			pi++
			si++
		default:
			// Exact match required for this segment.
			if pattern[pi] != subject[si] {
				return false
			}
			pi++
			si++
		}
	}

	// Consume trailing ** patterns (they can match zero segments).
	for pi < len(pattern) && pattern[pi] == "**" {
		pi++
	}

	return pi == len(pattern) && si == len(subject)
}
