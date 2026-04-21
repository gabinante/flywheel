package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationError represents a plan content validation failure.
// This is returned at submission time — before classification.
type ValidationError struct {
	Backend Backend
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("plan validation failed [%s.%s]: %s", e.Backend, e.Field, e.Message)
}

// ValidateContent validates plan content against the backend's typed sub-schema.
// Content validation happens at schema level before classification — a plan whose
// DDL field does not parse as valid SQL fails at submission, not at classification.
// Returns nil if valid.
func ValidateContent(backend Backend, content Content) error {
	switch backend {
	case BackendDatabase:
		return validateDatabase(content.Database)
	case BackendTerraform:
		return validateTerraform(content.Terraform)
	case BackendCode:
		return validateCode(content.Code)
	case BackendShell:
		return validateShell(content.Shell)
	case BackendDeploy:
		return validateDeploy(content.Deploy)
	default:
		return fmt.Errorf("unknown backend: %s", backend)
	}
}

// ValidateContentJSON validates raw JSON content for the given backend.
// Used for pre-persist validation when content comes as raw JSON bytes.
func ValidateContentJSON(backend Backend, raw json.RawMessage) error {
	var content Content
	switch backend {
	case BackendDatabase:
		var db DatabasePlan
		if err := json.Unmarshal(raw, &db); err != nil {
			return &ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for database plan: " + err.Error()}
		}
		content.Database = &db
	case BackendTerraform:
		var tf TerraformPlan
		if err := json.Unmarshal(raw, &tf); err != nil {
			return &ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for terraform plan: " + err.Error()}
		}
		content.Terraform = &tf
	case BackendCode:
		var code CodePlan
		if err := json.Unmarshal(raw, &code); err != nil {
			return &ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for code plan: " + err.Error()}
		}
		content.Code = &code
	case BackendShell:
		var shell ShellPlan
		if err := json.Unmarshal(raw, &shell); err != nil {
			return &ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for shell plan: " + err.Error()}
		}
		content.Shell = &shell
	case BackendDeploy:
		var deploy DeployPlan
		if err := json.Unmarshal(raw, &deploy); err != nil {
			return &ValidationError{Backend: backend, Field: "content", Message: "invalid JSON for deploy plan: " + err.Error()}
		}
		content.Deploy = &deploy
	default:
		return fmt.Errorf("unknown backend: %s", backend)
	}
	return ValidateContent(backend, content)
}

func validateDatabase(db *DatabasePlan) error {
	if db == nil {
		return &ValidationError{Backend: BackendDatabase, Field: "database", Message: "database plan content required"}
	}
	if strings.TrimSpace(db.MigrationName) == "" {
		return &ValidationError{Backend: BackendDatabase, Field: "migration_name", Message: "migration_name is required"}
	}
	if strings.TrimSpace(db.DDL) == "" {
		return &ValidationError{Backend: BackendDatabase, Field: "ddl", Message: "ddl is required"}
	}
	// Validate DDL is parseable SQL — basic structural check.
	// We check for common DDL statement prefixes to ensure it's actual SQL, not prose.
	if err := validateDDLSyntax(db.DDL); err != nil {
		return &ValidationError{Backend: BackendDatabase, Field: "ddl", Message: err.Error()}
	}
	if db.Direction != "up" && db.Direction != "down" {
		return &ValidationError{Backend: BackendDatabase, Field: "direction", Message: "direction must be 'up' or 'down'"}
	}
	// Validate rollback DDL if present
	if db.RollbackDDL != "" {
		if err := validateDDLSyntax(db.RollbackDDL); err != nil {
			return &ValidationError{Backend: BackendDatabase, Field: "rollback_ddl", Message: err.Error()}
		}
	}
	return nil
}

