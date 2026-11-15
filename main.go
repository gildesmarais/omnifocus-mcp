// Command omnifocus-mcp serves OmniFocus over MCP on stdio.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/gildesmarais/omnifocus-mcp/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := server.New(omni.New(), version).Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
