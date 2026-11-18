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

// clearMark tags the schemas of clearable fields so the null strip skips them.
const clearMark = "x-clear"

// stripNull removes null from every type, except under schemas marked clearMark
// (which lose only the mark). The SDK makes pointers and slices nullable; null
// is wanted only where it means "clear".
func stripNull(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if _, ok := s.Extra[clearMark]; ok {
		s.Extra = nil
		return
	}
	s.Types = slices.DeleteFunc(s.Types, func(t string) bool { return t == "null" })
	if len(s.Types) == 1 {
		s.Type, s.Types = s.Types[0], nil
	}
	for _, p := range s.Properties {
		stripNull(p)
	}
	stripNull(s.Items)
}

// clearable lets null through s where it means "clear"; the mark keeps
// stripNull off it.
func clearable(s *jsonschema.Schema) *jsonschema.Schema {
	mark := map[string]any{clearMark: true}
	if s.Type == "object" {
		return &jsonschema.Schema{AnyOf: []*jsonschema.Schema{s, {Type: "null"}}, Extra: mark}
	}
	c := s.CloneSchemas()
	c.Types, c.Type, c.Extra = []string{c.Type, "null"}, "", mark
	return c
}

// typeSchemas turns the rules omni reports for the roots (enum names, lower
// bounds) into schemas, keyed by Go type for jsonschema.ForOptions.TypeSchemas.
// The types are the one owner of the rules, so a violation fails the whole call
// at the schema before any script runs.
func typeSchemas(roots ...reflect.Type) map[reflect.Type]*jsonschema.Schema {
	types := map[reflect.Type]*jsonschema.Schema{}
	for _, root := range roots {
		for t, r := range omni.InputRules(root) {
			types[t] = ruleSchema(r)
		}
	}
	return types
}

func ruleSchema(r omni.Rule) *jsonschema.Schema {
	if r.Min != nil {
		min := float64(*r.Min)
		return &jsonschema.Schema{Type: "integer", Minimum: &min}
	}
	s := &jsonschema.Schema{Type: "string"}
	for _, n := range r.Names {
		s.Enum = append(s.Enum, n)
	}
	return s
}

// writeSchema builds In's input schema, with the rules of the types it reaches
// (typeSchemas). Null is accepted only where it means "clear": the
// omni.Nullable fields, as value-or-null. Every other null fails validation.
// withClear false (add tools: nothing to clear) makes the Nullable fields plain too.
func writeSchema[In any](withClear bool) *jsonschema.Schema {
	types := typeSchemas(reflect.TypeFor[In](), reflect.TypeFor[omni.Repetition](), reflect.TypeFor[omni.Minutes]())
	rep, err := jsonschema.For[omni.Repetition](&jsonschema.ForOptions{TypeSchemas: types})
	if err != nil {
		panic(err)
	}
	for t, base := range map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[omni.NullableString]():            {Type: "string"},
		reflect.TypeFor[omni.Nullable[omni.Minutes]]():    types[reflect.TypeFor[omni.Minutes]()],
		reflect.TypeFor[omni.Nullable[omni.Repetition]](): rep,
	} {
		if withClear {
			base = clearable(base)
		}
		types[t] = base
	}
	sc, err := jsonschema.For[In](&jsonschema.ForOptions{TypeSchemas: types})
	if err != nil {
		panic(err)
	}
	stripNull(sc)
	return sc
}

func addWriteTools(s *mcp.Server, b *omni.Bridge) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add_tasks",
		Description: "Create tasks in one batch. Each task goes to the inbox unless project or parent is given. Optional estimatedMinutes, repetition {rule (RRULE), scheduleType, anchor, catchUp}, sequential and completedByChildren. Returns one result per task (id, ok/error, task record); a bad item does not stop the others.",
		Annotations: notDestructive,
		InputSchema: writeSchema[addTasksIn](false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in addTasksIn) (*mcp.CallToolResult, taskWriteOut, error) {
		rs, err := b.AddTasks(ctx, in.Tasks)
		return nil, taskWriteOut{Results: rs}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "edit_tasks",
		Description: "Patch tasks in one batch: name, note, flagged, defer/due/planned (null clears), complete or uncomplete, move to a project, the inbox or a parent task, tags add/remove by id or name, estimatedMinutes and repetition (null clears), sequential, completedByChildren, restore true to un-drop a dropped task (not with complete). Returns one result per patch (id, ok/error, updated task record).",
		Annotations: notDestructive,
		InputSchema: writeSchema[editTasksIn](true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in editTasksIn) (*mcp.CallToolResult, taskWriteOut, error) {
		rs, err := b.EditTasks(ctx, in.Patches)
		return nil, taskWriteOut{Results: rs}, err
	})
}