// validateDDLSyntax performs structural validation of DDL content.
// Ensures DDL contains valid SQL statements, not arbitrary prose.
func validateDDLSyntax(ddl string) error {
	trimmed := strings.TrimSpace(ddl)
	if trimmed == "" {
		return fmt.Errorf("DDL cannot be empty")
	}

	// Split on semicolons to check individual statements
	statements := splitStatements(trimmed)
	if len(statements) == 0 {
		return fmt.Errorf("no valid SQL statements found")
	}

	// Valid DDL statement prefixes
	validPrefixes := []string{
		"CREATE", "ALTER", "DROP", "TRUNCATE", "RENAME",
		"GRANT", "REVOKE", "COMMENT", "SET", "INSERT",
		"UPDATE", "DELETE", "SELECT", "WITH", "BEGIN",
		"COMMIT", "ROLLBACK", "DO", "EXPLAIN",
		"--", "/*", // Allow SQL comments
	}

	for _, stmt := range statements {
		upper := strings.ToUpper(strings.TrimSpace(stmt))
		if upper == "" {
			continue
		}
		valid := false
		for _, prefix := range validPrefixes {
			if strings.HasPrefix(upper, prefix) {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("statement does not appear to be valid DDL (must start with a SQL keyword): %.50s", stmt)
		}
	}
	return nil
}

// splitStatements splits SQL on semicolons, respecting basic quoting.
func splitStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inDollarQuote := false
	dollarTag := ""

	runes := []rune(sql)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// Handle dollar quoting (PostgreSQL)
		if ch == '$' && !inSingleQuote && !inDoubleQuote {
			// Check for dollar-quoted string start/end
			end := strings.IndexRune(string(runes[i+1:]), '$')
			if end >= 0 {
				tag := "$" + string(runes[i+1:i+1+end]) + "$"
				if inDollarQuote && tag == dollarTag {
					inDollarQuote = false
					current.WriteString(tag)
					i += end + 1
					continue
				} else if !inDollarQuote {
					inDollarQuote = true
					dollarTag = tag
					current.WriteString(tag)
					i += end + 1
					continue
				}
			}
		}

		if inDollarQuote {
			current.WriteRune(ch)
			continue
		}

		switch ch {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
			current.WriteRune(ch)
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
			current.WriteRune(ch)
		case ';':
			if !inSingleQuote && !inDoubleQuote {
				stmt := strings.TrimSpace(current.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Don't forget trailing statement without semicolon
	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

func validateTerraform(tf *TerraformPlan) error {
	if tf == nil {
		return &ValidationError{Backend: BackendTerraform, Field: "terraform", Message: "terraform plan content required"}
	}
	if strings.TrimSpace(tf.PlanJSON) == "" {
		return &ValidationError{Backend: BackendTerraform, Field: "plan_json", Message: "plan_json is required"}
	}
	// Validate that plan_json is valid JSON
	if !json.Valid([]byte(tf.PlanJSON)) {
		return &ValidationError{Backend: BackendTerraform, Field: "plan_json", Message: "plan_json must be valid JSON"}
	}
	if strings.TrimSpace(tf.Provider) == "" {
		return &ValidationError{Backend: BackendTerraform, Field: "provider", Message: "provider is required"}
	}
	// Validate resource_changes have required fields
	for i, rc := range tf.ResourceChanges {
		if rc.Address == "" {
			return &ValidationError{Backend: BackendTerraform, Field: fmt.Sprintf("resource_changes[%d].address", i), Message: "address is required"}
		}
		if rc.ChangeAction == "" {
			return &ValidationError{Backend: BackendTerraform, Field: fmt.Sprintf("resource_changes[%d].change_action", i), Message: "change_action is required"}
		}
		validActions := []string{"create", "update", "delete", "replace", "no-op"}
		valid := false
		for _, a := range validActions {
			if rc.ChangeAction == a {
				valid = true
				break
			}
		}
		if !valid {
			return &ValidationError{Backend: BackendTerraform, Field: fmt.Sprintf("resource_changes[%d].change_action", i), Message: "change_action must be one of: create, update, delete, replace, no-op"}
		}
	}
	return nil
}

func validateCode(code *CodePlan) error {
	if code == nil {
		return &ValidationError{Backend: BackendCode, Field: "code", Message: "code plan content required"}
	}
	if strings.TrimSpace(code.Language) == "" {
		return &ValidationError{Backend: BackendCode, Field: "language", Message: "language is required"}
	}

	// Require at least one target entity — no "stealth" changes.
	if len(code.TargetEntities) == 0 {
		return &ValidationError{Backend: BackendCode, Field: "target_entities", Message: "at least one target entity is required"}
	}
	for i, te := range code.TargetEntities {
		if err := validateTargetEntity(i, te); err != nil {
			return err
		}
	}

	// Require at least one diff — the structured content that gets classified.
	if len(code.Diffs) == 0 {
		return &ValidationError{Backend: BackendCode, Field: "diffs", Message: "at least one diff is required"}
	}
	for i, d := range code.Diffs {
		if err := validateCodeDiff(i, d); err != nil {
			return err
		}
	}

	// Validate symbol snapshots if present.
	for i, ss := range code.SymbolSnapshots {
		if err := validateSymbolSnapshot(i, ss); err != nil {
			return err
		}
	}

	// Validate test expectations if present.
	if code.TestExpectations != nil {
		if err := validateTestExpectations(code.TestExpectations); err != nil {
			return err
		}
	}

	// Validate success contract if present.
	if code.SuccessContract != nil {
		if err := validateSuccessContract(code.SuccessContract); err != nil {
			return err
		}
	}

	// Validate rollback plan if present.
	if code.RollbackPlan != nil {
		if err := validateCodeRollbackPlan(code.RollbackPlan); err != nil {
			return err
		}
	}

	// Validate git context if present.
	if code.GitContext != nil {
		if err := validateGitContext(code.GitContext); err != nil {
			return err
		}
	}

	return nil
}

func validateTargetEntity(idx int, te TargetEntity) error {
	prefix := fmt.Sprintf("target_entities[%d]", idx)
	if strings.TrimSpace(te.ID) == "" {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".id", Message: "entity ID is required"}
	}
	if !isValidStringIn(te.EntityType, ValidEntityTypes) {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".entity_type", Message: fmt.Sprintf("entity_type must be one of: %s", strings.Join(ValidEntityTypes, ", "))}
	}
	if !isValidStringIn(te.OperationType, ValidOperationTypes) {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".operation_type", Message: fmt.Sprintf("operation_type must be one of: %s", strings.Join(ValidOperationTypes, ", "))}
	}
	if te.OperationType == "rename" && strings.TrimSpace(te.NewID) == "" {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".new_id", Message: "new_id is required when operation_type is 'rename'"}
	}
	return nil
}

