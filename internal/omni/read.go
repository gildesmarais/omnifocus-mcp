package omni

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
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

	EstimatedMinutes    *int        `json:"estimatedMinutes,omitempty"`
	Repetition          *Repetition `json:"repetition,omitempty"`
	HasChildren         bool        `json:"hasChildren"`
	Sequential          bool        `json:"sequential"`
	CompletedByChildren bool        `json:"completedByChildren"`
	DropDate            string      `json:"dropDate,omitempty"`
	Added               string      `json:"added"`
	Modified            string      `json:"modified"`
}

// Repetition is a repeat rule, as read from OmniFocus and as written: an RRULE
// string, how the next occurrence is scheduled (ScheduleType), which date the
// rule moves (Anchor) and whether missed occurrences are skipped (CatchUp). It
// owns the shape and its validation for every object that repeats.
type Repetition struct {
	Rule         string `json:"rule" jsonschema:"RRULE string, e.g. FREQ=WEEKLY;INTERVAL=1;BYDAY=FR"`
	ScheduleType string `json:"scheduleType" jsonschema:"Regularly (fixed schedule) or FromCompletion (counts from completion)"`
	Anchor       string `json:"anchor" jsonschema:"the date the rule moves: DeferDate, DueDate or PlannedDate"`
	CatchUp      bool   `json:"catchUp" jsonschema:"skip occurrences already past"`
}

// Names a write may set. OmniFocus also has a None schedule type, but creating a
// rule with it throws, so it is not accepted. Go validates against these sets
// before the scripts index OmniFocus's enums by name.
var (
	repetitionScheduleTypes = []string{"Regularly", "FromCompletion"}
	repetitionAnchors       = []string{"DeferDate", "DueDate", "PlannedDate"}
)

func (r Repetition) check() error {
	var errs []error
	if strings.TrimSpace(r.Rule) == "" {
		errs = append(errs, errors.New("repetition.rule: must not be empty"))
	}
	if !slices.Contains(repetitionScheduleTypes, r.ScheduleType) {
		errs = append(errs, fmt.Errorf("repetition.scheduleType: %q must be one of %v", r.ScheduleType, repetitionScheduleTypes))
	}
	if !slices.Contains(repetitionAnchors, r.Anchor) {
		errs = append(errs, fmt.Errorf("repetition.anchor: %q must be one of %v", r.Anchor, repetitionAnchors))
	}
	return errors.Join(errs...)
}

// checkEstimate validates an estimate in minutes.
func checkEstimate(m int) error {
	if m < 0 {
		return fmt.Errorf("estimatedMinutes: %d must be >= 0", m)
	}
	return nil
}

// ReviewInterval is how often a project is reviewed. Unit is one of days, weeks,
// months or years (OmniFocus also reports hours on read).
type ReviewInterval struct {
	Steps int    `json:"steps" jsonschema:"1 or more"`
	Unit  string `json:"unit" jsonschema:"days, weeks, months or years"`
}

// reviewUnits are the units a write may set; OmniFocus silently resets any other spelling to 1 week.
var reviewUnits = []string{"days", "weeks", "months", "years"}

func (r ReviewInterval) check() error {
	if r.Steps < 1 {
		return fmt.Errorf("reviewInterval.steps: %d must be >= 1", r.Steps)
	}
	if !slices.Contains(reviewUnits, r.Unit) {
		return fmt.Errorf("reviewInterval.unit: %q must be one of %v", r.Unit, reviewUnits)
	}
	return nil
}

// Project is a project with its folder, tags, dates and review schedule.
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Note        string `json:"note,omitempty"`
	Status      string `json:"status"`
	Flagged     bool   `json:"flagged"`
	Type        string `json:"type" jsonschema:"Parallel, Sequential or SingleActions"`
	Available   bool   `json:"available"`
	Blocked     bool   `json:"blocked"`
	Folder      *Ref   `json:"folder,omitempty"`
	Tags        []Ref  `json:"tags"`
	Defer       string `json:"defer,omitempty"`
	Due         string `json:"due,omitempty"`
	Planned     string `json:"planned,omitempty"`
	CompletedAt string `json:"completedAt,omitempty"`

	EstimatedMinutes    *int        `json:"estimatedMinutes,omitempty"`
	Repetition          *Repetition `json:"repetition,omitempty"`
	CompletedByChildren bool        `json:"completedByChildren"`

	ReviewInterval *ReviewInterval `json:"reviewInterval,omitempty"`
	NextReview     string          `json:"nextReviewDate,omitempty"`
	LastReview     string          `json:"lastReviewDate,omitempty"`
}

