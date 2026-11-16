package omni

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// JS-only guards cannot run without OmniFocus; these assert the guard code is
// in the embedded scripts (the live tests exercise the behavior).
func TestScriptGuardsPresent(t *testing.T) {
	src := func(name string) string { return mustRead("js/" + name + ".js") }
	for name, wants := range map[string][]string{
		"prelude":    {"containingProject.task === t", "is a project, use kind project", "const inside = ", `if (kind === "task") return findTask(id)`},
		"edit_tasks": {"findTask(p.id)", `findTask(p.move.parent, "parent task")`, "cannot move task into itself or its subtask", "inside(t, par)"},
		"add_tasks":  {`findTask(a.parent, "parent task")`},
		"tasks":      {"isProjectRoot(t)", "flattenedTasks.filter(t => t.inInbox)"},
		"delete":     {"o.flattenedTasks", "o.flattenedChildren", "remaining(", "drop it instead"},
		"drop":       {"flattenedTags", "inside(o, g)", "remaining task(s)"},
	} {
		for _, w := range wants {
			if !strings.Contains(src(name), w) {
				t.Errorf("%s.js lacks %q", name, w)
			}
		}
	}
	for _, name := range []string{"edit_tasks", "add_tasks"} {
		if strings.Contains(src(name), "Task.byIdentifier") {
			t.Errorf("%s.js must look tasks up through findTask", name)
		}
	}
	if strings.Contains(src("tasks"), "pool = inbox;") {
		t.Error("inbox filter must not use the top-level-only inbox")
	}
}

func TestDatesAcceptOffsets(t *testing.T) {
	for _, ok := range []string{"2026-11-18", "2026-11-18T08:00:00", "2026-11-18T08:00:00Z", "2026-11-18T08:00:00+01:00", "2026-11-18T08:00:00-05:00"} {
		if err := checkDate("due", ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"2026-11-18T08:00:00+0100", "2026-11-18 08:00", "tomorrow"} {
		if checkDate("due", bad) == nil {
			t.Errorf("%s: want error", bad)
		}
	}
}

func TestInboxWithProjectRejected(t *testing.T) {
	r := &captureRunner{res: `[]`}
	yes, no := true, false
	if _, err := (&Bridge{Runner: r}).ListTasks(context.Background(), TaskFilter{Inbox: &yes, Project: "p"}); err == nil || r.src != "" {
		t.Fatalf("want validation error before any script, got %v src=%q", err, r.src)
	}
	if _, err := (&Bridge{Runner: r}).ListTasks(context.Background(), TaskFilter{Inbox: &no, Project: "p"}); err != nil {
		t.Fatalf("inbox:false with project is fine: %v", err)
	}
}

// route answers by script: the tags script gets tags, everything else res.
type route struct {
	tags, res string
	tagsErr   error
	scripts   []string
}

func (r *route) Run(_ context.Context, src string) (string, error) {
	if strings.Contains(src, "flattenedTags") {
		r.scripts = append(r.scripts, "tags")
		return r.tags, r.tagsErr
	}
	r.scripts = append(r.scripts, "write")
	return r.res, nil
}

func TestAddTasksMixedResults(t *testing.T) {
	r := &route{res: `[{"id":"n1","ok":true,"task":{"id":"n1","name":"A","status":"Available","tags":[]}},{"ok":false,"error":"project not found"}]`}
	got, err := (&Bridge{Runner: r}).AddTasks(context.Background(), []TaskAdd{{Name: "A"}, {Name: ""}, {Name: "B", Project: "gone"}})
	if err != nil || len(got) != 3 {
		t.Fatalf("%+v %v", got, err)
	}
	if !got[0].OK || got[0].ID != "n1" || got[1].OK || !strings.Contains(got[1].Error, "name") || got[2].OK || got[2].Error != "project not found" {
		t.Fatalf("%+v", got)
	}
}

func TestAmbiguousTagNameFailsItemOnly(t *testing.T) {
	r := &route{tags: `[{"id":"t3","name":"dup"},{"id":"t4","name":"dup"},{"id":"t1","name":"home"}]`, res: `[{"id":"n1","ok":true,"task":{"id":"n1","name":"B","status":"Available","tags":[]}}]`}
	got, err := (&Bridge{Runner: r}).AddTasks(context.Background(), []TaskAdd{{Name: "A", Tags: []string{"dup"}}, {Name: "B", Tags: []string{"home"}}})
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got[0].OK || !strings.Contains(got[0].Error, "ambiguous") || !got[1].OK {
		t.Fatalf("%+v", got)
	}
	got, err = (&Bridge{Runner: r}).EditTasks(context.Background(), []TaskEdit{decodeEdit(t, `{"id":"a","tags":{"add":["dup"]}}`)})
	if err != nil || got[0].OK || !strings.Contains(got[0].Error, "ambiguous") {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestTagLookupFailureFailsBatchBeforeWrite(t *testing.T) {
	boom := errors.New("tags exploded")
	r := &route{tagsErr: boom}
	if _, err := (&Bridge{Runner: r}).AddTasks(context.Background(), []TaskAdd{{Name: "A", Tags: []string{"home"}}}); !errors.Is(err, boom) {
		t.Fatalf("add: %v", err)
	}
	if _, err := (&Bridge{Runner: r}).EditTasks(context.Background(), []TaskEdit{decodeEdit(t, `{"id":"a","tags":{"remove":["home"]}}`)}); !errors.Is(err, boom) {
		t.Fatalf("edit: %v", err)
	}
	for _, s := range r.scripts {
		if s == "write" {
			t.Fatal("write script ran after tag lookup failed")
		}
	}
}
