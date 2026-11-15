// Package server registers MCP tools and maps them onto the omni bridge.
package server

import (
	"context"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// New builds the MCP server with every implemented tool registered.
func New(b *omni.Bridge, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "omnifocus-mcp", Version: version}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_perspectives",
		Description: "List OmniFocus perspectives. Built-in ones have a null id and builtin=true.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, listPerspectives(b))
	return s
}

type listPerspectivesIn struct{}

type listPerspectivesOut struct {
	Perspectives []omni.Perspective `json:"perspectives"`
}

func listPerspectives(b *omni.Bridge) mcp.ToolHandlerFor[listPerspectivesIn, listPerspectivesOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ listPerspectivesIn) (*mcp.CallToolResult, listPerspectivesOut, error) {
		ps, err := b.Perspectives(ctx)
		if err != nil {
			return nil, listPerspectivesOut{}, err
		}
		return nil, listPerspectivesOut{Perspectives: ps}, nil
	}
}
