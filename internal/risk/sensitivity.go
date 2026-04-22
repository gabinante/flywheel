package risk

// SensitivityClassifier reclassifies risk upward based on service sensitivity tags
// from the project/service map layer. Sensitivity tags can only increase risk, never
// decrease it.
//
// Tags and their minimum risk floors:
//   - "pii"            → minimum high (handles personal data)
//   - "financial"      → minimum high (handles money/billing)
//   - "hipaa"          → minimum destructive (healthcare compliance)
//   - "sox"            → minimum destructive (financial compliance)
//   - "critical-path"  → minimum high (on the critical service path)
//   - "customer-facing"→ minimum reversible (directly affects users)
//   - "internal-only"  → no adjustment
//
// Operators can register additional tags via the TagFloors map.
type SensitivityClassifier struct {
	// TagFloors maps service sensitivity tags to their minimum risk floors.
	TagFloors map[string]RiskLevel
}

// DefaultSensitivityClassifier returns a classifier with standard tag definitions.
func DefaultSensitivityClassifier() *SensitivityClassifier {
	return &SensitivityClassifier{
		TagFloors: map[string]RiskLevel{
			"pii":             RiskHigh,
			"financial":       RiskHigh,
			"hipaa":           RiskDestructive,
			"sox":             RiskDestructive,
			"pci":             RiskDestructive,
			"critical-path":   RiskHigh,
			"customer-facing": RiskReversible,
			"auth":            RiskHigh,
			"payments":        RiskHigh,
			"internal-only":   RiskSafe, // no floor — no adjustment
		},
	}
}

// Apply checks service tags and raises the risk level if any tag has a higher floor.
// Tags can only raise risk, never lower it. Returns the adjusted level and any citations.
func (sc *SensitivityClassifier) Apply(level RiskLevel, tags []string) (RiskLevel, []RuleCitation) {
	if len(tags) == 0 {
		return level, nil
	}

	result := level
	var citations []RuleCitation

	for _, tag := range tags {
		floor, ok := sc.TagFloors[tag]
		if !ok {
			// Unknown tag → treat as high risk (conservative).
			floor = RiskHigh
			citations = append(citations, RuleCitation{
				Rule:        "sensitivity.unknown_tag",
				Backend:     "",
				Level:       floor,
				Description: "Unknown service tag '" + tag + "' — defaults to high risk floor",
			})
		}

		if floor > result {
			result = floor
			citations = append(citations, RuleCitation{
				Rule:        "sensitivity." + tag,
				Backend:     "",
				Level:       floor,
				Description: "Service tag '" + tag + "' raises minimum risk to " + floor.String(),
			})
		}
	}

	return result, citations
}
