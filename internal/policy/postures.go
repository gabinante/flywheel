package policy

// PostureBundle is a named, pre-built policy configuration that operators
// can choose as a starting point. Postures are reference configurations;
// operators can customize rules after applying a posture.
type PostureBundle struct {
	Name             string           `json:"name"`
	DisplayName      string           `json:"display_name"`
	Description      string           `json:"description"`
	Rules            []Rule           `json:"rules"`
	CredentialScopes CredentialScopes `json:"credential_scopes"`
	IsDefault        bool             `json:"is_default"` // true for the conservative default
}

// AllPostures returns all five named posture bundles.
// Ordered from most to least restrictive.
func AllPostures() []PostureBundle {
	return []PostureBundle{
		PosturePlanOnly(),
		PostureSandbox(),
		PostureProdGate(),
		PostureGraduatedRisk(),
		PostureParanoidService(),
	}
}

// GetPosture returns a posture by name, or nil if not found.
func GetPosture(name string) *PostureBundle {
	for _, p := range AllPostures() {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// DefaultPostureName returns the name of the conservative default posture.
func DefaultPostureName() string {
	return "plan-only"
}

// PosturePlanOnly creates the plan-only posture:
// Agents author plans and prepare changes, but the operator applies everything manually.
// This is the most conservative posture and the recommended first-run default.
func PosturePlanOnly() PostureBundle {
	return PostureBundle{
		Name:        "plan-only",
		DisplayName: "Plan Only",
		Description: "Agents author plans and prepare changes. Operator applies everything manually. " +
			"Most conservative posture — recommended for first-run.",
		IsDefault: true,
		Rules: []Rule{
			{
				ID:          "plan-only-all-transitions",
				Name:        "all-transitions-plan-only",
				Description: "All state transitions require plan-only mode",
				Predicates:  []Predicate{}, // catch-all
				Action:      ActionPlanOnly,
				Enabled:     true,
			},
		},
		CredentialScopes: CredentialScopes{
			CodeAccess:    CredReadOnly,
			DeployCreds:   CredNone,
			InfraCreds:    CredNone,
			SecretAccess:  CredNone,
			SandboxAccess: false,
		},
	}
}

// PostureSandbox creates the sandbox posture:
// Agents execute in sandboxes, land changes as PRs. Operator merges; their CI/CD deploys.
func PostureSandbox() PostureBundle {
	return PostureBundle{
		Name:        "sandbox",
		DisplayName: "Sandbox",
		Description: "Agents execute in sandboxes and land changes as PRs. " +
			"Operator reviews and merges; their CI/CD pipeline handles deployment.",
		Rules: []Rule{
			{
				ID:          "sandbox-planning",
				Name:        "planning-auto",
				Description: "Planning and speccing proceed automatically",
				Predicates: []Predicate{
					{Field: FieldTransition, Operator: OpIn, Values: []string{"spec", "plan", "claim", "start"}},
				},
				Action:  ActionAuto,
				Enabled: true,
			},
			{
				ID:          "sandbox-execution",
				Name:        "execution-auto",
				Description: "Execution proceeds automatically (in sandbox)",
				Predicates: []Predicate{
					{Field: FieldTransition, Operator: OpIn, Values: []string{"submit", "replan"}},
				},
				Action:  ActionAuto,
				Enabled: true,
			},
			{
				ID:          "sandbox-validation",
				Name:        "validation-approve",
				Description: "Validation (code review) requires human approval",
				Predicates: []Predicate{
					{Field: FieldTransition, Operator: OpIn, Values: []string{"validate", "approve"}},
				},
				Action:  ActionApprove,
				Enabled: true,
			},
			{
				ID:          "sandbox-deploy",
				Name:        "deploy-open-pr",
				Description: "Deployment lands as a PR; operator merges to deploy",
				Predicates: []Predicate{
					{Field: FieldTransition, Operator: OpIn, Values: []string{"deploy"}},
				},
				Action:  ActionOpenPRStop,
				Enabled: true,
			},
			{
				ID:          "sandbox-observe-close",
				Name:        "observe-close-approve",
				Description: "Observing and closing require human confirmation",
				Predicates: []Predicate{
					{Field: FieldTransition, Operator: OpIn, Values: []string{"observe", "close"}},
				},
				Action:  ActionApprove,
				Enabled: true,
			},
		},
		CredentialScopes: CredentialScopes{
			CodeAccess:    CredReadWrite,
			DeployCreds:   CredNone,
			InfraCreds:    CredReadOnly,
			SecretAccess:  CredNone,
			SandboxAccess: true,
		},
	}
}

// PostureProdGate creates the prod-gate posture:
// Agents handle development and staging autonomously. Production requires approval.
func PostureProdGate() PostureBundle {
	return PostureBundle{
		Name:        "prod-gate",
		DisplayName: "Production Gate",
		Description: "Agents operate autonomously in development and staging environments. " +
			"Production transitions always require human approval.",
		Rules: []Rule{
			{
				ID:          "prodgate-dev-auto",
				Name:        "dev-auto",
				Description: "Development environment transitions are automatic",
				Predicates: []Predicate{
					{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"development"}},
				},
				Action:  ActionAuto,
				Enabled: true,
			},
			{
				ID:          "prodgate-staging-auto",
				Name:        "staging-auto",
				Description: "Staging environment transitions are automatic",
				Predicates: []Predicate{
					{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"staging"}},
				},
				Action:  ActionAuto,
				Enabled: true,
			},
			{
				ID:          "prodgate-prod-approve",
				Name:        "prod-approve",
				Description: "Production environment transitions require approval",
				Predicates: []Predicate{
					{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"production"}},
				},
				Action:  ActionApprove,
				Enabled: true,
			},
			{
				ID:          "prodgate-prod-deploy-confirm",
				Name:        "prod-deploy-typed-confirm",
				Description: "Production deployments require typed confirmation",
				Predicates: []Predicate{
					{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"production"}},
					{Field: FieldTransition, Operator: OpEquals, Values: []string{"deploy"}},
				},
				Action:  ActionTypedConfirm,
				Enabled: true,
			},
		},
		CredentialScopes: CredentialScopes{
			CodeAccess:    CredReadWrite,
			DeployCreds:   CredReadWrite,
			InfraCreds:    CredReadWrite,
			SecretAccess:  CredReadOnly,
			SandboxAccess: true,
		},
	}
}

