package mcp

// ServerConfig describes a single MCP server's connection parameters.
type ServerConfig struct {
	Name      string
	Transport string // "stdio" or "sse"
	Command   string // for stdio: binary path
	Args      []string
	URL       string // for sse: endpoint URL
	Env       map[string]string
	Enabled   bool
}

// MCPTool describes a tool exposed by an MCP server.
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	ServerName  string // which MCP server provides this
}

// MCPResource describes a resource exposed by an MCP server.
type MCPResource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	ServerName  string
}

// MCPPrompt describes a prompt template exposed by an MCP server.
type MCPPrompt struct {
	Name        string
	Description string
	Arguments   []PromptArgument
	ServerName  string
}

// PromptArgument describes a single argument for an MCPPrompt.
type PromptArgument struct {
	Name        string
	Description string
	Required    bool
}
