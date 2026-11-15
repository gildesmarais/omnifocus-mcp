package omni

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// Kinds a structure tool operates on.
const (
	kindTask    = "task"
	kindProject = "project"
	kindTag     = "tag"
	kindFolder  = "folder"
)

// StructAdd is one add item: a project, tag or folder.
type StructAdd struct {
	Kind       string `json:"kind" jsonschema:"project, tag or folder"`
	Name       string `json:"name"`
	Parent     string `json:"parent,omitempty" jsonschema:"folder id (project, folder) or tag id (tag); omit for the root"`
	Status     string `json:"status,omitempty" jsonschema:"project: Active, OnHold, Done; tag: Active, OnHold"`
	Sequential bool   `json:"sequential,omitempty" jsonschema:"project only: sequential instead of parallel"`
}

// StructMove re-parents an object; parent null moves it to the root.
type StructMove struct {
	Parent NullableString `json:"parent"`
}

// StructReorder places an object next to a sibling of the same kind; exactly one of Before or After.
type StructReorder struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// StructEdit is one edit item. Omitted means unchanged.
type StructEdit struct {
	Kind       string         `json:"kind" jsonschema:"project, tag or folder"`
	ID         string         `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Status     string         `json:"status,omitempty" jsonschema:"project: Active, OnHold, Done; tag: Active, OnHold (Active re-activates a dropped tag); folders have no status edit"`
	Sequential *bool          `json:"sequential,omitempty" jsonschema:"project only"`
	Move       *StructMove    `json:"move,omitempty" jsonschema:"parent: folder id (project, folder), tag id (tag) or null for the root"`
	Reorder    *StructReorder `json:"reorder,omitempty" jsonschema:"mutually exclusive with move; the sibling's parent becomes the parent"`
}

// StructRef names one object for drop or delete.
type StructRef struct {
	Kind string `json:"kind" jsonschema:"task, project, tag or folder"`
	ID   string `json:"id"`
}

// StructResult is the outcome of one structure batch item. Name, Status and
// Parent describe the record after the write; delete returns only id and kind.
type StructResult struct {
	ID     string `json:"id,omitempty"`
	Kind   string `json:"kind,omitempty"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
	Parent *Ref   `json:"parent,omitempty"`
}

func failStruct(id, msg string) StructResult { return StructResult{ID: id, Error: msg} }

func checkKind(kind string, allowed ...string) error {
	if !slices.Contains(allowed, kind) {
		return fmt.Errorf("kind: %q is not one of %v", kind, allowed)
	}
	return nil
}

// checkStatus validates a status for kind; empty means unset.
func checkStatus(kind, status string) error {
	allowed := map[string][]string{kindProject: {"Active", "OnHold", "Done"}, kindTag: {"Active", "OnHold"}}[kind]
	switch {
	case status == "":
		return nil
	case len(allowed) == 0:
		return fmt.Errorf("status: not supported for %s", kind)
	case !slices.Contains(allowed, status):
		return fmt.Errorf("status: %q is not one of %v for a %s", status, allowed, kind)
	}
	return nil
}

func newStructAddItem(a StructAdd) (writeItem, error) {
	if err := errors.Join(checkKind(a.Kind, kindProject, kindTag, kindFolder), nonEmpty("name", a.Name), checkStatus(a.Kind, a.Status)); err != nil {
		return nil, err
	}
	if a.Sequential && a.Kind != kindProject {
		return nil, errors.New("sequential: only for projects")
	}
	it := writeItem{"kind": a.Kind, "name": a.Name}
	for k, v := range map[string]string{"parent": a.Parent, "status": a.Status} {
		if v != "" {
			it[k] = v
		}
	}
	if a.Sequential {
		it["sequential"] = true
	}
	return it, nil
}

