package omni

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
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
	Kind   Kind   `json:"kind" jsonschema:"project, tag or folder"`
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty" jsonschema:"folder id (project, folder) or tag id (tag); omit for the root"`
	Status Status `json:"status,omitempty" jsonschema:"project: Active, OnHold, Done; tag: Active, OnHold"`

	ProjectAdd
}

// ProjectAdd holds the add fields only a project has. Names and meaning match
// add_tasks. Every field is a pointer or slice so that nil means "not sent":
// a tag or folder is rejected for any project field the client sent, whatever its value.
type ProjectAdd struct {
	Type    *string  `json:"type,omitempty" jsonschema:"project only: Parallel (default), Sequential or SingleActions"`
	Note    *string  `json:"note,omitempty" jsonschema:"project only"`
	Flagged *bool    `json:"flagged,omitempty" jsonschema:"project only"`
	Defer   *string  `json:"defer,omitempty" jsonschema:"project only"`
	Due     *string  `json:"due,omitempty" jsonschema:"project only"`
	Planned *string  `json:"planned,omitempty" jsonschema:"project only"`
	Tags    []string `json:"tags,omitempty" jsonschema:"project only: tag ids or unambiguous names"`

	EstimatedMinutes    *int        `json:"estimatedMinutes,omitempty" jsonschema:"project only: 0 or more"`
	Repetition          *Repetition `json:"repetition,omitempty" jsonschema:"project only"`
	CompletedByChildren *bool       `json:"completedByChildren,omitempty" jsonschema:"project only: completes when its last task does"`
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
	Kind    Kind           `json:"kind" jsonschema:"project, tag or folder"`
	ID      string         `json:"id"`
	Name    *string        `json:"name,omitempty"`
	Status  Status         `json:"status,omitempty" jsonschema:"project: Active, OnHold, Done; tag: Active, OnHold; folder: Active (restores a dropped folder)"`
	Move    *StructMove    `json:"move,omitempty" jsonschema:"parent: folder id (project, folder), tag id (tag) or null for the root"`
	Reorder *StructReorder `json:"reorder,omitempty" jsonschema:"mutually exclusive with move; the sibling's parent becomes the parent"`

	ProjectEdit
}

