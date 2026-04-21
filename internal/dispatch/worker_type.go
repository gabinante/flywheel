package dispatch

// WorkerType identifies the specialized role a worker plays in the ticket lifecycle.
// Each type gets a tailored system prompt and restricted MCP tool access.
type WorkerType string

const (
	// WorkerTypePlanner generates plans from tickets. It reads code, investigates,
	// and produces a structured plan but does not modify files.
	WorkerTypePlanner WorkerType = "planner"

	// WorkerTypeExecutor produces diffs from plans. It writes code, runs tests,
	// and submits the result.
	WorkerTypeExecutor WorkerType = "executor"

	// WorkerTypeValidator verifies that diffs meet acceptance criteria. It reviews
	// PRs, runs tests, and approves or rejects.
	WorkerTypeValidator WorkerType = "validator"

	// WorkerTypeDeployer applies plans to target environments. It executes
	// deployment operations and observes outcomes.
	WorkerTypeDeployer WorkerType = "deployer"

	// WorkerTypeInvestigator runs as a subagent dispatch for research tasks.
	// It reads code, searches, and reports findings without modifying state.
	WorkerTypeInvestigator WorkerType = "investigator"
)

// AllWorkerTypes returns all defined worker types.
func AllWorkerTypes() []WorkerType {
	return []WorkerType{
		WorkerTypePlanner,
		WorkerTypeExecutor,
		WorkerTypeValidator,
		WorkerTypeDeployer,
		WorkerTypeInvestigator,
	}
}

// IsValid returns true if the worker type is a recognized value.
func (wt WorkerType) IsValid() bool {
	switch wt {
	case WorkerTypePlanner, WorkerTypeExecutor, WorkerTypeValidator,
		WorkerTypeDeployer, WorkerTypeInvestigator:
		return true
	}
	return false
}

// String returns the string representation of the worker type.
func (wt WorkerType) String() string {
	return string(wt)
}

// ToolAccess defines which MCP tools a worker type is allowed to use.
// Tools not in the allowed list are filtered from the MCP config.
type ToolAccess struct {
	// AllowedTools lists the MCP tool names this worker type can access.
	// An empty list means all tools are allowed (no filtering).
	AllowedTools []string

	// DeniedTools lists tools explicitly denied for this worker type.
	// Denial takes precedence over AllowedTools.
	DeniedTools []string
}

// workerTypeToolAccess defines the MCP tool access per worker type.
// These map to the Flywheel MCP server's tool names.
var workerTypeToolAccess = map[WorkerType]ToolAccess{
	WorkerTypePlanner: {
		AllowedTools: []string{
			"claim_ticket",
			"start_ticket",
			"get_ticket",
			"log_step",
			"submit_ticket",
			"escalate_ticket",
			"renew_lease",
			"get_project_context",
			"list_tickets",
			"get_work_stream",
		},
		DeniedTools: []string{
			"approve_ticket",
			"reject_ticket",
			"create_ticket",
			"create_work_stream",
			"update_work_stream_plan",
		},
	},
	WorkerTypeExecutor: {
		AllowedTools: []string{
			"claim_ticket",
			"start_ticket",
			"get_ticket",
			"log_step",
			"submit_ticket",
			"escalate_ticket",
			"renew_lease",
			"get_project_context",
		},
		DeniedTools: []string{
			"approve_ticket",
			"reject_ticket",
			"create_ticket",
			"create_work_stream",
			"update_work_stream_plan",
		},
	},
	WorkerTypeValidator: {
		AllowedTools: []string{
			"get_ticket",
			"approve_ticket",
			"reject_ticket",
			"log_step",
			"get_project_context",
			"list_tickets",
		},
		DeniedTools: []string{
			"claim_ticket",
			"start_ticket",
			"submit_ticket",
			"create_ticket",
			"create_work_stream",
		},
	},
	WorkerTypeDeployer: {
		AllowedTools: []string{
			"claim_ticket",
			"start_ticket",
			"get_ticket",
			"log_step",
			"submit_ticket",
			"escalate_ticket",
			"renew_lease",
			"get_project_context",
		},
		DeniedTools: []string{
			"approve_ticket",
			"reject_ticket",
			"create_ticket",
			"create_work_stream",
			"update_work_stream_plan",
		},
	},
	WorkerTypeInvestigator: {
		AllowedTools: []string{
			"get_ticket",
			"log_step",
			"get_project_context",
			"list_tickets",
			"get_work_stream",
			"escalate_ticket",
		},
		DeniedTools: []string{
			"claim_ticket",
			"start_ticket",
			"submit_ticket",
			"approve_ticket",
			"reject_ticket",
			"create_ticket",
			"create_work_stream",
		},
	},
}

// GetToolAccess returns the tool access definition for a worker type.
// Returns an empty ToolAccess (all tools allowed) for unknown types.
func GetToolAccess(wt WorkerType) ToolAccess {
	if ta, ok := workerTypeToolAccess[wt]; ok {
		return ta
	}
	return ToolAccess{}
}

// IsToolAllowed checks whether a specific tool is allowed for a worker type.
func IsToolAllowed(wt WorkerType, toolName string) bool {
	ta := GetToolAccess(wt)

	// Check denied list first (takes precedence).
	for _, denied := range ta.DeniedTools {
		if denied == toolName {
			return false
		}
	}

	// If no allowed list, all (non-denied) tools are allowed.
	if len(ta.AllowedTools) == 0 {
		return true
	}

	// Check allowed list.
	for _, allowed := range ta.AllowedTools {
		if allowed == toolName {
			return true
		}
	}

	return false
}
