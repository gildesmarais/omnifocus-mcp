//go:build live

package omni

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Read-only: requires a running OmniFocus with Automation permission.
func TestLivePerspectives(t *testing.T) {
	ps, err := New().Perspectives(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) == 0 {
		t.Fatal("expected at least the built-in perspectives")
	}
	t.Logf("%d perspectives, first: %+v", len(ps), ps[0])
}

// Counts only: no names are logged.
func TestLiveReads(t *testing.T) {
	ctx := context.Background()
	b := New()
	yes := true
	timed := func(name string, f func() int) {
		t0 := time.Now()
		n := f()
		t.Logf("%-22s count=%d wall=%s", name, n, time.Since(t0).Round(time.Millisecond))
	}
	var all []Task
	timed("list_tasks inbox", func() int {
		ts, err := b.ListTasks(ctx, TaskFilter{Inbox: &yes})
		if err != nil {
			t.Fatal(err)
		}
		return len(ts)
	})
	timed("list_tasks available", func() int {
		ts, err := b.ListTasks(ctx, TaskFilter{Available: &yes})
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range ts {
			if !x.Available || x.Completed || x.Dropped {
				t.Fatalf("task %s not available", x.ID)
			}
		}
		return len(ts)
	})
	timed("list_tasks all", func() int {
		var err error
		if all, err = b.ListTasks(ctx, TaskFilter{}); err != nil {
			t.Fatal(err)
		}
		return len(all)
	})
	timed("list_tasks default 50", func() int {
		f := 50
		ts, err := b.ListTasks(ctx, TaskFilter{Limit: &f})
		if err != nil || len(ts) > 50 {
			t.Fatalf("%d %v", len(ts), err)
		}
		return len(ts)
	})
	for _, x := range all {
		if x.EstimatedMinutes != nil && *x.EstimatedMinutes <= 0 {
			t.Fatalf("task %s estimatedMinutes %d", x.ID, *x.EstimatedMinutes)
		}
		if x.Repetition != nil && (!strings.HasPrefix(x.Repetition.Rule, "FREQ=") || x.Repetition.Method == "") {
			t.Fatalf("task %s repetition %+v", x.ID, *x.Repetition)
		}
	}
	if len(all) > 0 {
		x, err := b.GetTask(ctx, all[0].ID)
		if err != nil || x.ID != all[0].ID || x.Name != all[0].Name {
			t.Fatalf("get_task round trip: %+v %v", x, err)
		}
		t.Log("get_task round trip ok")
	}
	var ps []Project
	timed("list_projects", func() int {
		var err error
		if ps, err = b.ListProjects(ctx, ProjectFilter{}); err != nil {
			t.Fatal(err)
		}
		return len(ps)
	})
	if len(ps) > 0 {
		if p, err := b.GetProject(ctx, ps[0].ID); err != nil || p.ID != ps[0].ID {
			t.Fatalf("get_project %v", err)
		}
	}
	timed("list_projects Active", func() int {
		a, err := b.ListProjects(ctx, ProjectFilter{Status: "Active"})
		if err != nil {
			t.Fatal(err)
		}
		return len(a)
	})
	var tags []Tag
	timed("list_tags", func() int {
		var err error
		if tags, err = b.ListTags(ctx); err != nil {
			t.Fatal(err)
		}
		return len(tags)
	})
	if len(tags) > 0 {
		if g, err := b.GetTag(ctx, tags[0].ID); err != nil || g.ID != tags[0].ID {
			t.Fatalf("get_tag %v", err)
		}
	}
	var fs []Folder
	timed("list_folders", func() int {
		var err error
		if fs, err = b.ListFolders(ctx); err != nil {
			t.Fatal(err)
		}
		return len(fs)
	})
	if len(fs) > 0 {
		if f, err := b.GetFolder(ctx, fs[0].ID); err != nil || f.ID != fs[0].ID {
			t.Fatalf("get_folder %v", err)
		}
	}
	if _, err := b.GetTask(ctx, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
