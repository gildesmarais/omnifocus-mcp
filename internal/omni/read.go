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
// Limit nil means all tasks; callers choose the default.
type TaskFilter struct {
	Inbox       *bool  `json:"inbox,omitempty"`
	Project     string `json:"project,omitempty"`
	Tag         string `json:"tag,omitempty"`
	Available   *bool  `json:"available,omitempty"`
	Flagged     *bool  `json:"flagged,omitempty"`
	Completed   *bool  `json:"completed,omitempty"`
	DueBefore   string `json:"due_before,omitempty"`
	DeferBefore string `json:"defer_before,omitempty"`
	Search      string `json:"search,omitempty"`
	Limit       *int   `json:"limit"`
}

// ProjectFilter holds list_projects filters.
type ProjectFilter struct {
	Status string `json:"status,omitempty"`
	Folder string `json:"folder,omitempty"`
	Search string `json:"search,omitempty"`
	Limit  *int   `json:"limit"`
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
	if err := b.Call(ctx, "tasks", f, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTask returns one task or ErrNotFound.
func (b *Bridge) GetTask(ctx context.Context, id string) (Task, error) {
	return getOne[Task](ctx, b, "tasks", "task", id)
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
	if err := b.Call(ctx, "projects", f, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetProject returns one project or ErrNotFound.
func (b *Bridge) GetProject(ctx context.Context, id string) (Project, error) {
	return getOne[Project](ctx, b, "projects", "project", id)
}

// ListTags returns all tags.
func (b *Bridge) ListTags(ctx context.Context) ([]Tag, error) {
	var out []Tag
	if err := b.Call(ctx, "tags", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTag returns one tag or ErrNotFound.
func (b *Bridge) GetTag(ctx context.Context, id string) (Tag, error) {
	return getOne[Tag](ctx, b, "tags", "tag", id)
}

// ListFolders returns all folders.
func (b *Bridge) ListFolders(ctx context.Context) ([]Folder, error) {
	var out []Folder
	if err := b.Call(ctx, "folders", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFolder returns one folder or ErrNotFound.
func (b *Bridge) GetFolder(ctx context.Context, id string) (Folder, error) {
	return getOne[Folder](ctx, b, "folders", "folder", id)
}

// getOne runs a read script with {id} and unwraps its 0-or-1 element result.
func getOne[T any](ctx context.Context, b *Bridge, script, kind, id string) (T, error) {
	var zero T
	if id == "" {
		return zero, fmt.Errorf("%s id is required", kind)
	}
	var out []T
	if err := b.Call(ctx, script, map[string]string{"id": id}, &out); err != nil {
		return zero, err
	}
	if len(out) == 0 {
		return zero, fmt.Errorf("%s %q: %w", kind, id, ErrNotFound)
	}
	return out[0], nil
}
