package plan

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ScopeViolation represents an executor action that exceeds the declared plan scope.
// When a violation is detected, the executor must bounce to re-plan with re-classification.
type ScopeViolation struct {
	// Type classifies the violation: "file", "symbol", "operation".
	Type string `json:"type"`
	// Target is what was touched outside the plan (file path, symbol ID).
	Target string `json:"target"`
	// DeclaredScope summarizes what the plan declared.
	DeclaredScope string `json:"declared_scope"`
	// Description is a human-readable explanation.
	Description string `json:"description"`
}

func (v *ScopeViolation) Error() string {
	return fmt.Sprintf("scope violation [%s]: %s (target: %s, declared: %s)", v.Type, v.Description, v.Target, v.DeclaredScope)
}

// ScopeChecker validates that executor actions stay within the declared plan scope.
// Plan-as-contract: the executor is compared against declared scope, and exceeding
// scope bounces to re-plan with re-classification.
type ScopeChecker struct {
	// allowedFiles is the set of file paths the plan declares it will touch.
	allowedFiles map[string]string // path → operation_type
	// allowedSymbols is the set of symbol IDs the plan declares it will touch.
	allowedSymbols map[string]string // symbol_id → operation_type
	// allowedDiffFiles is the set of files with declared diffs.
	allowedDiffFiles map[string]bool
}

// NewScopeChecker creates a ScopeChecker from a CodePlan.
// The checker enforces that execution doesn't exceed declared targets.
func NewScopeChecker(plan *CodePlan) *ScopeChecker {
	sc := &ScopeChecker{
		allowedFiles:     make(map[string]string),
		allowedSymbols:   make(map[string]string),
		allowedDiffFiles: make(map[string]bool),
	}

	for _, te := range plan.TargetEntities {
		switch te.EntityType {
		case "file":
			sc.allowedFiles[te.ID] = te.OperationType
			if te.OperationType == "rename" && te.NewID != "" {
				sc.allowedFiles[te.NewID] = te.OperationType
			}
		case "symbol":
			sc.allowedSymbols[te.ID] = te.OperationType
			if te.OperationType == "rename" && te.NewID != "" {
				sc.allowedSymbols[te.NewID] = te.OperationType
			}
		case "service":
			// Service entities don't have file-level scope — they're for classification.
		}
	}

	for _, d := range plan.Diffs {
		sc.allowedDiffFiles[d.FilePath] = true
	}

	return sc
}

// CheckFileAccess verifies that a file access is within the plan's declared scope.
// Returns nil if allowed, ScopeViolation if the file was not declared.
func (sc *ScopeChecker) CheckFileAccess(filePath string) *ScopeViolation {
	// Normalize path for comparison.
	normalized := filepath.Clean(filePath)

	// Direct match.
	if _, ok := sc.allowedFiles[normalized]; ok {
		return nil
	}
	if _, ok := sc.allowedFiles[filePath]; ok {
		return nil
	}

	// Check diff files too (diffs may reference files not in entities for multi-file plans).
	if sc.allowedDiffFiles[normalized] || sc.allowedDiffFiles[filePath] {
		return nil
	}

	return &ScopeViolation{
		Type:          "file",
		Target:        filePath,
		DeclaredScope: formatAllowedFiles(sc.allowedFiles),
		Description:   fmt.Sprintf("file %q not declared in plan target entities — requires re-plan", filePath),
	}
}

// CheckSymbolAccess verifies that a symbol modification is within the plan's declared scope.
func (sc *ScopeChecker) CheckSymbolAccess(symbolID string) *ScopeViolation {
	if _, ok := sc.allowedSymbols[symbolID]; ok {
		return nil
	}

	return &ScopeViolation{
		Type:          "symbol",
		Target:        symbolID,
		DeclaredScope: formatAllowedSymbols(sc.allowedSymbols),
		Description:   fmt.Sprintf("symbol %q not declared in plan target entities — requires re-plan", symbolID),
	}
}

// CheckOperationType verifies that the operation on a file matches what was declared.
// e.g., if the plan says "modify" but the executor tries to "remove", that's a violation.
func (sc *ScopeChecker) CheckOperationType(filePath, actualOperation string) *ScopeViolation {
	declaredOp, ok := sc.allowedFiles[filePath]
	if !ok {
		declaredOp, ok = sc.allowedFiles[filepath.Clean(filePath)]
	}
	if !ok {
		// File not in plan — separate violation.
		return sc.CheckFileAccess(filePath)
	}

	// "modify" is compatible with "modify"; "add" with "add"; etc.
	// A "remove" operation on a file declared as "modify" is a violation.
	if !operationCompatible(declaredOp, actualOperation) {
		return &ScopeViolation{
			Type:          "operation",
			Target:        filePath,
			DeclaredScope: declaredOp,
			Description:   fmt.Sprintf("operation %q on %q exceeds declared operation %q — requires re-plan", actualOperation, filePath, declaredOp),
		}
	}

	return nil
}

// AllViolations runs a batch check against a set of file modifications and returns all violations.
// This is the main integration point for the executor: after execution, check all touched files.
func (sc *ScopeChecker) AllViolations(touchedFiles map[string]string) []ScopeViolation {
	var violations []ScopeViolation
	for filePath, operation := range touchedFiles {
		if v := sc.CheckFileAccess(filePath); v != nil {
			violations = append(violations, *v)
			continue
		}
		if v := sc.CheckOperationType(filePath, operation); v != nil {
			violations = append(violations, *v)
		}
	}
	return violations
}

// operationCompatible checks if the actual operation is compatible with the declared operation.
// Declared "modify" allows actual "modify". Declared "add" allows "add".
// "modify" also allows "add" (adding content to an existing concept).
// "remove" only allowed if declared "remove".
func operationCompatible(declared, actual string) bool {
	if declared == actual {
		return true
	}
	// "modify" is a superset that covers minor additions within the same file.
	if declared == "modify" && actual == "add" {
		return true
	}
	// "rename" involves both add and remove semantics.
	if declared == "rename" {
		return actual == "add" || actual == "remove" || actual == "modify"
	}
	return false
}

func formatAllowedFiles(files map[string]string) string {
	if len(files) == 0 {
		return "none declared"
	}
	var parts []string
	for path, op := range files {
		parts = append(parts, fmt.Sprintf("%s(%s)", path, op))
	}
	return strings.Join(parts, ", ")
}

func formatAllowedSymbols(symbols map[string]string) string {
	if len(symbols) == 0 {
		return "none declared"
	}
	var parts []string
	for sym, op := range symbols {
		parts = append(parts, fmt.Sprintf("%s(%s)", sym, op))
	}
	return strings.Join(parts, ", ")
}
