package risk

// BackendClassifier is the interface for per-backend risk classifiers.
// Each backend (database, terraform, code, shell, external API) implements this.
type BackendClassifier interface {
	// Classify returns the risk level and rule citations for a single operation.
	Classify(op Operation) (RiskLevel, []RuleCitation)
}

// Classifier is the top-level risk classification engine. It composes per-backend
// classifiers, the environment multiplier, sensitivity tags, and LLM advisory into
// a single deterministic classification pipeline.
//
// Classification pipeline:
//  1. Per-backend classifiers score each operation independently.
//  2. The highest per-operation score becomes the base level.
//  3. Environment multiplier adjusts (dev ↓, prod ↑).
//  4. Service sensitivity tags reclassify upward.
//  5. LLM advisory can raise (never lower).
//  6. Final level + all citations returned.
//
// Same Plan → same Classification, every time.
type Classifier struct {
	backends    map[Backend]BackendClassifier
	envMult     *EnvironmentMultiplier
	sensitivity *SensitivityClassifier
}

// NewClassifier creates a Classifier with all default per-backend classifiers,
// default environment multipliers, and default sensitivity tags.
func NewClassifier() *Classifier {
	return &Classifier{
		backends: map[Backend]BackendClassifier{
			BackendDatabase:    &DatabaseClassifier{},
			BackendTerraform:   &TerraformClassifier{},
			BackendCode:        &CodeClassifier{},
			BackendShell:       &ShellClassifier{},
			BackendExternalAPI: NewExternalAPIClassifier(),
		},
		envMult:     DefaultEnvironmentMultipliers(),
		sensitivity: DefaultSensitivityClassifier(),
	}
}

// SetEnvironmentMultiplier replaces the environment multiplier configuration.
func (c *Classifier) SetEnvironmentMultiplier(em *EnvironmentMultiplier) {
	c.envMult = em
}

// SetSensitivityClassifier replaces the sensitivity classifier configuration.
func (c *Classifier) SetSensitivityClassifier(sc *SensitivityClassifier) {
	c.sensitivity = sc
}

// RegisterBackend registers (or replaces) a per-backend classifier.
// Operators can use this to swap in custom classifiers for specific backends.
func (c *Classifier) RegisterBackend(backend Backend, classifier BackendClassifier) {
	c.backends[backend] = classifier
}

// Classify runs the full classification pipeline on a Plan and returns a
// deterministic Classification. Same Plan always produces the same result.
//
// If advisory is non-nil, it is applied as the final upward-only adjustment.
func (c *Classifier) Classify(plan Plan, advisory *LLMAdvisory) Classification {
	var allCitations []RuleCitation
	baseLevel := RiskSafe

	// Empty plan (no operations) → safe.
	if len(plan.Operations) == 0 {
		allCitations = append(allCitations, RuleCitation{
			Rule:        "plan.empty",
			Backend:     "",
			Level:       RiskSafe,
			Description: "Plan has no operations — classified as safe",
		})
		return Classification{
			BaseLevel:             RiskSafe,
			EnvironmentMultiplied: RiskSafe,
			SensitivityAdjusted:   RiskSafe,
			AdvisoryAdjusted:      RiskSafe,
			FinalLevel:            RiskSafe,
			Citations:             allCitations,
		}
	}

	// Step 1: Per-backend classification of each operation.
	for _, op := range plan.Operations {
		classifier, ok := c.backends[op.Backend]
		if !ok {
			// Unknown backend → maximum risk.
			level := RiskDestructive
			citation := RuleCitation{
				Rule:        "backend.unknown",
				Backend:     op.Backend,
				Level:       RiskDestructive,
				Description: "Unknown backend '" + string(op.Backend) + "' — defaults to maximum risk",
			}
			allCitations = append(allCitations, citation)
			if level > baseLevel {
				baseLevel = level
			}
			continue
		}

		level, citations := classifier.Classify(op)
		allCitations = append(allCitations, citations...)
		if level > baseLevel {
			baseLevel = level
		}
	}

	// Step 2: Environment multiplier.
	envLevel := baseLevel
	if c.envMult != nil {
		adjusted, citation := c.envMult.Apply(baseLevel, plan.Environment)
		envLevel = adjusted
		if citation != nil {
			allCitations = append(allCitations, *citation)
		}
	}

	// Step 3: Service sensitivity tags (upward only).
	sensLevel := envLevel
	if c.sensitivity != nil && len(plan.ServiceTags) > 0 {
		adjusted, citations := c.sensitivity.Apply(envLevel, plan.ServiceTags)
		sensLevel = adjusted
		allCitations = append(allCitations, citations...)
	}

	// Step 4: LLM advisory (upward only).
	advisoryLevel := sensLevel
	if advisory != nil {
		adjusted, citation := ApplyAdvisory(sensLevel, advisory)
		advisoryLevel = adjusted
		if citation != nil {
			allCitations = append(allCitations, *citation)
		}
	}

	return Classification{
		BaseLevel:             baseLevel,
		EnvironmentMultiplied: envLevel,
		SensitivityAdjusted:   sensLevel,
		AdvisoryAdjusted:      advisoryLevel,
		FinalLevel:            advisoryLevel,
		Citations:             allCitations,
	}
}