func validateCodeDiff(idx int, d CodeDiff) error {
	prefix := fmt.Sprintf("diffs[%d]", idx)
	if strings.TrimSpace(d.FilePath) == "" {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".file_path", Message: "file_path is required"}
	}
	if len(d.Hunks) == 0 {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".hunks", Message: "at least one hunk is required"}
	}
	for i, h := range d.Hunks {
		hunkPrefix := fmt.Sprintf("%s.hunks[%d]", prefix, i)
		if h.Operation == "" {
			return &ValidationError{Backend: BackendCode, Field: hunkPrefix + ".operation", Message: "operation is required"}
		}
		validOps := []string{"add", "remove", "modify"}
		if !isValidStringIn(h.Operation, validOps) {
			return &ValidationError{Backend: BackendCode, Field: hunkPrefix + ".operation", Message: "operation must be one of: add, remove, modify"}
		}
		if h.Content == "" {
			return &ValidationError{Backend: BackendCode, Field: hunkPrefix + ".content", Message: "content is required"}
		}
	}
	return nil
}

func validateSymbolSnapshot(idx int, ss SymbolSnapshot) error {
	prefix := fmt.Sprintf("symbol_snapshots[%d]", idx)
	if strings.TrimSpace(ss.SymbolID) == "" {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".symbol_id", Message: "symbol_id is required"}
	}
	if strings.TrimSpace(ss.FilePath) == "" {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".file_path", Message: "file_path is required"}
	}
	if strings.TrimSpace(ss.Kind) == "" {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".kind", Message: "kind is required"}
	}
	if !isValidStringIn(ss.Kind, ValidSymbolKinds) {
		return &ValidationError{Backend: BackendCode, Field: prefix + ".kind", Message: fmt.Sprintf("kind must be one of: %s", strings.Join(ValidSymbolKinds, ", "))}
	}
	// At least one of before/after signature must be present (new or removed symbols omit one).
	if ss.BeforeSignature == "" && ss.AfterSignature == "" {
		return &ValidationError{Backend: BackendCode, Field: prefix, Message: "at least one of before_signature or after_signature is required"}
	}
	return nil
}

