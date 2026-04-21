package mcp

import (
	"github.com/gabinante/flywheel/internal/agent"
	"github.com/gabinante/flywheel/internal/catalog"
	"github.com/gabinante/flywheel/internal/claims"
	"github.com/gabinante/flywheel/internal/execution"
	"github.com/gabinante/flywheel/internal/investigation"
	"github.com/gabinante/flywheel/internal/notification"
	"github.com/gabinante/flywheel/internal/org"
	"github.com/gabinante/flywheel/internal/project"
	"github.com/gabinante/flywheel/internal/queue"
	"github.com/gabinante/flywheel/internal/review"
	"github.com/gabinante/flywheel/internal/ticket"
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
	AgentStore    agent.AgentStore
	Investigation *investigation.Service
	Claims        *claims.Service
	Notification  *notification.Service
	Repos         *project.RepositoryService // nil-safe: multi-repo features disabled when nil

	// CodeIntel is the pluggable code intelligence provider (Layer 3).
	// When nil, code_* tools are not registered. Set via PluginRegistry.
	CodeIntel CodeIntelligenceProvider

	// Findings is the pluggable findings provider (Layer 4).
	// When non-nil, findings_* MCP tools are registered.
	// Default: Weaviate backend; alternatives: pgvector, Qdrant, in-memory.
	Findings FindingsProvider

	// Catalog (Layer 14 project map)
	Catalog        *catalog.Service
	CatalogScanner *catalog.Scanner

	// DefaultAgentID is used as a fallback when agent_id is not passed in args
	// and not available from HTTP auth context (e.g. stdio mode with FLYWHEEL_TOKEN).
	DefaultAgentID string
}
