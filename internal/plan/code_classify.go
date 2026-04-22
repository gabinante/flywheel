package plan

import (
	"strings"

	"github.com/gabinante/flywheel/internal/risk"
)

// CodePlanClassification classifies a CodePlan through the risk classification pipeline.
// This bridges the plan layer's CodePlan schema with the risk layer's classifier.
//
// Classification rules via AST analysis:
//   - Safe: comments, tests, formatting, documentation
//   - Reversible (Low): new functions, new flags, internal changes
//   - Destructive (High): public API changes, auth code
//   - Irreversible: side-effect-producing code (external calls, data mutations)
//
// Unknown code patterns default to destructive.
// Environment multipliers and service sensitivity flags apply same as infra.
func ClassifyCodePlan(codePlan *CodePlan, environment string, serviceTags []string) risk.Classification {
	classifier := risk.NewClassifier()
	riskPlan := codeToRiskPlan(codePlan, environment, serviceTags)
	return classifier.Classify(riskPlan, nil)
}

// codeToRiskPlan converts a CodePlan into a risk.Plan for classification.
// Each target entity and diff hunk becomes a risk.Operation with scope-based details.
func codeToRiskPlan(codePlan *CodePlan, environment string, serviceTags []string) risk.Plan {
	var operations []risk.Operation

	// Convert target entities to risk operations.
	for _, te := range codePlan.TargetEntities {
		op := risk.Operation{
			Backend: risk.BackendCode,
			Action:  mapOperationType(te.OperationType),
			Target:  te.ID,
			Details: make(map[string]any),
		}

		// Infer scope from entity properties.
		if te.EntityType == "file" {
			op.Details["file_type"] = inferFileType(te.ID)
			op.Details["scope"] = inferFileScope(te.ID)
		}

		operations = append(operations, op)
	}

	// Convert symbol snapshots to more precise risk operations.
	for _, ss := range codePlan.SymbolSnapshots {
		scope := inferSymbolScope(ss)
		op := risk.Operation{
			Backend: risk.BackendCode,
			Action:  inferSymbolAction(ss),
			Target:  ss.FilePath,
			Details: map[string]any{
				"scope":       scope,
				"symbol_id":   ss.SymbolID,
				"symbol_kind": ss.Kind,
				"exported":    ss.Exported,
				"callers":     ss.Callers,
			},
		}
		operations = append(operations, op)
	}

	// Convert diffs with symbol scopes to operations.
	for _, diff := range codePlan.Diffs {
		for _, hunk := range diff.Hunks {
			if hunk.SymbolScope != "" {
				op := risk.Operation{
					Backend: risk.BackendCode,
					Action:  hunk.Operation,
					Target:  diff.FilePath,
					Details: map[string]any{
						"scope":        inferHunkScope(hunk.SymbolScope, diff.FilePath),
						"symbol_scope": hunk.SymbolScope,
					},
				}
				operations = append(operations, op)
			}
		}
	}

	// If no operations were generated (no entities, no snapshots, no scoped hunks),
	// create a default operation from the diffs.
	if len(operations) == 0 {
		for _, diff := range codePlan.Diffs {
			op := risk.Operation{
				Backend: risk.BackendCode,
				Action:  "edit",
				Target:  diff.FilePath,
				Details: map[string]any{
					"scope": inferFileScope(diff.FilePath),
				},
			}
			operations = append(operations, op)
		}
	}

	return risk.Plan{
		Operations:  operations,
		Environment: environment,
		ServiceTags: serviceTags,
	}
}

// mapOperationType maps plan operation types to risk classifier actions.
func mapOperationType(opType string) string {
	switch opType {
	case "add":
		return "add"
	case "modify":
		return "edit"
	case "remove":
		return "delete"
	case "rename":
		return "rename"
	default:
		return "edit"
	}
}

// inferFileType returns the file type/extension category.
func inferFileType(filePath string) string {
	lower := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return "go"
	case strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".tsx"):
		return "typescript"
	case strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".jsx"):
		return "javascript"
	case strings.HasSuffix(lower, ".py"):
		return "python"
	case strings.HasSuffix(lower, ".rs"):
		return "rust"
	case strings.HasSuffix(lower, ".java"):
		return "java"
	case strings.HasSuffix(lower, ".md"):
		return "markdown"
	case strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml"):
		return "yaml"
	case strings.HasSuffix(lower, ".json"):
		return "json"
	default:
		return "unknown"
	}
}

