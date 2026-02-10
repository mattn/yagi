package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

type mcpConnection struct {
	name    string
	session *mcp.ClientSession
}

var mcpConnections []*mcpConnection

func loadMCPConfig(configDir string) error {
	path := filepath.Join(configDir, "mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing mcp.json: %w", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    name,
		Version: version,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for name, sc := range config.MCPServers {
		cmd := exec.Command(sc.Command, sc.Args...)
		transport := &mcp.CommandTransport{Command: cmd}

		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: MCP server %q failed to connect: %v\n", name, err)
			continue
		}

		conn := &mcpConnection{name: name, session: session}
		mcpConnections = append(mcpConnections, conn)

		result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: MCP server %q failed to list tools: %v\n", name, err)
			continue
		}

		for _, tool := range result.Tools {
			toolName := tool.Name
			sess := session
			registerTool(
				toolName,
				tool.Description,
				marshalSchema(tool.InputSchema),
				func(ctx context.Context, arguments string) (string, error) {
					var args map[string]any
					json.Unmarshal([]byte(arguments), &args)

					callCtx, callCancel := context.WithTimeout(ctx, 60*time.Second)
					defer callCancel()

					res, err := sess.CallTool(callCtx, &mcp.CallToolParams{
						Name:      toolName,
						Arguments: args,
					})

					if err != nil {
						return "", fmt.Errorf("%v", err)
					}
					if res.IsError {
						return "", fmt.Errorf("tool error: %s", contentToString(res.Content))
					}
					return contentToString(res.Content), nil
				},
				false,
			)
			if verbose {
				fmt.Fprintf(os.Stderr, "Loaded MCP tool: %s (from %s)\n", toolName, name)
			}
		}
	}
	return nil
}

func closeMCPConnections() {
	for _, conn := range mcpConnections {
		conn.session.Close()
	}
}

func marshalSchema(schema any) json.RawMessage {
	if schema == nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	return b
}

func contentToString(content []mcp.Content) string {
	var result string
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			result += tc.Text
		}
	}
	return result
}
