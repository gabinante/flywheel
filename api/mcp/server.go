package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerInstructions is the instructional text sent to MCP clients during initialization.
// It tells agents what Flywheel is, what tools are available, and the canonical workflow.
const ServerInstructions = `Flywheel is a work queue and shared context system for software projects. AI agents and humans use the same ticket system — agents claim tickets, execute work, and submit results for human review.

## Quick start

1. list_projects — see projects you have access to
2. get_project_context — load conventions, key files, and system prompt for a project
3. list_tickets — see available work (filter by state: pending)
4. claim_ticket — claim the next available ticket (returns ticket + lease_token)
5. get_ticket — load full ticket payload (objective, success criteria, dependencies)
6. start_ticket — move ticket to executing
7. Do the work, calling log_step after each significant action
8. submit_ticket — submit outputs for human review

## Two roles

- **Coordinator**: plans work — creates projects, work streams, tickets with dependencies. Never writes code.
- **Worker**: executes a single ticket — claims, starts, does the work, submits. Writes code in a git worktree.

## Key concepts

- **Work streams** group tickets toward a goal. When project has repo_url, set the git branch via update_work_stream.
- **Tickets** have dependencies (depends_on). A ticket is claimable only when all dependencies are done.
- **Lease** gives you exclusive access to a ticket. Renew with renew_lease if work takes longer than the TTL.
- **log_step** builds the execution trace that reviewers see. Call it frequently.

Read the full agent guide resource at flywheel://docs/agent-guide for detailed workflows, tool reference, and best practices.`

// NewServer creates an MCP server with Flywheel tools and resources using the official go-sdk.
// Returns the server and an HTTP handler for Streamable HTTP. The handler can be wrapped
// with MCPHTTPHandler for auth.
func NewServer(b *Backend) (*mcp.Server, error) {
	if b == nil {
		return nil, fmt.Errorf("mcp: backend is required")
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "Flywheel",
		Version: "0.2.0",
	}, &mcp.ServerOptions{
		Instructions: ServerInstructions,
	})
	RegisterTools(server, b)
	RegisterRepoTools(server, b)
	RegisterSessionTools(server, b)
	RegisterCodeReviewTools(server, b)
	registerResources(server)
	return server, nil
}

// NewStreamableHTTPHandler returns an http.Handler that serves MCP over Streamable HTTP.
// Pass the server returned by NewServer. The returned handler expects to be wrapped
// (e.g. by rest.MCPHTTPHandler) for authentication.
func NewStreamableHTTPHandler(server *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
	})
}

// NewSSEHandler returns an http.Handler that serves MCP over SSE (legacy transport).
// Needed for older Claude Code versions that use type:"sse" MCP config.
func NewSSEHandler(server *mcp.Server) http.Handler {
	return mcp.NewSSEHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)
}

func registerResources(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		URI:         AgentGuideURI,
		Name:        "Flywheel agent guide",
		Description: "Typical agent flow, tool summary, and ticket lifecycle for working with Flywheel via MCP.",
		MIMEType:    "text/markdown",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: AgentGuideURI, MIMEType: "text/markdown", Text: AgentGuideContent},
			},
		}, nil
	})
}

// RunStdio runs the MCP server over stdio (for IDE/agent integration). Blocks until exit.
func RunStdio(b *Backend) {
	s, err := NewServer(b)
	if err != nil {
		log.Fatalf("mcp: %v", err)
	}
	// Official SDK uses Server.Run with a transport. Stdio is typically via a different entrypoint.
	// For stdio we need a transport; the SDK has mcp.StdioTransport. Run blocks.
	if err := s.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("mcp stdio: %v", err)
	}
}