// inferFileScope determines the risk scope from file path patterns.
// Maps to the scope values the CodeClassifier in risk/ expects.
func inferFileScope(filePath string) string {
	lower := strings.ToLower(filePath)

	// Test files → safe.
	testPatterns := []string{
		"_test.go", "_test.ts", "_test.js", "_test.py",
		".test.ts", ".test.js", ".test.tsx", ".test.jsx",
		".spec.ts", ".spec.js",
		"__tests__/", "test/", "tests/", "testdata/", "fixtures/",
	}
	for _, p := range testPatterns {
		if strings.Contains(lower, p) {
			return "test"
		}
	}

	// Documentation → safe.
	docPatterns := []string{".md", "readme", "changelog", "license", "docs/", ".txt"}
	for _, p := range docPatterns {
		if strings.Contains(lower, p) {
			return "documentation"
		}
	}

	// Security-sensitive → destructive.
	securityPatterns := []string{
		"auth", "crypto", "permission", "acl", "rbac", "oauth",
		"jwt", "token", "secret", "credential", "password",
		"encrypt", "decrypt", "signing", "certificate",
	}
	for _, p := range securityPatterns {
		if strings.Contains(lower, p) {
			return "security"
		}
	}

	// Startup/main paths → high.
	startupPatterns := []string{
		"main.go", "main.ts", "main.py", "cmd/", "init.go",
		"bootstrap", "startup", "entrypoint", "app.go", "server.go",
	}
	for _, p := range startupPatterns {
		if strings.Contains(lower, p) {
			return "startup"
		}
	}

	// Internal/private → low.
	internalPatterns := []string{"internal/", "private/", "pkg/"}
	for _, p := range internalPatterns {
		if strings.Contains(lower, p) {
			return "internal"
		}
	}

	return ""
}

// inferSymbolScope determines the risk scope from a symbol snapshot.
func inferSymbolScope(ss SymbolSnapshot) string {
	// Exported symbol changes are public API — high risk.
	if ss.Exported {
		// Signature changes to exported symbols with many callers are destructive.
		if ss.BeforeSignature != "" && ss.AfterSignature != "" &&
			ss.BeforeSignature != ss.AfterSignature && ss.Callers > 5 {
			return "public_api"
		}
		if ss.Exported {
			return "exported"
		}
	}

	// Security-related symbols.
	lowerID := strings.ToLower(ss.SymbolID)
	for _, p := range []string{"auth", "crypto", "token", "secret", "password", "permission"} {
		if strings.Contains(lowerID, p) {
			return "security"
		}
	}

	// Private/internal symbols — low risk.
	return "internal"
}

// inferSymbolAction determines the operation from before/after signatures.
func inferSymbolAction(ss SymbolSnapshot) string {
	if ss.BeforeSignature == "" && ss.AfterSignature != "" {
		return "add"
	}
	if ss.BeforeSignature != "" && ss.AfterSignature == "" {
		return "delete"
	}
	return "edit"
}

// inferHunkScope maps hunk symbol_scope annotations to risk classifier scope values.
func inferHunkScope(symbolScope, filePath string) string {
	lower := strings.ToLower(symbolScope)

	// Direct scope annotations from Tree-sitter AST analysis.
	if strings.HasPrefix(lower, "comment") || strings.HasPrefix(lower, "doc") {
		return "comment"
	}
	if strings.HasPrefix(lower, "test") {
		return "test"
	}

	// Security-related symbol scopes.
	for _, p := range []string{"auth", "crypto", "token", "secret"} {
		if strings.Contains(lower, p) {
			return "security"
		}
	}

	// Fall back to file-level scope inference.
	if scope := inferFileScope(filePath); scope != "" {
		return scope
	}

	return "internal"
}

// TreeSitterLanguages is the set of languages supported by Tree-sitter in v1.
// Unsupported languages classify as unknown (destructive default per constraints).
var TreeSitterLanguages = map[string]bool{
	"go":         true,
	"typescript": true,
	"javascript": true,
	"python":     true,
	"rust":       true,
	"java":       true,
	"c":          true,
	"cpp":        true,
	"ruby":       true,
	"bash":       true,
	"json":       true,
	"yaml":       true,
	"toml":       true,
	"html":       true,
	"css":        true,
}

// IsTreeSitterSupported returns true if the language has Tree-sitter support.
// Unsupported languages classify as unknown → destructive default.
func IsTreeSitterSupported(language string) bool {
	return TreeSitterLanguages[strings.ToLower(language)]
}

// CodeAutoAdvanceEligible checks if a code plan is eligible for auto-advance
// (safe classification under default policy). This makes trivial changes feel
// lightweight — the "no fast path" principle works because auto-advance is invisible.
func CodeAutoAdvanceEligible(codePlan *CodePlan, environment string, serviceTags []string) bool {
	classification := ClassifyCodePlan(codePlan, environment, serviceTags)
	result := risk.Evaluate(risk.NewClassifier(), codeToRiskPlan(codePlan, environment, serviceTags), nil, nil)
	_ = classification // classification is captured in result
	return result.Allowed
}