func validateTestExpectations(te *TestExpectations) error {
	if len(te.TestCommands) == 0 {
		return &ValidationError{Backend: BackendCode, Field: "test_expectations.test_commands", Message: "at least one test command is required"}
	}
	for i, cmd := range te.TestCommands {
		if strings.TrimSpace(cmd) == "" {
			return &ValidationError{Backend: BackendCode, Field: fmt.Sprintf("test_expectations.test_commands[%d]", i), Message: "test command cannot be empty"}
		}
	}
	if te.MinCoverage < 0 || te.MinCoverage > 100 {
		return &ValidationError{Backend: BackendCode, Field: "test_expectations.min_coverage", Message: "min_coverage must be between 0 and 100"}
	}
	return nil
}

func validateSuccessContract(sc *SuccessContract) error {
	if len(sc.Checks) == 0 {
		return &ValidationError{Backend: BackendCode, Field: "success_contract.checks", Message: "at least one success check is required"}
	}
	for i, check := range sc.Checks {
		prefix := fmt.Sprintf("success_contract.checks[%d]", i)
		if strings.TrimSpace(check.Name) == "" {
			return &ValidationError{Backend: BackendCode, Field: prefix + ".name", Message: "check name is required"}
		}
		if strings.TrimSpace(check.Command) == "" {
			return &ValidationError{Backend: BackendCode, Field: prefix + ".command", Message: "check command is required"}
		}
	}
	return nil
}

func validateCodeRollbackPlan(rp *CodeRollbackPlan) error {
	if strings.TrimSpace(rp.Strategy) == "" {
		return &ValidationError{Backend: BackendCode, Field: "rollback_plan.strategy", Message: "strategy is required"}
	}
	if !isValidStringIn(rp.Strategy, ValidRollbackStrategies) {
		return &ValidationError{Backend: BackendCode, Field: "rollback_plan.strategy", Message: fmt.Sprintf("strategy must be one of: %s", strings.Join(ValidRollbackStrategies, ", "))}
	}
	if rp.Strategy == "git_revert" || rp.Strategy == "git_reset" {
		if strings.TrimSpace(rp.RevertCommitRef) == "" {
			return &ValidationError{Backend: BackendCode, Field: "rollback_plan.revert_commit_ref", Message: "revert_commit_ref is required for git-based rollback strategies"}
		}
	}
	if rp.Strategy == "manual" && len(rp.ManualSteps) == 0 {
		return &ValidationError{Backend: BackendCode, Field: "rollback_plan.manual_steps", Message: "manual_steps are required for manual rollback strategy"}
	}
	return nil
}

func validateGitContext(gc *GitContext) error {
	if strings.TrimSpace(gc.Branch) == "" {
		return &ValidationError{Backend: BackendCode, Field: "git_context.branch", Message: "branch is required"}
	}
	return nil
}

// isValidStringIn checks if s is one of the allowed values.
func isValidStringIn(s string, allowed []string) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

func validateShell(shell *ShellPlan) error {
	if shell == nil {
		return &ValidationError{Backend: BackendShell, Field: "shell", Message: "shell plan content required"}
	}
	if len(shell.Commands) == 0 {
		return &ValidationError{Backend: BackendShell, Field: "commands", Message: "at least one command is required"}
	}
	for i, cmd := range shell.Commands {
		if strings.TrimSpace(cmd.Command) == "" {
			return &ValidationError{Backend: BackendShell, Field: fmt.Sprintf("commands[%d].command", i), Message: "command is required"}
		}
	}
	// Validate the side-effect manifest at schema level before classification.
	if err := validateManifest(&shell.SideEffectManifest); err != nil {
		return err
	}
	return nil
}

