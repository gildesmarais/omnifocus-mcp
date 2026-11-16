package server

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connect serves the tools over an in-memory transport and returns the client session.
func connect(t *testing.T, r omni.Runner) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	if _, err := New(&omni.Bridge{Runner: r}, "test").Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

var update = flag.Bool("update", false, "rewrite testdata golden files")

type fakeRunner struct{ res string }

func (f fakeRunner) Run(context.Context, string) (string, error) { return f.res, nil }

func TestListPerspectivesOverMCP(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, fakeRunner{`[{"id":null,"name":"Inbox","builtin":true}]`})
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

// The tools/list result is the public contract: any change to a name, description,
// schema or annotation must show up in testdata/tools.golden.json (go test ./internal/server -update).
func TestToolsListGolden(t *testing.T) {
	res, err := connect(t, fakeRunner{`[]`}).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(res.Tools, func(i, j int) bool { return res.Tools[i].Name < res.Tools[j].Name })
	got, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const path = "testdata/tools.golden.json"
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("tools/list differs from %s; review the change and run with -update", path)
	}
}

func TestToolsListPins(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, fakeRunner{`[]`})
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tl := range res.Tools {
		tools[tl.Name] = tl
	}
	for _, name := range []string{"list_tasks", "list_projects"} {
		raw, _ := json.Marshal(tools[name].InputSchema)
		var sc struct {
			Required   []string                  `json:"required"`
			Properties map[string]map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(raw, &sc); err != nil {
			t.Fatal(err)
		}
		if d, _ := sc.Properties["limit"]["default"].(float64); d != 50 {
			t.Errorf("%s: limit default = %v, want 50", name, sc.Properties["limit"]["default"])
		}
		for _, r := range sc.Required {
			if r == "limit" {
				t.Errorf("%s: limit must not be required", name)
			}
		}
	}
	if a := tools["list_perspectives"].Annotations; a == nil || !a.ReadOnlyHint {
		t.Errorf("list_perspectives must be readOnly: %+v", a)
	}
	for _, name := range []string{"drop", "delete"} {
		if a := tools[name].Annotations; a == nil || a.DestructiveHint == nil || !*a.DestructiveHint {
			t.Errorf("%s must be destructive: %+v", name, a)
		}
	}
}

// call runs one tool against a fake runner and also returns the script source the bridge sent.
func call(t *testing.T, res string, name string, args map[string]any) (map[string]any, *mcp.CallToolResult, string) {
	t.Helper()
	r := &recRunner{res: res}
	cs := connect(t, r)
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := out.StructuredContent.(map[string]any)
	return m, out, r.src
}

type recRunner struct{ res, src string }

func (r *recRunner) Run(_ context.Context, src string) (string, error) {
	r.src = src
	return r.res, nil
}

func TestListTasksLimitDefaultAndNull(t *testing.T) {
	out, res, lastSrc := call(t, `[{"id":"a","name":"n","status":"Available","tags":[]}]`, "list_tasks", map[string]any{"flagged": true})
	if res.IsError || len(out["tasks"].([]any)) != 1 {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(lastSrc, `\"limit\":50`) || !strings.Contains(lastSrc, `\"flagged\":true`) {
		t.Fatalf("default limit/flagged not sent: %s", lastSrc)
	}
	_, _, lastSrc = call(t, `[]`, "list_tasks", map[string]any{"limit": nil})
	// null limit is dropped by omitempty; scripts treat absent like null ("all").
	if strings.Contains(lastSrc, `\"limit\"`) {
		t.Fatalf("null limit must not become a number: %s", lastSrc)
	}
}

func TestOtherReadToolsOverMCP(t *testing.T) {
	out, res, src := call(t, `[{"id":"p","name":"P","status":"OnHold","tags":[]}]`, "list_projects", map[string]any{"status": "OnHold", "folder": "F"})
	if res.IsError || len(out["projects"].([]any)) != 1 {
		t.Fatalf("list_projects: %+v", res)
	}
	if !strings.Contains(src, `\"status\":\"OnHold\"`) || !strings.Contains(src, `\"folder\":\"F\"`) {
		t.Fatalf("status/folder not forwarded: %s", src)
	}
	out, res, _ = call(t, `[{"id":"t","name":"T","status":"Active","remainingTasks":4}]`, "list_tags", map[string]any{})
	if res.IsError || out["tags"].([]any)[0].(map[string]any)["remainingTasks"] != float64(4) {
		t.Fatalf("list_tags: %+v", res)
	}
	out, res, _ = call(t, `[{"id":"f","name":"F","status":"Active"}]`, "list_folders", map[string]any{})
	if res.IsError || len(out["folders"].([]any)) != 1 {
		t.Fatalf("list_folders: %+v", res)
	}
}

// Decoding and ErrNotFound live in omni; here only the error mapping over MCP.
func TestGettersMissingIDIsToolError(t *testing.T) {
	for _, tool := range []string{"get_task", "get_project", "get_tag", "get_folder"} {
		if _, res, _ := call(t, `[]`, tool, map[string]any{"id": "nope"}); !res.IsError {
			t.Errorf("%s: want error for missing id", tool)
		}
	}
}

// A client may send `arguments: null`; tools with schema defaults must not crash the server.
func TestNullArgumentsDoNotPanic(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, fakeRunner{`[]`})
	for _, tool := range []string{"list_tasks", "list_projects", "list_tags", "list_folders", "list_perspectives"} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: json.RawMessage("null")})
		if err != nil || res.IsError {
			t.Errorf("%s: err=%v res=%+v", tool, err, res)
		}
	}
	// Getters require an id: null arguments must come back as an error, not a panic or hang.
	for _, tool := range []string{"get_task", "get_project", "get_tag", "get_folder"} {
		if res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: json.RawMessage("null")}); err == nil && !res.IsError {
			t.Errorf("%s: want error for null arguments, got %+v", tool, res)
		}
	}
}

