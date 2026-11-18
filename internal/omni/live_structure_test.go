//go:build live

package omni

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// Creates everything it touches (folder "MCP Sandbox Folder", tags "mcp-test-*",
// a project and tasks inside "MCP Sandbox") and mutates only objects it created
// by id. Everything is deleted at the end; a final sweep reports leftovers.
func TestLiveStructureWrites(t *testing.T) {
	ctx := context.Background()
	b := New()
	sandbox := sandboxProject(t, b)
	suffix := time.Now().Format("150405")

	one := func(rs []StructResult, err error) StructResult {
		t.Helper()
		if err != nil || len(rs) != 1 {
			t.Fatalf("%+v %v", rs, err)
		}
		return rs[0]
	}
	mustOK := func(r StructResult) StructResult {
		t.Helper()
		if !r.OK {
			t.Fatalf("want ok: %+v", r)
		}
		return r
	}
	refuse := func(r StructResult, want string) {
		t.Helper()
		if r.OK || !strings.Contains(r.Error, want) {
			t.Fatalf("want refusal %q: %+v", want, r)
		}
	}
	add := func(a StructAdd) StructResult { t.Helper(); return mustOK(one(b.AddStructure(ctx, []StructAdd{a}))) }
	edit := func(e StructEdit) StructResult { t.Helper(); return mustOK(one(b.EditStructure(ctx, []StructEdit{e}))) }
	drop := func(k Kind, id string) StructResult { return one(b.DropObjects(ctx, []StructRef{{Kind: k, ID: id}})) }
	del := func(k Kind, id string) StructResult { return one(b.DeleteObjects(ctx, []StructRef{{Kind: k, ID: id}})) }

	var created []StructRef // fallback cleanup, newest first on use
	track := func(k Kind, id string) { created = append(created, StructRef{Kind: k, ID: id}) }
	t.Cleanup(func() {
		for _, c := range slices.Backward(created) {
			if c.Kind == kindTag || c.Kind == kindFolder {
				drop(c.Kind, c.ID)
			}
			del(c.Kind, c.ID) // refusals for already-deleted objects are expected
		}
		leftovers(t, b)
	})

	folder := add(StructAdd{Kind: "folder", Name: "MCP Sandbox Folder"})
	track("folder", folder.ID)
	parentTag := add(StructAdd{Kind: "tag", Name: "mcp-test-parent-" + suffix})
	track("tag", parentTag.ID)
	childTag := add(StructAdd{Kind: "tag", Name: "mcp-test-child-" + suffix, Parent: parentTag.ID})
	track("tag", childTag.ID)
	proj := add(StructAdd{Kind: "project", Name: "mcp-test-proj-" + suffix, Parent: folder.ID, Type: "Sequential"})
	track("project", proj.ID)

	if g, err := b.GetTag(ctx, childTag.ID); err != nil || g.Parent == nil || g.Parent.ID != parentTag.ID {
		t.Fatalf("child tag: %+v %v", g, err)
	}
	if p, err := b.GetProject(ctx, proj.ID); err != nil || p.Folder == nil || p.Folder.ID != folder.ID || p.Type != "Sequential" {
		t.Fatalf("project: %+v %v", p, err)
	}

	// rename + status
	edit(StructEdit{Kind: "project", ID: proj.ID, Name: new("mcp-test-proj-renamed-" + suffix), Status: "OnHold"})
	if p, _ := b.GetProject(ctx, proj.ID); p.Name != "mcp-test-proj-renamed-"+suffix || p.Status != "OnHold" {
		t.Fatalf("after rename/hold: %+v", p)
	}
	edit(StructEdit{Kind: "project", ID: proj.ID, Status: "Active"})
	if p, _ := b.GetProject(ctx, proj.ID); p.Status != "Active" {
		t.Fatalf("after reactivate: %+v", p)
	}
	edit(StructEdit{Kind: "folder", ID: folder.ID, Name: new("MCP Sandbox Folder")}) // rename path for folders
	edit(StructEdit{Kind: "tag", ID: childTag.ID, Status: "OnHold"})
	if g, _ := b.GetTag(ctx, childTag.ID); g.Status != "OnHold" {
		t.Fatalf("tag hold: %+v", g)
	}
	edit(StructEdit{Kind: "tag", ID: childTag.ID, Status: "Active"})

	// move project to root and back
	edit(StructEdit{Kind: "project", ID: proj.ID, Move: &StructMove{Parent: NullableString{Set: true, Null: true}}})
	if p, _ := b.GetProject(ctx, proj.ID); p.Folder != nil {
		t.Fatalf("project not at root: %+v", p)
	}
	edit(StructEdit{Kind: "project", ID: proj.ID, Move: &StructMove{Parent: NullableString{Set: true, Value: folder.ID}}})
	if p, _ := b.GetProject(ctx, proj.ID); p.Folder == nil || p.Folder.ID != folder.ID {
		t.Fatalf("project not back in folder: %+v", p)
	}

	// reorder tags: child to root next to parent, then back under it; cycle refused
	edit(StructEdit{Kind: "tag", ID: childTag.ID, Reorder: &StructReorder{After: parentTag.ID}})
	if g, _ := b.GetTag(ctx, childTag.ID); g.Parent != nil {
		t.Fatalf("reorder should adopt sibling's (root) parent: %+v", g)
	}
	edit(StructEdit{Kind: "tag", ID: childTag.ID, Move: &StructMove{Parent: NullableString{Set: true, Value: parentTag.ID}}})
	refuse(one(b.EditStructure(ctx, []StructEdit{{Kind: "tag", ID: parentTag.ID, Move: &StructMove{Parent: NullableString{Set: true, Value: childTag.ID}}}})), "itself")

	// tag drop refused while a task carries it, allowed after untag
	tr, err := b.AddTasks(ctx, []TaskAdd{{Name: "mcp-test-task-" + suffix, Project: sandbox, Tags: []string{childTag.ID}}})
	if err != nil || !tr[0].OK {
		t.Fatalf("task: %+v %v", tr, err)
	}
	taskID := tr[0].ID
	track("task", taskID)
	refuse(drop("tag", childTag.ID), "remaining")
	if g, _ := b.GetTag(ctx, childTag.ID); g.Status != "Active" || g.RemainingTasks != 1 {
		t.Fatalf("refused drop must not change the tag: %+v", g)
	}
	refuse(drop("tag", parentTag.ID), "remaining") // task is tagged only with the child tag
	if g, _ := b.GetTag(ctx, parentTag.ID); g.Status != "Active" {
		t.Fatalf("refused parent drop must not change the tag: %+v", g)
	}
	if rs, err := b.EditTasks(ctx, []TaskEdit{{ID: taskID, Tags: &TagEdit{Remove: []string{childTag.ID}}}}); err != nil || !rs[0].OK {
		t.Fatalf("untag: %+v %v", rs, err)
	}
	mustOK(drop("tag", childTag.ID))
	if g, _ := b.GetTag(ctx, childTag.ID); g.Status != "Dropped" {
		t.Fatalf("tag drop: %+v", g)
	}

	// tag delete refused while Active, allowed after drop (children first)
	refuse(del("tag", parentTag.ID), "not dropped")
	mustOK(drop("tag", parentTag.ID))
	refuse(del("tag", parentTag.ID), "not empty") // still has the dropped child
	mustOK(del("tag", childTag.ID))
	mustOK(del("tag", parentTag.ID))
	if _, err := b.GetTag(ctx, parentTag.ID); err == nil {
		t.Fatal("parent tag still exists")
	}

	// folder: delete refused until dropped and empty
	refuse(del("folder", folder.ID), "not dropped")
	mustOK(drop("folder", folder.ID))
	if f, _ := b.GetFolder(ctx, folder.ID); f.Status != "Dropped" {
		t.Fatalf("folder drop: %+v", f)
	}
	refuse(del("folder", folder.ID), "not empty")
	mustOK(drop("project", proj.ID))
	if p, _ := b.GetProject(ctx, proj.ID); p.Status != "Dropped" {
		t.Fatalf("project drop: %+v", p)
	}
	mustOK(del("project", proj.ID))
	if _, err := b.GetProject(ctx, proj.ID); err == nil {
		t.Fatal("project still exists")
	}
	mustOK(del("folder", folder.ID))
	mustOK(del("task", taskID))
	if _, err := b.GetTask(ctx, taskID); err == nil {
		t.Fatal("task still exists")
	}
}

