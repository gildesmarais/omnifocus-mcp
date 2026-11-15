package omni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// NullableString distinguishes an omitted field (Set false) from an explicit
// null (Set and Null), which clears the value.
type NullableString struct {
	Set, Null bool
	Value     string
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

// writeItem is a validated item in script form: only the keys the script acts on.
type writeItem map[string]any

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
func newAddItem(a TaskAdd, tags []Tag) (writeItem, error) {
	if err := errors.Join(nonEmpty("name", a.Name), checkDate("defer", a.Defer), checkDate("due", a.Due), checkDate("planned", a.Planned)); err != nil {
		return nil, err
	}
	if a.Project != "" && a.Parent != "" {
		return nil, errors.New("project and parent are mutually exclusive")
	}
	ids, err := resolveTags(a.Tags, tags)
	if err != nil {
		return nil, err
	}
	it := writeItem{"name": a.Name}
	for k, v := range map[string]string{"note": a.Note, "defer": a.Defer, "due": a.Due, "planned": a.Planned, "project": a.Project, "parent": a.Parent} {
		if v != "" {
			it[k] = v
		}
	}
	if a.Flagged {
		it["flagged"] = true
	}
	if len(ids) > 0 {
		it["tags"] = ids
	}
	return it, nil
}

// editDate validates one clearable date patch and records it in it.
func editDate(it writeItem, key string, n NullableString) error {
	switch {
	case !n.Set:
		return nil
	case n.Null:
		it[key] = nil
		return nil
	case n.Value == "":
		return fmt.Errorf("%s: empty date; use null to clear", key)
	}
	if err := checkDate(key, n.Value); err != nil {
		return err
	}
	it[key] = n.Value
	return nil
}

// newPatchItem validates p and returns its script form. Nothing reaches a
// script unless every rule holds.
func newPatchItem(p TaskEdit, tags []Tag) (writeItem, error) {
	if err := nonEmpty("id", p.ID); err != nil {
		return nil, err
	}
	it := writeItem{"id": p.ID}
	errs := []error{editDate(it, "defer", p.Defer), editDate(it, "due", p.Due), editDate(it, "planned", p.Planned)}
	if p.Name != nil {
		errs = append(errs, nonEmpty("name", *p.Name))
		it["name"] = *p.Name
	}
	if p.Note.Set {
		if p.Note.Null {
			it["note"] = nil
		} else {
			it["note"] = p.Note.Value
		}
	}
	if p.Flagged != nil {
		it["flagged"] = *p.Flagged
	}
	if p.Complete && p.Uncomplete {
		errs = append(errs, errors.New("complete and uncomplete are mutually exclusive"))
	}
	if p.Complete {
		it["complete"] = true
	}
	if p.Uncomplete {
		it["uncomplete"] = true
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
			it["move"] = m
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
			it["tagsAdd"] = add
		}
		if len(rem) > 0 {
			it["tagsRemove"] = rem
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if len(it) == 1 {
		return nil, errors.New("patch changes nothing")
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
	return runBatch(ctx, b, "add_tasks", len(adds), nil, failTask, func(i int) (writeItem, error) { return newAddItem(adds[i], tags) })
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
	return runBatch(ctx, b, "edit_tasks", len(patches), ids, failTask, func(i int) (writeItem, error) { return newPatchItem(patches[i], tags) })
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
func runBatch[R any](ctx context.Context, b *Bridge, script string, n int, ids []string, fail func(id, msg string) R, build func(int) (writeItem, error)) ([]R, error) {
	results := make([]R, n)
	var items []writeItem
	var slots []int
	for i := 0; i < n; i++ {
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
	if err := b.Call(ctx, script, map[string]any{"items": items}, &got); err != nil {
		return nil, err
	}
	if len(got) != len(items) {
		return nil, fmt.Errorf("%s: script returned %d results for %d items", script, len(got), len(items))
	}
	for j, r := range got {
		results[slots[j]] = r
	}
	return results, nil
}
