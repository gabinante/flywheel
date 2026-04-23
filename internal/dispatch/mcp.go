package dispatch

// mcpConnection captures the Flywheel MCP endpoints a driver can use.
// Some harnesses want the legacy SSE transport, while others prefer the
// streamable HTTP endpoint.
type mcpConnection struct {
	Name    string
	SSEURL  string
	HTTPURL string
	Headers map[string]string
}

// mcpConfig is the on-disk Claude-compatible MCP configuration file structure.
type mcpConfig struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
}

type mcpServerConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func buildMCPConnection(serverURL, apiKey string) mcpConnection {
	return buildMCPConnectionForType("", serverURL, apiKey)
}

// buildMCPConnectionForType creates both SSE and streamable HTTP endpoints with
// optional tool filtering based on worker type. When workerType is empty, all
// tools are available.
func buildMCPConnectionForType(workerType WorkerType, serverURL, apiKey string) mcpConnection {
	sseURL := serverURL + "/sse"
	httpURL := serverURL + "/mcp"
	if workerType != "" && workerType.IsValid() {
		suffix := "?worker_type=" + string(workerType)
		sseURL += suffix
		httpURL += suffix
	}

	var headers map[string]string
	if apiKey != "" {
		headers = map[string]string{"X-API-Key": apiKey}
	}

	return mcpConnection{
		Name:    "flywheel",
		SSEURL:  sseURL,
		HTTPURL: httpURL,
		Headers: headers,
	}
}

func (c mcpConnection) SSEConfig() mcpConfig {
	serverCfg := mcpServerConfig{
		Type: "sse",
		URL:  c.SSEURL,
	}
	if len(c.Headers) > 0 {
		serverCfg.Headers = cloneStringMap(c.Headers)
	}
	return mcpConfig{
		MCPServers: map[string]mcpServerConfig{
			c.Name: serverCfg,
		},
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func buildMCPConfig(serverURL, apiKey string) mcpConfig {
	return buildMCPConnection(serverURL, apiKey).SSEConfig()
}

func buildMCPConfigForType(workerType WorkerType, serverURL, apiKey string) mcpConfig {
	return buildMCPConnectionForType(workerType, serverURL, apiKey).SSEConfig()
}
