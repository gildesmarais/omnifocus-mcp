package server

import (
	"context"
	"encoding/json"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var readOnly = &mcp.ToolAnnotations{ReadOnlyHint: true}

type listTasksIn struct {
	Inbox       *bool  `json:"inbox,omitempty" jsonschema:"only inbox tasks (true) or only non-inbox tasks (false)"`
	Project     string `json:"project,omitempty" jsonschema:"project id or exact name"`
	Tag         string `json:"tag,omitempty" jsonschema:"tag id or exact name"`
	Available   *bool  `json:"available,omitempty" jsonschema:"true: not blocked, deferred, completed or dropped"`
	Flagged     *bool  `json:"flagged,omitempty"`
	Completed   *bool  `json:"completed,omitempty"`
	DueBefore   string `json:"due_before,omitempty" jsonschema:"YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS, local time"`
	DeferBefore string `json:"defer_before,omitempty" jsonschema:"YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS, local time"`
	Search      string `json:"search,omitempty" jsonschema:"case-insensitive substring over name and note"`
	Limit       *int   `json:"limit,omitempty" jsonschema:"max results (default 50); null returns all"`
}

type listTasksOut struct {
	Tasks []omni.Task `json:"tasks"`
}

type listProjectsIn struct {
	Status string `json:"status,omitempty" jsonschema:"Active, OnHold, Done or Dropped"`
	Folder string `json:"folder,omitempty" jsonschema:"folder id or exact name; includes subfolders"`
	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring over name and note"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"max results (default 50); null returns all"`
}

type listProjectsOut struct {
	Projects []omni.Project `json:"projects"`
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

func addReadTools(s *mcp.Server, b *omni.Bridge) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks. Filters are AND-combined. Limit defaults to 50; null returns all.",
		Annotations: readOnly,
		InputSchema: withLimitDefault[listTasksIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listTasksIn) (*mcp.CallToolResult, listTasksOut, error) {
		ts, err := b.ListTasks(ctx, omni.TaskFilter(in))
		if ts == nil {
			ts = []omni.Task{}
		}
		return nil, listTasksOut{Tasks: ts}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: "List projects, optionally filtered by status, folder or search. Limit defaults to 50; null returns all.",
		Annotations: readOnly,
		InputSchema: withLimitDefault[listProjectsIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listProjectsIn) (*mcp.CallToolResult, listProjectsOut, error) {
		ps, err := b.ListProjects(ctx, omni.ProjectFilter(in))
		if ps == nil {
			ps = []omni.Project{}
		}
		return nil, listProjectsOut{Projects: ps}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tags",
		Description: "List tags with status and remainingTasks.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, listTagsOut, error) {
		ts, err := b.ListTags(ctx)
		if ts == nil {
			ts = []omni.Tag{}
		}
		return nil, listTagsOut{Tags: ts}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_folders",
		Description: "List folders with parent and status.",
		Annotations: readOnly,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ noIn) (*mcp.CallToolResult, listFoldersOut, error) {
		fs, err := b.ListFolders(ctx)
		if fs == nil {
			fs = []omni.Folder{}
		}
		return nil, listFoldersOut{Folders: fs}, err
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
