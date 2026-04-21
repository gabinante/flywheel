// Package plan implements the plan entity — a first-class, typed execution plan
// linked to tickets. Each plan carries a backend-specific sub-schema that constrains
// what agents can express. There is no untyped escape hatch.
package plan

import "time"

// Backend identifies the execution backend a plan targets.
// Each backend has its own typed sub-schema; backends evolve independently.
type Backend string

const (
	BackendDatabase  Backend = "database"
	BackendTerraform Backend = "terraform"
	BackendCode      Backend = "code"
	BackendShell     Backend = "shell"
	BackendDeploy    Backend = "deploy"
)

// AllBackends returns the complete set of valid backends.
func AllBackends() []Backend {
	return []Backend{BackendDatabase, BackendTerraform, BackendCode, BackendShell, BackendDeploy}
}

// IsValidBackend returns true if b is a known backend type.
func IsValidBackend(b Backend) bool {
	for _, valid := range AllBackends() {
		if b == valid {
			return true
		}
	}
	return false
}

// State represents a plan's lifecycle state.
type State string

const (
	StateDraft      State = "draft"
	StateSubmitted  State = "submitted"
	StateClassified State = "classified"
	StateApproved   State = "approved"
	StateApplied    State = "applied"
	StateSuperseded State = "superseded"
	StateRejected   State = "rejected"
)

// AllStates returns all valid plan states.
func AllStates() []State {
	return []State{StateDraft, StateSubmitted, StateClassified, StateApproved, StateApplied, StateSuperseded, StateRejected}
}

// IsValidState returns true if s is a known plan state.
func IsValidState(s State) bool {
	for _, valid := range AllStates() {
		if s == valid {
			return true
		}
	}
	return false
}

