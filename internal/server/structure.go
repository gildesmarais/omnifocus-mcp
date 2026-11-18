package server

import (
	"context"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type addIn struct {
	Items []omni.StructAdd `json:"items" jsonschema:"projects, tags or folders to create, applied in order"`
}

type editIn struct {
	Items []omni.StructEdit `json:"items" jsonschema:"patches applied in order; omitted field = unchanged"`
}

type refsIn struct {
	Items []omni.StructRef `json:"items" jsonschema:"objects to act on, applied in order"`
}

type structOut struct {
	Results []omni.StructResult `json:"results"`
}

func addStructureTools(s *mcp.Server, b *omni.Bridge) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "add",
		Description: "Create projects, tags or folders in one batch. parent is a folder id (project, folder) or tag id (tag); omit for the root. Optional status (project Active/OnHold/Done, tag Active/OnHold). Project only, named as in add_tasks: type Parallel|Sequential|SingleActions, defer/due/planned, flagged, note, tags by id or name, estimatedMinutes, repetition {rule (RRULE), scheduleType Regularly|FromCompletion, anchor DeferDate|DueDate|PlannedDate, catchUp}, completedByChildren. Returns one result per item (id, ok/error, name, status, parent); a bad item does not stop the others.",
		Annotations: notDestructive,
		InputSchema: addSchema[addIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in addIn) (*mcp.CallToolResult, structOut, error) {
		rs, err := b.AddStructure(ctx, in.Items)
		return nil, structOut{Results: rs}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "edit",
		Description: "Patch projects, tags or folders in one batch: rename; project status Active/OnHold/Done; tag status Active/OnHold; folder status Active restores a dropped folder; project only, named as in edit_tasks: type Parallel|Sequential|SingleActions, note, flagged, defer/due/planned, estimatedMinutes and repetition (null clears these), tags {add, remove} by id or name, completedByChildren; review (project only): reviewInterval {steps>=1, unit days|weeks|months|years}, nextReviewDate (truncated to local midnight), reviewed true to mark reviewed now (recomputes nextReviewDate; not with nextReviewDate); move under a parent id or to the root (parent null); reorder before or after a sibling of the same kind. Folders: rename, move, reorder, restore. Dropping and deleting are separate tools. Returns one result per item.",
		Annotations: notDestructive,
		InputSchema: writeSchema[editIn](),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in editIn) (*mcp.CallToolResult, structOut, error) {
		rs, err := b.EditStructure(ctx, in.Items)
		return nil, structOut{Results: rs}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "drop",
		Description: "Drop tasks, projects, tags or folders. Reversible through this server: projects, tags and folders by edit status Active, tasks by edit_tasks restore true. Dropping a project drops its tasks; dropping a folder drops its projects. Guard enforced by OmniFocus script: a tag is refused while it or any descendant tag has remaining tasks (untag or complete them first). Returns one result per item; a refusal comes back as that item's error.",
		Annotations: destructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in refsIn) (*mcp.CallToolResult, structOut, error) {
		rs, err := b.DropObjects(ctx, in.Items)
		return nil, structOut{Results: rs}, err
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "delete",
		Description: "Permanently delete tasks, projects, tags or folders. IRREVERSIBLE. Guards: a tag or folder is deleted only when already dropped and empty (delete children first); a task or project is deleted only without remaining (incomplete) tasks or subtasks; drop it instead. Returns one result per item; a refusal comes back as that item's error with the reason.",
		Annotations: destructive,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in refsIn) (*mcp.CallToolResult, structOut, error) {
		rs, err := b.DeleteObjects(ctx, in.Items)
		return nil, structOut{Results: rs}, err
	})
}
