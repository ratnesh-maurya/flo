// Package mcp handles subprocess lifecycle and MCP communication
// with the Stack Exchange MCP server.
//
// JSON-RPC flow when calling a tool:
//
//	Client sends:  {"jsonrpc":"2.0","id":N,"method":"tools/call","params":{"name":"<tool>","arguments":{...}}}
//	Server replies: {"jsonrpc":"2.0","id":N,"result":{"content":[{"type":"text","text":"..."}]}}
package mcp

import (
	"context"
	"fmt"
	"regexp"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpprotocol "github.com/mark3labs/mcp-go/mcp"
)

// Client wraps an MCP client connected to the Stack Exchange server subprocess.
type Client struct {
	inner mcpclient.MCPClient
}

// NewClient spawns the mcp-remote bridge via npx, which connects to
// the official Stack Overflow MCP server at mcp.stackoverflow.com
// using the stdio transport (JSON-RPC over stdin/stdout).
// On first run the user is taken through a browser-based OAuth flow;
// mcp-remote caches the token for subsequent calls.
func NewClient(ctx context.Context) (*Client, error) {
	inner, err := mcpclient.NewStdioMCPClient(
		"npx",
		nil,
		"-y", "mcp-remote", "https://mcp.stackoverflow.com",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn MCP server: %w", err)
	}

	// Send MCP "initialize" handshake.
	initReq := mcpprotocol.InitializeRequest{}
	initReq.Method = "initialize"
	initReq.Params.ProtocolVersion = mcpprotocol.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpprotocol.Implementation{
		Name:    "flo",
		Version: "1.0.0",
	}
	initReq.Params.Capabilities = mcpprotocol.ClientCapabilities{}

	_, err = inner.Initialize(ctx, initReq)
	if err != nil {
		inner.Close()
		return nil, fmt.Errorf("MCP initialize handshake failed: %w", err)
	}

	return &Client{inner: inner}, nil
}

// CallTool invokes a named tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, toolName string, args map[string]any) (*mcpprotocol.CallToolResult, error) {
	req := mcpprotocol.CallToolRequest{}
	req.Method = "tools/call"
	req.Params.Name = toolName
	req.Params.Arguments = args

	result, err := c.inner.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("tool call %q failed: %w", toolName, err)
	}
	if result.IsError {
		text := ExtractText(result)
		return nil, fmt.Errorf("tool %q returned error: %s", toolName, text)
	}

	return result, nil
}

// Close shuts down the MCP client and kills the subprocess.
func (c *Client) Close() error {
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

// ExtractText concatenates all TextContent items from a CallToolResult.
func ExtractText(result *mcpprotocol.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, content := range result.Content {
		if tc, ok := content.(mcpprotocol.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	result2 := parts[0]
	for i := 1; i < len(parts); i++ {
		result2 += "\n" + parts[i]
	}
	return result2
}

// ExtractQuestionID finds the first SO question ID in text.
// The server returns references like SO_Q12345678; also handles URL patterns.
func ExtractQuestionID(text string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`SO_Q(\d+)`),
		regexp.MustCompile(`/questions/(\d+)`),
		regexp.MustCompile(`stackoverflow\.com/q/(\d+)`),
		regexp.MustCompile(`(?i)question\s*(?:id)?[:\s]+(\d{5,})`),
	}
	for _, re := range patterns {
		if m := re.FindStringSubmatch(text); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}
