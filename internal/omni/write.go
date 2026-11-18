package omni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Nullable distinguishes an omitted field (Set false) from an explicit null
// (Set and Null), which clears the value.
type Nullable[T any] struct {
	Set, Null bool
	Value     T
}

// NullableString is a clearable string patch field.
type NullableString = Nullable[string]

// sent returns the value of a field set to a value; omitted and null give nil.
func (n Nullable[T]) sent() *T {
	if n.Set && !n.Null {
		return &n.Value
	}
	return nil
}

// IsZero reports an omitted field, so `omitzero` leaves it out of the payload.
func (n Nullable[T]) IsZero() bool { return !n.Set }

// MarshalJSON emits null for an explicit clear, the value otherwise.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Null || !n.Set {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// UnmarshalJSON implements json.Unmarshaler; it only runs for present fields.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Null, n.Value = true, *new(T)
		return nil
	}
	n.Null = false
	return json.Unmarshal(b, &n.Value)
}

// FactAdds are the add fields a task and a project share, the counterpart of
// FactEdits for creating. Every field is a pointer or slice so that nil means
// "not sent": a tag or folder is rejected for any project field the client sent,
// whatever its value. The descriptions are neutral because both hosts take them.
type FactAdds struct {
	Note    *string  `json:"note,omitempty"`
	Flagged *bool    `json:"flagged,omitempty"`
	Defer   *string  `json:"defer,omitempty"`
	Due     *string  `json:"due,omitempty"`
	Planned *string  `json:"planned,omitempty"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tag ids or unambiguous names"`

	EstimatedMinutes    *Minutes    `json:"estimatedMinutes,omitempty"`
	Repetition          *Repetition `json:"repetition,omitempty"`
	CompletedByChildren *bool       `json:"completedByChildren,omitempty" jsonschema:"completes when its last child does"`
}

// TaskAdd is one add_tasks item as received from the client. A validated one
// (validTaskAdd, tags resolved to ids) is also what add_tasks.js reads.
type TaskAdd struct {
	Name    string `json:"name"`
	Project string `json:"project,omitempty" jsonschema:"project id; omit both project and parent for the inbox"`
	Parent  string `json:"parent,omitempty" jsonschema:"parent task id"`

	FactAdds
	Sequential bool `json:"sequential,omitempty" jsonschema:"subtasks run in order"`
}

// FactEdits are the patch fields a task and a project share, with their one
// validation. Omitted means unchanged; null clears. Fields whose description
// differs by object (completedByChildren, tags) stay on the hosts, because the
// descriptions are part of the tool contract.
type FactEdits struct {
	Note    NullableString `json:"note,omitzero"`
	Flagged *bool          `json:"flagged,omitempty"`
	Defer   NullableString `json:"defer,omitzero"`
	Due     NullableString `json:"due,omitzero"`
	Planned NullableString `json:"planned,omitzero"`

	EstimatedMinutes Nullable[Minutes]    `json:"estimatedMinutes,omitzero" jsonschema:"null clears"`
	Repetition       Nullable[Repetition] `json:"repetition,omitzero" jsonschema:"null clears"`
}

// facts are the values a task or project write carries alike, as plain values
// where empty or nil means not sent: the add forms and FactEdits each project
// into it, so one rule set validates dates, estimate and repetition.
type facts struct {
	Defer, Due, Planned string
	Estimate            *Minutes
	Repetition          *Repetition
}

// facts projects the add fields into the shared rule set.
func (f FactAdds) facts() facts {
	return facts{Defer: text(f.Defer), Due: text(f.Due), Planned: text(f.Planned), Estimate: f.EstimatedMinutes, Repetition: f.Repetition}
}

func (f FactAdds) check() error { return f.facts().check() }

func (f facts) check() error {
	var errs []error
	for _, d := range [...]struct{ key, text string }{{"defer", f.Defer}, {"due", f.Due}, {"planned", f.Planned}} {
		_, err := parseDate(d.key, d.text)
		errs = append(errs, err)
	}
	if f.Estimate != nil {
		errs = append(errs, checkBound("estimatedMinutes", *f.Estimate))
	}
	if f.Repetition != nil {
		errs = append(errs, f.Repetition.check())
	}
	return errors.Join(errs...)
}

// check validates the patch fields that need no lookup. A date set to the
// empty string is rejected: null is how a patch clears one.
func (f FactEdits) check() error {
	var errs []error
	for _, d := range [...]struct {
		key string
		n   NullableString
	}{{"defer", f.Defer}, {"due", f.Due}, {"planned", f.Planned}} {
		if d.n.Set && !d.n.Null && d.n.Value == "" {
			errs = append(errs, fmt.Errorf("%s: empty date; use null to clear", d.key))
		}
	}
	return errors.Join(append(errs, facts{
		Defer: text(f.Defer.sent()), Due: text(f.Due.sent()), Planned: text(f.Planned.sent()),
		Estimate: f.EstimatedMinutes.sent(), Repetition: f.Repetition.sent(),
	}.check())...)
}