// leftovers fails the test when any object this suite creates still exists.
func leftovers(t *testing.T, b *Bridge) {
	ctx := context.Background()
	n := 0
	tags, _ := b.ListTags(ctx)
	for _, g := range tags {
		if strings.HasPrefix(g.Name, "mcp-test-") {
			n++
		}
	}
	folders, _ := b.ListFolders(ctx)
	for _, f := range folders {
		if f.Name == "MCP Sandbox Folder" || strings.HasPrefix(f.Name, "mcp-test-") {
			n++
		}
	}
	projects, _ := b.ListProjects(ctx, ProjectFilter{Search: "mcp-test-"})
	for _, p := range projects {
		if strings.HasPrefix(p.Name, "mcp-test-") {
			n++
		}
	}
	t.Logf("leftovers (mcp-test- tags/projects, MCP Sandbox Folder): %d", n)
	if n > 0 {
		t.Errorf("%d leftovers", n)
	}
}

// Creates two projects "mcp-test-guard-*" with tasks and deletes them at the end.
func TestLiveKindAndDeleteGuards(t *testing.T) {
	ctx := context.Background()
	b := New()
	suffix := time.Now().Format("150405")
	var created []StructRef
	t.Cleanup(func() {
		for _, c := range slices.Backward(created) {
			b.DropObjects(ctx, []StructRef{c})
			b.DeleteObjects(ctx, []StructRef{c})
		}
		leftovers(t, b)
	})
	addProject := func(name string) string {
		t.Helper()
		rs, err := b.AddStructure(ctx, []StructAdd{{Kind: "project", Name: name}})
		if err != nil || !rs[0].OK {
			t.Fatalf("%+v %v", rs, err)
		}
		created = append(created, StructRef{Kind: "project", ID: rs[0].ID})
		return rs[0].ID
	}
	addTask := func(a TaskAdd) string {
		t.Helper()
		rs, err := b.AddTasks(ctx, []TaskAdd{a})
		if err != nil || !rs[0].OK {
			t.Fatalf("%+v %v", rs, err)
		}
		created = append(created, StructRef{Kind: "task", ID: rs[0].ID})
		return rs[0].ID
	}
	refuse := func(r StructResult, err error, want string) {
		t.Helper()
		if err != nil || r.OK || !strings.Contains(r.Error, want) {
			t.Fatalf("want refusal %q: %+v %v", want, r, err)
		}
	}
	p := addProject("mcp-test-guard-" + suffix)
	parent := addTask(TaskAdd{Name: "mcp-test-parent", Project: p})
	addTask(TaskAdd{Name: "mcp-test-child", Parent: parent})
	other := addTask(TaskAdd{Name: "mcp-test-other", Project: p, Due: "2026-12-01T08:00:00+01:00"})

	// ISO offsets are honored: the stored instant equals the offset time, reported local.
	want, _ := time.Parse(time.RFC3339, "2026-12-01T08:00:00+01:00")
	if x, err := b.GetTask(ctx, other); err != nil || x.Due != want.Local().Format("2006-01-02T15:04:05") {
		t.Fatalf("offset due: %+v want %s (%v)", x, want.Local(), err)
	}

	// A project id is not a task.
	if _, err := b.GetTask(ctx, p); err == nil || !strings.Contains(err.Error(), "is a project") {
		t.Fatalf("get_task(project id): %v", err)
	}
	yes := true
	rs, err := b.EditTasks(ctx, []TaskEdit{{ID: p, Flagged: &yes}, {ID: other, Move: &Move{Parent: p}}})
	if err != nil || rs[0].OK || rs[1].OK || !strings.Contains(rs[0].Error, "is a project") || !strings.Contains(rs[1].Error, "is a project") {
		t.Fatalf("edit_tasks: %+v %v", rs, err)
	}
	if rs, err := b.AddTasks(ctx, []TaskAdd{{Name: "mcp-test-x", Parent: p}}); err != nil || rs[0].OK || !strings.Contains(rs[0].Error, "is a project") {
		t.Fatalf("add_tasks parent: %+v %v", rs, err)
	}
	r, err := b.DropObjects(ctx, []StructRef{{Kind: "task", ID: p}})
	refuse(r[0], err, "is a project")
	r, err = b.DeleteObjects(ctx, []StructRef{{Kind: "task", ID: p}})
	refuse(r[0], err, "is a project")

	// Remaining work blocks delete.
	r, err = b.DeleteObjects(ctx, []StructRef{{Kind: "task", ID: parent}})
	refuse(r[0], err, "remaining subtask")
	r, err = b.DeleteObjects(ctx, []StructRef{{Kind: "project", ID: p}})
	refuse(r[0], err, "remaining task")
	if _, err := b.GetProject(ctx, p); err != nil {
		t.Fatalf("refused delete must keep the project: %v", err)
	}

	// Inbox filter: top-level and nested inbox tasks, never project tasks.
	in := true
	ts, err := b.ListTasks(ctx, TaskFilter{Inbox: &in})
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range ts {
		if !x.Inbox || x.Project != nil {
			t.Fatalf("non-inbox task in inbox list: %+v", x)
		}
	}

	// Dropping a project drops its tasks, so a dropped project deletes.
	if rs, err := b.DropObjects(ctx, []StructRef{{Kind: "project", ID: p}}); err != nil || !rs[0].OK {
		t.Fatalf("drop: %+v %v", rs, err)
	}
	if rs, err := b.DeleteObjects(ctx, []StructRef{{Kind: "project", ID: p}}); err != nil || !rs[0].OK {
		t.Fatalf("delete dropped project with tasks: %+v %v", rs, err)
	}
}

