// Package server registers MCP tools and maps them onto the omni bridge.
package server

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool annotations, shared by every tool file.
var (
	readOnly       = &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
	notDestructive = &mcp.ToolAnnotations{DestructiveHint: new(false), OpenWorldHint: new(false)}
	destructive    = &mcp.ToolAnnotations{DestructiveHint: new(true), OpenWorldHint: new(false)}
)

// New builds the MCP server with every implemented tool registered.
func New(b *omni.Bridge, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "omnifocus-mcp", Version: version}, nil)
	addReadTools(s, b)
	addWriteTools(s, b)
	addStructureTools(s, b)
	s.AddReceivingMiddleware(hardenCalls)
	return s
}

// hardenCalls keeps the server alive on any tool input: `arguments: null` (or
// absent) becomes `{}` before the SDK applies schema defaults (it panics on a
// nil map), and a panic anywhere below becomes an error instead of a crash.
func hardenCalls(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (res mcp.Result, err error) {
		if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
			if a := bytes.TrimSpace(call.Params.Arguments); len(a) == 0 || bytes.Equal(a, []byte("null")) {
				call.Params.Arguments = []byte("{}")
			}
		}
		defer func() {
			if r := recover(); r != nil {
				res, err = nil, fmt.Errorf("internal error: %v", r)
			}
		}()
		return next(ctx, method, req)
	}
}