// PostureGraduatedRisk creates the graduated-risk posture:
// Safe/reversible operations proceed autonomously across all environments.
// Destructive/irreversible operations always prompt for confirmation.
func PostureGraduatedRisk() PostureBundle {
	return PostureBundle{
		Name:        "graduated-risk",
		DisplayName: "Graduated Risk",
		Description: "Safe and reversible operations proceed autonomously across all environments. " +
			"Destructive or irreversible operations always require confirmation, regardless of environment.",
		Rules: []Rule{
			{
				ID:          "gradrisk-safe-auto",
				Name:        "safe-reversible-auto",
				Description: "Safe, reversible operations proceed automatically",
				Predicates: []Predicate{
					{Field: FieldRiskLevel, Operator: OpIn, Values: []string{"low", "medium"}},
					{Field: FieldRiskReversible, Operator: OpEquals, Values: []string{"true"}},
				},
				Action:  ActionAuto,
				Enabled: true,
			},
			{
				ID:          "gradrisk-medium-notify",
				Name:        "medium-irreversible-notify",
				Description: "Medium-risk irreversible operations proceed with notification",
				Predicates: []Predicate{
					{Field: FieldRiskLevel, Operator: OpEquals, Values: []string{"medium"}},
					{Field: FieldRiskReversible, Operator: OpEquals, Values: []string{"false"}},
				},
				Action:  ActionNotify,
				Enabled: true,
			},
			{
				ID:          "gradrisk-high-approve",
				Name:        "high-risk-approve",
				Description: "High-risk operations require approval",
				Predicates: []Predicate{
					{Field: FieldRiskLevel, Operator: OpEquals, Values: []string{"high"}},
				},
				Action:  ActionApprove,
				Enabled: true,
			},
			{
				ID:          "gradrisk-critical-confirm",
				Name:        "critical-typed-confirm",
				Description: "Critical-risk operations require typed confirmation",
				Predicates: []Predicate{
					{Field: FieldRiskLevel, Operator: OpEquals, Values: []string{"critical"}},
				},
				Action:  ActionTypedConfirm,
				Enabled: true,
			},
			{
				ID:          "gradrisk-deploy-prod-approve",
				Name:        "deploy-prod-approve",
				Description: "Production deployments always require approval regardless of risk",
				Predicates: []Predicate{
					{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"production"}},
					{Field: FieldTransition, Operator: OpEquals, Values: []string{"deploy"}},
				},
				Action:  ActionApprove,
				Enabled: true,
			},
		},
		CredentialScopes: CredentialScopes{
			CodeAccess:    CredReadWrite,
			DeployCreds:   CredReadWrite,
			InfraCreds:    CredReadWrite,
			SecretAccess:  CredReadWrite,
			SandboxAccess: true,
		},
	}
}

// PostureParanoidService creates the paranoid-service posture:
// Specific services (configured by name) always require approval.
// All other services follow a moderate default (auto for dev/staging, approve for prod).
func PostureParanoidService() PostureBundle {
	return PostureBundle{
		Name:        "paranoid-service",
		DisplayName: "Paranoid Service",
		Description: "Specific critical services always require approval for any transition. " +
			"Other services follow environment-based defaults (auto for dev/staging, approve for production). " +
			"Customize the service predicates to match your critical services.",
		Rules: []Rule{
			{
				ID:          "paranoid-critical-svc",
				Name:        "critical-services-approve",
				Description: "Critical services always require approval (customize service names)",
				Predicates: []Predicate{
					{Field: FieldService, Operator: OpMatches, Values: []string{"*-prod", "payments*", "auth*", "billing*"}},
				},
				Action:  ActionApprove,
				Enabled: true,
			},
			{
				ID:          "paranoid-critical-svc-deploy",
				Name:        "critical-services-deploy-confirm",
				Description: "Critical service deployments require typed confirmation",
				Predicates: []Predicate{
					{Field: FieldService, Operator: OpMatches, Values: []string{"*-prod", "payments*", "auth*", "billing*"}},
					{Field: FieldTransition, Operator: OpEquals, Values: []string{"deploy"}},
				},
				Action:  ActionTypedConfirm,
				Enabled: true,
			},
			{
				ID:          "paranoid-dev-auto",
				Name:        "dev-staging-auto",
				Description: "Non-critical services in dev/staging are automatic",
				Predicates: []Predicate{
					{Field: FieldEnvironment, Operator: OpIn, Values: []string{"development", "staging"}},
				},
				Action:  ActionAuto,
				Enabled: true,
			},
			{
				ID:          "paranoid-prod-approve",
				Name:        "prod-approve",
				Description: "Production transitions for non-critical services require approval",
				Predicates: []Predicate{
					{Field: FieldEnvironment, Operator: OpEquals, Values: []string{"production"}},
				},
				Action:  ActionApprove,
				Enabled: true,
			},
		},
		CredentialScopes: CredentialScopes{
			CodeAccess:    CredReadWrite,
			DeployCreds:   CredReadWrite,
			InfraCreds:    CredReadWrite,
			SecretAccess:  CredReadOnly,
			SandboxAccess: true,
		},
	}
}
