package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeRunner struct{ res string }

func (f fakeRunner) Run(context.Context, string) (string, error) { return f.res, nil }

func TestListPerspectivesOverMCP(t *testing.T) {
	ctx := context.Background()
	b := &omni.Bridge{Runner: fakeRunner{`[{"id":null,"name":"Inbox","builtin":true}]`}}
	ct, st := mcp.NewInMemoryTransports()
	if _, err := New(b, "test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_perspectives"})
	if err != nil || res.IsError {
		t.Fatalf("err=%v res=%+v", err, res)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var out listPerspectivesOut
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Perspectives) != 1 || out.Perspectives[0].Name != "Inbox" {
		t.Fatalf("bad output %s (%v)", raw, err)
	}
}