// Plan is the core entity representing a typed execution plan.
type Plan struct {
	ID             string    `json:"id"`
	TicketID       string    `json:"ticket_id"`
	Backend        Backend   `json:"backend"`
	State          State     `json:"state"`
	Version        int       `json:"version"`
	Content        Content   `json:"content"`
	FreshnessStamp time.Time `json:"freshness_stamp"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// PlanVersion is an immutable snapshot of a plan's content at a version.
type PlanVersion struct {
	ID        string    `json:"id"`
	PlanID    string    `json:"plan_id"`
	Version   int       `json:"version"`
	Content   Content   `json:"content"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Content is the union type for plan content — exactly one backend sub-schema is populated.
// The classifier maps one-to-one: backend → sub-schema.
type Content struct {
	Database  *DatabasePlan  `json:"database,omitempty"`
	Terraform *TerraformPlan `json:"terraform,omitempty"`
	Code      *CodePlan      `json:"code,omitempty"`
	Shell     *ShellPlan     `json:"shell,omitempty"`
	Deploy    *DeployPlan    `json:"deploy,omitempty"`
}

// --- Per-backend typed sub-schemas ---

// DatabasePlan carries migration objects with DDL fields.
// DDL must parse as valid SQL at submission time.
type DatabasePlan struct {
	MigrationName string `json:"migration_name"`
	DDL           string `json:"ddl"`
	RollbackDDL   string `json:"rollback_ddl,omitempty"`
	Direction     string `json:"direction"` // "up" or "down"
	DatabaseName  string `json:"database_name,omitempty"`
	SchemaVersion string `json:"schema_version,omitempty"`
}

// TerraformPlan carries Terraform JSON plan output.
type TerraformPlan struct {
	PlanJSON        string           `json:"plan_json"`
	ResourceChanges []ResourceChange `json:"resource_changes"`
	Provider        string           `json:"provider"`
	Workspace       string           `json:"workspace,omitempty"`
	StateVersion    int              `json:"state_version,omitempty"`
}

// ResourceChange describes a single Terraform resource change.
type ResourceChange struct {
	Address      string `json:"address"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	ChangeAction string `json:"change_action"` // create, update, delete, replace
}

// CodePlan carries structured code changes as a first-class plan backend.
// Every code modification flows through plan-then-apply — no fast path.
// Safe classifications auto-advance under default policy so trivial changes feel lightweight.
//
// The plan addresses entities by ID (files, symbols, services), declares operation types,
// carries structured diffs (parseable, not prose), before/after symbol snapshots,
// test expectations, a success contract, and a rollback plan.
type CodePlan struct {
	// TargetEntities identifies every file, symbol, or service this plan touches.
	// The executor scope-checker validates that actual changes don't exceed this set.
	TargetEntities []TargetEntity `json:"target_entities"`

	// Diffs is the ordered set of structured diffs, one per file touched.
	// Each diff is validated at schema level before classification.
	Diffs []CodeDiff `json:"diffs"`

	// SymbolSnapshots captures before/after state of symbols affected by this plan.
	// Used by the classifier to determine scope (public API, internal, etc.) and
	// by the freshness checker to detect drift.
	SymbolSnapshots []SymbolSnapshot `json:"symbol_snapshots,omitempty"`

	// TestExpectations declares what tests must pass after this plan is applied.
	// The executor verifies these as part of the apply step.
	TestExpectations *TestExpectations `json:"test_expectations,omitempty"`

	// SuccessContract defines the post-apply verification criteria.
	SuccessContract *SuccessContract `json:"success_contract,omitempty"`

	// RollbackPlan declares how to revert this change.
	// Git-based: revert commit ref. For coordinated changes, may reference prior plan steps.
	RollbackPlan *CodeRollbackPlan `json:"rollback_plan,omitempty"`

	// GitContext carries git-specific metadata: branch, base commit, PR linkage.
	GitContext *GitContext `json:"git_context,omitempty"`

	// Language is the primary language of the code changes.
	// Cross-language support limited to Tree-sitter-supported languages in v1;
	// unsupported languages classify as unknown (destructive default).
	Language string `json:"language"`

	// ServiceName is the service affected (for service sensitivity flags).
	ServiceName string `json:"service_name,omitempty"`
}

// TargetEntity identifies a code entity this plan touches.
// Entities are addressed by stable ID so cross-layer references work.
type TargetEntity struct {
	// ID is the stable entity identifier (file path, symbol FQN, or service map key).
	ID string `json:"id"`
	// EntityType classifies the entity: "file", "symbol", "service".
	EntityType string `json:"entity_type"`
	// OperationType declares the intended operation: "modify", "add", "remove", "rename".
	OperationType string `json:"operation_type"`
	// NewID is set when operation_type is "rename" — the entity's new identifier.
	NewID string `json:"new_id,omitempty"`
}

// ValidEntityTypes is the set of allowed entity types.
var ValidEntityTypes = []string{"file", "symbol", "service"}

// ValidOperationTypes is the set of allowed operation types for target entities.
var ValidOperationTypes = []string{"modify", "add", "remove", "rename"}

// CodeDiff is a structured diff for a single file. It is parseable, not prose.
// Unparseable diff content fails at submission — before classification.
type CodeDiff struct {
	// FilePath is the file being changed (relative to repo root).
	FilePath string `json:"file_path"`
	// Language of this specific file (may differ from plan-level language in polyglot repos).
	Language string `json:"language,omitempty"`
	// BeforeHash is the content hash before the change (for freshness/drift detection).
	BeforeHash string `json:"before_hash,omitempty"`
	// AfterHash is the expected content hash after the change.
	AfterHash string `json:"after_hash,omitempty"`
	// Hunks is the ordered set of diff hunks within this file.
	Hunks []CodeHunk `json:"hunks"`
}

// CodeHunk represents a single diff hunk in a code plan.
type CodeHunk struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Operation string `json:"operation"` // add, remove, modify
	// SymbolScope identifies the symbol context this hunk operates in (for classification).
	// e.g. "function:processPayment", "type:UserService", "method:(*Server).Start"
	SymbolScope string `json:"symbol_scope,omitempty"`
}

// SymbolSnapshot captures before/after state of a symbol for classification and drift detection.
type SymbolSnapshot struct {
	// SymbolID is the fully-qualified symbol name (e.g. "pkg.FuncName", "pkg.TypeName.MethodName").
	SymbolID string `json:"symbol_id"`
	// FilePath where this symbol is defined.
	FilePath string `json:"file_path"`
	// Kind is the symbol kind: "function", "method", "type", "interface", "variable", "constant".
	Kind string `json:"kind"`
	// Exported indicates whether this is a public API symbol.
	Exported bool `json:"exported"`
	// BeforeSignature is the symbol signature before the change (empty for new symbols).
	BeforeSignature string `json:"before_signature,omitempty"`
	// AfterSignature is the symbol signature after the change (empty for removed symbols).
	AfterSignature string `json:"after_signature,omitempty"`
	// Callers is the count of known callers (from code knowledge layer, warrant-26).
	// High caller count + signature change = higher risk.
	Callers int `json:"callers,omitempty"`
}

// ValidSymbolKinds is the set of valid symbol kinds.
var ValidSymbolKinds = []string{"function", "method", "type", "interface", "variable", "constant", "package"}

// TestExpectations declares what tests must pass after this plan is applied.
type TestExpectations struct {
	// TestCommands are the commands to run (e.g., "go test ./internal/plan/...").
	TestCommands []string `json:"test_commands"`
	// ExpectedNewTests lists test names/patterns this change adds.
	ExpectedNewTests []string `json:"expected_new_tests,omitempty"`
	// MinCoverage is the minimum required test coverage percentage (0 = not enforced).
	MinCoverage float64 `json:"min_coverage,omitempty"`
}

// SuccessContract defines post-apply verification criteria.
type SuccessContract struct {
	// Checks is the ordered list of verification checks.
	Checks []SuccessCheck `json:"checks"`
}

// SuccessCheck is a single verification step in the success contract.
type SuccessCheck struct {
	// Name is a human-readable name for this check.
	Name string `json:"name"`
	// Command is the command to execute for verification.
	Command string `json:"command"`
	// ExpectedOutput is a substring or regex the output must match (optional).
	ExpectedOutput string `json:"expected_output,omitempty"`
	// TimeoutSeconds is the max time for this check (0 = use default).
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// CodeRollbackPlan declares how to revert code changes.
type CodeRollbackPlan struct {
	// Strategy is the rollback approach: "git_revert", "git_reset", "manual".
	Strategy string `json:"strategy"`
	// RevertCommitRef is the commit SHA to revert to (for git_revert/git_reset).
	RevertCommitRef string `json:"revert_commit_ref,omitempty"`
	// RevertBranch is the branch containing the revert (if different from current).
	RevertBranch string `json:"revert_branch,omitempty"`
	// ManualSteps describes manual rollback steps (for strategy "manual").
	ManualSteps []string `json:"manual_steps,omitempty"`
}

// ValidRollbackStrategies is the set of valid rollback strategies.
var ValidRollbackStrategies = []string{"git_revert", "git_reset", "manual"}

// GitContext carries git-specific metadata for code plans.
// Git is the substrate, not a bypass: branches per ticket, commits tagged,
// PRs from structured content, merge under policy.
type GitContext struct {
	// Branch is the working branch for this plan (e.g., "ticket/warrant-49").
	Branch string `json:"branch"`
	// BaseBranch is the target branch for merge (e.g., "main").
	BaseBranch string `json:"base_branch,omitempty"`
	// BaseCommitSHA is the commit the plan was generated against (for freshness).
	BaseCommitSHA string `json:"base_commit_sha,omitempty"`
	// CommitSHA is the commit SHA after the plan was applied.
	CommitSHA string `json:"commit_sha,omitempty"`
	// CommitMessage is the structured commit message.
	CommitMessage string `json:"commit_message,omitempty"`
	// PRNumber is the pull request number (set after PR creation).
	PRNumber int `json:"pr_number,omitempty"`
	// PRURL is the pull request URL (set after PR creation).
	PRURL string `json:"pr_url,omitempty"`
	// TicketID tags commits and PRs to the originating ticket.
	TicketID string `json:"ticket_id,omitempty"`
}

// ShellPlan carries shell commands with explicit side-effect manifests.
// There is no slot for unvalidated prose — every command and its effects are typed.
// The SideEffectManifest is the permission boundary: the sandbox enforces it, and the
// classifier reads it (not the script) to determine risk classification.
type ShellPlan struct {
	Commands           []ShellCommand     `json:"commands"`
	WorkingDir         string             `json:"working_dir,omitempty"`
	Environment        map[string]string  `json:"environment,omitempty"`
	SideEffectManifest SideEffectManifest `json:"side_effect_manifest"`
}

// ShellCommand is a single command in a shell plan.
type ShellCommand struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Idempotent  bool   `json:"idempotent"`
	Timeout     string `json:"timeout,omitempty"` // Duration string e.g. "30s"
}

// SideEffectManifest explicitly declares what side effects shell commands will have.
// This is the declarative permission boundary that precedes execution.
// The sandbox enforces it: if the script attempts operations not declared here,
// execution fails. The classifier reads this manifest, not the script body.
// Wildcard operations (glob patterns like "**") widen classification automatically.
type SideEffectManifest struct {
	// FileOps declares all file system operations the script may perform.
	FileOps []FileOp `json:"file_ops,omitempty"`

	// NetworkOps declares all network operations (outbound connections).
	NetworkOps []NetworkOp `json:"network_ops,omitempty"`

	// ProcessOps declares all subprocesses the script may spawn.
	ProcessOps []ProcessOp `json:"process_ops,omitempty"`

	// CredentialOps declares which secrets/credentials the script will read.
	CredentialOps []CredentialOp `json:"credential_ops,omitempty"`

	// ResourceLimits declares upper bounds on resource consumption.
	ResourceLimits ResourceLimits `json:"resource_limits"`
}

// FileOp declares a file system operation with typed action and glob pattern.
type FileOp struct {
	// Action is the type of file operation: "read", "write", or "delete".
	Action string `json:"action"`
	// Path is a glob pattern for the affected files (e.g., "/app/dist/**", "/tmp/*.log").
	Path string `json:"path"`
}

// NetworkOp declares an outbound network call.
type NetworkOp struct {
	// Endpoint is the target host:port or URL pattern (e.g., "api.github.com:443").
	Endpoint string `json:"endpoint"`
	// Method is the HTTP method or protocol (e.g., "GET", "POST", "TCP").
	Method string `json:"method"`
	// Idempotent indicates whether this call is safe to retry.
	Idempotent bool `json:"idempotent"`
}

// ProcessOp declares a subprocess the script may spawn.
type ProcessOp struct {
	// Binary is the executable name or path (e.g., "npm", "/usr/bin/git").
	Binary string `json:"binary"`
	// Args is the allowed argument patterns (glob-matched).
	Args []string `json:"args,omitempty"`
}

// CredentialOp declares a secret/credential the script will access.
type CredentialOp struct {
	// Name is the credential identifier (env var name or secret path).
	Name string `json:"name"`
	// Purpose describes why this credential is needed (for audit).
	Purpose string `json:"purpose,omitempty"`
}

// ResourceLimits sets upper bounds on resource consumption for sandbox enforcement.
type ResourceLimits struct {
	// MaxRuntimeSeconds is the maximum wall-clock time the script may run.
	// Zero means use system default.
	MaxRuntimeSeconds int `json:"max_runtime_seconds,omitempty"`
	// MaxDiskWriteBytes is the maximum total bytes the script may write to disk.
	// Zero means use system default.
	MaxDiskWriteBytes int64 `json:"max_disk_write_bytes,omitempty"`
	// MaxNetworkCalls is the maximum number of outbound network connections.
	// Zero means use system default.
	MaxNetworkCalls int `json:"max_network_calls,omitempty"`
}

// ValidFileOpActions is the set of allowed file operation actions.
var ValidFileOpActions = []string{"read", "write", "delete"}

// ValidNetworkMethods is the set of allowed network operation methods.
var ValidNetworkMethods = []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "TCP", "UDP"}

// DeployPlan carries deploy objects referencing specific artifact hashes and targets.
type DeployPlan struct {
	ArtifactHash   string `json:"artifact_hash"`
	ArtifactURL    string `json:"artifact_url,omitempty"`
	Target         string `json:"target"`           // Environment/cluster target
	DeployStrategy string `json:"deploy_strategy"`  // rolling, blue-green, canary, recreate
	HealthCheckURL string `json:"health_check_url,omitempty"`
	RollbackHash   string `json:"rollback_hash,omitempty"`
	Replicas       int    `json:"replicas,omitempty"`
}