// validateManifest validates the side-effect manifest structure and field values.
// Manifest validation happens at schema level before classification.
func validateManifest(m *SideEffectManifest) error {
	// Validate file ops.
	for i, op := range m.FileOps {
		if strings.TrimSpace(op.Path) == "" {
			return &ValidationError{Backend: BackendShell, Field: fmt.Sprintf("side_effect_manifest.file_ops[%d].path", i), Message: "path is required"}
		}
		if !isValidFileOpAction(op.Action) {
			return &ValidationError{
				Backend: BackendShell,
				Field:   fmt.Sprintf("side_effect_manifest.file_ops[%d].action", i),
				Message: fmt.Sprintf("action must be one of: %s", strings.Join(ValidFileOpActions, ", ")),
			}
		}
	}
	// Validate network ops.
	for i, op := range m.NetworkOps {
		if strings.TrimSpace(op.Endpoint) == "" {
			return &ValidationError{Backend: BackendShell, Field: fmt.Sprintf("side_effect_manifest.network_ops[%d].endpoint", i), Message: "endpoint is required"}
		}
		if strings.TrimSpace(op.Method) == "" {
			return &ValidationError{Backend: BackendShell, Field: fmt.Sprintf("side_effect_manifest.network_ops[%d].method", i), Message: "method is required"}
		}
		if !isValidNetworkMethod(op.Method) {
			return &ValidationError{
				Backend: BackendShell,
				Field:   fmt.Sprintf("side_effect_manifest.network_ops[%d].method", i),
				Message: fmt.Sprintf("method must be one of: %s", strings.Join(ValidNetworkMethods, ", ")),
			}
		}
	}
	// Validate process ops.
	for i, op := range m.ProcessOps {
		if strings.TrimSpace(op.Binary) == "" {
			return &ValidationError{Backend: BackendShell, Field: fmt.Sprintf("side_effect_manifest.process_ops[%d].binary", i), Message: "binary is required"}
		}
	}
	// Validate credential ops.
	for i, op := range m.CredentialOps {
		if strings.TrimSpace(op.Name) == "" {
			return &ValidationError{Backend: BackendShell, Field: fmt.Sprintf("side_effect_manifest.credential_ops[%d].name", i), Message: "name is required"}
		}
	}
	// Validate resource limits (non-negative).
	if m.ResourceLimits.MaxRuntimeSeconds < 0 {
		return &ValidationError{Backend: BackendShell, Field: "side_effect_manifest.resource_limits.max_runtime_seconds", Message: "must be non-negative"}
	}
	if m.ResourceLimits.MaxDiskWriteBytes < 0 {
		return &ValidationError{Backend: BackendShell, Field: "side_effect_manifest.resource_limits.max_disk_write_bytes", Message: "must be non-negative"}
	}
	if m.ResourceLimits.MaxNetworkCalls < 0 {
		return &ValidationError{Backend: BackendShell, Field: "side_effect_manifest.resource_limits.max_network_calls", Message: "must be non-negative"}
	}
	return nil
}

func isValidFileOpAction(action string) bool {
	for _, valid := range ValidFileOpActions {
		if action == valid {
			return true
		}
	}
	return false
}

func isValidNetworkMethod(method string) bool {
	for _, valid := range ValidNetworkMethods {
		if method == valid {
			return true
		}
	}
	return false
}

func validateDeploy(deploy *DeployPlan) error {
	if deploy == nil {
		return &ValidationError{Backend: BackendDeploy, Field: "deploy", Message: "deploy plan content required"}
	}
	if strings.TrimSpace(deploy.ArtifactHash) == "" {
		return &ValidationError{Backend: BackendDeploy, Field: "artifact_hash", Message: "artifact_hash is required"}
	}
	if strings.TrimSpace(deploy.Target) == "" {
		return &ValidationError{Backend: BackendDeploy, Field: "target", Message: "target is required"}
	}
	if strings.TrimSpace(deploy.DeployStrategy) == "" {
		return &ValidationError{Backend: BackendDeploy, Field: "deploy_strategy", Message: "deploy_strategy is required"}
	}
	validStrategies := []string{"rolling", "blue-green", "canary", "recreate"}
	valid := false
	for _, s := range validStrategies {
		if deploy.DeployStrategy == s {
			valid = true
			break
		}
	}
	if !valid {
		return &ValidationError{Backend: BackendDeploy, Field: "deploy_strategy", Message: "deploy_strategy must be one of: rolling, blue-green, canary, recreate"}
	}
	return nil
}
