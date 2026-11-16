package omni

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
)

// Kind names the object type a structure item targets. It stays a plain string
// on the wire: unknown values are rejected per item by the constructors, never
// by decoding or the schema, so one bad item cannot fail the whole call.
type Kind string

// Status is a project or tag status as the client spells it.
type Status string

// Kinds a structure tool operates on.
const (
	kindTask    Kind = "task"
	kindProject Kind = "project"
	kindTag     Kind = "tag"
	kindFolder  Kind = "folder"
)

// Statuses a structure item may set; which apply depends on the kind.
const (
	statusActive Status = "Active"
	statusOnHold Status = "OnHold"
	statusDone   Status = "Done"
)

// StructAdd is one add item: a project, tag or folder.
type StructAdd struct {
	Kind       Kind   `json:"kind" jsonschema:"project, tag or folder"`
	Name       string `json:"name"`
	Parent     string `json:"parent,omitempty" jsonschema:"folder id (project, folder) or tag id (tag); omit for the root"`
	Status     Status `json:"status,omitempty" jsonschema:"project: Active, OnHold, Done; tag: Active, OnHold"`
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
	Kind       Kind           `json:"kind" jsonschema:"project, tag or folder"`
	ID         string         `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Status     Status         `json:"status,omitempty" jsonschema:"project: Active, OnHold, Done; tag: Active, OnHold (Active re-activates a dropped tag); folders have no status edit"`
	Sequential *bool          `json:"sequential,omitempty" jsonschema:"project only"`
	Move       *StructMove    `json:"move,omitempty" jsonschema:"parent: folder id (project, folder), tag id (tag) or null for the root"`
	Reorder    *StructReorder `json:"reorder,omitempty" jsonschema:"mutually exclusive with move; the sibling's parent becomes the parent"`
}

// StructRef names one object for drop or delete.
type StructRef struct {
	Kind Kind   `json:"kind" jsonschema:"task, project, tag or folder"`
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

func checkKind(kind Kind, allowed ...Kind) error {
	if !slices.Contains(allowed, kind) {
		return fmt.Errorf("kind: %q is not one of %v", kind, allowed)
	}
	return nil
}

// checkStatus validates a status for kind; empty means unset.
func checkStatus(kind Kind, status Status) error {
	allowed := map[Kind][]Status{kindProject: {statusActive, statusOnHold, statusDone}, kindTag: {statusActive, statusOnHold}}[kind]
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

// structAddItem is a validated add_structure item in script form.
type structAddItem struct {
	Kind       Kind   `json:"kind"`
	Name       string `json:"name"`
	Parent     string `json:"parent,omitempty"`
	Status     Status `json:"status,omitempty"`
	Sequential bool   `json:"sequential,omitempty"`
}

// structEditItem is a validated edit_structure patch in script form. Move
// carries its parent as an explicit null for the root.
type structEditItem struct {
	Kind       Kind           `json:"kind"`
	ID         string         `json:"id"`
	Name       *string        `json:"name,omitempty"`
	Status     Status         `json:"status,omitempty"`
	Sequential *bool          `json:"sequential,omitempty"`
	Move       *StructMove    `json:"move,omitempty"`
	Reorder    *StructReorder `json:"reorder,omitempty"`
}

// changesNothing reports a patch that sets no field besides kind and id.
func (p structEditItem) changesNothing() bool {
	p.Kind, p.ID = "", ""
	return reflect.ValueOf(p).IsZero()
}

// structRefItem is a validated drop or delete item in script form.
type structRefItem struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id"`
}

func newStructAddItem(a StructAdd) (structAddItem, error) {
	if err := errors.Join(checkKind(a.Kind, kindProject, kindTag, kindFolder), nonEmpty("name", a.Name), checkStatus(a.Kind, a.Status)); err != nil {
		return structAddItem{}, err
	}
	if a.Sequential && a.Kind != kindProject {
		return structAddItem{}, errors.New("sequential: only for projects")
	}
	return structAddItem(a), nil
}

func newStructEditItem(p StructEdit) (structEditItem, error) {
	if err := errors.Join(checkKind(p.Kind, kindProject, kindTag, kindFolder), nonEmpty("id", p.ID), checkStatus(p.Kind, p.Status)); err != nil {
		return structEditItem{}, err
	}
	if p.Name != nil {
		if err := nonEmpty("name", *p.Name); err != nil {
			return structEditItem{}, err
		}
	}
	if p.Sequential != nil && p.Kind != kindProject {
		return structEditItem{}, errors.New("sequential: only for projects")
	}
	if p.Move != nil && p.Reorder != nil {
		return structEditItem{}, errors.New("move and reorder are mutually exclusive")
	}
	if m := p.Move; m != nil {
		switch {
		case !m.Parent.Set:
			return structEditItem{}, errors.New("move: parent is required (an id, or null for the root)")
		case !m.Parent.Null && m.Parent.Value == "":
			return structEditItem{}, errors.New("move: empty parent; use null for the root")
		case !m.Parent.Null && m.Parent.Value == p.ID:
			return structEditItem{}, errors.New("move: an object cannot be its own parent")
		}
	}
	if r := p.Reorder; r != nil {
		if (r.Before == "") == (r.After == "") {
			return structEditItem{}, errors.New("reorder: exactly one of before or after")
		}
		if r.Before == p.ID || r.After == p.ID {
			return structEditItem{}, errors.New("reorder: an object cannot be its own sibling")
		}
	}
	it := structEditItem(p)
	if it.changesNothing() {
		return structEditItem{}, errors.New("patch changes nothing")
	}
	return it, nil
}

func newStructRefItem(r StructRef) (structRefItem, error) {
	if err := errors.Join(checkKind(r.Kind, kindTask, kindProject, kindTag, kindFolder), nonEmpty("id", r.ID)); err != nil {
		return structRefItem{}, err
	}
	return structRefItem{Kind: r.Kind, ID: r.ID}, nil
}

// AddStructure creates projects, tags and folders in one script call; results keep input order.
func (b *Bridge) AddStructure(ctx context.Context, adds []StructAdd) ([]StructResult, error) {
	if len(adds) == 0 {
		return nil, errors.New("items: at least one item is required")
	}
	return runBatch(ctx, b, "add_structure", len(adds), nil, failStruct, func(i int) (structAddItem, error) { return newStructAddItem(adds[i]) })
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
	return runBatch(ctx, b, "edit_structure", len(patches), ids, failStruct, func(i int) (structEditItem, error) { return newStructEditItem(patches[i]) })
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
	return runBatch(ctx, b, script, len(refs), ids, failStruct, func(i int) (structRefItem, error) { return newStructRefItem(refs[i]) })
}