func newStructEditItem(p StructEdit) (writeItem, error) {
	if err := errors.Join(checkKind(p.Kind, kindProject, kindTag, kindFolder), nonEmpty("id", p.ID), checkStatus(p.Kind, p.Status)); err != nil {
		return nil, err
	}
	it := writeItem{"kind": p.Kind, "id": p.ID}
	if p.Name != nil {
		if err := nonEmpty("name", *p.Name); err != nil {
			return nil, err
		}
		it["name"] = *p.Name
	}
	if p.Status != "" {
		it["status"] = p.Status
	}
	if p.Sequential != nil {
		if p.Kind != kindProject {
			return nil, errors.New("sequential: only for projects")
		}
		it["sequential"] = *p.Sequential
	}
	if p.Move != nil && p.Reorder != nil {
		return nil, errors.New("move and reorder are mutually exclusive")
	}
	if m := p.Move; m != nil {
		switch {
		case !m.Parent.Set:
			return nil, errors.New("move: parent is required (an id, or null for the root)")
		case m.Parent.Null:
			it["move"] = map[string]any{"parent": nil}
		case m.Parent.Value == "":
			return nil, errors.New("move: empty parent; use null for the root")
		case m.Parent.Value == p.ID:
			return nil, errors.New("move: an object cannot be its own parent")
		default:
			it["move"] = map[string]any{"parent": m.Parent.Value}
		}
	}
	if r := p.Reorder; r != nil {
		if (r.Before == "") == (r.After == "") {
			return nil, errors.New("reorder: exactly one of before or after")
		}
		if r.Before == p.ID || r.After == p.ID {
			return nil, errors.New("reorder: an object cannot be its own sibling")
		}
		it["reorder"] = r
	}
	if len(it) == 2 {
		return nil, errors.New("patch changes nothing")
	}
	return it, nil
}

func newStructRefItem(r StructRef) (writeItem, error) {
	if err := errors.Join(checkKind(r.Kind, kindTask, kindProject, kindTag, kindFolder), nonEmpty("id", r.ID)); err != nil {
		return nil, err
	}
	return writeItem{"kind": r.Kind, "id": r.ID}, nil
}

// AddStructure creates projects, tags and folders in one script call; results keep input order.
func (b *Bridge) AddStructure(ctx context.Context, adds []StructAdd) ([]StructResult, error) {
	if len(adds) == 0 {
		return nil, errors.New("items: at least one item is required")
	}
	return runBatch(ctx, b, "add_structure", len(adds), nil, failStruct, func(i int) (writeItem, error) { return newStructAddItem(adds[i]) })
}

// EditStructure patches projects, tags and folders, with the same result contract as AddStructure.
func (b *Bridge) EditStructure(ctx context.Context, patches []StructEdit) ([]StructResult, error) {
	if len(patches) == 0 {
		return nil, errors.New("items: at least one item is required")
	}
	ids := make([]string, len(patches))
	for i, p := range patches {
		ids[i] = p.ID
	}
	return runBatch(ctx, b, "edit_structure", len(patches), ids, failStruct, func(i int) (writeItem, error) { return newStructEditItem(patches[i]) })
}

// DropObjects drops tasks, projects, tags or folders. Guards (a tag with
// remaining tasks) are enforced in the script, the only place that can see them.
func (b *Bridge) DropObjects(ctx context.Context, refs []StructRef) ([]StructResult, error) {
	return b.refBatch(ctx, "drop", refs)
}

// DeleteObjects irreversibly deletes objects. Tags and folders must already be
// dropped and empty; the script enforces that.
func (b *Bridge) DeleteObjects(ctx context.Context, refs []StructRef) ([]StructResult, error) {
	return b.refBatch(ctx, "delete", refs)
}

func (b *Bridge) refBatch(ctx context.Context, script string, refs []StructRef) ([]StructResult, error) {
	if len(refs) == 0 {
		return nil, errors.New("items: at least one item is required")
	}
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	return runBatch(ctx, b, script, len(refs), ids, failStruct, func(i int) (writeItem, error) { return newStructRefItem(refs[i]) })
}
