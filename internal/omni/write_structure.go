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

// changesNothing reports a patch that sets no field besides kind and id.
func (p StructEdit) changesNothing() bool {
	p.Kind, p.ID = "", ""
	return reflect.ValueOf(p).IsZero()
}

// validAdd checks the add item and returns the validated input.
func validAdd(a StructAdd) (StructAdd, error) {
	if err := errors.Join(checkKind(a.Kind, kindProject, kindTag, kindFolder), nonEmpty("name", a.Name), checkStatus(a.Kind, a.Status)); err != nil {
		return StructAdd{}, err
	}
	if a.Sequential && a.Kind != kindProject {
		return StructAdd{}, errors.New("sequential: only for projects")
	}
	return a, nil
}

// validEdit checks the edit item and returns the validated input.
func validEdit(p StructEdit) (StructEdit, error) {
	if err := errors.Join(checkKind(p.Kind, kindProject, kindTag, kindFolder), nonEmpty("id", p.ID), checkStatus(p.Kind, p.Status)); err != nil {
		return StructEdit{}, err
	}
	if p.Name != nil {
		if err := nonEmpty("name", *p.Name); err != nil {
			return StructEdit{}, err
		}
	}
	if p.Sequential != nil && p.Kind != kindProject {
		return StructEdit{}, errors.New("sequential: only for projects")
	}
	if p.Move != nil && p.Reorder != nil {
		return StructEdit{}, errors.New("move and reorder are mutually exclusive")
	}
	if m := p.Move; m != nil {
		switch {
		case !m.Parent.Set:
			return StructEdit{}, errors.New("move: parent is required (an id, or null for the root)")
		case !m.Parent.Null && m.Parent.Value == "":
			return StructEdit{}, errors.New("move: empty parent; use null for the root")
		case !m.Parent.Null && m.Parent.Value == p.ID:
			return StructEdit{}, errors.New("move: an object cannot be its own parent")
		}
	}
	if r := p.Reorder; r != nil {
		if (r.Before == "") == (r.After == "") {
			return StructEdit{}, errors.New("reorder: exactly one of before or after")
		}
		if r.Before == p.ID || r.After == p.ID {
			return StructEdit{}, errors.New("reorder: an object cannot be its own sibling")
		}
	}
	if p.changesNothing() {
		return StructEdit{}, errors.New("patch changes nothing")
	}
	return p, nil
}

// validRef checks the ref item and returns the validated input.
func validRef(r StructRef) (StructRef, error) {
	if err := errors.Join(checkKind(r.Kind, kindTask, kindProject, kindTag, kindFolder), nonEmpty("id", r.ID)); err != nil {
		return StructRef{}, err
	}
	return r, nil
}

// AddStructure creates projects, tags and folders in one script call; results keep input order.
func (b *Bridge) AddStructure(ctx context.Context, adds []StructAdd) ([]StructResult, error) {
	if len(adds) == 0 {
		return nil, errors.New("items: at least one item is required")
	}
	return runBatch(ctx, b, scriptAddStructure, len(adds), nil, failStruct, func(i int) (StructAdd, error) { return validAdd(adds[i]) })
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
	return runBatch(ctx, b, scriptEditStructure, len(patches), ids, failStruct, func(i int) (StructEdit, error) { return validEdit(patches[i]) })
}

// DropObjects drops tasks, projects, tags or folders. Guards (a tag with
// remaining tasks) are enforced in the script, the only place that can see them.
func (b *Bridge) DropObjects(ctx context.Context, refs []StructRef) ([]StructResult, error) {
	return b.refBatch(ctx, scriptDrop, refs)
}

// DeleteObjects irreversibly deletes objects. Tags and folders must already be
// dropped and empty; the script enforces that.
func (b *Bridge) DeleteObjects(ctx context.Context, refs []StructRef) ([]StructResult, error) {
	return b.refBatch(ctx, scriptDelete, refs)
}

func (b *Bridge) refBatch(ctx context.Context, s script, refs []StructRef) ([]StructResult, error) {
	if len(refs) == 0 {
		return nil, errors.New("items: at least one item is required")
	}
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.ID
	}
	return runBatch(ctx, b, s, len(refs), ids, failStruct, func(i int) (StructRef, error) { return validRef(refs[i]) })
}
