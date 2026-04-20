package mcp

import (
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/workstream"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
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

	// DefaultAgentID is used as a fallback when agent_id is not passed in args
	// and not available from HTTP auth context (e.g. stdio mode with FLYWHEEL_TOKEN).
	DefaultAgentID string
}