// Move is a move target: exactly one of Project, Inbox or Parent.
type Move struct {
	Project string `json:"project,omitempty"`
	Inbox   bool   `json:"inbox,omitempty"`
	Parent  string `json:"parent,omitempty"`
}

// TagEdit lists tags to add and remove, by id or unambiguous name.
type TagEdit struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

// TaskEdit is one edit_tasks patch as received from the client. Omitted means
// unchanged; null clears Note, Defer, Due, Planned, EstimatedMinutes and Repetition.
type TaskEdit struct {
	ID         string   `json:"id"`
	Name       *string  `json:"name,omitempty"`
	Complete   bool     `json:"complete,omitempty"`
	Uncomplete bool     `json:"uncomplete,omitempty"`
	Move       *Move    `json:"move,omitempty"`
	Tags       *TagEdit `json:"tags,omitempty"`

	FactEdits
	Sequential          *bool `json:"sequential,omitempty" jsonschema:"subtasks run in order"`
	CompletedByChildren *bool `json:"completedByChildren,omitempty" jsonschema:"completes when its last subtask does"`
	Restore             bool  `json:"restore,omitempty" jsonschema:"un-drop a dropped task; not with complete"`
}

// TaskResult is the outcome of one batch item. Task is the record after the write.
type TaskResult struct {
	ID    string `json:"id,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Task  *Task  `json:"task,omitempty"`
}

// patchItem is a validated edit_tasks patch in script form. Unset clearable
// fields are omitted and cleared ones are null; the script tells them apart by key presence.
type patchItem struct {
	ID   string  `json:"id"`
	Name *string `json:"name,omitempty"`

	FactEdits
	Complete            bool     `json:"complete,omitempty"`
	Uncomplete          bool     `json:"uncomplete,omitempty"`
	Move                *Move    `json:"move,omitempty"`
	TagsAdd             []string `json:"tagsAdd,omitempty"`
	TagsRemove          []string `json:"tagsRemove,omitempty"`
	Sequential          *bool    `json:"sequential,omitempty"`
	CompletedByChildren *bool    `json:"completedByChildren,omitempty"`
	Restore             bool     `json:"restore,omitempty"`
}

// changesNothing reports a patch that sets no field besides its id.
func (p patchItem) changesNothing() bool {
	p.ID = ""
	return reflect.ValueOf(p).IsZero()
}

func nonEmpty(field, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s: must not be empty", field)
	}
	return nil
}

// resolveTags maps ids or exact names to tag ids; unknown or ambiguous refs fail.
// No refs give nil, so an empty list stays an unset field.
func resolveTags(refs []string, tags []Tag) ([]string, error) {
	var ids []string
	for _, r := range refs {
		var hit []string
		for _, t := range tags {
			if t.ID == r {
				hit = []string{t.ID}
				break
			}
			if t.Name == r {
				hit = append(hit, t.ID)
			}
		}
		switch len(hit) {
		case 0:
			return nil, fmt.Errorf("tags: unknown tag %q", r)
		case 1:
			ids = append(ids, hit[0])
		default:
			return nil, fmt.Errorf("tags: %q is ambiguous (%d tags share that name); use an id", r, len(hit))
		}
	}
	return ids, nil
}

// resolveTagEdit maps both lists to tag ids and rejects a tag that is both added and removed.
func resolveTagEdit(te TagEdit, tags []Tag) (TagEdit, error) {
	add, err1 := resolveTags(te.Add, tags)
	rem, err2 := resolveTags(te.Remove, tags)
	errs := []error{err1, err2}
	for _, a := range add {
		if slices.Contains(rem, a) {
			errs = append(errs, fmt.Errorf("tags: %q is both added and removed", a))
		}
	}
	return TagEdit{Add: add, Remove: rem}, errors.Join(errs...)
}

// validTaskAdd validates a and returns its script form: tags resolved to ids.
func validTaskAdd(a TaskAdd, tags []Tag) (TaskAdd, error) {
	if err := errors.Join(nonEmpty("name", a.Name), a.facts().check()); err != nil {
		return TaskAdd{}, err
	}
	if a.Project != "" && a.Parent != "" {
		return TaskAdd{}, errors.New("project and parent are mutually exclusive")
	}
	ids, err := resolveTags(a.Tags, tags)
	if err != nil {
		return TaskAdd{}, err
	}
	a.Tags = ids
	return a, nil
}

// newPatchItem validates p and returns its script form. Nothing reaches a
// script unless every rule holds.
func newPatchItem(p TaskEdit, tags []Tag) (patchItem, error) {
	if err := nonEmpty("id", p.ID); err != nil {
		return patchItem{}, err
	}
	it := patchItem{ID: p.ID, Name: p.Name, Complete: p.Complete, Uncomplete: p.Uncomplete, FactEdits: p.FactEdits,
		Sequential: p.Sequential, CompletedByChildren: p.CompletedByChildren, Restore: p.Restore}
	errs := []error{p.FactEdits.check()}
	if p.Restore && p.Complete {
		errs = append(errs, errors.New("restore and complete are mutually exclusive"))
	}
	if p.Name != nil {
		errs = append(errs, nonEmpty("name", *p.Name))
	}
	if p.Complete && p.Uncomplete {
		errs = append(errs, errors.New("complete and uncomplete are mutually exclusive"))
	}
	if m := p.Move; m != nil {
		n := 0
		for _, set := range []bool{m.Project != "", m.Inbox, m.Parent != ""} {
			if set {
				n++
			}
		}
		if n != 1 {
			errs = append(errs, errors.New("move: exactly one of project, inbox or parent"))
		} else {
			it.Move = m
		}
	}
	if p.Tags != nil {
		te, err := resolveTagEdit(*p.Tags, tags)
		errs = append(errs, err)
		it.TagsAdd, it.TagsRemove = te.Add, te.Remove
	}
	if err := errors.Join(errs...); err != nil {
		return patchItem{}, err
	}
	if it.changesNothing() {
		return patchItem{}, errors.New("patch changes nothing")
	}
	return it, nil
}

// AddTasks creates tasks in one script call. Items failing validation get an
// error result without reaching the script; results keep input order.
func (b *Bridge) AddTasks(ctx context.Context, adds []TaskAdd) ([]TaskResult, error) {
	if len(adds) == 0 {
		return nil, errors.New("tasks: at least one task is required")
	}
	need := false
	for _, a := range adds {
		need = need || len(a.Tags) > 0
	}
	tags, err := b.tagsIf(ctx, need)
	if err != nil {
		return nil, err
	}
	return runBatch(ctx, b, scriptAddTasks, len(adds), nil, failTask, func(i int) (TaskAdd, error) { return validTaskAdd(adds[i], tags) })
}

// EditTasks applies patches in one script call, with the same result contract as AddTasks.
func (b *Bridge) EditTasks(ctx context.Context, patches []TaskEdit) ([]TaskResult, error) {
	if len(patches) == 0 {
		return nil, errors.New("patches: at least one patch is required")
	}
	need := false
	ids := make([]string, len(patches))
	for i, p := range patches {
		ids[i] = p.ID
		need = need || (p.Tags != nil && len(p.Tags.Add)+len(p.Tags.Remove) > 0)
	}
	tags, err := b.tagsIf(ctx, need)
	if err != nil {
		return nil, err
	}
	return runBatch(ctx, b, scriptEditTasks, len(patches), ids, failTask, func(i int) (patchItem, error) { return newPatchItem(patches[i], tags) })
}

func failTask(id, msg string) TaskResult { return TaskResult{ID: id, Error: msg} }

// tagsIf reads the tag list only when some item refers to a tag.
func (b *Bridge) tagsIf(ctx context.Context, need bool) ([]Tag, error) {
	if !need {
		return nil, nil
	}
	return b.ListTags(ctx)
}

// runBatch validates every item, sends the valid ones in a single script call
// and merges script results back into input order. ids (optional) label
// results whose item failed validation.
func runBatch[I, R any](ctx context.Context, b *Bridge, s script, n int, ids []string, fail func(id, msg string) R, build func(int) (I, error)) ([]R, error) {
	results := make([]R, n)
	var items []I
	var slots []int
	for i := range n {
		it, err := build(i)
		if err != nil {
			id := ""
			if ids != nil {
				id = ids[i]
			}
			results[i] = fail(id, err.Error())
			continue
		}
		items, slots = append(items, it), append(slots, i)
	}
	if len(items) == 0 {
		return results, nil
	}
	var got []R
	if err := b.call(ctx, s, struct {
		Items []I `json:"items"`
	}{items}, &got); err != nil {
		return nil, err
	}
	if len(got) != len(items) {
		return nil, fmt.Errorf("%s: script returned %d results for %d items", s.name, len(got), len(items))
	}
	for j, r := range got {
		results[slots[j]] = r
	}
	return results, nil
}
