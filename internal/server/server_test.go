package server

import (
	"context"
	"encoding/json"
	"strings"
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

func call(t *testing.T, res string, name string, args map[string]any) (map[string]any, *mcp.CallToolResult) {
	t.Helper()
	ctx := context.Background()
	r := &recRunner{res: res}
	ct, st := mcp.NewInMemoryTransports()
	if _, err := New(&omni.Bridge{Runner: r}, "test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := out.StructuredContent.(map[string]any)
	lastSrc = r.src
	return m, out
}

type recRunner struct{ res, src string }

func (r *recRunner) Run(_ context.Context, src string) (string, error) {
	r.src = src
	return r.res, nil
}

var lastSrc string

func TestListTasksLimitDefaultAndNull(t *testing.T) {
	out, res := call(t, `[{"id":"a","name":"n","status":"Available","tags":[]}]`, "list_tasks", map[string]any{"flagged": true})
	if res.IsError || len(out["tasks"].([]any)) != 1 {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(lastSrc, `\"limit\":50`) || !strings.Contains(lastSrc, `\"flagged\":true`) {
		t.Fatalf("default limit/flagged not sent: %s", lastSrc)
	}
	call(t, `[]`, "list_tasks", map[string]any{"limit": nil})
	if !strings.Contains(lastSrc, `\"limit\":null`) {
		t.Fatalf("null limit not sent: %s", lastSrc)
	}
}

func TestOtherReadToolsOverMCP(t *testing.T) {
	cases := []struct{ tool, res, key string }{
		{"list_projects", `[{"id":"p","name":"P","status":"Active","tags":[]}]`, "projects"},
		{"list_tags", `[{"id":"t","name":"T","status":"Active","remainingTasks":1}]`, "tags"},
		{"list_folders", `[{"id":"f","name":"F","status":"Active"}]`, "folders"},
	}
	for _, c := range cases {
		out, res := call(t, c.res, c.tool, map[string]any{})
		if res.IsError || len(out[c.key].([]any)) != 1 {
			t.Errorf("%s: %+v", c.tool, res)
		}
	}
	for _, tool := range []string{"get_task", "get_project", "get_tag", "get_folder"} {
		out, res := call(t, `[{"id":"x1","name":"N","status":"Active","tags":[]}]`, tool, map[string]any{"id": "x1"})
		if res.IsError || out["id"] != "x1" {
			t.Errorf("%s: %+v", tool, res)
		}
		if _, res := call(t, `[]`, tool, map[string]any{"id": "nope"}); !res.IsError {
			t.Errorf("%s: want error for missing id", tool)
		}
	}
}

// A client may send `arguments: null`; tools with schema defaults must not crash the server.
func TestNullArgumentsDoNotPanic(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	b := &omni.Bridge{Runner: fakeRunner{`[]`}}
	if _, err := New(b, "test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"list_tasks", "list_projects", "list_tags", "list_perspectives"} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: json.RawMessage("null")})
		if err != nil || res.IsError {
			t.Errorf("%s: err=%v res=%+v", tool, err, res)
		}
	}
}

func TestEditTasksOverMCP(t *testing.T) {
	res := `[{"id":"a","ok":true,"task":{"id":"a","name":"N","status":"Available","tags":[]}}]`
	out, r := call(t, res, "edit_tasks", map[string]any{"patches": []any{map[string]any{"id": "a", "defer": nil, "flagged": true}}})
	if r.IsError {
		t.Fatalf("%+v", r)
	}
	rs := out["results"].([]any)
	if len(rs) != 1 || rs[0].(map[string]any)["ok"] != true {
		t.Fatalf("%v", out)
	}
	if !strings.Contains(lastSrc, `\"defer\":null`) {
		t.Fatalf("null defer not forwarded: %s", lastSrc)
	}
	if _, r := call(t, `[{"id":"n","ok":true}]`, "add_tasks", map[string]any{"tasks": []any{map[string]any{"name": "x"}}}); r.IsError {
		t.Fatalf("add_tasks: %+v", r)
	}
}

func TestStructureToolsOverMCP(t *testing.T) {
	res := `[{"id":"a","kind":"tag","ok":true,"name":"T","status":"Active"}]`
	out, r := call(t, res, "edit", map[string]any{"items": []any{map[string]any{"kind": "tag", "id": "a", "move": map[string]any{"parent": nil}}}})
	if r.IsError || out["results"].([]any)[0].(map[string]any)["ok"] != true {
		t.Fatalf("edit: %+v", r)
	}
	if !strings.Contains(lastSrc, `\"move\":{\"parent\":null}`) {
		t.Fatalf("root move not forwarded: %s", lastSrc)
	}
	for tool, args := range map[string]map[string]any{
		"add":    {"items": []any{map[string]any{"kind": "tag", "name": "x"}}},
		"drop":   {"items": []any{map[string]any{"kind": "tag", "id": "a"}}},
		"delete": {"items": []any{map[string]any{"kind": "tag", "id": "a"}}},
	} {
		if _, r := call(t, res, tool, args); r.IsError {
			t.Errorf("%s: %+v", tool, r)
		}
	}
}

func TestToolAnnotations(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := New(&omni.Bridge{Runner: fakeRunner{`[]`}}, "test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		n++
		a := tool.Annotations
		if a == nil || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: OpenWorldHint must be false", tool.Name)
		}
		if (tool.Name == "add" || tool.Name == "add_tasks") && a != nil && a.IdempotentHint {
			t.Errorf("%s: adds are not idempotent", tool.Name)
		}
		if tool.Name == "drop" && !strings.Contains(tool.Description, "drops its tasks") {
			t.Errorf("drop description: %s", tool.Description)
		}
	}
	if n < 14 {
		t.Fatalf("only %d tools listed", n)
	}
}
