package omni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// NullableString distinguishes an omitted field (Set false) from an explicit
// null (Set and Null), which clears the value.
type NullableString struct {
	Set, Null bool
	Value     string
}

// IsZero reports an omitted field, so `omitzero` leaves it out of the payload.
func (n NullableString) IsZero() bool { return !n.Set }

// MarshalJSON emits null for an explicit clear, the string otherwise.
func (n NullableString) MarshalJSON() ([]byte, error) {
	if n.Null || !n.Set {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// UnmarshalJSON implements json.Unmarshaler; it only runs for present fields.
func (n *NullableString) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Null, n.Value = true, ""
		return nil
	}
	n.Null = false
	return json.Unmarshal(b, &n.Value)
}

// TaskAdd is one add_tasks item as received from the client.
type TaskAdd struct {
	Name    string   `json:"name"`
	Note    string   `json:"note,omitempty"`
	Flagged bool     `json:"flagged,omitempty"`
	Defer   string   `json:"defer,omitempty"`
	Due     string   `json:"due,omitempty"`
	Planned string   `json:"planned,omitempty"`
	Project string   `json:"project,omitempty" jsonschema:"project id; omit both project and parent for the inbox"`
	Parent  string   `json:"parent,omitempty" jsonschema:"parent task id"`
	Tags    []string `json:"tags,omitempty" jsonschema:"tag ids or unambiguous names"`
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
// unchanged; null clears Note, Defer, Due and Planned.
type TaskEdit struct {
	ID         string         `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Note       NullableString `json:"note,omitzero"`
	Flagged    *bool          `json:"flagged,omitempty"`
	Defer      NullableString `json:"defer,omitzero"`
	Due        NullableString `json:"due,omitzero"`
	Planned    NullableString `json:"planned,omitzero"`
	Complete   bool           `json:"complete,omitempty"`
	Uncomplete bool           `json:"uncomplete,omitempty"`
	Move       *Move          `json:"move,omitempty"`
	Tags       *TagEdit       `json:"tags,omitempty"`
}

// TaskResult is the outcome of one batch item. Task is the record after the write.
type TaskResult struct {
	ID    string `json:"id,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Task  *Task  `json:"task,omitempty"`
}

// addItem is a validated add_tasks item in script form: the keys add_tasks.js
// reads, with tags resolved to ids.
type addItem struct {
	Name    string   `json:"name"`
	Note    string   `json:"note,omitempty"`
	Flagged bool     `json:"flagged,omitempty"`
	Defer   string   `json:"defer,omitempty"`
	Due     string   `json:"due,omitempty"`
	Planned string   `json:"planned,omitempty"`
	Project string   `json:"project,omitempty"`
	Parent  string   `json:"parent,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// patchItem is a validated edit_tasks patch in script form. Unset clearable
// fields are omitted and cleared ones are null; the script tells them apart by key presence.
type patchItem struct {
	ID         string         `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Note       NullableString `json:"note,omitzero"`
	Flagged    *bool          `json:"flagged,omitempty"`
	Defer      NullableString `json:"defer,omitzero"`
	Due        NullableString `json:"due,omitzero"`
	Planned    NullableString `json:"planned,omitzero"`
	Complete   bool           `json:"complete,omitempty"`
	Uncomplete bool           `json:"uncomplete,omitempty"`
	Move       *Move          `json:"move,omitempty"`
	TagsAdd    []string       `json:"tagsAdd,omitempty"`
	TagsRemove []string       `json:"tagsRemove,omitempty"`
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
func resolveTags(refs []string, tags []Tag) ([]string, error) {
	ids := make([]string, 0, len(refs))
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

// newAddItem validates a and returns its script form.
func newAddItem(a TaskAdd, tags []Tag) (addItem, error) {
	if err := errors.Join(nonEmpty("name", a.Name), checkDate("defer", a.Defer), checkDate("due", a.Due), checkDate("planned", a.Planned)); err != nil {
		return addItem{}, err
	}
	if a.Project != "" && a.Parent != "" {
		return addItem{}, errors.New("project and parent are mutually exclusive")
	}
	ids, err := resolveTags(a.Tags, tags)
	if err != nil {
		return addItem{}, err
	}
	return addItem{Name: a.Name, Note: a.Note, Flagged: a.Flagged, Defer: a.Defer, Due: a.Due, Planned: a.Planned, Project: a.Project, Parent: a.Parent, Tags: ids}, nil
}

// checkEditDate validates one clearable date patch; unset and null pass.
func checkEditDate(key string, n NullableString) error {
	if !n.Set || n.Null {
		return nil
	}
	if n.Value == "" {
		return fmt.Errorf("%s: empty date; use null to clear", key)
	}
	return checkDate(key, n.Value)
}

// newPatchItem validates p and returns its script form. Nothing reaches a
// script unless every rule holds.
func newPatchItem(p TaskEdit, tags []Tag) (patchItem, error) {
	if err := nonEmpty("id", p.ID); err != nil {
		return patchItem{}, err
	}
	it := patchItem{ID: p.ID, Name: p.Name, Note: p.Note, Flagged: p.Flagged, Defer: p.Defer, Due: p.Due, Planned: p.Planned, Complete: p.Complete, Uncomplete: p.Uncomplete}
	errs := []error{checkEditDate("defer", p.Defer), checkEditDate("due", p.Due), checkEditDate("planned", p.Planned)}
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
		add, err1 := resolveTags(p.Tags.Add, tags)
		rem, err2 := resolveTags(p.Tags.Remove, tags)
		errs = append(errs, err1, err2)
		for _, a := range add {
			for _, r := range rem {
				if a == r {
					errs = append(errs, fmt.Errorf("tags: %q is both added and removed", a))
				}
			}
		}
		if len(add) > 0 {
			it.TagsAdd = add
		}
		if len(rem) > 0 {
			it.TagsRemove = rem
		}
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
	return runBatch(ctx, b, scriptAddTasks, len(adds), nil, failTask, func(i int) (addItem, error) { return newAddItem(adds[i], tags) })
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
	if err := b.Call(ctx, s, struct {
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