// Project types, the one owner of the names: a project is Parallel,
// Sequential (actions run in order) or SingleActions (a list of unrelated actions).
var projectTypes = []string{"Parallel", "Sequential", "SingleActions"}

func checkProjectType(t string) error {
	if !slices.Contains(projectTypes, t) {
		return fmt.Errorf("type: %q must be one of %v", t, projectTypes)
	}
	return nil
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
// Nothing hides completed or dropped tasks unless a filter asks for it, so the
// completed_* bounds and dropped need no switch to be found.
type TaskFilter struct {
	Inbox     *bool  `json:"inbox,omitempty" jsonschema:"only inbox tasks (true) or only non-inbox tasks (false)"`
	Project   string `json:"project,omitempty" jsonschema:"project id or exact name"`
	Tag       string `json:"tag,omitempty" jsonschema:"tag id or exact name"`
	Available *bool  `json:"available,omitempty" jsonschema:"true: not blocked, deferred, completed or dropped"`
	Flagged   *bool  `json:"flagged,omitempty"`
	Completed *bool  `json:"completed,omitempty" jsonschema:"only completed tasks (true) or only not-completed tasks (false)"`
	Untagged  *bool  `json:"untagged,omitempty" jsonschema:"only tasks without tags (true) or only tagged tasks (false)"`
	Repeating *bool  `json:"repeating,omitempty" jsonschema:"only tasks with a repetition rule (true) or only non-repeating tasks (false)"`
	Dropped   *bool  `json:"dropped,omitempty" jsonschema:"only dropped tasks (true) or only not-dropped tasks (false)"`

	DueBefore       string `json:"due_before,omitempty" jsonschema:"due on or before; a date-only value covers its whole day. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	DueAfter        string `json:"due_after,omitempty" jsonschema:"due on or after; a date-only value starts at 00:00. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	DeferBefore     string `json:"defer_before,omitempty" jsonschema:"defer date on or before; a date-only value covers its whole day. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	DeferAfter      string `json:"defer_after,omitempty" jsonschema:"defer date on or after; a date-only value starts at 00:00. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	PlannedBefore   string `json:"planned_before,omitempty" jsonschema:"planned date on or before; a date-only value covers its whole day. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	PlannedAfter    string `json:"planned_after,omitempty" jsonschema:"planned date on or after; a date-only value starts at 00:00. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	CompletedBefore string `json:"completed_before,omitempty" jsonschema:"completed on or before; a date-only value covers its whole day. Matches completed tasks only. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	CompletedAfter  string `json:"completed_after,omitempty" jsonschema:"completed on or after; a date-only value starts at 00:00. Matches completed tasks only. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	ModifiedAfter   string `json:"modified_after,omitempty" jsonschema:"modified on or after; a date-only value starts at 00:00. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
	AddedAfter      string `json:"added_after,omitempty" jsonschema:"added on or after; a date-only value starts at 00:00. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`

	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring over name and note"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"max results (default 50); null returns all"`
}

// ProjectFilter holds list_projects filters.
type ProjectFilter struct {
	Status string `json:"status,omitempty" jsonschema:"Active, OnHold, Done or Dropped"`
	Folder string `json:"folder,omitempty" jsonschema:"folder id or exact name; includes subfolders"`
	Search string `json:"search,omitempty" jsonschema:"case-insensitive substring over name and note"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"max results (default 50); null returns all"`

	ReviewDueBefore string `json:"review_due_before,omitempty" jsonschema:"next review on or before; a date-only value covers its whole day. YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS (local time), or RFC 3339 with offset"`
}

var projectStatuses = map[string]bool{"Active": true, "OnHold": true, "Done": true, "Dropped": true}

// dateOnly is the YYYY-MM-DD layout; in a write it means 00:00 local time.
const dateOnly = "2006-01-02"

// parseDate parses the accepted date forms: YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS
// (local time) or RFC 3339 with an offset. The empty string is the zero time.
func parseDate(field, s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{dateOnly, "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%s: %q is not YYYY-MM-DD, YYYY-MM-DDTHH:MM:SS or an RFC 3339 timestamp with offset (Z, +01:00)", field, s)
}

func checkDate(field, s string) error {
	_, err := parseDate(field, s)
	return err
}

// resolveBound validates one range bound and rewrites *s in place to a full
// timestamp, so the scripts compare instants and never expand a date-only
// value themselves. It is the one owner of the rule: a date-only value covers
// its whole local day, so an _after bound starts at 00:00:00 and a _before
// bound ends at 23:59:59 of it. A nil or empty bound is absent (zero time).
func resolveBound(field string, s *string, end bool) (time.Time, error) {
	if s == nil || *s == "" {
		return time.Time{}, nil
	}
	t, err := parseDate(field, *s)
	if err != nil || len(*s) != len(dateOnly) {
		return t, err
	}
	if end {
		*s += "T23:59:59"
	} else {
		*s += "T00:00:00"
	}
	return parseDate(field, *s)
}

// resolveRange resolves the _after/_before pair of one date field (either may
// be nil) and rejects an _after later than its _before.
func resolveRange(field string, after, before *string) error {
	lo, errLo := resolveBound(field+"_after", after, false)
	hi, errHi := resolveBound(field+"_before", before, true)
	if err := errors.Join(errLo, errHi); err != nil {
		return err
	}
	if !lo.IsZero() && !hi.IsZero() && lo.After(hi) {
		return fmt.Errorf("%s_after: %s is later than %s_before %s", field, *after, field, *before)
	}
	return nil
}

func checkLimit(l *int) error {
	if l != nil && *l < 0 {
		return fmt.Errorf("limit: must be >= 0 or null")
	}
	return nil
}

// resolve validates f and rewrites its date bounds to full timestamps (see
// resolveBound). It rejects contradictions: the completed_* bounds match
// completed tasks only, completed and dropped tasks are disjoint, and
// available excludes both.
func (f *TaskFilter) resolve() error {
	errs := []error{checkLimit(f.Limit)}
	for _, r := range []struct {
		field         string
		after, before *string
	}{
		{"due", &f.DueAfter, &f.DueBefore},
		{"defer", &f.DeferAfter, &f.DeferBefore},
		{"planned", &f.PlannedAfter, &f.PlannedBefore},
		{"completed", &f.CompletedAfter, &f.CompletedBefore},
		{"modified", &f.ModifiedAfter, nil},
		{"added", &f.AddedAfter, nil},
	} {
		errs = append(errs, resolveRange(r.field, r.after, r.before))
	}
	is := func(b *bool) bool { return b != nil && *b }
	completedBound := f.CompletedAfter != "" || f.CompletedBefore != ""
	if is(f.Inbox) && f.Project != "" {
		errs = append(errs, errors.New("inbox: cannot be combined with project (inbox tasks belong to no project)"))
	}
	if f.Completed != nil && !*f.Completed && completedBound {
		errs = append(errs, errors.New("completed: false contradicts completed_after/completed_before, which match completed tasks"))
	}
	if (is(f.Completed) || completedBound) && is(f.Dropped) {
		errs = append(errs, errors.New("dropped: true contradicts completed filters (a task is not both completed and dropped)"))
	}
	if is(f.Available) && (is(f.Completed) || completedBound || is(f.Dropped)) {
		errs = append(errs, errors.New("available: true contradicts completed or dropped filters (available excludes completed and dropped tasks)"))
	}
	return errors.Join(errs...)
}

// ListTasks returns tasks matching f (AND-combined).
func (b *Bridge) ListTasks(ctx context.Context, f TaskFilter) ([]Task, error) {
	if err := f.resolve(); err != nil {
		return nil, err
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
	_, errDue := resolveBound("review_due_before", &f.ReviewDueBefore, true)
	if err := errors.Join(errDue, checkLimit(f.Limit)); err != nil {
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
