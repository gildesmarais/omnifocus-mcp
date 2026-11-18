package server

import (
	"context"
	"encoding/json"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listTasksOut struct {
	Tasks []omni.Task `json:"tasks"`
}

type listProjectsOut struct {
	Projects []omni.Project `json:"projects"`
}

type listPerspectivesOut struct {
	Perspectives []omni.Perspective `json:"perspectives"`
}

type noIn struct{}

type listTagsOut struct {
	Tags []omni.Tag `json:"tags"`
}

type listFoldersOut struct {
	Folders []omni.Folder `json:"folders"`
}

type idIn struct {
	ID string `json:"id" jsonschema:"OmniFocus primary key"`
}

// withLimitDefault builds the input schema for In and sets limit's default,
// so an omitted limit becomes 50 while an explicit null stays "all".
func withLimitDefault[In any]() *jsonschema.Schema {
	sc, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	}
	sc.Properties["limit"].Default = json.RawMessage("50")
	return sc
}

// nonNil turns a nil slice into an empty one so list output is [] not null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func addReadTools(s *mcp.Server, b *omni.Bridge) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_perspectives",
		Description: "List OmniFocus perspectives. Built-in ones have a null id and builtin=true.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, listPerspectivesOut, error) {
		ps, err := b.Perspectives(ctx)
		return nil, listPerspectivesOut{Perspectives: nonNil(ps)}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks. Filters are AND-combined. *_after bounds are on or after, *_before bounds on or before; a date-only value covers its whole local day, and a task without that date never matches. Nothing hides completed or dropped tasks unless a filter does (available, completed, dropped); completed_* bounds match completed tasks only. Limit defaults to 50; null returns all.",
		Annotations: readOnly,
		InputSchema: withLimitDefault[omni.TaskFilter](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in omni.TaskFilter) (*mcp.CallToolResult, listTasksOut, error) {
		ts, err := b.ListTasks(ctx, in)
		return nil, listTasksOut{Tasks: nonNil(ts)}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: "List projects, optionally filtered by status, folder, search or review_due_before (next review on or before a date). Records include reviewInterval, nextReviewDate and lastReviewDate. Limit defaults to 50; null returns all.",
		Annotations: readOnly,
		InputSchema: withLimitDefault[omni.ProjectFilter](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in omni.ProjectFilter) (*mcp.CallToolResult, listProjectsOut, error) {
		ps, err := b.ListProjects(ctx, in)
		return nil, listProjectsOut{Projects: nonNil(ps)}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tags",
		Description: "List tags with status and remainingTasks.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, listTagsOut, error) {
		ts, err := b.ListTags(ctx)
		return nil, listTagsOut{Tags: nonNil(ts)}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_folders",
		Description: "List folders with parent and status.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, listFoldersOut, error) {
		fs, err := b.ListFolders(ctx)
		return nil, listFoldersOut{Folders: nonNil(fs)}, err
	})
	addGet(s, "get_task", "Get one task by id; errors if not found.", b.GetTask)
	addGet(s, "get_project", "Get one project by id; errors if not found.", b.GetProject)
	addGet(s, "get_tag", "Get one tag by id; errors if not found.", b.GetTag)
	addGet(s, "get_folder", "Get one folder by id; errors if not found.", b.GetFolder)
}

func addGet[Out any](s *mcp.Server, name, desc string, get func(context.Context, string) (Out, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc, Annotations: readOnly},
		func(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, Out, error) {
			out, err := get(ctx, in.ID)
			return nil, out, err
		})
}
