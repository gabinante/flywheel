package risk

// EnvironmentMultiplier adjusts risk level based on the deployment environment.
//
// The multiplier is a signed offset applied to the base risk level:
//   - Development: -1 (reduce severity, minimum safe)
//   - Staging:      0 (no change)
//   - Production:  +1 (increase severity, maximum destructive)
//
// Operators can configure custom multipliers per environment.
type EnvironmentMultiplier struct {
	// Multipliers maps environment names to their risk offset.
	// Positive values increase risk; negative values decrease.
	Multipliers map[string]int
}

// DefaultEnvironmentMultipliers returns the standard dev/staging/prod configuration.
func DefaultEnvironmentMultipliers() *EnvironmentMultiplier {
	return &EnvironmentMultiplier{
		Multipliers: map[string]int{
			"development": -1,
			"dev":         -1,
			"staging":     0,
			"stage":       0,
			"production":  1,
			"prod":        1,
		},
	}
}

// Apply adjusts a risk level based on the environment. Returns the adjusted level
// and a citation if the level changed.
func (em *EnvironmentMultiplier) Apply(level RiskLevel, environment string) (RiskLevel, *RuleCitation) {
	if environment == "" {
		return level, nil
	}

	offset, ok := em.Multipliers[environment]
	if !ok {
		// Unknown environment → treat as production (maximum caution).
		offset = 1
	}

	if offset == 0 {
		return level, nil
	}

	adjusted := RiskLevel(int(level) + offset)

	// Clamp to valid range [RiskSafe, RiskDestructive].
	if adjusted < RiskSafe {
		adjusted = RiskSafe
	}
	if adjusted > RiskDestructive {
		adjusted = RiskDestructive
	}

	if adjusted == level {
		return level, nil
	}

	direction := "increased"
	if adjusted < level {
		direction = "decreased"
	}

	return adjusted, &RuleCitation{
		Rule:        "env." + environment,
		Backend:     "",
		Level:       adjusted,
		Description: "Environment '" + environment + "' multiplier " + direction + " risk from " + level.String() + " to " + adjusted.String(),
	}
}