// ProjectEdit holds the edit fields only a project has. Names and meaning match
// edit_tasks: omitted is unchanged, null clears Note, Defer, Due, Planned,
// EstimatedMinutes and Repetition. As in ProjectAdd, nil or unset means "not sent".
type ProjectEdit struct {
	Type    *string        `json:"type,omitempty" jsonschema:"Parallel, Sequential or SingleActions"`
	Note    NullableString `json:"note,omitzero"`
	Flagged *bool          `json:"flagged,omitempty"`
	Defer   NullableString `json:"defer,omitzero"`
	Due     NullableString `json:"due,omitzero"`
	Planned NullableString `json:"planned,omitzero"`
	Tags    *TagEdit       `json:"tags,omitempty" jsonschema:"tag ids or unambiguous names to add and remove"`

	EstimatedMinutes    Nullable[int]        `json:"estimatedMinutes,omitzero" jsonschema:"0 or more; null clears"`
	Repetition          Nullable[Repetition] `json:"repetition,omitzero" jsonschema:"null clears"`
	CompletedByChildren *bool                `json:"completedByChildren,omitempty" jsonschema:"completes when its last task does"`

	ReviewInterval *ReviewInterval `json:"reviewInterval,omitempty" jsonschema:"cannot be cleared; does not move nextReviewDate"`
	NextReviewDate *string         `json:"nextReviewDate,omitempty" jsonschema:"OmniFocus truncates it to local midnight; not with reviewed"`
	Reviewed       *bool           `json:"reviewed,omitempty" jsonschema:"mark reviewed now; OmniFocus recomputes nextReviewDate from the interval"`
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

// Statuses a write may set per kind. A folder has no status on add; on edit,
// Active restores a dropped folder (dropping is the drop tool's job).
var (
	addStatuses  = map[Kind][]Status{kindProject: {statusActive, statusOnHold, statusDone}, kindTag: {statusActive, statusOnHold}}
	editStatuses = map[Kind][]Status{kindProject: {statusActive, statusOnHold, statusDone}, kindTag: {statusActive, statusOnHold}, kindFolder: {statusActive}}
)

// checkStatus validates a status for kind against allowed; empty means unset.
func checkStatus(allowed map[Kind][]Status, kind Kind, status Status) error {
	switch ok := allowed[kind]; {
	case status == "":
		return nil
	case len(ok) == 0:
		return fmt.Errorf("status: not supported for %s", kind)
	case !slices.Contains(ok, status):
		return fmt.Errorf("status: %q is not one of %v for a %s", status, ok, kind)
	}
	return nil
}

// errProjectOnly rejects the project-only struct v (ProjectAdd or ProjectEdit)
// on another kind, naming the JSON fields the client sent. Sent means non-zero:
// every field is a pointer, slice or Nullable, so only an omitted one is zero.
// Walking the fields keeps new project fields covered without a second list.
func errProjectOnly(kind Kind, v any) error {
	rv := reflect.ValueOf(v)
	var sent []string
	for i := range rv.NumField() {
		if !rv.Field(i).IsZero() {
			name, _, _ := strings.Cut(rv.Type().Field(i).Tag.Get("json"), ",")
			sent = append(sent, name)
		}
	}
	if len(sent) == 0 {
		return nil
	}
	return fmt.Errorf("%s: only for projects, not %s", strings.Join(sent, ", "), kind)
}

// text is the value of an optional string field; unset is empty.
func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// changesNothing reports a patch that sets no field besides kind and id.
func (p StructEdit) changesNothing() bool {
	p.Kind, p.ID = "", ""
	return reflect.ValueOf(p).IsZero()
}

// check validates the add fields that need no lookup.
func (a ProjectAdd) check() error {
	errs := []error{checkDate("defer", text(a.Defer)), checkDate("due", text(a.Due)), checkDate("planned", text(a.Planned))}
	if a.Type != nil {
		errs = append(errs, checkProjectType(*a.Type))
	}
	if a.EstimatedMinutes != nil {
		errs = append(errs, checkEstimate(*a.EstimatedMinutes))
	}
	if a.Repetition != nil {
		errs = append(errs, a.Repetition.check())
	}
	return errors.Join(errs...)
}

// check validates the edit fields that need no lookup: dates, type, estimate,
// repetition and the review fields (reviewed excludes nextReviewDate, which OmniFocus derives).
func (p ProjectEdit) check() error {
	errs := []error{checkEditDate("defer", p.Defer), checkEditDate("due", p.Due), checkEditDate("planned", p.Planned), checkDate("nextReviewDate", text(p.NextReviewDate))}
	if p.Type != nil {
		errs = append(errs, checkProjectType(*p.Type))
	}
	if m := p.EstimatedMinutes; m.Set && !m.Null {
		errs = append(errs, checkEstimate(m.Value))
	}
	if r := p.Repetition; r.Set && !r.Null {
		errs = append(errs, r.Value.check())
	}
	if p.ReviewInterval != nil {
		errs = append(errs, p.ReviewInterval.check())
	}
	if p.Reviewed != nil && *p.Reviewed && text(p.NextReviewDate) != "" {
		errs = append(errs, errors.New("reviewed and nextReviewDate are mutually exclusive: marking reviewed sets nextReviewDate"))
	}
	return errors.Join(errs...)
}

// validAdd checks the add item and returns the script form: tags resolved to ids.
func validAdd(a StructAdd, tags []Tag) (StructAdd, error) {
	if err := errors.Join(checkKind(a.Kind, kindProject, kindTag, kindFolder), nonEmpty("name", a.Name), checkStatus(addStatuses, a.Kind, a.Status)); err != nil {
		return StructAdd{}, err
	}
	if a.Kind != kindProject {
		if err := errProjectOnly(a.Kind, a.ProjectAdd); err != nil {
			return StructAdd{}, err
		}
		return a, nil
	}
	ids, err := resolveTags(a.Tags, tags)
	if err := errors.Join(a.ProjectAdd.check(), err); err != nil {
		return StructAdd{}, err
	}
	a.Tags = ids
	return a, nil
}

// validEdit checks the edit item and returns the script form: tags resolved to ids.
func validEdit(p StructEdit, tags []Tag) (StructEdit, error) {
	if err := errors.Join(checkKind(p.Kind, kindProject, kindTag, kindFolder), nonEmpty("id", p.ID), checkStatus(editStatuses, p.Kind, p.Status)); err != nil {
		return StructEdit{}, err
	}
	if p.Name != nil {
		if err := nonEmpty("name", *p.Name); err != nil {
			return StructEdit{}, err
		}
	}
	if p.Kind != kindProject {
		if err := errProjectOnly(p.Kind, p.ProjectEdit); err != nil {
			return StructEdit{}, err
		}
	}
	if err := p.ProjectEdit.check(); err != nil {
		return StructEdit{}, err
	}
	if p.Tags != nil {
		te, err := resolveTagEdit(*p.Tags, tags)
		if err != nil {
			return StructEdit{}, err
		}
		p.Tags = nil
		if len(te.Add)+len(te.Remove) > 0 {
			p.Tags = &te
		}
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
	need := false
	for _, a := range adds {
		need = need || len(a.Tags) > 0
	}
	tags, err := b.tagsIf(ctx, need)
	if err != nil {
		return nil, err
	}
	return runBatch(ctx, b, scriptAddStructure, len(adds), nil, failStruct, func(i int) (StructAdd, error) { return validAdd(adds[i], tags) })
}

// EditStructure patches projects, tags and folders, with the same result contract as AddStructure.
func (b *Bridge) EditStructure(ctx context.Context, patches []StructEdit) ([]StructResult, error) {
	if len(patches) == 0 {
		return nil, errors.New("items: at least one item is required")
	}
	need := false
	ids := make([]string, len(patches))
	for i, p := range patches {
		ids[i] = p.ID
		need = need || p.Tags != nil
	}
	tags, err := b.tagsIf(ctx, need)
	if err != nil {
		return nil, err
	}
	return runBatch(ctx, b, scriptEditStructure, len(patches), ids, failStruct, func(i int) (StructEdit, error) { return validEdit(patches[i], tags) })
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
