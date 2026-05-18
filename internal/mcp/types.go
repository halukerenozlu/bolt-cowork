package mcp

// ServerConfig describes a single MCP server's connection parameters.
// It is used both by the internal registry and for JSON serialization
// of ~/.bolt-cowork/mcp.json entries.
type ServerConfig struct {
	// Name is the unique identifier for this server within bolt-cowork.
	Name string `json:"name"`

	// Transport specifies the communication mechanism: "stdio" or "sse".
	Transport string `json:"transport"`

	// Command is the binary path or executable used for stdio transport.
	Command string `json:"command,omitempty"`

	// Args are the command-line arguments passed to the stdio process.
	Args []string `json:"args,omitempty"`

	// URL is the endpoint used for sse transport.
	URL string `json:"url,omitempty"`

	// Env contains additional environment variables injected into the server process.
	Env map[string]string `json:"env,omitempty"`

	// Enabled controls whether bolt-cowork will connect to this server at startup.
	Enabled bool `json:"enabled"`
}

// MCPConfig is the top-level structure of the ~/.bolt-cowork/mcp.json file.
// It lists every MCP server that bolt-cowork should connect to.
type MCPConfig struct {
	// Servers is the ordered list of MCP server definitions.
	Servers []ServerConfig `json:"servers"`
}

// MCPTool describes a tool exposed by an MCP server as stored in the registry.
// It extends the wire-format Tool with the originating server name.
type MCPTool struct {
	// Name is the tool's identifier.
	Name string `json:"name"`

	// Description is a human-readable explanation of what the tool does.
	Description string `json:"description,omitempty"`

	// InputSchema holds the raw JSON Schema for the tool's input parameters.
	InputSchema map[string]any `json:"input_schema,omitempty"`

	// ServerName identifies which MCP server provides this tool.
	ServerName string `json:"server_name"`
}

// MCPResource describes a resource exposed by an MCP server.
type MCPResource struct {
	// URI is the unique resource identifier.
	URI string `json:"uri"`

	// Name is the human-readable name of the resource.
	Name string `json:"name"`

	// Description explains the resource content.
	Description string `json:"description,omitempty"`

	// MimeType is the MIME content type of the resource (e.g. "text/plain").
	MimeType string `json:"mime_type,omitempty"`

	// ServerName identifies which MCP server provides this resource.
	ServerName string `json:"server_name"`
}

// MCPPrompt describes a prompt template exposed by an MCP server.
type MCPPrompt struct {
	// Name is the prompt template identifier.
	Name string `json:"name"`

	// Description explains the prompt's purpose.
	Description string `json:"description,omitempty"`

	// Arguments lists the parameters this prompt template accepts.
	Arguments []PromptArgument `json:"arguments,omitempty"`

	// ServerName identifies which MCP server provides this prompt.
	ServerName string `json:"server_name"`
}

// PromptArgument describes a single argument for an MCPPrompt.
type PromptArgument struct {
	// Name is the argument identifier.
	Name string `json:"name"`

	// Description explains the argument's purpose.
	Description string `json:"description,omitempty"`

	// Required indicates whether the argument must be supplied by the caller.
	Required bool `json:"required"`
}

// --- Wire-protocol types (tool discovery and execution) ---

// Tool represents a single MCP tool as returned by the tools/list method.
// Unlike MCPTool, this is the raw wire-format definition without registry
// bookkeeping fields such as ServerName.
type Tool struct {
	// Name is the tool's unique identifier within a server.
	Name string `json:"name"`

	// Description is a short, human-readable explanation of the tool.
	Description string `json:"description,omitempty"`

	// InputSchema defines the expected shape of the tool's input parameters.
	InputSchema ToolSchema `json:"inputSchema"`
}

// ToolSchema is a simplified JSON Schema object that describes the input
// parameters accepted by a Tool.
type ToolSchema struct {
	// Type is the JSON Schema type keyword (typically "object").
	Type string `json:"type"`

	// Properties maps parameter names to their individual property definitions.
	Properties map[string]ToolProperty `json:"properties,omitempty"`

	// Required lists the names of parameters that must be present.
	Required []string `json:"required,omitempty"`
}

// ToolProperty describes a single parameter within a ToolSchema.
type ToolProperty struct {
	// Type is the JSON Schema type of the parameter (e.g. "string", "integer").
	Type string `json:"type"`

	// Description is a human-readable explanation of the parameter's purpose.
	Description string `json:"description,omitempty"`
}

// CallToolResult is the response payload returned by the tools/call method.
type CallToolResult struct {
	// Content is the list of content items produced by the tool.
	Content []ToolResultContent `json:"content"`

	// IsError indicates that the tool itself signalled a failure condition.
	// When true the Content items describe the error rather than a success result.
	IsError bool `json:"isError,omitempty"`
}

// ToolResultContent represents a single piece of output within a CallToolResult.
type ToolResultContent struct {
	// Type is the content kind, e.g. "text" or "image".
	Type string `json:"type"`

	// Text holds the textual payload when Type is "text".
	Text string `json:"text,omitempty"`
}

// --- Lifecycle types (MCP handshake) ---

// InitializeParams is sent by the client as the params of the initialize request.
type InitializeParams struct {
	// ProtocolVersion is the MCP version the client wishes to use.
	ProtocolVersion string `json:"protocolVersion"`

	// ClientInfo identifies the connecting client application.
	ClientInfo ClientInfo `json:"clientInfo"`
}

// InitializeResult is the server's response to the initialize request.
type InitializeResult struct {
	// ProtocolVersion is the MCP version the server agreed to use.
	ProtocolVersion string `json:"protocolVersion"`

	// ServerInfo identifies the responding MCP server.
	ServerInfo ServerInfo `json:"serverInfo"`

	// Capabilities declares which optional MCP features the server supports.
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ClientInfo carries identifying information about the MCP client.
type ClientInfo struct {
	// Name is the human-readable name of the client application.
	Name string `json:"name"`

	// Version is the client application's version string.
	Version string `json:"version"`
}

// ServerInfo carries identifying information about the MCP server.
type ServerInfo struct {
	// Name is the human-readable name of the server.
	Name string `json:"name"`

	// Version is the server's version string.
	Version string `json:"version"`
}

// ServerCapabilities declares the optional MCP feature sets a server supports.
// A nil pointer for any field means the server does not support that feature.
type ServerCapabilities struct {
	// Tools signals that the server implements the tools/* methods.
	Tools *ToolsCapability `json:"tools,omitempty"`

	// Resources signals that the server implements the resources/* methods.
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

// ToolsCapability is a marker struct indicating tools/* support.
// Additional per-feature options may be added here in future protocol versions.
type ToolsCapability struct{}

// ResourcesCapability is a marker struct indicating resources/* support.
// Additional per-feature options may be added here in future protocol versions.
type ResourcesCapability struct{}
