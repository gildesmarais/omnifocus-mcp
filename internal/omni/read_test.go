package omni

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type captureRunner struct {
	res string
	src string
}

func (c *captureRunner) Run(_ context.Context, src string) (string, error) {
	c.src = src
	return c.res, nil
}

func TestListTasksMarshalsFiltersAndDecodes(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a1","name":"X","status":"Available","available":true,"tags":[{"id":"t1","name":"home"}],"project":{"id":"p1","name":"P"},"due":"2026-11-18T08:00:00"}]`}
	yes := true
	got, err := (&Bridge{Runner: r}).ListTasks(context.Background(), TaskFilter{Available: &yes, Tag: "home", DueBefore: "2026-11-20"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Project.Name != "P" || got[0].Tags[0].ID != "t1" || got[0].Due != "2026-11-18T08:00:00" || !got[0].Available {
		t.Fatalf("bad decode %+v", got)
	}
	for _, want := range []string{`available`, `due_before`, `2026-11-20`, `limit`} {
		if !strings.Contains(r.src, want) {
			t.Errorf("source lacks %q: %s", want, r.src)
		}
	}
}

func TestListTasksRejectsBadInput(t *testing.T) {
	b := &Bridge{Runner: &captureRunner{res: `[]`}}
	neg := -1
	for _, f := range []TaskFilter{{DueBefore: "tomorrow"}, {DeferBefore: "2026-13-01"}, {Limit: &neg}} {
		if _, err := b.ListTasks(context.Background(), f); err == nil {
			t.Errorf("%+v: want error", f)
		}
	}
}

func TestListProjectsStatusValidation(t *testing.T) {
	r := &captureRunner{res: `[{"id":"p1","name":"P","status":"Active","tags":[],"folder":{"id":"f1","name":"F"}}]`}
	b := &Bridge{Runner: r}
	got, err := b.ListProjects(context.Background(), ProjectFilter{Status: "Active", Folder: "F"})
	if err != nil || len(got) != 1 || got[0].Folder.ID != "f1" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if _, err := b.ListProjects(context.Background(), ProjectFilter{Status: "Bogus"}); err == nil {
		t.Fatal("want status error")
	}
}

func TestListTagsAndFolders(t *testing.T) {
	tags, err := (&Bridge{Runner: &captureRunner{res: `[{"id":"t1","name":"a","status":"Active","remainingTasks":3}]`}}).ListTags(context.Background())
	if err != nil || len(tags) != 1 || tags[0].RemainingTasks != 3 {
		t.Fatalf("%+v %v", tags, err)
	}
	fs, err := (&Bridge{Runner: &captureRunner{res: `[{"id":"f1","name":"a","status":"Active","parent":{"id":"f0","name":"r"}}]`}}).ListFolders(context.Background())
	if err != nil || len(fs) != 1 || fs[0].Parent.ID != "f0" {
		t.Fatalf("%+v %v", fs, err)
	}
}

func TestGetters(t *testing.T) {
	ctx := context.Background()
	r := &captureRunner{res: `[{"id":"a1","name":"X","status":"Active","tags":[]}]`}
	b := &Bridge{Runner: r}
	if tk, err := b.GetTask(ctx, "a1"); err != nil || tk.ID != "a1" || !strings.Contains(r.src, `\"id\":\"a1\"`) {
		t.Fatalf("task %+v %v src %s", tk, err, r.src)
	}
	if p, err := b.GetProject(ctx, "a1"); err != nil || p.ID != "a1" {
		t.Fatalf("project %+v %v", p, err)
	}
	if g, err := b.GetTag(ctx, "a1"); err != nil || g.ID != "a1" {
		t.Fatalf("tag %+v %v", g, err)
	}
	if f, err := b.GetFolder(ctx, "a1"); err != nil || f.ID != "a1" {
		t.Fatalf("folder %+v %v", f, err)
	}
	empty := &Bridge{Runner: &captureRunner{res: `[]`}}
	if _, err := empty.GetTask(ctx, "zz"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := empty.GetFolder(ctx, ""); err == nil {
		t.Fatal("want id required error")
	}
}
