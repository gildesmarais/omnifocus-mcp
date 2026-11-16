package omni

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound means no object has the requested id.
var ErrNotFound = errors.New("not found")

// Ref is an id+name pointer to a related object.
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Task is a task as OmniFocus reports it. Dates are local ISO 8601 without offset.
type Task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Note        string `json:"note,omitempty"`
	Status      string `json:"status"`
	Flagged     bool   `json:"flagged"`
	Completed   bool   `json:"completed"`
	Dropped     bool   `json:"dropped"`
	Available   bool   `json:"available"`
	Blocked     bool   `json:"blocked"`
	Inbox       bool   `json:"inbox"`
	Project     *Ref   `json:"project,omitempty"`
	Parent      *Ref   `json:"parent,omitempty"`
	Tags        []Ref  `json:"tags"`
	Defer       string `json:"defer,omitempty"`
	Due         string `json:"due,omitempty"`
	Planned     string `json:"planned,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`

	EstimatedMinutes *int        `json:"estimatedMinutes,omitempty"`
	Repetition       *Repetition `json:"repetition,omitempty"`
}

// Repetition is a task's repeat rule: an RRULE string and the method name (Fixed, DeferUntilDate, DueDate, ...).
type Repetition struct {
	Rule   string `json:"rule"`
	Method string `json:"method"`
}

// Project is a project with its folder, tags and dates.
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Note        string `json:"note,omitempty"`
	Status      string `json:"status"`
	Flagged     bool   `json:"flagged"`
	Sequential  bool   `json:"sequential"`
	Available   bool   `json:"available"`
	Blocked     bool   `json:"blocked"`
	Folder      *Ref   `json:"folder,omitempty"`
	Tags        []Ref  `json:"tags"`
	Defer       string `json:"defer,omitempty"`
	Due         string `json:"due,omitempty"`
	Planned     string `json:"planned,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`
}

// Tag is a tag with its open-task count.
type Tag struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	RemainingTasks int    `json:"remainingTasks"`
	Parent         *Ref   `json:"parent,omitempty"`
}

// Folder is a project folder.
type Folder struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Parent *Ref   `json:"parent,omitempty"`
}

// TaskFilter holds list_tasks filters; nil/zero means "no filter".
// Limit nil means all tasks; the MCP schema default (server withLimitDefault) is 50.
type TaskFilter struct {
	Inbox       *bool  `json:"inbox,omitempty" jsonschema:"only inbox tasks (true) or only non-inbox tasks (false)"`
	Project     string `json:"project,omitempty" jsonschema:"project id or exact name"`
	Tag         string `json:"tag,omitempty" jsonschema:"tag id or exact name"`
	Available   *bool  `json:"available,omitempty" jsonschema:"true: not blocked, deferred, completed or dropped"`
	Flagged     *bool  `json:"flagged,omitempty"`
	Completed   *bool  `json:"completed,omitempty"`
	DueBefore   string `json:"due_before,omitempty" jsonschema:"YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	DeferBefore string `json:"defer_before,omitempty" jsonschema:"YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	Search      string `json:"search,omitempty" jsonschema:"case-insensitive substring over name and note"`
	Limit       *int   `json:"limit,omitempty" jsonschema:"max results (default 50); null returns all"`
}

// ProjectFilter holds list_projects filters.
type ProjectFilter struct {
	Status string `json:"status,omitempty" jsonschema:"Active, OnHold, Done or Dropped"`
	Folder string `json:"folder,omitempty" jsonschema:"folder id or exact name; includes subfolders"`
	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring over name and note"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"max results (default 50); null returns all"`
}

var projectStatuses = map[string]bool{"Active": true, "OnHold": true, "Done": true, "Dropped": true}

func checkDate(field, s string) error {
	if s == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return nil
	}
	if _, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return nil
	}
	return fmt.Errorf("%s: %q is not YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS or an RFC 3339 timestamp with offset (Z, +01:00)", field, s)
}

func checkLimit(l *int) error {
	if l != nil && *l < 0 {
		return fmt.Errorf("limit: must be >= 0 or null")
	}
	return nil
}

// ListTasks returns tasks matching f (AND-combined).
func (b *Bridge) ListTasks(ctx context.Context, f TaskFilter) ([]Task, error) {
	if err := errors.Join(checkDate("due_before", f.DueBefore), checkDate("defer_before", f.DeferBefore), checkLimit(f.Limit)); err != nil {
		return nil, err
	}
	if f.Inbox != nil && *f.Inbox && f.Project != "" {
		return nil, errors.New("inbox: cannot be combined with project (inbox tasks belong to no project)")
	}
	var out []Task
	if err := b.call(ctx, scriptTasks, f, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTask returns one task or ErrNotFound.
func (b *Bridge) GetTask(ctx context.Context, id string) (Task, error) {
	return getOne[Task](ctx, b, scriptTasks, "task", id)
}

// ListProjects returns projects matching f.
func (b *Bridge) ListProjects(ctx context.Context, f ProjectFilter) ([]Project, error) {
	if f.Status != "" && !projectStatuses[f.Status] {
		return nil, fmt.Errorf("status: %q must be Active, OnHold, Done or Dropped", f.Status)
	}
	if err := checkLimit(f.Limit); err != nil {
		return nil, err
	}
	var out []Project
	if err := b.call(ctx, scriptProjects, f, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetProject returns one project or ErrNotFound.
func (b *Bridge) GetProject(ctx context.Context, id string) (Project, error) {
	return getOne[Project](ctx, b, scriptProjects, "project", id)
}

// ListTags returns all tags.
func (b *Bridge) ListTags(ctx context.Context) ([]Tag, error) {
	var out []Tag
	if err := b.call(ctx, scriptTags, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTag returns one tag or ErrNotFound.
func (b *Bridge) GetTag(ctx context.Context, id string) (Tag, error) {
	return getOne[Tag](ctx, b, scriptTags, "tag", id)
}

// ListFolders returns all folders.
func (b *Bridge) ListFolders(ctx context.Context) ([]Folder, error) {
	var out []Folder
	if err := b.call(ctx, scriptFolders, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFolder returns one folder or ErrNotFound.
func (b *Bridge) GetFolder(ctx context.Context, id string) (Folder, error) {
	return getOne[Folder](ctx, b, scriptFolders, "folder", id)
}

// getOne runs a read script with {id} and unwraps its 0-or-1 element result.
func getOne[T any](ctx context.Context, b *Bridge, s script, kind, id string) (T, error) {
	var zero T
	if id == "" {
		return zero, fmt.Errorf("%s id is required", kind)
	}
	var out []T
	if err := b.call(ctx, s, map[string]string{"id": id}, &out); err != nil {
		return zero, err
	}
	if len(out) == 0 {
		return zero, fmt.Errorf("%s %q: %w", kind, id, ErrNotFound)
	}
	return out[0], nil
}

// Perspective is a built-in (nil ID) or custom OmniFocus perspective.
type Perspective struct {
	ID      *string `json:"id"`
	Name    string  `json:"name"`
	Builtin bool    `json:"builtin"`
}

// Perspectives lists built-in and custom perspectives.
func (b *Bridge) Perspectives(ctx context.Context) ([]Perspective, error) {
	var out []Perspective
	if err := b.call(ctx, scriptListPerspectives, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
