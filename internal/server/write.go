package server

import (
	"context"
	"reflect"
	"slices"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type addTasksIn struct {
	Tasks []omni.TaskAdd `json:"tasks" jsonschema:"tasks to create, applied in order"`
}

type editTasksIn struct {
	Patches []omni.TaskEdit `json:"patches" jsonschema:"patches applied in order; omitted field = unchanged, null clears note/defer/due/planned/estimatedMinutes/repetition"`
}

type taskWriteOut struct {
	Results []omni.TaskResult `json:"results"`
}

// writeSchema builds In's input schema with the omni.Nullable fields as value-or-null.
func writeSchema[In any]() *jsonschema.Schema {
	rep, err := jsonschema.For[omni.Repetition](nil)
	if err != nil {
		panic(err)
	}
	zero := 0.0
	sc, err := jsonschema.For[In](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[omni.NullableString]():            {Types: []string{"string", "null"}},
		reflect.TypeFor[omni.Nullable[int]]():             {Types: []string{"integer", "null"}, Minimum: &zero},
		reflect.TypeFor[omni.Nullable[omni.Repetition]](): {AnyOf: []*jsonschema.Schema{rep, {Type: "null"}}},
	}})
	if err != nil {
		panic(err)
	}
	return sc
}

// addSchema is writeSchema for an add tool: add semantics have no clear, so no
// field accepts null, and the SDK's nullable pointers and slices are made plain.
func addSchema[In any]() *jsonschema.Schema {
	sc := writeSchema[In]()
	var strip func(*jsonschema.Schema)
	strip = func(s *jsonschema.Schema) {
		if s == nil {
			return
		}
		s.Types = slices.DeleteFunc(s.Types, func(t string) bool { return t == "null" })
		if len(s.Types) == 1 {
			s.Type, s.Types = s.Types[0], nil
		}
		for _, p := range s.Properties {
			strip(p)
		}
		strip(s.Items)
	}
	strip(sc)
	return sc
}

func addWriteTools(s *mcp.Server, b *omni.Bridge) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_tasks",
		Description: "Create tasks in one batch. Each task goes to the inbox unless project or parent is given. Optional estimatedMinutes, repetition {rule (RRULE), scheduleType Regularly|FromCompletion, anchor DeferDate|DueDate|PlannedDate, catchUp}, sequential and completedByChildren. Returns one result per task (id, ok/error, task record); a bad item does not stop the others.",
		Annotations: notDestructive,
		InputSchema: addSchema[addTasksIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in addTasksIn) (*mcp.CallToolResult, taskWriteOut, error) {
		rs, err := b.AddTasks(ctx, in.Tasks)
		return nil, taskWriteOut{Results: rs}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "edit_tasks",
		Description: "Patch tasks in one batch: name, note, flagged, defer/due/planned (null clears), complete or uncomplete, move to a project, the inbox or a parent task, tags add/remove by id or name, estimatedMinutes and repetition (null clears), sequential, completedByChildren, restore true to un-drop a dropped task (not with complete). Returns one result per patch (id, ok/error, updated task record).",
		Annotations: notDestructive,
		InputSchema: writeSchema[editTasksIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in editTasksIn) (*mcp.CallToolResult, taskWriteOut, error) {
		rs, err := b.EditTasks(ctx, in.Patches)
		return nil, taskWriteOut{Results: rs}, err
	})
}
