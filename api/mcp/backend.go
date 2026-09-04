package mcp

import (
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/codereview"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/report"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/sessions"
	"github.com/gabinante/flywheel/internal/ticket"
	"github.com/gabinante/flywheel/internal/workflow"
	"github.com/gabinante/flywheel/internal/workstream"
)

// Backend holds the services needed by MCP tools. Set by the server that runs MCP.
type Backend struct {
	Project    *project.Service
	WorkStream *workstream.Service
	Ticket     *ticket.Service
	Queue      *queue.Service
	Trace      *execution.Service
	Review     *review.Service
	Org        *org.Service
	AgentStore agent.AgentStore
	Repos      *project.RepositoryService // nil-safe: multi-repo features disabled when nil
	Workflow   *workflow.Engine
	Sessions   *sessions.Service   // nil-safe: session tools are not registered when nil
	CodeReview *codereview.Service // nil-safe: code review tools are not registered when nil
	Reports    *report.Service     // nil-safe: report tools are not registered when nil

	// DefaultAgentID is used as a fallback when agent_id is not passed in args
	// and not available from HTTP auth context.
	DefaultAgentID string
}