// Creates one project "mcp-test-review-*" at the root and deletes it at the end.
func TestLiveProjectReview(t *testing.T) {
	ctx := context.Background()
	b := New()
	name := "mcp-test-review-" + time.Now().Format("150405")
	rs, err := b.AddStructure(ctx, []StructAdd{{Kind: "project", Name: name}})
	if err != nil || !rs[0].OK {
		t.Fatalf("%+v %v", rs, err)
	}
	id := rs[0].ID
	t.Cleanup(func() {
		ref := []StructRef{{Kind: "project", ID: id}}
		b.DropObjects(ctx, ref)
		b.DeleteObjects(ctx, ref)
		leftovers(t, b)
	})
	edit := func(e StructEdit) {
		t.Helper()
		e.Kind, e.ID = "project", id
		if rs, err := b.EditStructure(ctx, []StructEdit{e}); err != nil || !rs[0].OK {
			t.Fatalf("%+v %v", rs, err)
		}
	}
	get := func() Project {
		t.Helper()
		p, err := b.GetProject(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	const layout = "2006-01-02T15:04:05"
	midnight := func(d time.Time) string {
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location()).Format(layout)
	}

	edit(StructEdit{ReviewInterval: &ReviewInterval{Steps: 2, Unit: "weeks"}})
	if ri := get().ReviewInterval; ri == nil || ri.Steps != 2 || ri.Unit != "weeks" {
		t.Fatalf("interval: %+v", ri)
	}

	now := time.Now()
	edit(StructEdit{Reviewed: true})
	p := get()
	if want := midnight(now.AddDate(0, 0, 14)); p.NextReview != want {
		t.Fatalf("nextReviewDate after reviewed: got %q want %q", p.NextReview, want)
	}
	if p.LastReview == "" || p.LastReview[:10] != now.Format("2006-01-02") {
		t.Fatalf("lastReviewDate: %q", p.LastReview)
	}

	edit(StructEdit{NextReviewDate: "2030-01-15T13:45:00"})
	if got := get().NextReview; got != "2030-01-15T00:00:00" {
		t.Fatalf("nextReviewDate truncates to midnight: got %q", got)
	}
	if ri := get().ReviewInterval; ri == nil || ri.Steps != 2 || ri.Unit != "weeks" {
		t.Fatalf("interval must survive a date edit: %+v", ri)
	}

	listed := func(before string) bool {
		t.Helper()
		ps, err := b.ListProjects(ctx, ProjectFilter{Search: name, ReviewDueBefore: before})
		if err != nil {
			t.Fatal(err)
		}
		return len(ps) == 1 && ps[0].ID == id
	}
	if !listed("2030-01-15") || !listed("2030-01-16T00:00:00") || listed("2030-01-14") {
		t.Fatal("review_due_before must include the due day and exclude earlier bounds")
	}

	if rs, err := b.EditStructure(ctx, []StructEdit{{Kind: "project", ID: id, ReviewInterval: &ReviewInterval{Steps: 1, Unit: "week"}}}); err != nil || rs[0].OK {
		t.Fatalf("singular unit must be refused: %+v %v", rs, err)
	}
}

// Creates folder "mcp-test-folder-*", tag "mcp-test-tag-*" and projects "mcp-test-*"
// inside it, mutates only those by id, and deletes them all at the end.
func TestLiveProjectWrites(t *testing.T) {
	ctx := context.Background()
	b := New()
	suffix := time.Now().Format("150405")

	var created []StructRef // cleanup, newest first on use
	t.Cleanup(func() {
		for _, c := range slices.Backward(created) {
			b.DropObjects(ctx, []StructRef{c})
			b.DeleteObjects(ctx, []StructRef{c}) // refusals for already-deleted objects are expected
		}
		leftovers(t, b)
	})
	add := func(a StructAdd) string {
		t.Helper()
		rs, err := b.AddStructure(ctx, []StructAdd{a})
		if err != nil || len(rs) != 1 || !rs[0].OK {
			t.Fatalf("add %+v: %+v %v", a, rs, err)
		}
		created = append(created, StructRef{Kind: a.Kind, ID: rs[0].ID})
		return rs[0].ID
	}
	edit := func(e StructEdit) {
		t.Helper()
		rs, err := b.EditStructure(ctx, []StructEdit{e})
		if err != nil || len(rs) != 1 || !rs[0].OK {
			t.Fatalf("edit %+v: %+v %v", e, rs, err)
		}
	}
	get := func(id string) Project {
		t.Helper()
		p, err := b.GetProject(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	hasTag := func(p Project, id string) bool {
		for _, g := range p.Tags {
			if g.ID == id {
				return true
			}
		}
		return false
	}

	folder := add(StructAdd{Kind: "folder", Name: "mcp-test-folder-" + suffix})
	tag := add(StructAdd{Kind: "tag", Name: "mcp-test-tag-" + suffix})

	// every type reads back as written, from add and across edits (both flags are written)
	for _, typ := range projectTypes {
		id := add(StructAdd{Kind: "project", Name: "mcp-test-type-" + typ + "-" + suffix, Parent: folder, Type: typ})
		if p := get(id); p.Type != typ {
			t.Fatalf("add type %s read back %s", typ, p.Type)
		}
	}
	id := add(StructAdd{Kind: "project", Name: "mcp-test-default-" + suffix, Parent: folder})
	if p := get(id); p.Type != "Parallel" || p.CompletedByChildren || p.EstimatedMinutes != nil || p.Repetition != nil {
		t.Fatalf("default project: %+v", p)
	}
	for _, typ := range []string{"SingleActions", "Sequential", "Parallel", "SingleActions", "Parallel"} {
		edit(StructEdit{Kind: "project", ID: id, Type: typ})
		if p := get(id); p.Type != typ {
			t.Fatalf("edit type %s read back %s", typ, p.Type)
		}
	}

	// every field on add
	rep := &Repetition{Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=FR", ScheduleType: "Regularly", Anchor: "DueDate", CatchUp: true}
	full := add(StructAdd{Kind: "project", Name: "mcp-test-full-" + suffix, Parent: folder,
		Type: "Sequential", Note: "first note", Flagged: true,
		Defer: "2030-03-01T08:00:00", Due: "2030-03-04T17:30:00", Planned: "2030-03-02T09:00:00",
		Tags: []string{tag}, EstimatedMinutes: new(30), Repetition: rep, CompletedByChildren: true})
	p := get(full)
	if p.Type != "Sequential" || p.Note != "first note" || !p.Flagged || p.Defer != "2030-03-01T08:00:00" || p.Due != "2030-03-04T17:30:00" || p.Planned != "2030-03-02T09:00:00" ||
		!hasTag(p, tag) || p.EstimatedMinutes == nil || *p.EstimatedMinutes != 30 || p.Repetition == nil || *p.Repetition != *rep || !p.CompletedByChildren {
		t.Fatalf("project after add: %+v", p)
	}

	// edit: replace every value, remove the tag
	rep2 := &Repetition{Rule: "FREQ=DAILY;INTERVAL=2", ScheduleType: "FromCompletion", Anchor: "DeferDate"}
	edit(StructEdit{Kind: "project", ID: full,
		Type: "SingleActions", Note: NullableString{Set: true, Value: "second note"}, Flagged: new(false),
		Defer:            NullableString{Set: true, Value: "2030-04-01T08:00:00"},
		Due:              NullableString{Set: true, Value: "2030-04-04"},
		Planned:          NullableString{Set: true, Value: "2030-04-02T09:00:00"},
		Tags:             &TagEdit{Remove: []string{tag}},
		EstimatedMinutes: Nullable[int]{Set: true, Value: 45}, Repetition: Nullable[Repetition]{Set: true, Value: *rep2}, CompletedByChildren: new(false)})
	p = get(full)
	if p.Type != "SingleActions" || p.Note != "second note" || p.Flagged || p.Defer != "2030-04-01T08:00:00" || p.Due != "2030-04-04T00:00:00" || p.Planned != "2030-04-02T09:00:00" ||
		hasTag(p, tag) || p.EstimatedMinutes == nil || *p.EstimatedMinutes != 45 || p.Repetition == nil || *p.Repetition != *rep2 || p.CompletedByChildren {
		t.Fatalf("project after edit: %+v", p)
	}
	edit(StructEdit{Kind: "project", ID: full, Tags: &TagEdit{Add: []string{tag}}})
	if !hasTag(get(full), tag) {
		t.Fatal("tag add")
	}

	// null clears dates, note, estimate and repetition; the rest stays
	none := NullableString{Set: true, Null: true}
	edit(StructEdit{Kind: "project", ID: full,
		Note: none, Defer: none, Due: none, Planned: none,
		EstimatedMinutes: Nullable[int]{Set: true, Null: true}, Repetition: Nullable[Repetition]{Set: true, Null: true}})
	p = get(full)
	if p.Note != "" || p.Defer != "" || p.Due != "" || p.Planned != "" || p.EstimatedMinutes != nil || p.Repetition != nil {
		t.Fatalf("project after clears: %+v", p)
	}
	if p.Type != "SingleActions" || !hasTag(p, tag) {
		t.Fatalf("clears must not touch other fields: %+v", p)
	}

	// a dropped folder is restored by edit status Active; its project is active again
	inner := add(StructAdd{Kind: "project", Name: "mcp-test-inner-" + suffix, Parent: folder})
	if rs, err := b.DropObjects(ctx, []StructRef{{Kind: "folder", ID: folder}}); err != nil || !rs[0].OK {
		t.Fatalf("drop folder: %+v %v", rs, err)
	}
	if f, err := b.GetFolder(ctx, folder); err != nil || f.Status != "Dropped" {
		t.Fatalf("folder after drop: %+v %v", f, err)
	}
	edit(StructEdit{Kind: "folder", ID: folder, Status: "Active"})
	if f, err := b.GetFolder(ctx, folder); err != nil || f.Status != "Active" {
		t.Fatalf("folder after restore: %+v %v", f, err)
	}
	if p := get(inner); p.Status != "Active" || !p.Available {
		t.Fatalf("project after folder restore: %+v", p)
	}
}
