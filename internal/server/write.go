package server

import (
	"context"
	"reflect"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type addTasksIn struct {
	Tasks []omni.TaskAdd `json:"tasks" jsonschema:"tasks to create, applied in order"`
}

type editTasksIn struct {
	Patches []omni.TaskEdit `json:"patches" jsonschema:"patches applied in order; omitted field = unchanged, null clears note/defer/due/planned"`
}

type taskWriteOut struct {
	Results []omni.TaskResult `json:"results"`
}

// writeSchema builds In's input schema with omni.NullableString as string-or-null.
func writeSchema[In any]() *jsonschema.Schema {
	sc, err := jsonschema.For[In](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[omni.NullableString](): {Types: []string{"string", "null"}},
	}})
	if err != nil {
		panic(err)
	}
	return sc
}

func addWriteTools(s *mcp.Server, b *omni.Bridge) {
	notDestructive := &mcp.ToolAnnotations{DestructiveHint: new(false)}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_tasks",
		Description: "Create tasks in one batch. Each task goes to the inbox unless project or parent is given. Returns one result per task (id, ok/error, task record); a bad item does not stop the others.",
		Annotations: notDestructive,
		InputSchema: writeSchema[addTasksIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in addTasksIn) (*mcp.CallToolResult, taskWriteOut, error) {
		rs, err := b.AddTasks(ctx, in.Tasks)
		return nil, taskWriteOut{Results: rs}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "edit_tasks",
		Description: "Patch tasks in one batch: name, note, flagged, defer/due/planned (null clears), complete or uncomplete, move to a project, the inbox or a parent task, tags add/remove by id or name. Returns one result per patch (id, ok/error, updated task record).",
		Annotations: notDestructive,
		InputSchema: writeSchema[editTasksIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in editTasksIn) (*mcp.CallToolResult, taskWriteOut, error) {
		rs, err := b.EditTasks(ctx, in.Patches)
		return nil, taskWriteOut{Results: rs}, err
	})
}
