package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer creates an MCP server with Flywheel tools and resources using the official go-sdk.
// Returns the server and an HTTP handler for Streamable HTTP. The handler can be wrapped
// with MCPHTTPHandler for auth.
func NewServer(b *Backend) (*mcp.Server, error) {
	if b == nil {
		return nil, fmt.Errorf("mcp: backend is required")
	}
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "Flywheel",
		Version: "0.1.0",
	}, nil)
	RegisterTools(server, b)
	RegisterCodeIntelTools(server, b)
	registerResources(server)
	return server, nil
}

// NewStreamableHTTPHandler returns an http.Handler that serves MCP over Streamable HTTP.
// Pass the server returned by NewServer. The returned handler expects to be wrapped
// (e.g. by rest.MCPHTTPHandler) for authentication.
func NewStreamableHTTPHandler(server *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)
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
