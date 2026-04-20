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

// CodePlan carries AST diff representation for code changes.
type CodePlan struct {
	FilePath   string     `json:"file_path"`
	Language   string     `json:"language"`
	BeforeHash string     `json:"before_hash"`
	AfterHash  string     `json:"after_hash"`
	Hunks      []CodeHunk `json:"hunks"`
}

// CodeHunk represents a single diff hunk in a code plan.
type CodeHunk struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Operation string `json:"operation"` // add, remove, modify
}

// ShellPlan carries shell commands with explicit side-effect manifests.
// There is no slot for unvalidated prose — every command and its effects are typed.
type ShellPlan struct {
	Commands          []ShellCommand      `json:"commands"`
	WorkingDir        string              `json:"working_dir,omitempty"`
	Environment       map[string]string   `json:"environment,omitempty"`
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
type SideEffectManifest struct {
	FilesCreated  []string `json:"files_created,omitempty"`
	FilesModified []string `json:"files_modified,omitempty"`
	FilesDeleted  []string `json:"files_deleted,omitempty"`
	NetworkAccess []string `json:"network_access,omitempty"` // host:port or CIDR
	ProcessesSpawned []string `json:"processes_spawned,omitempty"`
	EnvVarsSet    []string `json:"env_vars_set,omitempty"`
}

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
