//go:build live

package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// Writes only inside the project named "MCP Sandbox" (created if missing) and
// deletes the tasks it created at the end.
func TestLiveTaskWrites(t *testing.T) {
	ctx := context.Background()
	b := New()

	sandbox := sandboxProject(t, b)

	tags, err := b.ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, g := range tags {
		count[g.Name]++
	}
	var tag Tag
	for _, g := range tags {
		if count[g.Name] == 1 && g.Status == "Active" {
			tag = g
			break
		}
	}
	if tag.ID == "" {
		t.Skip("no uniquely named active tag")
	}

	timed := func(name string, f func() []TaskResult) []TaskResult {
		t0 := time.Now()
		rs := f()
		t.Logf("%-28s items=%d wall=%s", name, len(rs), time.Since(t0).Round(time.Millisecond))
		return rs
	}
	okAll := func(rs []TaskResult, n int) {
		t.Helper()
		if len(rs) != n {
			t.Fatalf("want %d results: %+v", n, rs)
		}
		for _, r := range rs {
			if !r.OK || r.Task == nil || r.Task.ID != r.ID {
				t.Fatalf("result not ok: %+v", r)
			}
		}
	}
	edit := func(name string, ps ...TaskEdit) []TaskResult {
		return timed(name, func() []TaskResult {
			rs, err := b.EditTasks(ctx, ps)
			if err != nil {
				t.Fatal(err)
			}
			return rs
		})
	}
	get := func(id string) Task {
		t.Helper()
		x, err := b.GetTask(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return x
	}
	str := func(s string) NullableString { return NullableString{Set: true, Value: s} }
	null := NullableString{Set: true, Null: true}
	yes, name1 := true, fmt.Sprintf("mcp-live-%d", time.Now().UnixNano())

	adds := timed("add_tasks x2", func() []TaskResult {
		rs, err := b.AddTasks(ctx, []TaskAdd{
			{Name: name1 + "-a", Project: sandbox, Note: "n1", Due: "2026-12-01"},
			{Name: name1 + "-b", Project: sandbox},
		})
		if err != nil {
			t.Fatal(err)
		}
		return rs
	})
	okAll(adds, 2)
	a, c := adds[0].ID, adds[1].ID
	t.Cleanup(func() {
		rs, err := b.DeleteObjects(ctx, []StructRef{{Kind: "task", ID: a}, {Kind: "task", ID: c}})
		if err != nil || !rs[0].OK || !rs[1].OK {
			t.Errorf("cleanup: %+v %v", rs, err)
		}
	})
	if x := get(a); x.Project == nil || x.Project.ID != sandbox || x.Note != "n1" || x.Due != "2026-12-01T00:00:00" {
		t.Fatalf("added task: %+v", x)
	}

	// Batch with one bad item (unknown id) and one rejected before the script (bad date).
	rs := edit("edit: rename/flag/defer/tag", TaskEdit{ID: a, Name: new(name1 + "-A"), Flagged: &yes, Defer: str("2026-12-05T09:30:00")},
		TaskEdit{ID: "does-not-exist", Flagged: &yes}, TaskEdit{ID: c, Due: str("soon")},
		TaskEdit{ID: c, Tags: &TagEdit{Add: []string{tag.Name}}})
	if !rs[0].OK || rs[1].OK || rs[2].OK || !rs[3].OK {
		t.Fatalf("per-item results: %+v", rs)
	}
	if x := get(a); x.Name != name1+"-A" || !x.Flagged || x.Defer != "2026-12-05T09:30:00" {
		t.Fatalf("after edit 1: %+v", x)
	}
	if x := get(c); len(x.Tags) != 1 || x.Tags[0].ID != tag.ID {
		t.Fatalf("tag add: %+v", x.Tags)
	}

	okAll(edit("edit: clear defer/untag", TaskEdit{ID: a, Defer: null, Note: null, Flagged: new(false)}, TaskEdit{ID: c, Tags: &TagEdit{Remove: []string{tag.ID}}}), 2)
	if x := get(a); x.Defer != "" || x.Note != "" || x.Flagged || x.Due == "" {
		t.Fatalf("after clear: %+v", x)
	}
	if x := get(c); len(x.Tags) != 0 {
		t.Fatalf("tag remove: %+v", x.Tags)
	}

	okAll(edit("edit: move to inbox", TaskEdit{ID: a, Move: &Move{Inbox: true}}), 1)
	if x := get(a); !x.Inbox || x.Project != nil {
		t.Fatalf("inbox: %+v", x)
	}
	okAll(edit("edit: move to project", TaskEdit{ID: a, Move: &Move{Project: sandbox}}), 1)
	if x := get(a); x.Inbox || x.Project == nil || x.Project.ID != sandbox {
		t.Fatalf("project: %+v", x)
	}
	okAll(edit("edit: move under parent", TaskEdit{ID: a, Move: &Move{Parent: c}}), 1)
	if x := get(a); x.Parent == nil || x.Parent.ID != c {
		t.Fatalf("parent: %+v", x)
	}

	okAll(edit("edit: complete", TaskEdit{ID: a, Complete: true}), 1)
	if x := get(a); !x.Completed {
		t.Fatalf("complete: %+v", x)
	}
	okAll(edit("edit: uncomplete", TaskEdit{ID: a, Uncomplete: true}), 1)
	if x := get(a); x.Completed {
		t.Fatalf("uncomplete: %+v", x)
	}
}

// sandboxProject returns the id of the project "MCP Sandbox", creating it if missing.
func sandboxProject(t *testing.T, b *Bridge) string {
	t.Helper()
	raw, err := b.Runner.Run(context.Background(), `JSON.stringify((function () {
		let p = flattenedProjects.find(p => p.name === "MCP Sandbox");
		if (!p) p = new Project("MCP Sandbox");
		return p.id.primaryKey;
	})())`)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := json.Unmarshal([]byte(raw), &id); err != nil || id == "" {
		t.Fatalf("sandbox id %q %v", raw, err)
	}
	return id
}
