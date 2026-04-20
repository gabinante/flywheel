package risk

import "strings"

// CodeClassifier classifies code changes by AST-level analysis of what is being modified.
//
// Classification rules:
//   - Comments, documentation, test files → safe
//   - Test code changes, fixture updates → safe
//   - Internal/private function bodies → low
//   - Config files, dependency updates → reversible
//   - Public API signatures, exported types → reversible+/high
//   - Startup paths, initialization code, main → high
//   - Security-sensitive code (auth, crypto, permissions) → destructive
//
// The classifier inspects:
//   - Operation.Action: "add", "edit", "delete", "rename", "move"
//   - Operation.Target: file path
//   - Operation.Details["scope"]: "comment", "test", "internal", "public_api", "startup", "security"
//   - Operation.Details["file_type"]: file extension or category
type CodeClassifier struct{}

// safeFilePatterns are file paths/patterns that are always safe to change.
var safeFilePatterns = []string{
	"_test.go",
	"_test.ts",
	"_test.js",
	"_test.py",
	".test.ts",
	".test.js",
	".test.tsx",
	".test.jsx",
	".spec.ts",
	".spec.js",
	"__tests__/",
	"test/",
	"tests/",
	"testdata/",
	"fixtures/",
	".md",
	"README",
	"CHANGELOG",
	"LICENSE",
	"docs/",
	".txt",
}

// securityPatterns are file paths/scopes that indicate security-sensitive code.
var securityPatterns = []string{
	"auth",
	"crypto",
	"permission",
	"acl",
	"rbac",
	"oauth",
	"jwt",
	"token",
	"secret",
	"credential",
	"password",
	"encrypt",
	"decrypt",
	"signing",
	"certificate",
}

// startupPatterns indicate initialization/startup code.
var startupPatterns = []string{
	"main.go",
	"main.ts",
	"main.py",
	"cmd/",
	"init.go",
	"bootstrap",
	"startup",
	"entrypoint",
	"app.go",
	"server.go",
}

// Classify classifies a code change operation.
func (c *CodeClassifier) Classify(op Operation) (RiskLevel, []RuleCitation) {
	action := strings.ToLower(strings.TrimSpace(op.Action))
	target := strings.ToLower(op.Target)
	scope := ""
	if s, ok := op.Details["scope"].(string); ok {
		scope = strings.ToLower(s)
	}

	// Explicit scope overrides from AST analysis (e.g. Tree-sitter output).
	if scope != "" {
		return classifyByScope(scope, op)
	}

	// Delete is always at least reversible; check for high-risk paths.
	if action == "delete" || action == "remove" {
		if matchesAnyPattern(target, securityPatterns) {
			return RiskDestructive, []RuleCitation{{
				Rule:        "code.delete_security",
				Backend:     BackendCode,
				Level:       RiskDestructive,
				Description: "Deleting security-sensitive code '" + op.Target + "' — requires careful review",
			}}
		}
		if matchesAnyPattern(target, startupPatterns) {
			return RiskHigh, []RuleCitation{{
				Rule:        "code.delete_startup",
				Backend:     BackendCode,
				Level:       RiskHigh,
				Description: "Deleting startup/initialization code '" + op.Target + "'",
			}}
		}
		if matchesAnyPattern(target, safeFilePatterns) {
			return RiskSafe, []RuleCitation{{
				Rule:        "code.delete_test_or_docs",
				Backend:     BackendCode,
				Level:       RiskSafe,
				Description: "Deleting test or documentation file '" + op.Target + "'",
			}}
		}
		return RiskReversible, []RuleCitation{{
			Rule:        "code.delete_file",
			Backend:     BackendCode,
			Level:       RiskReversible,
			Description: "Deleting code file '" + op.Target + "' — reversible via version control",
		}}
	}

	// Safe file patterns (tests, docs, comments).
	if matchesAnyPattern(target, safeFilePatterns) {
		return RiskSafe, []RuleCitation{{
			Rule:        "code.test_or_docs",
			Backend:     BackendCode,
			Level:       RiskSafe,
			Description: "Change to test or documentation file '" + op.Target + "'",
		}}
	}

	// Security-sensitive paths.
	if matchesAnyPattern(target, securityPatterns) {
		return RiskDestructive, []RuleCitation{{
			Rule:        "code.security_sensitive",
			Backend:     BackendCode,
			Level:       RiskDestructive,
			Description: "Change to security-sensitive code '" + op.Target + "' — requires careful review",
		}}
	}

	// Startup/initialization paths.
	if matchesAnyPattern(target, startupPatterns) {
		return RiskHigh, []RuleCitation{{
			Rule:        "code.startup_path",
			Backend:     BackendCode,
			Level:       RiskHigh,
			Description: "Change to startup/initialization code '" + op.Target + "' — affects service availability",
		}}
	}

	// Add action (new files) is generally low risk.
	if action == "add" || action == "create" {
		return RiskLow, []RuleCitation{{
			Rule:        "code.add_file",
			Backend:     BackendCode,
			Level:       RiskLow,
			Description: "Adding new code file '" + op.Target + "' — additive change",
		}}
	}

	// Default for edits to non-categorized files.
	return RiskReversible, []RuleCitation{{
		Rule:        "code.edit_file",
		Backend:     BackendCode,
		Level:       RiskReversible,
		Description: "Editing code file '" + op.Target + "' — reversible via version control",
	}}
}

// classifyByScope uses explicit AST scope annotations.
func classifyByScope(scope string, op Operation) (RiskLevel, []RuleCitation) {
	switch scope {
	case "comment", "documentation", "doc":
		return RiskSafe, []RuleCitation{{
			Rule:        "code.scope_comment",
			Backend:     BackendCode,
			Level:       RiskSafe,
			Description: "Change scoped to comments/documentation in '" + op.Target + "'",
		}}
	case "test", "test_fixture":
		return RiskSafe, []RuleCitation{{
			Rule:        "code.scope_test",
			Backend:     BackendCode,
			Level:       RiskSafe,
			Description: "Change scoped to test code in '" + op.Target + "'",
		}}
	case "internal", "private":
		return RiskLow, []RuleCitation{{
			Rule:        "code.scope_internal",
			Backend:     BackendCode,
			Level:       RiskLow,
			Description: "Change scoped to internal/private code in '" + op.Target + "'",
		}}
	case "public_api", "exported":
		return RiskHigh, []RuleCitation{{
			Rule:        "code.scope_public_api",
			Backend:     BackendCode,
			Level:       RiskHigh,
			Description: "Change to public API/exported symbols in '" + op.Target + "' — may break consumers",
		}}
	case "startup", "initialization", "main":
		return RiskHigh, []RuleCitation{{
			Rule:        "code.scope_startup",
			Backend:     BackendCode,
			Level:       RiskHigh,
			Description: "Change to startup/initialization path in '" + op.Target + "'",
		}}
	case "security", "auth", "crypto":
		return RiskDestructive, []RuleCitation{{
			Rule:        "code.scope_security",
			Backend:     BackendCode,
			Level:       RiskDestructive,
			Description: "Change to security-sensitive scope in '" + op.Target + "'",
		}}
	default:
		return RiskReversible, []RuleCitation{{
			Rule:        "code.scope_unknown",
			Backend:     BackendCode,
			Level:       RiskReversible,
			Description: "Change with unknown scope '" + scope + "' in '" + op.Target + "'",
		}}
	}
}

// matchesAnyPattern checks if target contains any of the given patterns.
func matchesAnyPattern(target string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(target, p) {
			return true
		}
	}
	return false
}