func TestEditTasksOverMCP(t *testing.T) {
	res := `[{"id":"a","ok":true,"task":{"id":"a","name":"N","status":"Available","tags":[]}}]`
	out, r, lastSrc := call(t, res, "edit_tasks", map[string]any{"patches": []any{map[string]any{"id": "a", "defer": nil, "flagged": true}}})
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
}

func TestAddTasksOverMCP(t *testing.T) {
	res := `[{"id":"n","ok":true,"task":{"id":"n","name":"Buy milk","status":"Available","tags":[]}}]`
	out, r, src := call(t, res, "add_tasks", map[string]any{"tasks": []any{map[string]any{"name": "Buy milk", "project": "p1", "due": "2026-11-20"}}})
	if r.IsError {
		t.Fatalf("add_tasks: %+v", r)
	}
	task := out["results"].([]any)[0].(map[string]any)["task"].(map[string]any)
	if task["name"] != "Buy milk" {
		t.Fatalf("task not decoded: %v", out)
	}
	if !strings.Contains(src, `\"project\":\"p1\"`) || !strings.Contains(src, `\"due\":\"2026-11-20\"`) {
		t.Fatalf("project/due not forwarded: %s", src)
	}
}

func TestStructureToolsOverMCP(t *testing.T) {
	res := `[{"id":"a","kind":"tag","ok":true,"name":"T","status":"Active"}]`
	out, r, lastSrc := call(t, res, "edit", map[string]any{"items": []any{map[string]any{"kind": "tag", "id": "a", "move": map[string]any{"parent": nil}}}})
	if r.IsError || out["results"].([]any)[0].(map[string]any)["ok"] != true {
		t.Fatalf("edit: %+v", r)
	}
	if !strings.Contains(lastSrc, `\"move\":{\"parent\":null}`) {
		t.Fatalf("root move not forwarded: %s", lastSrc)
	}
	for tool, args := range map[string]map[string]any{
		"add": {"items": []any{map[string]any{"kind": "tag", "name": "x"}}},
	} {
		if _, r, _ := call(t, res, tool, args); r.IsError {
			t.Errorf("%s: %+v", tool, r)
		}
	}
}

func TestAddStructureProjectOverMCP(t *testing.T) {
	res := `[{"id":"n","kind":"project","ok":true,"name":"P","status":"OnHold","parent":{"id":"f1","name":"Top"}}]`
	out, r, src := call(t, res, "add", map[string]any{"items": []any{map[string]any{"kind": "project", "name": "P", "parent": "f1", "sequential": true, "status": "OnHold"}}})
	if r.IsError {
		t.Fatalf("add: %+v", r)
	}
	for _, want := range []string{`\"parent\":\"f1\"`, `\"sequential\":true`, `\"status\":\"OnHold\"`} {
		if !strings.Contains(src, want) {
			t.Errorf("source lacks %s: %s", want, src)
		}
	}
	got := out["results"].([]any)[0].(map[string]any)
	if got["status"] != "OnHold" || got["parent"].(map[string]any)["name"] != "Top" {
		t.Fatalf("envelope: %v", got)
	}
}

// drop and delete keep per-item outcomes: a guard refusal on one item must not hide the other.
func TestDropDeleteMixedResultsOverMCP(t *testing.T) {
	res := `[{"id":"a","kind":"tag","ok":true,"name":"T","status":"Dropped"},{"id":"b","kind":"project","ok":false,"error":"project has 2 remaining task(s)"}]`
	items := []any{map[string]any{"kind": "tag", "id": "a"}, map[string]any{"kind": "project", "id": "b"}}
	for _, tool := range []string{"drop", "delete"} {
		out, r, _ := call(t, res, tool, map[string]any{"items": items})
		if r.IsError {
			t.Fatalf("%s: %+v", tool, r)
		}
		rs := out["results"].([]any)
		first, second := rs[0].(map[string]any), rs[1].(map[string]any)
		if len(rs) != 2 || first["id"] != "a" || first["ok"] != true || second["id"] != "b" || second["ok"] != false ||
			second["kind"] != "project" || !strings.Contains(second["error"].(string), "remaining") {
			t.Errorf("%s: %v", tool, rs)
		}
	}
}

func TestToolAnnotations(t *testing.T) {
	ctx := context.Background()
	cs := connect(t, fakeRunner{`[]`})
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
